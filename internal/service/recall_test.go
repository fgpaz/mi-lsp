package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

func boolPtr(value bool) *bool {
	return &value
}

// buildVector creates a deterministic vector based on text concepts
func buildVector(text string) []float32 {
	dim := 8
	v := make([]float32, dim)
	lower := strings.ToLower(text)

	// Axis 0: acidification concept (EN + ES)
	if strings.Contains(lower, "acidif") || strings.Contains(lower, "ferment") ||
		strings.Contains(lower, "acidificacion") || strings.Contains(lower, "fermentacion") ||
		strings.Contains(lower, "biologica") {
		v[0] = 0.9
	} else {
		v[0] = 0.1
	}

	// Axis 1: logistica/facturacion (distractor)
	if strings.Contains(lower, "logistica") || strings.Contains(lower, "transporte") ||
		strings.Contains(lower, "facturacion") || strings.Contains(lower, "factura") {
		v[1] = 0.8
	} else {
		v[1] = 0.1
	}

	// Axis 2: biological concept
	if strings.Contains(lower, "biologica") || strings.Contains(lower, "biological") {
		v[2] = 0.7
	} else {
		v[2] = 0.2
	}

	// Remaining axes: low values or storage-specific
	for i := 3; i < dim; i++ {
		if strings.Contains(lower, "storage") || strings.Contains(lower, "almacen") {
			v[i] = 0.3
		} else {
			v[i] = 0.15
		}
	}

	// Normalize to unit vector
	norm := float32(0.0)
	for _, val := range v {
		norm += val * val
	}
	norm = float32(math.Sqrt(float64(norm)))
	if norm > 0 {
		for i := range v {
			v[i] /= norm
		}
	}

	return v
}

// newFakeEmbeddings creates a deterministic httptest server for embeddings
func newFakeEmbeddings(t *testing.T) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read body", http.StatusBadRequest)
			return
		}
		defer r.Body.Close()

		var req map[string]any
		if err := json.Unmarshal(body, &req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}

		var inputs []string
		if input, ok := req["input"].(string); ok {
			inputs = []string{input}
		} else if inputArr, ok := req["input"].([]any); ok {
			for _, item := range inputArr {
				if s, ok := item.(string); ok {
					inputs = append(inputs, s)
				}
			}
		}

		if len(inputs) == 0 {
			http.Error(w, "no input provided", http.StatusBadRequest)
			return
		}

		// Build embeddings response
		var embeddings [][]float32
		for _, input := range inputs {
			embeddings = append(embeddings, buildVector(input))
		}

		resp := map[string]any{
			"object": "list",
			"data": func() []map[string]any {
				var data []map[string]any
				for i, emb := range embeddings {
					data = append(data, map[string]any{
						"object":    "embedding",
						"embedding": emb,
						"index":     i,
					})
				}
				return data
			}(),
			"model": "fake",
			"usage": map[string]any{
				"prompt_tokens":     len(inputs),
				"total_tokens":      len(inputs),
				"completion_tokens": 0,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))

	return server
}

