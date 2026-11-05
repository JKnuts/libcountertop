package previews

import (
	gogui "github.com/go-gui-org/go-gui/gui"

	"github.com/ygelfand/libcountertop/pkg/display/menu"
	"github.com/ygelfand/libcountertop/pkg/display/widgets"
)

type font string

func (Kit) Font(family string) menu.Preview { return font(family) }

func (f font) View(_ *gogui.Window, width, height float32) gogui.View {
	return widgets.FontSample(string(f), width, height)
}
