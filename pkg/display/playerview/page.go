package playerview

import (
	"fmt"
	"strings"

	gogui "github.com/go-gui-org/go-gui/gui"

	"github.com/ygelfand/libcountertop/pkg/display/ui"
	"github.com/ygelfand/libcountertop/pkg/display/widgets"
	"github.com/ygelfand/libcountertop/pkg/media/nowplaying"
	"github.com/ygelfand/libcountertop/pkg/say"
)

const (
	backdropW    = 48
	backdropH    = 30
	backdropDim  = 0.4
	blurPasses   = 3
	blurRadius   = 2
	wideShare    = 0.44
	coverShare   = 0.62
	uprightCover = 0.36
	uprightWidth = 0.7
	edgeShare    = 0.05
	titleUnits   = 6
	leadUnits    = 3.6
	bodyUnits    = 3
	dimUnits     = 2.6
	headUnits    = 2
	barRoom      = 16
	thumbUnits   = 6.5
	keyUnits     = 5
	keyRoom      = 2
	playUnits    = 11
	playIcon     = 5.5
	extraUnits   = 3.5
	coverRound   = 0.03
)

type look struct {
	title, lead, body, dim, head gogui.TextStyle
	dark                         bool
	unit                         float32
}

func newLook(dark bool, vw, vh int) look {
	t := gogui.CurrentTheme()
	l := look{
		title: t.TextStyleTitle,
		body:  t.Cfg.TextStyleDef,
		dim:   widgets.Secondary(),
		dark:  dark,
		unit:  float32(min(vw, vh)) / 100,
	}
	if dark {
		l.title.Color = gogui.RGB(0xff, 0xff, 0xff)
		l.body.Color = gogui.RGB(0xff, 0xff, 0xff)
		l.dim.Color = gogui.RGBA(0xff, 0xff, 0xff, 0xb0)
	}
	l.title.Align = gogui.TextAlignCenter
	l.title.Size = l.unit * titleUnits
	l.body.Size = l.unit * bodyUnits
	l.dim.Size = l.unit * dimUnits
	l.lead = l.body
	l.lead.Size = l.unit * leadUnits
	l.head = l.dim
	l.head.Size = l.unit * headUnits
	return l
}

func (p *Renderer) Body(w *gogui.Window) gogui.View {
	now := p.options.Player.Now()
	vw, vh := w.WindowSize()
	whole := ui.Rect{W: vw, H: vh}
	cover := ui.Picture(now.Art, now.ArtID)
	l := newLook(cover != nil, vw, vh)
	pad := float32(min(vw, vh)) * edgeShare
	b := p.badge(w, now, l, float32(vw)*wideShare-2*pad)
	var content gogui.View
	if vh > vw {
		var foot float32
		if b != nil {
			foot = l.unit * badgeUnits
		}
		content = p.upright(w, now, cover, l, vw, vh, foot)
	} else {
		content = p.wide(w, now, cover, l, vw, vh)
	}
	layers := []gogui.View{
		widgets.Placed(whole, coverBackdrop(cover, vw, vh)),
		widgets.Placed(whole, content),
	}
	if b != nil {
		layers = append(layers, widgets.Placed(whole, gogui.Column(gogui.ContainerCfg{
			Sizing:  gogui.FillFill,
			Padding: gogui.PadAll(pad),
			HAlign:  gogui.HAlignLeft,
			VAlign:  gogui.VAlignBottom,
			Content: []gogui.View{b},
		})))
	}
	return gogui.Column(gogui.ContainerCfg{
		Sizing:  gogui.FillFill,
		Padding: gogui.NoPadding,
		Color:   gogui.CurrentTheme().Cfg.ColorBackground,
		Content: append(layers,
			widgets.LeaveCorner(
				p.options.UI.Presses.Pressable,
				"player-close",
				widgets.CornerFloating,
				p.options.Shell.Controls(),
				p.options.Shell.Pop,
			),
		),
	})
}

