package lyrics

import (
	"sort"
	"strconv"
	"strings"
	"time"
)

type Line struct {
	At   time.Duration
	Text string
}

type Sheet struct {
	Lines  []Line
	Synced bool
}

func (s Sheet) Empty() bool { return len(s.Lines) == 0 }

func (s Sheet) Current(elapsed time.Duration) int {
	if !s.Synced {
		return -1
	}
	return sort.Search(len(s.Lines), func(i int) bool { return s.Lines[i].At > elapsed }) - 1
}

func ParseLRC(text string) []Line {
	var offset time.Duration
	var out []Line
	for raw := range strings.Lines(text) {
		row := strings.TrimSpace(raw)
		var stamps []time.Duration
		for strings.HasPrefix(row, "[") {
			end := strings.IndexByte(row, ']')
			if end < 0 {
				break
			}
			tag := row[1:end]
			row = row[end+1:]
			if at, ok := stamp(tag); ok {
				stamps = append(stamps, at)
				continue
			}
			if v, ok := strings.CutPrefix(tag, "offset:"); ok {
				if ms, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
					offset = time.Duration(ms) * time.Millisecond
				}
			}
		}
		words := strings.TrimSpace(row)
		for _, at := range stamps {
			out = append(out, Line{At: at, Text: words})
		}
	}
	for i := range out {
		out[i].At = max(out[i].At-offset, 0)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].At < out[j].At })
	return out
}

func ParsePlain(text string) []Line {
	var out []Line
	for raw := range strings.Lines(text) {
		out = append(out, Line{Text: strings.TrimSpace(raw)})
	}
	for len(out) > 0 && out[len(out)-1].Text == "" {
		out = out[:len(out)-1]
	}
	return out
}

func stamp(tag string) (time.Duration, bool) {
	mins, rest, ok := strings.Cut(tag, ":")
	if !ok {
		return 0, false
	}
	m, err := strconv.Atoi(mins)
	if err != nil || m < 0 {
		return 0, false
	}
	s, err := strconv.ParseFloat(rest, 64)
	if err != nil || s < 0 || s >= 60 {
		return 0, false
	}
	return time.Duration(m)*time.Minute + time.Duration(s*float64(time.Second)), true
}
