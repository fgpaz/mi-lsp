package service

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

// setupRefsWorkspace registers a single-repo workspace with the given files
// and catalog symbols (nil symbols leaves the catalog empty).
func setupRefsWorkspace(t *testing.T, languages []string, withEntrypoint bool, files map[string]string, symbols []model.SymbolRecord) (string, string) {
	t.Helper()
	ensureWritableTestHome(t)
	root := t.TempDir()
	alias := "refs-" + filepath.Base(root)
	for path, content := range files {
		writeWorkspaceFile(t, root, path, content)
	}
	project := model.ProjectFile{
		Project: model.ProjectBlock{Name: alias, Languages: languages, Kind: model.WorkspaceKindSingle, DefaultRepo: "main"},
		Repos:   []model.WorkspaceRepo{{ID: "main", Name: "main", Root: ".", Languages: languages}},
	}
	if withEntrypoint {
		project.Repos[0].DefaultEntrypoint = "main::A.sln"
		project.Entrypoints = []model.WorkspaceEntrypoint{{ID: "main::A.sln", RepoID: "main", Path: "A.sln", Kind: model.EntrypointKindSolution, Default: true}}
	}
	if err := workspace.SaveProjectFile(root, project); err != nil {
		t.Fatalf("SaveProjectFile: %v", err)
	}
	if _, err := workspace.RegisterWorkspace(alias, model.WorkspaceRegistration{Name: alias, Root: root, Languages: languages, Kind: model.WorkspaceKindSingle}); err != nil {
		t.Fatalf("register workspace: %v", err)
	}
	t.Cleanup(func() { _ = workspace.RemoveWorkspace(alias) })
	if symbols != nil {
		db, err := store.Open(root)
		if err != nil {
			t.Fatalf("store.Open: %v", err)
		}
		defer db.Close()
		seen := map[string]bool{}
		var records []model.FileRecord
		for i := range symbols {
			symbols[i].RepoID, symbols[i].RepoName = "main", "main"
			if !seen[symbols[i].FilePath] {
				seen[symbols[i].FilePath] = true
				records = append(records, model.FileRecord{FilePath: symbols[i].FilePath, RepoID: "main", RepoName: "main", Language: symbols[i].Language})
			}
		}
		if err := store.ReplaceCatalog(context.Background(), db, project, records, symbols); err != nil {
			t.Fatalf("ReplaceCatalog: %v", err)
		}
	}
	return root, alias
}

func runRefs(t *testing.T, root string, alias string, fake *fakeSemanticCaller, payload map[string]any) model.Envelope {
	t.Helper()
	env, err := New(root, fake).Execute(context.Background(), model.CommandRequest{
		Operation: "nav.refs",
		Context:   model.QueryOptions{Workspace: alias, MaxItems: 20},
		Payload:   payload,
	})
	if err != nil {
		t.Fatalf("nav.refs: %v", err)
	}
	return env
}

func envItems(t *testing.T, env model.Envelope) []map[string]any {
	t.Helper()
	items, ok := env.Items.([]map[string]any)
	if !ok {
		t.Fatalf("items = %T, want []map[string]any", env.Items)
	}
	return items
}

const goSource = `package demo

func Target() int { return 1 }

func Caller() int {
	return Target()
}
`

func goRefsSymbols() []model.SymbolRecord {
	return []model.SymbolRecord{
		{FilePath: "demo.go", Name: "Target", Kind: "function", StartLine: 3, EndLine: 3, Language: "go", QualifiedName: "demo::Target"},
		{FilePath: "demo.go", Name: "Caller", Kind: "function", StartLine: 5, EndLine: 7, Language: "go", QualifiedName: "demo::Caller"},
	}
}

