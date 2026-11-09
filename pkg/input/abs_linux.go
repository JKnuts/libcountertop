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

func readAbsInfo(f *os.File, code uint16) ([absInfoBytes]byte, error) {
	var b [absInfoBytes]byte
	req := uintptr(2<<30 | absInfoBytes<<16 | 'E'<<8 | (0x40 + uint32(code)))
	_, _, errno := unix.Syscall(
		unix.SYS_IOCTL,
		f.Fd(),
		req,
		uintptr(unsafe.Pointer(&b[0])),
	)
	runtime.KeepAlive(f)
	if errno != 0 {
		return b, errno
	}
	return b, nil
}

// An evdev axis reports only when it moves.
func (d *Device) Abs(code uint16) (int32, error) {
	b, err := readAbsInfo(d.f, code)
	if err != nil {
		return 0, err
	}
	return int32(binary.NativeEndian.Uint32(b[:4])), nil
}

// AbsRange reads the inclusive bounds of an axis on an open evdev file.
func AbsRange(f *os.File, code uint16) (min, max int32, err error) {
	b, err := readAbsInfo(f, code)
	if err != nil {
		return 0, 0, err
	}
	return int32(binary.NativeEndian.Uint32(b[4:8])), int32(binary.NativeEndian.Uint32(b[8:12])), nil
}
