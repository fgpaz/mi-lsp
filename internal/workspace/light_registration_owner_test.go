package workspace

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func privateLightRegistrationRoot(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), ".light-register-"+strings.Repeat("a", 24))
}

func privateLightRegistrationClaim(t *testing.T, root string) string {
	t.Helper()
	receipt, owner, err := reserveLightRegistrationAt(context.Background(), root)
	if err != nil || !owner {
		t.Fatalf("private claim: owner=%v err=%v", owner, err)
	}
	return receipt
}

func TestLightRegistrationAgedUnknownNeverStolen(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		root := privateLightRegistrationRoot(t)
		if legacy {
			if err := os.Mkdir(root, 0o700); err != nil {
				t.Fatal(err)
			}
		} else {
			privateLightRegistrationClaim(t, root)
		}
		old := time.Now().Add(-3 * time.Hour)
		if err := os.Chtimes(root, old, old); err != nil {
			t.Fatal(err)
		}
		before, err := os.ReadDir(root)
		if err != nil {
			t.Fatal(err)
		}
		receipt, owner, err := reserveLightRegistrationAt(context.Background(), root)
		if err != nil || owner || receipt != root {
			t.Fatalf("aged root stolen: %v %v", owner, err)
		}
		if err := finishLightRegistrationAt(context.Background(), root, errors.New("legacy is not authority")); !errors.Is(err, errLightRegistrationOwnerUnknown) {
			t.Fatalf("legacy finish: %v", err)
		}
		after, err := os.ReadDir(root)
		if err != nil || len(after) != len(before) {
			t.Fatalf("legacy/unknown changed: %v", err)
		}
	}
}

func TestLightRegistrationStaleFinishCannotDeleteOrPublish(t *testing.T) {
	root := privateLightRegistrationRoot(t)
	first := privateLightRegistrationClaim(t, root)
	if err := finishLightRegistrationAt(context.Background(), first, nil); err != nil {
		t.Fatal(err)
	}
	second := privateLightRegistrationClaim(t, root)
	if first == second {
		t.Fatal("generation reused")
	}
	if err := finishLightRegistrationAt(context.Background(), first, errors.New("stale")); !errors.Is(err, errLightRegistrationOwnerUnknown) {
		t.Fatalf("stale finish: %v", err)
	}
	if _, err := os.Stat(second); err != nil {
		t.Fatalf("new owner lost: %v", err)
	}
	if _, err := os.Lstat(root + ".failure"); !os.IsNotExist(err) {
		t.Fatalf("stale failure published: %v", err)
	}
}

func TestLightRegistrationWorkerOwnsGuardThroughFinish(t *testing.T) {
	root := privateLightRegistrationRoot(t)
	receipt := privateLightRegistrationClaim(t, root)
	err := runLightRegistrationAt(context.Background(), receipt, func() error {
		_, owner, err := reserveLightRegistrationAt(context.Background(), root)
		if owner || !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("worker guard not physically held: owner=%v err=%v", owner, err)
		}
		if _, err := os.Stat(receipt); err != nil {
			t.Fatal(err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(root); !os.IsNotExist(err) {
		t.Fatalf("completed generation retained: %v", err)
	}
	if _, err := os.Stat(root + ".guard"); err != nil {
		t.Fatalf("stable guard lost: %v", err)
	}
}

func TestLightRegistrationDuplicateWorkerCannotReplayEndedGeneration(t *testing.T) {
	root := privateLightRegistrationRoot(t)
	receipt := privateLightRegistrationClaim(t, root)
	calls := 0
	register := func() error { calls++; return nil }
	if err := runLightRegistrationAt(context.Background(), receipt, register); err != nil {
		t.Fatal(err)
	}
	if err := runLightRegistrationAt(context.Background(), receipt, register); !errors.Is(err, errLightRegistrationOwnerUnknown) {
		t.Fatalf("duplicate result: %v", err)
	}
	if calls != 1 {
		t.Fatalf("registry work replayed %d times", calls)
	}
}

func TestLightRegistrationUnexpectedEntriesRetainGeneration(t *testing.T) {
	root := privateLightRegistrationRoot(t)
	receipt := privateLightRegistrationClaim(t, root)
	if err := os.WriteFile(filepath.Join(root, "unexpected"), []byte("retain"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := finishLightRegistrationAt(context.Background(), receipt, nil); !errors.Is(err, errLightRegistrationOwnerUnknown) {
		t.Fatalf("unexpected entries accepted: %v", err)
	}
	if _, err := os.Stat(receipt); err != nil {
		t.Fatalf("generation removed: %v", err)
	}
}

func TestLightRegistrationFailuresGuardedAndRegistryUntouched(t *testing.T) {
	root := privateLightRegistrationRoot(t)
	registry := filepath.Join(filepath.Dir(root), "registry.toml")
	if err := os.WriteFile(registry, []byte("private unchanged registry"), 0o600); err != nil {
		t.Fatal(err)
	}
	first := privateLightRegistrationClaim(t, root)
	failure := errors.New("private registration failure")
	if err := runLightRegistrationAt(context.Background(), first, func() error { return failure }); !errors.Is(err, failure) {
		t.Fatalf("failure lost: %v", err)
	}
	second := privateLightRegistrationClaim(t, root)
	if got := lightRegistrationFailureAt(context.Background(), root); got != failure.Error() {
		t.Fatalf("failure=%q", got)
	}
	if got := lightRegistrationFailureAt(context.Background(), root); got != "" {
		t.Fatalf("failure replay=%q", got)
	}
	if _, err := os.Stat(second); err != nil {
		t.Fatalf("failure reader removed active generation: %v", err)
	}
	got, err := os.ReadFile(registry)
	if err != nil || string(got) != "private unchanged registry" {
		t.Fatalf("unintended registry write: %v", err)
	}
}

func TestLightRegistrationStableGuardIdentityAcrossGenerations(t *testing.T) {
	root := privateLightRegistrationRoot(t)
	first := privateLightRegistrationClaim(t, root)
	before, err := os.Stat(root + ".guard")
	if err != nil {
		t.Fatal(err)
	}
	if err := finishLightRegistrationAt(context.Background(), first, nil); err != nil {
		t.Fatal(err)
	}
	privateLightRegistrationClaim(t, root)
	after, err := os.Stat(root + ".guard")
	if err != nil || !os.SameFile(before, after) {
		t.Fatalf("guard inode changed: %v", err)
	}
}

func TestLightRegistrationConcurrentClaimsHaveSingleOwner(t *testing.T) {
	root := privateLightRegistrationRoot(t)
	start := make(chan struct{})
	type result struct {
		owner bool
		err   error
	}
	out := make(chan result, 8)
	var group sync.WaitGroup
	for i := 0; i < 8; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			_, owner, err := reserveLightRegistrationAt(context.Background(), root)
			out <- result{owner, err}
		}()
	}
	close(start)
	group.Wait()
	close(out)
	owners := 0
	for r := range out {
		if r.owner {
			owners++
		}
		if r.err != nil && !errors.Is(r.err, context.DeadlineExceeded) {
			t.Fatal(r.err)
		}
	}
	if owners != 1 {
		t.Fatalf("owners=%d", owners)
	}
}

func TestLightRegistrationPreCanceledGuardHasNoEffects(t *testing.T) {
	root := privateLightRegistrationRoot(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, owner, err := reserveLightRegistrationAt(ctx, root)
	if owner || !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled claim owner=%v err=%v", owner, err)
	}
	for _, path := range []string{root, root + ".guard", root + ".failure"} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("canceled mutation: %v", err)
		}
	}
}
