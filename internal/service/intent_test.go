package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
		{"neighborhood", model.IntentPlan{Operation: "neighborhood", Arguments: map[string]string{"selector": "Run"}}, []string{"mi-lsp nav neighbors Run", "--workspace demo"}},
		{"path-between", model.IntentPlan{Operation: "path-between", Arguments: map[string]string{"from": "Start", "to": "Finish"}}, []string{"mi-lsp nav path Start Finish", "--workspace demo"}},
		{"explain-edge", model.IntentPlan{Operation: "explain-edge", Arguments: map[string]string{"edge": "edge-123"}}, []string{"mi-lsp nav explain edge-123", "--workspace demo"}},
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
	expansion := intentExplainChangeExpansionForPlan("demo", plan)
	command := expansion.Command
	if !strings.Contains(command, "--path "+intentPlaceholder("paths")) || !strings.Contains(command, "--ref "+intentPlaceholder("ref")) {
		t.Fatalf("command=%q, want inert structured placeholders", command)
	}
	if strings.Contains(command, "internal/service/a b.go") || strings.Contains(command, "feature/quoted") || strings.Contains(command, "\\\"") {
		t.Fatalf("raw shell-sensitive input leaked into command: %q", command)
	}
	paths, ok := expansion.Arguments["paths"].([]string)
	if !ok || len(paths) != 2 || paths[0] != "internal/service/a b.go" || paths[1] != "src/quoted.go" {
		t.Fatalf("structured paths=%#v", expansion.Arguments["paths"])
	}
	if expansion.Arguments["ref"] != `feature/quoted "ref"` {
		t.Fatalf("structured ref=%#v", expansion.Arguments["ref"])
	}
	if strings.Contains(command, "--generation") || strings.Contains(command, "diff-context") || strings.Contains(command, "--from-git-diff") {
		t.Fatalf("explain-change expansion derived an unsupported/current-diff input: %q", command)
	}
}

