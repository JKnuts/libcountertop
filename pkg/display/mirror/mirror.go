package mirror

import (
	"regexp"
	"time"

	gogui "github.com/go-gui-org/go-gui/gui"

	"github.com/ygelfand/libcountertop/pkg/display/apps"
	"github.com/ygelfand/libcountertop/pkg/display/navigation"
	"github.com/ygelfand/libcountertop/pkg/display/shell"
	"github.com/ygelfand/libcountertop/pkg/display/ui"
	"github.com/ygelfand/libcountertop/pkg/display/widgets"
	"github.com/ygelfand/libcountertop/pkg/say"
	"github.com/ygelfand/libcountertop/pkg/settings/schema"
	"github.com/ygelfand/libcountertop/pkg/voice"
)

type View struct{ feed shell.Feed }

func (*View) Covers() bool { return true }

func (*View) Timeout() time.Duration { return shell.AppTimeout }

type Camera interface {
	Watch(v shell.View)
	WantMirror(box ui.Rect) shell.Feed
}

type Options struct {
	Shell  *shell.Shell
	Camera Camera
}

type Mirror struct {
	o    Options
	view *View
}

func New(o Options) *Mirror {
	m := &Mirror{o: o, view: &View{}}
	o.Camera.Watch(m.view)
	return m
}

func (m *Mirror) App() apps.App {
	return apps.App{
		ID:   schema.AppMirror,
		Name: func() string { return say.T("mirror.app") },
		Icon: "mdi:mirror",
		Open: m.open,
	}
}

var spoken = regexp.MustCompile(`^mirror$`)

func (m *Mirror) Command() voice.Hook {
	return voice.Hook{
		Name:     "mirror",
		Keywords: []string{"mirror"},
		Match:    spoken,
		Heard:    func(voice.Heard) { m.open() },
	}
}

func (m *Mirror) Screen(v shell.View) *navigation.Screen {
	if v != m.view {
		return nil
	}
	return &navigation.Screen{View: v, Clear: true, Fixed: true, Build: m.build}
}

func (m *Mirror) open() {
	m.view.feed = shell.Feed{Note: say.T("camera.starting")}
	m.o.Shell.Push(m.view)
}

func (m *Mirror) build(*gogui.Window) gogui.View {
	var content []gogui.View
	if !m.view.feed.Live {
		content = widgets.FeedNote(m.view.feed)
	}
	return gogui.Column(gogui.ContainerCfg{
		ID:      "mirror",
		Sizing:  gogui.FillFill,
		Padding: gogui.NoPadding,
		HAlign:  gogui.HAlignCenter,
		VAlign:  gogui.VAlignMiddle,
		Content: content,
		AmendLayout: func(e gogui.EventCtx) {
			s := e.Layout.Shape
			m.view.feed = m.o.Camera.WantMirror(
				ui.Rect{X: int(s.X), Y: int(s.Y), W: int(s.Width), H: int(s.Height)},
			)
		},
		OnClick: func(e gogui.EventCtx) {
			m.o.Shell.Remove(m.view)
			e.Consume()
		},
	})
}
