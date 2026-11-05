package widgets

import (
	"github.com/go-gui-org/go-glyph"
	gogui "github.com/go-gui-org/go-gui/gui"

	"github.com/ygelfand/libcountertop/pkg/display/style"
)

type Toolkit struct {
	Kit       func() style.Kit
	Pressable func(func(gogui.ContainerCfg) gogui.View, gogui.ContainerCfg, func(gogui.EventCtx)) gogui.View
}

func (kit *Toolkit) controls() style.Kit { return kit.Kit() }

func (kit *Toolkit) Panel(cfg gogui.ContainerCfg) gogui.ContainerCfg {
	cfg.Color, cfg.Radius = gogui.CurrentTheme().Cfg.ColorPanel, gogui.RadiusLarge
	kit.controls().Panel(&cfg)
	return cfg
}

func (kit *Toolkit) Slider(cfg gogui.SliderCfg) gogui.View {
	kit.controls().Slider(&cfg)
	return gogui.Slider(cfg)
}

func (kit *Toolkit) SeekBar(cfg gogui.SliderCfg, marks []style.Span) gogui.View {
	cfg.Look = kit.controls().Seek(marks)
	return gogui.Slider(cfg)
}

func (kit *Toolkit) KeyButton(
	id, glyph string,
	st gogui.TextStyle,
	state style.State,
	do func(gogui.EventCtx),
) gogui.View {
	side := st.Size * 2
	return kit.Keyed(
		gogui.ContainerCfg{
			ID:     id,
			Width:  side,
			Height: side,
			Sizing: gogui.FixedFixed,
			Radius: gogui.RadiusLarge,
			Color:  gogui.CurrentTheme().Cfg.ColorPanel,
		},
		glyph,
		st,
		state,
		do,
	)
}

func (kit *Toolkit) WideKey(
	id, glyph string,
	st gogui.TextStyle,
	height float32,
	state style.State,
	do func(gogui.EventCtx),
) gogui.View {
	return kit.Keyed(
		gogui.ContainerCfg{
			ID:     id,
			Height: height,
			Sizing: gogui.FillFixed,
			Radius: gogui.RadiusMedium,
			Color:  gogui.CurrentTheme().Cfg.ColorBackground,
		},
		glyph,
		st,
		state,
		do,
	)
}

func (kit *Toolkit) Keyed(
	cfg gogui.ContainerCfg,
	glyph string,
	st gogui.TextStyle,
	state style.State,
	do func(gogui.EventCtx),
) gogui.View {
	t := gogui.CurrentTheme().Cfg
	cfg.Padding = gogui.NoPadding
	cfg.HAlign = gogui.HAlignCenter
	cfg.VAlign = gogui.VAlignMiddle
	st.Color = t.TextStyleDef.Color
	if state == style.Chosen {
		cfg.Color, st.Color = t.ColorAccent, t.ColorBackground
	}
	kit.controls().Key(&cfg, &st, state)
	cfg.Content = []gogui.View{gogui.Label(glyph, st)}
	return kit.Pressable(gogui.Row, cfg, do)
}

func (kit *Toolkit) IconKey(
	cfg gogui.ContainerCfg,
	glyph string,
	st gogui.TextStyle,
	state style.State,
	do func(gogui.EventCtx),
) gogui.View {
	kit.controls().Key(&cfg, &st, state)
	cfg.Content = []gogui.View{gogui.Label(glyph, st)}
	return kit.Pressable(gogui.Row, cfg, do)
}

func (kit *Toolkit) ProgressBar(cfg gogui.ProgressBarCfg) gogui.View {
	kit.controls().Progress(&cfg)
	return gogui.ProgressBar(cfg)
}

func (kit *Toolkit) ListRow(cfg *gogui.ContainerCfg, state style.State) {
	switch state {
	case style.Chosen:
		Highlight(cfg)
	case style.Partial:
		Outline(cfg)
	}
	kit.controls().Row(cfg, state)
}

func (kit *Toolkit) Tab(cfg *gogui.ContainerCfg, st *gogui.TextStyle, state style.State) {
	t := gogui.CurrentTheme().Cfg
	cfg.Color, cfg.Radius, st.Color = t.ColorPanel, gogui.RadiusLarge, t.TextStyleDef.Color
	if state == style.Chosen {
		cfg.Color, st.Color = t.ColorAccent, t.ColorBackground
	}
	kit.controls().Tab(cfg, st, state)
}

