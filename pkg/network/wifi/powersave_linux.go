//go:build linux

package wifi

import (
	"fmt"
	"net"

	"github.com/mdlayher/genetlink"
	"github.com/mdlayher/netlink"
	"golang.org/x/sys/unix"
)

// The nl80211 family id is assigned when the driver registers.
const nl80211Family = "nl80211"

// The full-MAC driver reads this at association; a dozing station gets no group-addressed traffic.
func SetPowerSave(iface string, on bool) error {
	state := uint32(unix.NL80211_PS_DISABLED)
	if on {
		state = unix.NL80211_PS_ENABLED
	}
	_, err := nl80211(iface, unix.NL80211_CMD_SET_POWER_SAVE, netlink.Acknowledge,
		func(enc *netlink.AttributeEncoder) { enc.Uint32(unix.NL80211_ATTR_PS_STATE, state) })
	if err != nil {
		return fmt.Errorf("wifi: setting power save on %s: %w", iface, err)
	}
	return nil
}

// cfg80211 answers from its own record of the last setting, not from the driver.
func PowerSave(iface string) (bool, error) {
	msgs, err := nl80211(iface, unix.NL80211_CMD_GET_POWER_SAVE, 0, nil)
	if err != nil {
		return false, fmt.Errorf("wifi: reading power save on %s: %w", iface, err)
	}
	for _, m := range msgs {
		ad, err := netlink.NewAttributeDecoder(m.Data)
		if err != nil {
			return false, err
		}
		for ad.Next() {
			if ad.Type() == unix.NL80211_ATTR_PS_STATE {
				return ad.Uint32() == unix.NL80211_PS_ENABLED, nil
			}
		}
	}
	return false, fmt.Errorf("wifi: %s reported no power save state", iface)
}

func nl80211(
	iface string,
	cmd uint8,
	flags netlink.HeaderFlags,
	attrs func(*netlink.AttributeEncoder),
) ([]genetlink.Message, error) {
	link, err := net.InterfaceByName(iface)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", iface, err)
	}

	conn, err := genetlink.Dial(nil)
	if err != nil {
		return nil, fmt.Errorf("generic netlink: %w", err)
	}
	defer conn.Close()

	family, err := conn.GetFamily(nl80211Family)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", nl80211Family, err)
	}

	enc := netlink.NewAttributeEncoder()
	enc.Uint32(unix.NL80211_ATTR_IFINDEX, uint32(link.Index))
	if attrs != nil {
		attrs(enc)
	}
	data, err := enc.Encode()
	if err != nil {
		return nil, fmt.Errorf("encoding the request: %w", err)
	}

	return conn.Execute(
		genetlink.Message{
			Header: genetlink.Header{Command: cmd, Version: family.Version},
			Data:   data,
		},
		family.ID,
		netlink.Request|flags,
	)
}
