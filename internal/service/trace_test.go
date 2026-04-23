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

func TestNavTraceFallsBackToDiskWhenRFIsMissingFromDocIndex(t *testing.T) {
	alias := "trace-disk-fallback-" + filepath.Base(t.TempDir())
	root := createFunctionalPackWorkspaceFixture(t, alias)
	writeWorkspaceFile(t, root, ".docs/wiki/04_RF/RF-GAS.md", strings.Join([]string{
		"# RF-GAS",
		"",
		"| ID | Titulo |",
		"| --- | --- |",
		"| RF-GAS-09 | Projection de reminders de gastos |",
		"| RF-GAS-10 | Confirmacion requerida para handoff a gastos |",
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

	app := New(root, nil)
	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.trace",
		Context:   model.QueryOptions{Workspace: alias},
		Payload:   map[string]any{"rf": "RF-GAS-10"},
	})
	if err != nil {
		t.Fatalf("nav.trace: %v", err)
	}
	results, ok := env.Items.([]model.TraceResult)
	if !ok || len(results) != 1 {
		t.Fatalf("expected one trace result, got %#v", env.Items)
	}
	if results[0].RF != "RF-GAS-10" {
		t.Fatalf("trace RF = %q, want RF-GAS-10", results[0].RF)
	}
	if !strings.Contains(results[0].Title, "Confirmacion requerida") {
		t.Fatalf("trace title = %q, want disk fallback embedded title", results[0].Title)
	}
}

func TestNavTraceFallsBackToLegacyRFDirectoryWhenDocIndexIsMissing(t *testing.T) {
	alias := "trace-legacy-rf-fallback-" + filepath.Base(t.TempDir())
	root := createFunctionalPackWorkspaceFixture(t, alias)
	writeWorkspaceFile(t, root, ".docs/wiki/RF/RF-GAS.md", strings.Join([]string{
		"# RF-GAS",
		"",
		"| ID | Titulo |",
		"| --- | --- |",
		"| RF-GAS-09 | Projection de reminders de gastos |",
		"| RF-GAS-10 | Confirmacion requerida para handoff a gastos |",
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

	app := New(root, nil)
	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.trace",
		Context:   model.QueryOptions{Workspace: alias},
		Payload:   map[string]any{"rf": "RF-GAS-10"},
	})
	if err != nil {
		t.Fatalf("nav.trace: %v", err)
	}
	results, ok := env.Items.([]model.TraceResult)
	if !ok || len(results) != 1 {
		t.Fatalf("expected one trace result, got %#v", env.Items)
	}
	if results[0].RF != "RF-GAS-10" {
		t.Fatalf("trace RF = %q, want RF-GAS-10", results[0].RF)
	}
	if !strings.Contains(results[0].Title, "Confirmacion requerida") {
		t.Fatalf("trace title = %q, want legacy RF fallback embedded title", results[0].Title)
	}
}

func TestNavTraceFallsBackToLegacyRFRootIndexWhenDocIndexIsMissing(t *testing.T) {
	alias := "trace-legacy-rf-root-fallback-" + filepath.Base(t.TempDir())
	root := createFunctionalPackWorkspaceFixture(t, alias)
	writeWorkspaceFile(t, root, ".docs/wiki/RF.md", strings.Join([]string{
		"# RF",
		"",
		"| ID | Titulo |",
		"| --- | --- |",
		"| RF-GAS-09 | Projection de reminders de gastos |",
		"| RF-GAS-10 | Confirmacion requerida para handoff a gastos |",
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

	app := New(root, nil)
	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.trace",
		Context:   model.QueryOptions{Workspace: alias},
		Payload:   map[string]any{"rf": "RF-GAS-10"},
	})
	if err != nil {
		t.Fatalf("nav.trace: %v", err)
	}
	results, ok := env.Items.([]model.TraceResult)
	if !ok || len(results) != 1 {
		t.Fatalf("expected one trace result, got %#v", env.Items)
	}
	if results[0].RF != "RF-GAS-10" {
		t.Fatalf("trace RF = %q, want RF-GAS-10", results[0].RF)
	}
	if !strings.Contains(results[0].Title, "Confirmacion requerida") {
		t.Fatalf("trace title = %q, want legacy RF root fallback embedded title", results[0].Title)
	}
}

func TestNavTraceUsesTPDocsAsCoverageEvidenceForRFAndTPIDs(t *testing.T) {
	alias := "trace-tp-evidence-" + filepath.Base(t.TempDir())
	root := createFunctionalPackWorkspaceFixture(t, alias)
	writeWorkspaceFile(t, root, ".docs/wiki/04_RF/RF-GAS.md", strings.Join([]string{
		"# RF-GAS",
		"",
		"| ID | Titulo |",
		"| --- | --- |",
		"| RF-GAS-09 | Projection de reminders de gastos |",
		"| RF-GAS-10 | Confirmacion requerida para handoff a gastos |",
	}, "\n"))
	writeWorkspaceFile(t, root, ".docs/wiki/06_pruebas/TP-GAS.md", strings.Join([]string{
		"# TP-GAS",
		"",
		"| TP ID | RF | Tipo | Objetivo | Given | When | Then |",
		"| --- | --- | --- | --- | --- | --- | --- |",
		"| TP-GAS-17 | RF-GAS-09 | Positivo | Refrescar proyeccion por respuesta sync fresca | binding Active | CP recibe snapshotHint version nueva | syncStatus = Fresh |",
		"| TP-GAS-20 | RF-GAS-10 | Positivo | Renderizar dashboard web con binding activo | binding Active | persona abre /finanzas | ve dashboard poblado |",
		"| TP-GAS-23 | RF-GAS-06 | Positivo | Ejecutar query first-party sin binding | service registrado FirstPartySharedIdentity | runtime llama capability | request remoto lleva X-Teslita-Actor-Sub |",
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

	app := New(root, nil)
	if _, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "index.run",
		Context:   model.QueryOptions{Workspace: alias},
		Payload:   map[string]any{"docs_only": true},
	}); err != nil {
		t.Fatalf("index.run --docs-only: %v", err)
	}

	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.trace",
		Context:   model.QueryOptions{Workspace: alias},
		Payload:   map[string]any{"rf": "RF-GAS-10"},
	})
	if err != nil {
		t.Fatalf("nav.trace RF-GAS-10: %v", err)
	}
	results, ok := env.Items.([]model.TraceResult)
	if !ok || len(results) != 1 {
		t.Fatalf("expected one RF trace result, got %#v", env.Items)
	}
	if results[0].Status != "partial" {
		t.Fatalf("RF-GAS-10 status = %q, want partial", results[0].Status)
	}
	if len(results[0].Tests) == 0 || results[0].Tests[0].File != ".docs/wiki/06_pruebas/TP-GAS.md" {
		t.Fatalf("RF-GAS-10 tests = %#v, want TP-GAS doc evidence", results[0].Tests)
	}

	env, err = app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.trace",
		Context:   model.QueryOptions{Workspace: alias},
		Payload:   map[string]any{"rf": "TP-GAS-23"},
	})
	if err != nil {
		t.Fatalf("nav.trace TP-GAS-23: %v", err)
	}
	results, ok = env.Items.([]model.TraceResult)
	if !ok || len(results) != 1 {
		t.Fatalf("expected one TP trace result, got %#v", env.Items)
	}
	if got := results[0].RF; got != "TP-GAS-23" {
		t.Fatalf("TP trace id = %q, want TP-GAS-23", got)
	}
	if got := results[0].Title; got != "Ejecutar query first-party sin binding" {
		t.Fatalf("TP trace title = %q, want objective column", got)
	}
	if results[0].Status != "partial" {
		t.Fatalf("TP-GAS-23 status = %q, want partial", results[0].Status)
	}
	if len(results[0].Tests) == 0 || results[0].Tests[0].File != ".docs/wiki/06_pruebas/TP-GAS.md" {
		t.Fatalf("TP-GAS-23 tests = %#v, want TP-GAS doc evidence", results[0].Tests)
	}
}

