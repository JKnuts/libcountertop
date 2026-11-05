package yttv

import (
	"github.com/ygelfand/libcountertop/pkg/say"
	setting "github.com/ygelfand/libcountertop/pkg/settings"
	"github.com/ygelfand/libcountertop/pkg/settings/schema"
)

const entityMost = 128

type (
	Setting = setting.Setting[schema.TV]
	Table   = setting.Table[schema.TV]
)

const Group setting.Group = "TV"

func NewTable() *Table {
	return setting.NewTable("tv", []setting.Group{Group}, []Setting{
		setting.Words(
			Setting{Name: "default", Group: Group, Icon: "mdi:television", NoEntity: true},
			entityMost,
			func(c *schema.TV) *string { return &c.Default },
		),
	}, setting.Messages{Text: say.T, Missing: say.Missing})
}

func (g *Guide) Name() string { return "tv" }
