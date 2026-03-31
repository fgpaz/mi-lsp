package worker

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func TestResolveToolRootUsesExecutableRepoWhenCallerIsDifferentRepo(t *testing.T) {
	callerRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(callerRoot, "go.mod"), []byte("module example.com/caller\n"), 0o644); err != nil {
		t.Fatalf("write caller go.mod: %v", err)
	}

	execRoot := t.TempDir()
	writeToolRepoMarkers(t, execRoot)

	restoreExec := stubExecutablePath(t, filepath.Join(execRoot, "mi-lsp.exe"))
	defer restoreExec()

	root, kind := ResolveToolRoot(callerRoot)
	if root != execRoot {
		t.Fatalf("tool root = %q, want executable repo %q", root, execRoot)
	}
	if kind != "repo" {
		t.Fatalf("tool root kind = %q, want repo", kind)
	}
}

func TestResolveToolRootUsesExecutableDistributionWhenCallerIsDifferentRepo(t *testing.T) {
	callerRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(callerRoot, "go.mod"), []byte("module example.com/caller\n"), 0o644); err != nil {
		t.Fatalf("write caller go.mod: %v", err)
	}

	execRoot := t.TempDir()
	restoreExec := stubExecutablePath(t, filepath.Join(execRoot, "mi-lsp.exe"))
	defer restoreExec()

	root, kind := ResolveToolRoot(callerRoot)
	if root != execRoot {
		t.Fatalf("tool root = %q, want executable distribution %q", root, execRoot)
	}
	if kind != "distribution" {
		t.Fatalf("tool root kind = %q, want distribution", kind)
	}
}

func TestResolveToolRootFallsBackToCurrentRepoDuringGoRun(t *testing.T) {
	toolRoot := t.TempDir()
	writeToolRepoMarkers(t, toolRoot)

	execRoot := t.TempDir()
	restoreExec := stubExecutablePath(t, filepath.Join(execRoot, "mi-lsp.exe"))
	defer restoreExec()

	root, kind := ResolveToolRoot(toolRoot)
	if root != toolRoot {
		t.Fatalf("tool root = %q, want current repo %q", root, toolRoot)
	}
	if kind != "repo" {
		t.Fatalf("tool root kind = %q, want repo", kind)
	}
}

func TestResolveLaunchSpecUsesBundledWorkerBeforeInstalledWorker(t *testing.T) {
	toolRoot := t.TempDir()
	rid := ResolveRID()
	bundledPath := writeWorkerBinary(t, filepath.Join(toolRoot, "workers", rid))
	installedHome := t.TempDir()
	installedPath := writeWorkerBinary(t, filepath.Join(installedHome, ".mi-lsp", "workers", rid))

	restoreProbe := stubProbeWorker(t, func(path string) workerProbeResult {
		if path == bundledPath {
			return workerProbeResult{ProtocolVersion: "mi-lsp-v1.1", Compatible: true}
		}
		if path == installedPath {
			return workerProbeResult{ProtocolVersion: "legacy", Compatible: false, Err: errors.New("legacy worker")}
		}
		return workerProbeResult{Err: errors.New("unexpected candidate")}
	})
	defer restoreProbe()
	restoreHome := stubHomeDir(t, installedHome)
	defer restoreHome()
	restoreLookPath := stubLookPath(t, exec.ErrNotFound)
	defer restoreLookPath()

	spec, err := ResolveLaunchSpec(toolRoot)
	if err != nil {
		t.Fatalf("ResolveLaunchSpec: %v", err)
	}
	if spec.Command != bundledPath {
		t.Fatalf("command = %q, want bundled worker %q (installed %q)", spec.Command, bundledPath, installedPath)
	}
}

func TestResolveLaunchSpecDoesNotProbeCandidatesOnHotPath(t *testing.T) {
	toolRoot := t.TempDir()
	rid := ResolveRID()
	bundledPath := writeWorkerBinary(t, filepath.Join(toolRoot, "workers", rid))

	probeCalls := 0
	restoreProbe := stubProbeWorker(t, func(path string) workerProbeResult {
		probeCalls++
		return workerProbeResult{ProtocolVersion: model.ProtocolVersion, Compatible: true}
	})
	defer restoreProbe()
	restoreHome := stubHomeDir(t, t.TempDir())
	defer restoreHome()
	restoreLookPath := stubLookPath(t, exec.ErrNotFound)
	defer restoreLookPath()

	spec, err := ResolveLaunchSpec(toolRoot)
	if err != nil {
		t.Fatalf("ResolveLaunchSpec: %v", err)
	}
	if spec.Command != bundledPath {
		t.Fatalf("command = %q, want bundled worker %q", spec.Command, bundledPath)
	}
	if probeCalls != 0 {
		t.Fatalf("expected no worker probe calls on hot path, got %d", probeCalls)
	}
}

func TestInstallWorkerCopiesBundledWorkerWithoutRepoProject(t *testing.T) {
	toolRoot := t.TempDir()
	rid := ResolveRID()
	_ = writeWorkerBinary(t, filepath.Join(toolRoot, "workers", rid))
	installedHome := t.TempDir()

	restoreHome := stubHomeDir(t, installedHome)
	defer restoreHome()
	restoreLookPath := stubLookPath(t, exec.ErrNotFound)
	defer restoreLookPath()

	path, err := InstallWorker(toolRoot, rid)
	if err != nil {
		t.Fatalf("InstallWorker: %v", err)
	}

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(installed): %v", err)
	}
	if string(body) != "worker-binary" {
		t.Fatalf("installed worker contents = %q, want worker-binary", string(body))
	}
}

