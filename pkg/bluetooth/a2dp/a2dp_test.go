package a2dp

import (
	"testing"

	"github.com/ygelfand/libcountertop/pkg/bluetooth/pair"
	"github.com/ygelfand/libcountertop/pkg/bluetooth/sbc"
)

func TestMonoIsSentToBothEars(t *testing.T) {
	got := interleave(sbc.Samples{{1, 2, 3}})

	want := []int16{1, 1, 2, 2, 3, 3}
	if len(got) != len(want) {
		t.Fatalf("%d samples from 3 mono ones, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("sample %d is %d, want %d", i, got[i], want[i])
		}
	}
}

func TestStereoIsInterleaved(t *testing.T) {
	got := interleave(sbc.Samples{{1, 3, 5}, {2, 4, 6}})

	want := []int16{1, 2, 3, 4, 5, 6}
	if len(got) != len(want) {
		t.Fatalf("%d samples, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("sample %d is %d, want %d", i, got[i], want[i])
		}
	}
}

func TestNoAudioIsNoSamples(t *testing.T) {
	if got := interleave(nil); got != nil {
		t.Errorf("came back with %d samples", len(got))
	}
	if got := interleave(sbc.Samples{}); got != nil {
		t.Errorf("came back with %d samples", len(got))
	}
}

func TestAskingBeforeAnythingIsAgreed(t *testing.T) {
	var s Sink

	said, err := s.Event(pair.Event{Code: pair.EventConnectionRequest})
	if err != nil {
		t.Errorf("an event before the stack was built: %v", err)
	}
	if len(said) != 0 {
		t.Errorf("%d commands from a sink with no pairing manager", len(said))
	}

	out, err := s.Data(1, []byte{0x04, 0x00, 0x40, 0x00, 1, 2, 3, 4})
	if err != nil {
		t.Errorf("data before the stack was built: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("%d replies from a sink that is not up", len(out))
	}

	s.Gone(1)
}

func TestAnotherLinkEndingIsNotOurs(t *testing.T) {
	s := testSink()
	s.start("test")

	_, _ = s.Data(7, []byte{0x04, 0x00, 0x40, 0x00, 1, 2, 3, 4})

	s.Gone(9)
	if s.handle != 7 {
		t.Errorf("a stranger's link ended and this one is now %d", s.handle)
	}

	s.Gone(7)
	if s.handle != 0 {
		t.Errorf("this link ended and the handle is still %d", s.handle)
	}
}

func TestAMalformedPDUIsRefusedNotAnswered(t *testing.T) {
	s := testSink()
	s.start("test")

	for _, pdu := range [][]byte{
		{},
		{0x01},
		{0xff, 0xff, 0x40, 0x00},
	} {
		out, err := s.Data(1, pdu)
		if err == nil && len(out) > 0 {
			t.Errorf("% x was answered with %d frames", pdu, len(out))
		}
	}
}

func TestThePhonesVolumeIsWiredToTheSpeaker(t *testing.T) {
	s := testSink()
	s.start("volume test")

	if s.sink.Remote().Volume == nil {
		t.Fatal("nothing is listening for the phone's volume")
	}
}

func TestBothRemoteControlRolesAreBuilt(t *testing.T) {
	s := testSink()
	s.start("roles test")

	if s.sink.Remote() == nil {
		t.Fatal("there is no remote control")
	}
	if got := s.sink.Remote().State().Volume; got != 100 {
		t.Errorf(
			"initial local volume is %d, want the saved 100%% rather than a zero-volume reply",
			got,
		)
	}
}
