# Task L7: Governance + tokens + --profile agent

## Shared Context
**Goal:** Auto-resolución de alias; AECanon.Blocking condicional; --profile agent unificado; recortes de verbosidad; envelope-limits una sola vez; módulos AE no hardcodeados.
**Stack:** Go.
**Architecture:** Worktree `C:/wt/v050-l7-governance-tokens-profile`, branch `v050/l7-governance-tokens-profile`. Único dueño de `docgraph/governance.go`, `service/governance.go`, `output/formatter.go`, `output/truncator.go`, `workspace/registry.go`, `cli/root.go`, `model/types.go`.

## Locked Decisions
- `--profile agent` (auto-activado cuando `client_name` ∈ {claude-code, codex, claude-ai, opencode, ...}, nunca para manual-cli): activa compress + omite recent_accesses, index_sync_details, ae_canon duplicado; baja defaults. Flag explícito `--profile human|agent` override.
- AUD-03: si el alias pedido no existe pero el cwd resuelve a un workspace inequívoco, autocorregir (con nota) en vez de fallar; mantené el error si hay ambigüedad.
- governance-ae-canon-blocking: `AECanon.Blocking` solo si el workspace declara capa AE en su gobernanza; si no, `Blocking=false` (no rompe `docs_ready` en workspaces no-AE).
- envelope-limits-double-apply: aplicar `ApplyEnvelopeLimits` UNA sola vez (en `printEnvelope`); quitar la aplicación pre-telemetría o documentá por qué se mantiene.
- ae-canon-modules-hardcoded: leer la lista de módulos AE requeridos desde read-model/projection en vez del array Go (fallback al array si falta).

## Task Metadata
```yaml
id: L7
depends_on: [T2]
agent_type: general-purpose
goal_id: G4
github_issues: []
expected_outcome: "perfil agent recorta daemon status a <1500 tokens; alias se autocorrige; AECanon no bloquea no-AE; truncación única."
files:
  - modify: internal/docgraph/governance.go
  - modify: internal/service/governance.go
  - modify: internal/output/formatter.go
  - modify: internal/output/truncator.go
  - modify: internal/workspace/registry.go
  - modify: internal/cli/root.go
  - modify: internal/model/types.go
complexity: high
done_when:
  - "go build ./... exits 0"
  - "go test ./internal/output/... ./internal/service/... ./internal/docgraph/... passes"
  - "a test asserts profile=agent strips recent_accesses/index_sync_details and a non-AE workspace has AECanon.Blocking=false"
evidence_expected:
  - ".docs/auditoria/2026-06-09-milsp-v050-remediation/L7-verdict.yaml"
stop_if:
  - "profile=agent default would also strip a field a harness genuinely needs (keep ok/items/hint/stats)"
  - "AECanon change makes axi-smoke (declares AE) stop blocking when it should"
```

## Reference
`discovery.yaml` AUD-03, governance-ae-canon(368-391, ops:445), TOK-04/05/06, envelope-double(234,471), ae-canon-modules(26-36). `model/types.go` ya tiene `OutputProfile` desde T2.

## Prompt
Editá SOLO tu set. Cambios:
1. TOK-05/--profile agent: en `cli/root.go` resolvé `QueryEnvelope.Profile` desde flag `--profile` o `client_name`; en `formatter.go`/`truncator.go` aplicá: compress on, omitir recent_accesses, index_sync_details, ae_canon repetido cuando Profile==agent. Conservá siempre ok/items/hint/stats/next.
2. AUD-03: en `registry.go` autocorrección de alias cuando el cwd resuelve inequívocamente.
3. governance-ae-canon: en `docgraph/governance.go` setear `Blocking=true` solo si el workspace declara capa AE; `service/governance.go` consume.
4. TOK-04: `index_sync_details` opt-in (`--show-sync-details`); default solo count.
5. TOK-06: no repetir `ae_canon` completo en workspace.status Y nav.governance; referenciar.
6. envelope-double-apply: una sola aplicación.
7. ae-canon-modules: leer módulos requeridos de projection/read-model con fallback al array.

## Execution Procedure
1. `cd C:/wt/v050-l7-governance-tokens-profile`; `git merge --no-edit main`.
2. Aplicá los cambios + tests (profile agent; no-AE workspace).
3. `go build ./... && go vet ./... && go test ./internal/output/... ./internal/service/... ./internal/docgraph/...`.
4. Commit. `L7-verdict.yaml` con estimación de tokens daemon status agent vs human.

## Skeleton
```go
func resolveProfile(flag string, client string) model.OutputProfile {
    if flag != "" { return model.OutputProfile(flag) }
    if isHarnessClient(client) { return model.OutputProfileAgent }
    return model.OutputProfileHuman
}
// formatter: if env.Profile == OutputProfileAgent { compress=true; drop(recentAccesses, indexSyncDetails, dupAECanon) }
```

## Verify
`go test ./internal/output/... ./internal/service/... ./internal/docgraph/...` → PASS

## Commit
`feat(tokens): --profile agent, alias autocorrect, conditional AECanon block, single truncation (AUD-03 TOK-04/05/06)`
