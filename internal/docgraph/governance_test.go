package docgraph

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

func TestInspectGovernanceBlocksSourceDocOutsideWorkspace(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.md")
	if err := os.WriteFile(outside, []byte("# Outside\n"), 0o644); err != nil {
		t.Fatalf("write outside doc: %v", err)
	}
	writeReadModel(t, root, "../../"+filepath.Base(outside))

	status := InspectGovernance(root, false)
	if !status.Blocked {
		t.Fatalf("Blocked = false, want true")
	}
	if status.Sync != "invalid" {
		t.Fatalf("Sync = %q, want invalid", status.Sync)
	}
	if !strings.HasPrefix(status.HumanDoc, "INVALID:") {
		t.Fatalf("HumanDoc = %q, want invalid marker", status.HumanDoc)
	}
	if !strings.Contains(strings.Join(status.Issues, " "), "source_doc") {
		t.Fatalf("Issues = %v, want source_doc guidance", status.Issues)
	}
	joined := strings.Join(status.Issues, " ")
	if strings.Contains(joined, "must be under .docs/wiki/") {
		t.Fatalf("Issues = %v, must not require .docs/wiki/ prefix", status.Issues)
	}
	if !strings.Contains(joined, "[[canon]]") {
		t.Fatalf("Issues = %v, want [[canon]] guidance", status.Issues)
	}
}

func TestSafeGovernanceSourceDocAllowsRelativeWikiPath(t *testing.T) {
	root := t.TempDir()
	got, ok := safeGovernanceSourceDoc(root, ".docs/wiki/00_gobierno_documental.md", nil)
	if !ok {
		t.Fatal("safeGovernanceSourceDoc ok = false, want true")
	}
	if got != ".docs/wiki/00_gobierno_documental.md" {
		t.Fatalf("safe path = %q, want canonical wiki path", got)
	}
}

func TestSafeGovernanceSourceDocAllowsInWorkspaceNonWiki(t *testing.T) {
	root := t.TempDir()
	got, ok := safeGovernanceSourceDoc(root, "Ingenieria/00_gobierno_documental.md", nil)
	if !ok {
		t.Fatal("safeGovernanceSourceDoc ok = false, want true")
	}
	if got != "Ingenieria/00_gobierno_documental.md" {
		t.Fatalf("safe path = %q, want in-workspace Ingenieria path", got)
	}
}

func TestSafeGovernanceSourceDocAllowsDeclaredCanonRoot(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "code")
	canon := filepath.Join(parent, "wiki-repo", "Ingenieria")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	if err := os.MkdirAll(canon, 0o755); err != nil {
		t.Fatalf("mkdir canon: %v", err)
	}
	canons, err := workspace.ResolveCanons(root, model.ProjectFile{
		Canons: []model.WorkspaceCanon{{ID: "wiki", Root: "../wiki-repo/Ingenieria", Role: "producto"}},
	})
	if err != nil {
		t.Fatalf("ResolveCanons: %v", err)
	}
	got, ok := safeGovernanceSourceDoc(root, "../wiki-repo/Ingenieria/00_gobierno_documental.md", canons)
	if !ok {
		t.Fatal("safeGovernanceSourceDoc ok = false, want true for declared canon root")
	}
	if got != "../wiki-repo/Ingenieria/00_gobierno_documental.md" {
		t.Fatalf("safe path = %q, want slash-relative canon path", got)
	}
}

func TestSafeGovernanceSourceDocRejectsAbsolute(t *testing.T) {
	root := t.TempDir()
	for _, sourceDoc := range []string{"/tmp/00_gobierno_documental.md", "~/wiki/00_gobierno_documental.md", `C:\wiki\00_gobierno_documental.md`} {
		if _, ok := safeGovernanceSourceDoc(root, sourceDoc, nil); ok {
			t.Fatalf("safeGovernanceSourceDoc(%q) ok = true, want false", sourceDoc)
		}
	}
}

