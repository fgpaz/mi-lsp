---
linear_parent: not_applicable
linear_child: not_applicable
anchors:
  - RF-WIKI-001
  - CT-NAV-WIKI
allowed_paths:
  - internal/cli/nav.go
  - internal/service/**
  - internal/nav/**
forbidden_paths:
  - .docs/wiki/**
  - worker-dotnet/**
verify:
  - go build ./... -> exit 0
  - "mi-lsp nav wiki search 'governance' --all-workspaces --format toon -> ok=true; items have workspace<>'' (smoke)"
stop_if:
  - T7 (FanOutWiki) no está committeado
  - nav wiki search single-workspace está roto antes de empezar
secret_scan: clean
---

# Task T8: nav wiki search --all-workspaces

## Shared Context
**Goal:** Extender `nav wiki search` con el flag `--all-workspaces` reusando `FanOutWiki` de T7.
**Stack:** Go 1.22+.
**Architecture:** `nav wiki search` vive en `internal/cli/nav.go` (líneas ~546-689) con backend en `internal/service/`. El flag se agrega al cobra/flagset y la rama `--all-workspaces` delega a `FanOutWiki`.

## Locked Decisions
- Flag: `--all-workspaces` (no negociable).
- Flags actuales se preservan: `--layer`, `--top`, `--offset`, `--include-content`.
- Flag nuevo: `--top-global` (default 50) cuando `--all-workspaces=true`. Trunca el merge global.
- Item del envelope incluye `workspace:<alias>` y `host:""`.
- Stats agrega `workspaces_queried`, `workspaces_failed[]`, `truncated_per_workspace`.
- Backward-compat: cuando `--all-workspaces=false` el envelope es 100% idéntico al actual.
- Score per-item es el score local del FTS5; merge ordena por `(score DESC, workspace ASC, doc_id ASC)`.

## Task Metadata
```yaml
id: T8
depends_on: [T7]
agent_type: ps-worker
goal_id: G1
github_issues: []
expected_outcome: "nav wiki search soporta --all-workspaces; envelope merge-friendly; back-compat preservada."
files:
  - modify: internal/cli/nav.go
  - modify_or_create: internal/service/wiki_search.go  # si el handler vive aparte
  - read: internal/nav/fanout_wiki.go   # creado en T7
complexity: high
done_when:
  - "go build ./... exit 0"
  - "mi-lsp nav wiki search 'governance' --all-workspaces --format toon devuelve ok=true"
  - "items[].workspace<>'' en al menos uno cuando hay >= 2 workspaces docs_ready"
  - "stats.workspaces_queried >= 1"
  - "mi-lsp nav wiki search 'governance' --workspace mi-lsp --format toon (sin flag global) sigue retornando shape igual al pre-cambio (back-compat)"
evidence_expected:
  - "Output de las dos invocaciones (con y sin --all-workspaces)"
  - "Diff de internal/cli/nav.go"
stop_if:
  - "FanOutWiki helper de T7 no existe o tiene signatura distinta"
```

## Reference
- Comando actual: `internal/cli/nav.go:546-689` (sección de wiki commands).
- Helper: `internal/nav/fanout_wiki.go` (T7).
- Patrón de cobra flag: mirar cómo `nav search` y `nav find` exponen `--all-workspaces` en `internal/cli/nav.go`.
- **OBLIGATORIO** usar mi-lsp para localizar el punto exacto de modificación:
  ```powershell
  mi-lsp nav search "nav wiki search" --workspace mi-lsp --include-content --format toon
  mi-lsp nav search "AllWorkspaces" --workspace mi-lsp --include-content --format toon
  mi-lsp nav refs "nav wiki search" --workspace mi-lsp --format toon
  ```

## Prompt

Sos el ejecutor de T8 (ps-worker). Tu trabajo es agregar un flag y una rama de fan-out a UN subcomando existente.

1. **Localizar**: `mi-lsp nav search "wiki search" --workspace mi-lsp --include-content --format toon`. Confirmar la sección donde está el handler.
2. **Inspirarse en `--all-workspaces` existente**: `mi-lsp nav search "AllWorkspaces" --workspace mi-lsp --include-content --format toon` — mirar cómo `nav search` y `nav find` declaran el flag y bifurcan al fan-out.
3. **Editar `internal/cli/nav.go`** (sección de wiki search):
   - Agregar el flag bool `--all-workspaces` al cobra command (default false).
   - Agregar el flag int `--top-global` (default 50).
   - Bifurcar el handler:
     - Si `!allWorkspaces`: ejecutar la rama actual sin cambios.
     - Si `allWorkspaces`: invocar `nav.FanOutWiki(ctx, opts, fn)` donde `fn` consulta el doc-index per-workspace usando la misma lógica del handler single-workspace.
4. **Envelope merge**: el resultado del fan-out debe serializarse a TOON con:
   - `items[]`: lista plana ordenada por `(score DESC, workspace ASC, doc_id ASC)`, truncada al `topGlobal`.
   - Cada item tiene `workspace`, `host:""`, y los campos existentes (doc_id, block_id, score, snippet, layer, etc.).
   - `stats.workspaces_queried`, `stats.workspaces_failed[]`, `stats.truncated_per_workspace`.
5. **Back-compat**: cuando `--all-workspaces=false`, NO agregar ningún campo nuevo al envelope.
6. Build:
   ```powershell
   go build ./...
   go vet ./internal/cli/...
   ```
7. Smoke test:
   ```powershell
   mi-lsp nav wiki search "governance" --workspace mi-lsp --format toon
   mi-lsp nav wiki search "governance" --all-workspaces --format toon
   ```
8. Commit: `feat(nav): add --all-workspaces to nav wiki search`.
9. Reportar diff y outputs.

## Execution Procedure
1. mi-lsp search/refs sobre `wiki search` y `AllWorkspaces`.
2. Editar nav.go.
3. go build + vet.
4. Smoke test las dos modalidades.
5. Commit.
6. Reportar.

## Skeleton

```go
// dentro del cobra command de "nav wiki search"
var (
    allWorkspaces bool
    topGlobal     int
)
cmd.Flags().BoolVar(&allWorkspaces, "all-workspaces", false, "search across every registered workspace")
cmd.Flags().IntVar(&topGlobal, "top-global", 50, "global top-N when --all-workspaces is set")

cmd.RunE = func(cmd *cobra.Command, args []string) error {
    if !allWorkspaces {
        return runWikiSearchSingle(...)
    }
    result, err := nav.FanOutWiki(ctx, nav.WikiFanOutOptions{
        Timeout:  30 * time.Second,
        Parallel: 4,
    }, func(ctx context.Context, ws registry.WorkspaceRegistration) ([]any, map[string]any, error) {
        return wikiSearchOneWorkspace(ctx, ws, query, layer, top, offset, includeContent)
    })
    if err != nil {
        return err
    }
    return emitMergedWikiSearchEnvelope(out, result, topGlobal)
}
```

## Verify
`go build ./...` -> exit 0 AND `mi-lsp nav wiki search 'governance' --all-workspaces --format toon` -> `ok: true` con `items[].workspace<>''`

## Commit
`feat(nav): add --all-workspaces to nav wiki search`
