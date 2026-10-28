//go:build !windows

package daemon

func checkDaemonStartIntegrity() error { return nil }
