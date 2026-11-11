package a2dp

import "math"

func (s *Sink) setPlaybackGain(db int) {
	gain := math.Pow(10, float64(min(max(db, 0), maxPlaybackGainDB))/20)
	s.playbackGain.Store(math.Float64bits(gain))
}

func (s *Sink) applyPlaybackGain(samples []int16) {
	boostPlayback(samples, math.Float64frombits(s.playbackGain.Load()))
}

// boostPlayback applies optional gain with a soft knee above 75% of full scale.
func boostPlayback(samples []int16, gain float64) {
	if !(gain > 1) {
		return
	}
	gain = min(gain, 4)
	const knee = 24576
	const headroom = 32767 - knee
	for i, sample := range samples {
		x := int32(float64(sample) * gain)
		sign := int32(1)
		if x < 0 {
			x, sign = -x, -1
		}
		if x > knee {
			over := x - knee
			x = knee + headroom*over/(headroom+over)
		}
		samples[i] = int16(x * sign)
	}
}
