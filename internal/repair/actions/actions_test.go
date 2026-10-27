package actions

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/workspace"
)

func isolateHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", home)
	} else {
		t.Setenv("HOME", home)
	}
	return home
}

func TestInspectIsReadOnlyWithMissingRegistry(t *testing.T) {
	home := isolateHome(t)
	result, err := Inspect("config", "", "", "")
	if err != nil || !result.Needed || result.ReasonCode != "CONFIG_MISSING" {
		t.Fatalf("%+v %v", result, err)
	}
	if _, err := os.Stat(filepath.Join(home, ".mi-lsp")); !os.IsNotExist(err) {
		t.Fatalf("inspección creó estado: %v", err)
	}
	result, err = Inspect("worker", "", t.TempDir(), "")
	if err != nil || result.ReasonCode != "WORKER_MISSING" {
		t.Fatalf("%+v %v", result, err)
	}
	if _, err := os.Stat(filepath.Join(home, ".mi-lsp")); !os.IsNotExist(err) {
		t.Fatalf("worker creó estado: %v", err)
	}
}

func TestConfigApplyAndInvalidPreservation(t *testing.T) {
	isolateHome(t)
	r, err := Apply("config", "", "", "")
	if err != nil || !r.Changed {
		t.Fatalf("%+v %v", r, err)
	}
	r, err = Apply("config", "", "", "")
	if err != nil || r.Changed {
		t.Fatalf("%+v %v", r, err)
	}
	path, err := workspace.RegistryPathReadOnly()
	if err != nil {
		t.Fatal(err)
	}
	original := []byte("[invalid\n")
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	r, err = Apply("config", "", "", "")
	if err == nil || r.ReasonCode != "CONFIG_INVALID" {
		t.Fatalf("%+v %v", r, err)
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != string(original) {
		t.Fatal("configuración inválida modificada")
	}
}

func TestWorkspacePlanUnknownAliasAndRegister(t *testing.T) {
	isolateHome(t)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.invalid/demo\n\ngo 1.24\n"), 0600); err != nil {
		t.Fatal(err)
	}
	r, err := Inspect("workspace", "alias-inexistente", "", "")
	if err == nil || r.ReasonCode != "WORKSPACE_REQUIRED" {
		t.Fatalf("%+v %v", r, err)
	}
	r, err = Inspect("workspace", root, "", "")
	if err != nil || !r.Needed || r.ReasonCode != "WORKSPACE_UNREGISTERED" {
		t.Fatalf("%+v %v", r, err)
	}
	r, err = Apply("workspace", root, "", "")
	if err != nil || !r.Changed {
		t.Fatalf("%+v %v", r, err)
	}
	r, err = Inspect("workspace", root, "", "")
	if err != nil || r.Needed {
		t.Fatalf("%+v %v", r, err)
	}
	r, err = Inspect("index", root, "", "")
	if err != nil || r.ReasonCode != "INDEX_MISSING" {
		t.Fatalf("%+v %v", r, err)
	}
}

func TestRegisterPreservesUnknownRegistryFields(t *testing.T) {
	isolateHome(t)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.invalid/demo\n\ngo 1.24\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Apply("config", "", "", ""); err != nil {
		t.Fatal(err)
	}
	path, err := workspace.RegistryPathReadOnly()
	if err != nil {
		t.Fatal(err)
	}
	original := []byte("public_extension = 'preservar'\n")
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	result, err := Apply("workspace", root, "", "")
	if err == nil || result.Changed {
		t.Fatalf("%+v %v", result, err)
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != string(original) {
		t.Fatal("registro modificado")
	}
}

func TestFrameworkConfigClassification(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "runtimeconfig.json")
	for _, tc := range []struct {
		data string
		want bool
	}{
		{`{"runtimeOptions":{"framework":{"name":"Microsoft.NETCore.App","version":"10.0.0"}}}`, true},
		{`{"runtimeOptions":{"includedFrameworks":[{"name":"Microsoft.NETCore.App","version":"10.0.0"}]}}`, false},
		{`{"runtimeOptions":{"framework":{"name":"Microsoft.NETCore.App"}}}`, false},
		{`not json`, false},
	} {
		if err := os.WriteFile(path, []byte(tc.data), 0600); err != nil {
			t.Fatal(err)
		}
		if got := isFrameworkConfig(path); got != tc.want {
			t.Fatalf("%s: %v", tc.data, got)
		}
	}
}
