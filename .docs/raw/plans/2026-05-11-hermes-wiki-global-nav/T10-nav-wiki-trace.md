---
linear_parent: not_applicable
linear_child: not_applicable
anchors:
  - RF-WIKI-004
  - CT-NAV-WIKI
allowed_paths:
  - internal/cli/nav.go
  - internal/service/**
forbidden_paths:
  - .docs/wiki/**
  - worker-dotnet/**
verify:
  - go build ./... -> exit 0
  - "mi-lsp nav wiki trace --all-workspaces --format toon -> ok=true"
stop_if:
  - T7 no committeado
secret_scan: clean
---

# Task T10: nav wiki trace --all-workspaces

## Shared Context
**Goal:** Extender `nav wiki trace` con `--all-workspaces`. Conserva flags actuales (`--all` para RF-only, `--summary`).
**Stack:** Go.
**Architecture:** Handler `trace` en `internal/cli/nav.go`.

## Locked Decisions
- Flag `--all-workspaces` (bool).
- `--all` y `--summary` se preservan, se aplican per-workspace.
- NO se fusionan trazas entre wikis distintas (cada workspace devuelve su set de trazas; el envelope global agrupa por `workspace`).
- Items con `workspace`, `host:""`.

## Task Metadata
```yaml
id: T10
depends_on: [T7]
agent_type: ps-worker
goal_id: G1
github_issues: []
expected_outcome: "nav wiki trace soporta --all-workspaces."
files:
  - modify: internal/cli/nav.go
  - read: internal/nav/fanout_wiki.go
complexity: medium
done_when:
  - "go build PASS"
  - "mi-lsp nav wiki trace --all-workspaces --format toon -> ok=true"
  - "--all --all-workspaces preserva semántica per-workspace"
  - "back-compat single-workspace"
evidence_expected:
  - "Output con y sin --all-workspaces, con --all, con --summary"
  - "Diff de nav.go"
stop_if:
  - "FanOutWiki no disponible"
```

## Reference
- T8/T9 — copiar patrón cobra/RunE.
- `mi-lsp nav search "wiki trace" --workspace mi-lsp --include-content --format toon`.

## Prompt

Sos el ejecutor de T10. Mismo patrón que T8/T9 sobre `nav wiki trace`.

1. mi-lsp search del handler.
2. Flag `--all-workspaces`; preservar `--all` y `--summary`.
3. Bifurcar RunE. Cuando `--all-workspaces=true`, ejecutar trace per-workspace via closure.
4. Envelope global: items agrupados por workspace (no fusionar trazas cross-wiki).
5. Build, smoke, commit.

## Execution Procedure
1. mi-lsp search del handler.
2. Editar.
3. Build + smoke (al menos 3 combinaciones de flags).
4. Commit.
5. Reportar.

## Skeleton

(igual a T8 con `wikiTraceOneWorkspace`)

## Verify
`mi-lsp nav wiki trace --all-workspaces --format toon` -> `ok=true`

## Commit
`feat(nav): add --all-workspaces to nav wiki trace`