// TestRecall_BilingualEStoEN tests that a Spanish query retrieves an English note by semantic meaning
func TestRecall_BilingualEStoEN(t *testing.T) {
	server := newFakeEmbeddings(t)
	defer server.Close()

	ensureWritableTestHome(t)
	root := t.TempDir()

	// Add project marker
	writeWorkspaceFile(t, root, "go.mod", "module wiki-semantic-test\n\ngo 1.24\n")

	// Create fixture notes
	writeWorkspaceFile(t, root, ".docs/wiki/acidification.md", strings.Join([]string{
		"# Acidification in Biological Systems",
		"",
		"This document describes acidification processes in microbial fermentation systems.",
		"",
		"## Biological acidification",
		"",
		"Acidification through microbial fermentation is a natural process where bacteria and fungi lower the pH of a medium by producing organic acids. Lactobacillus species are common acidifiers in fermented foods.",
		"",
		"## Storage",
		"",
		"Acidified foods should be stored in cool, dry conditions.",
	}, "\n"))

	writeWorkspaceFile(t, root, ".docs/wiki/otros.md", strings.Join([]string{
		"# Otros Temas de Negocio",
		"",
		"## Logística de transporte",
		"",
		"La logística de transporte requiere planificación cuidadosa de rutas y horarios.",
		"",
		"## Facturación",
		"",
		"El proceso de facturación genera documentos legales.",
	}, "\n"))

	// Initialize workspace
	alias := "wiki-semantic-" + filepath.Base(root)
	app := New(root, nil)

	initEnv, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "workspace.init",
		Context:   model.QueryOptions{},
		Payload:   map[string]any{"path": root, "alias": alias, "no_index": true},
	})
	if err != nil {
		t.Fatalf("workspace.init: %v", err)
	}
	if !initEnv.Ok {
		t.Fatalf("workspace.init not ok")
	}
	defer func() { _ = workspace.RemoveWorkspace(alias) }()

	// Update project with embeddings config
	proj, err := workspace.LoadProjectFile(root)
	if err != nil {
		t.Fatalf("LoadProjectFile: %v", err)
	}
	proj.Embeddings = &model.EmbeddingsBlock{
		Enabled:   boolPtr(true),
		Provider:  "openai",
		BaseURL:   server.URL,
		Model:     "fake",
		Dim:       8,
		BatchSize: 4,
		TimeoutMS: 5000,
	}
	if err := workspace.SaveProjectFile(root, proj); err != nil {
		t.Fatalf("SaveProjectFile: %v", err)
	}

	// Index docs
	indexEnv, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "index.start",
		Context:   model.QueryOptions{Workspace: alias},
		Payload:   map[string]any{"docs_only": true, "wait": true},
	})
	if err != nil {
		t.Fatalf("index.start: %v", err)
	}
	if !indexEnv.Ok {
		t.Fatalf("index.start not ok")
	}

	db, err := store.Open(root)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	embs, err := store.AllWikiChunkEmbeddings(context.Background(), db)
	db.Close()
	if err != nil {
		t.Fatalf("AllWikiChunkEmbeddings: %v", err)
	}
	if len(embs) == 0 {
		t.Fatalf("index.start populated 0 embeddings; expected chunks from the indexed wiki notes (Windows path-separator regression?)")
	}

	// Now call recall with Spanish query
	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.recall",
		Context:   model.QueryOptions{Workspace: alias, MaxItems: 10},
		Payload:   map[string]any{"query": "acidificacion biologica"},
	})
	if err != nil {
		t.Fatalf("recall: %v", err)
	}

	if !env.Ok {
		t.Fatalf("recall not ok")
	}

	results, ok := env.Items.([]model.RecallResult)
	if !ok {
		t.Fatalf("expected []RecallResult, got %T", env.Items)
	}

	if len(results) == 0 {
		t.Fatalf("expected non-empty results for ES query")
	}

	// First result should be acidification note
	topResult := results[0]
	if !strings.HasSuffix(topResult.Archivo, "acidification.md") {
		t.Fatalf("first result should be acidification.md, got %q", topResult.Archivo)
	}

	if !strings.Contains(strings.ToLower(topResult.Heading), "acidif") {
		t.Fatalf("first result heading should contain 'acidif', got %q", topResult.Heading)
	}

	t.Logf("PASS: ES query ranked EN acidification note first (score=%.3f)", topResult.Score)
}

