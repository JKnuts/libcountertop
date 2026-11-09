package match

import (
	"slices"
	"strings"

	"github.com/antzucaro/matchr"

	"github.com/ygelfand/libcountertop/pkg/voice"
)

const (
	Threshold = 0.85
	margin    = 0.05
	sounds    = 0.93
	word      = 0.98
	shortest  = 3
)

type Candidate struct {
	Key  string
	Name string
}

type scored struct {
	key   string
	score float64
}

func Best(said string, candidates []Candidate) (string, bool) {
	all := ranked(said, candidates)
	if len(all) == 0 || all[0].score < Threshold {
		return "", false
	}
	exact := all[0].score == 1 && (len(all) == 1 || all[1].score < 1)
	if !exact && len(all) > 1 && all[1].key != all[0].key &&
		all[0].score-all[1].score < margin {
		return "", false
	}
	return all[0].key, true
}

func Closest(said string, candidates []Candidate) (string, bool) {
	all := ranked(said, candidates)
	if len(all) == 0 || all[0].score < Threshold {
		return "", false
	}
	return all[0].key, true
}

func ranked(said string, candidates []Candidate) []scored {
	words := voice.Normalize(said)
	words = strings.TrimPrefix(words, "the ")
	want := compact(words)
	if want == "" {
		return nil
	}
	all := make([]scored, 0, len(candidates))
	for _, c := range candidates {
		all = append(all, scored{key: c.Key, score: score(want, c)})
	}
	slices.SortStableFunc(all, func(a, b scored) int {
		switch {
		case a.score > b.score:
			return -1
		case a.score < b.score:
			return 1
		}
		return 0
	})
	return all
}

func score(want string, c Candidate) float64 {
	name := voice.Normalize(c.Name)
	var words []string
	for w := range strings.FieldsSeq(name) {
		if len(w) >= shortest {
			words = append(words, w)
		}
	}
	return max(
		similar(want, []string{compact(name), strings.ToLower(c.Key)}, 1),
		similar(want, words, word),
	)
}

func similar(want string, forms []string, exact float64) float64 {
	best := 0.0
	wa, wb := matchr.DoubleMetaphone(want)
	for _, f := range forms {
		if f == "" {
			continue
		}
		if f == want {
			return exact
		}
		s := matchr.JaroWinkler(want, f, false)
		if fa, fb := matchr.DoubleMetaphone(f); alike(wa, wb, fa, fb) {
			s = max(s, sounds)
		}
		best = max(best, min(s, exact))
	}
	return best
}

func alike(wa, wb, fa, fb string) bool {
	for _, w := range []string{wa, wb} {
		for _, f := range []string{fa, fb} {
			if w != "" && w == f {
				return true
			}
		}
	}
	return false
}

func compact(s string) string { return strings.ReplaceAll(s, " ", "") }
