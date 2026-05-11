---
linear_parent: not_applicable
linear_child: not_applicable
anchors:
  - RF-WIKI-003
  - CT-NAV-WIKI
allowed_paths:
  - internal/cli/nav.go
  - internal/service/**
forbidden_paths:
  - .docs/wiki/**
  - worker-dotnet/**
verify:
  - go build ./... -> exit 0
  - "mi-lsp nav wiki route 'governance' --all-workspaces --format toon -> ok=true"
stop_if:
  - T7 no committeado
  - nav wiki route single-workspace está roto
secret_scan: clean
---

# Task T9: nav wiki route --all-workspaces

## Shared Context
**Goal:** Extender `nav wiki route` con `--all-workspaces` reusando FanOutWiki.
**Stack:** Go.
**Architecture:** Igual que T8 pero sobre el handler de `route` en `internal/cli/nav.go`.

## Locked Decisions
- Flag `--all-workspaces`. Sin `--top-global` (route ya tiene `--top`).
- Cuando hay candidatos en N wikis, devuelve N items con `workspace<>''`. NO unifica rutas entre wikis.
- Items incluyen el campo `workspace` y `host:""`.

## Task Metadata
```yaml
id: T9
depends_on: [T7]
agent_type: ps-worker
goal_id: G1
github_issues: []
expected_outcome: "nav wiki route soporta --all-workspaces."
files:
  - modify: internal/cli/nav.go
  - read: internal/nav/fanout_wiki.go
complexity: medium
done_when:
  - "go build PASS"
  - "mi-lsp nav wiki route 'governance' --all-workspaces --format toon -> ok=true"
  - "items con workspace<>'' cuando hay matches en >= 2 wikis"
  - "single-workspace mode preserva back-compat"
evidence_expected:
  - "Output con y sin --all-workspaces"
  - "Diff de nav.go"
stop_if:
  - "FanOutWiki no exporta WikiFanOutOptions/Result"
```

## Reference
- T8 (search) — copiar patrón de bifurcación cobra/RunE.
- `mi-lsp nav search "wiki route" --workspace mi-lsp --include-content --format toon` para ubicar el handler.

## Prompt

Sos el ejecutor de T9 (ps-worker). Mismo patrón que T8 pero sobre `nav wiki route`.

1. Localizar handler con `mi-lsp nav search "wiki route" --workspace mi-lsp --include-content --format toon`.
2. Agregar flag `--all-workspaces` (bool, default false).
3. Bifurcar RunE: si false, comportamiento actual; si true, invocar `nav.FanOutWiki`.
4. Cada closure `fn` corre el resolutor de ruta canónica per-workspace.
5. Envelope final: items planos con `workspace`, `host:""`; `stats.workspaces_queried/failed`.
6. Build, smoke (con y sin flag), commit, reportar.
7. **NO** introducir lógica de "unificar rutas entre wikis" — devolver tal cual N items.

## Execution Procedure
1. mi-lsp search del handler.
2. Editar nav.go.
3. go build + smoke.
4. Commit.
5. Reportar.

## Skeleton

(idéntico al de T8 con `wikiRouteOneWorkspace` en lugar de search)

## Verify
`mi-lsp nav wiki route "governance" --all-workspaces --format toon` -> `ok=true`

## Commit
`feat(nav): add --all-workspaces to nav wiki route`
