package mix

import (
	"slices"
	"testing"
	"time"

	"github.com/ygelfand/libcountertop/pkg/settings/schema"
)

func TestBusScalesEachStreamByItsOwnGain(t *testing.T) {
	var b Bus
	b.SetGain(schema.StreamMedia, 0.5)
	b.SetGain(schema.StreamVoice, 0.25)
	b.Push(schema.StreamMedia, []int16{1000, 1000})
	b.Push(schema.StreamVoice, []int16{400, 400})

	out := make([]int32, 2)
	queued, rendered := b.Next(0, out)

	if want := []int32{600, 600}; !slices.Equal(out, want) {
		t.Errorf("mixed %v, want %v", out, want)
	}
	if queued != 2 || rendered {
		t.Errorf("queued %d rendered %t, want 2 and false", queued, rendered)
	}
}

func TestBusPartsHandEachPlayingStreamOverUnscaled(t *testing.T) {
	var b Bus
	b.SetGain(schema.StreamMedia, 0.5)
	b.SetGain(schema.StreamVoice, 0.25)
	b.SetGain(schema.StreamAlerts, 1)
	b.Mute(schema.StreamVoice, true)
	b.Push(schema.StreamMedia, []int16{1000, 1000})
	b.Push(schema.StreamVoice, []int16{400, 400})
	b.Attach(steady(7))

	parts, queued, rendered := b.Parts(0, 2)
	if queued != 2 || !rendered {
		t.Errorf("queued %d rendered %t, want 2 and true", queued, rendered)
	}
	got := map[schema.Stream][]Part{}
	for _, p := range parts {
		got[p.Stream] = append(got[p.Stream], p)
	}
	if len(got[schema.StreamAlerts]) != 0 {
		t.Error("a stream with nothing queued was handed over")
	}
	if v := got[schema.StreamVoice]; len(v) != 1 || v[0].Level != 0 || v[0].Samples[0] != 400 {
		t.Errorf("muted voice %+v, want its samples at level 0", v)
	}
	media := got[schema.StreamMedia]
	if len(media) != 2 {
		t.Fatalf("media parts %+v, want the queue and the source", media)
	}
	for _, p := range media {
		if p.Level != 0.5 || (p.Samples[0] != 1000 && p.Samples[0] != 7) {
			t.Errorf("media part %+v, want unscaled samples at level 0.5", p)
		}
	}
}

func TestBusAppliesAGainChangeToWhatIsAlreadyQueued(t *testing.T) {
	var b Bus
	b.Push(schema.StreamMedia, []int16{1000, 1000, 1000, 1000})

	out := make([]int32, 2)
	b.Next(0, out)
	b.SetGain(schema.StreamMedia, 0.5)
	b.Next(0, out)

	if want := []int32{500, 500}; !slices.Equal(out, want) {
		t.Errorf("after the change %v, want %v", out, want)
	}
}

func TestBusMuteSilencesOneStreamAndKeepsItsGain(t *testing.T) {
	var b Bus
	b.SetGain(schema.StreamMedia, 0.5)
	b.Mute(schema.StreamMedia, true)
	b.Push(schema.StreamMedia, []int16{1000, 1000})
	b.Push(schema.StreamFeedback, []int16{100, 100})

	out := make([]int32, 2)
	b.Next(0, out)
	if want := []int32{100, 100}; !slices.Equal(out, want) {
		t.Errorf("muted mix %v, want %v", out, want)
	}
	if !b.Muted(schema.StreamMedia) || b.Muted(schema.StreamFeedback) {
		t.Error("mute reached the wrong stream")
	}

	b.Mute(schema.StreamMedia, false)
	if got := b.Gain(schema.StreamMedia); got != 0.5 {
		t.Errorf("unmuted gain %v, want 0.5", got)
	}
}

func TestBusTakeReturnsSamplesUnscaled(t *testing.T) {
	var b Bus
	b.SetGain(schema.StreamMedia, 0.5)
	b.Push(schema.StreamMedia, []int16{1000, 1000})

	kept := b.Take(schema.StreamMedia)
	b.Push(schema.StreamMedia, kept)

	out := make([]int32, 2)
	b.Next(0, out)
	if want := []int32{500, 500}; !slices.Equal(out, want) {
		t.Errorf("replayed %v, want %v once scaled", out, want)
	}
}

func TestBusTakeLeavesOtherStreams(t *testing.T) {
	var b Bus
	b.Push(schema.StreamMedia, []int16{1, 2})
	b.Push(schema.StreamAlerts, []int16{3, 4, 5})

	if got := b.Take(schema.StreamMedia); !slices.Equal(got, []int16{1, 2}) {
		t.Errorf("took %v", got)
	}
	if got := b.Len(); got != 3 {
		t.Errorf("len %d, want the alerts still queued", got)
	}
	b.Clear()
	if got := b.Len(); got != 0 {
		t.Errorf("len %d after clear", got)
	}
}

type steady int16

func (s steady) Render(_ uint64, out []int16) {
	for i := range out {
		out[i] = int16(s)
	}
}

func TestBusScalesTheAttachedSourceAsMedia(t *testing.T) {
	var b Bus
	b.SetGain(schema.StreamMedia, 0.5)
	b.Attach(steady(800))

	out := make([]int32, 2)
	queued, rendered := b.Next(0, out)
	if want := []int32{400, 400}; !slices.Equal(out, want) {
		t.Errorf("rendered %v, want %v", out, want)
	}
	if queued != 0 || !rendered {
		t.Errorf("queued %d rendered %t, want 0 and true", queued, rendered)
	}
}

func TestBusOverlayMixesWithinOneStream(t *testing.T) {
	var b Bus
	b.Push(schema.StreamFeedback, []int16{10, 10})
	b.Overlay(schema.StreamFeedback, []int16{5, 5, 5})

	out := make([]int32, 3)
	b.Next(0, out)
	if want := []int32{15, 15, 5}; !slices.Equal(out, want) {
		t.Errorf("overlaid %v, want %v", out, want)
	}
}

func TestBusHearsWhatItRendered(t *testing.T) {
	var b Bus
	out := make([]int32, 2)
	if b.Heard(schema.StreamVoice, time.Minute) {
		t.Fatal("heard voice before any was pushed")
	}

	b.Push(schema.StreamVoice, []int16{1, 1})
	b.Next(0, out)
	if !b.Heard(schema.StreamVoice, time.Minute) || b.Heard(schema.StreamMedia, time.Minute) {
		t.Error("after voice played, want voice heard and media not")
	}
	if b.Heard(schema.StreamVoice, 0) {
		t.Error("voice heard within no time at all")
	}

	b.Attach(steady(1))
	b.Next(0, out)
	if !b.Heard(schema.StreamMedia, time.Minute) {
		t.Error("an attached source did not count as media")
	}
}
