package schema

import "github.com/ygelfand/libcountertop/pkg/settings/storage"

type TV struct {
	Default   string   `json:"default"`
	Favorites []string `json:"favorites,omitempty"`
}

func DefaultTV() TV { return TV{} }

type TVWriter struct{ s storage.Section[TV] }

func NewTVWriter(s storage.Section[TV]) TVWriter { return TVWriter{s: s} }

func (w TVWriter) Default(v string) error {
	return w.s.Update(func(c *TV) { c.Default = v })
}
