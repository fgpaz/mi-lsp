package daemon

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func writeLeaseFixture(t *testing.T, dir string, pid int, lock string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if lock != "" {
		body := fmt.Sprintf("version=1\npid=%d\nnonce=00112233445566778899aabbccddeeff\n", pid)
		if err := os.WriteFile(filepath.Join(dir, "instance.lock"), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	state := fmt.Sprintf(`{"pid":%d,"endpoint":"fixture-endpoint","admin_token":"fixture-secret"}`, pid)
	if err := os.WriteFile(filepath.Join(dir, "state.json"), []byte(state), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestReadLeaseSnapshotReportsLegacyOwner(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "daemon")
	writeLeaseFixture(t, dir, 48123, "")
	got := readLeaseSnapshot(dir, func(int) string { return "indeterminate" })
	if got.State != "legacy_owner" || got.OwnerPID != 48123 || got.Liveness != "indeterminate" || got.InstanceID != "" {
		t.Fatalf("unexpected legacy result: %+v", got)
	}
}

func TestReadLeaseSnapshotInvalidMetadata(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "daemon")
	writeLeaseFixture(t, dir, 48123, "present")
	if err := os.WriteFile(filepath.Join(dir, "instance.lock"), []byte("version=not-a-version\npid=nope\nnonce=broken\n"), 0600); err != nil {
		t.Fatal(err)
	}
	got := readLeaseSnapshot(dir, func(int) string { return "alive" })
	if got.State != "invalid" || got.Liveness != "indeterminate" {
		t.Fatalf("unexpected invalid result: %+v", got)
	}
}

func TestReadLeaseSnapshotStaleAndUnknownOwner(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "daemon")
	writeLeaseFixture(t, dir, 48123, "present")
	stale := readLeaseSnapshot(dir, func(int) string { return "dead" })
	if stale.State != "stale" || stale.Liveness != "dead" || stale.OwnerPID != 48123 {
		t.Fatalf("unexpected stale result: %+v", stale)
	}
	unknown := readLeaseSnapshot(dir, func(int) string { return "indeterminate" })
	if unknown.State != "indeterminate" || unknown.Liveness != "indeterminate" || unknown.OwnerPID != 48123 {
		t.Fatalf("unknown owner lost diagnostic identity: %+v", unknown)
	}
}

func TestReadLeaseSnapshotAbsentIsNonCreating(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "missing", "daemon")
	got := readLeaseSnapshot(dir, func(int) string { return "dead" })
	if got.State != "absent" {
		t.Fatalf("unexpected absent result: %+v", got)
	}
	if _, err := os.Stat(filepath.Dir(dir)); !os.IsNotExist(err) {
		t.Fatalf("reader created runtime path: %v", err)
	}
}
