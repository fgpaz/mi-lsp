package service

import (
	"context"
	"strings"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
)

func TestClassifySupportedIntentRoutesAllT3Operations(t *testing.T) {
	tests := []struct {
		question  string
		operation string
		wantArgs  []string
	}{
		{"show callers of HandleRequest", "callers", []string{"selector"}},
		{"show callees of HandleRequest", "callees", []string{"selector"}},
		{"what is affected by this change", "affected-change", nil},
		{"find path between Start and Finish", "path-between", []string{"from", "to"}},
		{"explain edge edge-123", "explain-edge", []string{"edge"}},
		{"show neighborhood of HandleRequest", "neighborhood", []string{"selector"}},
		{"explain change", "explain-change", nil},
	}
	for _, test := range tests {
		t.Run(test.operation, func(t *testing.T) {
			route, ok := classifySupportedIntent(test.question, nil)
			if !ok || route.Operation != test.operation {
				t.Fatalf("route=%+v ok=%v", route, ok)
			}
			for _, key := range test.wantArgs {
				if route.Arguments[key] == "" {
					t.Fatalf("missing extracted argument %q in %+v", key, route.Arguments)
				}
			}
		})
	}
}

func TestIntentExpansionCommandsUseExecutableCLIOperations(t *testing.T) {
	tests := []struct {
		name string
		plan model.IntentPlan
		want []string
	}{
		{"neighborhood", model.IntentPlan{Operation: "neighborhood", Arguments: map[string]string{"selector": "Run"}}, []string{"mi-lsp nav neighbors \"Run\"", "--workspace demo"}},
		{"path-between", model.IntentPlan{Operation: "path-between", Arguments: map[string]string{"from": "Start", "to": "Finish"}}, []string{"mi-lsp nav path \"Start\" \"Finish\"", "--workspace demo"}},
		{"explain-edge", model.IntentPlan{Operation: "explain-edge", Arguments: map[string]string{"edge": "edge-123"}}, []string{"mi-lsp nav explain \"edge-123\"", "--workspace demo"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			command := intentExpansionCommandForPlan("demo", test.plan)
			for _, part := range test.want {
				if !strings.Contains(command, part) {
					t.Fatalf("command %q does not contain %q", command, part)
				}
			}
		})
	}
}

func TestExplainChangeExpansionPreservesNormalizedPathsAndRef(t *testing.T) {
	plan := model.IntentPlan{
		Operation: "explain-change",
		Arguments: map[string]string{
			"paths": "internal/service/a b.go,src/quoted.go",
			"ref":   `feature/quoted "ref"`,
		},
		GenerationID: "generation-must-not-leak",
	}
	command := intentExplainChangeExpansion("demo", plan)
	if strings.Count(command, " --path ") != 2 || !strings.Contains(command, `--path "internal/service/a b.go"`) || !strings.Contains(command, `--path "src/quoted.go"`) {
		t.Fatalf("command=%q, want two safely quoted path flags", command)
	}
	if !strings.Contains(command, `--ref "feature/quoted \"ref\""`) {
		t.Fatalf("command=%q, want safely quoted ref", command)
	}
	if strings.Contains(command, "--generation") || strings.Contains(command, "diff-context") || strings.Contains(command, "--from-git-diff") {
		t.Fatalf("explain-change expansion derived an unsupported/current-diff input: %q", command)
	}
}

func TestIncompletePathExpansionUsesExecutableDiscovery(t *testing.T) {
	command := intentPathDiscoveryExpansion("demo", "Start", "")
	if !strings.Contains(command, "mi-lsp nav search") || !strings.Contains(command, `"Start"`) || !strings.Contains(command, "--include-content") {
		t.Fatalf("discovery command=%q", command)
	}
	if strings.Contains(command, "nav path") || strings.Contains(command, "--generation") {
		t.Fatalf("incomplete path emitted non-executable path/generation command=%q", command)
	}
	planCommand := intentExpansionCommandForPlan("demo", model.IntentPlan{Operation: "callers", GenerationID: "generation-123", Arguments: map[string]string{"selector": "Run"}})
	if !strings.Contains(planCommand, "--generation generation-123") {
		t.Fatalf("graph expansion dropped generation: %q", planCommand)
	}
}

