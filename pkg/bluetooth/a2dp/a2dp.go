package a2dp

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	esphome "github.com/ygelfand/go-esphome-device"

	"github.com/ygelfand/libcountertop/pkg/audio/analysis"
	speaker "github.com/ygelfand/libcountertop/pkg/audio/resample"
	"github.com/ygelfand/libcountertop/pkg/audio/volume"
	bt "github.com/ygelfand/libcountertop/pkg/bluetooth"
	"github.com/ygelfand/libcountertop/pkg/bluetooth/avdtp"
	"github.com/ygelfand/libcountertop/pkg/bluetooth/avrcp"
	"github.com/ygelfand/libcountertop/pkg/bluetooth/capture"
	"github.com/ygelfand/libcountertop/pkg/bluetooth/hci"
	"github.com/ygelfand/libcountertop/pkg/bluetooth/l2cap"
	"github.com/ygelfand/libcountertop/pkg/bluetooth/pair"
	"github.com/ygelfand/libcountertop/pkg/bluetooth/sbc"
	"github.com/ygelfand/libcountertop/pkg/bluetooth/sdp"
	"github.com/ygelfand/libcountertop/pkg/media/nowplaying"
	"github.com/ygelfand/libcountertop/pkg/media/pcm"
	"github.com/ygelfand/libcountertop/pkg/say"
	setting "github.com/ygelfand/libcountertop/pkg/settings"
	"github.com/ygelfand/libcountertop/pkg/settings/schema"
	"github.com/ygelfand/libcountertop/pkg/settings/storage"
)

type Sink struct {
	options Dependencies
	art     session
	dumping *capture.Dump
	mu      sync.Mutex

	links *connections // host service
	owner *connections // per-connection sink

	sink  *bt.Sink
	pairs *pair.Manager

	filter sbc.Filter

	resample *speaker.Rational
	drift    speaker.Drift

	knobs *setting.Controls[schema.Bluetooth]

	wake chan struct{}

	stop context.CancelFunc

	send   func(handle uint16, pdu []byte) error
	handle uint16

	track avrcp.Track
	label string

	queue   []avrcp.Item
	handles map[uint64]string

	asks    []uint64
	at      uint64
	counter uint16
	walked  time.Time

	later   []uint64
	waiting uint16

	asked int
	got   int

	asking time.Time

	answering bool
	held      [][]byte

	down bool
	gain float32

	streaming bool

	frames  int
	bad     int
	samples int
	why     error
	said    time.Time
}

func New(options Dependencies) *Sink {
	s := &Sink{options: options, dumping: capture.New(options.CaptureDirectory),
		wake: make(chan struct{}, 1),
	}
	s.knobs = &setting.Controls[schema.Bluetooth]{
		Table: NewTable(),
		Read:  options.Settings.Read,
		Save:  s.store,
	}
	return s
}

const Group setting.Group = "Bluetooth"

func NewTable() *setting.Table[schema.Bluetooth] {
	return setting.NewTable(
		"bluetooth",
		[]setting.Group{Group},
		[]setting.Setting[schema.Bluetooth]{
			setting.Flag(
				setting.Setting[schema.Bluetooth]{
					Name:  "speaker",
					Group: Group,
					Icon:  "mdi:speaker-bluetooth",
				},
				func(c *schema.Bluetooth) *bool { return &c.Speaker },
			),
		},
		setting.Messages{Text: say.T, Missing: say.Missing},
	)
}

func (s *Sink) Name() string { return "bluetooth speaker" }

func (s *Sink) Entities() []esphome.Entity { return s.knobs.Entities() }

func (s *Sink) Controls() *setting.Controls[schema.Bluetooth] { return s.knobs }

func (s *Sink) Enabled() bool { return s.options.Settings.Read().Speaker }

func (s *Sink) SetEnabled(on bool) {
	if err := s.knobs.Change("speaker", setting.OnOff(on)); err != nil {
		slog.Error("saving the bluetooth speaker setting failed", "err", err)
		s.knobs.Publish()
	}
}

func (s *Sink) store(row setting.Setting[schema.Bluetooth], v string) error {
	if err := setting.Store(s.options.Settings, row, v); err != nil {
		return err
	}
	select {
	case s.wake <- struct{}{}:
	default:
	}
	return nil
}

const retry = 10 * time.Second