func TestRecall_EmbeddingsImplicitlyActiveWithoutEnabled(t *testing.T) {
	server := newFakeEmbeddings(t)
	defer server.Close()

	ensureWritableTestHome(t)
	root := t.TempDir()

	writeWorkspaceFile(t, root, "go.mod", "module wiki-implicit-embeddings-test\n\ngo 1.24\n")
	writeWorkspaceFile(t, root, ".docs/wiki/intro.md", strings.Join([]string{
		"# Semantic Notes",
		"",
		"## Recall",
		"",
		"Semantic recall should embed wiki chunks when the embeddings block has a base URL and model.",
	}, "\n"))

	alias := "wiki-implicit-embeddings-" + filepath.Base(root)
	app := New(root, nil)

	initEnv, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "workspace.init",
		Context:   model.QueryOptions{},
		Payload:   map[string]any{"path": root, "alias": alias, "no_index": true},
	})
	if err != nil {
		t.Fatalf("workspace.init: %v", err)
	}
	if !initEnv.Ok {
		t.Fatalf("workspace.init not ok")
	}
	defer func() { _ = workspace.RemoveWorkspace(alias) }()

	proj, err := workspace.LoadProjectFile(root)
	if err != nil {
		t.Fatalf("LoadProjectFile: %v", err)
	}
	proj.Embeddings = &model.EmbeddingsBlock{
		Provider:  "openai",
		BaseURL:   server.URL,
		Model:     "fake",
		Dim:       8,
		BatchSize: 4,
		TimeoutMS: 5000,
	}
	if err := workspace.SaveProjectFile(root, proj); err != nil {
		t.Fatalf("SaveProjectFile: %v", err)
	}

	indexEnv, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "index.start",
		Context:   model.QueryOptions{Workspace: alias},
		Payload:   map[string]any{"docs_only": true, "wait": true},
	})
	if err != nil {
		t.Fatalf("index.start: %v", err)
	}
	if !indexEnv.Ok {
		t.Fatalf("index.start not ok")
	}

	db, err := store.Open(root)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	embs, err := store.AllWikiChunkEmbeddings(context.Background(), db)
	db.Close()
	if err != nil {
		t.Fatalf("AllWikiChunkEmbeddings: %v", err)
	}
	if len(embs) == 0 {
		t.Fatalf("embeddings block with base_url and model but omitted enabled populated 0 embeddings")
	}
}

func TestRecallIntentRerankingSeparatesFormulaAndRoute(t *testing.T) {
	server := newFakeEmbeddings(t)
	defer server.Close()

	ensureWritableTestHome(t)
	root := t.TempDir()
	writeWorkspaceFile(t, root, "go.mod", "module wiki-recall-intent-test\n\ngo 1.24\n")
	writeWorkspaceFile(t, root, ".docs/wiki/hops-ibu-formula-contract.md", strings.Join([]string{
		"---",
		"documentKey: hops-ibu-tinseth-standard-formula-contract",
		"body_role: source-grounded",
		"tags: [formula, calculation, evidence, hops]",
		"---",
		"# Hops IBU Formula Contract",
		"",
		"## Validated formula",
		"",
		"This note contains a source-grounded formula contract, units, fixtures and stop conditions for IBU calculation.",
	}, "\n"))
	writeWorkspaceFile(t, root, ".docs/wiki/workers/hopping-worker.md", strings.Join([]string{
		"---",
		"documentKey: hopping-recipe-design-worker",
		"body_role: worker-profile",
		"tags: [worker, hops, route]",
		"---",
		"# Hopping Worker",
		"",
		"## Mission",
		"",
		"This worker profile routes hop questions. It is route-only and not final formula evidence.",
	}, "\n"))

	alias := "wiki-recall-intent-" + filepath.Base(root)
	app := New(root, nil)
	initEnv, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "workspace.init",
		Context:   model.QueryOptions{},
		Payload:   map[string]any{"path": root, "alias": alias, "no_index": true},
	})
	if err != nil {
		t.Fatalf("workspace.init: %v", err)
	}
	if !initEnv.Ok {
		t.Fatalf("workspace.init not ok")
	}
	defer func() { _ = workspace.RemoveWorkspace(alias) }()

	proj, err := workspace.LoadProjectFile(root)
	if err != nil {
		t.Fatalf("LoadProjectFile: %v", err)
	}
	proj.Embeddings = &model.EmbeddingsBlock{
		Provider:       "openai",
		BaseURL:        server.URL,
		Model:          "fake",
		Dim:            8,
		BatchSize:      4,
		TimeoutMS:      5000,
		EncodingFormat: "float",
	}
	if err := workspace.SaveProjectFile(root, proj); err != nil {
		t.Fatalf("SaveProjectFile: %v", err)
	}

	indexEnv, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "index.start",
		Context:   model.QueryOptions{Workspace: alias},
		Payload:   map[string]any{"docs_only": true, "wait": true},
	})
	if err != nil {
		t.Fatalf("index.start: %v", err)
	}
	if !indexEnv.Ok {
		t.Fatalf("index.start not ok")
	}

	formulaEnv, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.recall",
		Context:   model.QueryOptions{Workspace: alias, MaxItems: 5},
		Payload:   map[string]any{"query": "IBU formula hops", "intent": "formula"},
	})
	if err != nil {
		t.Fatalf("formula recall: %v", err)
	}
	formulaResults := formulaEnv.Items.([]model.RecallResult)
	if len(formulaResults) == 0 || !strings.Contains(formulaResults[0].Archivo, "formula-contract") {
		t.Fatalf("formula intent top result = %#v, want formula contract", formulaResults)
	}
	if !containsRecallWhy(formulaResults[0].Why, "intent_boost") {
		t.Fatalf("formula top result why = %#v, want intent_boost", formulaResults[0].Why)
	}

	routeEnv, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.recall",
		Context:   model.QueryOptions{Workspace: alias, MaxItems: 5},
		Payload:   map[string]any{"query": "IBU formula hops", "intent": "route"},
	})
	if err != nil {
		t.Fatalf("route recall: %v", err)
	}
	routeResults := routeEnv.Items.([]model.RecallResult)
	if len(routeResults) == 0 || !strings.Contains(routeResults[0].Archivo, "workers/hopping-worker") {
		t.Fatalf("route intent top result = %#v, want worker profile", routeResults)
	}
}

