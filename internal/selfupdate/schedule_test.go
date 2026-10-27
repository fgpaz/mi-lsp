package selfupdate

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
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

func snapshotHome(t *testing.T, root string, lockedEmptyFile ...string) map[string]string {
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
			var b []byte
			if len(lockedEmptyFile) == 1 && filepath.Clean(path) == filepath.Clean(lockedEmptyFile[0]) {
				// Windows byte-range locks deny ReadFile even for this empty OS
				// lock. Size zero proves its empty hash without weakening snapshots.
				if info.Size() != 0 {
					return fmt.Errorf("OS lock is not empty")
				}
			} else {
				b, e = os.ReadFile(path)
				if e != nil {
					return e
				}
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
	before := snapshotHome(t, c.HomeDir, updateLockPath(c.HomeDir)+".lock")
	st, _ := New(c).Apply(context.Background(), false)
	if st.LastResult != "deferred" || st.Code == nil || *st.Code != "UPDATE_LOCKED" || s.calls != 0 {
		t.Fatalf("status=%+v calls=%d", st, s.calls)
	}
	if after := snapshotHome(t, c.HomeDir, updateLockPath(c.HomeDir)+".lock"); !reflect.DeepEqual(before, after) {
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

// cachePair models two installations with one home and real per-file readback.
func cachePair(t *testing.T, source Source) (Config, Config) {
	t.Helper()
	a := fixtureConfig(t, &fixtureSource{})
	a.Source = source
	b := a
	b.ExecutablePath = filepath.Join(a.HomeDir, ".local", "bin", "mi-lsp")
	if err := os.MkdirAll(filepath.Dir(b.ExecutablePath), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b.ExecutablePath, []byte("old"), 0700); err != nil {
		t.Fatal(err)
	}
	read := func(_ context.Context, p string) (string, error) {
		data, err := os.ReadFile(p)
		if err != nil {
			return "", err
		}
		switch string(data) {
		case "old":
			return "0.16.0", nil
		case "new":
			return "0.17.0", nil
		}
		return "", errors.New("unknown fixture bytes")
	}
	a.Readback = read
	b.Readback = read
	return a, b
}
func assertUpdatedFromCache(t *testing.T, c Config, st Status, when time.Time) {
	t.Helper()
	if st.LastResult != "updated" || st.Installed != "0.17.0" || st.Latest != "0.17.0" || st.Previous == nil || *st.Previous != "0.16.0" || st.Code != nil || st.LastCheck == nil || !st.LastCheck.Equal(when) {
		t.Fatalf("bad updated status: %+v", st)
	}
	data, err := os.ReadFile(c.ExecutablePath)
	if err != nil || string(data) != "new" {
		t.Fatalf("disk=%q error=%v", data, err)
	}
	cached, err := ReadCached(c)
	if err != nil || cached.Installed != "0.17.0" || cached.Previous == nil || *cached.Previous != "0.16.0" {
		t.Fatalf("state=%+v err=%v", cached, err)
	}
}
func TestAutomaticSharedReleaseAppliesEachInstallation(t *testing.T) {
	t.Setenv("MI_LSP_CHANNEL", "")
	t.Setenv("MI_LSP_AUTO_UPDATE", "")
	for _, oldState := range []bool{false, true} {
		t.Run(fmt.Sprintf("prior_state_%v", oldState), func(t *testing.T) {
			src := newReleaseFixture(t, false)
			a, b := cachePair(t, src)
			t0 := a.Now()
			checkAt := t0
			if oldState {
				old := &fixtureSource{release: Release{Version: "0.16.0"}}
				b.Source = old
				b.Now = func() time.Time { return t0.Add(-25 * time.Hour) }
				st, err := New(b).Apply(context.Background(), false)
				if err != nil || st.LastResult != "up_to_date" {
					t.Fatalf("old state=%+v err=%v", st, err)
				}
				b.Source = src
			}
			first, err := New(a).Apply(context.Background(), true)
			if err != nil {
				t.Fatalf("first apply: %v", err)
			}
			assertUpdatedFromCache(t, a, first, checkAt)
			b.Now = func() time.Time { return t0.Add(time.Hour) }
			second, err := New(b).Apply(context.Background(), true)
			if err != nil {
				t.Fatalf("second apply: %v", err)
			}
			assertUpdatedFromCache(t, b, second, checkAt)
			if src.calls != 1 {
				t.Fatalf("Latest calls=%d, want 1", src.calls)
			}
			pa, err := statePath(a)
			if err != nil {
				t.Fatal(err)
			}
			pb, err := statePath(b)
			if err != nil {
				t.Fatal(err)
			}
			if pa == pb {
				t.Fatal("installations share state file")
			}
		})
	}
}
func TestAutomaticSharedNetworkFailurePersistsForSecondInstallation(t *testing.T) {
	t.Setenv("MI_LSP_CHANNEL", "")
	t.Setenv("MI_LSP_AUTO_UPDATE", "")
	src := &fixtureSource{err: errors.New("offline")}
	a, b := cachePair(t, src)
	now := a.Now()
	first, _ := New(a).Apply(context.Background(), true)
	if first.LastResult != "failed" {
		t.Fatalf("first=%+v", first)
	}
	b.Now = func() time.Time { return now.Add(time.Hour) }
	second, err := New(b).Apply(context.Background(), true)
	if err != nil || second.LastResult != "failed" || second.Code == nil || *second.Code != "UPDATE_NETWORK_UNAVAILABLE" || second.Installed != "0.16.0" || second.Previous != nil || second.LastCheck == nil || !second.LastCheck.Equal(now) || src.calls != 1 {
		t.Fatalf("second=%+v calls=%d err=%v", second, src.calls, err)
	}
	cached, err := ReadCached(b)
	if err != nil || cached.LastResult != "failed" || cached.Installed != "0.16.0" {
		t.Fatalf("persisted=%+v err=%v", cached, err)
	}
	data, err := os.ReadFile(b.ExecutablePath)
	if err != nil || string(data) != "old" {
		t.Fatalf("disk=%q err=%v", data, err)
	}
}
func TestMaybeStartAutomaticForOtherInstallationWithFreshRelease(t *testing.T) {
	t.Setenv("MI_LSP_CHANNEL", "")
	t.Setenv("MI_LSP_AUTO_UPDATE", "")
	t.Setenv("MI_LSP_UPDATE_CHILD", "")
	src := newReleaseFixture(t, false)
	a, b := cachePair(t, src)
	now := a.Now()
	if _, err := New(a).Apply(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	b.Now = func() time.Time { return now.Add(time.Hour) }
	original := startAutomaticChild
	defer func() { startAutomaticChild = original }()
	calls := 0
	startAutomaticChild = func(cmd *exec.Cmd) error {
		calls++
		if cmd.Path != b.ExecutablePath {
			t.Errorf("wrong executable")
		}
		return nil
	}
	started, err := MaybeStartAutomatic(b, false)
	if err != nil || !started || calls != 1 || src.calls != 1 {
		t.Fatalf("started=%v starts=%d latest=%d err=%v", started, calls, src.calls, err)
	}
}
func TestAutomaticBootstrapBuildMetadataNotDevOrPinned(t *testing.T) {
	t.Setenv("MI_LSP_CHANNEL", "")
	t.Setenv("MI_LSP_AUTO_UPDATE", "")
	t.Setenv("MI_LSP_UPDATE_CHILD", "")
	src := &fixtureSource{}
	c := fixtureConfig(t, src)
	c.Version = "0.15.6+a15bootstrap"
	original := startAutomaticChild
	defer func() { startAutomaticChild = original }()
	calls := 0
	startAutomaticChild = func(*exec.Cmd) error { calls++; return nil }
	started, err := MaybeStartAutomatic(c, false)
	if err != nil || !started || calls != 1 || src.calls != 0 {
		t.Fatalf("started=%v calls=%d latest=%d err=%v", started, calls, src.calls, err)
	}
}

type guardedCachedSource struct{ downloads, latest int }

func (s *guardedCachedSource) Latest(context.Context) (Release, error) {
	s.latest++
	return Release{}, errors.New("unexpected metadata call")
}
func (s *guardedCachedSource) Download(context.Context, Asset, string) error {
	s.downloads++
	return errors.New("unexpected download")
}
func TestCachedReleaseValidatedBeforeUse(t *testing.T) {
	t.Setenv("MI_LSP_CHANNEL", "")
	t.Setenv("MI_LSP_AUTO_UPDATE", "")
	for _, tc := range []struct {
		name   string
		change func(*Release)
	}{
		{"bad semver", func(r *Release) { r.Version = "not-semver" }},
		{"below floor", func(r *Release) { r.Version = "0.15.5" }},
		{"downgrade", func(r *Release) { r.Version = "0.15.9" }},
		{"untrusted asset URL", func(r *Release) { r.Assets[0].URL = "https://evil.invalid/asset" }},
		{"oversized asset", func(r *Release) { r.Assets[0].Size = 1 << 40 }},
		{"negative asset size", func(r *Release) { r.Assets[0].Size = -1 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := &guardedCachedSource{}
			a, b := cachePair(t, src)
			if tc.name == "below floor" {
				b.Readback = func(context.Context, string) (string, error) { return "0.15.0", nil }
			}
			fixture := newReleaseFixture(t, false)
			release := fixture.release
			tc.change(&release)
			now := a.Now()
			if err := runtimeinfo.WriteJSONAtomic(cachePath(a), updateCache{LastCheck: now, Release: release}); err != nil {
				t.Fatal(err)
			}
			b.Now = func() time.Time { return now.Add(time.Hour) }
			st, _ := New(b).Apply(context.Background(), true)
			if st.LastResult == "updated" || src.latest != 0 || src.downloads != 0 {
				t.Fatalf("unsafe cached candidate used: %+v latest=%d downloads=%d", st, src.latest, src.downloads)
			}
			data, err := os.ReadFile(b.ExecutablePath)
			if err != nil || string(data) != "old" {
				t.Fatalf("disk=%q err=%v", data, err)
			}
		})
	}
}
func TestMaybeStartAutomaticForStaleInstallationState(t *testing.T) {
	t.Setenv("MI_LSP_CHANNEL", "")
	t.Setenv("MI_LSP_AUTO_UPDATE", "")
	t.Setenv("MI_LSP_UPDATE_CHILD", "")
	src := newReleaseFixture(t, false)
	a, b := cachePair(t, src)
	now := a.Now()
	old := &fixtureSource{release: Release{Version: "0.16.0"}}
	b.Source = old
	b.Now = func() time.Time { return now.Add(-25 * time.Hour) }
	if _, err := New(b).Apply(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	b.Source = src
	if _, err := New(a).Apply(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	b.Now = func() time.Time { return now.Add(time.Hour) }
	original := startAutomaticChild
	defer func() { startAutomaticChild = original }()
	calls := 0
	startAutomaticChild = func(*exec.Cmd) error { calls++; return nil }
	started, err := MaybeStartAutomatic(b, false)
	if err != nil || !started || calls != 1 || src.calls != 1 {
		t.Fatalf("started=%v starts=%d latest=%d err=%v", started, calls, src.calls, err)
	}
}

// openStateJSON is the private file-opening seam used by readJSON; this verifies
// the real state/cache reader stops before consuming an oversized body.
type countedReader struct {
	remaining int64
	read      int64
}

func (r *countedReader) Read(p []byte) (int, error) {
	if r.remaining == 0 {
		return 0, io.EOF
	}
	n := int64(len(p))
	if n > r.remaining {
		n = r.remaining
	}
	for i := 0; i < int(n); i++ {
		p[i] = ' '
	}
	r.remaining -= n
	r.read += n
	return int(n), nil
}
func (r *countedReader) Close() error { return nil }
func TestReadJSONRejectsOversizedBeforeReadingBody(t *testing.T) {
	r := &countedReader{remaining: 20 << 20}
	original := openStateJSON
	defer func() { openStateJSON = original }()
	openStateJSON = func(string) (io.ReadCloser, error) { return r, nil }
	var value map[string]any
	err := readJSON(filepath.Join(t.TempDir(), "state.json"), &value)
	if err == nil {
		t.Fatal("oversized state accepted")
	}
	if r.read > 2<<20+1 {
		t.Fatalf("read %d bytes before rejecting oversized state", r.read)
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
