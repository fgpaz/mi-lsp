# Task T3: Tests — TOON default, --axi=false, stage signal

## Shared Context
**Goal:** Verificar TOON default en AXI, --axi=false, y stage anchor|preview|discovery.
**Stack:** Go test, `internal/cli/axi_test.go` (para AXI mode tests), `internal/service/route_test.go`
**Architecture:** Los tests de AXI mode ya pasan. Hay que agregar tests para las nuevas reglas. Los tests de service no necesitan el CLI completo — pueden testear directamente los resultados de `route_test.go`.

## Task Metadata
```yaml
id: T3
depends_on: [T1, T2]
agent_type: ps-worker
files:
  - modify: internal/service/route_test.go      # stage signal tests
  - modify: internal/cli/axi_test.go            # --axi=false test (si aplica)
complexity: low
done_when: "go test ./internal/... -count=1 EXIT:0"
```

## Reference
`internal/service/route_test.go` — tests existentes de nav.route como patrón.
`internal/cli/axi_test.go` — tests AXI existentes como patrón.
`internal/service/governance_test_helpers_test.go:10-48` — fixture `createFunctionalPackWorkspaceFixture`.

## Prompt
Agrega tres tests nuevos.

**Test 1 en `route_test.go`: `TestNavRouteAnchorDocHasAnchorStage`**
```go
func TestNavRouteAnchorDocHasAnchorStage(t *testing.T) {
    // Setup con createFunctionalPackWorkspaceFixture + workspace register
    // Ejecutar nav.route con task="understand login"
    // Assertar: results[0].Canonical.AnchorDoc.Stage == "anchor"
}
```

**Test 2 en `route_test.go`: `TestNavRoutePreviewPackHasPreviewStage`**
```go
func TestNavRoutePreviewPackHasPreviewStage(t *testing.T) {
    // Setup con workspace.init (para tener índice)
    // Ejecutar nav.route con task="understand login" 
    // Si len(results[0].Canonical.PreviewPack) > 0:
    //   assertar que cada doc en PreviewPack tiene Stage == "preview" || Stage == "anchor"
    // (algunos puede que sean del Tier 1 con stage de canonicalPathsForStage)
}
```

**Test 3 en `route_test.go`: `TestNavRouteDiscoveryDocsHaveDiscoveryStage`**
```go
func TestNavRouteDiscoveryDocsHaveDiscoveryStage(t *testing.T) {
    // Setup con workspace.init (para tener índice)
    // Ejecutar nav.route con task="understand login" y Full=true (para activar discovery)
    // Si results[0].Discovery != nil && len(results[0].Discovery.Docs) > 0:
    //   assertar que cada doc tiene Stage == "discovery"
}
```

Para cada test:
- Usar `createFunctionalPackWorkspaceFixture(t, alias)` + `workspace.RegisterWorkspace`
- Usar `defer workspace.RemoveWorkspace(alias)`
- Usar `app := New(root, nil)`
- Ejecutar via `app.Execute`

**NO agregar** test de TOON format en service tests (el format default es una responsabilidad del CLI, no del service). El TOON default test sería en un integration test del CLI que está fuera de scope de este plan.

## Skeleton
```go
func TestNavRouteAnchorDocHasAnchorStage(t *testing.T) {
    alias := "route-stage-" + t.Name()
    root := createFunctionalPackWorkspaceFixture(t, alias)
    // ... register, defer, app.Execute ...
    results := env.Items.([]model.RouteResult)
    if results[0].Canonical.AnchorDoc.Stage != "anchor" {
        t.Fatalf("expected anchor stage, got %q", results[0].Canonical.AnchorDoc.Stage)
    }
}
```

## Verify
`go test ./internal/service -run "TestNavRoute.*Stage" -v -count=1` → PASS

## Commit
`test(route): verify anchor|preview|discovery stage signals in RouteDoc`
