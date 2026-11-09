//go:build !linux

package input

import (
	"errors"
	"os"
)

func (d *Device) Abs(uint16) (int32, error) {
	return 0, errors.New("input: absolute axes are Linux only")
}

func AbsRange(*os.File, uint16) (min, max int32, err error) {
	return 0, 0, errors.New("input: absolute axes are Linux only")
}
