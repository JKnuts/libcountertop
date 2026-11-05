package yttv

import (
	"github.com/ygelfand/libcountertop/pkg/runtime/product"
	"github.com/ygelfand/libcountertop/pkg/settings/schema"
	"github.com/ygelfand/libcountertop/pkg/settings/storage"
)

func Build(p product.Context, settings storage.Section[schema.TV], self func() string) *Guide {
	g := New(Options{
		HA:       p.Controller,
		States:   p.States,
		Settings: settings,
		Self:     self,
		Redraw:   p.Redraw,
		DeviceID: p.Screen,
	})
	p.Settings.Add(g.Settings())
	return g
}
