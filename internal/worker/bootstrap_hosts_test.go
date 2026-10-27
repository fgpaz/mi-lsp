package worker

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
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
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", home)
	} else {
		t.Setenv("HOME", home)
	}
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

func TestInstallDoesNotRecordIncompatibleWorker(t *testing.T) {
	home, root := t.TempDir(), t.TempDir()
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", home)
	} else {
		t.Setenv("HOME", home)
	}
	oldHome, oldProbe := userHomeDirFn, probeWorkerBinaryFn
	t.Cleanup(func() { userHomeDirFn, probeWorkerBinaryFn = oldHome, oldProbe })
	userHomeDirFn = func() (string, error) { return home, nil }
	probeWorkerBinaryFn = func(string) workerProbeResult { return workerProbeResult{Err: errors.New("incompatible")} }
	dir := filepath.Join(root, "workers", ResolveRID())
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, workerBinaryName()), []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	path, err := InstallWorkerMode(root, ResolveRID(), "self-contained")
	if err == nil || path != "" {
		t.Fatalf("verificación previa debe fallar sin publicar: %q %v", path, err)
	}
	if _, err := os.Stat(filepath.Join(home, ".mi-lsp", "repair", "installations.json")); !os.IsNotExist(err) {
		t.Fatalf("recibo inesperado: %v", err)
	}
}

func TestPublishWorkerPreservesPriorInstallation(t *testing.T) {
	for _, scenario := range []string{"copy", "probe", "promotion", "rollback"} {
		t.Run(scenario, func(t *testing.T) {
			parent := t.TempDir()
			source, target := filepath.Join(parent, "source"), filepath.Join(parent, "installed")
			for _, dir := range []string{source, target} {
				if err := os.MkdirAll(dir, 0755); err != nil {
					t.Fatal(err)
				}
			}
			name := workerBinaryName()
			if err := os.WriteFile(filepath.Join(source, name), []byte("nuevo"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(target, name), []byte("anterior"), 0600); err != nil {
				t.Fatal(err)
			}
			oldCopy, oldProbe, oldRename := copyDistributionFn, probeWorkerBinaryFn, renameDistributionFn
			t.Cleanup(func() { copyDistributionFn, probeWorkerBinaryFn, renameDistributionFn = oldCopy, oldProbe, oldRename })
			probeWorkerBinaryFn = func(string) workerProbeResult { return workerProbeResult{Compatible: scenario != "probe"} }
			if scenario == "copy" {
				copyDistributionFn = func(string, string) error { return errors.New("copy failed") }
			}
			if scenario == "promotion" || scenario == "rollback" {
				calls := 0
				renameDistributionFn = func(from, to string) error {
					calls++
					if calls == 2 || (scenario == "rollback" && calls == 3) {
						return errors.New("rename failed")
					}
					return os.Rename(from, to)
				}
			}
			path, err := publishWorkerDistribution(source, target, name)
			if err == nil {
				t.Fatal("se esperaba error")
			}
			if scenario == "rollback" {
				if path == "" {
					t.Fatal("mutación parcial debe devolver ruta")
				}
				backups, err := filepath.Glob(filepath.Join(parent, ".worker-backup-*", "previous", name))
				if err != nil || len(backups) != 1 {
					t.Fatalf("backup no conservado: %v %v", backups, err)
				}
				body, err := os.ReadFile(backups[0])
				if err != nil || string(body) != "anterior" {
					t.Fatalf("backup: %q %v", body, err)
				}
			} else {
				if path != "" {
					t.Fatalf("fallo sin mutación devolvió ruta: %q", path)
				}
				body, err := os.ReadFile(filepath.Join(target, name))
				if err != nil || string(body) != "anterior" {
					t.Fatalf("instalación anterior: %q %v", body, err)
				}
			}
		})
	}
}

func TestInstallRejectsUnknownMode(t *testing.T) {
	_, err := InstallWorkerMode(t.TempDir(), "test-rid", "bad-mode")
	if err == nil {
		t.Fatal("expected invalid mode error")
	}
}
