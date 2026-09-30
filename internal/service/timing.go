package service

import (
	"fmt"
	"os"
	"time"
)

func traceServiceTiming(stage string, started time.Time) {
	if os.Getenv("MI_LSP_TIMING") == "" {
		return
	}
	fmt.Fprintf(os.Stderr, "mi-lsp timing service.%s_ms=%d\n", stage, time.Since(started).Milliseconds())
}
