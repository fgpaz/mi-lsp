package selfupdate

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type fixtureSource struct {
	release          Release
	err              error
	calls, downloads int
}

func (s *fixtureSource) Latest(context.Context) (Release, error) { s.calls++; return s.release, s.err }
func (s *fixtureSource) Download(context.Context, Asset, string) error {
	s.downloads++
	return errors.New("fixture download failure")
}
func fixtureConfig(t *testing.T, source *fixtureSource) Config {
	t.Helper()
	home := t.TempDir()
	exe := filepath.Join(home, "bin", "mi-lsp")
	if err := os.MkdirAll(filepath.Dir(exe), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exe, []byte("old"), 0700); err != nil {
		t.Fatal(err)
	}
	return Config{HomeDir: home, ExecutablePath: exe, Version: "0.16.0", MinimumVersion: "0.16.0", Now: func() time.Time { return time.Date(2026, 10, 27, 19, 0, 0, 0, time.UTC) }, Source: source, Readback: func(context.Context, string) (string, error) { return "0.16.0", nil }}
}
func TestStatusPublicShape(t *testing.T) {
	b, err := json.Marshal(Status{})
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err = json.Unmarshal(b, &fields); err != nil {
		t.Fatal(err)
	}
	names := []string{"schema", "tool", "installed", "latest", "last_check", "last_result", "code", "previous", "channel", "auto"}
	if len(fields) != len(names) {
		t.Fatalf("fields: %s", b)
	}
	for _, name := range names {
		if _, ok := fields[name]; !ok {
			t.Errorf("missing %s", name)
		}
	}
}
func TestCheckAbsentIsReadonly(t *testing.T) {
	s := &fixtureSource{}
	c := fixtureConfig(t, s)
	_, err := New(c).Check(context.Background())
	if err == nil {
		t.Fatal("missing state must fail")
	}
	if s.calls != 0 || s.downloads != 0 {
		t.Fatal("check contacted source")
	}
	entries, e := os.ReadDir(c.HomeDir)
	if e != nil {
		t.Fatal(e)
	}
	if len(entries) != 1 || entries[0].Name() != "bin" {
		t.Fatalf("check wrote home: %v", entries)
	}
}
func TestApplyRejectedCandidates(t *testing.T) {
	for _, tc := range []struct {
		name, version, code string
		failure             error
	}{{"downgrade", "0.15.9", "UPDATE_DOWNGRADE_REFUSED", nil}, {"floor", "0.15.9", "UPDATE_BELOW_FLOOR", nil}, {"network", "", "UPDATE_NETWORK_UNAVAILABLE", errors.New("offline")}, {"no release", "0.17.0", "UPDATE_NO_RELEASE", nil}} {
		t.Run(tc.name, func(t *testing.T) {
			s := &fixtureSource{release: Release{Version: tc.version}, err: tc.failure}
			c := fixtureConfig(t, s)
			if tc.name == "floor" {
				c.Version = "0.15.0"
				c.Readback = func(context.Context, string) (string, error) { return "0.15.0", nil }
			}
			st, _ := New(c).Apply(context.Background(), false)
			if st.Code == nil || *st.Code != tc.code {
				t.Fatalf("code=%v want %s", st.Code, tc.code)
			}
			b, e := os.ReadFile(c.ExecutablePath)
			if e != nil || string(b) != "old" {
				t.Fatalf("executable changed: %q %v", b, e)
			}
		})
	}
}
func TestAutomaticNetworkFailureThrottled(t *testing.T) {
	s := &fixtureSource{err: errors.New("offline")}
	c := fixtureConfig(t, s)
	u := New(c)
	_, _ = u.Apply(context.Background(), true)
	_, _ = u.Apply(context.Background(), true)
	if s.calls != 1 {
		t.Fatalf("network calls=%d", s.calls)
	}
}
func TestExplicitPolicy(t *testing.T) {
	for _, tc := range []struct{ name, key, value, want string }{{"pin", "MI_LSP_CHANNEL", "pinned", "pinned"}, {"optout", "MI_LSP_AUTO_UPDATE", "0", "disabled"}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("MI_LSP_CHANNEL", "")
			t.Setenv("MI_LSP_AUTO_UPDATE", "")
			t.Setenv(tc.key, tc.value)
			s := &fixtureSource{}
			c := fixtureConfig(t, s)
			st, _ := New(c).Apply(context.Background(), false)
			if st.LastResult != tc.want || s.calls != 0 {
				t.Fatalf("result=%s calls=%d", st.LastResult, s.calls)
			}
		})
	}
}
