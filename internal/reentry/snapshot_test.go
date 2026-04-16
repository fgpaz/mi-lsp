package reentry

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func TestBuildSnapshotCollectsCanonicalChangesAndHandoff(t *testing.T) {
	root := t.TempDir()
	mustWriteSnapshotFile(t, root, ".docs/wiki/01_alcance_funcional.md", "# alcance\n")
	mustWriteSnapshotFile(t, root, ".docs/wiki/07_baseline_tecnica.md", "# baseline\n")
	mustWriteSnapshotFile(t, root, ".docs/raw/plans/2026-04-16-reentry-wave.md", "# plan\n")

	now := time.Now()
	setModTime(t, filepath.Join(root, ".docs/wiki/01_alcance_funcional.md"), now.Add(-2*time.Hour))
	setModTime(t, filepath.Join(root, ".docs/wiki/07_baseline_tecnica.md"), now.Add(-1*time.Hour))
	setModTime(t, filepath.Join(root, ".docs/raw/plans/2026-04-16-reentry-wave.md"), now.Add(-30*time.Minute))

	docs := []model.DocRecord{
		{Path: ".docs/wiki/01_alcance_funcional.md", Title: "Alcance", Layer: "01", Family: "functional", SearchText: "alcance funcional del workspace"},
		{Path: ".docs/wiki/07_baseline_tecnica.md", Title: "Baseline tecnica", Layer: "07", Family: "technical", DocID: "TECH-BASELINE", SearchText: "baseline tecnica del daemon"},
	}

	snapshot := BuildSnapshot(root, docs, now)
	if snapshot.SnapshotBuiltAt.IsZero() {
		t.Fatal("expected snapshot built time")
	}
	if len(snapshot.RecentCanonicalChanges) != 2 {
		t.Fatalf("recent changes = %d, want 2", len(snapshot.RecentCanonicalChanges))
	}
	if snapshot.RecentCanonicalChanges[0].Path != ".docs/wiki/07_baseline_tecnica.md" {
		t.Fatalf("first change path = %q", snapshot.RecentCanonicalChanges[0].Path)
	}
	if snapshot.BestReentry.Op != "nav.search" {
		t.Fatalf("best reentry op = %q, want nav.search", snapshot.BestReentry.Op)
	}
	if snapshot.Handoff != "plans/2026-04-16-reentry-wave" {
		t.Fatalf("handoff = %q", snapshot.Handoff)
	}
}

func TestSnapshotStaleDetectsNewerRawChanges(t *testing.T) {
	root := t.TempDir()
	mustWriteSnapshotFile(t, root, ".docs/wiki/07_baseline_tecnica.md", "# baseline\n")
	mustWriteSnapshotFile(t, root, ".docs/raw/prompts/reentry.md", "# prompt\n")

	builtAt := time.Now().Add(-2 * time.Hour)
	setModTime(t, filepath.Join(root, ".docs/wiki/07_baseline_tecnica.md"), builtAt.Add(-time.Hour))
	setModTime(t, filepath.Join(root, ".docs/raw/prompts/reentry.md"), builtAt.Add(time.Hour))

	if !SnapshotStale(root, builtAt) {
		t.Fatal("expected snapshot to be stale")
	}
}

func TestBuildSnapshotFallsBackToFilesystemCanonicalDocs(t *testing.T) {
	root := t.TempDir()
	mustWriteSnapshotFile(t, root, ".docs/wiki/09_contratos/CT-NAV-ASK.md", "# CT-NAV-ASK\n\nContrato tecnico reciente.\n")
	now := time.Now()
	setModTime(t, filepath.Join(root, ".docs/wiki/09_contratos/CT-NAV-ASK.md"), now.Add(-time.Hour))

	snapshot := BuildSnapshot(root, nil, now)
	if len(snapshot.RecentCanonicalChanges) != 1 {
		t.Fatalf("recent changes = %d, want 1", len(snapshot.RecentCanonicalChanges))
	}
	change := snapshot.RecentCanonicalChanges[0]
	if change.DocID != "CT-NAV-ASK" {
		t.Fatalf("doc id = %q, want CT-NAV-ASK", change.DocID)
	}
	if snapshot.BestReentry.Op != "nav.search" {
		t.Fatalf("best reentry op = %q, want nav.search", snapshot.BestReentry.Op)
	}
}

func mustWriteSnapshotFile(t *testing.T, root string, relativePath string, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relativePath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", relativePath, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", relativePath, err)
	}
}

func setModTime(t *testing.T, path string, modTime time.Time) {
	t.Helper()
	if err := os.Chtimes(path, modTime, modTime); err != nil {
		t.Fatalf("chtimes %s: %v", path, err)
	}
}