func TestNavTraceFallsBackToGovernedRFPathOutsideDefaultWikiRoot(t *testing.T) {
	alias := "trace-governed-rf-root-" + filepath.Base(t.TempDir())
	root := createFunctionalPackWorkspaceFixture(t, alias)
	writeWorkspaceFile(t, root, "spec/wiki/04_RF/RF-CUSTOM.md", strings.Join([]string{
		"# RF-CUSTOM",
		"",
		"| ID | Titulo |",
		"| --- | --- |",
		"| RF-CUSTOM-01 | Resuelve fallback desde paths gobernados |",
	}, "\n"))
	writeWorkspaceFile(t, root, ".docs/wiki/_mi-lsp/read-model.toml", strings.Join([]string{
		"version = 1",
		"",
		"[[family]]",
		`name = "functional"`,
		`intent_keywords = ["rf", "test"]`,
		`paths = ["spec/wiki/04_RF/*.md"]`,
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

	app := New(root, nil)
	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.trace",
		Context:   model.QueryOptions{Workspace: alias},
		Payload:   map[string]any{"rf": "RF-CUSTOM-01"},
	})
	if err != nil {
		t.Fatalf("nav.trace: %v", err)
	}
	results, ok := env.Items.([]model.TraceResult)
	if !ok || len(results) != 1 {
		t.Fatalf("expected one trace result, got %#v", env.Items)
	}
	if got := results[0].RF; got != "RF-CUSTOM-01" {
		t.Fatalf("trace RF = %q, want RF-CUSTOM-01", got)
	}
	if got := results[0].Title; got != "Resuelve fallback desde paths gobernados" {
		t.Fatalf("trace title = %q, want governed path embedded title", got)
	}
}
