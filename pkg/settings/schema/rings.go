package schema

import (
	"slices"
	"strings"

	"github.com/ygelfand/libcountertop/pkg/say"
)

type RingsLook string

const (
	RingsWheel  RingsLook = "wheel"
	RingsIcicle RingsLook = "icicle"
	RingsBurst  RingsLook = "burst"
)

func (l RingsLook) Label() string { return say.T("rings.look." + string(l)) }

func RingsLooks() []RingsLook { return []RingsLook{RingsWheel, RingsIcicle, RingsBurst} }

type RingsGroup string

const (
	ByRoom   RingsGroup = "room"
	BySystem RingsGroup = "system"
)

func (g RingsGroup) Label() string { return say.T("rings.group." + string(g)) }

func RingsGroups() []RingsGroup { return []RingsGroup{ByRoom, BySystem} }

type RingsSide string

const (
	SideRight RingsSide = "right"
	SideLeft  RingsSide = "left"
)

func (s RingsSide) Label() string { return say.T("rings.side." + string(s)) }

func RingsSides() []RingsSide { return []RingsSide{SideLeft, SideRight} }

type Rings struct {
	Look   RingsLook  `json:"look"`
	Group  RingsGroup `json:"group"`
	Depth  int        `json:"depth"`
	Side   RingsSide  `json:"side"`
	Floors []string   `json:"hidden_floors"`
	Areas  []string   `json:"hidden_areas"`
	Kinds  []string   `json:"hidden_kinds"`
	Labels []string   `json:"labels"`
}

func (r Rings) Clone() Rings {
	r.Floors = slices.Clone(r.Floors)
	r.Areas = slices.Clone(r.Areas)
	r.Kinds = slices.Clone(r.Kinds)
	r.Labels = slices.Clone(r.Labels)
	return r
}

const (
	RingsDeepest = 4
	RingsDepth   = 3
)

func (r Rings) Deep() int {
	if r.Depth < 1 || r.Depth > RingsDeepest {
		return RingsDepth
	}
	return r.Depth
}

func (r Rings) Placed() RingsSide {
	if r.Side == SideLeft {
		return SideLeft
	}
	return SideRight
}

func (r Rings) Grouping() RingsGroup {
	if slices.Contains(RingsGroups(), r.Group) {
		return r.Group
	}
	return ByRoom
}

func (r Rings) Shape() RingsLook {
	if slices.Contains(RingsLooks(), r.Look) {
		return r.Look
	}
	return RingsWheel
}

func ParseList(v string) []string {
	out := []string{}
	for part := range strings.SplitSeq(v, ",") {
		if part = strings.TrimSpace(part); part != "" && !slices.Contains(out, part) {
			out = append(out, part)
		}
	}
	return out
}

func Toggled(list []string, item string, in bool) []string {
	out := slices.DeleteFunc(slices.Clone(list), func(s string) bool { return s == item })
	if in {
		out = append(out, item)
	}
	return out
}