func TestResolveBackendTypeFindRefsBySymbolLanguage(t *testing.T) {
	csharp := model.WorkspaceRegistration{Languages: []string{"csharp"}}
	cases := []struct {
		name         string
		registration model.WorkspaceRegistration
		file         string
		want         string
	}{
		{"go file", csharp, "pkg/a.go", "gopls"},
		{"Rust file", csharp, "src/lib.rs", "rust-analyzer"},
		{"ts file", csharp, "src/a.ts", "tsserver"},
		{"tsx file", csharp, "src/a.tsx", "tsserver"},
		{"js file", csharp, "src/a.js", "tsserver"},
		{"jsx file", csharp, "src/a.jsx", "tsserver"},
		{"python file", csharp, "a.py", "pyright"},
		{"csharp file", model.WorkspaceRegistration{Languages: []string{"go"}}, "A.cs", "roslyn"},
		{"markdown file is unsupported", csharp, "README.md", "text"},
		{"yaml file is unsupported", csharp, "ci.yaml", "text"},
		{"go-only registration", model.WorkspaceRegistration{Languages: []string{"go"}}, "", "gopls"},
		{"Rust-only registration", model.WorkspaceRegistration{Languages: []string{"rust"}}, "", "rust-analyzer"},
		{"dominant registration language", model.WorkspaceRegistration{Languages: []string{"typescript", "csharp"}}, "", "tsserver"},
		{"empty registration never starts roslyn", model.WorkspaceRegistration{}, "", "text"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			request := model.CommandRequest{Payload: map[string]any{"symbol": "X", "file": tc.file}}
			if got := resolveBackendType(tc.registration, request, "find_refs"); got != tc.want {
				t.Fatalf("resolveBackendType = %q, want %q", got, tc.want)
			}
		})
	}
	if got := resolveBackendType(model.WorkspaceRegistration{Languages: []string{"go"}}, model.CommandRequest{Payload: map[string]any{}}, "get_deps"); got != "roslyn" {
		t.Fatalf("get_deps backend = %q, want roslyn", got)
	}
}

func TestFindRefsUsesRustAnalyzerAndCatalogAnchor(t *testing.T) {
	root, alias := setupRefsWorkspace(t, []string{"rust"}, false, map[string]string{"src/lib.rs": "pub fn run() {}\n"}, []model.SymbolRecord{{
		FilePath: "src/lib.rs", Name: "run", Kind: "function", StartLine: 1, EndLine: 1, Language: "rust", QualifiedName: "src/lib.rs::run",
	}})
	fake := &fakeSemanticCaller{callFn: func(_ context.Context, _ model.WorkspaceRegistration, request model.WorkerRequest) (model.WorkerResponse, error) {
		if request.BackendType != "rust-analyzer" {
			t.Fatalf("backend = %q, want rust-analyzer", request.BackendType)
		}
		return model.WorkerResponse{Ok: true, Backend: "rust-analyzer", Items: []map[string]any{{"file": filepath.Join(root, "src/lib.rs"), "line": 1, "column": 8}}}, nil
	}}

	env := runRefs(t, root, alias, fake, map[string]any{"symbol": "run"})
	calls := fake.requests()
	if len(calls) != 1 || calls[0].BackendType != "rust-analyzer" {
		t.Fatalf("calls = %#v, want a single rust-analyzer call", calls)
	}
	if calls[0].Payload["file"] != "src/lib.rs" || intFromAny(calls[0].Payload["line"], 0) != 1 {
		t.Fatalf("anchor payload = %#v, want src/lib.rs:1", calls[0].Payload)
	}
	if env.Backend != "rust-analyzer" || env.Degraded || len(envItems(t, env)) != 1 {
		t.Fatalf("env = backend %q degraded %v items %d", env.Backend, env.Degraded, len(envItems(t, env)))
	}
}

func TestResolveContextBackendTypeUsesRustAnalyzer(t *testing.T) {
	request := model.CommandRequest{Payload: map[string]any{"file": "src/lib.rs"}}
	if got := resolveContextBackendType(request); got != "rust-analyzer" {
		t.Fatalf("resolveContextBackendType(.rs) = %q, want rust-analyzer", got)
	}
}

func TestFindRefsUsesCatalogLanguageAndAnchor(t *testing.T) {
	root, alias := setupRefsWorkspace(t, []string{"csharp", "go"}, false, map[string]string{"demo.go": goSource}, goRefsSymbols())
	fake := &fakeSemanticCaller{callFn: func(context.Context, model.WorkspaceRegistration, model.WorkerRequest) (model.WorkerResponse, error) {
		return model.WorkerResponse{Ok: true, Backend: "gopls", Items: []map[string]any{{"file": filepath.Join(root, "demo.go"), "line": 6, "column": 9}}}, nil
	}}

	env := runRefs(t, root, alias, fake, map[string]any{"symbol": "Target"})

	calls := fake.requests()
	if len(calls) != 1 || calls[0].BackendType != "gopls" {
		t.Fatalf("calls = %#v, want a single gopls call (no roslyn)", calls)
	}
	payload := calls[0].Payload
	if payload["file"] != "demo.go" || intFromAny(payload["line"], 0) != 3 {
		t.Fatalf("anchor payload = %#v, want catalog definition demo.go:3", payload)
	}
	items := envItems(t, env)
	if env.Backend != "gopls" || env.Degraded || len(items) != 1 {
		t.Fatalf("env = backend %q degraded %v items %d", env.Backend, env.Degraded, len(items))
	}
	if items[0]["origin"] != "semantic" || items[0]["file"] != "demo.go" {
		t.Fatalf("item = %#v, want semantic origin and relative file", items[0])
	}
	caller, _ := items[0]["caller"].(map[string]any)
	if caller["name"] != "Caller" || caller["kind"] != "function" || intFromAny(caller["line"], 0) != 5 {
		t.Fatalf("caller = %#v, want Caller function at line 5", items[0]["caller"])
	}
}