func TestIncompletePathExpansionUsesExecutableDiscovery(t *testing.T) {
	command := intentPathDiscoveryExpansion("demo", "Start", "")
	if !strings.Contains(command, "mi-lsp nav search Start") || !strings.Contains(command, "--include-content") {
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
	if plan.Fallbacks == nil || len(plan.Fallbacks) != 0 {
		t.Fatalf("internal degradations must remain omissions, fallbacks=%+v omissions=%+v", plan.Fallbacks, plan.Omissions)
	}
	planJSON, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(planJSON), `"fallbacks":[]`) {
		t.Fatalf("planner explain-change JSON omitted empty fallbacks: %s", planJSON)
	}
	envelopeJSON, err := json.Marshal(env)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(envelopeJSON), `"backend":"planner"`) || !strings.Contains(string(envelopeJSON), `"fallbacks":[]`) {
		t.Fatalf("runtime planner envelope JSON shape=%s", envelopeJSON)
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

func TestIntentFallbackReasonCodeAllowlistAndJSONShape(t *testing.T) {
	valid := map[string]string{
		model.IntentFallbackUnsupportedOperation: "the requested operation is not supported",
		model.IntentFallbackUnavailableBinary:    "the required backend binary is unavailable",
		model.IntentFallbackInvalidWorkspace:     "the requested workspace is invalid",
		model.IntentFallbackExplicitIncomplete:   "the result is explicitly incomplete",
	}
	const hostileDetail = "token=secret-token path=C:\\Users\\Ana\\private.txt pii=ana@example.test"
	for code, canonicalDetail := range valid {
		fallback := model.IntentFallback{Section: "callers", Operation: "nav.callers", ReasonCode: code, Detail: hostileDetail}
		if !fallback.Valid() || !model.ValidIntentFallbackReasonCode(code) {
			t.Fatalf("fallback code %q was rejected", code)
		}
		encoded, err := json.Marshal(fallback)
		if err != nil {
			t.Fatal(err)
		}
		jsonText := string(encoded)
		if !strings.Contains(jsonText, `"reason_code":"`+code+`"`) || !strings.Contains(jsonText, `"detail":"`+canonicalDetail+`"`) {
			t.Fatalf("structured fallback JSON=%s", jsonText)
		}
		if strings.Contains(jsonText, hostileDetail) || strings.Contains(jsonText, `"reason"`) {
			t.Fatalf("arbitrary fallback detail leaked into JSON=%s", jsonText)
		}
	}
	for _, code := range []string{"", "backend_unavailable", "timeout", "raw prompt"} {
		if model.ValidIntentFallbackReasonCode(code) {
			t.Fatalf("invalid fallback code %q accepted", code)
		}
		if (model.IntentFallback{ReasonCode: code}).Valid() {
			t.Fatalf("invalid fallback struct %q accepted", code)
		}
		if _, err := json.Marshal(model.IntentFallback{ReasonCode: code, Detail: hostileDetail}); err == nil {
			t.Fatalf("invalid fallback code %q serialized", code)
		}
	}
	constructed, err := model.NewIntentFallback("callers", "nav.callers", model.IntentFallbackUnavailableBinary)
	if err != nil || constructed.Detail != valid[model.IntentFallbackUnavailableBinary] {
		t.Fatalf("constructor=%+v err=%v", constructed, err)
	}
}

func TestSanitizeIntentErrorUsesOnlyStableLabels(t *testing.T) {
	const hostile = "token=secret-token path=C:\\Users\\Ana\\private.txt pii=ana@example.test"
	cases := []struct {
		name string
		err  error
		want string
	}{
		{name: "operation", err: errors.New(hostile), want: "operation_error"},
		{name: "wrapped operation", err: fmt.Errorf("wrapper: %w", errors.New(hostile)), want: "operation_error"},
		{name: "known graph code", err: &model.GraphQueryError{Code: "GPH_QUERY_BACKEND_UNAVAILABLE", Message: hostile}, want: "GPH_QUERY_BACKEND_UNAVAILABLE"},
		{name: "unknown graph code", err: &model.GraphQueryError{Code: "GPH_QUERY_PRIVATE", Message: hostile}, want: "graph_query_error"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			got := sanitizeIntentError(test.err)
			if got != test.want {
				t.Fatalf("sanitizeIntentError=%q want %q", got, test.want)
			}
			if strings.Contains(got, "secret-token") || strings.Contains(got, "private.txt") || strings.Contains(got, "ana@example.test") {
				t.Fatalf("sanitized error leaked hostile input: %q", got)
			}
		})
	}
}

func TestNavIntentEnvelopeJSONKeepsLegacyAndGraphNativeShapes(t *testing.T) {
	legacy := model.Envelope{
		Ok:        true,
		Workspace: "demo",
		Backend:   "intent",
		Mode:      "docs",
		Items:     []map[string]any{{"doc_id": "CT-NAV-INTENT"}},
		Truncated: false,
	}
	planner := model.Envelope{
		Ok:        true,
		Workspace: "demo",
		Backend:   "planner",
		Mode:      "preview",
		Items: []model.IntentPlan{{
			Intent:     "callers",
			Operation:  "callers",
			Arguments:  map[string]string{"selector": "Run"},
			Confidence: 0.9,
			Freshness:  "graph-generation-bound",
			Preview:    []model.IntentPreview{{Section: "callers", Count: 0, Items: []any{}}},
			Omissions:  []model.IntentOmission{},
			Fallbacks:  []model.IntentFallback{},
			Expansions: []model.Expansion{{Command: "mi-lsp nav callers \\\"Run\\\" --workspace demo --format toon --full", Reason: "expand"}},
			Telemetry:  model.IntentTelemetry{PlannerVersion: "intent-v1", Operation: "callers"},
		}},
		Truncated: false,
	}
	for name, envelope := range map[string]model.Envelope{"legacy": legacy, "planner": planner} {
		encoded, err := json.Marshal(envelope)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		var decoded map[string]any
		if err := json.Unmarshal(encoded, &decoded); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if decoded["backend"] != envelope.Backend || decoded["mode"] != envelope.Mode {
			t.Fatalf("%s envelope=%s", name, encoded)
		}
		if name == "planner" {
			items, ok := decoded["items"].([]any)
			if !ok || len(items) != 1 {
				t.Fatalf("planner items shape=%T %#v envelope=%s", decoded["items"], decoded["items"], encoded)
			}
			planJSON, ok := items[0].(map[string]any)
			if !ok {
				t.Fatalf("planner item shape=%T %#v", items[0], items[0])
			}
			fallbacks, present := planJSON["fallbacks"]
			if !present {
				t.Fatalf("planner preview omitted fallbacks: %s", encoded)
			}
			if empty, ok := fallbacks.([]any); !ok || empty == nil || len(empty) != 0 {
				t.Fatalf("planner fallbacks shape=%T %#v envelope=%s", fallbacks, fallbacks, encoded)
			}
		}
	}
}