func (p *Renderer) wide(
	w *gogui.Window,
	now nowplaying.Now,
	cover *ui.Image,
	l look,
	vw, vh int,
) gogui.View {
	pad := float32(min(vw, vh)) * edgeShare
	leftW := float32(vw) * wideShare
	side := min(leftW-2*pad, float32(vh)*coverShare)
	rightW := float32(vw) - leftW - pad
	leftContent := []gogui.View{p.cover(cover, side), p.words(w, now, l, leftW-2*pad)}
	rightCfg := gogui.ContainerCfg{
		Sizing:  gogui.FillFill,
		Padding: gogui.NewPadding(pad, pad, pad, 0),
		Spacing: gogui.SpacingMedium,
		Content: p.column(w, now, l, rightW-pad),
	}
	if _, sing := p.singing(now); len(now.Queue) == 0 && !sing {
		side = min(leftW-2*pad, float32(vh)-2*pad)
		leftContent = []gogui.View{p.cover(cover, side)}
		rightCfg.VAlign = gogui.VAlignMiddle
		rightCfg.HAlign = gogui.HAlignCenter
		rightCfg.Spacing = gogui.SpacingLarge
		rightCfg.Content = append(
			[]gogui.View{p.words(w, now, l, rightW-pad)},
			p.controls(w, now, l)...)
	}
	left := gogui.Column(gogui.ContainerCfg{
		Width:   leftW,
		Sizing:  gogui.FixedFill,
		Padding: gogui.PadAll(pad),
		HAlign:  gogui.HAlignCenter,
		VAlign:  gogui.VAlignMiddle,
		Spacing: gogui.SpacingLarge,
		Content: leftContent,
	})
	right := gogui.Column(rightCfg)
	return gogui.Row(gogui.ContainerCfg{
		Sizing:  gogui.FillFill,
		Padding: gogui.NoPadding,
		Content: []gogui.View{left, right},
	})
}

func (p *Renderer) upright(
	w *gogui.Window,
	now nowplaying.Now,
	cover *ui.Image,
	l look,
	vw, vh int,
	foot float32,
) gogui.View {
	pad := float32(min(vw, vh)) * edgeShare
	width := float32(vw) - 2*pad
	side := min(width*uprightWidth, float32(vh)*uprightCover)
	content := []gogui.View{p.cover(cover, side), p.words(w, now, l, width)}
	content = append(content, p.controls(w, now, l)...)
	if s, sing := p.singing(now); sing {
		content = append(content, header(say.T("player.lyrics"), l), p.lyricLines(now, s, l))
	} else if len(now.Queue) > 0 {
		content = append(content, header(upNext(len(now.Queue)), l), p.queue(w, now, l, width))
	}
	bottom := pad
	if foot > 0 {
		bottom += foot + pad/2
	}
	return gogui.Column(gogui.ContainerCfg{
		Sizing:  gogui.FillFill,
		Padding: gogui.NewPadding(pad, pad, bottom, pad),
		HAlign:  gogui.HAlignCenter,
		Spacing: gogui.SpacingMedium,
		Content: content,
	})
}

func (p *Renderer) column(w *gogui.Window, now nowplaying.Now, l look, width float32) []gogui.View {
	if s, sing := p.singing(now); sing {
		out := []gogui.View{header(say.T("player.lyrics"), l), p.lyricLines(now, s, l)}
		return append(out, p.controls(w, now, l)...)
	}
	current := nowplaying.Track{
		Title:  nowplaying.Heading(now),
		Artist: now.Artist,
		Length: now.Length,
		Art:    now.Art,
		ArtID:  now.ArtID,
	}
	out := []gogui.View{
		header(say.T("player.now"), l),
		p.track(w, "player-current", current, l, width-barRoom),
		header(upNext(len(now.Queue)), l),
		p.queue(w, now, l, width),
	}
	return append(out, p.controls(w, now, l)...)
}

func (p *Renderer) controls(w *gogui.Window, now nowplaying.Now, l look) []gogui.View {
	var out []gogui.View
	if now.Length > 0 {
		sk, _ := p.options.Player.Transport().(nowplaying.Seeker)
		out = append(out, p.progress(w, now, sk, l.dim, nil))
	}
	return append(out, p.transport(now, l))
}

