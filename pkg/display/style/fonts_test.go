package style

import (
	"math"
	"strings"
	"testing"

	"golang.org/x/image/font/sfnt"

	"github.com/ygelfand/libcountertop/pkg/settings/schema"
)

func TestEveryFontIsEmbeddedInRegularAndBold(t *testing.T) {
	for _, f := range faces {
		for suffix, sub := range map[string]string{".ttf": "Regular", "bold.ttf": "Bold"} {
			parsed, err := sfnt.Parse(f.load(suffix))
			if err != nil {
				t.Fatalf("%s%s: %v", f.file, suffix, err)
			}
			var buf sfnt.Buffer
			family, _ := parsed.Name(&buf, sfnt.NameIDFamily)
			got, _ := parsed.Name(&buf, sfnt.NameIDSubfamily)
			if family != f.name() || got != sub {
				t.Errorf("%s%s is %q %q, want %q %q", f.file, suffix, family, got, f.name(), sub)
			}
		}
	}
}

func TestEveryFontTakesGosLineBoxAtItsSize(t *testing.T) {
	for _, f := range faces {
		for _, suffix := range []string{".ttf", "bold.ttf"} {
			_, raw := extents(mustRead(f.file + suffix))
			ttf := f.load(suffix)
			_, upem := extents(ttf)
			if got := lineBox(ttf); math.Abs(got-goLine) > 1/upem {
				t.Errorf("%s%s line box %.3f em, want %.3f", f.file, suffix, got, goLine)
			}
			if got := raw / upem; math.Abs(got-f.size()) > 1/upem {
				t.Errorf("%s%s drawn at ×%.3f, want ×%v", f.file, suffix, got, f.size())
			}
		}
	}
}

func TestEveryFamilyIsOneWord(t *testing.T) {
	for _, f := range faces {
		if words := strings.Fields(f.name()); len(words) != 1 {
			t.Errorf("%s asks for %q", f.font, f.name())
		}
	}
}

func TestEveryFontOnOfferHasAFace(t *testing.T) {
	for _, font := range schema.Fonts() {
		if font == schema.FontStyle {
			continue
		}
		if Family(Standard, font) == ByName(Standard).Family {
			t.Errorf("%s has no face", font)
		}
	}
	for _, f := range faces {
		if s := f.size(); s < 0.85 || s > 1.15 {
			t.Errorf("%s draws at ×%v", f.font, s)
		}
	}
}

func TestStyleDefaultKeepsEachStylesFamily(t *testing.T) {
	for _, s := range All {
		if got := Family(s.Name, schema.FontStyle); got != s.Family {
			t.Errorf("%s with its default font draws in %q, want %q", s.Name, got, s.Family)
		}
	}
}
