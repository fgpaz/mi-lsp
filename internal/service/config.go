package service

import "time"

// Config centralizes operational defaults that were previously hardcoded.
type Config struct {
	DefaultSearchLimit  int
	DefaultTokenBudget  int
	DefaultMaxItems     int
	OperationTimeout    time.Duration
	DaemonClientTimeout time.Duration
}

// DefaultConfig returns the default configuration matching prior hardcoded values.
func DefaultConfig() Config {
	return Config{
		DefaultSearchLimit:  200,
		DefaultTokenBudget:  4000,
		DefaultMaxItems:     50,
		OperationTimeout:    2 * time.Minute,
		DaemonClientTimeout: 5 * time.Second,
	}
}
