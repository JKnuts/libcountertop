package homecontrol

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/ygelfand/libcountertop/pkg/display/shell"
	ha "github.com/ygelfand/libcountertop/pkg/homeassistant"
	"github.com/ygelfand/libcountertop/pkg/homeassistant/states"
	config "github.com/ygelfand/libcountertop/pkg/settings/schema"
)

type homeStore struct {
	mu    sync.Mutex
	value config.Home
}

var testHome homeStore
var activeEngine *Engine

func resetTestHome() { testHome.mu.Lock(); testHome.value = config.DefaultHome(); testHome.mu.Unlock() }
func (s *homeStore) Read() config.Home {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.value
	c.Picks = map[string]config.HomePick{}
	for k, v := range s.value.Picks {
		c.Picks[k] = v.Clone()
	}
	c.Control = map[string]bool{}
	for k, v := range s.value.Control {
		c.Control[k] = v
	}
	c.Group = map[string]int{}
	for k, v := range s.value.Group {
		c.Group[k] = v
	}
	return c
}
func (s *homeStore) Pick(k string, v config.HomePick) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.value.Picks == nil {
		s.value.Picks = map[string]config.HomePick{}
	}
	s.value.Picks[k] = v.Clone()
	return nil
}
func (s *homeStore) Control(k string, v bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.value.Control == nil {
		s.value.Control = map[string]bool{}
	}
	s.value.Control[k] = v
	return nil
}
func (s *homeStore) Combine(v bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.value.Combine = v
	return nil
}
func (s *homeStore) Update(change func(*config.Home)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	change(&s.value)
	return nil
}

type homeClient struct{}

func (homeClient) Entities(context.Context, ha.Filter) ([]ha.Entity, error) { return nil, nil }
func (homeClient) Labels(context.Context) ([]ha.Label, error)               { return nil, nil }
func (homeClient) Call(context.Context, string, map[string]string) error    { return nil }
func (homeClient) Perform(context.Context, string, map[string]string, map[string]string) error {
	return nil
}

func (homeClient) Template(context.Context, string) (json.RawMessage, error) { return nil, nil }
func (homeClient) Enable(bool)                                               {}
func (homeClient) Probe()                                                    {}
func (homeClient) Watch(func(ha.Access)) func()                              { return func() {} }

type homeStates struct{}

func (homeStates) Follow(
	e, a string,
) *states.Value {
	return &states.Value{Entity: e, Attribute: a}
}

func testEngine() *Engine {
	e := New(
		Dependencies{
			Settings: &testHome,
			Shell:    shell.New(),
			Client:   homeClient{},
			States:   homeStates{},
		},
	)
	activeEngine = e
	return e
}
