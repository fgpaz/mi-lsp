package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

func TestRegistryGCDryRun(t *testing.T) {
	// Create a temporary registry directory
	tmpDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	defer func() {
		if oldHome != "" {
			_ = os.Setenv("HOME", oldHome)
		}
	}()

	// Create a temporary home directory with mi-lsp subdirectory
	homeDir := filepath.Join(tmpDir, "home")
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		t.Fatalf("failed to create home dir: %v", err)
	}
	if err := os.Setenv("HOME", homeDir); err != nil {
		t.Fatalf("failed to set HOME: %v", err)
	}

	// Create registry directory
	registryDir := filepath.Join(homeDir, ".mi-lsp")
	if err := os.MkdirAll(registryDir, 0o755); err != nil {
		t.Fatalf("failed to create registry dir: %v", err)
	}

	// Create two existing workspace paths
	existingPath1 := filepath.Join(tmpDir, "workspace1")
	existingPath2 := filepath.Join(tmpDir, "workspace2")
	if err := os.MkdirAll(existingPath1, 0o755); err != nil {
		t.Fatalf("failed to create existing path 1: %v", err)
	}
	if err := os.MkdirAll(existingPath2, 0o755); err != nil {
		t.Fatalf("failed to create existing path 2: %v", err)
	}

	// Create registry with 4 workspaces: 2 existing, 2 nonexistent
	registry := model.RegistryFile{
		Workspaces: map[string]model.WorkspaceRegistration{
			"exists1": {Name: "exists1", Root: existingPath1, Kind: "mixed"},
			"exists2": {Name: "exists2", Root: existingPath2, Kind: "mixed"},
			"stale1":  {Name: "stale1", Root: filepath.Join(tmpDir, "nonexistent1"), Kind: "mixed"},
			"stale2":  {Name: "stale2", Root: filepath.Join(tmpDir, "nonexistent2"), Kind: "mixed"},
		},
		Defaults: model.RegistryDefaults{LastWorkspace: "exists1"},
	}

	// Save registry
	if err := workspace.SaveRegistry(registry); err != nil {
		t.Fatalf("failed to save registry: %v", err)
	}

	// Run GarbageCollectRegistry with dry_run=true
	report, err := workspace.GarbageCollectRegistry(false) // apply=false means dry-run
	if err != nil {
		t.Fatalf("GarbageCollectRegistry failed: %v", err)
	}

	// Verify results
	if !report.DryRun {
		t.Error("expected DryRun=true")
	}
	if report.RemovedCount != 0 {
		t.Errorf("expected RemovedCount=0 in dry-run, got %d", report.RemovedCount)
	}
	if len(report.Candidates) != 2 {
		t.Errorf("expected 2 candidates, got %d", len(report.Candidates))
	}
	if len(report.Removed) != 0 {
		t.Errorf("expected 0 removed in dry-run, got %d", len(report.Removed))
	}

	// Verify registry unchanged
	loadedRegistry, err := workspace.LoadRegistry()
	if err != nil {
		t.Fatalf("failed to load registry: %v", err)
	}
	if len(loadedRegistry.Workspaces) != 4 {
		t.Errorf("expected 4 workspaces after dry-run, got %d", len(loadedRegistry.Workspaces))
	}
}

