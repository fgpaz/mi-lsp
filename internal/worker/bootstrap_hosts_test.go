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
	for _, path := range []string{dll, exe} {
		if err := os.WriteFile(path, []byte("fixture"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	config := filepath.Join(dir, "MiLsp.Worker.runtimeconfig.json")
	if err := os.WriteFile(config, []byte(`{"runtimeOptions":{"framework":{"name":"Microsoft.NETCore.App","version":"10.0.0"}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	oldLook, oldProbe := lookPathFn, probeWorkerBinaryFn
	t.Cleanup(func() { lookPathFn, probeWorkerBinaryFn = oldLook, oldProbe })
	lookPathFn = func(string) (string, error) { return "dotnet", nil }
	probeWorkerBinaryFn = func(path string) workerProbeResult { return workerProbeResult{Compatible: path == dll} }
	got, err := findPreferredCandidate([]string{dir})
	if err != nil || got != dll {
		t.Fatalf("compatible DLL: %q %v", got, err)
	}
	probeWorkerBinaryFn = func(string) workerProbeResult { return workerProbeResult{Err: errors.New("incompatible runtime")} }
	got, err = findPreferredCandidate([]string{dir})
	if err != nil || got != exe {
		t.Fatalf("incompatible runtime fallback: %q %v", got, err)
	}
	probeWorkerBinaryFn = func(string) workerProbeResult { return workerProbeResult{Compatible: true} }
	if err := os.WriteFile(config, []byte(`{"runtimeOptions":{"includedFrameworks":[{"name":"Microsoft.NETCore.App","version":"10.0.0"}]}}`), 0600); err != nil {
		t.Fatal(err)
	}
	got, err = findPreferredCandidate([]string{dir})
	if err != nil || got != exe {
		t.Fatalf("self-contained fallback: %q %v", got, err)
	}
	if mode, host := candidateMode(exe); mode != "self-contained" || host != "apphost" {
		t.Fatalf("mode=%s host=%s", mode, host)
	}
	lookPathFn = func(string) (string, error) { return "", errors.New("missing") }
	got, err = findPreferredCandidate([]string{dir})
	if err != nil || got != exe {
		t.Fatalf("fallback: %q %v", got, err)
	}
}

func TestRuntimeConfigFrameworkDependent(t *testing.T) {
	cases := []struct {
		name, body string
		want       bool
	}{
		{"framework", `{"runtimeOptions":{"framework":{"name":"Microsoft.NETCore.App","version":"10.0.0"}}}`, true},
		{"frameworks", `{"runtimeOptions":{"frameworks":[{"name":"Microsoft.NETCore.App","version":"10.0.0"},{"name":"Microsoft.AspNetCore.App","version":"10.0.0"}]}}`, true},
		{"self-contained", `{"runtimeOptions":{"includedFrameworks":[{"name":"Microsoft.NETCore.App","version":"10.0.0"}]}}`, false},
		{"mixed", `{"runtimeOptions":{"framework":{"name":"Microsoft.NETCore.App","version":"10.0.0"},"includedFrameworks":[]}}`, false},
		{"missing version", `{"runtimeOptions":{"framework":{"name":"Microsoft.NETCore.App"}}}`, false},
		{"empty", `{"runtimeOptions":{}}`, false},
		{"malformed", `{`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "runtimeconfig.json")
			if err := os.WriteFile(path, []byte(tc.body), 0600); err != nil {
				t.Fatal(err)
			}
			if got := runtimeConfigIsFrameworkDependent(path); got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSelectionMatchesLaunchSpecs(t *testing.T) {
	root, home := t.TempDir(), t.TempDir()
	oldHome, oldLook, oldProbe := userHomeDirFn, lookPathFn, probeWorkerBinaryFn
	t.Cleanup(func() { userHomeDirFn, lookPathFn, probeWorkerBinaryFn = oldHome, oldLook, oldProbe })
	userHomeDirFn = func() (string, error) { return home, nil }
	lookPathFn = func(string) (string, error) { return "dotnet", nil }
	probeWorkerBinaryFn = func(string) workerProbeResult { return workerProbeResult{Compatible: true} }
	bundle := filepath.Join(root, "workers", ResolveRID())
	installed := filepath.Join(home, ".mi-lsp", "workers", ResolveRID())
	for _, dir := range []string{bundle, installed} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(bundle, workerBinaryName()), []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bundle, "MiLsp.Worker.runtimeconfig.json"), []byte(`{"runtimeOptions":{"includedFrameworks":[]}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(installed, "MiLsp.Worker.dll"), []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(installed, "MiLsp.Worker.runtimeconfig.json"), []byte(`{"runtimeOptions":{"framework":{"name":"Microsoft.NETCore.App","version":"10.0.0"}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	status := InspectWorkerRuntime(root, ResolveRID())
	specs, err := ResolveLaunchSpecs(root)
	if err != nil {
		t.Fatal(err)
	}
	if status.Selected.Source != "installed" || status.Selected.Mode != "framework-dependent" || specs[0].CandidatePath != status.Selected.Path {
		t.Fatalf("selected=%+v first=%+v", status.Selected, specs[0])
	}
}

func TestSelfContainedInstallKeepsApphost(t *testing.T) {
	root, home := t.TempDir(), t.TempDir()
	oldHome, oldLook, oldProbe := userHomeDirFn, lookPathFn, probeWorkerBinaryFn
	t.Cleanup(func() { userHomeDirFn, lookPathFn, probeWorkerBinaryFn = oldHome, oldLook, oldProbe })
	userHomeDirFn = func() (string, error) { return home, nil }
	lookPathFn = func(string) (string, error) { return "dotnet", nil }
	probeWorkerBinaryFn = func(string) workerProbeResult { return workerProbeResult{Compatible: true} }
	bundle := filepath.Join(root, "workers", ResolveRID())
	if err := os.MkdirAll(bundle, 0755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{workerBinaryName(): "apphost", "MiLsp.Worker.dll": "dll", "MiLsp.Worker.runtimeconfig.json": `{"runtimeOptions":{"includedFrameworks":[{"name":"Microsoft.NETCore.App","version":"10.0.0"}]}}`} {
		if err := os.WriteFile(filepath.Join(bundle, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	path, err := InstallWorkerMode(root, ResolveRID(), "self-contained")
	if err != nil {
		t.Fatal(err)
	}
	status := InspectWorkerRuntime(root, ResolveRID())
	specs, err := ResolveLaunchSpecs(root)
	if err != nil {
		t.Fatal(err)
	}
	if path != status.Installed.Path || status.Installed.Mode != "self-contained" || status.Selected.Host != "apphost" || specs[0].CandidatePath != status.Selected.Path {
		t.Fatalf("path=%s status=%+v first=%+v", path, status, specs[0])
	}
}

func TestInstallRejectsUnknownMode(t *testing.T) {
	_, err := InstallWorkerMode(t.TempDir(), "test-rid", "bad-mode")
	if err == nil {
		t.Fatal("expected invalid mode error")
	}
}
