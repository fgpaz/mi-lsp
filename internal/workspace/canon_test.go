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

func TestCanonEscapeMax(t *testing.T) {
	t.Parallel()
	zero := 0
	two := 2
	tests := []struct {
		name    string
		project model.ProjectFile
		want    int
	}{
		{name: "omitted policy defaults to 1", project: model.ProjectFile{}, want: 1},
		{name: "nil escape_max defaults to 1", project: model.ProjectFile{CanonPolicy: &model.CanonPolicyBlock{}}, want: 1},
		{name: "explicit 0 disables parent escape", project: model.ProjectFile{CanonPolicy: &model.CanonPolicyBlock{EscapeMax: &zero}}, want: 0},
		{name: "explicit 2", project: model.ProjectFile{CanonPolicy: &model.CanonPolicyBlock{EscapeMax: &two}}, want: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CanonEscapeMax(tt.project); got != tt.want {
				t.Fatalf("CanonEscapeMax() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestValidateCanonDeclarationsEmpty(t *testing.T) {
	t.Parallel()
	if err := ValidateCanonDeclarations(model.ProjectFile{}); err != nil {
		t.Fatalf("empty Canons should be valid, got %v", err)
	}
}

func TestResolveCanonsEmpty(t *testing.T) {
	t.Parallel()
	got, err := ResolveCanons(t.TempDir(), model.ProjectFile{})
	if err != nil {
		t.Fatalf("ResolveCanons empty: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("ResolveCanons empty = %#v, want empty slice", got)
	}
}

func TestResolveCanonsRelativeFromWorkspaceRoot(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	workspaceRoot := filepath.Join(base, "code")
	canonDir := filepath.Join(base, "wiki-repo", "Ingenieria")
	mustCreateDir(t, workspaceRoot)
	mustCreateDir(t, canonDir)

	declared := "../wiki-repo/Ingenieria"
	project := model.ProjectFile{
		Canons: []model.WorkspaceCanon{{
			ID:   "wiki",
			Root: declared,
			Role: "producto",
		}},
	}
	got, err := ResolveCanons(workspaceRoot, project)
	if err != nil {
		t.Fatalf("ResolveCanons: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0].DeclaredRoot != declared {
		t.Fatalf("DeclaredRoot = %q, want slash path %q", got[0].DeclaredRoot, declared)
	}
	if strings.Contains(got[0].DeclaredRoot, workspaceRoot) || filepath.IsAbs(got[0].DeclaredRoot) {
		t.Fatalf("DeclaredRoot leaked an absolute path: %q", got[0].DeclaredRoot)
	}
	if got[0].ID != "wiki" || got[0].Role != "producto" || got[0].Mode != "read-only" {
		t.Fatalf("resolved = %#v", got[0])
	}
	if !testCanonPathsEqual(got[0].AbsRoot, canonDir) {
		t.Fatalf("AbsRoot = %q, want %q", got[0].AbsRoot, canonDir)
	}
}

func TestValidateCanonDeclarationsRejectsAbsoluteRoots(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		root string
	}{
		{name: "unix", root: "/opt/wiki"},
		{name: "windows volume", root: `C:\wiki-repo\Ingenieria`},
		{name: "windows volume slash", root: "C:/wiki-repo/Ingenieria"},
		{name: "unc", root: `\\server\share\wiki`},
		{name: "unc forward", root: "//server/share/wiki"},
		{name: "home", root: "~/wiki"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCanonDeclarations(model.ProjectFile{
				Canons: []model.WorkspaceCanon{{ID: "wiki", Root: tt.root, Role: "producto"}},
			})
			if err == nil || !errors.Is(err, ErrCanonRootAbsolute) {
				t.Fatalf("error = %v, want %v", err, ErrCanonRootAbsolute)
			}
			declared := declaredCanonRoot(tt.root)
			if !strings.Contains(err.Error(), declared) {
				t.Fatalf("error %q should include declared path %q", err, declared)
			}
			if !strings.Contains(err.Error(), "../sibling") {
				t.Fatalf("error %q should include a repair hint", err)
			}
		})
	}
}

func TestValidateCanonDeclarationsEscapeMax(t *testing.T) {
	t.Parallel()
	declared := "../../wiki"
	err := ValidateCanonDeclarations(model.ProjectFile{
		Canons: []model.WorkspaceCanon{{ID: "wiki", Root: declared, Role: "producto"}},
	})
	if err == nil || !errors.Is(err, ErrCanonRootEscape) {
		t.Fatalf("default max=1 error = %v, want %v", err, ErrCanonRootEscape)
	}
	if !strings.Contains(err.Error(), declared) {
		t.Fatalf("error %q should include declared path", err)
	}
	if !strings.Contains(err.Error(), "max is 1") {
		t.Fatalf("error %q should include the max", err)
	}
	if !strings.Contains(err.Error(), "[canon_policy].escape_max") {
		t.Fatalf("error %q should include a repair hint", err)
	}

	two := 2
	if err := ValidateCanonDeclarations(model.ProjectFile{
		CanonPolicy: &model.CanonPolicyBlock{EscapeMax: &two},
		Canons:      []model.WorkspaceCanon{{ID: "wiki", Root: declared, Role: "producto"}},
	}); err != nil {
		t.Fatalf("escape_max=2 should accept %q: %v", declared, err)
	}

	zero := 0
	err = ValidateCanonDeclarations(model.ProjectFile{
		CanonPolicy: &model.CanonPolicyBlock{EscapeMax: &zero},
		Canons:      []model.WorkspaceCanon{{ID: "wiki", Root: "../wiki-repo/Ingenieria", Role: "producto"}},
	})
	if err == nil || !errors.Is(err, ErrCanonRootEscape) {
		t.Fatalf("escape_max=0 error = %v, want %v", err, ErrCanonRootEscape)
	}
}

func TestResolveCanonsEscapeMaxTwo(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	workspaceRoot := filepath.Join(base, "mid", "code")
	canonDir := filepath.Join(base, "wiki")
	mustCreateDir(t, workspaceRoot)
	mustCreateDir(t, canonDir)
	two := 2
	declared := "../../wiki"
	got, err := ResolveCanons(workspaceRoot, model.ProjectFile{
		CanonPolicy: &model.CanonPolicyBlock{EscapeMax: &two},
		Canons:      []model.WorkspaceCanon{{ID: "wiki", Root: declared, Role: "ecosistema"}},
	})
	if err != nil {
		t.Fatalf("ResolveCanons: %v", err)
	}
	if len(got) != 1 || got[0].DeclaredRoot != declared {
		t.Fatalf("resolved = %#v", got)
	}
	if !testCanonPathsEqual(got[0].AbsRoot, canonDir) {
		t.Fatalf("AbsRoot = %q, want %q", got[0].AbsRoot, canonDir)
	}
}

func TestValidateCanonDeclarationsIdentityRoleMode(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		canons  []model.WorkspaceCanon
		wantErr error
	}{
		{
			name:    "missing id",
			canons:  []model.WorkspaceCanon{{Root: "../wiki-repo/Ingenieria", Role: "producto"}},
			wantErr: ErrCanonIDMissing,
		},
		{
			name: "duplicate ids case-insensitive",
			canons: []model.WorkspaceCanon{
				{ID: "Wiki", Root: "../wiki-a", Role: "producto"},
				{ID: "wiki", Root: "../wiki-b", Role: "ecosistema"},
			},
			wantErr: ErrCanonIDDuplicate,
		},
		{
			name:    "bad role",
			canons:  []model.WorkspaceCanon{{ID: "wiki", Root: "../wiki-repo/Ingenieria", Role: "docs"}},
			wantErr: ErrCanonRoleInvalid,
		},
		{
			name:    "empty role",
			canons:  []model.WorkspaceCanon{{ID: "wiki", Root: "../wiki-repo/Ingenieria"}},
			wantErr: ErrCanonRoleInvalid,
		},
		{
			name:    "mode write",
			canons:  []model.WorkspaceCanon{{ID: "wiki", Root: "../wiki-repo/Ingenieria", Role: "producto", Mode: "write"}},
			wantErr: ErrCanonModeInvalid,
		},
		{
			name:   "omitted mode is valid",
			canons: []model.WorkspaceCanon{{ID: "wiki", Root: "../wiki-repo/Ingenieria", Role: "gobierno_local"}},
		},
		{
			name:   "read-only mode is valid",
			canons: []model.WorkspaceCanon{{ID: "wiki", Root: "../wiki-repo/Ingenieria", Role: "producto", Mode: "read-only"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCanonDeclarations(model.ProjectFile{Canons: tt.canons})
			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("ValidateCanonDeclarations: %v", err)
				}
				return
			}
			if err == nil || !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestResolveCanonsOmittedModeIsReadOnly(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	workspaceRoot := filepath.Join(base, "code")
	canonDir := filepath.Join(base, "wiki-repo", "Ingenieria")
	mustCreateDir(t, workspaceRoot)
	mustCreateDir(t, canonDir)
	got, err := ResolveCanons(workspaceRoot, model.ProjectFile{
		Canons: []model.WorkspaceCanon{{ID: "wiki", Root: "../wiki-repo/Ingenieria", Role: "producto"}},
	})
	if err != nil {
		t.Fatalf("ResolveCanons: %v", err)
	}
	if len(got) != 1 || got[0].Mode != "read-only" {
		t.Fatalf("mode = %#v, want read-only", got)
	}
}

func TestResolveCanonsMissingUsesDeclaredPath(t *testing.T) {
	t.Parallel()
	workspaceRoot := t.TempDir()
	declared := "../wiki-repo/Ingenieria"
	_, err := ResolveCanons(workspaceRoot, model.ProjectFile{
		Canons: []model.WorkspaceCanon{{ID: "wiki", Root: declared, Role: "producto"}},
	})
	if err == nil || !errors.Is(err, ErrCanonRootMissing) {
		t.Fatalf("error = %v, want %v", err, ErrCanonRootMissing)
	}
	if !strings.Contains(err.Error(), declared) {
		t.Fatalf("error %q should include declared path", err)
	}
	abs := filepath.Clean(filepath.Join(workspaceRoot, filepath.FromSlash(declared)))
	if strings.Contains(err.Error(), abs) {
		t.Fatalf("error %q should not include absolute path %q", err, abs)
	}
}

func TestResolveCanonsRejectsSymlinkComponent(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	workspaceRoot := filepath.Join(base, "code")
	realDir := filepath.Join(base, "wiki-real")
	link := filepath.Join(base, "wiki-repo")
	mustCreateDir(t, workspaceRoot)
	mustCreateDir(t, filepath.Join(realDir, "Ingenieria"))
	if err := os.Symlink(realDir, link); err != nil {
		t.Skipf("symlink unavailable on %s: %v", runtime.GOOS, err)
	}
	declared := "../wiki-repo/Ingenieria"
	_, err := ResolveCanons(workspaceRoot, model.ProjectFile{
		Canons: []model.WorkspaceCanon{{ID: "wiki", Root: declared, Role: "producto"}},
	})
	if err == nil || !errors.Is(err, ErrCanonRootSymlink) {
		t.Fatalf("error = %v, want %v", err, ErrCanonRootSymlink)
	}
	if !strings.Contains(err.Error(), declared) {
		t.Fatalf("error %q should include declared path", err)
	}
}

func TestResolveCanonsRejectsJunctionComponent(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("directory junctions are Windows-specific")
	}
	base := t.TempDir()
	workspaceRoot := filepath.Join(base, "code")
	realDir := filepath.Join(base, "wiki-real")
	link := filepath.Join(base, "wiki-repo")
	mustCreateDir(t, workspaceRoot)
	mustCreateDir(t, filepath.Join(realDir, "Ingenieria"))
	command := exec.Command("cmd", "/c", "mklink", "/J", link, realDir)
	if output, err := command.CombinedOutput(); err != nil {
		t.Skipf("junction unavailable: %v: %s", err, strings.TrimSpace(string(output)))
	}
	declared := "../wiki-repo/Ingenieria"
	_, err := ResolveCanons(workspaceRoot, model.ProjectFile{
		Canons: []model.WorkspaceCanon{{ID: "wiki", Root: declared, Role: "producto"}},
	})
	if err == nil || !errors.Is(err, ErrCanonRootSymlink) {
		t.Fatalf("error = %v, want %v", err, ErrCanonRootSymlink)
	}
	if !strings.Contains(err.Error(), declared) {
		t.Fatalf("error %q should include declared path", err)
	}
}

func TestLoadSaveProjectFileRoundTripPreservesCanon(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	zero := 0
	project := model.ProjectFile{
		Project: model.ProjectBlock{Name: "demo", Kind: model.WorkspaceKindSingle},
		Canons: []model.WorkspaceCanon{
			{ID: "wiki", Root: "../wiki-repo/Ingenieria", Role: "producto"},
			{ID: "gov", Root: "../gobierno", Role: "gobierno_local", Mode: "read-only"},
		},
		CanonPolicy: &model.CanonPolicyBlock{EscapeMax: &zero},
	}
	if err := SaveProjectFile(root, project); err != nil {
		t.Fatalf("SaveProjectFile: %v", err)
	}
	got, err := LoadProjectFile(root)
	if err != nil {
		t.Fatalf("LoadProjectFile: %v", err)
	}
	if len(got.Canons) != 2 {
		t.Fatalf("canons = %#v", got.Canons)
	}
	if got.Canons[0].ID != "wiki" || got.Canons[0].Root != "../wiki-repo/Ingenieria" || got.Canons[0].Role != "producto" || got.Canons[0].Mode != "" {
		t.Fatalf("canon[0] = %#v", got.Canons[0])
	}
	if got.Canons[1].ID != "gov" || got.Canons[1].Root != "../gobierno" || got.Canons[1].Role != "gobierno_local" || got.Canons[1].Mode != "read-only" {
		t.Fatalf("canon[1] = %#v", got.Canons[1])
	}
	if got.CanonPolicy == nil || got.CanonPolicy.EscapeMax == nil || *got.CanonPolicy.EscapeMax != 0 {
		t.Fatalf("CanonPolicy = %#v, want escape_max=0", got.CanonPolicy)
	}
	if filepath.IsAbs(got.Canons[0].Root) {
		t.Fatalf("round-trip leaked absolute root: %q", got.Canons[0].Root)
	}
}

func TestMergeProjectFilePreservesCanons(t *testing.T) {
	t.Parallel()
	zero := 0
	existing := model.ProjectFile{
		Project:     model.ProjectBlock{Name: "demo"},
		Canons:      []model.WorkspaceCanon{{ID: "wiki", Root: "../wiki-repo/Ingenieria", Role: "producto"}},
		CanonPolicy: &model.CanonPolicyBlock{EscapeMax: &zero},
	}
	detected := model.ProjectFile{
		Project: model.ProjectBlock{Name: "detected", Languages: []string{"go"}},
		Canons:  []model.WorkspaceCanon{{ID: "should-not-copy", Root: ".", Role: "ecosistema"}},
	}
	got := mergeProjectFile(existing, detected)
	if len(got.Canons) != 1 || got.Canons[0].ID != "wiki" || got.Canons[0].Root != "../wiki-repo/Ingenieria" {
		t.Fatalf("canons = %#v", got.Canons)
	}
	if got.CanonPolicy == nil || got.CanonPolicy.EscapeMax == nil || *got.CanonPolicy.EscapeMax != 0 {
		t.Fatalf("CanonPolicy = %#v", got.CanonPolicy)
	}
}

func TestResolveCanonsByRole(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	workspaceRoot := filepath.Join(base, "code")
	producto := filepath.Join(base, "wiki-repo", "Ingenieria")
	gobierno := filepath.Join(base, "gobierno")
	mustCreateDir(t, workspaceRoot)
	mustCreateDir(t, producto)
	mustCreateDir(t, gobierno)
	project := model.ProjectFile{
		Canons: []model.WorkspaceCanon{
			{ID: "wiki", Root: "../wiki-repo/Ingenieria", Role: "producto"},
			{ID: "gov", Root: "../gobierno", Role: "gobierno_local"},
		},
	}

	all, err := ResolveCanonsByRole(workspaceRoot, project, "")
	if err != nil || len(all) != 2 {
		t.Fatalf("empty role = %#v err=%v", all, err)
	}
	got, err := ResolveCanonsByRole(workspaceRoot, project, "producto")
	if err != nil || len(got) != 1 || got[0].ID != "wiki" {
		t.Fatalf("role producto = %#v err=%v", got, err)
	}
	_, err = ResolveCanonsByRole(workspaceRoot, project, "ecosistema")
	if err == nil {
		t.Fatal("zero matches should fail closed")
	}
	if !strings.Contains(err.Error(), "wiki") || !strings.Contains(err.Error(), "gov") {
		t.Fatalf("error %q should list available ids", err)
	}
}

func TestPathIsReadOnlyCanon(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	workspaceRoot := filepath.Join(base, "code")
	canonDir := filepath.Join(base, "wiki-repo", "Ingenieria")
	mustCreateDir(t, workspaceRoot)
	mustCreateDir(t, filepath.Join(canonDir, "nested"))
	project := model.ProjectFile{
		Canons: []model.WorkspaceCanon{{ID: "wiki", Root: "../wiki-repo/Ingenieria", Role: "producto"}},
	}

	inside, err := PathIsReadOnlyCanon(workspaceRoot, project, filepath.Join(canonDir, "nested", "doc.md"))
	if err != nil || !inside {
		t.Fatalf("inside = %v err=%v, want true", inside, err)
	}
	rootHit, err := PathIsReadOnlyCanon(workspaceRoot, project, canonDir)
	if err != nil || !rootHit {
		t.Fatalf("root = %v err=%v, want true", rootHit, err)
	}
	outside, err := PathIsReadOnlyCanon(workspaceRoot, project, filepath.Join(workspaceRoot, "main.go"))
	if err != nil || outside {
		t.Fatalf("outside = %v err=%v, want false", outside, err)
	}
	neighbor, err := PathIsReadOnlyCanon(workspaceRoot, project, filepath.Join(base, "wiki-repo", "Ingenieria-extra"))
	if err != nil || neighbor {
		t.Fatalf("neighbor prefix = %v err=%v, want false", neighbor, err)
	}

	empty, err := PathIsReadOnlyCanon(workspaceRoot, model.ProjectFile{}, filepath.Join(workspaceRoot, "main.go"))
	if err != nil || empty {
		t.Fatalf("no canons = %v err=%v, want false", empty, err)
	}

	_, err = PathIsReadOnlyCanon(workspaceRoot, model.ProjectFile{
		Canons: []model.WorkspaceCanon{{ID: "wiki", Root: "/abs/wiki", Role: "producto"}},
	}, filepath.Join(workspaceRoot, "main.go"))
	if err == nil || !errors.Is(err, ErrCanonRootAbsolute) {
		t.Fatalf("invalid canons should fail closed, err=%v", err)
	}
}

func testCanonPathsEqual(left, right string) bool {
	leftAbs, err := filepath.Abs(left)
	if err != nil {
		return false
	}
	rightAbs, err := filepath.Abs(right)
	if err != nil {
		return false
	}
	leftAbs = filepath.Clean(leftAbs)
	rightAbs = filepath.Clean(rightAbs)
	if IsCaseInsensitivePlatform() {
		return strings.EqualFold(leftAbs, rightAbs)
	}
	return leftAbs == rightAbs
}