func (p *Renderer) cover(cover *ui.Image, side float32) gogui.View {
	if cover != nil {
		return rounded(gogui.Image(gogui.ImageCfg{Src: Squared(cover), Width: side, Height: side}),
			side, side*coverRound)
	}
	t := gogui.CurrentTheme()
	st := t.TextStyleIconXLarge
	st.Color = t.Cfg.ColorTextSecondary
	return gogui.Column(p.kit.Panel(gogui.ContainerCfg{
		Width: side, Height: side, Sizing: gogui.FixedFixed,
		HAlign: gogui.HAlignCenter, VAlign: gogui.VAlignMiddle,
		Content: []gogui.View{gogui.Label(gogui.IconMusic, st)},
	}))
}

func (p *Renderer) words(w *gogui.Window, now nowplaying.Now, l look, width float32) gogui.View {
	lines := []gogui.View{gogui.Text(gogui.TextCfg{
		Text:      nowplaying.Heading(now),
		TextStyle: l.title,
		Mode:      gogui.TextModeWrap,
	})}
	for _, s := range []struct {
		text string
		st   gogui.TextStyle
	}{
		{now.Artist, l.lead},
		{now.Album, l.dim},
	} {
		if s.text != "" {
			lines = append(lines, gogui.Label(widgets.Ellipsize(w, s.text, s.st, width), s.st))
		}
	}
	return gogui.Column(gogui.ContainerCfg{
		Width:   width,
		Sizing:  gogui.FixedFit,
		Padding: gogui.NoPadding,
		HAlign:  gogui.HAlignCenter,
		Spacing: gogui.SpacingSmall,
		Content: lines,
	})
}

func (p *Renderer) track(
	w *gogui.Window,
	id string,
	tr nowplaying.Track,
	l look,
	width float32,
) gogui.View {
	side := l.unit * thumbUnits
	gap := gogui.CurrentTheme().Cfg.SpacingMedium
	var content []gogui.View
	used := side + 2*float32(gogui.PadSmall)
	if art := ui.Picture(tr.Art, tr.ArtID); art != nil {
		content = append(
			content,
			gogui.Image(gogui.ImageCfg{Src: Squared(art), Width: side, Height: side}),
		)
	} else {
		st := iconStyle(l.dim.Color)
		content = append(content, gogui.Column(gogui.ContainerCfg{
			Width: side, Height: side, Sizing: gogui.FixedFixed, Padding: gogui.NoPadding,
			HAlign: gogui.HAlignCenter, VAlign: gogui.VAlignMiddle,
			Content: []gogui.View{gogui.Label(gogui.IconMusic, st)},
		}))
	}
	clock := ""
	if tr.Length > 0 {
		clock = ClockText(tr.Length)
		used += w.TextWidth(clock, l.dim) + gap
	}
	room := max(width-used-gap, 0)
	lines := []gogui.View{gogui.Label(widgets.Ellipsize(w, tr.Title, l.body, room), l.body)}
	if tr.Artist != "" {
		lines = append(lines, gogui.Label(widgets.Ellipsize(w, tr.Artist, l.dim, room), l.dim))
	}
	content = append(content, gogui.Column(gogui.ContainerCfg{
		Sizing: gogui.FillFit, Padding: gogui.NoPadding, Spacing: gogui.SpacingPx(0),
		Clip: true, Content: lines,
	}))
	if clock != "" {
		content = append(content, gogui.Label(clock, l.dim))
	}
	row := gogui.ContainerCfg{
		ID:      id,
		Sizing:  gogui.FillFit,
		VAlign:  gogui.VAlignMiddle,
		Padding: gogui.PaddingSmall,
		Spacing: gogui.SpacingMedium,
		Radius:  gogui.RadiusMedium,
		Content: content,
	}
	if tr.Play == nil {
		return gogui.Row(row)
	}
	play := tr.Play
	return p.options.UI.Presses.Pressable(gogui.Row, row, func(gogui.EventCtx) { play() })
}

