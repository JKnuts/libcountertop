package screen

import (
	"errors"
	"path/filepath"
	"testing"

	esphome "github.com/ygelfand/go-esphome-device"

	"github.com/ygelfand/libcountertop/pkg/display/theme"
	"github.com/ygelfand/libcountertop/pkg/settings/schema"
	"github.com/ygelfand/libcountertop/pkg/settings/storage"
)

type rig struct {
	store *storage.Store[schema.Screen]
	lit   []int
	fail  bool
}

func panel(t *testing.T) (*Screen, *rig) {
	t.Helper()
	st, err := storage.Load(
		filepath.Join(t.TempDir(), "state.json"),
		schema.DefaultScreen(),
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	r := &rig{store: st}
	s := New(Options{
		Settings: storage.Bind(
			func() *storage.Store[schema.Screen] { return st },
			func(c *schema.Screen) *schema.Screen { return c },
		),
		Backlight: func(level int) error {
			if r.fail {
				return errors.New("no backlight")
			}
			r.lit = append(r.lit, level)
			return nil
		},
		UISize: func() string { return schema.UISizeLarge },
	})
	return s, r
}

func (r *rig) saved() schema.Screen { return r.store.Get() }

func TestRestoreTakesTheSavedTheme(t *testing.T) {
	s, _ := panel(t)

	cfg := schema.DefaultScreen()
	cfg.Theme = "Paper"
	s.Restore(cfg)

	if got := s.Theme().Name; got != "Paper" {
		t.Errorf("theme = %q, want Paper", got)
	}
}

func TestUnknownThemeIsKept(t *testing.T) {
	s, _ := panel(t)

	cfg := schema.DefaultScreen()
	cfg.Theme = "Midnight"
	s.Restore(cfg)

	cfg.Theme = "Nonesuch"
	s.Restore(cfg)

	if got := s.Theme().Name; got != "Midnight" {
		t.Errorf("theme = %q, want it held at Midnight", got)
	}
}

func TestThemedFiresOnChange(t *testing.T) {
	s, _ := panel(t)
	s.Use("Midnight")

	got := make(chan theme.Theme, 4)
	cancel := s.Themed.Listen(func(t theme.Theme) { got <- t })
	defer cancel()

	s.Use("Ocean")

	select {
	case th := <-got:
		if th.Name != "Ocean" {
			t.Errorf("Themed carried %q, want Ocean", th.Name)
		}
	default:
		t.Fatal("Themed did not fire")
	}
}

func TestThemedIsQuietWhenNothingChanged(t *testing.T) {
	s, _ := panel(t)
	s.Use("Midnight")

	got := make(chan theme.Theme, 4)
	cancel := s.Themed.Listen(func(t theme.Theme) { got <- t })
	defer cancel()

	s.Use("Midnight")

	select {
	case th := <-got:
		t.Errorf("Themed fired for an unchanged theme: %q", th.Name)
	default:
	}
}

func TestEntities(t *testing.T) {
	s, _ := panel(t)

	if got := len(s.Entities()); got != 20 {
		t.Errorf(
			"%d entities, want backlight, mode, theme, style, size, the drawer edge, the dock hide delay, the alert style, the timer style, keyboard, language, marks, show touches, the six custom theme colors, and the level",
			got,
		)
	}
	for i, e := range s.Entities() {
		if e == nil {
			t.Errorf("entity %d is nil", i)
		}
	}
}

func TestSettingTheSliderOnAutomaticOnlyRemembersIt(t *testing.T) {
	s, r := panel(t)

	s.SetMode(schema.ModeAuto)
	s.Lit(64)

	s.SetBacklight(20)

	if got := r.saved().Backlight; got != 20 {
		t.Errorf("the slider stored %d, want 20", got)
	}
	if got := s.knob("backlight").(*esphome.Number).Get(); got != 20 {
		t.Errorf("the entity reads %v, want 20", got)
	}
	if got := s.level.Get(); got != 64 {
		t.Errorf("the panel was driven to %v by the slider, want it left at 64", got)
	}
	if len(r.lit) != 0 {
		t.Errorf("the backlight was driven to %v on automatic", r.lit)
	}
}

func TestSettingTheSliderOnManualDrivesThePanel(t *testing.T) {
	s, r := panel(t)

	s.SetMode(schema.ModeManual)

	s.SetBacklight(20)

	if got := r.saved().Backlight; got != 20 {
		t.Errorf("the slider stored %d, want 20", got)
	}
	if got := s.level.Get(); got != 20 {
		t.Errorf("the panel reads %v, want the 20 that was set", got)
	}
}

func TestAFailedBacklightKeepsTheSavedLevel(t *testing.T) {
	s, r := panel(t)

	s.SetMode(schema.ModeManual)
	s.SetBacklight(30)
	r.fail = true
	s.SetBacklight(80)

	if got := r.saved().Backlight; got != 30 {
		t.Errorf("a failed backlight stored %d, want 30", got)
	}
}

func TestEveryEdgeIsOffered(t *testing.T) {
	s, _ := panel(t)

	for _, e := range schema.Edges() {
		var found bool
		for _, o := range s.knob("drawer").(*esphome.Select).Options {
			if o == e.Label() {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("the %q edge is not offered", e.Label())
		}
	}
}

func TestTheDrawerEdgeIsRestored(t *testing.T) {
	s, _ := panel(t)

	cfg := schema.DefaultScreen()
	cfg.Drawer = schema.EdgeTop
	s.Restore(cfg)

	if got := s.knob("drawer").(*esphome.Select).Get(); got != schema.EdgeTop.Label() {
		t.Errorf("the drawer select reads %q, want %q", got, schema.EdgeTop.Label())
	}
}

func TestEveryThemeIsOffered(t *testing.T) {
	s, _ := panel(t)

	offered := map[string]bool{}
	for _, name := range s.knob("theme").(*esphome.Select).Options {
		offered[name] = true
	}

	for _, th := range theme.All {
		if !offered[th.Name] {
			t.Errorf("%s is not offered as a choice", th.Name)
		}
	}
}

func TestSettingTheDrawerEdgeTellsBothSides(t *testing.T) {
	s, r := panel(t)

	for _, want := range schema.Edges() {
		s.SetDrawer(want)

		if got := r.saved().Drawer; got != want {
			t.Errorf("the file says %q, want %q", got, want)
		}
		if got := s.knob("drawer").(*esphome.Select).Get(); got != want.Label() {
			t.Errorf("the entity says %q, want %q", got, want.Label())
		}
	}
}

func TestTheDrawerCommandFromHomeAssistantIsSaved(t *testing.T) {
	s, r := panel(t)

	for _, want := range schema.Edges() {
		s.knob("drawer").(*esphome.Select).OnCommand(want.Label())

		if got := r.saved().Drawer; got != want {
			t.Errorf("%q from Home Assistant saved as %q", want, got)
		}
	}
}

func TestSettingTheModeTellsBothSides(t *testing.T) {
	s, r := panel(t)

	for _, want := range []schema.ScreenMode{schema.ModeManual, schema.ModeAuto} {
		s.SetMode(want)

		if got := r.saved().Mode; got != want {
			t.Errorf("the file says %q, want %q", got, want)
		}
		if got := s.knob("auto").(*esphome.Switch).Get(); got != (want == schema.ModeAuto) {
			t.Errorf("the entity says %v for %q", got, want)
		}
	}
}

func TestUsingAThemeTellsBothSides(t *testing.T) {
	s, _ := panel(t)

	for _, want := range theme.All {
		s.Use(want.Name)

		if got := s.knob("theme").(*esphome.Select).Get(); got != want.Name {
			t.Errorf("the entity says %q, want %q", got, want.Name)
		}
		if got := s.Theme().Name; got != want.Name {
			t.Errorf("the screen is drawing %q, want %q", got, want.Name)
		}
	}
}

func TestAnUnknownThemeKeepsWhatIsShowing(t *testing.T) {
	s, _ := panel(t)

	s.Use(theme.DefaultName)
	s.Use("Chartreuse")

	if got := s.Theme().Name; got != theme.DefaultName {
		t.Errorf("an unknown theme left the screen on %q", got)
	}
}

func TestTheBacklightBiasTellsBothSidesOnAutomatic(t *testing.T) {
	s, r := panel(t)

	s.SetMode(schema.ModeAuto)

	for _, want := range []int{0, 37, 76, 100} {
		s.SetBacklight(want)

		if got := r.saved().Backlight; got != want {
			t.Errorf("the file says %d, want %d", got, want)
		}
		if got := s.knob("backlight").(*esphome.Number).Get(); got != float32(want) {
			t.Errorf("the entity says %v, want %d", got, want)
		}
	}
}

func TestTheBacklightIsHeldToItsRange(t *testing.T) {
	s, r := panel(t)

	s.SetMode(schema.ModeAuto)

	s.SetBacklight(150)
	if got := r.saved().Backlight; got != 100 {
		t.Errorf("150 saved as %d, want 100", got)
	}

	s.SetBacklight(-20)
	if got := r.saved().Backlight; got != 0 {
		t.Errorf("-20 saved as %d, want 0", got)
	}
}

func TestSetSavesTheRowAndDrivesAManualPanel(t *testing.T) {
	s, r := panel(t)

	var restyled []schema.Screen
	cancel := s.Restyled.Listen(func(c schema.Screen) { restyled = append(restyled, c) })
	defer cancel()

	if err := s.Set("auto", "off"); err != nil {
		t.Fatal(err)
	}
	if err := s.Set("backlight", "42"); err != nil {
		t.Fatal(err)
	}
	if got := r.saved(); got.Mode != schema.ModeManual || got.Backlight != 42 {
		t.Errorf("saved %+v", got)
	}
	if len(r.lit) == 0 || r.lit[len(r.lit)-1] != 42 {
		t.Errorf("the backlight was driven to %v, want 42 last", r.lit)
	}
	if len(restyled) != 2 {
		t.Errorf("restyled %d times, want 2", len(restyled))
	}
	if err := s.Set("drawer", "sideways"); err == nil {
		t.Error("an unknown edge was accepted")
	}
}

func TestEverySettableEntityTakesACommand(t *testing.T) {
	s, _ := panel(t)

	for _, e := range s.Entities() {
		switch v := e.(type) {
		case *esphome.Select:
			if v.OnCommand == nil {
				t.Errorf("%s takes no command", v.ObjectID)
			}
		case *esphome.Switch:
			if v.OnCommand == nil {
				t.Errorf("%s takes no command", v.ObjectID)
			}
		case *esphome.Number:
			if v.OnCommand == nil {
				t.Errorf("%s takes no command", v.ObjectID)
			}
		case *esphome.Text:
			if v.OnCommand == nil {
				t.Errorf("%s takes no command", v.ObjectID)
			}
		case *esphome.Sensor:
		default:
			t.Errorf("%s is a %T, which this does not know how to check", e, e)
		}
	}
}
