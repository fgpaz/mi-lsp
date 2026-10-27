package selfupdate

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/fgpaz/mi-lsp/internal/processutil"
	"github.com/fgpaz/mi-lsp/internal/runtimeinfo"
)

func snapshotHome(t *testing.T, root string) map[string]string {
	t.Helper()
	result := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		info, e := d.Info()
		if e != nil {
			return e
		}
		rel, e := filepath.Rel(root, path)
		if e != nil {
			return e
		}
		v := fmt.Sprintf("%v:%d:%d", info.Mode(), info.Size(), info.ModTime().UnixNano())
		if !d.IsDir() {
			b, e := os.ReadFile(path)
			if e != nil {
				return e
			}
			v += fmt.Sprintf(":%x", sha256.Sum256(b))
		}
		result[rel] = v
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}
func TestCachedCheckNeverWritesOrContactsSource(t *testing.T) {
	for _, age := range []time.Duration{time.Hour, 25 * time.Hour} {
		t.Run(age.String(), func(t *testing.T) {
			s := &fixtureSource{release: Release{Version: "0.16.0"}}
			c := fixtureConfig(t, s)
			now := c.Now()
			c.Now = func() time.Time { return now }
			first, err := New(c).Apply(context.Background(), false)
			if err != nil || first.LastResult != "up_to_date" {
				t.Fatalf("initial status=%+v err=%v", first, err)
			}
			before := snapshotHome(t, c.HomeDir)
			s.err = errors.New("must not call source")
			s.calls = 0
			c.Now = func() time.Time { return now.Add(age) }
			got, err := New(c).Check(context.Background())
			if err != nil || got.LastResult != "up_to_date" || got.Installed != "0.16.0" || got.LastCheck == nil || first.LastCheck == nil || !got.LastCheck.Equal(*first.LastCheck) {
				t.Fatalf("cached=%+v err=%v", got, err)
			}
			if s.calls != 0 || s.downloads != 0 {
				t.Fatal("check called source")
			}
			if after := snapshotHome(t, c.HomeDir); !reflect.DeepEqual(before, after) {
				t.Fatalf("check changed home: before=%v after=%v", before, after)
			}
		})
	}
}
func TestAutomaticThrottleBoundariesPersistAcrossInstances(t *testing.T) {
	for _, tc := range []struct {
		age  time.Duration
		want int
	}{{24*time.Hour - time.Nanosecond, 1}, {24 * time.Hour, 2}, {24*time.Hour + time.Second, 2}} {
		t.Run(tc.age.String(), func(t *testing.T) {
			s := &fixtureSource{err: errors.New("offline")}
			c := fixtureConfig(t, s)
			now := c.Now()
			c.Now = func() time.Time { return now }
			_, _ = New(c).Apply(context.Background(), true)
			c.Now = func() time.Time { return now.Add(tc.age) }
			_, _ = New(c).Apply(context.Background(), true)
			if s.calls != tc.want {
				t.Fatalf("calls=%d want=%d", s.calls, tc.want)
			}
		})
	}
}
func TestCheckReadbackOverridesOldProcessVersion(t *testing.T) {
	s := &fixtureSource{release: Release{Version: "0.16.0"}}
	c := fixtureConfig(t, s)
	_, _ = New(c).Apply(context.Background(), false)
	c.Readback = func(context.Context, string) (string, error) { return "0.18.0", nil }
	st, err := New(c).Check(context.Background())
	if err == nil && st.Installed != "0.18.0" {
		t.Fatalf("attributed old process to disk: %+v", st)
	}
}

// The same test executable holds the production OS lock; TestMain from transaction tests remains compatible.
func TestA15CoreLockHelper(t *testing.T) {
	if os.Getenv("A15_CORE_LOCK_HELPER") != "1" {
		return
	}
	path := os.Getenv("A15_CORE_LOCK_PATH")
	ready := os.Getenv("A15_CORE_LOCK_READY")
	release := os.Getenv("A15_CORE_LOCK_RELEASE")
	err := runtimeinfo.WithFileLock(path, func() error {
		if e := os.WriteFile(ready, []byte("ready"), 0600); e != nil {
			return e
		}
		deadline := time.NewTimer(10 * time.Second)
		defer deadline.Stop()
		tick := time.NewTicker(10 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-deadline.C:
				return errors.New("lock helper timeout")
			case <-tick.C:
				if _, e := os.Stat(release); e == nil {
					return nil
				}
			}
		}
	})
	if err != nil {
		t.Fatal(err)
	}
}
func TestApplyLockAcrossProcesses(t *testing.T) {
	s := &fixtureSource{}
	c := fixtureConfig(t, s)
	ready := filepath.Join(c.HomeDir, "ready")
	release := filepath.Join(c.HomeDir, "release")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, "-test.run=^TestA15CoreLockHelper$")
	cmd.Env = append(os.Environ(), "A15_CORE_LOCK_HELPER=1", "A15_CORE_LOCK_PATH="+updateLockPath(c.HomeDir), "A15_CORE_LOCK_READY="+ready, "A15_CORE_LOCK_RELEASE="+release)
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := false
	defer func() {
		if !done {
			_ = os.WriteFile(release, []byte("release"), 0600)
			_ = cmd.Wait()
		}
	}()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		if _, e := os.Stat(ready); e == nil {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("helper did not acquire lock")
		case <-ticker.C:
		}
	}
	before := snapshotHome(t, c.HomeDir)
	st, _ := New(c).Apply(context.Background(), false)
	if st.LastResult != "deferred" || st.Code == nil || *st.Code != "UPDATE_LOCKED" || s.calls != 0 {
		t.Fatalf("status=%+v calls=%d", st, s.calls)
	}
	if after := snapshotHome(t, c.HomeDir); !reflect.DeepEqual(before, after) {
		t.Fatal("lock competitor wrote state")
	}
	if err = os.WriteFile(release, []byte("release"), 0600); err != nil {
		t.Fatal(err)
	}
	err = cmd.Wait()
	done = true
	if err != nil {
		t.Fatal(err)
	}
}