func (s *Sink) Run(ctx context.Context) error {
	quiet := s.options.Volume.Watch(s.volumed)
	defer quiet()
	ctx, stop := context.WithCancel(ctx)
	defer stop()
	done := make(chan struct{})
	defer func() { stop(); <-done }()

	go func() {
		defer close(done)
		for {
			select {
			case <-ctx.Done():
				return
			case <-s.wake:
				s.interrupt()
			}
		}
	}()

	for ctx.Err() == nil {
		if err := s.attempt(ctx); err != nil {
			slog.Warn("not answering as a speaker", "err", err)
		}

		select {
		case <-ctx.Done():
		case <-time.After(retry):
		}
	}
	return nil
}

func (s *Sink) attempt(parent context.Context) error {
	ctx, stop := context.WithCancel(parent)
	defer stop()

	s.mu.Lock()
	s.stop = stop
	s.mu.Unlock()

	if !s.Enabled() {
		return nil
	}

	radio := s.options.Radio()
	if !radio.Up() {
		return fmt.Errorf("a2dp: the radio is not up")
	}

	name := s.options.Name()
	links := &connections{
		host: s, name: name, peers: make(map[uint16]*connectedPeer), volume: make(chan struct{}, 1),
	}
	go links.watchVolume(ctx)
	s.mu.Lock()
	s.openPairings()
	s.links = links
	s.mu.Unlock()
	defer func() {
		links.close()
		s.mu.Lock()
		s.links = nil
		s.mu.Unlock()
	}()

	slog.Info("bluetooth speaker", "name", name, "class", "loudspeaker")
	return radio.Speaker(ctx, name, links)
}

func (s *Sink) Sending(send func(handle uint16, pdu []byte) error) {
	s.mu.Lock()
	s.send = send
	s.mu.Unlock()
}

// AVRCP absolute volume: the phone is the controller and this device the target.
func (s *Sink) volumed(level int) {
	if links := s.connections(); links != nil {
		select {
		case links.volume <- struct{}{}:
		default:
		}
		return
	}
	s.tell(fmt.Sprintf("volume %d", level), func(sink *bt.Sink) ([]l2cap.Frame, error) {
		return sink.Volumed(level)
	})
}

func (s *Sink) announce(t avrcp.Track) {
	s.mu.Lock()
	was := s.track
	s.track = t
	s.mu.Unlock()

	s.cover(t.Art)

	if t.Title == was.Title && t.Artist == was.Artist && t.Album == was.Album {
		return
	}

	slog.Info("bluetooth now playing", "title", t.Title, "artist", t.Artist, "album", t.Album,
		"duration", t.Duration, "art", t.Art)

	s.list()
}

func (s *Sink) interrupt() {
	s.mu.Lock()
	stop := s.stop
	s.mu.Unlock()

	if stop != nil {
		stop()
	}
}

func (s *Sink) start(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.openPairings()
	s.startPeer(name)
}

func (s *Sink) openPairings() {
	keys, err := pair.Open(s.options.Bonds)
	if err != nil {
		slog.Warn("pairings will not be remembered", "path", s.options.Bonds, "err", err)
		s.pairs = pair.New(nil)
	} else {
		s.pairs = pair.New(keys)
	}
}

