from pathlib import Path

# --- governance_test.go line-based patches ---
gov_path = Path(r"C:\wt\milsp-ae-v2-only\internal\service\governance_test.go")
lines = gov_path.read_text(encoding="utf-8").splitlines(keepends=True)

def find_line(prefix: str, start: int = 0) -> int:
    for i in range(start, len(lines)):
        if lines[i].lstrip("\t ").startswith(prefix):
            return i
    raise SystemExit(f"line not found: {prefix}")

# Rename declared roots test
i = find_line("func TestNavGovernanceReportsDeclaredAECanonRoots")
lines[i] = lines[i].replace(
    "TestNavGovernanceReportsDeclaredAECanonRoots",
    "TestNavGovernanceRejectsDeclaredLegacyAECanonRoots",
)

# Replace expectations inside that test (blocked/valid block)
i_blocked = find_line('if status.Blocked {', i)
# expect four if-blocks until roots check ends before `})` of t.Run
chunk = "".join(lines[i_blocked:i_blocked + 12])
if 'want valid' not in chunk and 'expected governance to pass' not in chunk:
    raise SystemExit(f"unexpected declared-roots expectation block:\n{chunk}")
indent = "\t\t\t"
new_block = [
    f"{indent}if !status.Blocked {{\n",
    f'{indent}\tt.Fatalf("expected legacy AE declaration to block governance, got %#v", status)\n',
    f"{indent}}}\n",
    f'{indent}if status.AECanon.Status != "mismatch" || status.AECanon.Reason != "ae_canon_legacy_mode_rejected" {{\n',
    f'{indent}\tt.Fatalf("expected legacy AE rejection, got %#v", status.AECanon)\n',
    f"{indent}}}\n",
    f"{indent}if len(status.AECanon.Roots) != 1 || status.AECanon.Roots[0] != aeRoot {{\n",
    f'{indent}\tt.Fatalf("ae_canon.roots = %#v, want [%s]", status.AECanon.Roots, aeRoot)\n',
    f"{indent}}}\n",
]
# remove old blocks until the closing of roots if
end = i_blocked
seen_roots = 0
while end < len(lines):
    if "ae_canon.roots" in lines[end]:
        seen_roots += 1
    if seen_roots and lines[end].strip() == "}":
        end += 1
        break
    end += 1
lines[i_blocked:end] = new_block

# knowledge-wiki expectation
i = find_line('t.Fatalf("expected valid fallback AE canon')
# replace the surrounding if block
start = i - 1
if "status.AECanon.Status != \"valid\"" not in lines[start]:
    raise SystemExit(f"bad knowledge wiki context: {lines[start]!r}{lines[i]!r}")
lines[start:i+2] = [
    "\t\tif status.AECanon.Status == \"valid\" {\n",
    "\t\t\tt.Fatalf(\"knowledge-wiki without kernel_v2 must not promote repo-local AE as authority, got %#v\", status.AECanon)\n",
    "\t\t}\n",
]

# redirect test rename + body expectations
i = find_line("func TestNavGovernanceFollowsExplicitAECanonReadmeRedirect")
lines[i] = lines[i].replace(
    "TestNavGovernanceFollowsExplicitAECanonReadmeRedirect",
    "TestNavGovernanceRejectsExplicitAECanonReadmeRedirect",
)
i_insp = find_line("if status := docgraph.InspectGovernance(root, true); status.Blocked", i)
lines[i_insp:i_insp+3] = [
    "\tif status := docgraph.InspectGovernance(root, true); !status.Blocked || status.AECanon.Reason != \"ae_canon_legacy_mode_rejected\" {\n",
    "\t\tt.Fatalf(\"expected redirected legacy AE canon to be rejected, got %#v\", status)\n",
    "\t}\n",
]
i_status = find_line("status := env.Items.([]model.GovernanceStatus)[0]", i_insp)
# replace final assertions until end of function
j = i_status + 1
while j < len(lines) and not lines[j].startswith("func "):
    j += 1
# back up over blank lines before next func, keep one closing brace of function
# find function end: line with only }
k = j - 1
while k > i_status and lines[k].strip() == "":
    k -= 1
# k should be closing } of function
if lines[k].strip() != "}":
    raise SystemExit(f"function end not found near {k}: {lines[k]!r}")
lines[i_status:k] = [
    "\tstatus := env.Items.([]model.GovernanceStatus)[0]\n",
    "\tif !status.Blocked || status.AECanon.Reason != \"ae_canon_legacy_mode_rejected\" {\n",
    "\t\tt.Fatalf(\"expected legacy redirect rejection, got %#v\", status)\n",
    "\t}\n",
]

gov_path.write_text("".join(lines), encoding="utf-8")
print("governance_test patched")

# --- route_test.go ---
route_path = Path(r"C:\wt\milsp-ae-v2-only\internal\service\route_test.go")
r = route_path.read_text(encoding="utf-8")
start = r.find("func TestNavRouteExplicitAEIDPrefersDeclaredCanonRoot")
if start < 0:
    raise SystemExit("route start missing")
end = r.find("\nfunc TestNavRoutePreviewModeByDefault", start)
if end < 0:
    raise SystemExit("route end missing")
new_route = r'''func TestNavRouteExplicitAEIDUsesCompatibilityHistoryUnderKernelV2(t *testing.T) {
	alias := "route-ae-canon-" + t.Name()
	ensureWritableTestHome(t)
	root := t.TempDir()
	writeWorkspaceFile(t, root, "src/App.csproj", `<Project Sdk="Microsoft.NET.Sdk"></Project>`)
	writeWorkspaceFile(t, root, ".docs/wiki/07_baseline_tecnica.md", "# 07. Baseline tecnica\n")
	writeSpecBackendGovernanceFixtureWithKernelV2(t, root)
	// Local historical projection remains discoverable for navigation, but is not AE authority.
	writeWorkspaceFile(t, root, ".docs/wiki/ae/AE-HARNESS-MANIFEST.md", "# AE-HARNESS-MANIFEST\n\nid: AE-HARNESS-MANIFEST\n\nCompatibility history only.\n")
	writeWorkspaceFile(t, root, ".docs/ae/AE-HARNESS-MANIFEST.md", "# AE-HARNESS-MANIFEST\n\nid: AE-HARNESS-MANIFEST\n\nMust not win over wiki compatibility path when kernel_v2 is active.\n")
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
route_path.write_text(r[:start] + new_route + r[end:], encoding="utf-8")
print("route_test patched")

# verify helpers has kernel helper
helpers = Path(r"C:\wt\milsp-ae-v2-only\internal\service\governance_test_helpers_test.go").read_text(encoding="utf-8")
if "writeSpecBackendGovernanceFixtureWithKernelV2" not in helpers:
    raise SystemExit("kernel helper missing")
if "ae_canon_legacy_mode_rejected" not in helpers:
    raise SystemExit("legacy helper expectation missing")
print("helpers verified")