func TestRecall_RerankExtensionReordersCandidates(t *testing.T) {
	server := newFakeEmbeddings(t)
	defer server.Close()

	command, args := serviceRerankHelperCommand(t, "reverse")
	ensureWritableTestHome(t)
	root := t.TempDir()
	writeWorkspaceFile(t, root, "go.mod", "module wiki-rerank-extension-test\n\ngo 1.24\n")
	writeWorkspaceFile(t, root, ".docs/wiki/acidification.md", strings.Join([]string{
		"# Acidification",
		"",
		"Acidification and biological fermentation lower pH through microbial organic acids.",
	}, "\n"))
	writeWorkspaceFile(t, root, ".docs/wiki/logistics.md", strings.Join([]string{
		"# Logistics",
		"",
		"Logistica de transporte y facturacion para rutas comerciales.",
	}, "\n"))

	alias := "wiki-rerank-extension-" + filepath.Base(root)
	app := New(root, nil)
	initEnv, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "workspace.init",
		Context:   model.QueryOptions{},
		Payload:   map[string]any{"path": root, "alias": alias, "no_index": true},
	})
	if err != nil {
		t.Fatalf("workspace.init: %v", err)
	}
	if !initEnv.Ok {
		t.Fatalf("workspace.init not ok")
	}
	defer func() { _ = workspace.RemoveWorkspace(alias) }()

	proj, err := workspace.LoadProjectFile(root)
	if err != nil {
		t.Fatalf("LoadProjectFile: %v", err)
	}
	proj.Embeddings = &model.EmbeddingsBlock{
		Enabled:   boolPtr(true),
		Provider:  "openai",
		BaseURL:   server.URL,
		Model:     "fake",
		Dim:       8,
		BatchSize: 4,
		TimeoutMS: 5000,
	}
	proj.Recall = &model.RecallBlock{RerankExtension: &model.RerankExtensionBlock{
		Enabled:         boolPtr(true),
		Command:         command,
		Args:            args,
		TimeoutMS:       5000,
		CandidateCount:  2,
		TopN:            1,
		MaxSnippetChars: 80,
	}}
	if err := workspace.SaveProjectFile(root, proj); err != nil {
		t.Fatalf("SaveProjectFile: %v", err)
	}

	indexEnv, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "index.start",
		Context:   model.QueryOptions{Workspace: alias},
		Payload:   map[string]any{"docs_only": true, "wait": true},
	})
	if err != nil {
		t.Fatalf("index.start: %v", err)
	}
	if !indexEnv.Ok {
		t.Fatalf("index.start not ok")
	}

	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.recall",
		Context:   model.QueryOptions{Workspace: alias, MaxItems: 1},
		Payload:   map[string]any{"query": "acidificacion biologica"},
	})
	if err != nil {
		t.Fatalf("recall: %v", err)
	}
	results := env.Items.([]model.RecallResult)
	if len(results) != 1 {
		t.Fatalf("results len = %d, want 1", len(results))
	}
	if !strings.HasSuffix(results[0].Archivo, "logistics.md") {
		t.Fatalf("rerank top result = %#v, want logistics.md", results[0])
	}
	if !containsRecallWhy(results[0].Why, "external_rerank") {
		t.Fatalf("why = %#v, want external_rerank", results[0].Why)
	}
}