func TestIntentPlanJSONHasNoRawPromptOrPromptTelemetry(t *testing.T) {
	question := "explain change; do not persist this raw prompt"
	plan := model.IntentPlan{
		Intent:     "explain-change",
		Operation:  "explain-change",
		Arguments:  map[string]string{"paths": "internal/service/intent.go", "ref": "feature/test"},
		Confidence: 0.9,
		Freshness:  "working-tree-snapshot",
		Telemetry:  model.IntentTelemetry{PlannerVersion: "intent-v1", Operation: "explain-change"},
		Expansions: []model.Expansion{{Command: "mi-lsp nav explain-change --workspace demo", Reason: "expand"}},
	}
	encoded, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	jsonText := string(encoded)
	for _, forbidden := range []string{question, `"question"`, `"prompt"`, `"raw_prompt"`} {
		if strings.Contains(jsonText, forbidden) {
			t.Fatalf("raw prompt marker %q leaked into plan JSON=%s", forbidden, jsonText)
		}
	}
}

func TestLegacyIntentDocNextQueriesNeverEchoRawQuestion(t *testing.T) {
	const hostileQuestion = `how do I rotate token=secret-token for ana@example.test at C:\\Users\\Ana\\private.txt?`
	queries := buildIntentDocNextQueries("demo", hostileQuestion, `.docs/wiki/09_contratos/CT-NAV-INTENT.md`, "CT-NAV-INTENT")
	if len(queries) < 2 {
		t.Fatalf("queries=%#v, want canonical continuations", queries)
	}
	joined := strings.Join(queries, " ")
	for _, forbidden := range []string{hostileQuestion, "secret-token", "ana@example.test", `C:\\Users\\Ana\\private.txt`, " nav ask ", " nav pack "} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("legacy next_queries leaked %q: %#v", forbidden, queries)
		}
	}
	if !strings.Contains(joined, "nav search") || !strings.Contains(joined, "nav multi-read") {
		t.Fatalf("queries=%#v, want search and multi-read canonical continuations", queries)
	}
}

func TestIntentExpansionsPreserveRepoScopeAndAffectedSnapshot(t *testing.T) {
	route, ok := classifySupportedIntent("callers of Run", map[string]any{"repo": "backend"})
	if !ok || route.Arguments["repo"] != "backend" {
		t.Fatalf("route=%+v ok=%v, want canonical repo scope", route, ok)
	}
	callers := intentExpansionCommandForPlan("demo", model.IntentPlan{
		Operation: "callers",
		Arguments: route.Arguments,
	})
	if !strings.Contains(callers, "--repo backend") {
		t.Fatalf("callers expansion=%q, want --repo backend", callers)
	}

	plan := model.IntentPlan{
		Operation: "affected-change",
		Arguments: map[string]string{
			"paths":         `src/space name.go,internal/quoted"name.go`,
			"changed_ref":   `feature/quoted "ref"`,
			"from_git_diff": "false",
			"repo":          "backend",
		},
		GenerationID: "generation-123",
	}
	firstExpansion := intentAffectedExpansionForPlan("demo", plan)
	secondExpansion := intentAffectedExpansionForPlan("demo", plan)
	first, second := firstExpansion.Command, secondExpansion.Command
	if first != second {
		t.Fatalf("affected expansion is nondeterministic: %q != %q", first, second)
	}
	for _, want := range []string{"--repo backend", "--generation generation-123"} {
		if !strings.Contains(first, want) {
			t.Fatalf("affected expansion=%q, want %q", first, want)
		}
	}
	if !strings.Contains(first, intentPlaceholder("paths")) || strings.Contains(first, "src/space name.go") || strings.Contains(first, "internal/quoted") {
		t.Fatalf("affected expansion did not isolate structured paths: %q", first)
	}
	paths, ok := firstExpansion.Arguments["paths"].([]string)
	if !ok || len(paths) != 2 || paths[0] != "src/space name.go" || paths[1] != `internal/quoted"name.go` {
		t.Fatalf("structured affected paths=%#v", firstExpansion.Arguments["paths"])
	}
	if strings.Contains(first, "--from-git-diff") || strings.Contains(first, "--changed-ref") {
		t.Fatalf("explicit-path expansion replaced or added git snapshot input: %q", first)
	}
}

