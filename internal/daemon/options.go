package daemon

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	WatchModeOff   = "off"
	WatchModeLazy  = "lazy"
	WatchModeEager = "eager"

	defaultMaxWatchedRoots = 8
	defaultMaxInflight     = 48
	// defaultMaxWorkers covers one runtime per supported backend (Roslyn,
	// gopls, pyright, tsserver, rust-analyzer, Kotlin) plus headroom for a
	// second project of the same language, so concurrent multi-language work
	// does not thrash expensive runtimes through LRU eviction.
	defaultMaxWorkers = 8
	// DefaultIdleTimeout frees an idle language-server runtime after 10 minutes.
	// Post-edit diagnostics from many workspaces kept runtimes alive for 30
	// minutes each and saturated CPU/RAM on the operator machine (08/11/2026).
	DefaultIdleTimeout = 10 * time.Minute
)

// DefaultMaxWorkers returns MI_LSP_DAEMON_MAX_WORKERS when it is a positive
// integer, or defaultMaxWorkers otherwise.
func DefaultMaxWorkers() int {
	return envInt("MI_LSP_DAEMON_MAX_WORKERS", defaultMaxWorkers)
}

// normalizeMaxWorkers maps unset or invalid values to DefaultMaxWorkers.
func normalizeMaxWorkers(maxWorkers int) int {
	if maxWorkers <= 0 {
		return DefaultMaxWorkers()
	}
	return maxWorkers
}

type StartOptions struct {
	WatchMode       string
	MaxWatchedRoots int
	MaxInflight     int
}

func DefaultStartOptions() StartOptions {
	return NormalizeStartOptions(StartOptions{
		WatchMode:       os.Getenv("MI_LSP_WATCH_MODE"),
		MaxWatchedRoots: envInt("MI_LSP_WATCH_MAX_ROOTS", defaultMaxWatchedRoots),
		MaxInflight:     envInt("MI_LSP_DAEMON_MAX_INFLIGHT", defaultMaxInflight),
	})
}

func NormalizeStartOptions(options StartOptions) StartOptions {
	mode := strings.ToLower(strings.TrimSpace(options.WatchMode))
	switch mode {
	case WatchModeOff, WatchModeEager:
	default:
		mode = WatchModeLazy
	}
	if options.MaxWatchedRoots <= 0 {
		options.MaxWatchedRoots = defaultMaxWatchedRoots
	}
	if options.MaxInflight <= 0 {
		options.MaxInflight = defaultMaxInflight
	}
	options.WatchMode = mode
	return options
}

func ValidateWatchMode(mode string) error {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", WatchModeOff, WatchModeLazy, WatchModeEager:
		return nil
	default:
		return fmt.Errorf("invalid watch mode %q; valid values: off, lazy, eager", mode)
	}
}

func envInt(name string, defaultValue int) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return defaultValue
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return defaultValue
	}
	return value
}
