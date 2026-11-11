package bt

import (
	"fmt"
	"slices"

	"github.com/ygelfand/libcountertop/pkg/bluetooth/avdtp"
	"github.com/ygelfand/libcountertop/pkg/bluetooth/avrcp"
	"github.com/ygelfand/libcountertop/pkg/bluetooth/l2cap"
	"github.com/ygelfand/libcountertop/pkg/bluetooth/sbc"
	"github.com/ygelfand/libcountertop/pkg/bluetooth/sdp"
)

// AVDTP signalling and media are two channels on PSM 0x0019, told apart by order.
type Sink struct {
	l2      *l2cap.Manager
	stream  *avdtp.Server
	remote  *avrcp.Controller
	records []sdp.Record

	// Zero is not a valid CID.
	signalling uint16
	transport  uint16

	control uint16

	browse uint16

	finding uint16

	transaction uint16

	art    uint16
	artPSM uint16

	media   avdtp.Reassembler
	command avrcp.Reassembler

	Frame func([]byte)

	Started func(*avdtp.Endpoint)
	Stopped func(*avdtp.Endpoint)

	Controls func()

	Opened func(psm uint16)

	Listed func(avrcp.Browse)

	Found func(records []sdp.Record, more bool)

	Image func(packet []byte)
}

// Any of 1 to 0x3e.
const SEID = 1

const MTU = 2048

func NewSink(name string, mtu uint16) *Sink {
	s := &Sink{
		l2:      l2cap.NewManager(mtu),
		stream:  avdtp.NewServer(SEID),
		remote:  avrcp.New(),
		records: Records(name),
	}

	s.l2.Serve(l2cap.PSMSDP)
	s.l2.Serve(SinkPSM)
	s.l2.Serve(ControlPSM)
	s.l2.Serve(BrowsePSM)

	s.stream.Started = func(e *avdtp.Endpoint) {
		if s.Started != nil {
			s.Started(e)
		}
	}
	s.stream.Stopped = func(e *avdtp.Endpoint) {
		s.media = avdtp.Reassembler{}
		if s.Stopped != nil {
			s.Stopped(e)
		}
	}

	return s
}

func (s *Sink) Endpoint() *avdtp.Endpoint { return s.stream.Endpoint(SEID) }

func (s *Sink) Receive(f l2cap.Frame) ([]l2cap.Frame, error) {
	if f.CID == l2cap.CIDSignalling {
		commands, err := s.l2.Handle(f)
		s.adopt()

		if len(commands) == 0 {
			return nil, err
		}
		return []l2cap.Frame{l2cap.Signal(commands...)}, err
	}

	ch := s.l2.Channel(f.CID)
	if ch == nil {
		return nil, fmt.Errorf("bt: data on channel %#x, which is not open", f.CID)
	}

	switch {
	case ch.PSM == l2cap.PSMSDP:
		return s.discovery(ch, f.Payload)
	case ch.PSM == ControlPSM:
		return s.avctp(ch, f.Payload)
	case ch.PSM == BrowsePSM:
		return s.browsing(ch, f.Payload)
	case f.CID == s.signalling:
		return s.signal(ch, f.Payload)
	case f.CID == s.transport:
		return nil, s.audio(f.Payload)

	case f.CID == s.art:
		if s.Image != nil {
			s.Image(f.Payload)
		}
		return nil, nil
	}

	return nil, fmt.Errorf("bt: channel %#x carries nothing this understands", f.CID)
}

func (s *Sink) adopt() {
	var control, browse, finding, art uint16
	var stream []*l2cap.Channel

	for _, c := range s.l2.Open() {
		switch {
		case c.PSM == ControlPSM:
			control = c.Local
		case c.PSM == BrowsePSM:
			browse = c.Local

		case c.PSM == l2cap.PSMSDP && c.Ours:
			finding = c.Local

		case s.artPSM != 0 && c.PSM == s.artPSM:
			art = c.Local
		case c.PSM == SinkPSM:
			stream = append(stream, c)
		}
	}

	slices.SortFunc(stream, func(a, b *l2cap.Channel) int { return int(a.Local) - int(b.Local) })

	s.signalling, s.transport = 0, 0
	if len(stream) > 0 {
		s.signalling = stream[0].Local
	}
	if len(stream) > 1 {
		s.transport = stream[1].Local
	}

	if was := s.control; was != control {
		s.control = control
		s.stale()
		browse, finding, art = 0, 0, 0

		if control != 0 && s.Controls != nil {
			s.Controls()
		}
	}

	s.came(&s.browse, browse, BrowsePSM)
	s.came(&s.finding, finding, l2cap.PSMSDP)
	s.came(&s.art, art, s.artPSM)
}

func (s *Sink) stale() {
	for _, at := range []*uint16{&s.browse, &s.finding, &s.art} {
		if *at != 0 {
			s.l2.Drop(*at)
			*at = 0
		}
	}
	s.artPSM = 0
}

func took(at *uint16, now uint16) bool {
	was := *at
	*at = now
	return now != 0 && now != was
}

