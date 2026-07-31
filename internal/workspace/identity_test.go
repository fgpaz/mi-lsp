package workspace

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

func TestResolveWorkspaceSelectionReadOnlyFailsClosedForExplicitSelectors(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	caller := t.TempDir()
	stale := filepath.Join(t.TempDir(), "stale")
	registerTestWorkspace(t, "live", t.TempDir())
	registerTestWorkspace(t, "stale", stale)

	for _, resolve := range []struct {
		name string
		fn   func(string, string) (WorkspaceResolution, error)
	}{
		{name: "normal", fn: ResolveWorkspaceSelection},
		{name: "read-only", fn: ResolveWorkspaceSelectionReadOnly},
	} {
		t.Run(resolve.name, func(t *testing.T) {
			if _, err := resolve.fn("missing", caller); err == nil {
				t.Fatal("missing explicit alias should fail closed")
			} else {
				var selectorErr *WorkspaceSelectorError
				if !errors.As(err, &selectorErr) || selectorErr.Code != WorkspaceSelectorNotFound {
					t.Fatalf("error = %v, want %s", err, WorkspaceSelectorNotFound)
				}
			}
		})
	}
	for _, resolve := range []struct {
		name string
		fn   func(string, string) (WorkspaceResolution, error)
	}{
		{name: "normal", fn: ResolveWorkspaceSelection},
		{name: "read-only", fn: ResolveWorkspaceSelectionReadOnly},
	} {
		t.Run(resolve.name+" stale", func(t *testing.T) {
			if _, err := resolve.fn("stale", caller); err == nil {
				t.Fatal("stale explicit alias should fail closed")
			} else {
				var selectorErr *WorkspaceSelectorError
				if !errors.As(err, &selectorErr) || selectorErr.Code != WorkspaceSelectorStale {
					t.Fatalf("error = %v, want %s", err, WorkspaceSelectorStale)
				}
			}
		})
	}
}

func TestWorkspaceIdentitySeparatesDisplayAndCanonicalSymlink(t *testing.T) {
	root := t.TempDir()
	link := filepath.Join(t.TempDir(), "workspace-link")
	if err := os.Symlink(root, link); err != nil {
		t.Skipf("symlink unavailable on %s: %v", runtime.GOOS, err)
	}
	identity, err := InspectWorkspaceIdentity(link)
	if err != nil {
		t.Fatalf("InspectWorkspaceIdentity: %v", err)
	}
	if identity.DisplayRoot == identity.CanonicalRoot {
		t.Fatalf("display root %q should preserve link spelling distinct from canonical %q", identity.DisplayRoot, identity.CanonicalRoot)
	}
	want, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatalf("EvalSymlinks(root): %v", err)
	}
	if identity.CanonicalRoot != filepath.Clean(want) {
		t.Fatalf("canonical root = %q, want %q", identity.CanonicalRoot, filepath.Clean(want))
	}
}

func TestWorkspaceIdentityJunctionResolvesPhysicalRootOnWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("directory junctions are Windows-specific")
	}
	root := t.TempDir()
	link := filepath.Join(t.TempDir(), "workspace-junction")
	command := exec.Command("cmd", "/c", "mklink", "/J", link, root)
	if output, err := command.CombinedOutput(); err != nil {
		t.Skipf("junction unavailable: %v: %s", err, strings.TrimSpace(string(output)))
	}
	identity, err := InspectWorkspaceIdentity(link)
	if err != nil {
		t.Fatalf("InspectWorkspaceIdentity: %v", err)
	}
	if !identity.Exists || identity.CanonicalRoot == "" {
		t.Fatalf("junction identity = %#v, want an existing canonical root", identity)
	}
	want, err := filepath.EvalSymlinks(link)
	if err != nil {
		t.Fatalf("EvalSymlinks(junction): %v", err)
	}
	// Windows hosts differ in whether EvalSymlinks exposes a junction's
	// target. Assert the physical target only when the host exposes it.
	if filepath.Clean(want) != identity.DisplayRoot && identity.CanonicalRoot != filepath.Clean(want) {
		t.Fatalf("canonical root = %q, want evaluated junction %q", identity.CanonicalRoot, filepath.Clean(want))
	}
}

func TestWorkspaceIdentityCasingFollowsPlatform(t *testing.T) {
	root := t.TempDir()
	variant := strings.ToUpper(root)
	left, ok := ComparableWorkspacePath(root)
	if !ok {
		t.Fatal("root path should be comparable")
	}
	right, ok := ComparableWorkspacePath(variant)
	if !ok {
		t.Fatal("variant path should be comparable")
	}
	if IsCaseInsensitivePlatform() && left != right {
		t.Fatalf("case-insensitive platform paths differ: %q != %q", left, right)
	}
	if !IsCaseInsensitivePlatform() && left == right && root != variant {
		t.Fatalf("case-sensitive platform collapsed casing: %q", left)
	}
}

func TestWorkspaceIdentityKeepsRegistrationModelCompatible(t *testing.T) {
	root := t.TempDir()
	registration := model.WorkspaceRegistration{Name: "demo", Root: root}
	identity, err := InspectWorkspaceIdentity(registration.Root)
	if err != nil || !identity.Exists {
		t.Fatalf("identity = %#v err=%v", identity, err)
	}
}

func TestResolveWorkspaceSelectionReadOnlyRejectsFileRoot(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	root := t.TempDir()
	file := filepath.Join(root, "root.txt")
	if err := os.WriteFile(file, []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := RegisterWorkspace("file-root", model.WorkspaceRegistration{Name: "file-root", Root: file}); err != nil {
		t.Fatalf("RegisterWorkspace: %v", err)
	}
	_, err := ResolveWorkspaceSelectionReadOnly("file-root", root)
	if err == nil {
		t.Fatal("file root should fail")
	}
	var selectorErr *WorkspaceSelectorError
	if !errors.As(err, &selectorErr) || selectorErr.Code != WorkspaceSelectorNotDirectory {
		t.Fatalf("error = %v, want %s", err, WorkspaceSelectorNotDirectory)
	}
}
