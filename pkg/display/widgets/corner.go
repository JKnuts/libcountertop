package widgets

import (
	gogui "github.com/go-gui-org/go-gui/gui"

	"github.com/ygelfand/libcountertop/pkg/display/icons"
	"github.com/ygelfand/libcountertop/pkg/display/style"
	"github.com/ygelfand/libcountertop/pkg/display/ui"
)

type CornerLook int

const (
	CornerFloating CornerLook = iota
	CornerSolid
)

type Pressable func(func(gogui.ContainerCfg) gogui.View, gogui.ContainerCfg, func(gogui.EventCtx)) gogui.View

func LeaveCorner(
	press Pressable,
	id string,
	look CornerLook,
	level float32,
	tap func(),
) gogui.View {
	level = look.level(level)
	return placeCorner(int(gogui.PadLarge), level, LeaveButton(press, id, look, level, tap))
}

func SettingsCorner(
	press Pressable,
	id string,
	vw int,
	look CornerLook,
	level float32,
	tap func(),
) gogui.View {
	level = look.level(level)
	x := vw - int(gogui.PadLarge) - CornerSide()
	return placeCorner(x, level, SettingsButton(press, id, look, level, tap))
}

func (l CornerLook) level(level float32) float32 {
	if l == CornerSolid {
		return 1
	}
	return level
}

func LeaveButton(
	press Pressable,
	id string,
	look CornerLook,
	level float32,
	tap func(),
) gogui.View {
	return cornerButton(press, id, gogui.IconArrowLeft, cornerStyle(), look, level, tap)
}

func SettingsButton(
	press Pressable,
	id string,
	look CornerLook,
	level float32,
	tap func(),
) gogui.View {
	st := cornerStyle()
	st.Family = icons.Family
	return cornerButton(press, id, icons.Glyph(icons.Named("mdi:cog")), st, look, level, tap)
}

func CornerSide() int { return int(cornerStyle().Size * 2) }

func cornerStyle() gogui.TextStyle {
	st := gogui.CurrentTheme().TextStyleIconMedium
	st.Size = style.Reach() * 0.75
	return st
}

func placeCorner(x int, level float32, button gogui.View) gogui.View {
	if level <= 0 {
		return Placed(ui.Rect{}, gogui.Column(gogui.ContainerCfg{Padding: gogui.NoPadding}))
	}
	side, pad := CornerSide(), int(gogui.PadLarge)
	return Placed(ui.Rect{X: x, Y: pad, W: side, H: side}, button)
}

func cornerButton(
	press Pressable,
	id, glyph string,
	st gogui.TextStyle,
	look CornerLook,
	level float32,
	tap func(),
) gogui.View {
	t := gogui.CurrentTheme()
	fill := t.Cfg.ColorPanel
	st.Color = t.TextStyleDef.Color
	if look == CornerFloating {
		fade := min(max(level, 0), 1)
		st.Color = gogui.RGB(0xff, 0xff, 0xff).WithOpacity(fade)
		fill = gogui.RGBA(0, 0, 0, uint8(0x99*fade))
	}
	side := st.Size * 2
	return press(gogui.Row, gogui.ContainerCfg{
		ID:      id,
		Width:   side,
		Height:  side,
		Sizing:  gogui.FixedFixed,
		Color:   fill,
		Radius:  gogui.RadiusLarge,
		Padding: gogui.NoPadding,
		HAlign:  gogui.HAlignCenter,
		VAlign:  gogui.VAlignMiddle,
		Content: []gogui.View{gogui.Label(glyph, st)},
	}, func(e gogui.EventCtx) {
		tap()
		e.Consume()
	})
}
