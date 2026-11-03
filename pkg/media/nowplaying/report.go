package nowplaying

import (
	"fmt"
	"strings"
	"time"
)

func Report(now Now, source string, external bool, args []string) (string, error) {
	if len(args) > 0 {
		return "", fmt.Errorf("player takes no arguments")
	}

	var b strings.Builder
	fmt.Fprintf(&b, "source      %s\n", source)
	fmt.Fprintf(&b, "external    %v\n", external)
	fmt.Fprintf(&b, "playing     %v\n", now.Playing)
	fmt.Fprintf(&b, "paused      %v\n", now.Paused)
	fmt.Fprintf(&b, "title       %q\n", now.Title)
	fmt.Fprintf(&b, "artist      %q\n", now.Artist)
	fmt.Fprintf(&b, "album       %q\n", now.Album)
	fmt.Fprintf(&b, "art         %v\n", now.Art != nil)
	fmt.Fprintf(&b, "elapsed     %v\n", now.Elapsed.Round(time.Second))
	fmt.Fprintf(&b, "length      %v\n", now.Length.Round(time.Second))
	fmt.Fprintf(&b, "live        %v\n", now.LiveWithin != 0)
	fmt.Fprintf(&b, "queue       %d\n", len(now.Queue))
	fmt.Fprintf(&b, "controls    %04b\n", now.Can)

	return b.String(), nil
}
