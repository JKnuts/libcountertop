package wifi

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/ygelfand/libcountertop/pkg/network/wpa"
	"github.com/ygelfand/libcountertop/pkg/runtime/safe"
)

const MoveWindow = 2 * time.Minute

var ErrMoving = errors.New("wifi: already moving to another network")

type Networks interface {
	Network() string
	Scan(ctx context.Context) ([]wpa.Network, error)
	Move(ctx context.Context, ssid, passphrase string, ready func(context.Context) error) error
}

type MoveState struct {
	SSID   string
	Until  time.Time
	Moving bool
	Failed string
}

type Mover struct {
	networks Networks
	ready    func(context.Context) error
	now      func() time.Time

	mu    sync.Mutex
	state MoveState
}

func NewMover(n Networks, ready func(context.Context) error) *Mover {
	return &Mover{networks: n, ready: ready, now: time.Now}
}

func (m *Mover) Network() string { return m.networks.Network() }

func (m *Mover) Scan(ctx context.Context) ([]wpa.Network, error) { return m.networks.Scan(ctx) }

func (m *Mover) State() MoveState {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.state
}

func (m *Mover) Start(ssid, passphrase string) error {
	if ssid == "" {
		return errors.New("wifi: no network named")
	}
	m.mu.Lock()
	if m.state.Moving {
		m.mu.Unlock()
		return ErrMoving
	}
	m.state = MoveState{SSID: ssid, Until: m.now().Add(MoveWindow), Moving: true}
	m.mu.Unlock()

	safe.Go("wifi move", func() { m.run(ssid, passphrase) })
	return nil
}

func (m *Mover) run(ssid, passphrase string) {
	ctx, stop := context.WithTimeout(context.Background(), MoveWindow)
	defer stop()

	err := m.networks.Move(ctx, ssid, passphrase, m.ready)

	m.mu.Lock()
	m.state.Moving = false
	if err != nil {
		m.state.Failed = err.Error()
	}
	m.mu.Unlock()

	if err != nil {
		slog.Warn("wifi move failed", "network", ssid, "err", err)
		return
	}
	slog.Info("wifi moved", "network", ssid)
}