func TestRecall_RerankExtensionFailurePreservesOrderAndSanitizesWarning(t *testing.T) {
	server := newFakeEmbeddings(t)
	defer server.Close()

	command, args := serviceRerankHelperCommand(t, "invalid-json")
	ensureWritableTestHome(t)
	root := t.TempDir()
	writeWorkspaceFile(t, root, "go.mod", "module wiki-rerank-fallback-test\n\ngo 1.24\n")
	writeWorkspaceFile(t, root, ".docs/wiki/acidification.md", "# Acidification\n\nAcidification SECRET_SNIPPET_TOKEN biological fermentation.\n")
	writeWorkspaceFile(t, root, ".docs/wiki/logistics.md", "# Logistics\n\nLogistica de transporte y facturacion.\n")

	alias := "wiki-rerank-fallback-" + filepath.Base(root)
	app := New(root, nil)
	initEnv, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "workspace.init",
		Context:   model.QueryOptions{},
		Payload:   map[string]any{"path": root, "alias": alias, "no_index": true},
	})
	if err != nil {
		t.Fatalf("workspace.init: %v", err)
	}
	if !initEnv.Ok {
		t.Fatalf("workspace.init not ok")
	}
	defer func() { _ = workspace.RemoveWorkspace(alias) }()

	proj, err := workspace.LoadProjectFile(root)
	if err != nil {
		t.Fatalf("LoadProjectFile: %v", err)
	}
	proj.Embeddings = &model.EmbeddingsBlock{
		Enabled:   boolPtr(true),
		Provider:  "openai",
		BaseURL:   server.URL,
		Model:     "fake",
		Dim:       8,
		BatchSize: 4,
		TimeoutMS: 5000,
	}
	proj.Recall = &model.RecallBlock{RerankExtension: &model.RerankExtensionBlock{
		Enabled:        boolPtr(true),
		Command:        command,
		Args:           args,
		TimeoutMS:      5000,
		CandidateCount: 2,
	}}
	if err := workspace.SaveProjectFile(root, proj); err != nil {
		t.Fatalf("SaveProjectFile: %v", err)
	}

	indexEnv, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "index.start",
		Context:   model.QueryOptions{Workspace: alias},
		Payload:   map[string]any{"docs_only": true, "wait": true},
	})
	if err != nil {
		t.Fatalf("index.start: %v", err)
	}
	if !indexEnv.Ok {
		t.Fatalf("index.start not ok")
	}

	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.recall",
		Context:   model.QueryOptions{Workspace: alias, MaxItems: 1},
		Payload:   map[string]any{"query": "acidificacion biologica SECRET_QUERY_TOKEN"},
	})
	if err != nil {
		t.Fatalf("recall: %v", err)
	}
	results := env.Items.([]model.RecallResult)
	if len(results) != 1 || !strings.HasSuffix(results[0].Archivo, "acidification.md") {
		t.Fatalf("failure should preserve semantic order, got %#v", results)
	}
	warningText := strings.Join(env.Warnings, " ")
	if !strings.Contains(warningText, "rerank extension invalid_json") {
		t.Fatalf("warnings = %#v, want sanitized rerank failure", env.Warnings)
	}
	for _, forbidden := range []string{"SECRET_QUERY_TOKEN", "SECRET_SNIPPET_TOKEN", "not-json"} {
		if strings.Contains(warningText, forbidden) {
			t.Fatalf("warning leaked %q: %q", forbidden, warningText)
		}
	}
}