func (s *Sink) came(at *uint16, now, psm uint16) {
	if took(at, now) && s.Opened != nil {
		s.Opened(psm)
	}
}

func (s *Sink) Ask() ([]l2cap.Frame, error) {
	messages, err := s.remote.Start()
	if err != nil {
		return nil, err
	}
	return s.Control(messages)
}

func (s *Sink) Media(cid uint16) bool { return cid != 0 && cid == s.transport }

func (s *Sink) Remote() *avrcp.Controller { return s.remote }

func (s *Sink) Controlling() bool { return s.control != 0 }

func (s *Sink) avctp(ch *l2cap.Channel, payload []byte) ([]l2cap.Frame, error) {
	whole, err := s.command.Push(payload)
	if err != nil {
		return nil, err
	}
	if whole == nil {
		return nil, nil
	}

	m, err := avrcp.ParseTransport(whole)
	if err != nil {
		return nil, err
	}

	answers, err := s.remote.Receive(m)
	if err != nil {
		return nil, err
	}

	var out []l2cap.Frame
	for _, a := range answers {
		out = append(out, s.send(ch, a.Marshal())...)
	}
	return out, nil
}

// The browsing channel does not fragment at AVCTP.
func (s *Sink) browsing(ch *l2cap.Channel, payload []byte) ([]l2cap.Frame, error) {
	m, err := avrcp.ParseTransport(payload)
	if err != nil {
		return nil, err
	}

	if m.Type == avrcp.MessageResponse {
		if s.Listed == nil {
			return nil, nil
		}

		b, err := avrcp.ParseBrowse(m.Payload)
		if err != nil {
			return nil, err
		}
		s.Listed(b)
		return nil, nil
	}

	answers, bad := avrcp.Browsed(m)

	var out []l2cap.Frame
	for _, a := range answers {
		out = append(out, s.send(ch, a.Marshal())...)
	}
	return out, bad
}

func (s *Sink) Control(messages []avrcp.Transport) ([]l2cap.Frame, error) {
	if s.control == 0 {
		return nil, fmt.Errorf("bt: the remote control channel is not open")
	}

	ch := s.l2.Channel(s.control)
	if ch == nil {
		return nil, fmt.Errorf("bt: the remote control channel went away")
	}

	var out []l2cap.Frame
	for _, m := range messages {
		out = append(out, s.send(ch, m.Marshal())...)
	}
	return out, nil
}

func (s *Sink) Volumed(percent int) ([]l2cap.Frame, error) {
	messages := s.remote.Volumed(percent)
	if len(messages) == 0 {
		return nil, nil
	}
	return s.Control(messages)
}

func (s *Sink) Browsing() bool {
	if s.browse == 0 {
		return false
	}
	ch := s.l2.Channel(s.browse)
	return ch != nil && ch.State == l2cap.Open
}

func (s *Sink) OpenBrowsing() ([]l2cap.Frame, error) {
	if s.browse != 0 {
		return nil, fmt.Errorf("bt: the browsing channel is already open")
	}

	c, err := s.l2.Connect(BrowsePSM)
	if err != nil {
		return nil, err
	}
	return []l2cap.Frame{l2cap.Signal(c)}, nil
}

func (s *Sink) Browse(messages []avrcp.Transport) ([]l2cap.Frame, error) {
	if !s.Browsing() {
		return nil, fmt.Errorf("bt: the browsing channel is not open")
	}

	ch := s.l2.Channel(s.browse)

	var out []l2cap.Frame
	for _, m := range messages {
		out = append(out, s.send(ch, m.Marshal())...)
	}
	return out, nil
}

func (s *Sink) OpenArt(psm uint16) ([]l2cap.Frame, error) {
	if psm == 0 {
		return nil, fmt.Errorf("bt: no channel was published for images")
	}
	if s.art != 0 {
		return nil, fmt.Errorf("bt: the image channel is already open")
	}

	c, err := s.l2.Connect(psm)
	if err != nil {
		return nil, err
	}

	s.artPSM = psm
	return []l2cap.Frame{l2cap.Signal(c)}, nil
}

func (s *Sink) Imaging() bool {
	if s.art == 0 {
		return false
	}
	ch := s.l2.Channel(s.art)
	return ch != nil && ch.State == l2cap.Open
}

func (s *Sink) Art(packet []byte) ([]l2cap.Frame, error) {
	if s.art == 0 {
		return nil, fmt.Errorf("bt: the image channel is not open")
	}

	ch := s.l2.Channel(s.art)
	if ch == nil || ch.State != l2cap.Open {
		return nil, fmt.Errorf("bt: the image channel is not open")
	}
	return s.send(ch, packet), nil
}

func (s *Sink) Attributes(ids ...uint32) ([]l2cap.Frame, error) {
	messages, err := s.remote.Attributes(ids...)
	if err != nil {
		return nil, err
	}
	return s.Control(messages)
}