func TestSafeProjectionOutputDefaultsEmptyAndAllowsInWorkspace(t *testing.T) {
	root := t.TempDir()
	got, ok := safeProjectionOutput(root, "", nil)
	if !ok || got != defaultProjectionRelPath {
		t.Fatalf("empty output = %q ok=%v, want %s", got, ok, defaultProjectionRelPath)
	}
	got, ok = safeProjectionOutput(root, "Ingenieria/_mi-lsp/read-model.toml", nil)
	if !ok || got != "Ingenieria/_mi-lsp/read-model.toml" {
		t.Fatalf("in-workspace output = %q ok=%v", got, ok)
	}
}

func TestSafeProjectionOutputRejectsCanonRootAndEscapes(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "Ingenieria"), 0o755); err != nil {
		t.Fatalf("mkdir Ingenieria: %v", err)
	}
	canons, err := workspace.ResolveCanons(root, model.ProjectFile{
		Canons: []model.WorkspaceCanon{{ID: "wiki", Root: "Ingenieria", Role: "producto"}},
	})
	if err != nil {
		t.Fatalf("ResolveCanons: %v", err)
	}
	if _, ok := safeProjectionOutput(root, "Ingenieria/_mi-lsp/read-model.toml", canons); ok {
		t.Fatal("projection inside canon root should be rejected")
	}
	if _, ok := safeProjectionOutput(root, "../outside/_mi-lsp/read-model.toml", canons); ok {
		t.Fatal("escaping projection.output should be rejected")
	}
	if _, ok := safeProjectionOutput(root, "/tmp/read-model.toml", nil); ok {
		t.Fatal("absolute projection.output should be rejected")
	}
}

func TestDiscoverProfilePathOrder(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "code")
	canon := filepath.Join(parent, "wiki-repo", "Ingenieria")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(canon, "_mi-lsp"), 0o755); err != nil {
		t.Fatalf("mkdir canon profile: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "Ingenieria", "_mi-lsp"), 0o755); err != nil {
		t.Fatalf("mkdir ingenieria profile: %v", err)
	}
	if err := workspace.SaveProjectFile(root, model.ProjectFile{
		Canons: []model.WorkspaceCanon{{ID: "wiki", Root: "../wiki-repo/Ingenieria", Role: "producto"}},
	}); err != nil {
		t.Fatalf("SaveProjectFile: %v", err)
	}
	canonProfile := filepath.Join(canon, "_mi-lsp", "read-model.toml")
	ingenieriaProfile := filepath.Join(root, "Ingenieria", "_mi-lsp", "read-model.toml")
	if err := os.WriteFile(canonProfile, []byte("version = 1\n"), 0o644); err != nil {
		t.Fatalf("write canon profile: %v", err)
	}
	if err := os.WriteFile(ingenieriaProfile, []byte("version = 1\n"), 0o644); err != nil {
		t.Fatalf("write ingenieria profile: %v", err)
	}

	if got := DiscoverProfilePath(root); got != canonProfile {
		t.Fatalf("without default, DiscoverProfilePath = %q, want canon %q", got, canonProfile)
	}
	defaultPath := ProfilePath(root)
	if err := os.MkdirAll(filepath.Dir(defaultPath), 0o755); err != nil {
		t.Fatalf("mkdir default profile: %v", err)
	}
	if err := os.WriteFile(defaultPath, []byte("version = 1\n"), 0o644); err != nil {
		t.Fatalf("write default profile: %v", err)
	}
	if got := DiscoverProfilePath(root); got != defaultPath {
		t.Fatalf("with default, DiscoverProfilePath = %q, want %q", got, defaultPath)
	}
}

