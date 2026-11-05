package sendspin

import (
	"context"
	"encoding/base64"
	"log/slog"
	"math"
	"slices"
	"sync"
	"time"

	"github.com/Sendspin/sendspin-go/pkg/protocol"
	ssync "github.com/Sendspin/sendspin-go/pkg/sync"
	"github.com/gorilla/websocket"

	"github.com/ygelfand/libcountertop/pkg/runtime/safe"
)

// Clock sync runs in bursts, matching the reference player.
const (
	syncEvery   = 10 * time.Second
	syncBurst   = 8
	syncTimeout = 500 * time.Millisecond
)

// sendspin-go exports no protocol version constant and does not default it; zero is refused.
const protocolVersion = 1

type Session struct {
	client    *protocol.Client
	clock     *ssync.ClockSync
	out       *Output
	bg        Arbitration
	callbacks Callbacks

	dec    decoder
	chunks int

	lastTS int64
	opened bool
	muted  bool

	mu sync.Mutex

	commands []string
}

func newSession(
	conn *websocket.Conn,
	o *Output,
	bg Arbitration,
	name string,
	identity Identity,
	callbacks Callbacks,
) *Session {
	artworkSize := identity.ArtworkSize
	if artworkSize <= 0 {
		artworkSize = 512
	}
	offered := make([]protocol.AudioFormat, 0, len(formats()))
	for _, f := range formats() {
		offered = append(offered, protocol.AudioFormat{
			Codec: f.Codec, Channels: f.Channels, SampleRate: f.SampleRate, BitDepth: f.BitDepth,
		})
	}
	mac := identity.ID

	client := protocol.NewClientFromConn(protocol.Config{
		Name: name,

		Version: protocolVersion,

		SupportedRoles: []string{"player@v1", "metadata@v1", "controller@v1", "artwork@v1"},

		ClientID: mac,
		DeviceInfo: protocol.DeviceInfo{
			ProductName:     identity.Model,
			Manufacturer:    identity.Manufacturer,
			SoftwareVersion: identity.Version,
		},
		PlayerV1Support: protocol.PlayerV1Support{
			SupportedFormats:  offered,
			BufferCapacity:    bufferCapacity,
			SupportedCommands: []string{"volume", "mute"},
		},
		ArtworkV1Support: &protocol.ArtworkV1Support{
			Channels: []protocol.ArtworkChannel{{
				Source:      "album",
				Format:      "jpeg",
				MediaWidth:  artworkSize,
				MediaHeight: artworkSize,
			}},
		},
	}, conn)

	return &Session{
		client:    client,
		clock:     ssync.NewClockSync(),
		out:       o,
		bg:        bg,
		callbacks: callbacks,
	}
}

// The spec counts buffer capacity in bytes, one number for every format.
const bufferSeconds = 30

const bufferCapacity = bufferSeconds * Rate * Channels * Bits / 8

func (s *Session) run(ctx context.Context) error {
	defer s.finish()

	if err := s.client.Start(); err != nil {
		return err
	}

	s.reported()
	s.out.use(s.clock)
	safe.Go("sendspin clock", func() { s.synced(ctx) })

	for {
		select {
		case <-ctx.Done():
			_ = s.client.SendGoodbye("user_request")
			return nil

		case <-s.client.Done():
			return nil

		case start, ok := <-s.client.StreamStart:
			if !ok {
				return nil
			}
			s.began(start)

		// Music Assistant ends the stream instead; the spec has this for seeks.
		case <-s.client.StreamClear:
			s.cleared()

		case <-s.client.StreamEnd:
			late, dropped := s.out.misses()
			slog.Debug(
				"sendspin stream end",
				"queued_ms",
				s.out.queuedMs(),
				"late",
				late,
				"dropped",
				dropped,
			)
			s.ended()

		case chunk, ok := <-s.client.AudioChunks:
			if !ok {
				return nil
			}
			s.heard(chunk)

		case cmd := <-s.client.ControlMsgs:
			s.told(cmd)

		case g := <-s.client.GroupUpdate:
			s.grouped(g)

		case st := <-s.client.ServerState:
			s.said(st)

		case art := <-s.client.ArtworkChunks:
			s.callbacks.artwork(art)
		}
	}
}

func (s *Session) began(start protocol.StreamStart) {
	if start.Player == nil {
		return
	}
	p := start.Player

	header, err := base64.StdEncoding.DecodeString(p.CodecHeader)
	if err != nil {
		slog.Error("sendspin codec header", "codec", p.Codec, "err", err)
		return
	}

	dec, err := decoderFor(p.Codec, p.SampleRate, p.Channels, p.BitDepth, header)
	if err != nil {
		slog.Error("sendspin cannot play what was offered", "codec", p.Codec,
			"rate", p.SampleRate, "ch", p.Channels, "bits", p.BitDepth, "err", err)
		return
	}
	if err := s.out.open(p.SampleRate, p.Channels, p.BitDepth); err != nil {
		slog.Error("sendspin", "err", err)
		return
	}

	first := s.dec == nil
	if !first {
		if err := s.dec.close(); err != nil {
			slog.Warn("sendspin decoder close", "err", err)
		}
	}
	s.dec = dec
	s.opened = true
	s.callbacks.began()
	if first {
		s.bg.Took(s.out)
		s.callbacks.state(statePlaying)
	}
	slog.Debug(
		"sendspin stream",
		"codec",
		p.Codec,
		"rate",
		p.SampleRate,
		"ch",
		p.Channels,
		"bits",
		p.BitDepth,
	)
}

