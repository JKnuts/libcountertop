package homecontrol

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/ygelfand/libcountertop/pkg/homeassistant"
)

const (
	fetchWait  = 10 * time.Second
	cacheFor   = 10 * time.Minute
	retryAfter = 5 * time.Second
)

type result struct {
	entities   []homeassistant.Entity
	labels     []homeassistant.Label
	err        error
	at         time.Time
	asking     bool
	refreshing bool
}

type cache struct {
	closed  bool
	ctx     context.Context
	workers sync.WaitGroup
	mu      sync.Mutex
	got     map[string]*result
	fetch   func(context.Context, homeassistant.Filter) ([]homeassistant.Entity, []homeassistant.Label, error)
	now     func() time.Time
	done    func()
}

func (c *cache) get(s Selection) result {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return result{err: context.Canceled}
	}
	r := c.got[s.Key]
	switch {
	case r == nil:
		r = &result{asking: true}
		c.got[s.Key] = r
	case !r.asking && !r.refreshing && r.err == nil && c.now().Sub(r.at) > cacheFor:
		r.refreshing = true
	case !r.asking && r.err != nil && c.now().Sub(r.at) > retryAfter:
		r.asking = true
	default:
		return *r
	}
	c.workers.Add(1)
	go func() { defer c.workers.Done(); c.load(s, r) }()
	return *r
}

func (c *cache) load(s Selection, r *result) {
	parent := c.ctx
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, fetchWait)
	defer cancel()
	entities, labels, err := c.fetch(ctx, s.Filter)
	if err != nil {
		slog.Warn("Home Assistant did not list the entities", "selection", s.Key, "err", err)
	}
	if err == nil && entities == nil {
		entities = []homeassistant.Entity{}
	}
	c.mu.Lock()
	switch {
	case c.got[s.Key] != r:
	case err != nil && r.refreshing:
		r.refreshing, r.at = false, c.now()
	default:
		c.got[s.Key] = &result{entities: entities, labels: labels, err: err, at: c.now()}
	}
	active := !c.closed
	c.mu.Unlock()
	if active {
		c.done()
	}
}

func (c *cache) forget(key string) {
	c.mu.Lock()
	delete(c.got, key)
	c.mu.Unlock()
	c.done()
}

func (c *cache) retry() {
	c.mu.Lock()
	for k, r := range c.got {
		if r.err != nil {
			delete(c.got, k)
		}
	}
	c.mu.Unlock()
	c.done()
}

func (c *cache) clear() {
	c.mu.Lock()
	c.got = map[string]*result{}
	c.mu.Unlock()
	c.done()
}

func (c *cache) stale() {
	c.mu.Lock()
	for k, r := range c.got {
		switch {
		case r.err != nil:
			delete(c.got, k)
		case !r.asking:
			r.at = time.Time{}
		}
	}
	c.mu.Unlock()
	c.done()
}

func (e *Engine) Refresh() { e.cache.stale(); e.options.Client.Probe() }