func TestIntentPlanDoesNotAutoSelectAmbiguousSymbol(t *testing.T) {
	root, alias := setupTestWorkspace(t)
	project := testProject(alias)
	db, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := store.ReplaceCatalog(context.Background(), db, project, []model.FileRecord{
		{FilePath: "src/one.go", RepoID: "main", RepoName: "main", Language: "go"},
		{FilePath: "src/two.go", RepoID: "main", RepoName: "main", Language: "go"},
	}, []model.SymbolRecord{
		{FilePath: "src/one.go", RepoID: "main", RepoName: "main", Name: "Run", Kind: "function", StartLine: 1, EndLine: 1, QualifiedName: "src/one.go::Run", Language: "go"},
		{FilePath: "src/two.go", RepoID: "main", RepoName: "main", Name: "Run", Kind: "function", StartLine: 2, EndLine: 2, QualifiedName: "src/two.go::Run", Language: "go"},
	}); err != nil {
		t.Fatal(err)
	}

	env, err := New(root, nil).Execute(context.Background(), model.CommandRequest{
		Operation: "nav.intent",
		Context:   model.QueryOptions{Workspace: alias},
		Payload:   map[string]any{"question": "callers of Run"},
	})
	if err != nil {
		t.Fatal(err)
	}
	plans, ok := env.Items.([]model.IntentPlan)
	if !ok || len(plans) != 1 {
		t.Fatalf("items=%T %#v", env.Items, env.Items)
	}
	if len(plans[0].Candidates) != 2 || len(plans[0].Preview) != 0 {
		t.Fatalf("ambiguous plan auto-selected or previewed: %+v", plans[0])
	}
	if plans[0].Omissions[0].Code != "INTENT_SELECTOR_AMBIGUOUS" {
		t.Fatalf("omission=%+v", plans[0].Omissions)
	}
}

func TestExplainChangePreviewHasSevenSectionsWikiEvidenceAndExpansions(t *testing.T) {
	root, alias := setupTestWorkspace(t)
	writeWorkspaceFile(t, root, "internal/service/changed.go", "package service\n\nfunc Changed() {}\n")

	env, err := New(root, nil).Execute(context.Background(), model.CommandRequest{
		Operation: "nav.intent",
		Context:   model.QueryOptions{Workspace: alias, MaxItems: 5},
		Payload: map[string]any{
			"question": "explain change",
			"intent":   "explain-change",
			"paths":    []any{"internal/service/changed.go"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	plans, ok := env.Items.([]model.IntentPlan)
	if !ok || len(plans) != 1 {
		t.Fatalf("items=%T %#v", env.Items, env.Items)
	}
	plan := plans[0]
	if plan.Operation != "explain-change" || len(plan.Preview) != 7 {
		t.Fatalf("plan=%+v", plan)
	}
	wantSections := []string{"change", "affected", "callers", "callees", "tests", "contracts", "wiki"}
	for i, want := range wantSections {
		if plan.Preview[i].Section != want {
			t.Fatalf("section[%d]=%q want %q", i, plan.Preview[i].Section, want)
		}
	}
	if len(plan.Wiki.MustRead) == 0 || plan.Wiki.MustRead[0].Path != ".docs/wiki/00_gobierno_documental.md" {
		t.Fatalf("wiki must_read=%+v", plan.Wiki.MustRead)
	}
	if len(plan.Wiki.MustRead[0].EvidencePaths) != 1 || plan.Wiki.MustRead[0].EvidencePaths[0] != "internal/service/changed.go" {
		t.Fatalf("wiki evidence=%+v", plan.Wiki.MustRead[0])
	}
	if len(plan.Expansions) < 3 || plan.Expansions[0].Command == "" || plan.Expansions[0].Reason == "" {
		t.Fatalf("expansions=%+v", plan.Expansions)
	}
	if plan.Telemetry.PlannerVersion == "" || plan.Telemetry.Operation != "explain-change" {
		t.Fatalf("telemetry=%+v", plan.Telemetry)
	}
	if plan.DeterminismDigest == "" {
		t.Fatal("missing determinism digest")
	}
}

func TestIntentPlanDigestIsStable(t *testing.T) {
	plan := model.IntentPlan{Intent: "callers", Operation: "callers", Confidence: 0.9, Freshness: "catalog-current", Arguments: map[string]string{"selector": "Run"}}
	first := model.IntentPlanDigest(plan)
	second := model.IntentPlanDigest(plan)
	if first == "" || first != second {
		t.Fatalf("digest first=%q second=%q", first, second)
	}
}
