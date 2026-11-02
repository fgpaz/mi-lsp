package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func TestUpdateOwnerCacheConcurrencyAndInvalidation(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir, err := daemonRootDir()
	if err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(dir, "state.json")
	if err := os.WriteFile(statePath, []byte("one"), 0600); err != nil {
		t.Fatal(err)
	}
	var probes, dials atomic.Int32
	expected := model.DaemonState{ExecutablePath: "/opt/mi-lsp", Version: "0.16.6"}
	probe := func(ctx context.Context) (model.DaemonState, error) {
		probes.Add(1)
		time.Sleep(25 * time.Millisecond)
		return expected, nil
	}
	dial := func(context.Context) error { dials.Add(1); return nil }
	var wg sync.WaitGroup
	errs := make(chan error, 24)
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			got, err := cachedUpdateOwnerWith(ctx, probe, dial)
			if err != nil {
				errs <- err
				return
			}
			if got.ExecutablePath != expected.ExecutablePath || got.Version != expected.Version {
				errs <- errors.New("wrong owner")
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	if got := probes.Load(); got != 1 {
		t.Fatalf("probes under contention: %d", got)
	}
	if got := dials.Load(); got != 23 {
		t.Fatalf("fresh dials: %d", got)
	}
	cache := filepath.Join(dir, "update-owner-cache.json")
	info, err := os.Stat(cache)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("cache permissions: %v", info.Mode())
	}
	body, err := os.ReadFile(cache)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err = json.Unmarshal(body, &fields); err != nil {
		t.Fatal(err)
	}
	if len(fields) != 5 {
		t.Fatalf("unexpected cached fields: %v", fields)
	}
	if err = os.WriteFile(statePath, []byte("changed-state"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = cachedUpdateOwnerWith(context.Background(), probe, dial); err != nil {
		t.Fatal(err)
	}
	if probes.Load() != 2 {
		t.Fatalf("state change not invalidated: %d", probes.Load())
	}
	body, err = os.ReadFile(cache)
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(body, &fields); err != nil {
		t.Fatal(err)
	}
	fields["checked"] = time.Now().Add(-71 * time.Second)
	body, err = json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(cache, body, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = cachedUpdateOwnerWith(context.Background(), probe, dial); err != nil {
		t.Fatal(err)
	}
	if probes.Load() != 3 {
		t.Fatalf("expiry not invalidated: %d", probes.Load())
	}
	broken := func(context.Context) error { return errors.New("dial failure") }
	failed := func(context.Context) (model.DaemonState, error) {
		return model.DaemonState{}, errors.New("probe failure")
	}
	if _, err = cachedUpdateOwnerWith(context.Background(), failed, broken); err == nil {
		t.Fatal("dial/probe failure produced fake owner")
	}
}

func TestUpdateOwnerLockCancellation(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir, err := daemonRootDir()
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(filepath.Join(dir, "update-owner.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	ok, err := tryLockStartGuard(f)
	if err != nil || !ok {
		t.Fatalf("lock: %v %v", ok, err)
	}
	defer unlockStartGuard(f)
	var probes atomic.Int32
	probe := func(context.Context) (model.DaemonState, error) { probes.Add(1); return model.DaemonState{}, nil }
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_, err = cachedUpdateOwnerWith(ctx, probe, func(context.Context) error { return nil })
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("want deadline: %v", err)
	}
	if probes.Load() != 0 {
		t.Fatalf("lock contention caused probes: %d", probes.Load())
	}
}