func TestFindRefsEmptySemanticFallsBackToText(t *testing.T) {
	root, alias := setupRefsWorkspace(t, []string{"go"}, false, map[string]string{"demo.go": goSource}, goRefsSymbols())
	fake := &fakeSemanticCaller{callFn: func(context.Context, model.WorkspaceRegistration, model.WorkerRequest) (model.WorkerResponse, error) {
		return model.WorkerResponse{Ok: true, Backend: "gopls"}, nil
	}}

	env := runRefs(t, root, alias, fake, map[string]any{"symbol": "Target"})

	items := envItems(t, env)
	if len(items) == 0 {
		t.Fatalf("expected text hits, got none: %#v", env)
	}
	if !env.Degraded || env.Reason != model.ReasonSemanticEmptyTextHits || env.FallbackUsed != "text" || env.Backend != "text" {
		t.Fatalf("env = degraded %v reason %q fallback %q backend %q", env.Degraded, env.Reason, env.FallbackUsed, env.Backend)
	}
	for _, item := range items {
		if item["origin"] != "text" {
			t.Fatalf("item origin = %v, want text", item["origin"])
		}
	}
}

func TestFindRefsTextFallbackIsWordBounded(t *testing.T) {
	files := map[string]string{"demo.go": "package demo\n\nfunc Target() {}\n\nfunc TargetExtra() {}\n"}
	root, alias := setupRefsWorkspace(t, []string{"go"}, false, files, nil)
	fake := &fakeSemanticCaller{callFn: func(context.Context, model.WorkspaceRegistration, model.WorkerRequest) (model.WorkerResponse, error) {
		return model.WorkerResponse{}, errors.New("gopls is unavailable")
	}}

	env := runRefs(t, root, alias, fake, map[string]any{"symbol": "Target", "file": "demo.go"})

	items := envItems(t, env)
	if len(items) != 1 || intFromAny(items[0]["line"], 0) != 3 {
		t.Fatalf("items = %#v, want only the whole-word hit on line 3", items)
	}
}

func TestFindRefsEmptySemanticAndTextIsNoMatches(t *testing.T) {
	root, alias := setupRefsWorkspace(t, []string{"go"}, false, map[string]string{"demo.go": goSource}, goRefsSymbols())
	fake := &fakeSemanticCaller{callFn: func(context.Context, model.WorkspaceRegistration, model.WorkerRequest) (model.WorkerResponse, error) {
		return model.WorkerResponse{Ok: true, Backend: "gopls"}, nil
	}}

	for name, payload := range map[string]map[string]any{
		"no anchor":     {"symbol": "NoSuchSymbolAnywhere"},
		"explicit file": {"symbol": "NoSuchSymbolAnywhere", "file": "demo.go"},
	} {
		t.Run(name, func(t *testing.T) {
			env := runRefs(t, root, alias, fake, payload)
			items := envItems(t, env)
			if !env.Ok || env.Degraded || env.Reason != model.ReasonNoMatches || len(items) != 0 {
				t.Fatalf("env = ok %v degraded %v reason %q items %d, want ok no_matches empty", env.Ok, env.Degraded, env.Reason, len(items))
			}
		})
	}
}

func TestFinalizeRefsPreservesEmptyTimeoutFailure(t *testing.T) {
	env := model.Envelope{
		Ok:      false,
		Backend: "text",
		Reason:  "timeout",
		Error:   &model.EnvelopeError{Code: "timeout", ReasonCode: "timeout"},
		Items:   []map[string]any{},
	}
	got := (&App{}).finalizeRefs(context.Background(), model.WorkspaceRegistration{}, env, "Target", 0, time.Now())
	if got.Ok || got.Reason != "timeout" || got.Error == nil || got.Error.Code != "timeout" {
		t.Fatalf("finalizeRefs converted timeout into success: %+v", got)
	}
}

