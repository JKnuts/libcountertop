package input

import (
	"encoding/binary"
	"os"
	"runtime"
	"unsafe"

	"golang.org/x/sys/unix"
)

// EVIOCGABS(abs) = _IOR('E', 0x40+abs, struct input_absinfo), six s32.
const absInfoBytes = 24

func AbsInfo(f *os.File, code uint16) (Axis, error) {
	var b [absInfoBytes]byte
	req := uintptr(2<<30 | absInfoBytes<<16 | 'E'<<8 | (0x40 + uint32(code)))
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, f.Fd(), req, uintptr(unsafe.Pointer(&b[0])))
	runtime.KeepAlive(f)
	if errno != 0 {
		return Axis{}, errno
	}
	return Axis{
		Value: int32(binary.LittleEndian.Uint32(b[0:])),
		Min:   int32(binary.LittleEndian.Uint32(b[4:])),
		Max:   int32(binary.LittleEndian.Uint32(b[8:])),
	}, nil
}

// An evdev axis reports only when it moves.
func (d *Device) Abs(code uint16) (int32, error) {
	a, err := AbsInfo(d.f, code)
	return a.Value, err
}
