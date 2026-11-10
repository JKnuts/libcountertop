package wpa

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

type fake struct {
	on string

	joins   map[string]bool
	refuses map[string]bool
	blocked map[string]bool

	configured map[string]string
	next       int

	said  []string
	saved int
}

func newFake(on string) *fake {
	return &fake{
		on:         on,
		joins:      map[string]bool{},
		refuses:    map[string]bool{},
		blocked:    map[string]bool{},
		configured: map[string]string{},
	}
}

func (f *fake) Cmd(words ...string) (string, error) {
	f.said = append(f.said, strings.Join(words, " "))

	switch words[0] {
	case "status":
		if f.on == "" {
			return "wpa_state=DISCONNECTED", nil
		}
		return fmt.Sprintf("ssid=%s\nwpa_state=COMPLETED\nip_address=192.168.1.10", f.on), nil

	case "list_networks":
		out := "network id / ssid / bssid / flags"
		for id, name := range f.configured {
			flags := "[CURRENT]"
			if f.blocked[id] {
				flags = "[TEMP-DISABLED]"
			}
			out += fmt.Sprintf("\n%s\t%s\tany\t%s", id, name, flags)
		}
		return out, nil

	case "add_network":
		id := fmt.Sprint(f.next)
		f.next++
		f.configured[id] = ""
		return id, nil

	case "set_network":
		if len(words) == 4 && words[2] == "ssid" {
			f.configured[words[1]] = unhex(words[3])
		}
		return "OK", nil

	case "remove_network":
		delete(f.configured, words[1])
		return "OK", nil

	case "select_network":
		name := f.configured[words[1]]
		if f.joins[name] {
			f.on = name
		}
		if f.refuses[name] {
			f.blocked[words[1]] = true
		}
		return "OK", nil

	case "enable_network", "reassociate", "disconnect":
		return "OK", nil

	case "save_config":
		f.saved++
		return "OK", nil
	}
	return "OK", nil
}

func (f *fake) asked(cmd string) bool {
	for _, s := range f.said {
		if strings.HasPrefix(s, cmd) {
			return true
		}
	}
	return false
}

func unhex(s string) string {
	var out []byte
	for i := 0; i+1 < len(s); i += 2 {
		var b int
		_, _ = fmt.Sscanf(s[i:i+2], "%02x", &b)
		out = append(out, byte(b))
	}
	return string(out)
}

func TestSwitchingToAWorkingNetworkTakes(t *testing.T) {
	f := newFake("old")
	f.joins["new"] = true

	if err := Switch(context.Background(), f, "new", "a good passphrase"); err != nil {
		t.Fatalf("switching to a network that associates: %v", err)
	}
	if f.on != "new" {
		t.Errorf("on %q after switching, want %q", f.on, "new")
	}
	if f.saved == 0 {
		t.Error("the configuration was not saved, so the network would not survive a reboot")
	}
}

func TestASwitchForgetsTheOldNetwork(t *testing.T) {
	f := newFake("old")
	f.configured["0"], f.next = "old", 1
	f.joins["new"] = true

	if err := Switch(context.Background(), f, "new", "a good passphrase"); err != nil {
		t.Fatalf("switching to a network that associates: %v", err)
	}
	for id, name := range f.configured {
		if name != "new" {
			t.Errorf("network %s (%q) is still configured after the switch", id, name)
		}
	}
	if len(f.configured) != 1 {
		t.Errorf("%d networks configured after the switch, want 1", len(f.configured))
	}
}

func TestAFailedSwitchKeepsTheOldNetwork(t *testing.T) {
	f := newFake("old")
	f.configured["0"], f.next = "old", 1
	f.joins["new"] = false

	within, stop := context.WithTimeout(context.Background(), 2*time.Second)
	defer stop()
	_ = Switch(within, f, "new", "the wrong passphrase")

	if f.configured["0"] != "old" {
		t.Errorf("the old network is gone after a failed switch: %v", f.configured)
	}
}

func TestAMoveThatNeverGetsReadyGoesBack(t *testing.T) {
	f := newFake("old")
	f.configured["0"], f.next = "old", 1
	f.joins["new"] = true

	err := Move(context.Background(), f, "new", "a good passphrase",
		func(context.Context) error { return errors.New("no address") })
	if err == nil {
		t.Fatal("a move that never got ready succeeded")
	}
	if f.configured["0"] != "old" || len(f.configured) != 1 {
		t.Errorf("after the move went back, configured %v, want only the old network", f.configured)
	}
}

func TestAMoveThatGetsReadyKeepsOnlyTheNewNetwork(t *testing.T) {
	f := newFake("old")
	f.configured["0"], f.next = "old", 1
	f.joins["new"] = true

	asked := false
	err := Move(context.Background(), f, "new", "a good passphrase",
		func(context.Context) error { asked = f.on == "new"; return nil })
	if err != nil || !asked {
		t.Fatalf("move: %v, ready asked while on the new network: %v", err, asked)
	}
	if len(f.configured) != 1 || f.saved == 0 {
		t.Errorf(
			"configured %v, saved %d times, want only the new network saved",
			f.configured,
			f.saved,
		)
	}
}

