package service

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/indexer"
	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

func TestGraphCLIContractIntegrationIsDeterministicAndReadOnly(t *testing.T) {
	root := t.TempDir()
	alias := "graph-contract-" + filepath.Base(root)
	if err := workspace.SaveProjectFile(root, model.ProjectFile{Project: model.ProjectBlock{Name: alias, Kind: model.WorkspaceKindSingle, DefaultRepo: "repo", Languages: []string{"go"}}, Repos: []model.WorkspaceRepo{{ID: "repo", Name: "repo", Root: ".", RepositoryIdentity: "https://example.com/" + alias, Languages: []string{"go"}}}}); err != nil {
		t.Fatal(err)
	}
	writeGraphContractFile(t, root, "go.mod", "module example.com/graphcontract\n\ngo 1.23\n")
	writeGraphContractFile(t, root, "subject.go", "package graphcontract\n\nfunc Subject() string { return \"ok\" }\n\nfunc Caller() string { return Subject() }\n\nfunc CallerTwo() string { return Subject() }\n\nfunc CallerThree() string { return Subject() }\n")
	if _, err := indexer.IndexWorkspaceWithGraphProgress(context.Background(), root, true, "", nil, indexer.GraphIndexOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := workspace.RegisterWorkspace(alias, model.WorkspaceRegistration{Name: alias, Root: root, Languages: []string{"go"}, Kind: model.WorkspaceKindSingle}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = workspace.RemoveWorkspace(alias) })
	db, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	before := graphContractDatabaseState(t, db)
	explainRID := graphContractEdgeRID(t, db, before.generationID)
	app := New(root, nil)
	requests := []struct {
		op         string
		payload    map[string]any
		needsProof bool
	}{
		{"nav.graph.stats", map[string]any{}, false},
		{"nav.graph.validate", map[string]any{}, false},
		{"nav.neighbors", map[string]any{"selector": "Subject", "depth": 2, "limit": 10, "direction": "both"}, true},
		{"nav.callers", map[string]any{"selector": "Subject"}, true},
		{"nav.callees", map[string]any{"selector": "Caller"}, true},
		{"nav.path", map[string]any{"from": "Caller", "to": "Subject"}, true},
		{"nav.explain", map[string]any{"selector": explainRID}, true},
	}
	for _, tc := range requests {
		request := model.CommandRequest{Operation: tc.op, Context: model.QueryOptions{Workspace: alias}, Payload: tc.payload}
		first, err := app.Execute(context.Background(), request)
		if err != nil {
			t.Fatalf("%s: %v", tc.op, err)
		}
		assertGraphContractEnvelope(t, tc.op, first, before.generationID)
		if tc.needsProof {
			assertGraphContractProof(t, tc.op, first.Items)
		}
		firstGraph, _ := json.Marshal(first.Graph)
		firstItems, _ := json.Marshal(first.Items)
		firstOmissions, _ := json.Marshal(first.Omissions)
		for i := 0; i < 10; i++ {
			next, err := app.Execute(context.Background(), request)
			if err != nil {
				t.Fatalf("%s rerun: %v", tc.op, err)
			}
			nextGraph, _ := json.Marshal(next.Graph)
			nextItems, _ := json.Marshal(next.Items)
			nextOmissions, _ := json.Marshal(next.Omissions)
			if !bytes.Equal(firstGraph, nextGraph) || !bytes.Equal(firstItems, nextItems) || !bytes.Equal(firstOmissions, nextOmissions) {
				t.Fatalf("%s is not deterministic", tc.op)
			}
		}
	}

	tiny := model.CommandRequest{Operation: "nav.neighbors", Context: model.QueryOptions{Workspace: alias}, Payload: map[string]any{"selector": "Subject", "depth": 2, "limit": 10, "token_budget": 1, "direction": "both"}}
	firstTiny, err := app.Execute(context.Background(), tiny)
	if err != nil {
		t.Fatal(err)
	}
	if !firstTiny.Truncated || firstTiny.Graph == nil || firstTiny.Graph.NextCursor == "" || firstTiny.Continuation == nil || firstTiny.Continuation.Next.Query != firstTiny.Graph.NextCursor {
		t.Fatalf("tiny budget did not expose a stable bounded continuation: %#v", firstTiny)
	}
	secondTiny, err := app.Execute(context.Background(), tiny)
	if err != nil {
		t.Fatal(err)
	}
	if secondTiny.Graph.NextCursor != firstTiny.Graph.NextCursor || secondTiny.Continuation.Next.Query != firstTiny.Continuation.Next.Query {
		t.Fatalf("tiny budget continuation is not stable: first=%#v second=%#v", firstTiny.Graph, secondTiny.Graph)
	}
	if after := graphContractDatabaseState(t, db); after != before {
		t.Fatalf("graph queries changed database: before=%+v after=%+v", before, after)
	}
}

