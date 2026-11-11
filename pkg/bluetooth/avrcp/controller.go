package avrcp

import (
	"fmt"
	"log/slog"
	"time"
)

type Controller struct {
	Changed func(State)

	Volume func(percent int)

	state State

	pending map[byte]byte

	registered map[byte]bool

	registering map[byte]byte

	supported map[byte]bool
	asked     bool

	arting map[byte]uint32
	age    uint32

	player uint16

	watching byte
	watched  bool

	label byte
}

type State struct {
	Track    Track
	Progress Progress

	UID uint64

	Player uint16

	Volume int
}

func New() *Controller {
	return &Controller{
		pending:     map[byte]byte{},
		arting:      map[byte]uint32{},
		registering: map[byte]byte{},
		registered:  map[byte]bool{},
		supported:   map[byte]bool{},
		state:       State{Volume: -1},
	}
}

func (c *Controller) State() State { return c.state }

func (c *Controller) nextLabel() (byte, error) {
	for range 16 {
		c.label = (c.label + 1) & 0x0f
		if _, busy := c.pending[c.label]; !busy {
			return c.label, nil
		}
	}

	held := make(map[byte]string, len(c.pending))
	for label, pdu := range c.pending {
		if event, ok := c.registering[label]; ok {
			held[label] = fmt.Sprintf("notify %#02x", event)
			continue
		}
		held[label] = fmt.Sprintf("pdu %#02x", pdu)
	}
	return 0, fmt.Errorf("avrcp: all sixteen transaction labels are outstanding: %v", held)
}

func (c *Controller) command(frame AVC, expecting byte) (Transport, error) {
	label, err := c.nextLabel()
	if err != nil {
		return Transport{}, err
	}

	c.pending[label] = expecting
	return Transport{Label: label, Type: MessageCommand, Payload: frame.Marshal()}, nil
}

func (c *Controller) Press(op byte) ([]Transport, error) {
	down, err := c.command(PassThrough(op, Pressed), OpPassThrough)
	if err != nil {
		return nil, err
	}

	up, err := c.command(PassThrough(op, Released), OpPassThrough)
	if err != nil {
		return nil, err
	}

	return []Transport{down, up}, nil
}

func (c *Controller) Start() ([]Transport, error) {
	capabilities, err := c.command(
		GetCapabilities(CapabilityEvents).Frame(Status),
		PDUGetCapabilities,
	)
	if err != nil {
		return nil, err
	}

	attributes, err := c.command(
		GetElementAttributes(Basics...).Frame(Status),
		PDUGetElementAttributes,
	)
	if err != nil {
		return nil, err
	}

	cover, err := c.cover()
	if err != nil {
		return nil, err
	}

	status, err := c.command(GetPlayStatus().Frame(Status), PDUGetPlayStatus)
	if err != nil {
		return nil, err
	}

	return []Transport{capabilities, attributes, cover, status}, nil
}

func (c *Controller) Refresh() ([]Transport, error) {
	attributes, err := c.command(
		GetElementAttributes(Basics...).Frame(Status),
		PDUGetElementAttributes,
	)
	if err != nil {
		return nil, err
	}

	cover, err := c.cover()
	if err != nil {
		return nil, err
	}

	status, err := c.command(GetPlayStatus().Frame(Status), PDUGetPlayStatus)
	if err != nil {
		return nil, err
	}

	return []Transport{attributes, cover, status}, nil
}

func (c *Controller) Attributes(ids ...uint32) ([]Transport, error) {
	t, err := c.command(GetElementAttributes(ids...).Frame(Status), PDUGetElementAttributes)
	if err != nil {
		return nil, err
	}
	return []Transport{t}, nil
}

func (c *Controller) Receive(t Transport) ([]Transport, error) {
	if t.Invalid {
		delete(c.pending, t.Label)
		return nil, fmt.Errorf("avrcp: the far end does not speak this profile")
	}

	frame, err := ParseAVC(t.Payload)
	if err != nil {
		return nil, err
	}

	if t.Type == MessageCommand {
		return c.commanded(t, frame)
	}
	return c.answered(t, frame)
}

const capabilityCompanies = 0x02