func TestARefusedPasswordGoesBackAtOnce(t *testing.T) {
	f := newFake("old")
	f.configured["0"], f.next = "old", 1
	f.refuses["new"] = true

	start := time.Now()
	err := Move(context.Background(), f, "new", "the wrong passphrase", nil)
	if !errors.Is(err, ErrRefused) {
		t.Fatalf("move with a refused password: %v, want ErrRefused", err)
	}
	if took := time.Since(start); took > 2*time.Second {
		t.Errorf("going back took %v, want it at once", took)
	}
	if f.configured["0"] != "old" || len(f.configured) != 1 {
		t.Errorf("after going back, configured %v, want only the old network", f.configured)
	}
}

func TestAMoveGivesUpJoiningAfterSettle(t *testing.T) {
	if testing.Short() {
		t.Skip("waits out Settle")
	}
	f := newFake("old")
	f.configured["0"], f.next = "old", 1

	start := time.Now()
	err := Move(context.Background(), f, "new", "a good passphrase", nil)
	if err == nil {
		t.Fatal("a move to a network that never answers succeeded")
	}
	if took := time.Since(start); took > Settle+3*time.Second {
		t.Errorf("gave up after %v, want about %v", took, Settle)
	}
}

func TestOnlyNeedsANetworkToKeep(t *testing.T) {
	f := newFake("old")
	f.configured["0"] = "old"

	if err := Only(f, ""); err == nil {
		t.Error("keeping no network was accepted")
	}
	if len(f.configured) != 1 {
		t.Errorf("%d networks configured, want the one there was", len(f.configured))
	}
}

func TestAFailedSwitchGoesBack(t *testing.T) {
	f := newFake("old")
	f.joins["new"] = false

	within, stop := context.WithTimeout(context.Background(), 2*time.Second)
	defer stop()

	err := Switch(within, f, "new", "the wrong passphrase")
	if err == nil {
		t.Fatal("switching to a network that never associates reported success")
	}
	if !strings.Contains(err.Error(), "old") {
		t.Errorf("the error is %q, want it to say which network it went back to", err)
	}

	if f.on != "old" {
		t.Errorf("on %q after a failed switch, want to be back on %q", f.on, "old")
	}
	if !f.asked("enable_network all") || !f.asked("reassociate") {
		t.Error("nothing put the other networks back: select_network disables them all")
	}
}

func TestAFailedSwitchLeavesNothingBehind(t *testing.T) {
	f := newFake("old")
	f.joins["new"] = false

	within, stop := context.WithTimeout(context.Background(), 2*time.Second)
	defer stop()
	_ = Switch(within, f, "new", "the wrong passphrase")

	for id, name := range f.configured {
		if name == "new" {
			t.Errorf("network %s is still configured as %q after failing to join", id, name)
		}
	}
}

func TestAFailedSwitchDoesNotSave(t *testing.T) {
	f := newFake("old")
	f.joins["new"] = false

	within, stop := context.WithTimeout(context.Background(), 2*time.Second)
	defer stop()
	_ = Switch(within, f, "new", "the wrong passphrase")

	if f.saved != 0 {
		t.Errorf("saved the configuration %d times after a failed switch, want none", f.saved)
	}
}

func TestSwitchingToTheNetworkAlreadyOnIsRefused(t *testing.T) {
	f := newFake("old")

	if err := Switch(context.Background(), f, "old", "whatever"); err == nil {
		t.Error("switching to the network already in use reported success")
	}
	if f.asked("add_network") {
		t.Error("it configured a second copy of the network it was already on")
	}
}

func TestSwitchingToNothingIsRefused(t *testing.T) {
	f := newFake("old")

	if err := Switch(context.Background(), f, "", "whatever"); err == nil {
		t.Error("switching to an unnamed network reported success")
	}
}

func TestAFailedSwitchFromNothingStillCleansUp(t *testing.T) {
	f := newFake("")
	f.joins["new"] = false

	within, stop := context.WithTimeout(context.Background(), 2*time.Second)
	defer stop()

	if err := Switch(within, f, "new", "the wrong passphrase"); err == nil {
		t.Fatal("switching to a network that never associates reported success")
	}
	if len(f.configured) != 0 {
		t.Errorf("%d networks left configured, want none", len(f.configured))
	}
}

func TestABadPassphraseNeverTouchesTheConnection(t *testing.T) {
	f := newFake("old")

	if err := Switch(context.Background(), f, "new", "short"); err == nil {
		t.Fatal("a five character passphrase was accepted")
	}
	if f.on != "old" {
		t.Errorf("on %q, want to still be on %q", f.on, "old")
	}
	if f.asked("select_network") {
		t.Error("it selected a network before the passphrase was checked")
	}
}
