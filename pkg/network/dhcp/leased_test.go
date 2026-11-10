package dhcp

import (
	"context"
	"testing"
	"time"
)

func TestFreshWaitsForANewLeaseNotTheOneHeld(t *testing.T) {
	c := &Client{}
	c.set(&Lease{})

	quick, stop := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer stop()
	if err := c.Fresh(quick); err == nil {
		t.Fatal("Fresh returned for the lease already held")
	}

	done := make(chan error, 1)
	go func() { done <- c.Fresh(context.Background()) }()
	tick := time.NewTicker(5 * time.Millisecond)
	defer tick.Stop()
	timeout := time.After(2 * time.Second)
	for {
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("Fresh: %v", err)
			}
			return
		case <-tick.C:
			c.set(&Lease{})
		case <-timeout:
			t.Fatal("Fresh never heard the new lease")
		}
	}
}

func TestLeasedCarriesEachChangeOnce(t *testing.T) {
	c := &Client{}
	var got []*Lease
	stop := c.Leased.Listen(func(l *Lease) { got = append(got, l) })
	defer stop()

	l := &Lease{}
	c.set(l)
	c.set(l)
	c.set(nil)
	c.set(nil)

	if len(got) != 2 || got[0] != l || got[1] != nil {
		t.Errorf("heard %v, want the lease then nil", got)
	}
}
