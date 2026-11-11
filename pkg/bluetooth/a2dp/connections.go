package a2dp

import (
	"cmp"
	"context"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ygelfand/libcountertop/pkg/bluetooth/pair"
)

// Connection is an attached Bluetooth Classic peer. The address identifies the
// connection even when two devices have the same display name.
type Connection struct {
	Address string
	Name    string
	Active  bool
}

type connectedPeer struct {
	address      pair.Addr
	sink         *Sink
	lastActivity time.Time
}

// connections keeps one protocol stack per HCI handle. The lock serializes
// incoming packets, link events and selection. Only active owns the PCM queue.
type connections struct {
	mu     sync.Mutex
	host   *Sink
	name   string
	peers  map[uint16]*connectedPeer
	send   func(uint16, []byte) error
	volume chan struct{}
	active atomic.Pointer[Sink]
	manual bool
}

func (s *Sink) connections() *connections {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.links
}

func (s *Sink) selected() *Sink {
	if links := s.connections(); links != nil {
		return links.active.Load()
	}
	return nil
}

// Connections returns the current peers and whether playback follows new sources.
func (s *Sink) Connections() ([]Connection, bool) {
	links := s.connections()
	if links == nil {
		return nil, true
	}
	links.mu.Lock()
	defer links.mu.Unlock()

	active := links.active.Load()
	out := make([]Connection, 0, len(links.peers))
	for _, p := range links.peers {
		address, name := p.address.String(), p.sink.Label()
		if name == "" {
			name = address
		}
		out = append(out, Connection{Address: address, Name: name, Active: p.sink == active})
	}
	slices.SortFunc(out, func(a, b Connection) int {
		return cmp.Or(cmp.Compare(a.Name, b.Name), cmp.Compare(a.Address, b.Address))
	})
	return out, !links.manual
}

// SelectConnection pins a connected source. An empty address restores automatic
// selection. Selecting a source never drops either Bluetooth connection.
func (s *Sink) SelectConnection(address string) error {
	links := s.connections()
	if links == nil {
		if address == "" {
			return nil
		}
		return fmt.Errorf("bluetooth: no connected source")
	}
	links.mu.Lock()
	defer links.mu.Unlock()

	if address == "" {
		links.manual = false
		links.activate(links.latest())
		return nil
	}
	for _, p := range links.peers {
		if p.address.String() == address {
			links.manual = true
			links.activate(p.sink)
			return nil
		}
	}
	return fmt.Errorf("bluetooth: source %s is no longer connected", address)
}

func (c *connections) watchVolume(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-c.volume:
			c.mu.Lock()
			if peer := c.active.Load(); peer != nil {
				peer.volumed(c.host.options.Volume.Level())
			}
			c.mu.Unlock()
		}
	}
}

func (c *connections) Sending(send func(uint16, []byte) error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.send = send
	for _, p := range c.peers {
		p.sink.Sending(send)
	}
}

func (c *connections) Event(e pair.Event) ([]pair.Command, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	out, err := c.host.pairEvent(e)
	if err != nil {
		return nil, err
	}
	switch e.Code {
	case pair.EventConnectionComplete:
		address, ok := pair.Connected(e.Params)
		if !ok || len(e.Params) < 11 || e.Params[9] != pair.LinkACL {
			return out, nil
		}
		handle, _ := pair.Handle(e.Params)
		if c.peers[handle] == nil {
			p := &Sink{
				options: c.host.options,
				dumping: c.host.dumping,
				owner:   c,
				handle:  handle,
				send:    c.send,
			}
			p.startPeer(c.name)
			c.peers[handle] = &connectedPeer{address: address, sink: p}
			if c.active.Load() == nil {
				c.activate(p)
			}
		}
		out = append(out, pair.RemoteName(address))
	case pair.EventRemoteName:
		address, name, err := pair.ParseRemoteName(e.Params)
		if err != nil {
			return out, nil
		}
		for _, p := range c.peers {
			if p.address == address {
				p.sink.mu.Lock()
				p.sink.label = name
				p.sink.mu.Unlock()
				slog.Info("bluetooth connected", "name", name)
				c.host.options.Media.Changed()
				break
			}
		}
	}
	return out, nil
}

func (c *connections) Data(handle uint16, pdu []byte) ([][]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if p := c.peers[handle]; p != nil {
		return p.sink.Data(handle, pdu)
	}
	return nil, nil
}

func (c *connections) Gone(handle uint16) {
	c.mu.Lock()
	defer c.mu.Unlock()
	p := c.peers[handle]
	if p == nil {
		return
	}
	delete(c.peers, handle)
	p.sink.Sending(nil)
	if c.active.Load() == p.sink {
		c.manual = false
		c.activate(c.latest())
	}
	slog.Info("bluetooth source disconnected", "handle", handle)
}

func (c *connections) close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.activate(nil)
	clear(c.peers)
}

func (c *connections) latest() *Sink {
	var latest *connectedPeer
	for _, p := range c.peers {
		if latest == nil || p.lastActivity.After(latest.lastActivity) ||
			(p.lastActivity.Equal(latest.lastActivity) && p.sink.handle < latest.sink.handle) {
			latest = p
		}
	}
	if latest == nil {
		return nil
	}
	return latest.sink
}

func (c *connections) activate(next *Sink) {
	previous := c.active.Swap(next)
	if previous == next {
		return
	}
	if previous != nil {
		if previous.playing() {
			c.host.options.Arbitration.Gave(previous)
		}
		previous.detach()
	}
	c.host.options.Queue.Drain()
	if next != nil {
		if volume := c.host.options.Volume; volume != nil {
			next.volumed(volume.Level())
		}
		slog.Info(
			"bluetooth source selected",
			"handle",
			next.handle,
			"name",
			next.Label(),
			"manual",
			c.manual,
		)
		if next.playing() {
			next.beginPlayback()
		}
	}
	c.host.options.Media.Changed()
}

const sourceIdle = 2 * time.Second

func (c *connections) started(p *Sink) {
	// AVDTP Start can precede the first media packet. Give the new source
	// the same idle window as a source that has just sent a packet.
	c.peers[p.handle].lastActivity = time.Now()
	if c.active.Load() == p {
		p.beginPlayback()
	} else if !c.manual {
		c.activate(p)
	}
}

func (c *connections) frame(p *Sink, frame []byte) {
	now := time.Now()
	c.peers[p.handle].lastActivity = now
	current := c.active.Load()
	if !c.manual && current != p && p.playing() {
		if current == nil || !current.playing() ||
			now.Sub(c.peers[current.handle].lastActivity) > sourceIdle {
			c.activate(p)
		}
	}
	if c.active.Load() == p {
		p.play(frame)
	}
}
