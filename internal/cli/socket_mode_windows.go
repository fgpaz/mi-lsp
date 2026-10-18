//go:build windows

package cli

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
)

func listenOwnerSocket(path string) (net.Listener, error) {
	return nil, fmt.Errorf("Unix socket transport is unavailable on Windows")
}

func socketSignalContext(parent context.Context) (context.Context, context.CancelFunc) {
	return signal.NotifyContext(parent, os.Interrupt)
}