func TestWorkspaceDBDiagnosticsUseStableCodeWithoutSecretPayload(t *testing.T) {
	const hostile = `token=secret-token path=C:\\Users\\Ana\\private.db pii=ana@example.test`
	err := &workspaceDBOpenError{cause: errors.New(hostile)}
	if strings.Contains(err.Error(), "secret-token") || strings.Contains(err.Error(), "private.db") || strings.Contains(err.Error(), "ana@example.test") {
		t.Fatalf("workspace DB error leaked cause: %q", err)
	}
	if got := sanitizeIntentError(err); got != workspaceDBOpenErrorCode {
		t.Fatalf("sanitizeIntentError=%q want %q", got, workspaceDBOpenErrorCode)
	}
	warning := "catalog unavailable; symbol evidence omitted: " + sanitizeIntentError(err)
	if got := sanitizeIntentWarning("catalog unavailable; symbol evidence omitted: " + hostile); got != "catalog_unavailable" {
		t.Fatalf("sanitizeIntentWarning=%q want catalog_unavailable", got)
	}
	encoded, marshalErr := json.Marshal(model.Envelope{Ok: true, Warnings: []string{warning}, Items: []any{}})
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	jsonText := string(encoded)
	for _, forbidden := range []string{"secret-token", "private.db", "ana@example.test", "C:\\Users\\Ana"} {
		if strings.Contains(jsonText, forbidden) {
			t.Fatalf("warning payload leaked %q: %s", forbidden, jsonText)
		}
	}
}

func TestIntentExpansionShellSensitiveValuesUseStructuredPlaceholders(t *testing.T) {
	hostiles := []string{"$(whoami)", "`whoami`", "a;b", "a|b", `a"b`, "a b"}
	for _, hostile := range hostiles {
		t.Run(hostile, func(t *testing.T) {
			expansion := intentExpansionForPlan("workspace;secret", model.IntentPlan{
				Operation: "callers",
				Arguments: map[string]string{"selector": hostile, "repo": hostile},
			})
			for _, forbidden := range append(append([]string{}, hostiles...), "workspace;secret") {
				if strings.Contains(expansion.Command, forbidden) {
					t.Fatalf("raw shell-sensitive input %q leaked into command %q", forbidden, expansion.Command)
				}
			}
			if !strings.Contains(expansion.Command, intentPlaceholder("selector")) || !strings.Contains(expansion.Command, intentPlaceholder("workspace")) || !strings.Contains(expansion.Command, intentPlaceholder("repo")) {
				t.Fatalf("command=%q lacks structured placeholders", expansion.Command)
			}
			if expansion.Arguments["selector"] != hostile || expansion.Arguments["repo"] != intentPlaceholder("repo") || expansion.Arguments["workspace"] != "workspace;secret" {
				t.Fatalf("structured arguments=%#v", expansion.Arguments)
			}
		})
	}
}

