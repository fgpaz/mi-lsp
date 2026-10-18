//go:build !windows

package cli

import (
	"context"
	"net"
	"os"
	"os/signal"
	"syscall"
)

func listenOwnerSocket(path string) (net.Listener, error) {
	previous := syscall.Umask(0o077)
	defer syscall.Umask(previous)
	return net.Listen("unix", path)
}

func socketSignalContext(parent context.Context) (context.Context, context.CancelFunc) {
	return signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
}
