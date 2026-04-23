package service

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

func TestNavTraceFindsRFEmbeddedInAggregateDoc(t *testing.T) {
	alias := "trace-embedded-rf-" + filepath.Base(t.TempDir())
	root := createFunctionalPackWorkspaceFixture(t, alias)
	writeWorkspaceFile(t, root, ".docs/wiki/04_RF/RF-IDN.md", strings.Join([]string{
		"# RF-IDN",
		"",
		"| ID | Titulo |",
		"| --- | --- |",
		"| RF-IDN-07 | OAuth device authorization sanitizado |",
		"| RF-IDN-08 | Reindexado documental observable |",
	}, "\n"))
	if _, err := workspace.RegisterWorkspace(alias, model.WorkspaceRegistration{
		Name:      alias,
		Root:      root,
		Languages: []string{"csharp"},
		Kind:      model.WorkspaceKindSingle,
	}); err != nil {
		t.Fatalf("register workspace: %v", err)
	}
	defer func() { _ = workspace.RemoveWorkspace(alias) }()

	db, err := store.Open(root)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	if err := store.ReplaceDocs(context.Background(), db, []model.DocRecord{{
		Path:       ".docs/wiki/04_RF/RF-IDN.md",
		Title:      "RF-IDN",
		DocID:      "RF-IDN",
		Layer:      "04",
		Family:     "functional",
		SearchText: "rf idn rf idn 07 rf idn 08 reindexado documental observable",
		IndexedAt:  1,
	}}, nil, []model.DocMention{{
		DocPath:      ".docs/wiki/04_RF/RF-IDN.md",
		MentionType:  "doc_id",
		MentionValue: "RF-IDN-08",
	}}); err != nil {
		t.Fatalf("ReplaceDocs: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("db.Close: %v", err)
	}

	app := New(root, nil)
	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.trace",
		Context:   model.QueryOptions{Workspace: alias},
		Payload:   map[string]any{"rf": "RF-IDN-08"},
	})
	if err != nil {
		t.Fatalf("nav.trace: %v", err)
	}
	results, ok := env.Items.([]model.TraceResult)
	if !ok || len(results) != 1 {
		t.Fatalf("expected one trace result, got %#v", env.Items)
	}
	if got := results[0].RF; got != "RF-IDN-08" {
		t.Fatalf("trace RF = %q, want RF-IDN-08", got)
	}
	if !strings.Contains(results[0].Title, "Reindexado documental") {
		t.Fatalf("trace title = %q, want embedded RF title", results[0].Title)
	}
}

func TestNavTracePrefersAggregateRFDocOverRFIndexDoc(t *testing.T) {
	alias := "trace-embedded-rf-specific-" + filepath.Base(t.TempDir())
	root := createFunctionalPackWorkspaceFixture(t, alias)
	writeWorkspaceFile(t, root, ".docs/wiki/04_RF.md", "# 04 Requerimientos Funcionales (RF)\n\nRF-IDN-08 listado general.\n")
	writeWorkspaceFile(t, root, ".docs/wiki/04_RF/RF-IDN.md", strings.Join([]string{
		"# RF-IDN",
		"",
		"| ID | Titulo |",
		"| --- | --- |",
		"| RF-IDN-08 | Reindexado documental observable |",
	}, "\n"))
	if _, err := workspace.RegisterWorkspace(alias, model.WorkspaceRegistration{
		Name:      alias,
		Root:      root,
		Languages: []string{"csharp"},
		Kind:      model.WorkspaceKindSingle,
	}); err != nil {
		t.Fatalf("register workspace: %v", err)
	}
	defer func() { _ = workspace.RemoveWorkspace(alias) }()

	db, err := store.Open(root)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	if err := store.ReplaceDocs(context.Background(), db, []model.DocRecord{
		{
			Path:       ".docs/wiki/04_RF.md",
			Title:      "04 Requerimientos Funcionales (RF)",
			DocID:      "RF-IDN-08",
			Layer:      "04",
			Family:     "functional",
			SearchText: "rf idn 08 requerimientos funcionales",
			IndexedAt:  1,
		},
		{
			Path:       ".docs/wiki/04_RF/RF-IDN.md",
			Title:      "RF-IDN",
			DocID:      "RF-IDN",
			Layer:      "04",
			Family:     "functional",
			SearchText: "rf idn 08 reindexado documental observable",
			IndexedAt:  1,
		},
	}, nil, []model.DocMention{{
		DocPath:      ".docs/wiki/04_RF/RF-IDN.md",
		MentionType:  "doc_id",
		MentionValue: "RF-IDN-08",
	}}); err != nil {
		t.Fatalf("ReplaceDocs: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("db.Close: %v", err)
	}

	app := New(root, nil)
	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.trace",
		Context:   model.QueryOptions{Workspace: alias},
		Payload:   map[string]any{"rf": "RF-IDN-08"},
	})
	if err != nil {
		t.Fatalf("nav.trace: %v", err)
	}
	results := env.Items.([]model.TraceResult)
	if len(results) != 1 {
		t.Fatalf("expected one trace result, got %#v", results)
	}
	if results[0].Title != "Reindexado documental observable" {
		t.Fatalf("trace title = %q, want aggregate RF row title", results[0].Title)
	}
}
