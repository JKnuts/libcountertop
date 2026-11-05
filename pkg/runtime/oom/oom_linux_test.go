package oom

import (
	"os"
	"testing"
)

func TestAProcessCanGiveUpProtection(t *testing.T) {
	self := os.Getpid()
	was, err := Get(self)
	if err != nil {
		t.Fatal(err)
	}
	want := min(was+1, 1000)
	if err := Set(self, want); err != nil {
		t.Fatal(err)
	}
	if got, err := Get(self); err != nil || got != want {
		t.Errorf("score = %d, %v, want %d", got, err, want)
	}
}
