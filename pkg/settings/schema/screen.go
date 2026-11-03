package schema

import (
	"github.com/ygelfand/libcountertop/pkg/display/theme"
	"github.com/ygelfand/libcountertop/pkg/say"
)

type Screen struct {
	Backlight int        `json:"backlight"`
	Mode      ScreenMode `json:"mode"`
	Theme     string     `json:"theme"`
	Style     string     `json:"style"`
	Size      string     `json:"size"`
	Hours     HourFormat `json:"hours"`

	Keyboard KeyboardSize `json:"keyboard"`

	Drawer Edge `json:"drawer"`

	DockHide Delay `json:"dock_hide"`

	Volume Edge `json:"volume"`

	Alert AlertStyle `json:"alert"`

	Timer TimerStyle `json:"timer"`

	Marks bool `json:"marks"`

	Touches bool `json:"touches,omitempty"`

	Language string `json:"language"`

	Custom theme.Custom `json:"custom"`
}

const (
	DefaultBacklight = 70

	DefaultScreenMode = ModeAuto

	DefaultHourFormat = TwentyFourHour

	DefaultMarks = true
)

func DefaultScreen() Screen {
	return Screen{
		Backlight: DefaultBacklight,
		Mode:      DefaultScreenMode,
		Theme:     theme.Follow,
		Hours:     DefaultHourFormat,
		Keyboard:  KeyboardCompact,
		Drawer:    DefaultEdge,
		DockHide:  DelayNever,
		Volume:    DefaultEdge,
		Alert:     DefaultAlertStyle,
		Timer:     DefaultTimerStyle,
		Marks:     DefaultMarks,
	}
}

const (
	UISizeMini    = "mini"
	UISizeCompact = "compact"
	UISizeMedium  = "medium"
	UISizeLarge   = "large"
	UISizeXLarge  = "xlarge"
)

func ScreenSizes() []string {
	return []string{UISizeMini, UISizeCompact, UISizeMedium, UISizeLarge, UISizeXLarge}
}

type ScreenMode string

const (
	ModeAuto ScreenMode = "auto"

	ModeManual ScreenMode = "manual"
)

func (m ScreenMode) Label() string {
	switch m {
	case ModeAuto:
		return say.T("mode.auto")
	case ModeManual:
		return say.T("mode.manual")
	}
	return string(m)
}

func ScreenModes() []ScreenMode { return []ScreenMode{ModeAuto, ModeManual} }

type HourFormat string

const (
	TwentyFourHour HourFormat = "24"
	TwelveHour     HourFormat = "12"
)

func (h HourFormat) Label() string {
	switch h {
	case TwentyFourHour:
		return say.T("hours.24")
	case TwelveHour:
		return say.T("hours.12")
	}
	return string(h)
}

func HourFormats() []HourFormat { return []HourFormat{TwentyFourHour, TwelveHour} }

type KeyboardSize string

const (
	KeyboardCompact  KeyboardSize = "compact"
	KeyboardStandard KeyboardSize = "standard"
	KeyboardLarge    KeyboardSize = "large"
)

func (k KeyboardSize) Label() string {
	switch k {
	case KeyboardCompact:
		return say.T("keyboard.compact")
	case KeyboardStandard:
		return say.T("keyboard.standard")
	case KeyboardLarge:
		return say.T("keyboard.large")
	}
	return string(k)
}

func KeyboardSizes() []KeyboardSize {
	return []KeyboardSize{KeyboardCompact, KeyboardStandard, KeyboardLarge}
}

const DefaultTheme = theme.Follow
