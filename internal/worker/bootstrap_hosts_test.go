package worker

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestPreferredCandidateUsesCompatibleDLLAndFallsBack(t *testing.T) {
	dir := t.TempDir()
	dll := filepath.Join(dir, "MiLsp.Worker.dll")
	exe := filepath.Join(dir, workerBinaryName())
	for _, path := range []string{dll, exe, filepath.Join(dir, "MiLsp.Worker.runtimeconfig.json")} {
		if err := os.WriteFile(path, []byte("fixture"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	oldLook, oldProbe := lookPathFn, probeWorkerBinaryFn
	t.Cleanup(func() { lookPathFn, probeWorkerBinaryFn = oldLook, oldProbe })
	lookPathFn = func(string) (string, error) { return "dotnet", nil }
	probeWorkerBinaryFn = func(path string) workerProbeResult { return workerProbeResult{Compatible: path == dll} }
	got, err := findPreferredCandidate([]string{dir})
	if err != nil || got != dll {
		t.Fatalf("compatible DLL: %q %v", got, err)
	}
	lookPathFn = func(string) (string, error) { return "", errors.New("missing") }
	got, err = findPreferredCandidate([]string{dir})
	if err != nil || got != exe {
		t.Fatalf("fallback: %q %v", got, err)
	}
}

func TestInstallRejectsUnknownMode(t *testing.T) {
	_, err := InstallWorkerMode(t.TempDir(), "test-rid", "bad-mode")
	if err == nil {
		t.Fatal("expected invalid mode error")
	}
}
