//go:build !linux

package netcmd

import (
	"context"
	"errors"
	"net/netip"
	"time"
)

var errDeviceOnly = errors.New("net: only works on the device")

func capture(context.Context, string, string, time.Duration, string) (string, error) {
	return "", errDeviceOnly
}

func arpWatch(context.Context, string, int, time.Duration) (string, error) {
	return "", errDeviceOnly
}

func arpPing(context.Context, string, netip.Addr, int) (string, error) { return "", errDeviceOnly }

func neighbors(string) (string, error) { return "", errDeviceOnly }

func stats(string) (string, error) { return "", errDeviceOnly }

func radio(string) (string, error) { return "", errDeviceOnly }
