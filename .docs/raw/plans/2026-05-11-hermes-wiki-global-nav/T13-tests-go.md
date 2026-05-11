---
linear_parent: not_applicable
linear_child: not_applicable
anchors:
  - RF-WIKI-001
  - RF-WIKI-002
  - RF-WIKI-003
  - RF-WIKI-004
  - RF-WIKI-005
  - TP-WIKI
allowed_paths:
  - internal/cli/nav_test.go
  - internal/nav/**_test.go
  - internal/service/**_test.go
  - testdata/wiki-fanout/**
forbidden_paths:
  - .docs/wiki/**
  - worker-dotnet/**
verify:
  - go test ./internal/cli/... ./internal/nav/... ./internal/service/... -count=1 -> PASS
stop_if:
  - T8-T12 no committeadas
  - el test runner reporta failures pre-existentes en archivos no relacionados (governance issue, no improvisar)
secret_scan: clean
---

# Task T13: Tests Go para los cinco subcomandos federados

## Shared Context
**Goal:** Cubrir con tests Go los criterios de aceptación de RF-WIKI-001..005 y el primitive FanOutWiki: back-compat, envelope merge-friendly, fallo no-fatal.
**Stack:** Go testing standard.
**Architecture:** Tests viven en `internal/cli/nav_test.go`, `internal/nav/*_test.go`, `internal/service/*_test.go`. Fixtures (mini-wikis de prueba) en `testdata/wiki-fanout/`.

## Locked Decisions
- Tests del primitive `FanOutWiki` en `internal/nav/fanout_wiki_test.go`.
- Tests CLI con tabletests para los cinco subcomandos en `internal/cli/nav_test.go` (archivo existente).
- Fixtures de mini-wikis: tres workspaces sintéticos en `testdata/wiki-fanout/` con `.docs/wiki/` poblado mínimamente.
- Mínimo de assertions por RF: 3 test cases que cubran los TCs documentados en TP-WIKI (no necesariamente uno por TC).

## Task Metadata
```yaml
id: T13
depends_on: [T8, T9, T10, T11, T12]
agent_type: ps-worker
goal_id: G1
github_issues: []
expected_outcome: "Go tests cubren back-compat, envelope merge-friendly y fallo no-fatal para los cinco subcomandos. go test PASS."
files:
  - modify: internal/cli/nav_test.go
  - create: internal/nav/fanout_wiki_test.go
  - create_optional: internal/service/wiki_inventory_test.go
  - create: testdata/wiki-fanout/  # tres directorios sintéticos con .docs/wiki/
complexity: high
done_when:
  - "go test ./internal/cli/... ./internal/nav/... ./internal/service/... -count=1 PASS"
  - "tests cubren: back-compat single-workspace, items con workspace<>'', stats.workspaces_queried, workspaces_failed, fallo no-fatal"
  - "FanOutWiki test simula un workspace que falla (timeout) y verifica que el resultado global no aborta"
evidence_expected:
  - "Output completo de go test (al menos el resumen final con N tests passed)"
  - "Lista de nombres de tests agregados"
stop_if:
  - "Helpers de cobra para invocar comandos en tests no existen — pedir guía antes de inventar"
```

## Reference
- Archivos test existentes:
  - `internal/cli/nav_test.go` (extender; ya existe)
  - `internal/service/ask_test.go` (ver cómo testean `AllWorkspaces`; replicar setup de fixtures)
- Patrón para fixtures: buscar en `testdata/` (probable existencia).
- Comandos de mi-lsp para localizar patrones:
  ```powershell
  mi-lsp nav search "AllWorkspaces" --workspace mi-lsp --include-content --format toon
  mi-lsp nav search "testdata" --workspace mi-lsp --include-content --format toon
  ```

## Prompt

Sos el ejecutor de T13 (ps-worker). Escribir tests Go.

1. **Inspeccionar patrones existentes**:
   ```powershell
   mi-lsp nav search "AllWorkspaces" --workspace mi-lsp --include-content --format toon
   mi-lsp nav search "testdata" --workspace mi-lsp --include-content --format toon
   mi-lsp nav refs FanOutWiki --workspace mi-lsp --format toon
   ```
2. Leer `internal/service/ask_test.go` para entender cómo testean `AllWorkspaces` (fixtures, mocks, asserts).
3. **Crear fixtures** en `testdata/wiki-fanout/` con tres workspaces sintéticos:
   - `ws-alpha/`: docs_ready=true, gobierno OK, contiene `.docs/wiki/04_RF/RF-DUMMY-001.md`
   - `ws-bravo/`: docs_ready=true, gobierno OK, contiene `.docs/wiki/03_FL/FL-DUMMY-01.md`
   - `ws-charlie/`: docs_ready=false (sin `.docs/wiki/`) o con governance_blocked simulado
4. **Crear `internal/nav/fanout_wiki_test.go`** con al menos:
   - `TestFanOutWiki_HappyPath`: tres workspaces, todos OK, verifica `WorkspacesQueried=3`, `WorkspacesFailed=[]`, items planos con workspace<>''.
   - `TestFanOutWiki_OneFails`: dos OK, uno timeout simulado, verifica `WorkspacesQueried=3`, `len(WorkspacesFailed)=1`, resultado global no nil.
   - `TestFanOutWiki_SemaphoreBounds`: lanza N=20 workspaces sintéticos y verifica que nunca hay más de 4 goroutines simultáneas (vía contador atómico).
5. **Extender `internal/cli/nav_test.go`** con table tests para los cinco subcomandos:
   - Cada subcomando: dos test cases mínimos:
     - `<subcmd>_SingleWorkspace_BackCompat`: invoca sin `--all-workspaces`, verifica que el envelope NO contiene `workspaces_queried`.
     - `<subcmd>_AllWorkspaces_HasWorkspaceField`: invoca con flag, verifica `items[].workspace<>''` y `stats.workspaces_queried >= 1`.
6. **Inventory specific test**: `TestNavWikiInventory_WithLayerCounts` verifica que el field `layers` está presente solo con el flag opt-in.
7. **Pack specific test**: `TestNavWikiPack_AllWorkspaces_NMiniPacks` verifica que el envelope tiene N items (uno por workspace), NO un super-pack mergeado.
8. Correr:
   ```powershell
   go test ./internal/cli/... ./internal/nav/... ./internal/service/... -count=1 -v
   ```
   Confirmar PASS.
9. Commit: `test(nav): cover FanOutWiki and federated nav wiki subcommands`.
10. Reportar resumen al orquestador.

## Execution Procedure
1. mi-lsp search/refs sobre patrones.
2. Leer ask_test.go.
3. Crear fixtures.
4. Crear fanout_wiki_test.go.
5. Extender nav_test.go.
6. go test -count=1 PASS.
7. Commit.
8. Reportar.

## Skeleton

```go
// internal/nav/fanout_wiki_test.go
func TestFanOutWiki_HappyPath(t *testing.T) {
    // setup three fixtures
    res, err := FanOutWiki(ctx, WikiFanOutOptions{...}, func(ctx context.Context, ws registry.WorkspaceRegistration) ([]any, map[string]any, error) {
        return []any{"item-from-" + ws.Alias}, nil, nil
    })
    require.NoError(t, err)
    require.Equal(t, 3, res.WorkspacesQueried)
    require.Empty(t, res.WorkspacesFailed)
    require.Len(t, res.Items, 3)
}

func TestFanOutWiki_OneFails(t *testing.T) {
    res, err := FanOutWiki(ctx, WikiFanOutOptions{Timeout: 100 * time.Millisecond, Parallel: 4}, func(ctx context.Context, ws registry.WorkspaceRegistration) ([]any, map[string]any, error) {
        if ws.Alias == "ws-charlie" {
            time.Sleep(200 * time.Millisecond)  // forzar timeout
            return nil, nil, ctx.Err()
        }
        return []any{ws.Alias}, nil, nil
    })
    require.NoError(t, err)
    require.Equal(t, 3, res.WorkspacesQueried)
    require.Len(t, res.WorkspacesFailed, 1)
    require.Equal(t, "ws-charlie", res.WorkspacesFailed[0].Alias)
}
```

## Verify
`go test ./internal/cli/... ./internal/nav/... ./internal/service/... -count=1` -> `PASS` con al menos 10 tests nuevos

## Commit
`test(nav): cover FanOutWiki and federated nav wiki subcommands`
