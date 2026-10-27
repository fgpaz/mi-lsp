package selfupdate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestSeparateInstallationsSameHome(t *testing.T) {
	s := &fixtureSource{err: errors.New("offline")}
	a := fixtureConfig(t, s)
	b := a
	b.ExecutablePath = filepath.Join(a.HomeDir, ".local", "bin", "mi-lsp")
	if err := os.MkdirAll(filepath.Dir(b.ExecutablePath), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b.ExecutablePath, []byte("other"), 0700); err != nil {
		t.Fatal(err)
	}
	a.Readback = func(context.Context, string) (string, error) { return "0.16.0", nil }
	b.Readback = func(context.Context, string) (string, error) { return "0.18.0", nil }
	_, _ = New(a).Apply(context.Background(), false)
	if _, err := New(b).Check(context.Background()); err == nil {
		t.Fatal("second installation inherited first installation's status")
	}
}
func TestUserBinIsNotImplicitPin(t *testing.T) {
	t.Setenv("MI_LSP_CHANNEL", "")
	t.Setenv("MI_LSP_AUTO_UPDATE", "")
	s := &fixtureSource{err: errors.New("offline")}
	c := fixtureConfig(t, s)
	st, _ := New(c).Apply(context.Background(), false)
	if st.LastResult == "pinned" || s.calls != 1 {
		t.Fatalf("bin inferred pinned: %+v, calls=%d", st, s.calls)
	}
}
