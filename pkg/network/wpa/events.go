package wpa

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	EventConnected    = "CTRL-EVENT-CONNECTED"
	EventStateChange  = "CTRL-EVENT-STATE-CHANGE"
	EventDisconnected = "CTRL-EVENT-DISCONNECTED"
	EventTerminating  = "CTRL-EVENT-TERMINATING"
	EventScanResults  = "CTRL-EVENT-SCAN-RESULTS"
)

// wpa_state COMPLETED.
const stateCompleted = 9

// Sending STATUS makes the supplicant emit CTRL-EVENT-STATE-CHANGE again.
type StateChange struct {
	Completed bool
	SSID      string
	BSSID     string
}

// Format: "id=0 state=9 BSSID=.. SSID=..", with SSID running to the end of the line.
func ParseState(text string) (StateChange, bool) {
	rest, ok := strings.CutPrefix(text, EventStateChange+" ")
	if !ok {
		return StateChange{}, false
	}

	var s StateChange
	hasState := false
	for rest != "" {
		var field string
		field, rest, _ = strings.Cut(rest, " ")

		key, value, ok := strings.Cut(field, "=")
		if !ok {
			continue
		}

		switch key {
		case "state":
			n, err := strconv.Atoi(value)
			if err != nil {
				return StateChange{}, false
			}
			hasState = true
			s.Completed = n == stateCompleted
		case "BSSID":
			s.BSSID = value
		case "SSID":
			s.SSID = value
			if rest != "" {
				s.SSID += " " + rest
			}
			rest = ""
		}
	}
	return s, hasState
}

type Event struct {
	Priority int
	Text     string
}

func (e Event) Is(kind string) bool { return e.Text == kind || strings.HasPrefix(e.Text, kind+" ") }

type EventConn interface {
	Cmd(string) (string, error)
	Read([]byte) (int, error)
	SetReadDeadline(time.Time) error
}

func Monitor(ctx context.Context, c EventConn, initial func(), handler func(Event)) error {
	if out, err := c.Cmd("ATTACH"); err != nil {
		return err
	} else if strings.TrimSpace(out) != "OK" {
		return fmt.Errorf("wifi: ATTACH said %q", out)
	}
	defer c.Cmd("DETACH")
	if initial != nil {
		initial()
	}

	buf := make([]byte, 4096)
	for {
		if ctx.Err() != nil {
			return nil
		}

		if err := c.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
			return err
		}

		n, err := c.Read(buf)
		if err != nil {
			if isTimeout(err) {
				continue
			}
			return fmt.Errorf("wifi: reading events: %w", err)
		}
		if e, ok := ParseEvent(string(buf[:n])); ok {
			handler(e)
		}
	}
}

// Format: "<3>CTRL-EVENT-CONNECTED ...".
func ParseEvent(raw string) (Event, bool) {
	raw = strings.TrimRight(raw, "\n")
	if !strings.HasPrefix(raw, "<") {
		return Event{}, false
	}
	end := strings.Index(raw, ">")
	if end < 0 {
		return Event{}, false
	}

	priority, err := strconv.Atoi(raw[1:end])
	if err != nil {
		return Event{}, false
	}
	return Event{Priority: priority, Text: raw[end+1:]}, true
}

func isTimeout(err error) bool {
	type timeout interface{ Timeout() bool }
	t, ok := err.(timeout)
	return ok && t.Timeout()
}
