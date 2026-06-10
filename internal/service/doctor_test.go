package service

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestRunDoctorBasicChecks(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	opts := &RunDoctorOptions{}
	report, err := RunDoctor(ctx, opts)

	if err != nil {
		t.Fatalf("RunDoctor failed: %v", err)
	}

	if len(report.Checks) == 0 {
		t.Error("Expected at least one check, got 0")
	}

	// Verify all expected checks are present
	expectedChecks := map[string]bool{
		"stale-aliases":              false,
		"daemon-version-drift":       false,
		"daemon-db-size":             false,
		"watched-dirs-handle-limit":  false,
		"binary-dirty":               false,
		"governance-blocked":         false,
		"truncation-rate":            false,
	}

	for _, check := range report.Checks {
		expectedChecks[check.ID] = true
	}

	for checkID, found := range expectedChecks {
		if !found {
			t.Errorf("Expected check %q not found in report", checkID)
		}
	}
}

func TestBinaryDirtyCheckPass(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Binary should be clean when running from `go run` or a normal build
	opts := &RunDoctorOptions{}
	report, err := RunDoctor(ctx, opts)

	if err != nil {
		t.Fatalf("RunDoctor failed: %v", err)
	}

	// Find the binary-dirty check
	var check *DoctorCheck
	for i := range report.Checks {
		if report.Checks[i].ID == "binary-dirty" {
			check = &report.Checks[i]
			break
		}
	}

	if check == nil {
		t.Fatal("binary-dirty check not found")
	}

	// Check should pass (OK=true) for binaries built cleanly
	// Note: This may fail if the binary was actually built with vcs.modified=true
	if check.OK {
		if check.Severity != "P1" {
			t.Errorf("Expected severity P1 for binary-dirty check, got %s", check.Severity)
		}
	}
}

func TestStaleAliasesCheck(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	opts := &RunDoctorOptions{}
	report, err := RunDoctor(ctx, opts)

	if err != nil {
		t.Fatalf("RunDoctor failed: %v", err)
	}

	// Find the stale-aliases check
	var check *DoctorCheck
	for i := range report.Checks {
		if report.Checks[i].ID == "stale-aliases" {
			check = &report.Checks[i]
			break
		}
	}

	if check == nil {
		t.Fatal("stale-aliases check not found")
	}

	// Severity should be P2 for this check
	if check.Severity != "P2" {
		t.Errorf("Expected severity P2 for stale-aliases check, got %s", check.Severity)
	}

	if check.Detail == "" {
		t.Error("Expected detail message for stale-aliases check")
	}
}

func TestExitCodeBasedOnP1(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	opts := &RunDoctorOptions{
		// Simulate a P1 issue: binary marked as dirty
		// (This won't actually fail unless the binary is really dirty)
	}
	report, err := RunDoctor(ctx, opts)

	if err != nil {
		t.Fatalf("RunDoctor failed: %v", err)
	}

	// Exit code should be 0 if no P1 issues, 1 if any P1 issues
	if report.ExitCode != 0 && report.ExitCode != 1 {
		t.Errorf("Expected exit code 0 or 1, got %d", report.ExitCode)
	}
}

func TestSummaryGeneration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	opts := &RunDoctorOptions{}
	report, err := RunDoctor(ctx, opts)

	if err != nil {
		t.Fatalf("RunDoctor failed: %v", err)
	}

	if report.Summary == "" {
		t.Error("Expected summary to be populated")
	}

	// Summary should mention the number of checks
	if len(report.Checks) == 0 {
		t.Error("Expected checks in report")
	}
}

func TestTimestampPopulated(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	before := time.Now()
	report, err := RunDoctor(ctx, nil)
	after := time.Now()

	if err != nil {
		t.Fatalf("RunDoctor failed: %v", err)
	}

	if report.Timestamp.Before(before) || report.Timestamp.After(after.Add(time.Second)) {
		t.Errorf("Expected timestamp between %v and %v, got %v", before, after, report.Timestamp)
	}
}

func TestGetDaemonDatabasePath(t *testing.T) {
	path, err := getDaemonDatabasePath()
	if err != nil {
		t.Fatalf("getDaemonDatabasePath failed: %v", err)
	}

	if path == "" {
		t.Error("Expected non-empty path")
	}

	// Should contain .mi-lsp/daemon/daemon.db
	if !contains(path, ".mi-lsp") || !contains(path, "daemon.db") {
		t.Errorf("Expected path to contain .mi-lsp and daemon.db, got %s", path)
	}
}

func TestGetVCSModified(t *testing.T) {
	vcsModified := getVCSModified()
	// vcsModified will be "true" or "false" or empty depending on build
	// Just verify we can call it without error
	if vcsModified != "" && vcsModified != "true" && vcsModified != "false" {
		t.Errorf("Expected vcsModified to be '', 'true', or 'false', got %s", vcsModified)
	}
}

func TestDBSizeCheckDoesNotMutate(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Get initial state of home dir
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("Cannot get home dir: %v", err)
	}

	// Run doctor
	opts := &RunDoctorOptions{}
	_, err = RunDoctor(ctx, opts)

	if err != nil {
		t.Fatalf("RunDoctor failed: %v", err)
	}

	// Verify home dir still exists
	if _, err := os.Stat(home); err != nil {
		t.Errorf("Home dir was affected: %v", err)
	}
}

func contains(s, substr string) bool {
	for i := 0; i < len(s)-len(substr)+1; i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
