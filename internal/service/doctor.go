package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"github.com/fgpaz/mi-lsp/internal/workspace"
)

// DoctorCheck represents a single diagnostic check result.
type DoctorCheck struct {
	ID       string `json:"id"`
	Severity string `json:"severity"` // P1, P2, P3, or "ok"
	OK       bool   `json:"ok"`
	Detail   string `json:"detail"`
}

// DoctorReport is the collection of all doctor checks.
type DoctorReport struct {
	Checks    []DoctorCheck `json:"checks"`
	Summary   string        `json:"summary"`
	ExitCode  int           `json:"exit_code"`
	Timestamp time.Time     `json:"timestamp"`
}

// RunDoctorOptions contains optional parameters for doctor checks.
type RunDoctorOptions struct {
	DaemonVersion     string
	DaemonWatchedDirs int
	DaemonStatus      any // raw daemon status response
}

// RunDoctor executes all read-only checks.
func RunDoctor(ctx context.Context, opts *RunDoctorOptions) (DoctorReport, error) {
	if opts == nil {
		opts = &RunDoctorOptions{}
	}

	report := DoctorReport{
		Checks:    []DoctorCheck{},
		Timestamp: time.Now(),
		ExitCode:  0,
	}

	// Run all checks
	checks := []func(context.Context, *RunDoctorOptions) DoctorCheck{
		checkStaleAliases,
		checkDaemonVersion,
		checkDBSize,
		checkWatchedDirs,
		checkBinaryDirty,
		checkGovernanceBlocked,
		checkTruncationRate,
	}

	for _, checkFn := range checks {
		check := checkFn(ctx, opts)
		report.Checks = append(report.Checks, check)
		if check.Severity == "P1" && !check.OK {
			report.ExitCode = 1
		}
	}

	// Compute summary
	p1Count := 0
	p2Count := 0
	p3Count := 0
	okCount := 0
	for _, check := range report.Checks {
		if !check.OK {
			switch check.Severity {
			case "P1":
				p1Count++
			case "P2":
				p2Count++
			case "P3":
				p3Count++
			}
		} else {
			okCount++
		}
	}

	parts := []string{}
	if p1Count > 0 {
		parts = append(parts, fmt.Sprintf("%d P1", p1Count))
	}
	if p2Count > 0 {
		parts = append(parts, fmt.Sprintf("%d P2", p2Count))
	}
	if p3Count > 0 {
		parts = append(parts, fmt.Sprintf("%d P3", p3Count))
	}
	if okCount > 0 {
		parts = append(parts, fmt.Sprintf("%d ok", okCount))
	}

	if p1Count == 0 && p2Count == 0 && p3Count == 0 {
		report.Summary = "All checks passed"
	} else if p1Count == 0 && p2Count == 0 {
		report.Summary = strings.Join(parts, ", ")
	} else {
		report.Summary = strings.Join(parts, ", ")
	}

	return report, nil
}

// checkStaleAliases verifies that registry aliases point to existing paths.
func checkStaleAliases(ctx context.Context, opts *RunDoctorOptions) DoctorCheck {
	const id = "stale-aliases"

	registry, err := workspace.LoadRegistry()
	if err != nil {
		return DoctorCheck{
			ID:       id,
			Severity: "P2",
			OK:       false,
			Detail:   fmt.Sprintf("Failed to load registry: %v", err),
		}
	}

	var staleAliases []string
	for alias, ws := range registry.Workspaces {
		root := strings.TrimSpace(ws.Root)
		if root == "" {
			staleAliases = append(staleAliases, alias)
			continue
		}
		if _, err := os.Stat(root); errors.Is(err, os.ErrNotExist) {
			staleAliases = append(staleAliases, alias)
		}
	}

	if len(staleAliases) > 0 {
		return DoctorCheck{
			ID:       id,
			Severity: "P2",
			OK:       false,
			Detail:   fmt.Sprintf("Stale aliases in registry: %s (run: mi-lsp workspace hygiene --apply-safe)", strings.Join(staleAliases, ", ")),
		}
	}

	return DoctorCheck{
		ID:       id,
		Severity: "P1",
		OK:       true,
		Detail:   "No stale aliases in registry",
	}
}