func TestRecall_EmbeddingFallbackWarningIsSanitized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		http.Error(w, "provider echoed "+string(body)+" SECRET_PROVIDER_TOKEN", http.StatusInternalServerError)
	}))
	defer server.Close()

	ensureWritableTestHome(t)
	root := t.TempDir()
	writeWorkspaceFile(t, root, "go.mod", "module wiki-embedding-fallback-sanitize-test\n\ngo 1.24\n")
	writeWorkspaceFile(t, root, ".docs/wiki/secret.md", "# Secret\n\nSECRET_QUERY_TOKEN lexical fallback target.\n")

	alias := "wiki-embedding-fallback-sanitize-" + filepath.Base(root)
	app := New(root, nil)
	initEnv, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "workspace.init",
		Context:   model.QueryOptions{},
		Payload:   map[string]any{"path": root, "alias": alias, "no_index": true},
	})
	if err != nil {
		t.Fatalf("workspace.init: %v", err)
	}
	if !initEnv.Ok {
		t.Fatalf("workspace.init not ok")
	}
	defer func() { _ = workspace.RemoveWorkspace(alias) }()

	proj, err := workspace.LoadProjectFile(root)
	if err != nil {
		t.Fatalf("LoadProjectFile: %v", err)
	}
	proj.Embeddings = &model.EmbeddingsBlock{
		Enabled:   boolPtr(true),
		Provider:  "openai",
		BaseURL:   server.URL,
		Model:     "fake",
		Dim:       8,
		TimeoutMS: 5000,
	}
	if err := workspace.SaveProjectFile(root, proj); err != nil {
		t.Fatalf("SaveProjectFile: %v", err)
	}

	indexEnv, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "index.run",
		Context:   model.QueryOptions{Workspace: alias},
		Payload:   map[string]any{"docs_only": true},
	})
	if err != nil {
		t.Fatalf("index.run: %v", err)
	}
	if !indexEnv.Ok {
		t.Fatalf("index.run not ok")
	}

	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.recall",
		Context:   model.QueryOptions{Workspace: alias, MaxItems: 5},
		Payload:   map[string]any{"query": "SECRET_QUERY_TOKEN"},
	})
	if err != nil {
		t.Fatalf("recall: %v", err)
	}
	warningText := strings.Join(env.Warnings, " ")
	if !strings.Contains(warningText, "embeddings unavailable; served lexical results") {
		t.Fatalf("warnings = %#v, want sanitized embedding fallback warning", env.Warnings)
	}
	for _, forbidden := range []string{"SECRET_QUERY_TOKEN", "SECRET_PROVIDER_TOKEN", "provider echoed", "embeddings endpoint returned"} {
		if strings.Contains(warningText, forbidden) {
			t.Fatalf("warning leaked %q: %q", forbidden, warningText)
		}
	}
}