func (s *Sink) startPeer(name string) {
	s.sink = bt.NewSink(name, bt.MTU)
	if s.options.Volume != nil {
		s.sink.Remote().Volumed(s.options.Volume.Level())
	}
	s.filter = sbc.Filter{}
	s.queue, s.handles = nil, nil
	s.art.clear()

	s.sink.Remote().Volume = func(percent int) {
		if s.audible() {
			s.options.Volume.Set(percent)
		}
	}

	s.sink.Remote().Changed = func(state avrcp.State) {
		s.announce(state.Track)
		s.options.Media.Changed()
	}

	s.sink.Controls = func() { s.ask(); s.open(); s.look() }

	s.sink.Opened = func(psm uint16) {
		s.art.mu.Lock()
		images := s.art.psm
		s.art.mu.Unlock()

		switch {
		case psm == bt.BrowsePSM:
			s.list()
		case psm == l2cap.PSMSDP:
			s.look()
		case images != 0 && psm == images:
			s.Hello()
		}
	}

	s.sink.Image = s.imaged

	s.sink.Found = func(records []sdp.Record, more bool) {
		for _, r := range records {
			features, _ := r.Attribute(sdp.AttrFeatures)
			bits, _ := features.Uint()

			art, served := sdp.MorePSM(r, sdp.UUIDOBEX)
			browse, browsable := sdp.MorePSM(r, sdp.UUIDAVCTP)

			slog.Debug("bluetooth the far end serves",
				"classes", fmt.Sprintf("%#x", sdp.Classes(r)),
				"features", fmt.Sprintf("%#04x", bits),
				"cover_art_psm", fmt.Sprintf("%#04x", art), "cover_art", served,
				"browse_psm", fmt.Sprintf("%#04x", browse), "browsable", browsable)
		}
		if more {
			slog.Debug("bluetooth the far end held back more records than fitted")
		}
		s.looked(records)
	}

	s.sink.Listed = s.listed

	s.sink.Started = func(e *avdtp.Endpoint) {
		s.mu.Lock()
		s.filter = sbc.Filter{}
		s.streaming = true
		s.said = time.Now()
		s.frames, s.bad, s.samples, s.why = 0, 0, 0, nil
		s.mu.Unlock()

		if codec, ok := e.Codec(); ok {
			rate, _ := codec.Rate()
			resample := speaker.NewSkewable(rate, Rate, 2)
			s.mu.Lock()
			s.resample = resample
			s.drift.Reset()
			s.mu.Unlock()

			up, down := resample.Ratio()
			slog.Info("bluetooth audio started", "handle", s.handle,
				"rate", rate, "mode", codec.Channels, "bitpool", codec.MaxBitpool,
				"resampled", fmt.Sprintf("%d:%d", up, down))
		}

		if remote := s.remote(); remote != nil {
			remote.Cleared()
		}
		s.holds(nil)
		s.ask()

		if s.owner != nil {
			s.owner.started(s)
		} else {
			s.beginPlayback()
		}
	}

	s.sink.Stopped = func(*avdtp.Endpoint) {
		slog.Info("bluetooth audio stopped", "handle", s.handle)
		s.mu.Lock()
		wasStreaming := s.streaming
		s.streaming = false
		s.resample = nil
		s.mu.Unlock()

		if wasStreaming && s.audible() {
			s.options.Arbitration.Gave(s)
			s.options.Queue.Drain()
		}
	}

	s.sink.Frame = s.play
	if s.owner != nil {
		s.sink.Frame = func(frame []byte) { s.owner.frame(s, frame) }
	}
}

func (s *Sink) audible() bool {
	return s.owner == nil || s.owner.active.Load() == s
}

func (s *Sink) beginPlayback() {
	s.options.Queue.Drain()
	s.options.Arbitration.Took(s)
	s.attach()
}

func (s *Sink) play(frame []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.frames == 0 {
		s.said = time.Now()
	}

	s.frames++
	s.dumping.Frame(frame)

	got, err := sbc.Unpack(frame)
	if err != nil {
		s.bad++
		s.why = err
		s.tally()
		return
	}

	audio, err := s.filter.Synthesize(got)
	if err != nil {
		s.bad++
		s.why = err
		s.tally()
		return
	}

	if s.down {
		return
	}

	out := interleave(audio)
	decoded := out

	if s.resample != nil {
		out = s.resample.Run(out)
	}
	s.dumping.Write(decoded, out)
	if s.gain != 0 {
		analysis.Scale(out, s.gain)
	}

	s.samples += len(out) / 2
	s.tally()

	s.options.Queue.Play(out)

	if s.resample != nil {
		s.resample.Skew(s.drift.Observe(s.options.Queue.Queued(), time.Now()))
	}
}

const counted = 5 * time.Second

const adrift = Rate / 100

func (s *Sink) tally() {
	if time.Since(s.said) < counted {
		return
	}
	since := time.Since(s.said)

	rate := int(float64(s.samples) / since.Seconds())

	if s.bad > 0 || rate < Rate-adrift || rate > Rate+adrift {
		slog.Warn("bluetooth audio is not keeping up", "frames", s.frames, "bad", s.bad,
			"samples_per_second", rate, "card", Rate, "last", s.why)
	}

	s.said = time.Now()
	s.frames, s.bad, s.samples, s.why = 0, 0, 0, nil
}

func interleave(audio sbc.Samples) []int16 {
	if len(audio) == 0 {
		return nil
	}

	if len(audio) == 1 {
		out := make([]int16, 0, len(audio[0])*2)
		for _, v := range audio[0] {
			out = append(out, v, v)
		}
		return out
	}

	out := make([]int16, 0, len(audio[0])*len(audio))
	for i := range audio[0] {
		for ch := range audio {
			out = append(out, audio[ch][i])
		}
	}
	return out
}

