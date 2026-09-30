package daemon

import (
	"os"
	"runtime/debug"
	"strconv"
	"strings"
)

// ApplyDaemonMemoryPolicy bounds a new daemon process. It does not touch a
// daemon that is already running. MI_LSP_DAEMON_GOMEMLIMIT defaults to 96MiB,
// above the observed ~30MB working set. MI_LSP_DAEMON_GOGC is applied only
// when set, so the default collector cadence stays unchanged.
func ApplyDaemonMemoryPolicy() int64 {
	limit := parseByteLimit(os.Getenv("MI_LSP_DAEMON_GOMEMLIMIT"), 96<<20)
	debug.SetMemoryLimit(limit)
	if raw := strings.TrimSpace(os.Getenv("MI_LSP_DAEMON_GOGC")); raw != "" {
		if percent, err := strconv.Atoi(raw); err == nil && percent > 0 {
			debug.SetGCPercent(percent)
		}
	}
	return limit
}

func parseByteLimit(raw string, fallback int64) int64 {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return fallback
	}
	mult := int64(1)
	switch {
	case strings.HasSuffix(raw, "mib"), strings.HasSuffix(raw, "mb"), strings.HasSuffix(raw, "m"):
		raw = strings.TrimRight(raw, "mib")
		raw = strings.TrimSuffix(raw, "m")
		mult = 1 << 20
	case strings.HasSuffix(raw, "kib"), strings.HasSuffix(raw, "kb"), strings.HasSuffix(raw, "k"):
		raw = strings.TrimRight(raw, "kib")
		raw = strings.TrimSuffix(raw, "k")
		mult = 1 << 10
	}
	value, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || value <= 0 {
		return fallback
	}
	return value * mult
}
