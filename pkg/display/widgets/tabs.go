package widgets

import gogui "github.com/go-gui-org/go-gui/gui"

type Tab struct {
	Key   string
	Label string
}

type TabStripCfg struct {
	ID     string
	Tabs   []Tab
	Chosen string
	Width  float32
	Pick   func(key string)
}

func (ui *UI) TabStrip(cfg TabStripCfg) gogui.View {
	tabs := make([]gogui.View, 0, len(cfg.Tabs))
	for _, t := range cfg.Tabs {
		tab := gogui.ContainerCfg{
			ID:      cfg.ID + "-" + t.Key,
			Sizing:  gogui.FitFit,
			Padding: gogui.PaddingMedium,
			VAlign:  gogui.VAlignMiddle,
		}
		st := gogui.CurrentTheme().Cfg.TextStyleDef
		ui.Toolkit.Tab(&tab, &st, Chosen(t.Key == cfg.Chosen))
		tab.Content = []gogui.View{gogui.Label(t.Label, st)}
		key := t.Key
		tabs = append(tabs, ui.Toolkit.Pressable(gogui.Row, tab, func(e gogui.EventCtx) {
			cfg.Pick(key)
			e.Consume()
			e.Window.InvalidateLayout()
		}))
	}
	bar := gogui.CurrentTheme().ScrollbarStyle
	strip := gogui.ContainerCfg{
		ID:         cfg.ID,
		Sizing:     gogui.FillFit,
		Padding:    gogui.NewPadding(0, 0, bar.Size+2*bar.GapEdge, 0),
		Spacing:    gogui.SpacingMedium,
		Scrollable: true,
		ScrollMode: gogui.ScrollHorizontalOnly,
		Clip:       true,
		Content:    tabs,
	}
	if cfg.Width > 0 {
		strip.Width, strip.Sizing = cfg.Width, gogui.FixedFit
	}
	return gogui.Row(strip)
}
