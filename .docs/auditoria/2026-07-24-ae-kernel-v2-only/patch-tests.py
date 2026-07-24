from pathlib import Path

helpers = Path(r"C:\wt\milsp-ae-v2-only\internal\service\governance_test_helpers_test.go")
h = helpers.read_text(encoding="utf-8")
old = '''func writeSpecBackendGovernanceFixtureWithAE(t *testing.T, root string, aeRoot string) {
	t.Helper()
	writeSpecBackendGovernanceFixture(t, root)
	addAEDeclarationToGovernanceFixture(t, root, aeRoot)

	status := docgraph.InspectGovernance(root, true)
	if status.Blocked {
		t.Fatalf("expected governance fixture with AE to be valid, got blocked status: %#v", status)
	}
}
'''
new = '''func writeSpecBackendGovernanceFixtureWithAE(t *testing.T, root string, aeRoot string) {
	t.Helper()
	writeSpecBackendGovernanceFixture(t, root)
	addAEDeclarationToGovernanceFixture(t, root, aeRoot)

	// Hard cut: hierarchy-declared repo-local AE without kernel_v2 is rejected.
	status := docgraph.InspectGovernance(root, true)
	if !status.Blocked || status.AECanon.Reason != "ae_canon_legacy_mode_rejected" {
		t.Fatalf("expected legacy AE declaration to be rejected, got %#v", status)
	}
}

func writeSpecBackendGovernanceFixtureWithKernelV2(t *testing.T, root string) {
	t.Helper()
	kernelHome := t.TempDir()
	t.Setenv("AE_KERNEL_HOME", kernelHome)
	writeSpecBackendGovernanceFixture(t, root)
	addKernelV2AECanonToGovernanceFixture(t, root)
	writeKernelV2CanonModules(t, kernelHome)
	writeKernelV2RepoPolicy(t, root)

	status := docgraph.InspectGovernance(root, true)
	if status.Blocked || status.AECanon.Status != "valid" || status.AECanon.Source != "kernel_v2" {
		t.Fatalf("expected kernel_v2 governance fixture to be valid, got %#v", status)
	}
}
'''
if old not in h:
    raise SystemExit("helpers old block missing")
helpers.write_text(h.replace(old, new), encoding="utf-8")
print("helpers ok")

gov = Path(r"C:\wt\milsp-ae-v2-only\internal\service\governance_test.go")
g = gov.read_text(encoding="utf-8")

# rename + rewrite declared roots test expectations by targeted replacements
replacements = [
    (
        "func TestNavGovernanceReportsDeclaredAECanonRoots(t *testing.T) {",
        "func TestNavGovernanceRejectsDeclaredLegacyAECanonRoots(t *testing.T) {",
    ),
    (
        '\t\t\tif status.Blocked {\n\t\t\t\tt.Fatalf("expected governance to pass, got %#v", status)\n\t\t\t}\n\t\t\tif status.AECanon.Status != "valid" {\n\t\t\t\tt.Fatalf("ae_canon.status = %q, want valid: %#v", status.AECanon.Status, status.AECanon)\n\t\t\t}\n\t\t\tif status.AECanon.Source != "governance" {\n\t\t\t\tt.Fatalf("ae_canon.source = %q, want governance", status.AECanon.Source)\n\t\t\t}\n\t\t\tif len(status.AECanon.Roots) != 1 || status.AECanon.Roots[0] != aeRoot {\n\t\t\t\tt.Fatalf("ae_canon.roots = %#v, want [%s]", status.AECanon.Roots, aeRoot)\n\t\t\t}',
        '\t\t\tif !status.Blocked {\n\t\t\t\tt.Fatalf("expected legacy AE declaration to block governance, got %#v", status)\n\t\t\t}\n\t\t\tif status.AECanon.Status != "mismatch" || status.AECanon.Reason != "ae_canon_legacy_mode_rejected" {\n\t\t\t\tt.Fatalf("expected legacy AE rejection, got %#v", status.AECanon)\n\t\t\t}\n\t\t\tif len(status.AECanon.Roots) != 1 || status.AECanon.Roots[0] != aeRoot {\n\t\t\t\tt.Fatalf("ae_canon.roots = %#v, want [%s]", status.AECanon.Roots, aeRoot)\n\t\t\t}',
    ),
    (
        '\t\tif status.AECanon.Status != "valid" {\n\t\t\tt.Fatalf("expected valid fallback AE canon, got %#v", status.AECanon)\n\t\t}',
        '\t\tif status.AECanon.Status == "valid" {\n\t\t\tt.Fatalf("knowledge-wiki without kernel_v2 must not promote repo-local AE as authority, got %#v", status.AECanon)\n\t\t}',
    ),
    (
        "func TestNavGovernanceFollowsExplicitAECanonReadmeRedirect(t *testing.T) {",
        "func TestNavGovernanceRejectsExplicitAECanonReadmeRedirect(t *testing.T) {",
    ),
    (
        '\tif status := docgraph.InspectGovernance(root, true); status.Blocked {\n\t\tt.Fatalf("expected redirected AE canon to pass, got %#v", status)\n\t}',
        '\tif status := docgraph.InspectGovernance(root, true); !status.Blocked || status.AECanon.Reason != "ae_canon_legacy_mode_rejected" {\n\t\tt.Fatalf("expected redirected legacy AE canon to be rejected, got %#v", status)\n\t}',
    ),
    (
        '\tstatus := env.Items.([]model.GovernanceStatus)[0]\n\tif status.AECanon.Status != "valid" || status.AECanon.Source != "redirect" {\n\t\tt.Fatalf("expected valid redirected ae_canon, got %#v", status.AECanon)\n\t}\n\tif len(status.AECanon.Roots) != 1 || status.AECanon.Roots[0] != ".docs/ae" {\n\t\tt.Fatalf("ae_canon.roots = %#v, want [.docs/ae]", status.AECanon.Roots)\n\t}\n}',
        '\tstatus := env.Items.([]model.GovernanceStatus)[0]\n\tif !status.Blocked || status.AECanon.Reason != "ae_canon_legacy_mode_rejected" {\n\t\tt.Fatalf("expected legacy redirect rejection, got %#v", status)\n\t}\n}',
    ),
]

