package service

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/output"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

func TestEvidenceInventoryPrefersManifestVerdictAndCountsHeavyArtifacts(t *testing.T) {
	alias := "evidence-inventory-" + strings.ReplaceAll(t.Name(), "/", "-")
	root := createFunctionalPackWorkspaceFixture(t, alias)
	writeWorkspaceFile(t, root, ".docs/wiki/04_RF/RF-QA-CONVERSACIONAL.md", "# RF-QA-CONVERSACIONAL\n\nqa conversacional evidence lifecycle\n")
	writeWorkspaceFile(t, root, ".docs/auditoria/qa-conversacional/telegram/CQA-EXAMPLE/run-2026-05-01T00-00Z/manifest.yaml", "id: CQA-EXAMPLE\n")
	writeWorkspaceFile(t, root, ".docs/auditoria/qa-conversacional/telegram/CQA-EXAMPLE/run-2026-05-01T00-00Z/verdict.md", "# Verdict\n\nPASS\n")
	writeWorkspaceFile(t, root, ".docs/auditoria/qa-conversacional/telegram/CQA-EXAMPLE/run-2026-05-01T00-00Z/issues.yaml", "issues: []\n")
	writeWorkspaceFile(t, root, ".docs/auditoria/qa-conversacional/telegram/CQA-EXAMPLE/run-2026-05-01T00-00Z/turns/turn-001.json", `{"speaker":"user","text":"patient Alice api key sk-secret-123"}`)
	writeWorkspaceFile(t, root, ".docs/auditoria/qa-conversacional/telegram/CQA-EXAMPLE/run-2026-05-01T00-00Z/logs/run.log", "alice@example.com token should not leak")
	writeWorkspaceFile(t, root, ".docs/auditoria/qa-conversacional/telegram/CQA-EXAMPLE/run-2026-05-01T00-00Z/screenshots/step-001.png", "fakepng")
	writeWorkspaceFile(t, root, ".docs/raw/prompts/2026-05-31-qa-conversacional.md", "raw prompt body with patient Alice and sk-secret-123")

	app := New(root, nil)
	if _, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "workspace.init",
		Payload:   map[string]any{"path": root, "alias": alias},
	}); err != nil {
		t.Fatalf("workspace.init: %v", err)
	}
	defer func() { _ = workspace.RemoveWorkspace(alias) }()

	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.evidence.inventory",
		Context:   model.QueryOptions{Workspace: alias, MaxItems: 10},
		Payload:   map[string]any{"query": "qa conversacional evidence lifecycle"},
	})
	if err != nil {
		t.Fatalf("nav.evidence.inventory: %v", err)
	}
	if env.Backend != "evidence.inventory" {
		t.Fatalf("backend = %q, want evidence.inventory", env.Backend)
	}

	results, ok := env.Items.([]evidenceInventoryResult)
	if !ok || len(results) != 1 {
		t.Fatalf("items = %T %#v, want one evidenceInventoryResult", env.Items, env.Items)
	}
	result := results[0]
	if result.RecommendedReadPath != "manifest_verdict" {
		t.Fatalf("recommended_read_path = %q, want manifest_verdict", result.RecommendedReadPath)
	}
	if result.ContextLoadingProfile != "CL1_EXACT" || result.EvidenceLoadingProfile != "EL1_MANIFEST_VERDICT" {
		t.Fatalf("profiles = %s/%s, want CL1_EXACT/EL1_MANIFEST_VERDICT", result.ContextLoadingProfile, result.EvidenceLoadingProfile)
	}
	if result.Canonical.AnchorDoc.Path == "" || !strings.Contains(result.Canonical.AnchorDoc.Path, ".docs/wiki/") {
		t.Fatalf("expected canonical wiki anchor first, got %#v", result.Canonical.AnchorDoc)
	}

	run := findEvidenceRoot(result.EvidenceRoots, ".docs/auditoria/qa-conversacional/telegram/CQA-EXAMPLE/run-2026-05-01T00-00Z")
	if run == nil {
		t.Fatalf("expected run evidence root, got %#v", result.EvidenceRoots)
	}
	if run.Authority != "evidence_not_canon" || run.ArtifactType != "cqa_bundle" {
		t.Fatalf("run authority/type = %s/%s", run.Authority, run.ArtifactType)
	}
	if got := strings.Join(run.SummaryFirst, ","); got != "manifest.yaml,verdict.md,issues.yaml" {
		t.Fatalf("summary_first = %q", got)
	}
	for _, kind := range []string{"turns", "logs", "screenshots"} {
		stats, ok := run.HeavyArtifacts[kind]
		if !ok {
			t.Fatalf("missing heavy artifact %s in %#v", kind, run.HeavyArtifacts)
		}
		if stats.Files != 1 || stats.Bytes <= 0 || !stats.OmittedRaw || stats.ContentEmbedded {
			t.Fatalf("%s stats = %#v, want metadata-only single raw artifact", kind, stats)
		}
	}

	rawPrompts := findEvidenceRoot(result.EvidenceRoots, ".docs/raw/prompts")
	if rawPrompts == nil {
		t.Fatalf("expected raw prompts root, got %#v", result.EvidenceRoots)
	}
	if rawPrompts.ArtifactType != "raw_prompts" || rawPrompts.Authority != "evidence_not_canon" {
		t.Fatalf("raw prompts type/authority = %s/%s", rawPrompts.ArtifactType, rawPrompts.Authority)
	}

	renderedJSON, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	renderedTOON, err := output.Render(env, "toon", false)
	if err != nil {
		t.Fatalf("render toon: %v", err)
	}
	combined := string(renderedJSON) + "\n" + string(renderedTOON)
	for _, forbidden := range []string{"sk-secret-123", "alice@example.com", "patient Alice", "raw prompt body"} {
		if strings.Contains(combined, forbidden) {
			t.Fatalf("inventory output leaked raw content %q in %s", forbidden, combined)
		}
	}
	if !strings.Contains(string(renderedTOON), "tokens_est") {
		t.Fatalf("TOON output should include tokens_est, got %s", string(renderedTOON))
	}
}

func findEvidenceRoot(roots []evidenceInventoryRoot, root string) *evidenceInventoryRoot {
	for i := range roots {
		if roots[i].Root == root {
			return &roots[i]
		}
	}
	return nil
}