// startAutomaticChild is the private scheduler injection point; the real factory
// must produce the very command passed here, not an unused test-only helper.
func TestMaybeStartAutomaticChildFactory(t *testing.T) {
	t.Setenv("MI_LSP_CHANNEL", "")
	t.Setenv("MI_LSP_AUTO_UPDATE", "")
	t.Setenv("MI_LSP_UPDATE_CHILD", "")
	s := &fixtureSource{release: Release{Version: "0.16.0"}}
	c := fixtureConfig(t, s)
	original := startAutomaticChild
	defer func() { startAutomaticChild = original }()
	var received *exec.Cmd
	calls := 0
	startAutomaticChild = func(cmd *exec.Cmd) error { calls++; received = cmd; return nil }
	started, err := MaybeStartAutomatic(c, false)
	if err != nil || !started || calls != 1 {
		t.Fatalf("started=%v calls=%d err=%v", started, calls, err)
	}
	if received == nil || received.Path != c.ExecutablePath || !reflect.DeepEqual(received.Args, []string{c.ExecutablePath, "update"}) {
		t.Fatalf("command=%+v", received)
	}
	marker := false
	for _, v := range received.Env {
		if v == "MI_LSP_UPDATE_CHILD=1" {
			marker = true
		}
		if strings.HasPrefix(v, "MI_LSP_UPDATE_CHILD=") && v != "MI_LSP_UPDATE_CHILD=1" {
			t.Fatalf("conflicting marker %q", v)
		}
	}
	if !marker {
		t.Fatal("missing child marker")
	}
	if received.Stdin != nil || received.Stdout != nil || received.Stderr != nil {
		t.Fatal("child must have disconnected streams")
	}
	expected := processutil.Command(c.ExecutablePath, "update")
	processutil.ConfigureDetachedCommand(expected)
	if !reflect.DeepEqual(received.SysProcAttr, expected.SysProcAttr) {
		t.Fatalf("detached attributes=%+v want=%+v", received.SysProcAttr, expected.SysProcAttr)
	}
	if s.calls != 0 || s.downloads != 0 {
		t.Fatal("scheduling contacted source")
	}
}
func TestMaybeStartAutomaticSkipsDaemonAndFreshCache(t *testing.T) {
	t.Setenv("MI_LSP_CHANNEL", "")
	t.Setenv("MI_LSP_AUTO_UPDATE", "")
	t.Setenv("MI_LSP_UPDATE_CHILD", "")
	s := &fixtureSource{release: Release{Version: "0.16.0"}}
	c := fixtureConfig(t, s)
	original := startAutomaticChild
	defer func() { startAutomaticChild = original }()
	calls := 0
	startAutomaticChild = func(*exec.Cmd) error { calls++; return nil }
	started, err := MaybeStartAutomatic(c, true)
	if err != nil || started || calls != 0 {
		t.Fatalf("daemon started=%v calls=%d err=%v", started, calls, err)
	}
	if _, err = New(c).Apply(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	sourceCalls := s.calls
	started, err = MaybeStartAutomatic(c, false)
	if err != nil || started || calls != 0 || s.calls != sourceCalls {
		t.Fatalf("fresh cache started=%v starts=%d source=%d err=%v", started, calls, s.calls, err)
	}
	c.Now = func() time.Time { return time.Date(2026, 10, 28, 19, 0, 1, 0, time.UTC) }
	started, err = MaybeStartAutomatic(c, false)
	if err != nil || !started || calls != 1 || s.calls != sourceCalls {
		t.Fatalf("expired cache started=%v starts=%d source=%d err=%v", started, calls, s.calls, err)
	}
}
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
