//go:build !linux

package wifi

import "errors"

var errDeviceOnly = errors.New("wifi: only works on the device")

func SetPowerSave(string, bool) error { return errDeviceOnly }

func PowerSave(string) (bool, error) { return false, errDeviceOnly }
