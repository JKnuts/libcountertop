package sendspin

import (
	"time"

	"github.com/Sendspin/sendspin-go/pkg/protocol"

	"github.com/ygelfand/libcountertop/pkg/audio/background"
	"github.com/ygelfand/libcountertop/pkg/audio/volume"
	"github.com/ygelfand/libcountertop/pkg/settings/schema"
	"github.com/ygelfand/libcountertop/pkg/settings/storage"
)

const Rate = 48000
const Channels = 2
const Bits = 16
const DefaultHardwareTail = 150 * time.Millisecond
const StateOff = "off"
const StateWaiting = "waiting"
const StateJoined = "joined"
const StatePlaying = "playing"
const stateWaiting = StateWaiting
const stateJoined = StateJoined
const statePlaying = StatePlaying
const Service = "_sendspin._tcp"
const Domain = "local."
const Path = "/sendspin"
const path = Path

type Renderer interface{ Render(at uint64, out []int16) }
type Producer = background.Producer
type Sink interface {
	Attach(Renderer)
	Written() uint64
}
type Arbitration interface {
	Took(Producer)
	Gave(Producer)
}
type Controls struct {
	Volume       volume.Channel
	Media        interface{ Mute(bool) }
	Settings     storage.Section[schema.Media]
	HardwareTail time.Duration
}
type Identity struct {
	ID, Model, Manufacturer, Version string
	ArtworkSize                      int
}
type Callbacks struct {
	State      func(string)
	Connection func(*Session)
	Began      func()
	Grouped    func(string)
	Metadata   func(*protocol.MetadataState, func() int64)
	Artwork    func(protocol.ArtworkChunk)
	Controller func(protocol.ControllerState)
}

func (c Callbacks) state(s string) {
	if c.State != nil {
		c.State(s)
	}
}
func (c Callbacks) connection(s *Session) {
	if c.Connection != nil {
		c.Connection(s)
	}
}
func (c Callbacks) began() {
	if c.Began != nil {
		c.Began()
	}
}
func (c Callbacks) grouped(s string) {
	if c.Grouped != nil {
		c.Grouped(s)
	}
}
func (c Callbacks) metadata(m *protocol.MetadataState, now func() int64) {
	if c.Metadata != nil {
		c.Metadata(m, now)
	}
}
func (c Callbacks) artwork(a protocol.ArtworkChunk) {
	if c.Artwork != nil {
		c.Artwork(a)
	}
}

func (c Callbacks) controller(s protocol.ControllerState) {
	if c.Controller != nil {
		c.Controller(s)
	}
}
