package settings

import (
	"github.com/ygelfand/libcountertop/pkg/display/clockview"
	"github.com/ygelfand/libcountertop/pkg/display/menu"
	"github.com/ygelfand/libcountertop/pkg/display/previews"
	"github.com/ygelfand/libcountertop/pkg/display/shell"
	"github.com/ygelfand/libcountertop/pkg/say"
	config "github.com/ygelfand/libcountertop/pkg/settings/schema"
)

type ClockState struct {
	Screen config.Screen
	Clock  config.Clock
}
type ClockController interface {
	Placement
	SetFace(config.Face)
	SetInk(config.Ink)
	SetBacking(config.Backing)
	SetDate(bool)
}
type ClockDependencies struct {
	Preferences Preferences
	Shell       *shell.Shell
	Controller  ClockController
	Previews    previews.Kit
}
type ClockPages struct{ options ClockDependencies }

func NewClockPages(options ClockDependencies) *ClockPages { return &ClockPages{options: options} }

func (p *ClockPages) open(
	view shell.View,
) func(int) {
	return func(int) { p.options.Shell.Push(view) }
}
func (p *ClockPages) Page() *shell.Page {
	return &shell.Page{
		Title: say.T("clock.title"),
		Build: func() ([]menu.Row, []func(int)) {
			cfg := p.options.Preferences.ClockState()

			face, pick := p.faceRow(cfg.Clock)
			rows := []menu.Row{face}
			taps := []func(int){pick}
			placed, placing := Placing{
				Positions: config.Positions(),
				Aligns:    config.Aligns(),
				Read: func() config.Placement {
					return p.options.Preferences.ClockState().Clock.Placement
				},
				Set:     p.options.Controller,
				Preview: p.placed,
			}.Rows(p.options.Shell)
			color, paint := p.colorRow(cfg.Clock)
			rows = append(append(rows, placed...),
				color,
				menu.Row{
					Label: say.T("clock.backing"),
					Kind:  menu.Chevron,
					Value: cfg.Clock.Backing.Label(),
				},
				menu.Row{Label: say.T("clock.date"), Kind: menu.Toggle, On: cfg.Clock.Date},
			)
			taps = append(append(taps, placing...),
				paint,
				p.open(Choose(
					say.T("clock.backing"),
					config.Backings(),
					func() config.Backing { return p.options.Preferences.ClockState().Clock.Backing },
					p.options.Controller.SetBacking,
					nil,
				)),
				func(int) { p.options.Controller.SetDate(!cfg.Clock.Date) },
			)
			return rows, taps
		},
	}
}

func (p *ClockPages) Compact() *shell.Page {
	return &shell.Page{
		Title: say.T("clock.title"),
		Build: func() ([]menu.Row, []func(int)) {
			cfg := p.options.Preferences.ClockState()
			face, pick := p.faceRow(cfg.Clock)
			color, paint := p.colorRow(cfg.Clock)
			return []menu.Row{face, color}, []func(int){pick, paint}
		},
	}
}

func (p *ClockPages) faceRow(c config.Clock) (menu.Row, func(int)) {
	return menu.Row{
		Label: say.T("clock.face"),
		Kind:  menu.Chevron,
		Value: c.Face.Label(),
	}, p.open(Tiles(say.T("clock.face"),
		clockview.Names(),
		func() config.Face { return p.options.Preferences.ClockState().Clock.Face },
		p.options.Controller.SetFace,
		p.options.Previews.ClockFace))
}

func (p *ClockPages) colorRow(c config.Clock) (menu.Row, func(int)) {
	return menu.Row{
		Label: say.T("clock.color"),
		Kind:  menu.Chevron,
		Value: c.Ink.Label(),
	}, p.open(p.Colors())
}

func (p *ClockPages) Colors() *shell.Page {
	return &shell.Page{
		Title: say.T("clock.color.title"),
		Build: func() ([]menu.Row, []func(int)) {
			now := p.options.Preferences.ClockState().Clock.Ink

			rows := make([]menu.Row, 0, len(config.Inks()))
			acts := make([]func(int), 0, len(config.Inks()))

			for _, ink := range config.Inks() {
				rows = append(rows, menu.Row{
					Label:   ink.Label(),
					Chosen:  ink == now,
					Preview: p.inking(ink),
				})
				acts = append(acts, p.useInk(ink))
			}
			return rows, acts
		},
	}
}

func (p *ClockPages) useInk(ink config.Ink) func(int) {
	return func(int) { p.options.Controller.SetInk(ink) }
}

func (p *ClockPages) placed(at config.Placement) menu.Preview {
	return p.options.Previews.ClockPlaced(p.options.Preferences.ClockState().Clock.Face, at)
}

func (p *ClockPages) inking(ink config.Ink) menu.Preview {
	return p.options.Previews.ClockInk(p.options.Preferences.ClockState().Clock.Face, ink)
}