func TestIntentExpansionsRejectUnsafeWorkspacePaths(t *testing.T) {
	unsafePaths := []string{
		`C:\\Users\\Ana\\private.cs`,
		`/etc/passwd`,
		`../private.cs`,
		`internal/../../private.cs`,
		`internal/private;secret.cs`,
	}
	for _, path := range unsafePaths {
		t.Run(path, func(t *testing.T) {
			plan := model.IntentPlan{Operation: "explain-change", Arguments: map[string]string{"paths": path}}
			explain := intentExplainChangeExpansionForPlan("demo", plan)
			if !strings.Contains(explain.Command, "--path "+intentPlaceholder("paths")) {
				t.Fatalf("explain command=%q, want inert path placeholder", explain.Command)
			}
			if strings.Contains(explain.Command, path) {
				t.Fatalf("unsafe path leaked into explain command=%q", explain.Command)
			}
			paths, ok := explain.Arguments["paths"].([]string)
			if !ok || len(paths) != 1 || paths[0] != path {
				t.Fatalf("explain arguments=%#v, want original path", explain.Arguments)
			}

			affected := intentAffectedExpansionForPlan("demo", model.IntentPlan{Operation: "affected-change", Arguments: map[string]string{"paths": path}})
			if !strings.Contains(affected.Command, intentPlaceholder("paths")) || strings.Contains(affected.Command, path) {
				t.Fatalf("unsafe path leaked into affected command=%q", affected.Command)
			}
			paths, ok = affected.Arguments["paths"].([]string)
			if !ok || len(paths) != 1 || paths[0] != path {
				t.Fatalf("affected arguments=%#v, want original path", affected.Arguments)
			}
		})
	}
}

func TestIntentExpansionsEmbedOnlySafeWorkspaceRelativePaths(t *testing.T) {
	plan := model.IntentPlan{Operation: "explain-change", Arguments: map[string]string{"paths": `internal\service\intent.go`}}
	expansion := intentExplainChangeExpansionForPlan("demo", plan)
	if !strings.Contains(expansion.Command, "--path internal/service/intent.go") {
		t.Fatalf("command=%q, want normalized safe workspace-relative path", expansion.Command)
	}
	if len(expansion.Arguments) != 0 {
		t.Fatalf("arguments=%#v, want no structured argument for safe path", expansion.Arguments)
	}
}

func TestIntentPlanAndExpansionUseCanonicalFuzzyRepo(t *testing.T) {
	project := model.ProjectFile{Repos: []model.WorkspaceRepo{{ID: "internal", Name: "internal", Root: "internal"}}}
	resolution := resolveRepoSelector(project, "intern")
	if resolution.Envelope != nil || resolution.Repo.Name != "internal" {
		t.Fatalf("resolution=%+v, want unique canonical internal repo", resolution)
	}
	plan := model.IntentPlan{Operation: "callers", Arguments: map[string]string{"selector": "Run", "repo": "intern"}}
	plan.Arguments["repo"] = resolution.Repo.Name
	if plan.Arguments["repo"] != "internal" {
		t.Fatalf("plan repo=%q, want internal", plan.Arguments["repo"])
	}
	expansion := intentExpansionForPlan("demo", plan)
	if !strings.Contains(expansion.Command, "--repo internal") || strings.Contains(expansion.Command, "--repo intern ") {
		t.Fatalf("expansion=%q, want canonical repo binding", expansion.Command)
	}
}

func TestIntentPlanRedactsUnpublishableRepoNameAndStopsExecution(t *testing.T) {
	hostile := `$(whoami)`
	repo := &model.WorkspaceRepo{Name: hostile, ID: "hostile", Root: "internal"}
	env, handled, err := New(t.TempDir(), nil).intentPlan(context.Background(), model.CommandRequest{
		Operation: "nav.intent",
		Context:   model.QueryOptions{Workspace: "demo"},
		Payload:   map[string]any{"question": "callers of Run", "repo": hostile},
	}, model.WorkspaceRegistration{Name: "demo"}, model.ProjectFile{}, "callers of Run", repo, nil)
	if err != nil || !handled {
		t.Fatalf("intentPlan handled=%v err=%v", handled, err)
	}
	plans, ok := env.Items.([]model.IntentPlan)
	if !ok || len(plans) != 1 {
		t.Fatalf("items=%T %#v", env.Items, env.Items)
	}
	plan := plans[0]
	if plan.Arguments["repo"] != intentPlaceholder("repo") || !plan.Incomplete || len(plan.Expansions) != 0 {
		t.Fatalf("plan=%+v, want redacted incomplete plan without executable expansion", plan)
	}
	encoded, marshalErr := json.Marshal(plan)
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	if strings.Contains(string(encoded), hostile) {
		t.Fatalf("hostile repo name leaked into plan JSON=%s", encoded)
	}
}

