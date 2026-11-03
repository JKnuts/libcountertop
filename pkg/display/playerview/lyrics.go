package playerview

import (
	"fmt"

	gogui "github.com/go-gui-org/go-gui/gui"

	"github.com/ygelfand/libcountertop/pkg/media/lyrics"
	"github.com/ygelfand/libcountertop/pkg/media/nowplaying"
)

type pane uint8

const (
	queuePane pane = iota
	lyricsPane
)

const (
	lyricUnits  = 3.6
	singUnits   = 4.4
	linesBefore = 2
	rest        = "♪"
)

func (p *Renderer) sheet(now nowplaying.Now) lyrics.Sheet {
	if p.options.Lyrics == nil || now.Artist == "" || now.LiveWithin != 0 {
		return lyrics.Sheet{}
	}
	return p.options.Lyrics.Find(lyrics.Query{
		Title:  now.Title,
		Artist: now.Artist,
		Album:  now.Album,
		Length: now.Length,
	})
}

func (p *Renderer) singing(now nowplaying.Now) (lyrics.Sheet, bool) {
	if p.pane != lyricsPane {
		return lyrics.Sheet{}, false
	}
	s := p.sheet(now)
	return s, !s.Empty()
}

func (p *Renderer) flip() {
	if p.pane == lyricsPane {
		p.pane = queuePane
		return
	}
	p.pane = lyricsPane
}

func (p *Renderer) lyricLines(now nowplaying.Now, s lyrics.Sheet, l look) gogui.View {
	sung, next := l.dim, l.dim
	sung.Size, next.Size = l.unit*lyricUnits, l.unit*lyricUnits
	sung.Color.A /= 2
	current := l.body
	current.Size = l.unit * singUnits
	at := s.Current(p.seeking.shown(now.Elapsed))
	from := 0
	if s.Synced {
		from = max(at-linesBefore, 0)
	}
	rows := make([]gogui.View, 0, len(s.Lines)-from)
	for i := from; i < len(s.Lines); i++ {
		st := next
		switch {
		case !s.Synced:
			st = l.body
			st.Size = l.unit * lyricUnits
		case i == at:
			st = current
		case i < at:
			st = sung
		}
		text := s.Lines[i].Text
		if text == "" {
			text = rest
		}
		rows = append(rows, gogui.Text(gogui.TextCfg{
			ID:        fmt.Sprintf("lyric-%d", i),
			Text:      text,
			TextStyle: st,
			Mode:      gogui.TextModeWrap,
		}))
	}
	return gogui.Column(gogui.ContainerCfg{
		ID:         fmt.Sprintf("lyrics-%d", from),
		Sizing:     gogui.FillFill,
		Padding:    gogui.NewPadding(0, barRoom, 0, 0),
		Spacing:    gogui.SpacingMedium,
		Scrollable: true,
		OnGesture:  p.options.UI.HoldStill,
		Content:    rows,
	})
}
