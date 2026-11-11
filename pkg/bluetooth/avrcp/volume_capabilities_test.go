package avrcp

import "testing"

func TestMacCanDiscoverAbsoluteVolume(t *testing.T) {
	c := New()
	c.Volumed(70)
	out, err := c.Receive(
		Transport{
			Label:   4,
			Type:    MessageCommand,
			Payload: GetCapabilities(CapabilityEvents).Frame(Status).Marshal(),
		},
	)
	if err != nil || len(out) != 1 {
		t.Fatalf("capabilities: %v %v", out, err)
	}
	f, err := ParseAVC(out[0].Payload)
	if err != nil || f.Code != Stable {
		t.Fatalf("capabilities refused: %v %v", f, err)
	}
	p, err := ParsePDU(f.Operands)
	if err != nil {
		t.Fatal(err)
	}
	events, err := ParseCapabilities(p.Params)
	if err != nil || len(events) != 1 || events[0] != EventVolume {
		t.Fatalf("events: %v %v", events, err)
	}
	out, err = c.Receive(
		Transport{
			Label: 5,
			Type:  MessageCommand,
			Payload: PDU{
				ID:     PDURegisterNotification,
				Params: []byte{EventVolume, 0, 0, 0, 0},
			}.Frame(Status).
				Marshal(),
		},
	)
	if err != nil || len(out) != 1 {
		t.Fatal("volume registration failed", err)
	}
	f, _ = ParseAVC(out[0].Payload)
	p, _ = ParsePDU(f.Operands)
	if f.Code != Interim || len(p.Params) != 2 || p.Params[1] != Volume(70) {
		t.Fatalf("wrong initial volume: %x", out[0].Payload)
	}
}