func TestIntentExpansionRejectsUnpublishableRepoNames(t *testing.T) {
	for _, hostile := range []string{`$(whoami)`, `../secret`, `C:\\Users\\Ana\\secret`} {
		t.Run(hostile, func(t *testing.T) {
			expansion := intentExpansionForPlan("demo", model.IntentPlan{
				Operation: "callers",
				Arguments: map[string]string{"selector": "Run", "repo": hostile},
			})
			if !strings.Contains(expansion.Command, "--repo "+intentPlaceholder("repo")) || strings.Contains(expansion.Command, hostile) {
				t.Fatalf("expansion command=%q, want inert repo placeholder", expansion.Command)
			}
			if expansion.Arguments["repo"] != intentPlaceholder("repo") {
				t.Fatalf("expansion arguments=%#v, want only stable placeholder", expansion.Arguments)
			}
		})
	}
}

func TestPathDiscoveryExpansionPreservesRepoWithoutShellInterpolation(t *testing.T) {
	hostile := "$(cat /tmp/secret);`whoami`"
	expansion := intentPathDiscoveryExpansionForPlan("workspace with spaces", "", hostile, map[string]string{"repo": "backend"})
	if !strings.Contains(expansion.Command, "--repo backend") || !strings.Contains(expansion.Command, intentPlaceholder("selector")) || !strings.Contains(expansion.Command, intentPlaceholder("workspace")) {
		t.Fatalf("discovery expansion=%q", expansion.Command)
	}
	if strings.Contains(expansion.Command, "$(") || strings.Contains(expansion.Command, "`") || strings.Contains(expansion.Command, ";") || expansion.Arguments["selector"] != hostile {
		t.Fatalf("hostile endpoint escaped structured continuation: command=%q args=%#v", expansion.Command, expansion.Arguments)
	}
}

func TestExplainChangeRefNormalizesToAffectedChangedRef(t *testing.T) {
	route, ok := classifySupportedIntent("explain change", map[string]any{"ref": `feature/quoted "ref"`})
	if !ok || route.Arguments["ref"] == "" || route.Arguments["changed_ref"] != route.Arguments["ref"] || route.Arguments["from_git_diff"] != "true" {
		t.Fatalf("route=%+v ok=%v, want semantic changed_ref normalization", route, ok)
	}
	expansion := intentAffectedExpansionForPlan("demo", model.IntentPlan{Operation: "affected-change", Arguments: route.Arguments})
	if !strings.Contains(expansion.Command, "--from-git-diff --include-tests") || !strings.Contains(expansion.Command, "--changed-ref "+intentPlaceholder("changed_ref")) {
		t.Fatalf("affected expansion=%q", expansion.Command)
	}
	if expansion.Arguments["changed_ref"] != `feature/quoted "ref"` {
		t.Fatalf("changed_ref args=%#v", expansion.Arguments)
	}
}

func TestGraphOmissionReasonNeverCrossesPlannerEnvelopeRaw(t *testing.T) {
	const hostile = "graph_unresolved path=C:\\Users\\Ana\\private.cs token=secret; $()"
	for _, code := range []string{"graph_unresolved", "GPH_QUERY_PRIVATE", ""} {
		got := sanitizeIntentOmissionReason(code, hostile)
		if got == hostile || strings.Contains(got, "private.cs") || strings.Contains(got, "secret") || strings.Contains(got, "$()") {
			t.Fatalf("code=%q produced unsafe omission reason %q", code, got)
		}
		if got != "graph_unresolved" && got != "GPH_QUERY_PRIVATE" && got != "graph_omission" {
			t.Fatalf("code=%q produced unexpected stable reason %q", code, got)
		}
	}
}