func (s *Session) cleared() {
	drained := 0
	for {
		select {
		case <-s.client.AudioChunks:
			drained++
		default:
			slog.Debug("sendspin clear", "undecoded", drained, "queued_ms", s.out.queuedMs())
			s.out.flush()
			return
		}
	}
}

// Group update fields are deltas: an absent state means unchanged.
func (s *Session) grouped(g protocol.GroupUpdate) {
	if g.PlaybackState == nil {
		return
	}
	slog.Info("sendspin group", "state", *g.PlaybackState, "queued_ms", s.out.queuedMs())
	s.callbacks.grouped(*g.PlaybackState)
}

func (s *Session) said(st protocol.ServerStateMessage) {
	if c := st.Controller; c != nil {
		s.mu.Lock()
		s.commands = c.SupportedCommands
		s.mu.Unlock()
		slog.Debug("sendspin server commands", "commands", c.SupportedCommands)
		s.callbacks.controller(*c)
	}

	if m := st.Metadata; m != nil {
		s.callbacks.metadata(m, s.clock.ServerMicrosNow)
	}

}

func (s *Session) tell(want ...string) bool {
	s.mu.Lock()
	can := s.commands
	s.mu.Unlock()

	for _, cmd := range want {
		if !slices.Contains(can, cmd) {
			continue
		}
		if err := s.client.Send("client/command", map[string]any{
			"controller": map[string]string{"command": cmd},
		}); err != nil {
			slog.Error("sendspin command", "command", cmd, "err", err)
			return false
		}
		slog.Info("sendspin command", "command", cmd)
		return true
	}

	slog.Warn("sendspin will not take this", "wanted", want, "server takes", can)
	return false
}

// The spec has stream/end stop output and clear buffers; servers send it on stop, skip and seek.
func (s *Session) ended() {
	if s.dec == nil {
		return
	}
	if err := s.dec.close(); err != nil {
		slog.Warn("sendspin decoder close", "err", err)
	}
	s.dec = nil
	s.cleared()
	s.out.close()
	s.bg.Gave(s.out)
	s.callbacks.state(stateJoined)
}

func (s *Session) heard(chunk protocol.AudioChunk) {
	if s.dec == nil {
		return
	}

	pcm, err := s.dec.decode(chunk.Data)
	if err != nil {
		slog.Warn("sendspin decode", "err", err)
		return
	}
	if len(pcm) == 0 {
		return
	}
	s.out.write(chunk.Timestamp, pcm)

	if s.opened {
		s.opened = false
		slog.Debug("sendspin stream first chunk",
			"lead_ms", (chunk.Timestamp-s.clock.ServerMicrosNow())/1000,
			"prev_last_lead_ms", (s.lastTS-s.clock.ServerMicrosNow())/1000,
			"overlap_ms", (s.lastTS-chunk.Timestamp)/1000,
			"queued_ms", s.out.queuedMs())
	}
	s.lastTS = chunk.Timestamp

	if s.chunks++; s.chunks%250 == 0 {
		late, dropped := s.out.misses()
		drift, corrected := s.out.drifting()
		slog.Debug("sendspin ahead",
			"queued_ms", s.out.queuedMs(),
			"undecoded", len(s.client.AudioChunks),
			"lead_ms", (chunk.Timestamp-s.clock.ServerMicrosNow())/1000,
			"late", late,
			"dropped", dropped,
			"drift_ms", drift*1000/Rate,
			"corrected", corrected)
	}
}

func (s *Session) told(cmd protocol.PlayerCommand) {
	switch cmd.Command {
	case "volume":
		s.out.setVolume(cmd.Volume)
	case "mute":
		s.muted = cmd.Mute
		s.out.setMuted(cmd.Mute)
	}
	s.reported()
}

func (s *Session) reported() {
	if err := s.client.SendState(protocol.PlayerState{
		State:  "synchronized",
		Volume: s.out.volume(),
		Muted:  s.muted,
	}); err != nil {
		slog.Debug("sendspin client state", "err", err)
	}
}

func (s *Session) synced(ctx context.Context) {
	ticker := time.NewTicker(syncEvery)
	defer ticker.Stop()

	for {
		s.measure(ctx)

		select {
		case <-ctx.Done():
			return
		case <-s.client.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Session) measure(ctx context.Context) {
drain:
	for {
		select {
		case <-s.client.TimeSyncResp:
		default:
			break drain
		}
	}

	var best protocol.ServerTime
	var arrived int64
	least := int64(math.MaxInt64)

	for range syncBurst {
		sent := micros()
		if err := s.client.SendTimeSync(sent); err != nil {
			slog.Debug("sendspin time sync", "err", err)
			return
		}

		timeout := time.NewTimer(syncTimeout)
		select {
		case <-ctx.Done():
			timeout.Stop()
			return
		case reply := <-s.client.TimeSyncResp:
			timeout.Stop()
			back := micros()
			rtt := (back - reply.ClientTransmitted) - (reply.ServerTransmitted - reply.ServerReceived)
			if rtt < least {
				least, best, arrived = rtt, reply, back
			}
		case <-timeout.C:
		}
	}

	if least == math.MaxInt64 {
		slog.Debug("sendspin time sync", "err", "no reply in the burst")
		return
	}
	s.clock.ProcessSyncResponse(
		best.ClientTransmitted,
		best.ServerReceived,
		best.ServerTransmitted,
		arrived,
	)
}

func (s *Session) finish() {
	s.ended()
	s.client.Close()
}

func micros() int64 { return time.Now().UnixMicro() }

func (s *Session) Close() { s.client.Close() }

func (s *Session) Command(want ...string) bool { return s.tell(want...) }

func (s *Session) Takes(cmd ...string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range cmd {
		if !slices.Contains(s.commands, c) {
			return false
		}
	}
	return true
}
