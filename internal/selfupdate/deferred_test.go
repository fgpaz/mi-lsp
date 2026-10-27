package selfupdate

import (
	"context"
	"os"
	"os/exec"
	"testing"
)

// A deferral consumes no new metadata request, but must not suppress the idle retry.
func TestDeferredUpdateRetriesUsingFreshMetadataWhenIdle(t *testing.T) {
	t.Setenv("MI_LSP_CHANNEL", "")
	t.Setenv("MI_LSP_AUTO_UPDATE", "")
	t.Setenv("MI_LSP_UPDATE_CHILD", "")
	src := newReleaseFixture(t, false)
	c, _ := cachePair(t, src)
	backup := c.ExecutablePath + ".old"
	if err := os.WriteFile(backup, []byte("fixture-owned busy backup"), 0600); err != nil {
		t.Fatal(err)
	}
	first, err := New(c).Apply(context.Background(), true)
	if err != nil || first.LastResult != "deferred" || first.Code == nil || *first.Code != "UPDATE_IN_USE_DEFERRED" {
		t.Fatalf("first=%+v err=%v", first, err)
	}
	original := startAutomaticChild
	defer func() { startAutomaticChild = original }()
	starts := 0
	startAutomaticChild = func(*exec.Cmd) error { starts++; return nil }
	started, err := MaybeStartAutomatic(c, false)
	if err != nil || !started || starts != 1 {
		t.Errorf("idle retry not scheduled: started=%v starts=%d err=%v", started, starts, err)
	}
	// Remove only this test's own placeholder, modeling the busy condition ending.
	if err := os.Remove(backup); err != nil {
		t.Fatal(err)
	}
	second, err := New(c).Apply(context.Background(), true)
	if err != nil || second.LastResult != "updated" || second.Installed != "0.17.0" || second.Previous == nil || *second.Previous != "0.16.0" {
		t.Fatalf("idle retry=%+v err=%v", second, err)
	}
	if src.calls != 1 || first.LastCheck == nil || second.LastCheck == nil || !first.LastCheck.Equal(*second.LastCheck) {
		t.Fatalf("retry rechecked metadata: calls=%d first=%+v second=%+v", src.calls, first, second)
	}
}
