package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/workspace"
)

func TestKineticDetectsWikiWithoutProductManifest(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "wiki", "10-conceptos")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "modelo.md"), []byte("# m\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "acciones.md"), []byte("# a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok := kineticProject(root); !ok {
		t.Fatal("wiki pair without a product manifest should be kinetic")
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok := kineticProject(root); ok {
		t.Fatal("go.mod should disqualify a kinetic project")
	}
}

func TestKineticPromoverLinksActionCaseAndDecision(t *testing.T) {
	root := writeKineticFixture(t, "| D-063 | wiki/90-decisiones.md:1-1 |\n")
	casos := t.TempDir()
	line := `{"accion":"promover","que":"D-063 aceptada","ts":"t","quien":"ejecutor","rol":"ejecutor"}` + "\n"
	if err := os.WriteFile(filepath.Join(casos, "ontologia.jsonl"), []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}
	env := runKineticIntent(t, root, "promover", casos)
	if env.Mode != "docs" || !env.Ok || env.Backend != "intent" {
		t.Fatalf("envelope = ok:%v mode:%s backend:%s", env.Ok, env.Mode, env.Backend)
	}
	items, _ := env.Items.([]map[string]any)
	if !kineticHas(items, "accion", "nombre", "promover") {
		t.Fatalf("missing promover action: %#v", items)
	}
	if !kineticHas(items, "caso", "accion", "promover") {
		t.Fatalf("missing caso: %#v", items)
	}
	if !kineticHas(items, "cosa", "decision_id", "D-063") {
		t.Fatalf("missing D-063: %#v", items)
	}
	closeItem := items[len(items)-1]
	if closeItem["result_kind"] != "wiki_close" || closeItem["status"] != "al_dia" {
		t.Fatalf("wiki_close = %#v", closeItem)
	}
}

func TestKineticUnknownActionMarksCatalogDrift(t *testing.T) {
	root := writeKineticFixture(t, "")
	casos := t.TempDir()
	line := `{"accion":"inventar","que":"nada","ts":"t","quien":"ejecutor","rol":"ejecutor"}` + "\n"
	if err := os.WriteFile(filepath.Join(casos, "x.jsonl"), []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}
	env := runKineticIntent(t, root, "hola", casos)
	items, _ := env.Items.([]map[string]any)
	closeItem := items[len(items)-1]
	if closeItem["status"] != "drift" {
		t.Fatalf("status = %#v", closeItem["status"])
	}
	pages, _ := closeItem["pages"].([]map[string]any)
	if len(pages) != 1 || pages[0]["path"] != "wiki/10-conceptos/acciones.md" {
		t.Fatalf("pages = %#v", closeItem["pages"])
	}
}

func TestKineticDecisionQuestionStaysDocs(t *testing.T) {
	root := writeKineticFixture(t, "| D-056 | wiki/90-decisiones.md:1-1 |\n")
	env := runKineticIntent(t, root, "decisión D-056", t.TempDir())
	if env.Mode != "docs" {
		t.Fatalf("mode = %q", env.Mode)
	}
	if env.Mode == "code" {
		t.Fatal("kinetic project returned mode=code")
	}
}

func writeKineticFixture(t *testing.T, mapRow string) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "wiki", "10-conceptos")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	modelo := "| Tipo | Qué es | Campos |\n|---|---|---|\n| decisión | Elección | id |\n"
	acciones := "| Acción | Parámetros | Precondición | Efecto | Rol |\n|---|---|---|---|---|\n| promover | id | dicha | fila | camino |\n"
	if err := os.WriteFile(filepath.Join(dir, "modelo.md"), []byte(modelo), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "acciones.md"), []byte(acciones), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "wiki"), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "| ID | Rango |\n|---|---|\n" + mapRow
	if err := os.WriteFile(filepath.Join(root, "wiki", "90-mapa-ids.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "wiki", "90-decisiones.md"), []byte("decisión\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func runKineticIntent(t *testing.T, root, question, casos string) model.Envelope {
	t.Helper()
	ensureWritableTestHome(t)
	name := "kinetic-" + filepath.Base(root)
	reg := model.WorkspaceRegistration{Name: name, Root: root, Kind: model.WorkspaceKindSingle}
	if _, err := workspace.RegisterWorkspace(name, reg); err != nil {
		t.Fatalf("register: %v", err)
	}
	t.Cleanup(func() { _ = workspace.RemoveWorkspace(name) })
	app := New(root, nil)
	env, err := app.Execute(context.Background(), model.CommandRequest{
		Operation: "nav.intent",
		Context:   model.QueryOptions{Workspace: name},
		Payload:   map[string]any{"question": question, "casos": casos},
	})
	if err != nil {
		t.Fatalf("nav.intent: %v", err)
	}
	return env
}

func kineticHas(items []map[string]any, kind, key, value string) bool {
	for _, item := range items {
		if item["result_kind"] == kind && item[key] == value {
			return true
		}
	}
	return false
}
