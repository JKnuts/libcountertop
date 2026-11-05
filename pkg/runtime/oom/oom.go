package oom

import (
	"log/slog"
	"os"
	"strconv"
	"strings"
)

// Persistent is the score Android gives its persistent system services; its low-memory killer never picks below 0.
const Persistent = -800

func Set(pid, score int) error {
	return os.WriteFile(path(pid), []byte(strconv.Itoa(score)), 0o644)
}

func Get(pid int) (int, error) {
	b, err := os.ReadFile(path(pid))
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(b)))
}

func Settle() {
	if err := Set(os.Getpid(), Persistent); err != nil {
		slog.Warn("lowering the daemon's out-of-memory protection failed", "err", err)
	}
}

func path(pid int) string { return "/proc/" + strconv.Itoa(pid) + "/oom_score_adj" }