func containsRecallWhy(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func TestRecall_EmbeddingsExplicitFalseDisablesConfiguredBlock(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		http.Error(w, "embeddings provider should not be called", http.StatusInternalServerError)
	}))
	defer server.Close()

	ensureWritableTestHome(t)
	root := t.TempDir()

	writeWorkspaceFile(t, root, "go.mod", "module wiki-explicit-disabled-embeddings-test\n\ngo 1.24\n")
	writeWorkspaceFile(t, root, ".docs/wiki/intro.md", "# Disabled Embeddings\n\nThis note should stay lexical only.\n")

	alias := "wiki-explicit-disabled-embeddings-" + filepath.Base(root)
	app := New(root, nil)

	initEnv, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "workspace.init",
		Context:   model.QueryOptions{},
		Payload:   map[string]any{"path": root, "alias": alias, "no_index": true},
	})
	if err != nil {
		t.Fatalf("workspace.init: %v", err)
	}
	if !initEnv.Ok {
		t.Fatalf("workspace.init not ok")
	}
	defer func() { _ = workspace.RemoveWorkspace(alias) }()

	proj, err := workspace.LoadProjectFile(root)
	if err != nil {
		t.Fatalf("LoadProjectFile: %v", err)
	}
	proj.Embeddings = &model.EmbeddingsBlock{
		Enabled: boolPtr(false),
		BaseURL: server.URL,
		Model:   "fake",
		Dim:     8,
	}
	if err := workspace.SaveProjectFile(root, proj); err != nil {
		t.Fatalf("SaveProjectFile: %v", err)
	}

	indexEnv, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "index.run",
		Context:   model.QueryOptions{Workspace: alias},
		Payload:   map[string]any{"docs_only": true},
	})
	if err != nil {
		t.Fatalf("index.run: %v", err)
	}
	if !indexEnv.Ok {
		t.Fatalf("index.run not ok")
	}
	if got := calls.Load(); got != 0 {
		t.Fatalf("explicit enabled=false should not call embeddings provider, got %d calls", got)
	}

	db, err := store.Open(root)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	embs, err := store.AllWikiChunkEmbeddings(context.Background(), db)
	db.Close()
	if err != nil {
		t.Fatalf("AllWikiChunkEmbeddings: %v", err)
	}
	if len(embs) != 0 {
		t.Fatalf("explicit enabled=false populated %d embeddings; expected 0", len(embs))
	}
}

// TestRecall_KnowledgeWikiNoGovernance tests that knowledge-wiki without 00_gobierno_documental.md works
func TestRecall_KnowledgeWikiNoGovernance(t *testing.T) {
	server := newFakeEmbeddings(t)
	defer server.Close()

	ensureWritableTestHome(t)
	root := t.TempDir()

	writeWorkspaceFile(t, root, "go.mod", "module knowledge-wiki-test\n\ngo 1.24\n")
	writeWorkspaceFile(t, root, ".docs/wiki/intro.md", "# Introduction\n\n## Overview\n\nTest content.\n")

	writeWorkspaceFile(t, root, ".mi-lsp/project.toml", strings.Join([]string{
		"[project]",
		"name = \"knowledge-wiki-test\"",
		"languages = [\"markdown\"]",
		"kind = \"single\"",
		"",
		"[repo.main]",
		"id = \"main\"",
		"name = \"main\"",
		"root = \".\"",
		"",
		"[embeddings]",
		"enabled = true",
		"provider = \"openai\"",
		fmt.Sprintf("base_url = \"%s\"", server.URL),
		"model = \"fake\"",
		"dim = 8",
	}, "\n"))

	alias := "knowledge-wiki-" + filepath.Base(root)
	app := New(root, nil)

	initEnv, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "workspace.init",
		Context:   model.QueryOptions{},
		Payload:   map[string]any{"path": root, "alias": alias},
	})
	if err != nil {
		t.Fatalf("workspace.init: %v", err)
	}
	if !initEnv.Ok {
		t.Fatalf("workspace.init not ok")
	}
	defer func() { _ = workspace.RemoveWorkspace(alias) }()

	// Call recall (should succeed without governance block)
	recallEnv, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.recall",
		Context:   model.QueryOptions{Workspace: alias, MaxItems: 5},
		Payload:   map[string]any{"query": "introduction"},
	})
	if err != nil {
		t.Fatalf("recall: %v", err)
	}

	if !recallEnv.Ok {
		t.Fatalf("recall not ok")
	}

	if !strings.Contains(recallEnv.Backend, "recall") {
		t.Fatalf("unexpected backend: %q", recallEnv.Backend)
	}

	t.Logf("PASS: knowledge-wiki indexed and recalled without governance block")
}

