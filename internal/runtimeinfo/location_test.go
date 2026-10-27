package runtimeinfo

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func isolatedHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", home)
		t.Setenv("LOCALAPPDATA", filepath.Join(home, "local"))
	} else {
		t.Setenv("HOME", home)
		t.Setenv("XDG_DATA_HOME", filepath.Join(home, "data"))
	}
	return home
}

func TestLocateReadOnly(t *testing.T) {
	home := isolatedHome(t)
	location := Locate()
	base := filepath.Join(home, "data")
	name := "mi-lsp"
	if runtime.GOOS == "windows" {
		base = filepath.Join(home, "local")
		name += ".exe"
	}
	if location.ManagedExecutablePath != filepath.Join(base, "mi-lsp-runtime", name) || location.ReceiptPath != filepath.Join(home, ".mi-lsp", "repair", "installations.json") || !filepath.IsAbs(location.ExecutablePath) {
		t.Fatalf("location: %+v", location)
	}
	if _, err := os.Stat(filepath.Join(home, ".mi-lsp")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("location must not create state: %v", err)
	}
}

func TestRecordRemovedAndInvalidReceipts(t *testing.T) {
	isolatedHome(t)
	removed, err := Removed()
	if err != nil || len(removed) != 0 {
		t.Fatalf("removed=%v err=%v", removed, err)
	}
	file := filepath.Join(t.TempDir(), "worker")
	if err := os.WriteFile(file, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Record("worker", file); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	removed, err = Removed()
	if err != nil || len(removed) != 1 || removed[0].Path != file {
		t.Fatalf("removed=%v err=%v", removed, err)
	}
	if err := os.WriteFile(file, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	bad := []byte(`{"schema":"wrong","installations":[]}`)
	path := Locate().ReceiptPath
	if err := os.WriteFile(path, bad, 0600); err != nil {
		t.Fatal(err)
	}
	if err := Record("worker", file); err == nil {
		t.Fatal("invalid receipts accepted")
	}
	if _, err := Removed(); err == nil {
		t.Fatal("invalid receipts accepted on read")
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != string(bad) {
		t.Fatalf("invalid receipt overwritten: %q %v", got, err)
	}
	if err := Record("other", file); err == nil {
		t.Fatal("invalid component accepted")
	}
	if err := Record("worker", "relative"); err == nil {
		t.Fatal("relative path accepted")
	}
}

func TestLockExcludesConcurrentWriter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	entered := make(chan struct{})
	release := make(chan struct{})
	result := make(chan error, 1)
	go func() { result <- WithFileLock(path, func() error { close(entered); <-release; return nil }) }()
	<-entered
	err := WithFileLock(path, func() error { return nil })
	close(release)
	if err == nil {
		t.Fatal("concurrent writer acquired lock")
	}
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	if err := WithFileLock(path, func() error { return errors.New("callback") }); err == nil || !strings.Contains(err.Error(), "callback") {
		t.Fatalf("callback error: %v", err)
	}
	if err := WithFileLock(path, func() error { return nil }); err != nil {
		t.Fatalf("lock not released: %v", err)
	}
}
