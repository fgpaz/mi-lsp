package service

import (
	"context"
	"database/sql"
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

func TestExtractIntentSelectorSupportsExplicitRelativePathsWithoutGuessing(t *testing.T) {
	tests := []struct {
		name     string
		question string
		payload  map[string]any
		want     string
	}{
		{"slash path", "show neighborhood of internal/service/wiki_code_context.go", nil, "internal/service/wiki_code_context.go"},
		{"backslash path", `show neighborhood of internal\service\wiki_code_context.go`, nil, "internal/service/wiki_code_context.go"},
		{"mixed case quoted path", `show neighborhood of "Internal\Service\Wiki_Code_Context.Go"`, nil, "Internal/Service/Wiki_Code_Context.Go"},
		{"unchanged symbol", "show neighborhood of HandleRequest", nil, "HandleRequest"},
		{"explicit payload wins", "show neighborhood of internal/service/wiki_code_context.go", map[string]any{"selector": "HandleRequest"}, "HandleRequest"},
		{"ambiguous paths", "show neighborhood of internal/service/one.go and internal/service/two.go", nil, ""},
		{"unsafe absolute path", "show neighborhood of /etc/Private.go", nil, ""},
		{"unsafe shell path", "show neighborhood of internal/private;Secret.go", nil, ""},
		{"unsafe url", "show neighborhood of https://example.test/Private.go", nil, ""},
		{"missing selector", "show neighborhood of", nil, ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			route, ok := classifySupportedIntent(test.question, test.payload)
			if !ok || route.Operation != "neighborhood" {
				t.Fatalf("route=%+v ok=%v", route, ok)
			}
			if got := route.Arguments["selector"]; got != test.want {
				t.Fatalf("selector=%q, want %q; arguments=%+v", got, test.want, route.Arguments)
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

func intentTermWords(terms []intentTerm) string {
	words := make([]string, len(terms))
	for i, term := range terms {
		words[i] = term.Word
	}
	return " " + strings.Join(words, " ") + " "
}

func TestIntentTermsDropStopwordsAndSplitIdentifiers(t *testing.T) {
	terms := intentTerms("where is the workspace registry garbage collected")
	joined := intentTermWords(terms)
	for _, want := range []string{"workspace", "registry", "garbage", "collected"} {
		if !strings.Contains(joined, " "+want+" ") {
			t.Fatalf("terms=%v missing %q", terms, want)
		}
	}
	for _, noise := range []string{"where", "the", "is"} {
		if strings.Contains(joined, " "+noise+" ") {
			t.Fatalf("terms=%v keep stopword %q", terms, noise)
		}
	}
	if terms[3].Patterns[0] != "collect" {
		t.Fatalf("collected patterns=%v want stem collect", terms[3].Patterns)
	}
	split := intentTerms("donde se llama HandleDaemon_error en internal/service/app.go")
	joined = intentTermWords(split)
	for _, want := range []string{"handle", "daemon", "error", "internal", "service", "app"} {
		if !strings.Contains(joined, " "+want+" ") {
			t.Fatalf("split terms=%v missing %q", split, want)
		}
	}
	if strings.Contains(joined, " donde ") || strings.Contains(joined, " llama ") {
		t.Fatalf("split terms=%v keep spanish stopwords", split)
	}
}

func TestIntentStemAndAliases(t *testing.T) {
	for word, want := range map[string]string{
		"collected": "collect", "scores": "score", "settled": "settl", "submitted": "submit",
		"entries": "entry", "checked": "check", "class": "class", "registry": "registry",
	} {
		if got := intentStem(word); got != want {
			t.Errorf("intentStem(%q)=%q want %q", word, got, want)
		}
	}
	dedup := intentTerm{Word: "deduplicate", Patterns: intentTermPatterns("deduplicate")}
	if !intentTermMatches(dedup, "dedupmessage", intentFieldParts("DedupMessage")) {
		t.Fatalf("deduplicate must match Dedup*: %v", dedup.Patterns)
	}
	garbage := intentTerm{Word: "garbage", Patterns: intentTermPatterns("garbage")}
	if !intentTermMatches(garbage, "gcregistry", intentFieldParts("GCRegistry")) || intentTermMatches(garbage, "msgcount", intentFieldParts("msgcount")) {
		t.Fatalf("gc alias must match only the whole part: %v", garbage.Patterns)
	}
}

func TestHasIntentCodeSignals(t *testing.T) {
	for question, want := range map[string]bool{
		"how does WorkspaceRegistry garbage collect":     true,
		"why is index_not_ready returned":                true,
		"what calls service.Execute":                     true,
		"explain internal/workspace/registry.go":         true,
		"which struct holds the registry lock":           true,
		"where is the lock implemented":                  true,
		"donde se define el lock del registry":           true,
		"How does the governance model work":             false,
		"what requirements cover the onboarding journey": false,
	} {
		if got := hasIntentCodeSignals(question); got != want {
			t.Errorf("hasIntentCodeSignals(%q)=%v want %v", question, got, want)
		}
	}
}

func intentMixedFixture(t *testing.T) (string, string) {
	t.Helper()
	root, alias := setupTestWorkspace(t)
	db, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	symbols := []model.SymbolRecord{
		{FilePath: "internal/workspace/registry.go", RepoID: "main", RepoName: "main", Name: "CollectGarbage", Kind: "function", StartLine: 40, EndLine: 60, QualifiedName: "internal/workspace/registry.go::CollectGarbage", Language: "go", SearchText: "collect garbage function workspace registry"},
		{FilePath: "internal/other/render.go", RepoID: "main", RepoName: "main", Name: "RenderWorkspace", Kind: "function", StartLine: 5, EndLine: 9, QualifiedName: "internal/other/render.go::RenderWorkspace", Language: "go", SearchText: "render workspace function render"},
	}
	files := []model.FileRecord{
		{FilePath: "internal/workspace/registry.go", RepoID: "main", RepoName: "main", Language: "go"},
		{FilePath: "internal/other/render.go", RepoID: "main", RepoName: "main", Language: "go"},
	}
	if err := store.ReplaceCatalog(context.Background(), db, testProject(alias), files, symbols); err != nil {
		t.Fatal(err)
	}
	return root, alias
}

func TestIntentMixedPutsStrongCodeBeforeDocs(t *testing.T) {
	root, alias := intentMixedFixture(t)
	env, err := New(root, nil).Execute(context.Background(), model.CommandRequest{
		Operation: "nav.intent",
		Context:   model.QueryOptions{Workspace: alias},
		Payload:   map[string]any{"question": "where is the workspace registry garbage collected", "top": 10},
	})
	if err != nil {
		t.Fatal(err)
	}
	if env.Mode != "mixed" {
		t.Fatalf("mode=%q want mixed (items=%#v)", env.Mode, env.Items)
	}
	items, ok := env.Items.([]map[string]any)
	if !ok || len(items) == 0 {
		t.Fatalf("items=%T %#v", env.Items, env.Items)
	}
	if items[0]["result_kind"] != "code" || items[0]["file"] != "internal/workspace/registry.go" || items[0]["origin"] != model.ItemOriginCatalog || items[0]["kind"] != "function" {
		t.Fatalf("first item=%#v want strong registry.go code match", items[0])
	}
	for _, item := range items {
		if item["result_kind"] != "code" && item["result_kind"] != "doc" {
			t.Fatalf("item without code|doc kind: %#v", item)
		}
	}
}

func TestIntentDocsQuestionWithoutCodeSignalsStaysDocs(t *testing.T) {
	root, alias := intentMixedFixture(t)
	env, err := New(root, nil).Execute(context.Background(), model.CommandRequest{
		Operation: "nav.intent",
		Context:   model.QueryOptions{Workspace: alias},
		Payload:   map[string]any{"question": "how does the governance approval process work", "top": 10},
	})
	if err != nil {
		t.Fatal(err)
	}
	if env.Mode != "docs" {
		t.Fatalf("mode=%q want docs", env.Mode)
	}
}

func TestIntentMixWithCodeWithoutCatalogReturnsDocsUntouched(t *testing.T) {
	docs := model.Envelope{Ok: true, Mode: "docs", Items: []map[string]any{{"result_kind": "doc", "doc_path": "a.md"}}}
	got := New(t.TempDir(), nil).intentMixWithCode(context.Background(), model.WorkspaceRegistration{Name: "demo", Root: t.TempDir()}, "where is RegistryLock implemented", 10, 0, nil, docs)
	if got.Mode != "docs" || len(got.Items.([]map[string]any)) != 1 {
		t.Fatalf("envelope=%+v want docs untouched", got)
	}
}

func TestIsStrongIntentCodeMatch(t *testing.T) {
	if !isStrongIntentCodeMatch(intentMatch{Exact: true, Matched: 1, Total: 5}) {
		t.Fatal("exact symbol name must be strong")
	}
	if !isStrongIntentCodeMatch(intentMatch{Named: true, Matched: 2, Total: 9}) {
		t.Fatal("a file named after the question must be strong")
	}
	if !isStrongIntentCodeMatch(intentMatch{Matched: 2, Total: 4}) {
		t.Fatal("50% coverage must be strong")
	}
	if isStrongIntentCodeMatch(intentMatch{Matched: 1, Total: 4}) || isStrongIntentCodeMatch(intentMatch{Matched: 1, Total: 2}) {
		t.Fatal("low coverage must be weak")
	}
	if isStrongIntentCodeMatch(intentMatch{Matched: 1, Total: 1}) {
		t.Fatal("a single covered term is not strong without an exact name")
	}
}

func TestIntentQuestionIdentifiersOnlyKeepIdentifierLikeWords(t *testing.T) {
	got := strings.Join(intentQuestionIdentifiers("where does the gateway call NewServer or dedup_message in gateway/service.go"), ",")
	if got != "newserver,dedup_message" {
		t.Fatalf("identifiers=%q", got)
	}
	if got := strings.Join(intentQuestionIdentifiers("RegistryLock"), ","); got != "registrylock" {
		t.Fatalf("short question identifiers=%q", got)
	}
}

func TestIsIntentTestPath(t *testing.T) {
	for path, want := range map[string]bool{
		"internal/workspace/registry_test.go": true, "src/app.test.ts": true, "src/Tests/helper.cs": false,
		"pkg/tests/helper.go": true, "internal/workspace/registry.go": false, "src/latest.go": false,
	} {
		if got := isIntentTestPath(strings.ToLower(path)); got != want && path != "src/Tests/helper.cs" {
			t.Errorf("isIntentTestPath(%q)=%v want %v", path, got, want)
		}
	}
	if !isIntentTestPath(strings.ToLower("src/Tests/helper.cs")) {
		t.Error("Tests/ directory must count as test path")
	}
}

func intentRankingFixture(t *testing.T, symbols []model.SymbolRecord) (*sql.DB, string) {
	t.Helper()
	root, alias := setupTestWorkspace(t)
	db, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	seen := map[string]bool{}
	files := []model.FileRecord{}
	for i := range symbols {
		symbols[i].RepoID, symbols[i].RepoName, symbols[i].Language = "main", "main", "go"
		if symbols[i].QualifiedName == "" {
			symbols[i].QualifiedName = symbols[i].FilePath + "::" + symbols[i].Name
		}
		if !seen[symbols[i].FilePath] {
			seen[symbols[i].FilePath] = true
			files = append(files, model.FileRecord{FilePath: symbols[i].FilePath, RepoID: "main", RepoName: "main", Language: "go"})
		}
	}
	if err := store.ReplaceCatalog(context.Background(), db, testProject(alias), files, symbols); err != nil {
		t.Fatal(err)
	}
	return db, alias
}

func intentRankedFiles(t *testing.T, db *sql.DB, question string, topN int) []intentMatch {
	t.Helper()
	matches, err := intentCodeSearch(context.Background(), db, intentTerms(question), intentQuestionIdentifiers(question), topN, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	return matches
}

func TestIntentFileLevelRankingPrefersRareTermCoverageOverCommonTermRepetition(t *testing.T) {
	symbols := []model.SymbolRecord{}
	// Generic terms ("message", "types") repeat in many symbols of a types file.
	for i, name := range []string{"MessageKind", "MessageEnvelope", "MessagePayload", "MessageHeader", "MessageFlags", "MessageTypes", "MessageOptions"} {
		symbols = append(symbols, model.SymbolRecord{FilePath: "botapi/types.go", Name: name, Kind: "type", StartLine: i + 1, EndLine: i + 1})
	}
	// Filler files make "message" common and "dedup"/"accept" rare.
	for i := 0; i < 8; i++ {
		symbols = append(symbols, model.SymbolRecord{FilePath: fmt.Sprintf("adapter/file%d.go", i), Name: fmt.Sprintf("SendMessage%d", i), Kind: "function", StartLine: 1, EndLine: 2})
	}
	symbols = append(symbols,
		model.SymbolRecord{FilePath: "gateway/service.go", Name: "AcceptSubmitted", Kind: "function", StartLine: 30, EndLine: 60},
		model.SymbolRecord{FilePath: "gateway/service.go", Name: "dedupByID", Kind: "function", StartLine: 70, EndLine: 90},
		model.SymbolRecord{FilePath: "gateway/service_test.go", Name: "TestAcceptSubmittedDedup", Kind: "function", StartLine: 5, EndLine: 9},
	)
	db, _ := intentRankingFixture(t, symbols)

	matches := intentRankedFiles(t, db, "where does the gateway accept a submitted message and deduplicate it by message id", 5)
	if len(matches) == 0 {
		t.Fatal("no matches")
	}
	if matches[0].Symbol.FilePath != "gateway/service.go" {
		t.Fatalf("first file=%q want gateway/service.go (matches=%+v)", matches[0].Symbol.FilePath, matches)
	}
	if matches[0].Symbol.Name != "AcceptSubmitted" && matches[0].Symbol.Name != "dedupByID" {
		t.Fatalf("best symbol=%q want one of the matching symbols", matches[0].Symbol.Name)
	}
	if !isStrongIntentCodeMatch(matches[0]) {
		t.Fatalf("first match must be strong: %+v", matches[0])
	}
	seen := map[string]bool{}
	for _, match := range matches {
		if seen[match.Symbol.FilePath] {
			t.Fatalf("duplicate file %q: one item per file", match.Symbol.FilePath)
		}
		seen[match.Symbol.FilePath] = true
	}
	var testScore, srcScore float64
	for _, match := range matches {
		switch match.Symbol.FilePath {
		case "gateway/service_test.go":
			testScore = match.Score
		case "gateway/service.go":
			srcScore = match.Score
		}
	}
	if testScore >= srcScore {
		t.Fatalf("test file score %.2f must be below source %.2f", testScore, srcScore)
	}
}

func TestIntentFileLevelRankingFindsRegistryGarbageCollection(t *testing.T) {
	symbols := []model.SymbolRecord{
		{FilePath: "internal/workspace/registry.go", Name: "GarbageCollectRegistry", Kind: "function", StartLine: 40, EndLine: 80},
		{FilePath: "internal/workspace/registry.go", Name: "loadRegistry", Kind: "function", StartLine: 10, EndLine: 30},
		{FilePath: "internal/workspace/registry_lock.go", Name: "acquireLock", Kind: "function", StartLine: 12, EndLine: 40},
		{FilePath: "internal/workspace/autoregister.go", Name: "AutoRegister", Kind: "function", StartLine: 3, EndLine: 9},
		{FilePath: "internal/service/workspace_map.go", Name: "WorkspaceMap", Kind: "function", StartLine: 3, EndLine: 9},
		{FilePath: "internal/service/workspace_status.go", Name: "WorkspaceStatus", Kind: "function", StartLine: 3, EndLine: 9},
		{FilePath: "README.md", Name: "Workspace registry", Kind: "heading", StartLine: 1, EndLine: 2},
	}
	db, _ := intentRankingFixture(t, symbols)
	matches := intentRankedFiles(t, db, "where is the workspace registry garbage collected", 5)
	if len(matches) == 0 || matches[0].Symbol.FilePath != "internal/workspace/registry.go" || matches[0].Symbol.Name != "GarbageCollectRegistry" {
		t.Fatalf("matches=%+v want registry.go/GarbageCollectRegistry first", matches)
	}
	lock := intentRankedFiles(t, db, "how does the registry file lock time out when another process holds it", 5)
	if len(lock) == 0 || lock[0].Symbol.FilePath != "internal/workspace/registry_lock.go" {
		t.Fatalf("lock matches=%+v want registry_lock.go first", lock)
	}
	for _, match := range matches {
		if match.Symbol.FilePath == "README.md" && match.Score >= matches[0].Score {
			t.Fatalf("non-code file outranks code: %+v", matches)
		}
	}
}

func TestIntentFileLevelRankingExactSymbolNameWins(t *testing.T) {
	symbols := []model.SymbolRecord{
		{FilePath: "pkg/a/rate_limiter.go", Name: "TokenBucket", Kind: "type", StartLine: 4, EndLine: 20},
		{FilePath: "pkg/b/other.go", Name: "NewTokenBucketPool", Kind: "function", StartLine: 4, EndLine: 20},
		{FilePath: "pkg/b/other.go", Name: "Refill", Kind: "function", StartLine: 30, EndLine: 40},
	}
	db, _ := intentRankingFixture(t, symbols)
	matches := intentRankedFiles(t, db, "who uses TokenBucket", 5)
	if len(matches) < 2 || !matches[0].Exact || matches[0].Symbol.Name != "TokenBucket" || matches[0].Symbol.FilePath != "pkg/a/rate_limiter.go" {
		t.Fatalf("matches=%+v want exact-name file first", matches)
	}
	if !isStrongIntentCodeMatch(matches[0]) || matches[1].Exact {
		t.Fatalf("only the exact-name file is exact: %+v", matches)
	}
}

func TestIntentFileLevelRankingPrefersFileNamedAfterTheQuestion(t *testing.T) {
	symbols := []model.SymbolRecord{
		{FilePath: "src/workflows/workflow-settlement.ts", Name: "WorkflowSettlementPlan", Kind: "type", StartLine: 3, EndLine: 9},
		{FilePath: "src/runs/executor.ts", Name: "settleWorkflowSteerInbox", Kind: "function", StartLine: 3, EndLine: 9},
		{FilePath: "src/runs/executor.ts", Name: "partialBudgetTurn", Kind: "function", StartLine: 12, EndLine: 19},
		{FilePath: "src/runs/other.ts", Name: "budgetPartial", Kind: "function", StartLine: 3, EndLine: 9},
		{FilePath: "src/runs/returned.ts", Name: "returnedValue", Kind: "function", StartLine: 3, EndLine: 9},
	}
	db, _ := intentRankingFixture(t, symbols)
	matches := intentRankedFiles(t, db, "where is a workflow settled as partial when the turn budget is exceeded", 5)
	rank := -1
	for i, match := range matches {
		if match.Symbol.FilePath == "src/workflows/workflow-settlement.ts" {
			rank = i
			if !match.Named || !isStrongIntentCodeMatch(match) {
				t.Fatalf("settlement file must be Named and strong: %+v", match)
			}
		}
		if match.Symbol.FilePath == "src/runs/returned.ts" {
			t.Fatalf("\"turn\" must not match inside \"returned\": %+v", match)
		}
	}
	if rank < 0 || rank > 1 {
		t.Fatalf("settlement rank=%d want top 2 (matches=%+v)", rank, matches)
	}
}

func TestIntentMixWithCodeMarksDegradedWhenCatalogUnavailableForCodeQuestion(t *testing.T) {
	docs := model.Envelope{Ok: true, Mode: "docs", Items: []map[string]any{{"result_kind": "doc", "doc_path": "a.md"}}}
	registration := model.WorkspaceRegistration{Name: "demo", Root: t.TempDir()}
	got := New(t.TempDir(), nil).intentMixWithCode(context.Background(), registration, "where is textReferenceFallback implemented", 10, 0, nil, docs)
	if !got.Ok || got.Mode != "docs" || len(got.Items.([]map[string]any)) != 1 {
		t.Fatalf("envelope=%+v want docs kept", got)
	}
	if !got.Degraded || got.Reason != model.ReasonIndexNotReady || got.FallbackUsed != "" {
		t.Fatalf("degraded=%v reason=%q fallback=%q want degraded index_not_ready without fallback", got.Degraded, got.Reason, got.FallbackUsed)
	}
	if len(got.Warnings) != 1 || !strings.Contains(got.Warnings[0], "mi-lsp nav search textReferenceFallback") {
		t.Fatalf("warnings=%v want nav search suggestion", got.Warnings)
	}
}

func TestIntentCatalogUnavailableClassifiesSchemaAndSkipsNonCodeQuestions(t *testing.T) {
	docs := model.Envelope{Ok: true, Mode: "docs"}
	broken := intentCatalogUnavailable(docs, "where is RegistryLock", true, errors.New("no such table: symbols"))
	if broken.Reason != model.ReasonIndexSchemaBroken || !broken.Degraded {
		t.Fatalf("broken=%+v want index_schema_broken", broken)
	}
	plain := intentCatalogUnavailable(docs, "how does governance work", false, errors.New("unable to open"))
	if plain.Degraded || plain.Reason != "" || len(plain.Warnings) != 0 {
		t.Fatalf("plain=%+v want untouched docs", plain)
	}
}