type graphContractState struct {
	generationID string
	status       string
	generations  int
	nodes        int
	edges        int
	evidence     int
	unresolved   int
	changes      int64
}

func graphContractDatabaseState(t *testing.T, db *sql.DB) graphContractState {
	t.Helper()
	var state graphContractState
	if err := db.QueryRow(`SELECT lower(hex(generation_id)), status FROM graph_generations WHERE status = ? ORDER BY generation_id LIMIT 1`, model.GraphGenerationActive).Scan(&state.generationID, &state.status); err != nil {
		t.Fatal(err)
	}
	for _, check := range []struct {
		table string
		out   *int
	}{
		{"graph_generations", &state.generations}, {"graph_nodes", &state.nodes}, {"graph_edges", &state.edges}, {"graph_evidence", &state.evidence}, {"graph_unresolved", &state.unresolved},
	} {
		if err := db.QueryRow(`SELECT COUNT(*) FROM ` + check.table).Scan(check.out); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.QueryRow("SELECT total_changes()").Scan(&state.changes); err != nil {
		t.Fatal(err)
	}
	return state
}

func graphContractEdgeRID(t *testing.T, db *sql.DB, generationID string) string {
	t.Helper()
	var rid string
	if err := db.QueryRow(`SELECT cross_rid FROM graph_edges ORDER BY edge_key LIMIT 1`).Scan(&rid); err != nil || rid == "" {
		t.Fatalf("read-only edge selector: rid=%q err=%v", rid, err)
	}
	return rid
}

func assertGraphContractEnvelope(t *testing.T, operation string, env model.Envelope, generationID string) {
	t.Helper()
	if !env.Ok || env.Operation != operation || env.GenerationID != generationID || env.GraphSchemaVersion <= 0 || env.DeterminismDigest == "" || env.Backend != "sqlite-direct" || env.Mode != "query_only" || env.Graph == nil || env.Items == nil || env.Omissions == nil {
		t.Fatalf("%s incomplete graph envelope: %#v", operation, env)
	}
	if env.Graph.Operation != operation || env.Graph.GenerationID != generationID || env.Graph.Schema != env.GraphSchemaVersion || env.Graph.DeterminismDigest != env.DeterminismDigest || env.Graph.Stats != (model.GraphQueryStats{}) && env.Graph.Stats.Depth < 0 {
		t.Fatalf("%s invalid graph metadata: %#v", operation, env.Graph)
	}
}

func assertGraphContractProof(t *testing.T, operation string, rawItems any) {
	t.Helper()
	var items []model.GraphQueryItem
	switch typed := rawItems.(type) {
	case []model.GraphQueryItem:
		items = typed
	case []any:
		for _, raw := range typed {
			bytes, _ := json.Marshal(raw)
			var item model.GraphQueryItem
			if json.Unmarshal(bytes, &item) == nil {
				items = append(items, item)
			}
		}
	default:
		t.Fatalf("%s graph items have unexpected type %T", operation, rawItems)
	}
	for _, item := range items {
		if item.CrossRID != "" && len(item.EvidenceRefs) > 0 {
			return
		}
	}
	t.Fatalf("%s did not return an item with CrossRID and EvidenceRefs: %#v", operation, items)
}

func writeGraphContractFile(t *testing.T, root, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
