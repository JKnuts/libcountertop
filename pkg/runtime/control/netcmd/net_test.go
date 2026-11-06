package netcmd

import (
	"testing"
	"time"
)

func TestNumbersFallBackAndStayInRange(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want int
		bad  bool
	}{
		{nil, 10, false},
		{[]string{"3"}, 3, false},
		{[]string{"0"}, 0, true},
		{[]string{"1001"}, 0, true},
		{[]string{"x"}, 0, true},
	} {
		got, err := number(tc.args, 0, 10, 1, 1000)
		if (err != nil) != tc.bad || got != tc.want {
			t.Errorf("number(%v) = %d, %v; want %d, bad %v", tc.args, got, err, tc.want, tc.bad)
		}
	}
}

func TestDurationsAreWholeSecondsWithinTheCap(t *testing.T) {
	if got, err := duration([]string{"5"}, 0, 0); err != nil || got != 5*time.Second {
		t.Errorf("duration(5) = %s, %v", got, err)
	}
	if got, err := duration(nil, 0, watchFor); err != nil || got != watchFor {
		t.Errorf("duration() = %s, %v; want the fallback", got, err)
	}
	for _, bad := range []string{"0", "601", "1.5", "-2"} {
		if _, err := duration([]string{bad}, 0, 0); err == nil {
			t.Errorf("duration(%q) took it", bad)
		}
	}
}