func TestFindRefsRoslynErrorFallsBackToText(t *testing.T) {
	files := map[string]string{"A.cs": "class A {\n  void Run() { Helper(); }\n  void Helper() {}\n}\n"}
	symbols := []model.SymbolRecord{
		{FilePath: "A.cs", Name: "Helper", Kind: "method", StartLine: 3, EndLine: 3, Language: "csharp"},
		{FilePath: "A.cs", Name: "Run", Kind: "method", StartLine: 2, EndLine: 2, Language: "csharp"},
		{FilePath: "A.cs", Name: "A", Kind: "class", StartLine: 1, EndLine: 4, Language: "csharp"},
	}
	root, alias := setupRefsWorkspace(t, []string{"csharp"}, true, files, symbols)
	fake := &fakeSemanticCaller{callFn: func(context.Context, model.WorkspaceRegistration, model.WorkerRequest) (model.WorkerResponse, error) {
		return model.WorkerResponse{}, errors.New("roslyn worker crashed")
	}}

	env := runRefs(t, root, alias, fake, map[string]any{"symbol": "Helper"})

	calls := fake.requests()
	if calls[0].BackendType != "roslyn" {
		t.Fatalf("backend = %q, want roslyn for a C# symbol", calls[0].BackendType)
	}
	items := envItems(t, env)
	if !env.Ok || !env.Degraded || env.Reason != model.ReasonLSPError || env.FallbackUsed != "text" || len(items) == 0 {
		t.Fatalf("env = ok %v degraded %v reason %q fallback %q items %d", env.Ok, env.Degraded, env.Reason, env.FallbackUsed, len(items))
	}
	var runCaller bool
	for _, item := range items {
		if caller, _ := item["caller"].(map[string]any); caller["name"] == "Run" {
			runCaller = true
		}
	}
	if !runCaller {
		t.Fatalf("no item attributed to caller Run: %#v", items)
	}
}

func TestFindRefsMissingBinaryIsLSPUnavailable(t *testing.T) {
	root, alias := setupRefsWorkspace(t, []string{"go"}, false, map[string]string{"demo.go": goSource}, goRefsSymbols())
	fake := &fakeSemanticCaller{callFn: func(context.Context, model.WorkspaceRegistration, model.WorkerRequest) (model.WorkerResponse, error) {
		return model.WorkerResponse{}, errors.New("gopls is unavailable; install it")
	}}

	env := runRefs(t, root, alias, fake, map[string]any{"symbol": "Target"})
	if !env.Degraded || env.Reason != model.ReasonLSPUnavailable || len(envItems(t, env)) == 0 {
		t.Fatalf("env = degraded %v reason %q items %d", env.Degraded, env.Reason, len(envItems(t, env)))
	}
}

func TestFindRefsNonCodeFileIsLanguageUnsupported(t *testing.T) {
	root, alias := setupRefsWorkspace(t, []string{"csharp"}, false, map[string]string{"docs/note.md": "see Widget here\n"}, nil)
	fake := &fakeSemanticCaller{}

	for name, payload := range map[string]map[string]any{
		"explicit file":  {"symbol": "Widget", "file": "docs/note.md"},
		"text hits only": {"symbol": "Widget"},
	} {
		t.Run(name, func(t *testing.T) {
			env := runRefs(t, root, alias, fake, payload)
			if !env.Degraded || env.Reason != model.ReasonLanguageUnsupported || env.FallbackUsed != "text" || len(envItems(t, env)) != 1 {
				t.Fatalf("env = degraded %v reason %q fallback %q items %d", env.Degraded, env.Reason, env.FallbackUsed, len(envItems(t, env)))
			}
		})
	}
	if calls := fake.requests(); len(calls) != 0 {
		t.Fatalf("no semantic backend should start for non-code files, got %#v", calls)
	}
}

