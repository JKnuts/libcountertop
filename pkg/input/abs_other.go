//go:build !linux

package input

import (
	"errors"
	"os"
)

var errNoAbs = errors.New("input: absolute axes are Linux only")

func AbsInfo(*os.File, uint16) (Axis, error) { return Axis{}, errNoAbs }

func (d *Device) Abs(uint16) (int32, error) { return 0, errNoAbs }
