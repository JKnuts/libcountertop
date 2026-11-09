package entitycard

import (
	"strconv"
	"strings"
)

type Entity struct {
	ID         string
	Name       string
	State      string
	Available  bool
	Attributes map[string]any
}

func (e Entity) Domain() string {
	domain, _, _ := strings.Cut(e.ID, ".")
	return domain
}

func (e Entity) Text(key string) string {
	switch v := e.Attributes[key].(type) {
	case string:
		return v
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	}
	return ""
}

func (e Entity) Number(key string) (float64, bool) {
	v, ok := e.Attributes[key].(float64)
	return v, ok
}

func (e Entity) Bool(key string) (bool, bool) {
	v, ok := e.Attributes[key].(bool)
	return v, ok
}

func (e Entity) Numbers(key string) []float64 {
	list, _ := e.Attributes[key].([]any)
	out := make([]float64, 0, len(list))
	for _, v := range list {
		f, ok := v.(float64)
		if !ok {
			return nil
		}
		out = append(out, f)
	}
	return out
}

func (e Entity) Texts(key string) []string {
	list, _ := e.Attributes[key].([]any)
	out := make([]string, 0, len(list))
	for _, v := range list {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func (e Entity) number(key string, fallback float64) float64 {
	if v, ok := e.Number(key); ok {
		return v
	}
	return fallback
}

func (e Entity) features() int {
	f, _ := e.Number("supported_features")
	return int(f)
}
