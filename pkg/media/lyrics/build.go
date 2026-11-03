package lyrics

import (
	"github.com/ygelfand/libcountertop/pkg/fetch"
	"github.com/ygelfand/libcountertop/pkg/runtime/product"
)

func Build(p product.Context) *Finder {
	return New(Options{
		Client:  fetch.Client(askTime),
		Agent:   p.Name + " " + p.Version + " (https://" + p.Site + ")",
		Changed: p.Redraw,
	})
}