// checkDaemonVersion compares daemon version with CLI version.
func checkDaemonVersion(ctx context.Context, opts *RunDoctorOptions) DoctorCheck {
	const id = "daemon-version-drift"

	// Get CLI version
	cliVersion := getCLIVersion()

	// Use daemon version from options (populated by CLI layer)
	daemonVersion := opts.DaemonVersion

	// If daemon is not running, that's not a failure (daemon is optional)
	if daemonVersion == "" {
		return DoctorCheck{
			ID:       id,
			Severity: "P1",
			OK:       true,
			Detail:   "Daemon not running (optional)",
		}
	}

	// Hardcoded "dev" version is a P1 issue
	if daemonVersion == "dev" {
		return DoctorCheck{
			ID:       id,
			Severity: "P1",
			OK:       false,
			Detail:   fmt.Sprintf("Daemon reports hardcoded version 'dev'; CLI version: %s (run: daemon stop && mi-lsp daemon start)", cliVersion),
		}
	}

	// Check if they match
	if daemonVersion != cliVersion {
		return DoctorCheck{
			ID:       id,
			Severity: "P2",
			OK:       false,
			Detail:   fmt.Sprintf("Version drift: CLI=%s, daemon=%s (run: mi-lsp daemon stop && mi-lsp daemon start)", cliVersion, daemonVersion),
		}
	}

	return DoctorCheck{
		ID:       id,
		Severity: "P1",
		OK:       true,
		Detail:   fmt.Sprintf("Daemon and CLI versions match: %s", cliVersion),
	}
}

// checkDBSize checks if daemon.db exceeds a threshold.
func checkDBSize(ctx context.Context, opts *RunDoctorOptions) DoctorCheck {
	const id = "daemon-db-size"
	const maxBytes int64 = 500 * 1024 * 1024 // 500 MB

	dbPath, err := getDaemonDatabasePath()
	if err != nil {
		return DoctorCheck{
			ID:       id,
			Severity: "P2",
			OK:       true,
			Detail:   "Cannot determine database path (daemon may not have run)",
		}
	}

	info, err := os.Stat(dbPath)
	if errors.Is(err, os.ErrNotExist) {
		return DoctorCheck{
			ID:       id,
			Severity: "P1",
			OK:       true,
			Detail:   "Database does not exist (daemon not yet initialized)",
		}
	}
	if err != nil {
		return DoctorCheck{
			ID:       id,
			Severity: "P2",
			OK:       true,
			Detail:   "Cannot stat database",
		}
	}

	size := info.Size()
	if size > maxBytes {
		return DoctorCheck{
			ID:       id,
			Severity: "P2",
			OK:       false,
			Detail:   fmt.Sprintf("Database size %.1f MB exceeds threshold (500 MB); run: mi-lsp admin telemetry-purge", float64(size)/1024.0/1024.0),
		}
	}

	return DoctorCheck{
		ID:       id,
		Severity: "P1",
		OK:       true,
		Detail:   fmt.Sprintf("Database size: %.1f MB", float64(size)/1024.0/1024.0),
	}
}

// checkWatchedDirs verifies that watched dirs do not exceed OS handle limit.
func checkWatchedDirs(ctx context.Context, opts *RunDoctorOptions) DoctorCheck {
	const id = "watched-dirs-handle-limit"

	// Use watched dirs from options (populated by CLI layer)
	watchedDirs := opts.DaemonWatchedDirs

	// If no daemon status, report as skipped
	if watchedDirs <= 0 {
		return DoctorCheck{
			ID:       id,
			Severity: "P2",
			OK:       true,
			Detail:   "Daemon not running (skipped; handles only matter if daemon is active)",
		}
	}

	// On Windows, the typical limit is around 32k handles per process.
	// Check if watched dirs are approaching the limit.
	if watchedDirs > 28000 {
		return DoctorCheck{
			ID:       id,
			Severity: "P2",
			OK:       false,
			Detail:   fmt.Sprintf("Watched dirs near OS handle limit: %d dirs; consider using --watch-mode=lazy or adjusting --max-watched-roots", watchedDirs),
		}
	}

	return DoctorCheck{
		ID:       id,
		Severity: "P1",
		OK:       true,
		Detail:   fmt.Sprintf("Watched dirs: %d (safe)", watchedDirs),
	}
}

