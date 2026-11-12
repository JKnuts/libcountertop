package schema

import "strconv"

type Bluetooth struct {
	Proxy   bool      `json:"proxy"`
	Speaker bool      `json:"speaker"`
	Scan    ScanShare `json:"scan"`
}

type ScanShare string

const (
	ScanShare9 ScanShare = "9"
	ScanShare5 ScanShare = "5"
	ScanShare2 ScanShare = "2"
	ScanShare1 ScanShare = "1"
)

func ScanShares() []ScanShare { return []ScanShare{ScanShare9, ScanShare5, ScanShare2, ScanShare1} }

func (s ScanShare) Label() string { return string(s.percentOrMost()) + "%" }

func (s ScanShare) Percent() int {
	n, _ := strconv.Atoi(string(s.percentOrMost()))
	return n
}

func (s ScanShare) percentOrMost() ScanShare {
	for _, known := range ScanShares() {
		if s == known {
			return s
		}
	}
	return DefaultScanShare
}

const (
	DefaultBluetoothProxy = false
	DefaultScanShare      = ScanShare9
)

func DefaultBluetooth() Bluetooth {
	return Bluetooth{Proxy: DefaultBluetoothProxy, Scan: DefaultScanShare}
}