func TestDiscoverProfilePathUsesInWorkspaceIngenieria(t *testing.T) {
	root := t.TempDir()
	ingenieriaProfile := filepath.Join(root, "Ingenieria", "_mi-lsp", "read-model.toml")
	if err := os.MkdirAll(filepath.Dir(ingenieriaProfile), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(ingenieriaProfile, []byte("version = 1\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if got := DiscoverProfilePath(root); got != ingenieriaProfile {
		t.Fatalf("DiscoverProfilePath = %q, want %q", got, ingenieriaProfile)
	}
	profile, source, warnings := LoadProfile(root)
	if source != "project" || len(warnings) != 0 || profile.Version != 1 {
		t.Fatalf("LoadProfile source=%q warnings=%v version=%d", source, warnings, profile.Version)
	}
}

func TestInspectGovernanceHonorsInWorkspaceNonWikiSource(t *testing.T) {
	root := t.TempDir()
	human := "Ingenieria/00_gobierno_documental.md"
	writeGovernanceMarkdown(t, root, human, minimalGovernanceYAML(human, ""))

	status := InspectGovernance(root, true)
	if status.Blocked {
		t.Fatalf("Blocked = true, want false: %#v", status)
	}
	if status.HumanDoc != human {
		t.Fatalf("HumanDoc = %q, want %q", status.HumanDoc, human)
	}
	if status.ProjectionDoc != defaultProjectionRelPath {
		t.Fatalf("ProjectionDoc = %q, want default in-workspace path", status.ProjectionDoc)
	}
	if _, err := os.Stat(filepath.Join(root, ".docs", "wiki", "00_gobierno_documental.md")); !os.IsNotExist(err) {
		t.Fatalf("must not require a duplicate wiki human doc, stat err=%v", err)
	}
	if _, err := os.Stat(ProfilePath(root)); err != nil {
		t.Fatalf("expected in-workspace projection write: %v", err)
	}
	profile, source, warnings := LoadProfile(root)
	if source != "project" || len(warnings) != 0 {
		t.Fatalf("LoadProfile source=%q warnings=%v", source, warnings)
	}
	if profile.Governance.SourceDoc != human {
		t.Fatalf("projected SourceDoc = %q, want %q", profile.Governance.SourceDoc, human)
	}
}

func TestInspectGovernanceHonorsDeclaredCanonSource(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "code")
	canon := filepath.Join(parent, "wiki-repo", "Ingenieria")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	if err := os.MkdirAll(canon, 0o755); err != nil {
		t.Fatalf("mkdir canon: %v", err)
	}
	if err := workspace.SaveProjectFile(root, model.ProjectFile{
		Canons: []model.WorkspaceCanon{{ID: "wiki", Root: "../wiki-repo/Ingenieria", Role: "producto"}},
	}); err != nil {
		t.Fatalf("SaveProjectFile: %v", err)
	}
	human := "../wiki-repo/Ingenieria/00_gobierno_documental.md"
	if err := os.WriteFile(filepath.Join(canon, "00_gobierno_documental.md"), []byte(minimalGovernanceYAML(human, "")), 0o644); err != nil {
		t.Fatalf("write canon governance: %v", err)
	}

	status := InspectGovernance(root, true)
	if status.Blocked {
		t.Fatalf("Blocked = true, want false: %#v", status)
	}
	if status.HumanDoc != human {
		t.Fatalf("HumanDoc = %q, want %q", status.HumanDoc, human)
	}
	if filepath.IsAbs(status.HumanDoc) {
		t.Fatalf("HumanDoc leaked an absolute path: %q", status.HumanDoc)
	}
	if status.ProjectionDoc != defaultProjectionRelPath {
		t.Fatalf("ProjectionDoc = %q, want in-workspace default", status.ProjectionDoc)
	}
	if _, err := os.Stat(ProfilePath(root)); err != nil {
		t.Fatalf("expected in-workspace projection: %v", err)
	}
	if _, err := os.Stat(filepath.Join(canon, "_mi-lsp", "read-model.toml")); !os.IsNotExist(err) {
		t.Fatalf("must not write into the foreign canon tree, stat err=%v", err)
	}
	profile, source, _ := LoadProfile(root)
	if source != "project" || profile.Governance.SourceDoc != human {
		t.Fatalf("projected SourceDoc = %q source=%q", profile.Governance.SourceDoc, source)
	}
}

func TestInspectGovernanceBlocksProjectionOutputInsideCanon(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "Ingenieria"), 0o755); err != nil {
		t.Fatalf("mkdir Ingenieria: %v", err)
	}
	if err := workspace.SaveProjectFile(root, model.ProjectFile{
		Canons: []model.WorkspaceCanon{{ID: "wiki", Root: "Ingenieria", Role: "producto"}},
	}); err != nil {
		t.Fatalf("SaveProjectFile: %v", err)
	}
	human := ".docs/wiki/00_gobierno_documental.md"
	writeGovernanceMarkdown(t, root, human, minimalGovernanceYAML(human, "Ingenieria/_mi-lsp/read-model.toml"))

	status := InspectGovernance(root, true)
	if !status.Blocked {
		t.Fatalf("Blocked = false, want true: %#v", status)
	}
	joined := strings.Join(status.Issues, " ")
	if !strings.Contains(joined, "canon_root_read_only") {
		t.Fatalf("Issues = %v, want canon_root_read_only", status.Issues)
	}
	if _, err := os.Stat(filepath.Join(root, "Ingenieria", "_mi-lsp", "read-model.toml")); !os.IsNotExist(err) {
		t.Fatalf("must not write into the canon root, stat err=%v", err)
	}
}