func TestFindRefsContextLines(t *testing.T) {
	root, alias := setupRefsWorkspace(t, []string{"go"}, false, map[string]string{"demo.go": goSource}, goRefsSymbols())
	fake := &fakeSemanticCaller{callFn: func(context.Context, model.WorkspaceRegistration, model.WorkerRequest) (model.WorkerResponse, error) {
		return model.WorkerResponse{Ok: true, Backend: "gopls", Items: []map[string]any{{"file": "demo.go", "line": 6}}}, nil
	}}

	env := runRefs(t, root, alias, fake, map[string]any{"symbol": "Target", "context": 1})
	got, _ := envItems(t, env)[0]["context"].(string)
	want := "func Caller() int {\n\treturn Target()\n}"
	if got != want {
		t.Fatalf("context = %q, want %q", got, want)
	}

	env = runRefs(t, root, alias, fake, map[string]any{"symbol": "Target"})
	if _, ok := envItems(t, env)[0]["context"]; ok {
		t.Fatalf("context must be absent by default")
	}

	env = runRefs(t, root, alias, fake, map[string]any{"symbol": "Target", "context": 99})
	if lines := strings.Count(envItems(t, env)[0]["context"].(string), "\n") + 1; lines > 8 {
		t.Fatalf("context not clamped to 5 lines each side, got %d lines", lines)
	}
}

func TestInnermostSymbolPicksSmallestContainingRange(t *testing.T) {
	symbols := []model.SymbolRecord{
		{Name: "Outer", Kind: "class", StartLine: 1, EndLine: 20},
		{Name: "Method", Kind: "method", StartLine: 5, EndLine: 10},
		{Name: "Other", Kind: "method", StartLine: 12, EndLine: 15},
	}
	got, ok := innermostSymbol(symbols, 7, "X")
	if !ok || got.Name != "Method" {
		t.Fatalf("innermost at 7 = %#v, %v; want Method", got, ok)
	}
	got, ok = innermostSymbol(symbols, 11, "X")
	if !ok || got.Name != "Outer" {
		t.Fatalf("innermost at 11 = %#v, %v; want Outer", got, ok)
	}
	if _, ok := innermostSymbol(symbols, 30, "X"); ok {
		t.Fatalf("line outside every symbol must have no caller")
	}
	got, _ = innermostSymbol(symbols, 5, "Method")
	if got.Name != "Outer" {
		t.Fatalf("definition line of the queried symbol must not be its own caller, got %q", got.Name)
	}
}

func TestSemanticFailureReason(t *testing.T) {
	cases := map[string]string{
		"gopls is unavailable; install it":                     model.ReasonLSPUnavailable,
		"exec: \"dotnet\": executable file not found in $PATH": model.ReasonLSPUnavailable,
		"request timed out after 30s":                          model.ReasonLSPError,
		"file is required for LSP queries":                     model.ReasonLSPError,
	}
	for message, want := range cases {
		if got := semanticFailureReason(errors.New(message)); got != want {
			t.Errorf("semanticFailureReason(%q) = %q, want %q", message, got, want)
		}
	}
}

func TestFindRefsRoslynWithoutEntrypointFallsBackToText(t *testing.T) {
	root, alias := setupRefsWorkspace(t, []string{"csharp"}, false, map[string]string{"A.cs": "class A { void Helper() {} }\n"}, nil)
	fake := &fakeSemanticCaller{}

	env := runRefs(t, root, alias, fake, map[string]any{"symbol": "Helper"})

	if calls := fake.requests(); len(calls) != 0 {
		t.Fatalf("no worker call expected without entrypoint, got %#v", calls)
	}
	if !env.Ok || !env.Degraded || env.Reason != model.ReasonLSPError || len(envItems(t, env)) != 1 {
		t.Fatalf("env = ok %v degraded %v reason %q items %d", env.Ok, env.Degraded, env.Reason, len(envItems(t, env)))
	}
}

func TestFindRefsTextFallbackRanksCodeOfSymbolLanguage(t *testing.T) {
	files := map[string]string{
		"src/Repo.cs":              "class JournalRepository {}\n",
		"src/Other.cs":             "var r = new JournalRepository();\n",
		"src/helper.go":            "package x\n// JournalRepository mention\n",
		".docs/auditoria/run.yaml": "subject: JournalRepository\n",
		"docs/note.md":             "JournalRepository notes\n",
	}
	symbols := []model.SymbolRecord{{FilePath: "src/Repo.cs", Name: "JournalRepository", Kind: "class", StartLine: 1, EndLine: 1, Language: "csharp"}}
	root, alias := setupRefsWorkspace(t, []string{"csharp"}, true, files, symbols)
	fake := &fakeSemanticCaller{callFn: func(context.Context, model.WorkspaceRegistration, model.WorkerRequest) (model.WorkerResponse, error) {
		return model.WorkerResponse{}, errors.New("roslyn worker crashed")
	}}

	env := runRefs(t, root, alias, fake, map[string]any{"symbol": "JournalRepository"})

	var got []string
	for _, item := range envItems(t, env) {
		got = append(got, item["file"].(string))
	}
	if strings.Join(got, ",") != "src/Other.cs,src/Repo.cs" {
		t.Fatalf("files = %v, want only the .cs usages", got)
	}
	if env.Reason != model.ReasonLSPError {
		t.Fatalf("reason = %q, want lsp_error", env.Reason)
	}
}

