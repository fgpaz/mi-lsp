---
linear_parent: not_applicable
linear_child: not_applicable
anchors:
  - RF-WIKI-005
  - CT-NAV-WIKI
allowed_paths:
  - internal/cli/nav.go
  - internal/service/**
forbidden_paths:
  - .docs/wiki/**
  - worker-dotnet/**
verify:
  - go build ./... -> exit 0
  - "mi-lsp nav wiki pack --all-workspaces --format toon -> ok=true"
stop_if:
  - T7 no committeado
secret_scan: clean
---

# Task T11: nav wiki pack --all-workspaces

## Shared Context
**Goal:** Extender `nav wiki pack` con `--all-workspaces`. Resultado: N mini-packs (uno por workspace), NO un super-pack.
**Stack:** Go.
**Architecture:** Handler `pack` en `internal/cli/nav.go`.

## Locked Decisions
- Flag `--all-workspaces` (bool).
- `--rf`, `--fl`, `--doc` se aplican per-workspace.
- El envelope global tiene `items[]` donde cada item es un mini-pack del workspace correspondiente (con su propio `doc_count`, `tokens_est`, etc., más `workspace` y `host:""`).
- **No mergear documentos en un super-pack** — cada wiki es self-contained.

## Task Metadata
```yaml
id: T11
depends_on: [T7]
agent_type: ps-worker
goal_id: G1
github_issues: []
expected_outcome: "nav wiki pack soporta --all-workspaces devolviendo mini-packs por workspace."
files:
  - modify: internal/cli/nav.go
  - read: internal/nav/fanout_wiki.go
complexity: medium
done_when:
  - "go build PASS"
  - "mi-lsp nav wiki pack --all-workspaces --format toon -> ok=true; items[N] cada uno con workspace<>''"
  - "items[].kind o equivalente identifica que es un mini-pack"
  - "back-compat single-workspace"
evidence_expected:
  - "Output con y sin --all-workspaces, y con --rf <id> federado"
  - "Diff de nav.go"
stop_if:
  - "FanOutWiki no disponible"
  - "pack single-workspace tiene una semántica que hace incompatible la federación (reportar al humano antes de improvisar)"
```

## Reference
- T8/T9/T10 — patrón cobra/RunE.
- `mi-lsp nav search "wiki pack" --workspace mi-lsp --include-content --format toon`.

## Prompt

Sos el ejecutor de T11. Mismo patrón pero la salida es **distinta**: items planos donde cada item es un mini-pack.

1. mi-lsp search del handler pack.
2. Flag `--all-workspaces`. Preservar `--rf`, `--fl`, `--doc`.
3. RunE bifurca: single-workspace = comportamiento actual; --all-workspaces = FanOutWiki con closure que arma pack per-workspace.
4. Envelope: `items[]` donde cada elemento contiene `workspace`, `host:""`, y el pack-data del workspace (resúmenes, doc_count, tokens_est).
5. **Crítico**: si un workspace no tiene el `--rf <id>` solicitado, el item para ese workspace es vacío o se omite con `hint`. NO falla el global.
6. Build, smoke (con y sin flags), commit.
7. Reportar comportamiento observado al orquestador (especialmente si encontraste que algún flag actual NO se puede federar — pausar y reportar antes de bypass).

## Execution Procedure
1. mi-lsp search del handler.
2. Inspeccionar single-workspace pack — entender qué retorna.
3. Editar nav.go con el patrón.
4. Build + smoke con `--rf RF-WIKI-001 --all-workspaces` y sin flags.
5. Commit.
6. Reportar.

## Skeleton

```go
// closure que construye un mini-pack por workspace
fn := func(ctx context.Context, ws registry.WorkspaceRegistration) ([]any, map[string]any, error) {
    pack, err := buildWikiPack(ctx, ws, rfFilter, flFilter, docFilter)
    if err != nil {
        return nil, nil, err
    }
    return []any{pack}, packStats(pack), nil
}
```

## Verify
`mi-lsp nav wiki pack --all-workspaces --format toon` -> `ok=true` con items[N] cada uno con `workspace<>''`

## Commit
`feat(nav): add --all-workspaces to nav wiki pack (N mini-packs per workspace)`