func TestInspectWorkerRuntimePrefersCompatibleBundledWorker(t *testing.T) {
	toolRoot := t.TempDir()
	rid := ResolveRID()
	bundledPath := writeWorkerBinary(t, filepath.Join(toolRoot, "workers", rid))
	installedHome := t.TempDir()
	installedPath := writeWorkerBinary(t, filepath.Join(installedHome, ".mi-lsp", "workers", rid))

	restoreProbe := stubProbeWorker(t, func(path string) workerProbeResult {
		if path == bundledPath {
			return workerProbeResult{ProtocolVersion: "mi-lsp-v1.1", Compatible: true}
		}
		if path == installedPath {
			return workerProbeResult{ProtocolVersion: "mi-lsp-v1.0", Compatible: false, Err: errors.New("protocol version mismatch")}
		}
		return workerProbeResult{Err: errors.New("unexpected candidate")}
	})
	defer restoreProbe()
	restoreHome := stubHomeDir(t, installedHome)
	defer restoreHome()
	restoreLookPath := stubLookPath(t, exec.ErrNotFound)
	defer restoreLookPath()

	info := InspectWorkerRuntime(toolRoot, rid)
	if info.Selected.Source != "bundle" {
		t.Fatalf("selected source = %q, want bundle", info.Selected.Source)
	}
	if !info.Selected.Compatible {
		t.Fatalf("selected worker should be compatible, got %#v", info.Selected)
	}
	if info.Installed.Compatible {
		t.Fatalf("installed worker should be incompatible, got %#v", info.Installed)
	}
	if !strings.Contains(info.Installed.Error, "protocol version mismatch") {
		t.Fatalf("installed error = %q, want protocol version mismatch", info.Installed.Error)
	}
}

func TestInspectWorkerRuntimeInRepoUsesDevLocalFallbackBeforeRepoBuildArtifacts(t *testing.T) {
	toolRoot := t.TempDir()
	writeToolRepoMarkers(t, toolRoot)
	rid := ResolveRID()
	_ = writeWorkerBinary(t, filepath.Join(toolRoot, "bin", "workers", rid))
	devLocalPath := filepath.Join(toolRoot, "worker-dotnet", "MiLsp.Worker", "bin", "Debug", "net10.0", "MiLsp.Worker.dll")
	if err := os.MkdirAll(filepath.Dir(devLocalPath), 0o755); err != nil {
		t.Fatalf("mkdir dev-local path: %v", err)
	}
	if err := os.WriteFile(devLocalPath, []byte("dev-local"), 0o644); err != nil {
		t.Fatalf("write dev-local dll: %v", err)
	}

	restoreLookPath := stubLookPath(t, nil)
	defer restoreLookPath()
	restoreHome := stubHomeDir(t, t.TempDir())
	defer restoreHome()
	info := InspectWorkerRuntime(toolRoot, rid)
	if info.Bundled.Path != "" {
		t.Fatalf("repo tool root should not expose bundled worker, got %#v", info.Bundled)
	}
	if info.Selected.Source != "dev-local" {
		t.Fatalf("selected source = %q, want dev-local", info.Selected.Source)
	}
	if info.Selected.Path != devLocalPath {
		t.Fatalf("selected path = %q, want %q", info.Selected.Path, devLocalPath)
	}
}
func writeToolRepoMarkers(t *testing.T, dir string) {
	t.Helper()
	markerFiles := []string{
		filepath.Join(dir, "worker-dotnet", "MiLsp.Worker", "MiLsp.Worker.csproj"),
		filepath.Join(dir, "cmd", "mi-lsp", "main.go"),
	}
	for _, path := range markerFiles {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}
		if err := os.WriteFile(path, []byte("marker"), 0o644); err != nil {
			t.Fatalf("write marker %s: %v", path, err)
		}
	}
}

func writeWorkerBinary(t *testing.T, dir string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	path := filepath.Join(dir, workerBinaryName())
	mode := os.FileMode(0o755)
	if runtime.GOOS == "windows" {
		mode = 0o644
	}
	if err := os.WriteFile(path, []byte("worker-binary"), mode); err != nil {
		t.Fatalf("write worker binary: %v", err)
	}
	return path
}

func stubExecutablePath(t *testing.T, path string) func() {
	t.Helper()
	previous := executablePathFn
	executablePathFn = func() (string, error) { return path, nil }
	return func() { executablePathFn = previous }
}

func stubHomeDir(t *testing.T, dir string) func() {
	t.Helper()
	previous := userHomeDirFn
	userHomeDirFn = func() (string, error) { return dir, nil }
	return func() { userHomeDirFn = previous }
}

func stubLookPath(t *testing.T, err error) func() {
	t.Helper()
	previous := lookPathFn
	lookPathFn = func(name string) (string, error) {
		if err != nil {
			return "", err
		}
		return name, nil
	}
	return func() { lookPathFn = previous }
}

func stubProbeWorker(t *testing.T, fn func(path string) workerProbeResult) func() {
	t.Helper()
	previous := probeWorkerBinaryFn
	probeWorkerBinaryFn = fn
	return func() { probeWorkerBinaryFn = previous }
}