func TestFindRefsTextFallbackNonCodeOnlyWhenNoCodeHits(t *testing.T) {
	files := map[string]string{
		".docs/auditoria/run.yaml": "subject: Widget\n",
		"docs/note.md":             "Widget notes\n",
		"cfg/app.yaml":             "name: Widget\n",
	}
	root, alias := setupRefsWorkspace(t, []string{"csharp"}, false, files, nil)

	env := runRefs(t, root, alias, &fakeSemanticCaller{}, map[string]any{"symbol": "Widget"})

	var got []string
	for _, item := range envItems(t, env) {
		got = append(got, item["file"].(string))
	}
	if strings.Join(got, ",") != "cfg/app.yaml,docs/note.md" || env.Reason != model.ReasonLanguageUnsupported {
		t.Fatalf("files = %v reason %q, want visible non-code hits with language_unsupported", got, env.Reason)
	}
}

func TestFindRefsNormalizesSemanticPaths(t *testing.T) {
	root, alias := setupRefsWorkspace(t, []string{"go"}, false, map[string]string{"demo.go": goSource}, goRefsSymbols())
	outside := filepath.Join(t.TempDir(), "elsewhere.go")
	fake := &fakeSemanticCaller{callFn: func(context.Context, model.WorkspaceRegistration, model.WorkerRequest) (model.WorkerResponse, error) {
		return model.WorkerResponse{Ok: true, Backend: "gopls", Items: []map[string]any{
			{"file": filepath.Join(root, "pkg", "demo.go"), "line": 1},
			{"file": "demo.go", "line": 6},
			{"file": outside, "line": 2},
		}}, nil
	}}

	items := envItems(t, runRefs(t, root, alias, fake, map[string]any{"symbol": "Target"}))

	want := []string{"pkg/demo.go", "demo.go", outside}
	for i, item := range items {
		if item["file"] != want[i] {
			t.Errorf("item %d file = %v, want %v", i, item["file"], want[i])
		}
	}
}

func TestRankRefsTextHitsFallsBackToOtherLanguageCode(t *testing.T) {
	hits := []map[string]any{
		{"file": ".docs/a.yaml"},
		{"file": "docs/note.md"},
		{"file": "web/app.ts"},
		{"file": "tools/run.py"},
	}

	items, codeOnly := rankRefsTextHits(hits, "gopls", 10)
	if !codeOnly || len(items) != 2 || items[0]["file"] != "web/app.ts" || items[1]["file"] != "tools/run.py" {
		t.Fatalf("items = %v codeOnly %v, want other-language code hits only", items, codeOnly)
	}

	hits = append(hits, map[string]any{"file": "pkg/x.go"})
	items, _ = rankRefsTextHits(hits, "gopls", 10)
	if len(items) != 1 || items[0]["file"] != "pkg/x.go" {
		t.Fatalf("items = %v, want only the symbol-language hit when present", items)
	}

	items, _ = rankRefsTextHits(nil, "gopls", 10)
	if items == nil || len(items) != 0 {
		t.Fatalf("items = %#v, want empty non-nil slice when rg finds nothing", items)
	}
}

func TestFindRefsOtherLanguageHitsStayDegradedText(t *testing.T) {
	files := map[string]string{"demo.go": goSource, "web/app.ts": "export const x = Orphan();\n"}
	root, alias := setupRefsWorkspace(t, []string{"go"}, false, files, nil)
	fake := &fakeSemanticCaller{callFn: func(context.Context, model.WorkspaceRegistration, model.WorkerRequest) (model.WorkerResponse, error) {
		return model.WorkerResponse{Ok: true, Backend: "gopls"}, nil
	}}

	env := runRefs(t, root, alias, fake, map[string]any{"symbol": "Orphan", "file": "demo.go"})

	items := envItems(t, env)
	if len(items) != 1 || items[0]["file"] != "web/app.ts" || items[0]["origin"] != "text" {
		t.Fatalf("items = %#v, want the .ts text hit", items)
	}
	if !env.Degraded || env.Reason != model.ReasonSemanticEmptyTextHits || env.Backend != "text" {
		t.Fatalf("env = degraded %v reason %q backend %q", env.Degraded, env.Reason, env.Backend)
	}
}

