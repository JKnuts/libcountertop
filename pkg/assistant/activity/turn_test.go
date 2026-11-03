package activity

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func caught() (*Log, *[]map[string]string) {
	var got []map[string]string
	return New(Options{Emit: func(fields map[string]string) { got = append(got, fields) }}), &got
}

func TestConcurrentCompletionEmitsOnceOutsideLock(t *testing.T) {
	var count atomic.Int32
	var turn *Turn
	log := New(Options{Emit: func(fields map[string]string) {
		count.Add(1)
		turn.Heard("late transcript")
		if fields["heard"] != "original" || fields["outcome"] != string(Completed) {
			t.Errorf("completion snapshot: %v", fields)
		}
	}})
	turn = log.Begin(1, "alexa")
	turn.Listening()
	turn.Heard("original")
	var callers sync.WaitGroup
	for range 32 {
		callers.Go(func() { turn.Ends(Completed) })
	}
	callers.Wait()
	if count.Load() != 1 {
		t.Fatalf("completion emitted %d times", count.Load())
	}
	turn.mu.Lock()
	defer turn.mu.Unlock()
	if turn.heard != "original" {
		t.Fatal("closed turn changed after completion")
	}
}

func TestAFailedTurnReportsNoPhaseItNeverReached(t *testing.T) {
	log, got := caught()

	turn := log.Begin(1, "hey jarvis")
	time.Sleep(2 * time.Millisecond)
	turn.Listening()
	time.Sleep(2 * time.Millisecond)
	turn.Heard("what time is it")
	time.Sleep(2 * time.Millisecond)
	turn.Ends(Failed)

	if len(*got) != 1 {
		t.Fatalf("fired %d events, want 1", len(*got))
	}

	data := (*got)[0]
	for _, key := range []string{"listen_ms", "think_ms"} {
		if data[key] == "" {
			t.Errorf("%s missing, and the turn reached that phase", key)
		}
	}
	if data["speak_ms"] != "" {
		t.Errorf("speak_ms is %q, and the turn never replied", data["speak_ms"])
	}
	if data["reply"] != "" {
		t.Errorf("reply is %q, and there was none", data["reply"])
	}
	if data["outcome"] != string(Failed) {
		t.Errorf("outcome is %q, want %q", data["outcome"], Failed)
	}
	if data["heard"] != "what time is it" {
		t.Errorf("heard is %q", data["heard"])
	}
}

func TestAWholeTurnReportsEveryPhase(t *testing.T) {
	log, got := caught()

	turn := log.Begin(2, "alexa")
	for _, step := range []func(){
		turn.Listening,
		func() { turn.Heard("play something") },
		func() { turn.Replying("playing") },
	} {
		time.Sleep(2 * time.Millisecond)
		step()
	}
	time.Sleep(2 * time.Millisecond)
	turn.Ends(Completed)

	data := (*got)[0]
	for _, key := range []string{"listen_ms", "think_ms", "speak_ms"} {
		if data[key] == "" {
			t.Errorf("%s missing", key)
		}
	}
	if data["slot"] != "2" {
		t.Errorf("slot is %q, want 2", data["slot"])
	}
	if data["id"] == "" {
		t.Error("no id, so nothing could ask for the recording")
	}
	if data["version"] != Version {
		t.Errorf("version is %q, want %q", data["version"], Version)
	}
}

func TestEndingTwiceFiresOnce(t *testing.T) {
	log, got := caught()

	turn := log.Begin(1, "hey jarvis")
	turn.Ends(Completed)
	turn.Ends(Timeout)

	if len(*got) != 1 {
		t.Fatalf("fired %d events, want 1", len(*got))
	}
	if (*got)[0]["outcome"] != string(Completed) {
		t.Errorf("outcome is %q, want the first one", (*got)[0]["outcome"])
	}
}

func TestNoTurnIsSafeToMark(t *testing.T) {
	_, got := caught()

	var turn *Turn
	turn.Listening()
	turn.Heard("nothing")
	turn.Replying("nothing")
	turn.Ends(Cancelled)

	if turn.ID() != "" {
		t.Errorf("id is %q, want empty", turn.ID())
	}
	if len(*got) != 0 {
		t.Errorf("fired %d events, want none", len(*got))
	}
}

func TestDuckingCarriesItsDepth(t *testing.T) {
	var got []map[string]string
	log := New(Options{
		Duck:   func(fields map[string]string) { got = append(got, fields) },
		DuckDB: func() float64 { return -15 },
	})
	log.Ducked(true)
	log.Ducked(false)
	if len(got) != 2 {
		t.Fatalf("fired %d events, want 2", len(got))
	}
	for i, want := range []string{"true", "false"} {
		if got[i]["ducked"] != want || got[i]["duck_db"] != "-15" {
			t.Errorf("event %d: %v, want ducked=%s duck_db=-15", i, got[i], want)
		}
	}
	New(Options{}).Ducked(true)
}
