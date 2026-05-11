---
linear_parent: not_applicable
linear_child: not_applicable
anchors:
  - RF-WIKI-002
  - CT-NAV-WIKI
allowed_paths:
  - internal/cli/nav.go
  - internal/service/**
forbidden_paths:
  - .docs/wiki/**
  - worker-dotnet/**
verify:
  - go build ./... -> exit 0
  - "mi-lsp nav wiki inventory --all-workspaces --format toon -> ok=true; items con alias, root, governance_blocked"
  - "mi-lsp nav wiki inventory --all-workspaces --with-layer-counts --format toon -> items con layers"
stop_if:
  - T7 no committeado
  - registry no expone last_indexed_at o equivalente per-workspace
secret_scan: clean
---

# Task T12: nav wiki inventory --all-workspaces (NUEVO subcomando)

## Shared Context
**Goal:** Crear el subcomando NUEVO `nav wiki inventory` con default `--all-workspaces` (porque single-workspace inventory es trivial) y modo ligero por default + `--with-layer-counts` opt-in.
**Stack:** Go.
**Architecture:** Nuevo cobra command bajo `nav wiki`. Vive en `internal/cli/nav.go` junto a los otros wiki subcommands.

## Locked Decisions
- Comando: `nav wiki inventory`.
- Flag `--all-workspaces` opcional (default `true` — porque "inventory de un solo workspace" es lo que ya hace `workspace status`; `inventory` tiene sentido como vista global).
- Flag `--with-layer-counts` (default `false`).
- Item shape default (`~2-3KB / 50 wikis`):
  - `alias`, `root`, `wiki_root`, `governance_blocked`, `docs_ready`, `doc_count`, `last_indexed_at`.
- Con `--with-layer-counts` agregar `layers: {RS, FL, RF, TP, TECH, DB, CT}` con conteos enteros.
- `stats.workspaces_queried`, `stats.workspaces_failed[]`.
- Workspaces con `governance_blocked=true` NO se omiten — entran al inventario para que Hermes los vea.

## Task Metadata
```yaml
id: T12
depends_on: [T7]
agent_type: ps-worker
goal_id: G1
github_issues: []
expected_outcome: "nav wiki inventory existe como subcomando nuevo y devuelve el envelope ligero + opt-in con conteos por capa."
files:
  - modify: internal/cli/nav.go
  - modify_or_create: internal/service/wiki_inventory.go  # si el handler vive aparte
  - read: internal/nav/fanout_wiki.go
  - read: internal/registry/      # para WorkspaceRegistration shape (alias, root, last_indexed_at)
complexity: high
done_when:
  - "go build PASS"
  - "mi-lsp nav wiki inventory --format toon devuelve items con shape mínimo"
  - "mi-lsp nav wiki inventory --with-layer-counts --format toon agrega el campo layers"
  - "items[].governance_blocked está presente y refleja el estado real"
  - "stats.workspaces_queried >= 1"
evidence_expected:
  - "Output de las dos invocaciones (con y sin --with-layer-counts)"
  - "Tamaño del envelope para 50 workspaces aproximadamente <= 5KB con --with-layer-counts; <= 3KB sin él"
stop_if:
  - "registry no exporta last_indexed_at — alertar para discutir alternativa antes de improvisar"
```

## Reference
- T8 (search) — patrón cobra para el comando nuevo.
- T7 (FanOutWiki) — primitiva a usar.
- Inspirarse en `workspace status` que ya muestra parte de la info per-workspace:
  ```powershell
  mi-lsp nav search "workspace status" --workspace mi-lsp --include-content --format toon
  mi-lsp nav search "last_indexed_at" --workspace mi-lsp --include-content --format toon
  ```

## Prompt

Sos el ejecutor de T12 (ps-worker). Tu trabajo es crear un comando NUEVO bajo `nav wiki`.

1. Localizar la definición del grupo `nav wiki` en `internal/cli/nav.go` (donde se registran search/route/trace/pack).
2. Crear el subcomando `inventory` siguiendo el patrón:
   ```go
   wikiInventoryCmd := &cobra.Command{
       Use:   "inventory",
       Short: "List registered wikis with metadata (alias, root, governance, doc_count, last_indexed_at)",
       RunE: func(cmd *cobra.Command, args []string) error {
           // ...
       },
   }
   ```
3. Flags:
   - `--all-workspaces` (bool, default `true`).
   - `--with-layer-counts` (bool, default `false`).
   - `--workspace <alias>` permitido cuando `--all-workspaces=false` (single-workspace = comportamiento subset).
4. Implementar el handler:
   - Si `allWorkspaces=true`: usar `nav.FanOutWiki` con closure que construye el item per-workspace.
   - Cada closure lee: `alias`, `root`, `wiki_root` (default `.docs/wiki`), `governance_blocked`, `docs_ready`, `doc_count`, `last_indexed_at` desde el registry y el repo-local index (`.mi-lsp/index.db`).
   - Si `withLayerCounts=true`: además consultar el doc-index per-workspace para contar docs por capa (RS, FL, RF, TP, TECH, DB, CT). Usar SQL contra `doc_records` o equivalente.
5. **No incluir items vacíos**: si un workspace no tiene `.docs/wiki/`, mantenerlo en el inventario con `wiki_root: ""` y `docs_ready: false` (no omitir — Hermes debe ver todos).
6. **Workspaces con governance_blocked**: incluir con `governance_blocked: true`; NO consultar capas adicionales si está bloqueado (skip `layers`).
7. Registrar el comando en el grupo:
   ```go
   wikiCmd.AddCommand(wikiInventoryCmd)
   ```
8. Build y smoke:
   ```powershell
   go build ./...
   mi-lsp nav wiki inventory --format toon | Out-String -Stream | Select-Object -First 50
   mi-lsp nav wiki inventory --with-layer-counts --format toon | Out-String -Stream | Select-Object -First 50
   ```
9. Commit: `feat(nav): add nav wiki inventory subcommand with --all-workspaces and --with-layer-counts`.
10. Reportar tamaño del envelope (líneas TOON) en ambos modos.

## Execution Procedure
1. mi-lsp search del grupo wiki y de `last_indexed_at`.
2. Inspeccionar registry para conocer fields disponibles.
3. Implementar `wikiInventoryCmd` y `wikiInventoryRun` (handler).
4. Conectar con FanOutWiki.
5. Build + smoke en ambos modos.
6. Commit.
7. Reportar.

## Skeleton

```go
// dentro de internal/cli/nav.go en la sección wiki
var (
    invAllWorkspaces   bool
    invWithLayerCounts bool
)

wikiInventoryCmd := &cobra.Command{
    Use:   "inventory",
    Short: "List registered wikis with metadata",
    RunE:  runWikiInventory,
}
wikiInventoryCmd.Flags().BoolVar(&invAllWorkspaces, "all-workspaces", true, "list every registered workspace (default true)")
wikiInventoryCmd.Flags().BoolVar(&invWithLayerCounts, "with-layer-counts", false, "include doc counts per layer (RS, FL, RF, TP, TECH, DB, CT)")

wikiCmd.AddCommand(wikiInventoryCmd)

func runWikiInventory(cmd *cobra.Command, args []string) error {
    result, err := nav.FanOutWiki(ctx, opts, func(ctx context.Context, ws registry.WorkspaceRegistration) ([]any, map[string]any, error) {
        item := map[string]any{
            "alias":              ws.Alias,
            "root":               ws.Root,
            "wiki_root":          detectWikiRoot(ws),
            "governance_blocked": ws.GovernanceBlocked,
            "docs_ready":         ws.DocsReady,
            "doc_count":          countDocs(ws),
            "last_indexed_at":    ws.LastIndexedAt,
        }
        if invWithLayerCounts && !ws.GovernanceBlocked && ws.DocsReady {
            item["layers"] = countByLayer(ws)
        }
        return []any{item}, nil, nil
    })
    // emit envelope con items y stats
    return emitInventoryEnvelope(out, result)
}
```

## Verify
`mi-lsp nav wiki inventory --format toon` -> items con `alias`/`root`/`governance_blocked`/`docs_ready`/`doc_count`/`last_indexed_at`; con `--with-layer-counts` agrega `layers`

## Commit
`feat(nav): add nav wiki inventory subcommand with --all-workspaces and --with-layer-counts`