func TestFindRefsSemanticTimeoutFallsBackToTextAndCoolsDown(t *testing.T) {
	t.Setenv("MI_LSP_REFS_TIMEOUT", "200ms")
	root, alias := setupRefsWorkspace(t, []string{"go"}, false, map[string]string{"demo.go": goSource}, goRefsSymbols())
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	fake := &fakeSemanticCaller{callFn: func(ctx context.Context, _ model.WorkspaceRegistration, _ model.WorkerRequest) (model.WorkerResponse, error) {
		<-release
		return model.WorkerResponse{Ok: true, Backend: "gopls"}, nil
	}}
	app := New(root, fake)
	request := model.CommandRequest{Operation: "nav.refs", Context: model.QueryOptions{Workspace: alias, MaxItems: 20}, Payload: map[string]any{"symbol": "Target"}}

	started := time.Now()
	env, err := app.Execute(context.Background(), request)
	if err != nil {
		t.Fatalf("nav.refs: %v", err)
	}
	if elapsed := time.Since(started); elapsed > 4*time.Second {
		t.Fatalf("timeout fallback took %s", elapsed)
	}
	if !env.Ok || !env.Degraded || env.Reason != model.ReasonLSPError || env.FallbackUsed != "text" || env.Backend != "text" || len(envItems(t, env)) == 0 {
		t.Fatalf("env = ok %v degraded %v reason %q fallback %q backend %q items %d", env.Ok, env.Degraded, env.Reason, env.FallbackUsed, env.Backend, len(envItems(t, env)))
	}
	if !strings.Contains(strings.Join(env.Warnings, " "), "semantic backend timed out after") {
		t.Fatalf("missing timeout warning: %v", env.Warnings)
	}

	env, err = app.Execute(context.Background(), request)
	if err != nil {
		t.Fatalf("second nav.refs: %v", err)
	}
	if calls := fake.requests(); len(calls) != 1 {
		t.Fatalf("cooldown must skip the backend on the next call, calls = %d", len(calls))
	}
	if env.Backend != "text" || !env.Degraded || env.Reason != model.ReasonLSPError || !strings.Contains(strings.Join(env.Warnings, " "), "cooldown") {
		t.Fatalf("second env = backend %q degraded %v reason %q warnings %v", env.Backend, env.Degraded, env.Reason, env.Warnings)
	}
}

func TestFindRefsTSWarmupReturnsDegradedTextFallback(t *testing.T) {
	t.Setenv("MI_LSP_REFS_TIMEOUT", "8s")
	source := "export function Target() { return 1; }\nexport function Caller() { return Target(); }\n"
	root, alias := setupRefsWorkspace(t, []string{"typescript"}, false, map[string]string{"src/app.ts": source}, []model.SymbolRecord{
		{FilePath: "src/app.ts", Name: "Target", Kind: "function", StartLine: 1, EndLine: 1, Language: "typescript"},
	})
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	fake := &fakeSemanticCaller{callFn: func(context.Context, model.WorkspaceRegistration, model.WorkerRequest) (model.WorkerResponse, error) {
		<-release
		return model.WorkerResponse{Ok: true, Backend: "tsserver"}, nil
	}}
	app := New(root, fake)
	request := model.CommandRequest{Operation: "nav.refs", Context: model.QueryOptions{Workspace: alias, MaxItems: 20}, Payload: map[string]any{"symbol": "Target"}}

	env, err := app.Execute(context.Background(), request)
	if err != nil {
		t.Fatalf("nav.refs: %v", err)
	}
	items := envItems(t, env)
	if !env.Ok || !env.Degraded || env.Backend != "text" || env.FallbackUsed != "text" || len(items) == 0 {
		t.Fatalf("cold result lost its degraded text status: ok=%v degraded=%v backend=%q fallback=%q items=%d", env.Ok, env.Degraded, env.Backend, env.FallbackUsed, len(items))
	}
	if items[0]["origin"] != model.ItemOriginText {
		t.Fatalf("fallback origin = %#v, want text", items[0]["origin"])
	}
	if !strings.Contains(strings.Join(env.Warnings, " "), "tsserver is still warming") {
		t.Fatalf("missing warm-up warning: %v", env.Warnings)
	}
}

