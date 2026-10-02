package service

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

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
		{"ts file", csharp, "src/a.ts", "tsserver"},
		{"tsx file", csharp, "src/a.tsx", "tsserver"},
		{"js file", csharp, "src/a.js", "tsserver"},
		{"jsx file", csharp, "src/a.jsx", "tsserver"},
		{"python file", csharp, "a.py", "pyright"},
		{"csharp file", model.WorkspaceRegistration{Languages: []string{"go"}}, "A.cs", "roslyn"},
		{"markdown file is unsupported", csharp, "README.md", "text"},
		{"yaml file is unsupported", csharp, "ci.yaml", "text"},
		{"go-only registration", model.WorkspaceRegistration{Languages: []string{"go"}}, "", "gopls"},
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

func TestFindRefsUsesCatalogLanguageAndAnchor(t *testing.T) {
	root, alias := setupRefsWorkspace(t, []string{"csharp", "go"}, false, map[string]string{"demo.go": goSource}, goRefsSymbols())
	fake := &fakeSemanticCaller{callFn: func(context.Context, model.WorkspaceRegistration, model.WorkerRequest) (model.WorkerResponse, error) {
		return model.WorkerResponse{Ok: true, Backend: "gopls", Items: []map[string]any{{"file": filepath.Join(root, "demo.go"), "line": 6, "column": 9}}}, nil
	}}

	env := runRefs(t, root, alias, fake, map[string]any{"symbol": "Target"})

	if len(fake.calls) != 1 || fake.calls[0].BackendType != "gopls" {
		t.Fatalf("calls = %#v, want a single gopls call (no roslyn)", fake.calls)
	}
	payload := fake.calls[0].Payload
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

	if fake.calls[0].BackendType != "roslyn" {
		t.Fatalf("backend = %q, want roslyn for a C# symbol", fake.calls[0].BackendType)
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
	if len(fake.calls) != 0 {
		t.Fatalf("no semantic backend should start for non-code files, got %#v", fake.calls)
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

	if len(fake.calls) != 0 {
		t.Fatalf("no worker call expected without entrypoint, got %#v", fake.calls)
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
