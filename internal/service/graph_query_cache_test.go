package service

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
)

func cacheFixtureEntry(t *testing.T, key string) *worktreeGraphCacheEntry {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if err = db.PingContext(context.Background()); err != nil {
		db.Close()
		t.Fatal(err)
	}
	entry := &worktreeGraphCacheEntry{db: db, generation: key}
	t.Cleanup(func() {
		worktreeGraphCache.Lock()
		if worktreeGraphCache.entries[key] == entry {
			delete(worktreeGraphCache.entries, key)
		}
		worktreeGraphCache.Unlock()
		db.Close()
	})
	return entry
}

func TestWorktreeGraphCacheHitAndDifferentIdentity(t *testing.T) {
	key := t.TempDir()
	calls := 0
	firstEntry := cacheFixtureEntry(t, key)
	build := func(context.Context) (*worktreeGraphCacheEntry, error) { calls++; return firstEntry, nil }
	first, state, release, err := acquireWorktreeGraphSnapshot(context.Background(), key, build)
	if err != nil {
		t.Fatal(err)
	}
	release()
	if state != "miss" {
		t.Fatalf("first state=%s", state)
	}
	second, state, release, err := acquireWorktreeGraphSnapshot(context.Background(), key, build)
	if err != nil {
		t.Fatal(err)
	}
	release()
	if first != second || state != "hit" || calls != 1 {
		t.Fatalf("cache not reused: state=%s calls=%d", state, calls)
	}
	nextKey := t.TempDir()
	nextEntry := cacheFixtureEntry(t, nextKey)
	next, state, release, err := acquireWorktreeGraphSnapshot(context.Background(), nextKey, func(context.Context) (*worktreeGraphCacheEntry, error) { calls++; return nextEntry, nil })
	if err != nil {
		t.Fatal(err)
	}
	release()
	if next == first || state != "miss" || calls != 2 {
		t.Fatalf("different identity reused stale snapshot: state=%s calls=%d", state, calls)
	}
}

func TestWorktreeGraphCacheCancelledFlightCannotReplaceRetry(t *testing.T) {
	key := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan struct{})
	releaseOld := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(releaseOld) }) }
	defer unblock()
	result := make(chan error, 1)
	go func() {
		_, _, release, err := acquireWorktreeGraphSnapshot(ctx, key, func(buildCtx context.Context) (*worktreeGraphCacheEntry, error) {
			close(started)
			<-buildCtx.Done()
			<-releaseOld
			return nil, buildCtx.Err()
		})
		if release != nil {
			release()
		}
		result <- err
	}()
	<-started
	worktreeGraphCache.Lock()
	oldFlight := worktreeGraphCache.flights[key]
	worktreeGraphCache.Unlock()
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel err=%v", err)
	}
	freshEntry := cacheFixtureEntry(t, key)
	fresh, state, release, err := acquireWorktreeGraphSnapshot(context.Background(), key, func(context.Context) (*worktreeGraphCacheEntry, error) { return freshEntry, nil })
	if err != nil {
		t.Fatal(err)
	}
	release()
	if state != "miss" {
		t.Fatalf("retry did not get a new flight: %s", state)
	}
	unblock()
	<-oldFlight.done
	reused, state, release, err := acquireWorktreeGraphSnapshot(context.Background(), key, func(context.Context) (*worktreeGraphCacheEntry, error) { return nil, errors.New("fresh snapshot lost") })
	if err != nil {
		t.Fatal(err)
	}
	release()
	if reused != fresh || state != "hit" {
		t.Fatalf("cancelled flight corrupted retry: %s", state)
	}
}
