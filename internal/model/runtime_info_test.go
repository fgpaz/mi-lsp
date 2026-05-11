package model

import (
	"strings"
	"testing"
	"time"
)

func TestDaemonStaleWarningWhenMetadataMissing(t *testing.T) {
	warning := DaemonStaleWarning(DaemonState{}, ExecutableSnapshot{Path: "C:/Users/test/bin/mi-lsp.exe"})
	if !strings.Contains(warning, "older build") || !strings.Contains(warning, "daemon restart") {
		t.Fatalf("warning = %q, want older-build restart guidance", warning)
	}
}

func TestDaemonStaleWarningWhenExecutableMTimeDiffers(t *testing.T) {
	base := time.Date(2026, 5, 11, 10, 0, 0, 0, time.UTC)
	warning := DaemonStaleWarning(
		DaemonState{ExecutablePath: "C:/Users/test/bin/mi-lsp.exe", ExecutableSize: 10, ExecutableMTime: base},
		ExecutableSnapshot{Path: "C:/Users/test/bin/mi-lsp.exe", Size: 10, ModTime: base.Add(time.Minute)},
	)
	if !strings.Contains(warning, "appears stale") || !strings.Contains(warning, "daemon restart") {
		t.Fatalf("warning = %q, want stale restart guidance", warning)
	}
}

func TestDaemonStaleWarningWhenExecutableHashDiffers(t *testing.T) {
	base := time.Date(2026, 5, 11, 10, 0, 0, 0, time.UTC)
	warning := DaemonStaleWarning(
		DaemonState{ExecutablePath: "C:/Users/test/bin/mi-lsp.exe", ExecutableSize: 10, ExecutableMTime: base, ExecutableSHA256: "aaaaaaaaaaaaaaaa"},
		ExecutableSnapshot{Path: "C:/Users/test/bin/mi-lsp.exe", Size: 10, ModTime: base, SHA256: "bbbbbbbbbbbbbbbb"},
	)
	if !strings.Contains(warning, "appears stale") || !strings.Contains(warning, "hash") {
		t.Fatalf("warning = %q, want hash stale guidance", warning)
	}
}

func TestDaemonStaleWarningIgnoresDifferentPathForSameExecutableHash(t *testing.T) {
	base := time.Date(2026, 5, 11, 10, 0, 0, 0, time.UTC)
	warning := DaemonStaleWarning(
		DaemonState{ExecutablePath: "C:/Users/test/AppData/Local/go-build/cache/mi-lsp.exe", ExecutableSize: 10, ExecutableMTime: base, ExecutableSHA256: "aaaaaaaaaaaaaaaa"},
		ExecutableSnapshot{Path: "C:/Users/test/AppData/Local/Temp/go-build/exe/mi-lsp.exe", Size: 10, ModTime: base.Add(time.Minute), SHA256: "aaaaaaaaaaaaaaaa"},
	)
	if warning != "" {
		t.Fatalf("warning = %q, want empty for same executable content", warning)
	}
}

func TestDaemonStaleWarningReturnsEmptyForSameExecutable(t *testing.T) {
	base := time.Date(2026, 5, 11, 10, 0, 0, 0, time.UTC)
	warning := DaemonStaleWarning(
		DaemonState{ExecutablePath: "C:/Users/test/bin/mi-lsp.exe", ExecutableSize: 10, ExecutableMTime: base},
		ExecutableSnapshot{Path: "C:/Users/test/bin/mi-lsp.exe", Size: 10, ModTime: base},
	)
	if warning != "" {
		t.Fatalf("warning = %q, want empty", warning)
	}
}
