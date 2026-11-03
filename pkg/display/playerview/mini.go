package playerview

import (
	"fmt"
	"sync"

	gogui "github.com/go-gui-org/go-gui/gui"

	"github.com/ygelfand/libcountertop/pkg/display/ui"
	"github.com/ygelfand/libcountertop/pkg/media/nowplaying"
	"github.com/ygelfand/libcountertop/pkg/settings/schema"
)

const (
	tintDim   = 0.55
	keptTints = 16
	lineH     = 3
	miniUnits = 8
	miniIcon  = 4.5
)

var tints = struct {
	sync.Mutex
	by map[*ui.Image]gogui.Color
}{by: map[*ui.Image]gogui.Color{}}

func Tint(img *ui.Image) gogui.Color {
	tints.Lock()
	defer tints.Unlock()
	if c, ok := tints.by[img]; ok {
		return c
	}
	w, h := img.Size()
	small := shrink(img, w, h)
	var r, g, b float64
	for i := 0; i < len(small); i += 3 {
		r, g, b = r+small[i], g+small[i+1], b+small[i+2]
	}
	n := float64(len(small) / 3)
	c := gogui.RGB(byte(r/n*tintDim), byte(g/n*tintDim), byte(b/n*tintDim))
	if len(tints.by) >= keptTints {
		clear(tints.by)
	}
	tints.by[img] = c
	return c
}

func (p *Renderer) Mini(w *gogui.Window) gogui.View {
	now := p.options.Player.Now()
	if p.options.Shell.Open() || !now.Playing && !now.Paused {
		return nil
	}
	vw, vh := w.WindowSize()
	cover := ui.Picture(now.Art, now.ArtID)
	l := newLook(cover != nil, vw, vh)
	side := l.unit * miniUnits

	var art gogui.View
	if cover != nil {
		art = gogui.Image(gogui.ImageCfg{Src: Squared(cover), Width: side, Height: side})
	} else {
		art = gogui.Column(gogui.ContainerCfg{
			Width: side, Height: side, Sizing: gogui.FixedFixed, Padding: gogui.NoPadding,
			HAlign: gogui.HAlignCenter, VAlign: gogui.VAlignMiddle,
			Content: []gogui.View{gogui.Label(gogui.IconMusic, iconStyle(l.dim.Color))},
		})
	}
	words := []gogui.View{gogui.Label(nowplaying.Heading(now), l.body)}
	if now.Artist != "" {
		words = append(words, gogui.Label(now.Artist, l.dim))
	}
	content := []gogui.View{
		art,
		gogui.Column(gogui.ContainerCfg{
			Sizing: gogui.FillFit, Padding: gogui.NoPadding, Clip: true, Content: words,
		}),
	}
	mark := gogui.CurrentTheme().TextStyleIconLarge
	mark.Color = l.body.Color
	mark.Size = l.unit * miniIcon
	for i, c := range p.miniControls(now) {
		do := c.do
		content = append(content, p.options.UI.Presses.Pressable(gogui.Column, gogui.ContainerCfg{
			ID:      fmt.Sprintf("mini-%d", i),
			Width:   side,
			Height:  side,
			Sizing:  gogui.FixedFixed,
			Padding: gogui.NoPadding,
			HAlign:  gogui.HAlignCenter,
			VAlign:  gogui.VAlignMiddle,
			Radius:  gogui.RadiusMedium,
			Content: []gogui.View{gogui.Label(c.glyph, mark)},
		}, func(gogui.EventCtx) { do() }))
	}
	rows := []gogui.View{gogui.Row(gogui.ContainerCfg{
		Sizing:  gogui.FillFit,
		VAlign:  gogui.VAlignMiddle,
		Padding: gogui.PaddingSmall,
		Spacing: gogui.SpacingMedium,
		Content: content,
	})}
	width := float32(vw) - 2*float32(gogui.PadLarge)
	if now.Length > 0 {
		rows = append(rows, progressLine(now, width, l))
	}
	card := p.kit.Panel(gogui.ContainerCfg{
		ID:      "mini",
		Sizing:  gogui.FillFit,
		Padding: gogui.NoPadding,
		Clip:    true,
		Shadow:  &gogui.BoxShadow{Color: gogui.Black.WithOpacity(0.5), OffsetY: 4, BlurRadius: 18},
		OnClick: func(e gogui.EventCtx) { e.Consume() },
		Content: rows,
	})
	if cover != nil {
		card.Color = Tint(cover)
	}
	pin := gogui.VAlignBottom
	if p.options.Dock != nil && p.options.Dock() == schema.EdgeBottom {
		pin = gogui.VAlignTop
	}
	return gogui.Column(gogui.ContainerCfg{
		Width:   float32(vw),
		Height:  float32(vh),
		Sizing:  gogui.FixedFixed,
		Padding: gogui.PaddingLarge,
		VAlign:  pin,
		Content: []gogui.View{gogui.Column(card)},
	})
}

type miniControl struct {
	glyph string
	do    func()
}

func (p *Renderer) miniControls(now nowplaying.Now) []miniControl {
	t := p.options.Player.Transport
	out := []miniControl{{gogui.IconPlay, func() { t().Play() }}}
	if now.Playing && !now.Paused {
		out[0] = miniControl{gogui.IconPause, func() { t().Pause() }}
	}
	if now.Can.Has(nowplaying.CanNext) {
		out = append(out, miniControl{gogui.IconFastForward, func() { t().Next() }})
	}
	return append(out, miniControl{gogui.IconExpand, p.options.Player.Open})
}

func progressLine(now nowplaying.Now, width float32, l look) gogui.View {
	done := min(max(float32(now.Elapsed)/float32(now.Length), 0), 1)
	rest := l.dim.Color
	rest.A /= 3
	return gogui.Row(gogui.ContainerCfg{
		Width:   width,
		Height:  lineH,
		Sizing:  gogui.FixedFixed,
		Padding: gogui.NoPadding,
		Color:   rest,
		Content: []gogui.View{gogui.Column(gogui.ContainerCfg{
			Width:   width * done,
			Height:  lineH,
			Sizing:  gogui.FixedFixed,
			Padding: gogui.NoPadding,
			Color:   l.body.Color,
		})},
	})
}
