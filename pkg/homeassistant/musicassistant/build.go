package musicassistant

import (
	"github.com/ygelfand/libcountertop/pkg/fetch"
	"github.com/ygelfand/libcountertop/pkg/runtime/product"
)

func BuildMusic(p product.Context, player func() string) *Music {
	return NewMusic(MusicOptions{
		HA:      p.Controller,
		States:  p.States,
		Player:  player,
		Changed: p.Redraw,
	})
}

func Build(p product.Context, player func() string) *Queue {
	return New(Options{
		HA:      p.Controller,
		Player:  player,
		Client:  fetch.Client(askTime),
		Changed: p.Redraw,
	})
}
