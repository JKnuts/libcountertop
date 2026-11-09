package touch

import (
	"fmt"
	"os"

	"github.com/ygelfand/libcountertop/pkg/input"
)

type axisRange struct{ min, max int32 }
type axes struct{ x, y axisRange }

func readAxis(f *os.File, code uint16) (axisRange, error) {
	minimum, maximum, err := input.AbsRange(f, code)
	if err != nil {
		return axisRange{}, fmt.Errorf("touch: EVIOCGABS(%#x): %w", code, err)
	}
	if maximum <= minimum {
		return axisRange{}, fmt.Errorf("touch: invalid range %d..%d for axis %#x", minimum, maximum, code)
	}
	return axisRange{minimum, maximum}, nil
}

func readAxes(f *os.File) (axes, error) {
	x, err := readAxis(f, absMTPositionX)
	if err != nil {
		return axes{}, err
	}
	y, err := readAxis(f, absMTPositionY)
	if err != nil {
		return axes{}, err
	}
	return axes{x, y}, nil
}

func (a axisRange) pixel(value, size int) int {
	if size <= 1 || a.max <= a.min {
		return 0
	}
	lo, hi := int64(a.min), int64(a.max)
	offset := min(max(int64(value), lo), hi) - lo
	span := hi - lo
	// Both endpoints are inclusive. Round to the nearest pixel using ARM32-safe arithmetic.
	return int((offset*int64(size-1) + span/2) / span)
}