func TestRegistryGCApply(t *testing.T) {
	// Create a temporary registry directory
	tmpDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	defer func() {
		if oldHome != "" {
			_ = os.Setenv("HOME", oldHome)
		}
	}()

	// Create a temporary home directory with mi-lsp subdirectory
	homeDir := filepath.Join(tmpDir, "home")
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		t.Fatalf("failed to create home dir: %v", err)
	}
	if err := os.Setenv("HOME", homeDir); err != nil {
		t.Fatalf("failed to set HOME: %v", err)
	}

	// Create registry directory
	registryDir := filepath.Join(homeDir, ".mi-lsp")
	if err := os.MkdirAll(registryDir, 0o755); err != nil {
		t.Fatalf("failed to create registry dir: %v", err)
	}

	// Create two existing workspace paths
	existingPath1 := filepath.Join(tmpDir, "workspace1")
	existingPath2 := filepath.Join(tmpDir, "workspace2")
	if err := os.MkdirAll(existingPath1, 0o755); err != nil {
		t.Fatalf("failed to create existing path 1: %v", err)
	}
	if err := os.MkdirAll(existingPath2, 0o755); err != nil {
		t.Fatalf("failed to create existing path 2: %v", err)
	}

	// Create registry with 4 workspaces: 2 existing, 2 nonexistent
	registry := model.RegistryFile{
		Workspaces: map[string]model.WorkspaceRegistration{
			"exists1": {Name: "exists1", Root: existingPath1, Kind: "mixed"},
			"exists2": {Name: "exists2", Root: existingPath2, Kind: "mixed"},
			"stale1":  {Name: "stale1", Root: filepath.Join(tmpDir, "nonexistent1"), Kind: "mixed"},
			"stale2":  {Name: "stale2", Root: filepath.Join(tmpDir, "nonexistent2"), Kind: "mixed"},
		},
		Defaults: model.RegistryDefaults{LastWorkspace: "stale1"},
	}

	// Save registry
	if err := workspace.SaveRegistry(registry); err != nil {
		t.Fatalf("failed to save registry: %v", err)
	}

	// Run GarbageCollectRegistry with apply=true
	report, err := workspace.GarbageCollectRegistry(true) // apply=true
	if err != nil {
		t.Fatalf("GarbageCollectRegistry failed: %v", err)
	}

	// Verify results
	if report.DryRun {
		t.Error("expected DryRun=false when apply=true")
	}
	if report.RemovedCount != 2 {
		t.Errorf("expected RemovedCount=2, got %d", report.RemovedCount)
	}
	if len(report.Candidates) != 2 {
		t.Errorf("expected 2 candidates, got %d", len(report.Candidates))
	}
	if len(report.Removed) != 2 {
		t.Errorf("expected 2 removed, got %d", len(report.Removed))
	}

	// Verify registry is updated
	loadedRegistry, err := workspace.LoadRegistry()
	if err != nil {
		t.Fatalf("failed to load registry: %v", err)
	}
	if len(loadedRegistry.Workspaces) != 2 {
		t.Errorf("expected 2 workspaces after apply, got %d", len(loadedRegistry.Workspaces))
	}

	// Verify stale workspaces are removed
	if _, ok := loadedRegistry.Workspaces["stale1"]; ok {
		t.Error("stale1 should be removed")
	}
	if _, ok := loadedRegistry.Workspaces["stale2"]; ok {
		t.Error("stale2 should be removed")
	}

	// Verify existing workspaces are kept
	if _, ok := loadedRegistry.Workspaces["exists1"]; !ok {
		t.Error("exists1 should be kept")
	}
	if _, ok := loadedRegistry.Workspaces["exists2"]; !ok {
		t.Error("exists2 should be kept")
	}

	// Verify LastWorkspace was cleared since it was stale
	if loadedRegistry.Defaults.LastWorkspace != "" {
		t.Errorf("expected LastWorkspace to be empty, got %q", loadedRegistry.Defaults.LastWorkspace)
	}

	// Verify backup was created
	registryPath, err := workspace.RegistryPath()
	if err != nil {
		t.Fatalf("failed to get registry path: %v", err)
	}
	backupDir := filepath.Dir(registryPath)
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		t.Fatalf("failed to read registry dir: %v", err)
	}

	backupFound := false
	for _, entry := range entries {
		if !entry.IsDir() && len(entry.Name()) > len("registry.toml.bak-") &&
			entry.Name()[:len("registry.toml.bak-")] == "registry.toml.bak-" {
			backupFound = true
			break
		}
	}
	if !backupFound {
		t.Error("backup file was not created")
	}
}

func TestRegistryGCWithEmptyRoot(t *testing.T) {
	// Create a temporary registry directory
	tmpDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	defer func() {
		if oldHome != "" {
			_ = os.Setenv("HOME", oldHome)
		}
	}()

	// Create a temporary home directory with mi-lsp subdirectory
	homeDir := filepath.Join(tmpDir, "home")
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		t.Fatalf("failed to create home dir: %v", err)
	}
	if err := os.Setenv("HOME", homeDir); err != nil {
		t.Fatalf("failed to set HOME: %v", err)
	}

	// Create registry directory
	registryDir := filepath.Join(homeDir, ".mi-lsp")
	if err := os.MkdirAll(registryDir, 0o755); err != nil {
		t.Fatalf("failed to create registry dir: %v", err)
	}

	// Create one existing workspace path
	existingPath := filepath.Join(tmpDir, "workspace1")
	if err := os.MkdirAll(existingPath, 0o755); err != nil {
		t.Fatalf("failed to create existing path: %v", err)
	}

	// Create registry with workspace with empty root (should be skipped)
	registry := model.RegistryFile{
		Workspaces: map[string]model.WorkspaceRegistration{
			"exists": {Name: "exists", Root: existingPath, Kind: "mixed"},
			"empty":  {Name: "empty", Root: "", Kind: "mixed"},
		},
		Defaults: model.RegistryDefaults{LastWorkspace: "exists"},
	}

	// Save registry
	if err := workspace.SaveRegistry(registry); err != nil {
		t.Fatalf("failed to save registry: %v", err)
	}

	// Run GarbageCollectRegistry with dry_run=true
	report, err := workspace.GarbageCollectRegistry(false) // apply=false means dry-run
	if err != nil {
		t.Fatalf("GarbageCollectRegistry failed: %v", err)
	}

	// Verify that empty root is skipped, not treated as a candidate
	if len(report.Candidates) != 0 {
		t.Errorf("expected 0 candidates for empty root, got %d", len(report.Candidates))
	}
	if len(report.Skipped) != 1 {
		t.Errorf("expected 1 skipped, got %d", len(report.Skipped))
	}
}
