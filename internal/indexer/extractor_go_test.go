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
		Root:               root,
		RepositoryIdentity: "https://github.com/example/go-index-fixture",
		ProjectOrModule:    "go.mod",
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
