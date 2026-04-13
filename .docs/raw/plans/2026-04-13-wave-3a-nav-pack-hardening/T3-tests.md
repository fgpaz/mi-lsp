# Task T3: Tests — anchor hardening + next_queries

## Shared Context
**Goal:** Verificar que el anchor explícito gana sobre route core, y que next_queries se popula.
**Stack:** Go test, `internal/service/pack_test.go`, fixtures en `createFunctionalPackWorkspaceFixture`
**Architecture:** Los tests nuevos se agregan a `pack_test.go`. El fixture existente `createFunctionalPackWorkspaceFixture` crea wiki con governance + docs reales. Los tests de Wave 2 (stale/generic) ya pasan.

## Task Metadata
```yaml
id: T3
depends_on: [T1, T2]
agent_type: ps-worker
files:
  - modify: internal/service/pack_test.go
complexity: low
done_when: "go test ./internal/service -run Pack -v -count=1 EXIT:0"
```

## Reference
`internal/service/pack_test.go:50-102` — `TestNavPackBuildsFunctionalReadingPackInCanonicalOrder` como patrón.
`internal/service/governance_test_helpers_test.go` — `writeSpecBackendGovernanceFixture` y `writeWorkspaceFile`.
`internal/service/route_test.go` — patrón de tests para RouteResult.

## Prompt
Eres un agente de implementación. Agrega tests a `pack_test.go` para cubrir:

**Test 1: `TestNavPackNextQueriesArePopulated`**
- Setup: usar `createFunctionalPackWorkspaceFixture` + `workspace.init`
- Ejecutar `nav.pack` con task="understand how login works"
- Assertar: `results[0].NextQueries` tiene al menos 1 elemento
- Assertar: cada query contiene "mi-lsp" como prefijo

**Test 2: `TestNavPackExplicitRFAnchorWinsOverRouteCore`**
- Setup: usar `createFunctionalPackWorkspaceFixture` + `workspace.init`
- Ejecutar `nav.pack` con payload `{"task": "understand login", "rf": "RF-AUTH-001"}`
- Assertar: `results[0].PrimaryDoc` es el path del doc con DocID=RF-AUTH-001 (`.docs/wiki/04_RF/RF-AUTH-001.md`)

Nota importante: el fixture ya indexa los docs con `workspace.init`. Asegurate de que el workspace esté registrado y el índice esté disponible antes de ejecutar el pack.

Patrón de test a seguir — copiar y adaptar de `TestNavPackBuildsFunctionalReadingPackInCanonicalOrder`:
```go
func TestNavPackNextQueriesArePopulated(t *testing.T) {
    alias := "pack-nq-" + filepath.Base(t.TempDir())
    root := createFunctionalPackWorkspaceFixture(t, alias)
    app := New(root, nil)
    if _, err := app.Execute(context.Background(), model.CommandRequest{
        Operation: "workspace.init",
        Context:   model.QueryOptions{},
        Payload:   map[string]any{"path": root, "alias": alias},
    }); err != nil {
        t.Fatalf("workspace.init: %v", err)
    }
    defer func() { _ = workspace.RemoveWorkspace(alias) }()

    env, err := app.Execute(context.Background(), model.CommandRequest{
        Operation: "nav.pack",
        Context:   model.QueryOptions{Workspace: alias, AXI: true, MaxItems: 6},
        Payload:   map[string]any{"task": "understand how login works"},
    })
    if err != nil {
        t.Fatalf("nav.pack: %v", err)
    }
    results := env.Items.([]model.PackResult)
    if len(results[0].NextQueries) == 0 {
        t.Fatalf("expected next_queries to be populated, got empty")
    }
    if !strings.HasPrefix(results[0].NextQueries[0], "mi-lsp") {
        t.Fatalf("expected next query to start with mi-lsp, got %q", results[0].NextQueries[0])
    }
}
```

NO tocar los tests existentes. Solo agregar al final del archivo.

## Skeleton
```go
// Al final de pack_test.go

func TestNavPackNextQueriesArePopulated(t *testing.T) { ... }

func TestNavPackExplicitRFAnchorWinsOverRouteCore(t *testing.T) { ... }
```

## Verify
`go test ./internal/service -run "TestNavPackNextQueries|TestNavPackExplicit" -v -count=1` → PASS

## Commit
`test(pack): add next_queries and explicit anchor hardening tests`