func TestTSRefsWarmupSurvivesRequestAndEnablesSemanticRetry(t *testing.T) {
	source := "export function Target() { return 1; }\n"
	root, alias := setupRefsWorkspace(t, []string{"typescript"}, false, map[string]string{"src/app.ts": source}, []model.SymbolRecord{
		{FilePath: "src/app.ts", Name: "Target", Kind: "function", StartLine: 1, EndLine: 1, Language: "typescript"},
	})
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseWarmup := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(releaseWarmup)
	started := make(chan struct{})
	probeCancellation := make(chan struct{})
	backgroundContextErr := make(chan error, 1)
	warmupFinished := make(chan struct{})
	var calls atomic.Int32
	fake := &fakeSemanticCaller{callFn: func(ctx context.Context, _ model.WorkspaceRegistration, _ model.WorkerRequest) (model.WorkerResponse, error) {
		if calls.Add(1) == 1 {
			close(started)
			<-probeCancellation
			backgroundContextErr <- ctx.Err()
			if ctx.Err() != nil {
				return model.WorkerResponse{}, ctx.Err()
			}
			<-release
			close(warmupFinished)
		}
		return model.WorkerResponse{Ok: true, Backend: "tsserver", Items: []map[string]any{{"file": "src/app.ts", "line": 1, "origin": "semantic"}}}, nil
	}}
	app := New(root, fake)
	request := model.CommandRequest{Operation: "nav.refs", Context: model.QueryOptions{Workspace: alias, MaxItems: 20}, Payload: map[string]any{"symbol": "Target"}}
	ctx, cancel := context.WithCancel(context.Background())
	first, err := app.Execute(ctx, request)
	if err != nil {
		cancel()
		t.Fatalf("first nav.refs: %v", err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		cancel()
		t.Fatal("tsserver warm-up did not start")
	}
	cancel()
	close(probeCancellation)
	if ctxErr := <-backgroundContextErr; ctxErr != nil {
		t.Fatalf("request cancellation reached detached tsserver warm-up: %v", ctxErr)
	}
	if !first.Degraded || first.Backend != "text" || first.FallbackUsed != "text" || len(envItems(t, first)) == 0 {
		t.Fatalf("first result must be visibly degraded text: degraded=%v backend=%q fallback=%q", first.Degraded, first.Backend, first.FallbackUsed)
	}
	releaseWarmup()
	<-warmupFinished

	second, err := app.Execute(context.Background(), request)
	if err != nil {
		t.Fatalf("second nav.refs: %v", err)
	}
	secondItems := envItems(t, second)
	if second.Backend != "tsserver" || second.Degraded || len(secondItems) == 0 || secondItems[0]["origin"] != model.ItemOriginSemantic || calls.Load() != 2 {
		t.Fatalf("semantic retry = backend %q degraded %v items %#v calls %d", second.Backend, second.Degraded, secondItems, calls.Load())
	}
}

func TestSemanticRefsFallbackBudgetForWarmup(t *testing.T) {
	if got := semanticRefsFallbackBudget(&semanticTimeoutError{Warmup: true}); got != 1200*time.Millisecond {
		t.Fatalf("warm-up fallback budget = %s, want 1.2s", got)
	}
	if got := semanticRefsFallbackBudget(&semanticTimeoutError{}); got != refsTimedOutTextBudget {
		t.Fatalf("regular fallback budget = %s, want %s", got, refsTimedOutTextBudget)
	}
}

func TestRefsSemanticTimeoutConfiguration(t *testing.T) {
	t.Setenv("MI_LSP_REFS_TIMEOUT", "")
	if got := refsSemanticTimeout(context.Background()); got != 8*time.Second {
		t.Fatalf("default = %s, want 8s", got)
	}
	t.Setenv("MI_LSP_REFS_TIMEOUT", "3")
	if got := refsSemanticTimeout(context.Background()); got != 3*time.Second {
		t.Fatalf("plain seconds = %s, want 3s", got)
	}
	t.Setenv("MI_LSP_REFS_TIMEOUT", "1m")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if got := refsSemanticTimeout(ctx); got >= 2*time.Second || got < time.Second {
		t.Fatalf("shorter ctx deadline must cap the timeout, got %s", got)
	}
}