func TestInspectGovernanceWritesDeclaredInWorkspaceProjectionOutput(t *testing.T) {
	root := t.TempDir()
	human := ".docs/wiki/00_gobierno_documental.md"
	output := "Ingenieria/_mi-lsp/read-model.toml"
	writeGovernanceMarkdown(t, root, human, minimalGovernanceYAML(human, output))

	status := InspectGovernance(root, true)
	if status.Blocked {
		t.Fatalf("Blocked = true, want false: %#v", status)
	}
	if status.ProjectionDoc != output {
		t.Fatalf("ProjectionDoc = %q, want %q", status.ProjectionDoc, output)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(output))); err != nil {
		t.Fatalf("expected declared in-workspace projection: %v", err)
	}
}

func TestInspectGovernanceFailsClosedOnAmbiguousWellKnownDocs(t *testing.T) {
	root := t.TempDir()
	writeGovernanceMarkdown(t, root, "Ingenieria/00_gobierno_documental.md", "# a\n")
	writeGovernanceMarkdown(t, root, "wiki/00_gobierno_documental.md", "# b\n")

	status := InspectGovernance(root, false)
	if !status.Blocked {
		t.Fatalf("Blocked = false, want true: %#v", status)
	}
	if !strings.HasPrefix(status.HumanDoc, "INVALID:") {
		t.Fatalf("HumanDoc = %q, want INVALID marker", status.HumanDoc)
	}
	joined := strings.Join(status.Issues, " ")
	if !strings.Contains(joined, "Ingenieria/00_gobierno_documental.md") || !strings.Contains(joined, "wiki/00_gobierno_documental.md") {
		t.Fatalf("Issues = %v, want listed candidates", status.Issues)
	}
}

func writeReadModel(t *testing.T, root string, sourceDoc string) {
	t.Helper()
	path := ProfilePath(root)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir read model: %v", err)
	}
	body := "version = 1\n\n[governance]\nsource_doc = " + strconvQuote(sourceDoc) + "\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write read model: %v", err)
	}
}

func writeGovernanceMarkdown(t *testing.T, root, rel, body string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir governance doc: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write governance doc: %v", err)
	}
}

func minimalGovernanceYAML(sourcePath, projectionOutput string) string {
	if strings.TrimSpace(projectionOutput) == "" {
		projectionOutput = defaultProjectionRelPath
	}
	return strings.Join([]string{
		"# Gobierno documental",
		"",
		"```yaml",
		"version: 1",
		"profile: ordered_wiki",
		"hierarchy:",
		"  - id: governance",
		"    layer: \"00\"",
		"    family: functional",
		"    pack_stage: governance",
		"    paths:",
		"      - " + sourcePath,
		"context_chain:",
		"  - governance",
		"closure_chain:",
		"  - governance",
		"audit_chain:",
		"  - governance",
		"blocking_rules:",
		"  - missing_human_governance_doc",
		"projection:",
		"  output: " + projectionOutput,
		"  format: toml",
		"  auto_sync: true",
		"  versioned: true",
		"```",
		"",
	}, "\n")
}

func strconvQuote(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `\"`) + `"`
}
