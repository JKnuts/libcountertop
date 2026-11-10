package wifi

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ygelfand/libcountertop/pkg/network/wpa"
)

type moves struct {
	release chan error
	asked   chan string
}

func (*moves) Network() string                             { return "old" }
func (*moves) Scan(context.Context) ([]wpa.Network, error) { return nil, nil }
func (m *moves) Move(_ context.Context, ssid, _ string, _ func(context.Context) error) error {
	m.asked <- ssid
	return <-m.release
}

func settled(t *testing.T, m *Mover) MoveState {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if s := m.State(); !s.Moving {
			return s
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("the move never finished")
	return MoveState{}
}

func TestOneMoveAtATimeWithItsDeadline(t *testing.T) {
	n := &moves{release: make(chan error), asked: make(chan string, 1)}
	m := NewMover(n, nil)
	at := time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC)
	m.now = func() time.Time { return at }

	if err := m.Start("new", "pass"); err != nil {
		t.Fatal(err)
	}
	<-n.asked
	if s := m.State(); !s.Moving || s.SSID != "new" || !s.Until.Equal(at.Add(MoveWindow)) {
		t.Errorf("while moving: %+v, want moving to new until %v", s, at.Add(MoveWindow))
	}
	if err := m.Start("other", "pass"); !errors.Is(err, ErrMoving) {
		t.Errorf("a second move during the first: %v, want ErrMoving", err)
	}

	n.release <- errors.New("no address")
	if s := settled(t, m); s.Failed == "" {
		t.Errorf("after a failed move: %+v, want the reason", s)
	}

	if err := m.Start("new", "pass"); err != nil {
		t.Fatalf("starting again after a failure: %v", err)
	}
	<-n.asked
	n.release <- nil
	if s := settled(t, m); s.Failed != "" || s.SSID != "new" {
		t.Errorf("after a good move: %+v, want new with no failure", s)
	}
}

func TestAMoveNeedsANetwork(t *testing.T) {
	m := NewMover(&moves{}, nil)
	if err := m.Start("", "pass"); err == nil {
		t.Error("a move to no network started")
	}
}
