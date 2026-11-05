package youtube

import (
	"github.com/ygelfand/libcountertop/pkg/media/cast/preferences"
	"github.com/ygelfand/libcountertop/pkg/say"
	setting "github.com/ygelfand/libcountertop/pkg/settings"
	"github.com/ygelfand/libcountertop/pkg/settings/schema"
	"github.com/ygelfand/libcountertop/pkg/settings/storage"
)

const (
	Group = setting.Group("Cast")

	loungeAlways = "always"
	loungeDemand = "demand"
)

func NewTable() *setting.Table[schema.Lounge] {
	var t *setting.Table[schema.Lounge]
	t = setting.NewTable("youtube", []setting.Group{Group}, []setting.Setting[schema.Lounge]{{
		Name: "lounge", Group: Group, ID: "cast_youtube_lounge", Icon: "mdi:youtube",
		Kind: setting.Choice,
		Options: []setting.Option{
			{Value: loungeAlways, Key: loungeAlways},
			{Value: loungeDemand, Key: loungeDemand},
		},
		Read: func(l *schema.Lounge) string {
			if l.OnDemand {
				return loungeDemand
			}
			return loungeAlways
		},
		Write: func(l *schema.Lounge, v string) error {
			switch v {
			case loungeAlways:
				l.OnDemand = false
			case loungeDemand:
				l.OnDemand = true
			default:
				return t.MustRow("lounge").Bad(v, "")
			}
			return nil
		},
	}}, setting.Messages{Text: say.T, Missing: say.Missing})
	return t
}

func Preferences(
	requestIDPrefix string,
	cast storage.Section[schema.Cast],
	lounge storage.Section[schema.Lounge],
) preferences.Provider {
	return preferences.Provider{
		Prefix: requestIDPrefix,
		Read: func() preferences.Settings {
			l := lounge.Read()
			return preferences.Settings{YouTube: preferences.YouTube{
				Device:   l.Device,
				Music:    l.Music,
				Video:    l.Video,
				Skip:     cast.Read().YouTube.Skip,
				OnDemand: l.OnDemand,
			}}
		},
		Save: schema.NewLoungeWriter(lounge).Screens,
	}
}