for old, new in replacements:
    if old not in g:
        raise SystemExit(f"gov replacement missing:\n{old[:120]}")
    g = g.replace(old, new)
gov.write_text(g, encoding="utf-8")
print("governance_test ok")

route = Path(r"C:\wt\milsp-ae-v2-only\internal\service\route_test.go")
r = route.read_text(encoding="utf-8")
old_route = '''func TestNavRouteExplicitAEIDPrefersDeclaredCanonRoot(t *testing.T) {
	alias := "route-ae-canon-" + t.Name()
	ensureWritableTestHome(t)
	root := t.TempDir()
	writeWorkspaceFile(t, root, "src/App.csproj", `<Project Sdk="Microsoft.NET.Sdk"></Project>`)
	writeWorkspaceFile(t, root, ".docs/wiki/07_baseline_tecnica.md", "# 07. Baseline tecnica\\n")
	writeAECanonModules(t, root, ".docs/ae")
	writeSpecBackendGovernanceFixtureWithAE(t, root, ".docs/ae")
	writeWorkspaceFile(t, root, ".docs/wiki/ae/AE-HARNESS-MANIFEST.md", "# AE-HARNESS-MANIFEST\\n\\nLegacy projection only.\\n")
	if _, err := workspace.RegisterWorkspace(alias, model.WorkspaceRegistration{
		Name:      alias,
		Root:      root,
		Languages: []string{"csharp"},
		Kind:      model.WorkspaceKindSingle,
	}); err != nil {
		t.Fatalf("register workspace: %v", err)
	}
	defer func() { _ = workspace.RemoveWorkspace(alias) }()

	app := New(root, nil)
	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.route",
		Context:   model.QueryOptions{Workspace: alias},
		Payload:   map[string]any{"task": "AE-HARNESS-MANIFEST"},
	})
	if err != nil {
		t.Fatalf("nav.route: %v", err)
	}
	results := env.Items.([]model.RouteResult)
	anchor := results[0].Canonical.AnchorDoc
	if anchor.Path != ".docs/ae/AE-HARNESS-MANIFEST.md" {
		t.Fatalf("AE anchor path = %q, want declared .docs/ae root", anchor.Path)
	}
}
'''
# The file uses real newlines in string literals, not escaped. Rebuild carefully from markers.
start = r.find("func TestNavRouteExplicitAEIDPrefersDeclaredCanonRoot")
if start < 0:
    raise SystemExit("route test start missing")
end = r.find("\nfunc TestNavRoutePreviewModeByDefault", start)
if end < 0:
    raise SystemExit("route test end missing")
new_route = '''func TestNavRouteExplicitAEIDUsesCompatibilityHistoryUnderKernelV2(t *testing.T) {
	alias := "route-ae-canon-" + t.Name()
	ensureWritableTestHome(t)
	root := t.TempDir()
	writeWorkspaceFile(t, root, "src/App.csproj", `<Project Sdk="Microsoft.NET.Sdk"></Project>`)
	writeWorkspaceFile(t, root, ".docs/wiki/07_baseline_tecnica.md", "# 07. Baseline tecnica\\n")
	writeSpecBackendGovernanceFixtureWithKernelV2(t, root)
	// Local historical projection remains discoverable for navigation, but is not AE authority.
	writeWorkspaceFile(t, root, ".docs/wiki/ae/AE-HARNESS-MANIFEST.md", "# AE-HARNESS-MANIFEST\\n\\nid: AE-HARNESS-MANIFEST\\n\\nCompatibility history only.\\n")
	writeWorkspaceFile(t, root, ".docs/ae/AE-HARNESS-MANIFEST.md", "# AE-HARNESS-MANIFEST\\n\\nid: AE-HARNESS-MANIFEST\\n\\nMust not win over wiki compatibility path when kernel_v2 is active.\\n")
	if _, err := workspace.RegisterWorkspace(alias, model.WorkspaceRegistration{
		Name:      alias,
		Root:      root,
		Languages: []string{"csharp"},
		Kind:      model.WorkspaceKindSingle,
	}); err != nil {
		t.Fatalf("register workspace: %v", err)
	}
	defer func() { _ = workspace.RemoveWorkspace(alias) }()

	app := New(root, nil)
	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.route",
		Context:   model.QueryOptions{Workspace: alias},
		Payload:   map[string]any{"task": "AE-HARNESS-MANIFEST"},
	})
	if err != nil {
		t.Fatalf("nav.route: %v", err)
	}
	results := env.Items.([]model.RouteResult)
	anchor := results[0].Canonical.AnchorDoc
	if anchor.Path != ".docs/wiki/ae/AE-HARNESS-MANIFEST.md" {
		t.Fatalf("AE anchor path = %q, want compatibility history .docs/wiki/ae", anchor.Path)
	}
}
'''
# Fix the accidental double-escaped newlines in writeWorkspaceFile content for Go source.
new_route = new_route.replace("\\\\n", "\\n")
r = r[:start] + new_route + r[end:]
route.write_text(r, encoding="utf-8")
print("route_test ok")