func (c *Controller) commanded(t Transport, frame AVC) ([]Transport, error) {
	answer := func(payload AVC) []Transport {
		return []Transport{{
			Label:   t.Label,
			Type:    MessageResponse,
			Payload: payload.Marshal(),
		}}
	}

	if frame.Opcode != OpVendorDependent {
		frame.Code = NotImplemented
		return answer(frame), nil
	}

	p, err := ParsePDU(frame.Operands)
	if err != nil {
		return nil, err
	}

	switch p.ID {
	case PDUGetCapabilities:
		if len(p.Params) != 1 {
			return nil, ErrShort
		}
		switch p.Params[0] {
		case CapabilityEvents:
			return answer(
				PDU{
					ID:     PDUGetCapabilities,
					Params: []byte{CapabilityEvents, 1, EventVolume},
				}.Frame(
					Stable,
				),
			), nil
		case capabilityCompanies:
			return answer(
				PDU{
					ID: PDUGetCapabilities,
					Params: []byte{
						capabilityCompanies,
						1,
						byte(SIG >> 16),
						byte(SIG >> 8 & 0xff),
						byte(SIG & 0xff),
					},
				}.Frame(
					Stable,
				),
			), nil
		default:
			return answer(PDU{ID: PDUGetCapabilities, Params: []byte{0x01}}.Frame(Rejected)), nil
		}
	case PDUSetAbsoluteVolume:
		if len(p.Params) < 1 {
			return nil, ErrShort
		}

		v := p.Params[0] & 0x7f
		c.state.Volume = Percent(v)
		if c.Volume != nil {
			c.Volume(c.state.Volume)
		}
		c.publish()

		return answer(SetAbsoluteVolume(v).Frame(Accepted)), nil

	case PDURegisterNotification:
		if len(p.Params) < 1 {
			return nil, ErrShort
		}

		if p.Params[0] == EventVolume {
			c.watching, c.watched = t.Label, true

			v := byte(0)
			if c.state.Volume >= 0 {
				v = Volume(c.state.Volume)
			}
			body := PDU{ID: PDURegisterNotification, Params: []byte{EventVolume, v}}
			return answer(body.Frame(Interim)), nil
		}
	}

	frame.Code = NotImplemented
	return answer(frame), nil
}

func (c *Controller) answered(t Transport, frame AVC) ([]Transport, error) {
	asked, waiting := c.pending[t.Label]
	if !waiting {
		return nil, nil
	}

	// INTERIM is followed by the final response on the same label.
	if frame.Code != Interim {
		delete(c.pending, t.Label)
	}

	if frame.Code == Rejected || frame.Code == NotImplemented {
		return c.refused(t.Label, asked, frame)
	}

	if frame.Opcode == OpPassThrough {
		return nil, nil
	}
	if frame.Opcode != OpVendorDependent {
		return nil, nil
	}

	p, err := ParsePDU(frame.Operands)
	if err != nil {
		return nil, err
	}

	if p.Packet == PacketStart || p.Packet == PacketContinue {
		return c.command1(AbortContinuing(p.ID).Frame(Control), PDUAbortContinuing)
	}

	switch p.ID {
	case PDUGetCapabilities:
		return c.capabilities(p)
	case PDUGetElementAttributes:
		return nil, c.attributes(t.Label, p)
	case PDUGetPlayStatus:
		return nil, c.playStatus(p)
	case PDURegisterNotification:
		return c.notified(t.Label, p, frame.Code == Interim)
	}
	return nil, nil
}

// A rejection carries an error code where a completion carries the event.
func (c *Controller) refused(label, asked byte, frame AVC) ([]Transport, error) {
	if asked != PDURegisterNotification {
		return nil, fmt.Errorf("avrcp: %#02x was refused", asked)
	}

	event, known := c.registering[label]
	delete(c.registering, label)
	if !known {
		return nil, nil
	}
	c.registered[event] = false

	status := byte(RefusedBadCommand)
	if p, err := ParsePDU(frame.Operands); err == nil && len(p.Params) > 0 {
		status = p.Params[0]
	}

	if status == RefusedPlayerMoved {
		return c.register(event)
	}

	c.supported[event] = false
	slog.Debug("the far end will not report an event",
		"event", fmt.Sprintf("%#02x", event), "status", fmt.Sprintf("%#02x", status))
	return nil, nil
}

func (c *Controller) capabilities(p PDU) ([]Transport, error) {
	events, err := ParseCapabilities(p.Params)
	if err != nil {
		return nil, err
	}

	c.asked = true
	for _, e := range events {
		c.supported[e] = true
	}

	slog.Debug("the far end reports these events", "events", fmt.Sprintf("% #02x", events))

	return c.register(EventPlaybackStatus, EventTrackChanged, EventPosition,
		EventNowPlaying, EventAddressPlayer)
}

func (c *Controller) register(events ...byte) ([]Transport, error) {
	var out []Transport

	for _, e := range events {
		if c.registered[e] {
			continue
		}
		if c.asked && !c.supported[e] {
			continue
		}

		interval := uint32(0)
		if e == EventPosition {
			interval = 1
		}

		t, err := c.command(
			RegisterNotification(e, interval).Frame(Notify),
			PDURegisterNotification,
		)
		if err != nil {
			return out, err
		}

		c.registered[e] = true
		c.registering[t.Label] = e
		out = append(out, t)
	}
	return out, nil
}