func Chosen(on bool) style.State {
	if on {
		return style.Chosen
	}
	return style.Rest
}

func FontSample(family string, w, h float32) gogui.View {
	t := gogui.CurrentTheme()
	big := t.Cfg.TextStyleDef
	big.Family, big.Typeface, big.Size = family, glyph.TypefaceBold, h*0.42
	small := t.Cfg.TextStyleDef
	small.Family, small.Size, small.Color = family, h*0.15, t.Cfg.ColorTextSecondary
	return gogui.Column(gogui.ContainerCfg{
		Width:   w,
		Height:  h,
		Sizing:  gogui.FixedFixed,
		Color:   t.Cfg.ColorPanel,
		Radius:  gogui.RadiusLarge,
		Padding: gogui.NoPadding,
		HAlign:  gogui.HAlignCenter,
		VAlign:  gogui.VAlignMiddle,
		Clip:    true,
		Content: []gogui.View{
			gogui.Label("Aa", big),
			gogui.Label("10:42 Il1 O0", small),
		},
	})
}

func StyleSample(name string, w, h float32) gogui.View {
	st := style.ByName(name)
	t := gogui.CurrentTheme()
	face := t.Cfg.TextStyleDef
	face.Family, face.Size = st.Family, face.Size*1.3
	mark := t.TextStyleIconLarge
	mark.Size = style.Reach() * 0.4
	key := gogui.ContainerCfg{
		Width:   style.Reach() * 0.9,
		Height:  style.Reach() * 0.9,
		Sizing:  gogui.FixedFixed,
		Color:   t.Cfg.ColorAccent,
		Radius:  gogui.RadiusLarge,
		HAlign:  gogui.HAlignCenter,
		VAlign:  gogui.VAlignMiddle,
		Padding: gogui.NoPadding,
	}
	mark.Color = t.Cfg.ColorBackground
	st.Kit.Key(&key, &mark, style.Chosen)
	key.Content = []gogui.View{gogui.Label(gogui.IconPlay, mark)}
	level := gogui.SliderCfg{
		ID:     "sample-" + name,
		Value:  60,
		Max:    100,
		Sizing: gogui.FillFit,
		Height: style.Reach() * 0.7,
	}
	st.Kit.Slider(&level)
	card := gogui.ContainerCfg{
		Width:   w,
		Height:  h,
		Sizing:  gogui.FixedFixed,
		Color:   t.Cfg.ColorPanel,
		Radius:  gogui.RadiusLarge,
		Padding: gogui.PaddingMedium,
		Spacing: gogui.SpacingSmall,
		VAlign:  gogui.VAlignMiddle,
		Content: []gogui.View{
			gogui.Row(
				gogui.ContainerCfg{
					Sizing:  gogui.FillFit,
					Padding: gogui.NoPadding,
					VAlign:  gogui.VAlignMiddle,
					Spacing: gogui.SpacingSmall,
					Content: []gogui.View{
						gogui.Label(
							"Aa",
							face,
						),
						gogui.Row(
							gogui.ContainerCfg{Sizing: gogui.FillFit, Padding: gogui.NoPadding},
						),
						st.Kit.Toggle("sample-toggle-"+name, true),
					},
				},
			),
			gogui.Row(
				gogui.ContainerCfg{
					Sizing:  gogui.FillFit,
					Padding: gogui.NoPadding,
					VAlign:  gogui.VAlignMiddle,
					Spacing: gogui.SpacingSmall,
					Content: []gogui.View{
						gogui.Slider(level), gogui.Row(key),
					},
				},
			),
		},
	}
	st.Kit.Panel(&card)
	return gogui.Column(gogui.ContainerCfg{
		Sizing:   gogui.FitFit,
		Padding:  gogui.NoPadding,
		Disabled: true,
		Content:  []gogui.View{gogui.Column(card)},
	})
}

func Outline(cfg *gogui.ContainerCfg) {
	cfg.ColorBorder = gogui.CurrentTheme().Cfg.ColorAccent.WithOpacity(0.7)
	cfg.SizeBorder = gogui.BorderPx(2)
}
func Highlight(cfg *gogui.ContainerCfg) {
	accent := gogui.CurrentTheme().Cfg.ColorAccent
	cfg.Color = accent.WithOpacity(0.18)
	cfg.ColorBorder = accent
	cfg.SizeBorder = gogui.BorderPx(2)
}
