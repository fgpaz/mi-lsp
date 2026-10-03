package worker

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func TestCanUseGoplsDoesNotPanic(t *testing.T) {
	_ = CanUseGopls("/tmp")
}

func TestNewGoplsClientWithoutBinaryFailsGracefully(t *testing.T) {
	client, err := NewGoplsClient(model.WorkspaceRegistration{
		Root: "/path/that/should/not/contain/gopls",
		Name: "test",
	})
	if err == nil {
		if client == nil {
			t.Fatal("NewGoplsClient returned nil client with nil error")
		}
		return
	}
	if !strings.Contains(err.Error(), "gopls") {
		t.Fatalf("error = %q, want gopls guidance", err.Error())
	}
}

func writeFakeGopls(t *testing.T, dir string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	path := filepath.Join(dir, "gopls")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write fake gopls: %v", err)
	}
	return path
}

func isolateGoplsEnv(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("PATH", t.TempDir())
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("GOBIN", "")
	t.Setenv("GOPATH", "")
	t.Setenv("MI_LSP_GOPLS_PATH", "")
	return home
}

func TestFindGoplsBinaryLookupOrder(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake gopls is a shell script")
	}
	workspaceRoot := t.TempDir()

	t.Run("home go bin", func(t *testing.T) {
		home := isolateGoplsEnv(t)
		want := writeFakeGopls(t, filepath.Join(home, "go", "bin"))
		got, err := findGoplsBinary(workspaceRoot)
		if err != nil || got != want {
			t.Fatalf("findGoplsBinary = %q, %v; want %q", got, err, want)
		}
	})

	t.Run("gopath bin", func(t *testing.T) {
		isolateGoplsEnv(t)
		gopath := t.TempDir()
		t.Setenv("GOPATH", gopath)
		want := writeFakeGopls(t, filepath.Join(gopath, "bin"))
		got, err := findGoplsBinary(workspaceRoot)
		if err != nil || got != want {
			t.Fatalf("findGoplsBinary = %q, %v; want %q", got, err, want)
		}
	})

	t.Run("gobin wins over gopath", func(t *testing.T) {
		isolateGoplsEnv(t)
		gopath := t.TempDir()
		t.Setenv("GOPATH", gopath)
		writeFakeGopls(t, filepath.Join(gopath, "bin"))
		gobin := t.TempDir()
		t.Setenv("GOBIN", gobin)
		want := writeFakeGopls(t, gobin)
		got, err := findGoplsBinary(workspaceRoot)
		if err != nil || got != want {
			t.Fatalf("findGoplsBinary = %q, %v; want %q", got, err, want)
		}
	})

	t.Run("override wins over everything", func(t *testing.T) {
		home := isolateGoplsEnv(t)
		writeFakeGopls(t, filepath.Join(home, "go", "bin"))
		want := writeFakeGopls(t, t.TempDir())
		t.Setenv("MI_LSP_GOPLS_PATH", want)
		got, err := findGoplsBinary(workspaceRoot)
		if err != nil || got != want {
			t.Fatalf("findGoplsBinary = %q, %v; want %q", got, err, want)
		}
	})

	t.Run("missing everywhere", func(t *testing.T) {
		isolateGoplsEnv(t)
		if _, err := findGoplsBinary(workspaceRoot); err == nil || !strings.Contains(err.Error(), "unavailable") {
			t.Fatalf("expected unavailable error, got %v", err)
		}
	})
}
