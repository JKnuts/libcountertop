package touch

import (
	"os"

	"github.com/ygelfand/libcountertop/pkg/input"
)

type span struct{ lo, hi int32 }

type axes struct{ x, y span }

func readAxes(f *os.File) axes {
	return axes{readSpan(f, absMTPositionX), readSpan(f, absMTPositionY)}
}

func readSpan(f *os.File, code uint16) span {
	a, err := input.AbsInfo(f, code)
	if err != nil || a.Max <= a.Min {
		return span{}
	}
	return span{a.Min, a.Max}
}

func (s span) pixel(v, size int) int {
	if s.hi <= s.lo || size <= 1 {
		return v
	}
	lo, hi := int64(s.lo), int64(s.hi)
	off := min(max(int64(v), lo), hi) - lo
	return int((off*int64(size-1) + (hi-lo)/2) / (hi - lo))
}
