package tvapp

import (
	"strings"
	"testing"

	"github.com/ygelfand/libcountertop/pkg/homeassistant/yttv"
)

func names(cs []yttv.Channel) string {
	var out []string
	for _, c := range cs {
		out = append(out, c.Name)
	}
	return strings.Join(out, " ")
}

func showing(name, title string) yttv.Channel {
	a := airing(title, "15:00", "16:00")
	return yttv.Channel{Name: name, Title: a.Title, StartsAt: a.StartsAt, EndsAt: a.EndsAt}
}

func TestChannelsRunAToZ(t *testing.T) {
	espn := showing("ESPN", "Game")
	espn.Pinned = true
	got := order([]yttv.Channel{
		{Name: "quiet"},
		showing("PBS", "Nova"),
		espn,
		showing("CNN", "News"),
		{Name: "Adult Swim"},
	}, "", nil)
	if names(got) != "Adult Swim CNN ESPN PBS quiet" {
		t.Errorf("order %q", names(got))
	}
}

func TestFavoritesComeFirst(t *testing.T) {
	got := order([]yttv.Channel{
		showing("CNN", "News"),
		showing("ESPN", "Game"),
		{Name: "Quiet"},
		showing("PBS", "Nova"),
	}, "", []string{"PBS", "Quiet"})
	if names(got) != "PBS Quiet CNN ESPN" {
		t.Errorf("order %q", names(got))
	}
}

func TestSearchMatchesChannelOrAnyShow(t *testing.T) {
	pbs := showing("PBS", "Nova Science Now")
	cnn := showing("CNN", "News")
	cnn.Next = []yttv.Airing{airing("Anderson Cooper", "16:00", "17:00")}
	channels := []yttv.Channel{cnn, pbs, showing("ESPN", "Game")}
	if got := names(order(channels, "nova", nil)); got != "PBS" {
		t.Errorf("show search found %q", got)
	}
	if got := names(order(channels, "cooper", nil)); got != "CNN" {
		t.Errorf("upcoming show search found %q", got)
	}
	if got := names(order(channels, " espn ", nil)); got != "ESPN" {
		t.Errorf("channel search found %q", got)
	}
}