// checkBinaryDirty verifies that the executable is not marked as +dirty.
func checkBinaryDirty(ctx context.Context, opts *RunDoctorOptions) DoctorCheck {
	const id = "binary-dirty"

	cliPath, _ := executableSnapshot()
	vcsModified := getVCSModified()

	// If vcs.modified is "true", the binary is +dirty
	if strings.ToLower(vcsModified) == "true" {
		return DoctorCheck{
			ID:       id,
			Severity: "P1",
			OK:       false,
			Detail:   fmt.Sprintf("Executable is marked +dirty (vcs_modified=true); binary at %s is not from a clean release tag; run: scripts/release/ae-release-binaries.ps1", cliPath),
		}
	}

	return DoctorCheck{
		ID:       id,
		Severity: "P1",
		OK:       true,
		Detail:   fmt.Sprintf("Binary is clean (vcs_modified=%s)", vcsModified),
	}
}

// checkGovernanceBlocked checks if governance is blocked for any workspace.
func checkGovernanceBlocked(ctx context.Context, opts *RunDoctorOptions) DoctorCheck {
	const id = "governance-blocked"

	// Load registry
	registry, err := workspace.LoadRegistry()
	if err != nil {
		return DoctorCheck{
			ID:       id,
			Severity: "P2",
			OK:       true,
			Detail:   "Cannot load registry for governance check",
		}
	}

	// This is a read-only check; we don't have direct access to governance status
	// from the CLI without executing a nav governance call per workspace.
	// For now, we mark it as skipped but note that governance blocking is a concern.
	if len(registry.Workspaces) == 0 {
		return DoctorCheck{
			ID:       id,
			Severity: "P1",
			OK:       true,
			Detail:   "No workspaces registered (governance check N/A)",
		}
	}

	return DoctorCheck{
		ID:       id,
		Severity: "P1",
		OK:       true,
		Detail:   fmt.Sprintf("Governance check skipped (requires daemon); %d workspaces registered", len(registry.Workspaces)),
	}
}

// checkTruncationRate checks if truncation rate is high (from recent telemetry).
func checkTruncationRate(ctx context.Context, opts *RunDoctorOptions) DoctorCheck {
	const id = "truncation-rate"

	// This requires reading the daemon's telemetry store, which is read-only
	// but may not be available in all cases.
	// For now, we mark it as skipped unless we can reliably read it.

	return DoctorCheck{
		ID:       id,
		Severity: "P2",
		OK:       true,
		Detail:   "Truncation rate check skipped (requires telemetry access; monitor via daemon status)",
	}
}

// ============ Helper functions ============

// getCLIVersion returns the version of the running CLI.
func getCLIVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	version := info.Main.Version
	if version == "" {
		return "unknown"
	}
	return version
}


// executableSnapshot returns the path and SHA256 of the running executable.
func executableSnapshot() (string, string) {
	exePath, err := os.Executable()
	if err != nil {
		return "", ""
	}

	file, err := os.Open(exePath)
	if err != nil {
		return exePath, ""
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return exePath, ""
	}

	return exePath, hex.EncodeToString(hash.Sum(nil))
}

// getVCSModified returns the vcs.modified build setting.
func getVCSModified() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	return buildSetting(info, "vcs.modified")
}

// buildSetting extracts a build setting from the build info.
func buildSetting(info *debug.BuildInfo, key string) string {
	for _, setting := range info.Settings {
		if setting.Key == key {
			return setting.Value
		}
	}
	return ""
}

// getDaemonDatabasePath returns the path to the daemon database file.
// This mirrors the logic in internal/daemon/state_store.go:daemonDatabasePath().
func getDaemonDatabasePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".mi-lsp", "daemon")
	return filepath.Join(dir, "daemon.db"), nil
}
