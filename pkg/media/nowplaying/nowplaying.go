package nowplaying

import (
	"time"
)

type Now struct {
	Playing bool
	Paused  bool

	Hold bool

	Title  string
	Artist string
	Album  string

	Art   []byte
	ArtID uint64

	Mark   []byte
	MarkID uint64

	Logo   []byte
	LogoID uint64
	Brand  Brand

	Elapsed  time.Duration
	Length   time.Duration
	Earliest time.Duration
	Edge     time.Duration

	LiveWithin time.Duration

	Break *Break

	Queue []Track

	Shuffle bool
	Repeat  Repeat
	Liked   bool

	Can Controls
}

type Repeat string

const (
	RepeatOff Repeat = "off"
	RepeatAll Repeat = "all"
	RepeatOne Repeat = "one"
)

func (r Repeat) Next() Repeat {
	switch r {
	case RepeatAll:
		return RepeatOne
	case RepeatOne:
		return RepeatOff
	}
	return RepeatAll
}

type Shuffler interface {
	SetShuffle(on bool)
}

type Repeater interface {
	SetRepeat(r Repeat)
}

type Liker interface {
	SetLiked(on bool)
}

type Break struct {
	Clip, Clips int
	At, Length  time.Duration
}

type Track struct {
	Title  string
	Artist string
	Length time.Duration

	Art   []byte
	ArtID uint64

	Play func()
}

type Seeker interface {
	Seek(to time.Duration)
	CanSeek() bool
}

type Opener interface {
	Open() bool
}

type Controls uint8

const (
	CanPause Controls = 1 << iota
	CanStop
	CanNext
	CanPrevious
	CanShuffle
	CanRepeat
	CanLike
)

func (c Controls) Has(want Controls) bool { return c&want == want }

type Kind uint8

const (
	FromQueue Kind = iota
	FromBluetooth
	FromGroup
	FromCast
)

type Brand uint8

const (
	Unbranded Brand = iota
	AirPlay
)

type Details interface {
	Now() Now
	Next()
	Previous()

	Kind() Kind

	Label() string
}

type Controller interface {
	Play()
	Pause()
	Stop()
	Next()
	Previous()
}

type Source interface {
	Controller
	Details
}
