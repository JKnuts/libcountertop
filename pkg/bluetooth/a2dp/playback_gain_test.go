package a2dp

import (
	"slices"
	"testing"

	"github.com/ygelfand/libcountertop/pkg/settings/schema"
)

func TestQuietPlaybackIsRaisedWithoutClippingLoudPeaks(t *testing.T) {
	s := []int16{0, 860, -860, 6144, 12000, 32767, -32768}
	unchanged := slices.Clone(s)
	boostPlayback(s, 0)
	if !slices.Equal(s, unchanged) {
		t.Fatalf("default gain changed samples: %v", s)
	}
	boostPlayback(s, 4)
	if s[0] != 0 || s[1] != 3440 || s[2] != -3440 {
		t.Fatalf("silence or quiet signal changed unexpectedly: %v", s)
	}
	if s[3] >= s[4] || s[4] >= s[5] || s[5] == 32767 || s[6] == -32768 {
		t.Fatalf("peak limiter clipped or folded samples: %v", s)
	}
}

type gainSettings struct{ saved schema.Bluetooth }

func (s *gainSettings) Read() schema.Bluetooth { return s.saved }

func (s *gainSettings) Update(change func(*schema.Bluetooth)) error {
	change(&s.saved)
	return nil
}

func TestPlaybackGainCanChangeWithoutRestartingBluetooth(t *testing.T) {
	settings := &gainSettings{}
	sink := New(Dependencies{Settings: settings})
	if err := sink.Controls().Change(playbackGainSetting, "6"); err != nil {
		t.Fatal(err)
	}

	select {
	case <-sink.wake:
		t.Fatal("changing the gain interrupted the Bluetooth service")
	default:
	}
	if err := sink.Controls().Change(playbackGainSetting, "13"); err == nil {
		t.Fatal("accepted gain above 12 dB")
	}

	for _, candidate := range []*Sink{sink, New(Dependencies{Settings: settings})} {
		samples := []int16{1000}
		candidate.applyPlaybackGain(samples)
		if samples[0] < 1990 || samples[0] > 2000 {
			t.Fatalf("+6 dB turned 1000 into %d, want about 1995", samples[0])
		}
	}
}