func (s *Sink) Event(e pair.Event) ([]pair.Command, error) {
	said, err := s.pairEvent(e)
	if err != nil {
		return nil, err
	}
	return append(said, s.named(e)...), nil
}

func (s *Sink) pairEvent(e pair.Event) ([]pair.Command, error) {
	s.mu.Lock()
	pairs := s.pairs
	s.mu.Unlock()

	if pairs == nil {
		return nil, nil
	}

	said, err := pairs.Handle(e)
	if err != nil {
		slog.Warn("bluetooth pairing", "code", fmt.Sprintf("%#02x", e.Code), "err", err)
		return nil, err
	}
	return said, nil
}

func (s *Sink) named(e pair.Event) []pair.Command {
	switch e.Code {
	case pair.EventConnectionComplete:
		addr, ok := pair.Connected(e.Params)
		if !ok {
			return nil
		}
		return []pair.Command{pair.RemoteName(addr)}

	case pair.EventRemoteName:
		_, name, err := pair.ParseRemoteName(e.Params)
		if err != nil {
			slog.Warn("bluetooth name", "err", err)
			return nil
		}

		s.mu.Lock()
		s.label = name
		s.mu.Unlock()

		slog.Info("bluetooth connected", "name", name)
		s.options.Media.Changed()
	}
	return nil
}

func (s *Sink) Label() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.label
}

func brief(b []byte) string {
	const most = 40

	if len(b) <= most {
		return fmt.Sprintf("% x", b)
	}
	return fmt.Sprintf("% x ... (%d bytes)", b[:most], len(b))
}

func (s *Sink) Data(handle uint16, pdu []byte) ([][]byte, error) {
	s.mu.Lock()
	sink := s.sink

	s.handle = handle
	s.mu.Unlock()

	if sink == nil {
		return nil, nil
	}

	f, err := l2cap.ParseFrame(pdu)
	if err != nil {
		return nil, err
	}

	said := tracing() && !sink.Media(f.CID)
	if said {
		slog.Info("bluetooth said", "cid", fmt.Sprintf("%#04x", f.CID), "pdu", brief(f.Payload))
	}

	s.mu.Lock()
	s.answering = true
	s.mu.Unlock()

	out, err := sink.Receive(f)

	s.mu.Lock()
	held := s.held
	s.answering, s.held = false, nil
	s.mu.Unlock()

	if err != nil {
		slog.Warn("bluetooth data", "cid", fmt.Sprintf("%#04x", f.CID),
			"asked", brief(f.Payload), "err", err)
		return nil, err
	}

	if said {
		for _, r := range out {
			slog.Info(
				"bluetooth answered",
				"cid",
				fmt.Sprintf("%#04x", r.CID),
				"pdu",
				brief(r.Payload),
			)
		}
	}

	replies := make([][]byte, 0, len(out)+len(held))
	for _, r := range out {
		replies = append(replies, r.Marshal())
	}
	return append(replies, held...), nil
}

func (s *Sink) Gone(handle uint16) {
	s.mu.Lock()
	ours, held := s.handle != 0 && s.handle == handle, s.handle
	s.mu.Unlock()

	slog.Info("bluetooth link ended", "handle", handle, "held", held, "ours", ours)

	if !ours {
		return
	}

	s.detach()

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.sink != nil {
		s.sink.Forget()
	}
	s.filter = sbc.Filter{}
	s.handle = 0
	s.queue, s.handles, s.label = nil, nil, ""
}

const Rate = 48000
const Channels = 2

type Queue interface {
	Play([]int16)
	Drain()
	Queued() int
	Adjust(func([]int16))
}
type Media interface {
	Began(nowplaying.Source)
	Ended(nowplaying.Source)
	Release(nowplaying.Source)
	Changed()
}
type Dependencies struct {
	Queue                   Queue
	Radio                   func() *hci.Radio
	Bonds, CaptureDirectory string
	Name                    func() string
	Settings                storage.Section[schema.Bluetooth]
	Volume                  volume.Channel
	Media                   Media
	Arbitration             interface {
		Took(pcm.Producer)
		Gave(pcm.Producer)
	}
	Ducking storage.Section[schema.Media]
}

func (s *Sink) Dump(within time.Duration) (string, error) { return s.dumping.Start(within) }