// TestRecall_ConfigGatedWhenDisabled tests that recall returns hint when embeddings disabled
func TestRecall_ConfigGatedWhenDisabled(t *testing.T) {
	ensureWritableTestHome(t)
	root := t.TempDir()

	writeWorkspaceFile(t, root, "go.mod", "module no-embeddings-test\n\ngo 1.24\n")
	writeWorkspaceFile(t, root, ".docs/wiki/test.md", "# Test\n\nContent.\n")

	writeWorkspaceFile(t, root, ".mi-lsp/project.toml", strings.Join([]string{
		"[project]",
		"name = \"no-embeddings-test\"",
		"languages = [\"markdown\"]",
		"kind = \"single\"",
		"",
		"[repo.main]",
		"id = \"main\"",
		"name = \"main\"",
		"root = \".\"",
	}, "\n"))

	alias := "no-embeddings-" + filepath.Base(root)
	app := New(root, nil)

	initEnv, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "workspace.init",
		Context:   model.QueryOptions{},
		Payload:   map[string]any{"path": root, "alias": alias},
	})
	if err != nil {
		t.Fatalf("workspace.init: %v", err)
	}
	if !initEnv.Ok {
		t.Fatalf("workspace.init not ok")
	}
	defer func() { _ = workspace.RemoveWorkspace(alias) }()

	// Call recall with embeddings disabled
	recallEnv, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.recall",
		Context:   model.QueryOptions{Workspace: alias},
		Payload:   map[string]any{"query": "test"},
	})
	if err != nil {
		t.Fatalf("recall: %v", err)
	}

	if !recallEnv.Ok {
		t.Fatalf("recall not ok; expected ok=true when embeddings disabled")
	}

	results, ok := recallEnv.Items.([]model.RecallResult)
	if !ok {
		t.Fatalf("expected []RecallResult, got %T", recallEnv.Items)
	}

	if len(results) > 0 {
		t.Fatalf("expected empty items when embeddings disabled, got %d", len(results))
	}

	if recallEnv.Hint == "" {
		t.Fatalf("expected non-empty hint when embeddings disabled")
	}

	if !strings.Contains(recallEnv.Hint, "embeddings") {
		t.Fatalf("hint should mention embeddings, got: %q", recallEnv.Hint)
	}

	t.Logf("PASS: recall with embeddings disabled returned ok=true, empty items, and hint")
}

func serviceRerankHelperCommand(t *testing.T, mode string) (string, []string) {
	t.Helper()
	t.Setenv("MI_LSP_SERVICE_RERANK_HELPER", "1")
	return os.Args[0], []string{"-test.run=TestServiceRerankHelperProcess", "--", mode}
}

func TestServiceRerankHelperProcess(t *testing.T) {
	if os.Getenv("MI_LSP_SERVICE_RERANK_HELPER") != "1" {
		return
	}
	mode := ""
	for i, arg := range os.Args {
		if arg == "--" && i+1 < len(os.Args) {
			mode = os.Args[i+1]
			break
		}
	}
	switch mode {
	case "reverse":
		body, _ := io.ReadAll(os.Stdin)
		var req struct {
			Candidates []any `json:"candidates"`
		}
		_ = json.Unmarshal(body, &req)
		if len(req.Candidates) >= 2 {
			fmt.Println(`{"protocol_version":"mi-lsp-rerank-extension-v1","indices":[1,0]}`)
		} else {
			fmt.Println(`{"protocol_version":"mi-lsp-rerank-extension-v1","indices":[0]}`)
		}
	case "invalid-json":
		fmt.Println("not-json SECRET_SNIPPET_TOKEN")
	default:
		fmt.Println(`{"protocol_version":"mi-lsp-rerank-extension-v1","indices":[0]}`)
	}
	os.Exit(0)
}