func (c *Controller) cover() (Transport, error) {
	t, err := c.command(GetElementAttributes(AttrCoverArt).Frame(Status), PDUGetElementAttributes)
	if err != nil {
		return t, err
	}

	c.arting[t.Label] = c.age
	return t, nil
}

func (c *Controller) attributes(label byte, p PDU) error {
	track, err := ParseElementAttributes(p.Params)
	if err != nil {
		return err
	}

	if track.Empty() {
		age, asked := c.arting[label]
		delete(c.arting, label)

		if asked && age == c.age && track.Art != "" && track.Art != c.state.Track.Art {
			c.state.Track.Art = track.Art
			c.publish()
		}
		return nil
	}
	delete(c.arting, label)

	if track.Art == "" && track.Title == c.state.Track.Title &&
		track.Artist == c.state.Track.Artist && track.Album == c.state.Track.Album {
		track.Art = c.state.Track.Art
	}

	if track == c.state.Track {
		return nil
	}

	if track.Title != c.state.Track.Title || track.Artist != c.state.Track.Artist {
		c.age++
	}

	c.state.Track = track
	c.publish()
	return nil
}

func (c *Controller) playStatus(p PDU) error {
	progress, err := ParsePlayStatus(p.Params)
	if err != nil {
		return err
	}

	c.state.Progress = progress
	c.publish()
	return nil
}

func (c *Controller) notified(label byte, p PDU, interim bool) ([]Transport, error) {
	n, err := ParseNotification(p.Params, interim)
	if err != nil {
		return nil, err
	}

	if !interim {
		delete(c.registering, label)
	}

	was := c.state.Progress.Status
	moved := false

	switch n.Event {
	case EventPlaybackStatus:
		c.state.Progress.Status = n.Status
	case EventPosition:
		c.state.Progress.Position = n.Position
	case EventVolume:
		c.state.Volume = Percent(n.Volume)
	case EventTrackChanged:
		c.state.UID = n.Track

	case EventAddressPlayer:
		moved = n.Player != c.player
		c.player = n.Player
		c.state.Player = n.Player

		if moved {
			slog.Debug("the far end addressed a different player", "player", n.Player)
		}

	case EventNowPlaying:
		moved = !interim
	}
	c.publish()

	started := n.Event == EventPlaybackStatus && n.Status == StatusPlaying && was != StatusPlaying

	var out []Transport
	if !interim {
		c.registered[n.Event] = false

		out, err = c.register(n.Event)
		if err != nil {
			return out, err
		}
	}

	if n.Event == EventTrackChanged || started || moved {
		more, err := c.Refresh()
		if err != nil {
			return out, err
		}
		out = append(out, more...)
	}
	return out, nil
}

func (c *Controller) command1(frame AVC, expecting byte) ([]Transport, error) {
	t, err := c.command(frame, expecting)
	if err != nil {
		return nil, err
	}
	return []Transport{t}, nil
}

func (c *Controller) Volumed(percent int) []Transport {
	if percent == c.state.Volume {
		return nil
	}
	c.state.Volume = percent
	c.publish()

	if !c.watched {
		return nil
	}
	c.watched = false

	body := PDU{ID: PDURegisterNotification, Params: []byte{EventVolume, Volume(percent)}}
	return []Transport{{
		Label:   c.watching,
		Type:    MessageResponse,
		Payload: body.Frame(Changed).Marshal(),
	}}
}

func (c *Controller) SetVolume(percent int) (Transport, error) {
	c.state.Volume = percent
	return c.command(SetAbsoluteVolume(Volume(percent)).Frame(Control), PDUSetAbsoluteVolume)
}

func (c *Controller) Forget() {
	clear(c.pending)
	clear(c.registering)
	clear(c.registered)
	clear(c.supported)
	c.player = 0
	c.asked = false
	c.state = State{Volume: -1}
}

func (c *Controller) Cleared() {
	c.state.Track = Track{}
	c.state.Progress = Progress{}
	c.state.UID = 0
	c.publish()
}

func (c *Controller) publish() {
	if c.Changed != nil {
		c.Changed(c.state)
	}
}

func (s State) Elapsed(since time.Duration) time.Duration {
	if s.Progress.Status != StatusPlaying {
		return s.Progress.Position
	}

	at := s.Progress.Position + since
	if s.Progress.Length > 0 && at > s.Progress.Length {
		return s.Progress.Length
	}
	return at
}
