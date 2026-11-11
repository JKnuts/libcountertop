package a2dp

import (
	"slices"
	"testing"
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
