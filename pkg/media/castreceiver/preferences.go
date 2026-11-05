package castreceiver

import (
	"github.com/ygelfand/libcountertop/pkg/media/cast/preferences"
	"github.com/ygelfand/libcountertop/pkg/settings/schema"
	"github.com/ygelfand/libcountertop/pkg/settings/storage"
)

func Preferences(requestIDPrefix string, s storage.Section[schema.Cast]) preferences.Provider {
	return preferences.Provider{
		Prefix: requestIDPrefix,
		Read: func() preferences.Settings {
			return preferences.Settings{YouTube: preferences.YouTube{Skip: s.Read().YouTube.Skip}}
		},
	}
}