func (s *Sink) Press(op byte) ([]l2cap.Frame, error) {
	messages, err := s.remote.Press(op)
	if err != nil {
		return nil, err
	}
	return s.Control(messages)
}

func (s *Sink) discovery(ch *l2cap.Channel, payload []byte) ([]l2cap.Frame, error) {
	p, err := sdp.ParsePDU(payload)
	if err != nil {
		return nil, err
	}

	switch p.ID {
	case sdp.PDUSearchAttributeResponse:
		return nil, s.found(p)

	case sdp.PDUError:
		return nil, fmt.Errorf("sdp: the far end refused a search: % x", p.Params)
	}

	mtu := int(ch.MTU)
	if mtu <= 0 {
		mtu = l2cap.MTUDefault
	}
	answer, err := s.answer(p, mtu)
	if err != nil {
		return nil, err
	}
	return s.send(ch, answer.Marshal()), nil
}

func (s *Sink) found(p sdp.PDU) error {
	records, more, err := sdp.ParseSearchAttributeResponse(p.Params)
	if err != nil {
		return err
	}
	if s.Found != nil {
		s.Found(records, len(more) > 0)
	}
	return nil
}

func (s *Sink) OpenDiscovery() ([]l2cap.Frame, error) {
	if s.finding != 0 {
		return nil, fmt.Errorf("bt: already looking the far end up")
	}

	c, err := s.l2.Connect(l2cap.PSMSDP)
	if err != nil {
		return nil, err
	}
	return []l2cap.Frame{l2cap.Signal(c)}, nil
}

func (s *Sink) Finding() bool {
	if s.finding == 0 {
		return false
	}
	ch := s.l2.Channel(s.finding)
	return ch != nil && ch.State == l2cap.Open
}

func (s *Sink) Discover(search sdp.Search) ([]l2cap.Frame, error) {
	if s.finding == 0 {
		return nil, fmt.Errorf("bt: no channel to look the far end up on")
	}

	ch := s.l2.Channel(s.finding)
	if ch == nil || ch.State != l2cap.Open {
		return nil, fmt.Errorf("bt: the lookup channel is not open")
	}

	s.transaction++
	p, err := search.Request(s.transaction)
	if err != nil {
		return nil, err
	}
	return s.send(ch, p.Marshal()), nil
}

func (s *Sink) answer(p sdp.PDU, mtu int) (sdp.PDU, error) {
	if p.ID == sdp.PDUServiceSearchRequest || p.ID == sdp.PDUAttributeRequest {
		return s.answerSeparate(p, mtu), nil
	}
	if p.ID != sdp.PDUSearchAttributeRequest {
		return sdp.Error(p.Transaction, sdp.ErrorBadSyntax), nil
	}

	search, err := sdp.ParseSearchAttribute(p.Params)
	if err != nil {
		return sdp.Error(p.Transaction, sdp.ErrorBadSyntax), nil
	}

	var records []sdp.Record
	for _, r := range s.records {
		if search.Wants(r.UUIDs()) {
			records = append(records, search.Selected(r))
		}
	}

	return sdp.SearchAttributeResponse(p.Transaction, records)
}

func (s *Sink) signal(ch *l2cap.Channel, payload []byte) ([]l2cap.Frame, error) {
	m, err := avdtp.ParseMessage(payload)
	if err != nil {
		return nil, err
	}

	answer := s.stream.Handle(m)

	if answer.Signal == 0 {
		return nil, nil
	}
	return s.send(ch, answer.Marshal()), nil
}

func (s *Sink) audio(payload []byte) error {
	m, err := avdtp.ParseMedia(payload)
	if err != nil {
		return err
	}

	body := s.media.Push(m)
	if body == nil || s.Frame == nil {
		return nil
	}

	for len(body) > 0 {
		n, ok := frameLength(body)
		if !ok {
			return fmt.Errorf("bt: %d bytes left over that are not a frame", len(body))
		}
		s.Frame(body[:n])
		body = body[n:]
	}
	return nil
}

func frameLength(buf []byte) (int, bool) {
	h, err := sbc.ParseHeader(buf)
	if err != nil {
		return 0, false
	}

	n := h.Length()
	if n <= 0 || n > len(buf) {
		return 0, false
	}
	return n, true
}

func (s *Sink) send(ch *l2cap.Channel, payload []byte) []l2cap.Frame {
	mtu := int(ch.MTU)
	if mtu <= 0 {
		mtu = l2cap.MTUDefault
	}

	var out []l2cap.Frame
	for len(payload) > mtu {
		out = append(out, l2cap.Frame{CID: ch.Remote, Payload: payload[:mtu]})
		payload = payload[mtu:]
	}
	return append(out, l2cap.Frame{CID: ch.Remote, Payload: payload})
}

func (s *Sink) Forget() {
	s.l2.Forget()
	s.stream.Forget()
	s.remote.Forget()
	s.media = avdtp.Reassembler{}
	s.command = avrcp.Reassembler{}
	s.signalling = 0
	s.transport = 0
	s.control = 0
}
