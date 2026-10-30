package indexer

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestObserveGoGraphIgnoresTestOnlyPackages(t *testing.T) {
	root := t.TempDir()
	write := func(path, contents string) {
		t.Helper()
		fullPath := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fullPath, []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	write("go.mod", "module example.com/go-index-fixture\n\ngo 1.24\n")
	write("fixture.go", "package fixture\n\nfunc Ready() bool { return true }\n")
	write("integration/integration_test.go", "package integration_test\n\nimport \"testing\"\n\nfunc TestIntegration(t *testing.T) {}\n")

	batch, err := ObserveGoGraph(context.Background(), GoGraphObservationRequest{
		Root: root, RepositoryIdentity: "https://github.com/example/go-index-fixture", ProjectOrModule: "go.mod",
	})
	if err != nil {
		t.Fatalf("ObserveGoGraph() error = %v", err)
	}
	if batch.Completeness != "complete" {
		t.Fatalf("completeness = %q, want complete; omissions = %+v", batch.Completeness, batch.Omissions)
	}
	if err := batch.ReadyForStaging(); err != nil {
		t.Fatalf("ReadyForStaging() error = %v", err)
	}
	for _, omission := range batch.Omissions {
		if omission.ReasonCode == "no_parseable_sources" || omission.ReasonCode == "type_check_no_sources" {
			t.Fatalf("test-only package produced a source omission: %+v", omission)
		}
	}
}

func TestObserveGoGraphMarksOutsideSymlinkSourceIncomplete(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	writeGoFixture := func(path, content string) {
		t.Helper()
		full := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	writeGoFixture("go.mod", "module example.com/go-symlink-fixture\n\ngo 1.23\n")
	writeGoFixture("ordinary.go", "package fixture\n\nfunc Local() {}\n")
	external := filepath.Join(outside, "external.go")
	if err := os.WriteFile(external, []byte("package fixture\n\nfunc Outside() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, filepath.Join(root, "external.go")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	batch, err := ObserveGoGraph(context.Background(), GoGraphObservationRequest{
		Root: root, RepositoryIdentity: "https://example.com/go-symlink-fixture", ProjectOrModule: "go.mod", GoOffline: true,
	})
	if err != nil {
		t.Fatalf("ObserveGoGraph() error = %v", err)
	}
	if batch.Completeness != "partial" {
		t.Fatalf("completeness = %q, want partial; omissions=%+v", batch.Completeness, batch.Omissions)
	}
	if err := batch.ReadyForStaging(); err == nil {
		t.Fatal("outside-root symlink source produced a stageable observation")
	}
	found := false
	for _, omission := range batch.Omissions {
		if omission.OwnerPath == "external.go" && omission.ReasonCode == "external_target" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("missing external-target omission for source: %+v", batch.Omissions)
	}
}

func TestObserveGoGraphDoesNotStageUnresolvableSource(t *testing.T) {
	root := t.TempDir()
	for path, content := range map[string]string{
		"go.mod":      "module example.com/go-broken-symlink-fixture\n\ngo 1.23\n",
		"ordinary.go": "package fixture\n\nfunc Local() {}\n",
	} {
		if err := os.WriteFile(filepath.Join(root, path), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(filepath.Join(root, "missing-target.go"), filepath.Join(root, "broken.go")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	batch, err := ObserveGoGraph(context.Background(), GoGraphObservationRequest{
		Root: root, RepositoryIdentity: "https://example.com/go-broken-symlink-fixture", ProjectOrModule: "go.mod", GoOffline: true,
	})
	if err == nil && batch.ReadyForStaging() == nil {
		t.Fatal("unresolvable Go source produced a stageable observation")
	}
}
