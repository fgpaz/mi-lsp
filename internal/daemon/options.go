package daemon

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

const (
	WatchModeOff   = "off"
	WatchModeLazy  = "lazy"
	WatchModeEager = "eager"

	defaultMaxWatchedRoots = 8
	defaultMaxInflight     = 16
)

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
