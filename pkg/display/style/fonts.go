package style

import (
	"bytes"
	"embed"
	"encoding/binary"
	"math"
	"slices"
	"strings"

	gogui "github.com/go-gui-org/go-gui/gui"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/ygelfand/libcountertop/pkg/settings/schema"
)

const (
	SerifFamily = "PT Serif"
	LightFamily = "Lato Light"
)

//go:embed *.ttf
var ttfs embed.FS

type face struct {
	font   schema.Font
	family string
	file   string
	scale  float64
}

var faces = []face{
	{font: schema.FontRoboto, family: "Roboto", file: "roboto"},
	{font: schema.FontAtkinson, family: "Atkinson Hyperlegible", file: "atkinson"},
	{font: schema.FontNunito, family: "Nunito", file: "nunito"},
	{font: schema.FontFredoka, family: "Fredoka", file: "fredoka"},
	{font: schema.FontComfortaa, family: "Comfortaa", file: "comfortaa", scale: 0.89},
	{font: schema.FontBarlow, family: "Barlow Condensed", file: "barlowcondensed"},
	{font: schema.FontOswald, family: "Oswald", file: "oswald"},
	{font: schema.FontRobotoSlab, family: "Roboto Slab", file: "robotoslab"},
	{font: schema.FontLora, family: "Lora", file: "lora"},
	{font: schema.FontSpaceGrotesk, family: "Space Grotesk", file: "spacegrotesk", scale: 0.95},
	{font: schema.FontExo, family: "Exo 2", file: "exo2"},
}

var goLine = lineBox(goregular.TTF)

func init() {
	gogui.RegisterAppFontBytes(mustRead("ptserif.ttf"))
	gogui.RegisterAppFontBytes(mustRead("latolight.ttf"))
	for _, f := range faces {
		gogui.RegisterAppFontBytes(f.load(".ttf"))
		gogui.RegisterAppFontBytes(f.load("bold.ttf"))
	}
}

func Family(style string, font schema.Font) string {
	for _, f := range faces {
		if f.font == font {
			return f.name()
		}
	}
	return ByName(style).Family
}

// go-glyph strips trailing numbers and words like "Condensed" from a family name.
func (f face) name() string { return strings.ReplaceAll(f.family, " ", "-") }

func (f face) size() float64 {
	if f.scale > 0 {
		return f.scale
	}
	return 1
}

func (f face) load(suffix string) []byte {
	return fitted(renamed(mustRead(f.file+suffix), f.family, f.name()), goLine, f.size())
}

func fitted(ttf []byte, box, scale float64) []byte {
	at, upem := extents(ttf)
	if at == 0 {
		return ttf
	}
	out := slices.Clone(ttf)
	upem = math.Round(upem / scale)
	binary.BigEndian.PutUint16(out[sfntTable(out, "head")+18:], uint16(upem))
	asc := float64(int16(binary.BigEndian.Uint16(out[at:])))
	desc := -float64(int16(binary.BigEndian.Uint16(out[at+2:])))
	total := math.Round(box * upem)
	a := math.Round(total * asc / (asc + desc))
	for _, t := range []uint32{sfntTable(out, "hhea") + 4, sfntTable(out, "OS/2") + 68} {
		binary.BigEndian.PutUint16(out[t:], uint16(int16(a)))
		binary.BigEndian.PutUint16(out[t+2:], uint16(int16(a-total)))
		binary.BigEndian.PutUint16(out[t+4:], 0)
	}
	return out
}

func lineBox(ttf []byte) float64 {
	at, upem := extents(ttf)
	asc := int(int16(binary.BigEndian.Uint16(ttf[at:])))
	desc := int(int16(binary.BigEndian.Uint16(ttf[at+2:])))
	gap := max(0, int(int16(binary.BigEndian.Uint16(ttf[at+4:]))))
	return float64(asc-desc+gap) / upem
}

// go-text reads OS/2 typo metrics when fsSelection bit 7 is set, hhea otherwise.
func extents(ttf []byte) (at uint32, upem float64) {
	hhea, head, os2 := sfntTable(ttf, "hhea"), sfntTable(ttf, "head"), sfntTable(ttf, "OS/2")
	if hhea == 0 || head == 0 || os2 == 0 {
		return 0, 0
	}
	upem = float64(binary.BigEndian.Uint16(ttf[head+18:]))
	if binary.BigEndian.Uint16(ttf[os2+62:])&(1<<7) != 0 {
		return os2 + 68, upem
	}
	return hhea + 4, upem
}

func renamed(ttf []byte, from, to string) []byte {
	at := sfntTable(ttf, "name")
	if at == 0 || from == to || len(from) != len(to) {
		return ttf
	}
	out := slices.Clone(ttf)
	pool := int(at) + int(binary.BigEndian.Uint16(out[at+4:]))
	count := int(binary.BigEndian.Uint16(out[at+2:]))
	for i := range count {
		rec := out[int(at)+6+12*i:]
		length := int(binary.BigEndian.Uint16(rec[8:]))
		start := pool + int(binary.BigEndian.Uint16(rec[10:]))
		text := out[start : start+length]
		copy(text, bytes.ReplaceAll(text, []byte(from), []byte(to)))
		copy(text, bytes.ReplaceAll(text, utf16be(from), utf16be(to)))
	}
	return out
}

func utf16be(s string) []byte {
	out := make([]byte, 0, 2*len(s))
	for _, r := range s {
		out = append(out, byte(r>>8), byte(r))
	}
	return out
}

func mustRead(name string) []byte {
	b, err := ttfs.ReadFile(name)
	if err != nil {
		panic(err)
	}
	return b
}

func sfntTable(ttf []byte, tag string) uint32 {
	n := int(binary.BigEndian.Uint16(ttf[4:]))
	for i := range n {
		rec := ttf[12+16*i:]
		if string(rec[:4]) == tag {
			return binary.BigEndian.Uint32(rec[8:])
		}
	}
	return 0
}