func (p *Renderer) queue(w *gogui.Window, now nowplaying.Now, l look, width float32) gogui.View {
	rows := make([]gogui.View, 0, len(now.Queue))
	for i, tr := range now.Queue {
		rows = append(rows, p.track(w, fmt.Sprintf("queue-%d", i), tr, l, width-barRoom))
	}
	return gogui.Column(gogui.ContainerCfg{
		ID:         "queue",
		Sizing:     gogui.FillFill,
		Padding:    gogui.NewPadding(0, barRoom, 0, 0),
		Scrollable: true,
		OnGesture:  p.options.UI.HoldStill,
		Content:    rows,
	})
}

func header(text string, l look) gogui.View {
	line := l.head.Color
	line.A /= 3
	return gogui.Row(gogui.ContainerCfg{
		Sizing:  gogui.FillFit,
		Padding: gogui.NoPadding,
		VAlign:  gogui.VAlignMiddle,
		Spacing: gogui.SpacingMedium,
		Content: []gogui.View{
			gogui.Label(strings.ToUpper(text), l.head),
			gogui.Column(gogui.ContainerCfg{
				Height:  1,
				Sizing:  gogui.FillFixed,
				Padding: gogui.NoPadding,
				Color:   line,
			}),
		},
	})
}

func upNext(n int) string { return fmt.Sprintf("%s  %d", say.T("player.next"), n) }

func coverBackdrop(cover *ui.Image, vw, vh int) gogui.View {
	if cover == nil {
		return gogui.Column(gogui.ContainerCfg{Sizing: gogui.FillFill, Padding: gogui.NoPadding})
	}
	return gogui.Image(
		gogui.ImageCfg{Src: Backdrop(cover), Width: float32(vw), Height: float32(vh)},
	)
}

func Backdrop(img *ui.Image) string {
	w, h := img.Size()
	key := fmt.Sprintf("backdrop/%p/%dx%d", img, w, h)
	if gogui.HasImage(key) {
		return "mem:" + key
	}
	small := shrink(img, w, h)
	for range blurPasses {
		small = blur(small, backdropW, backdropH, 1, 0)
		small = blur(small, backdropW, backdropH, 0, 1)
	}
	pix := make([]byte, 0, backdropW*backdropH*4)
	for i := 0; i < len(small); i += 3 {
		pix = append(
			pix,
			byte(
				small[i]*backdropDim,
			),
			byte(small[i+1]*backdropDim),
			byte(small[i+2]*backdropDim),
			255,
		)
	}
	return gogui.UseImage(key, backdropW, backdropH, pix)
}

func shrink(img *ui.Image, w, h int) []float64 {
	out := make([]float64, backdropW*backdropH*3)
	for y := range backdropH {
		y0, y1 := y*h/backdropH, max((y+1)*h/backdropH, y*h/backdropH+1)
		for x := range backdropW {
			x0, x1 := x*w/backdropW, max((x+1)*w/backdropW, x*w/backdropW+1)
			var r, g, b, n float64
			for sy := y0; sy < min(y1, h); sy++ {
				for sx := x0; sx < min(x1, w); sx++ {
					c := img.At(sx, sy)
					r, g, b, n = r+float64(c.R), g+float64(c.G), b+float64(c.B), n+1
				}
			}
			if n > 0 {
				at := (y*backdropW + x) * 3
				out[at], out[at+1], out[at+2] = r/n, g/n, b/n
			}
		}
	}
	return out
}

func blur(in []float64, w, h, dx, dy int) []float64 {
	out := make([]float64, len(in))
	for y := range h {
		for x := range w {
			var r, g, b float64
			for k := -blurRadius; k <= blurRadius; k++ {
				sx := min(max(x+k*dx, 0), w-1)
				sy := min(max(y+k*dy, 0), h-1)
				at := (sy*w + sx) * 3
				r, g, b = r+in[at], g+in[at+1], b+in[at+2]
			}
			n := float64(2*blurRadius + 1)
			at := (y*w + x) * 3
			out[at], out[at+1], out[at+2] = r/n, g/n, b/n
		}
	}
	return out
}
