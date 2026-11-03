package volume

import (
	"sync"
	"time"

	"github.com/ygelfand/libcountertop/pkg/display/shell"
	config "github.com/ygelfand/libcountertop/pkg/settings/schema"
)

const (
	linger = 5 * time.Second
	opened = 8 * time.Second
)

type Card struct {
	shell      *shell.Shell
	generation uint64
	mu         sync.Mutex
	held       *shell.Hold
	timer      *time.Timer
	open       bool

	stream config.Stream
}

func (c *Card) Covers() bool { return false }

func (c *Card) Selected() (config.Stream, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.held.Held() {
		return "", false
	}
	return c.stream, true
}

func (c *Card) Show(stream config.Stream) {
	c.mu.Lock()
	if c.held.Held() && c.open {
		c.mu.Unlock()
		c.stirred(true)
		return
	}
	c.stream = stream

	if !c.held.Held() {
		c.held = c.shell.Hold(c)
		c.mu.Unlock()

		c.Linger()
		return
	}
	c.mu.Unlock()

	c.shell.Redraw()
	c.Linger()
}

func (c *Card) Stir() {
	c.mu.Lock()
	up := c.held.Held()
	c.mu.Unlock()

	c.stirred(up)
}

func (c *Card) stirred(up bool) {
	if !up {
		return
	}

	c.shell.Redraw()
	c.Linger()
}

func (c *Card) Linger() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.generation++
	stay := linger
	if c.open {
		stay = opened
	}

	if c.timer == nil {
		generation := c.generation
		c.timer = time.AfterFunc(stay, func() { c.expire(generation) })
		return
	}
	c.timer.Stop()
	generation := c.generation
	c.timer = time.AfterFunc(stay, func() { c.expire(generation) })
}

func (c *Card) Dismiss() {
	c.mu.Lock()
	held := c.held
	c.generation++
	c.held, c.open = nil, false

	if c.timer != nil {
		c.timer.Stop()
		c.timer = nil
	}
	c.mu.Unlock()

	held.Release()
}

func (c *Card) Expand() {
	c.mu.Lock()
	c.open = !c.open
	c.mu.Unlock()

	c.Linger()
}

func (c *Card) selects(stream config.Stream) func() {
	return func() {
		c.mu.Lock()
		c.stream, c.open = stream, false
		c.mu.Unlock()

		c.Linger()
	}
}

func (c *Card) Shows(s config.Stream) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.held.Held() && (c.open || c.stream == s)
}

func NewCard(stack *shell.Shell) *Card { return &Card{shell: stack} }
func (c *Card) Picked() (config.Stream, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.stream, c.open
}
func (c *Card) Pick(stream config.Stream) { c.selects(stream)() }
func (c *Card) Close()                    { c.Dismiss() }
func (c *Card) expire(generation uint64) {
	c.mu.Lock()
	if c.generation != generation {
		c.mu.Unlock()
		return
	}
	held := c.held
	c.held, c.open, c.timer = nil, false, nil
	c.mu.Unlock()
	held.Release()
}
