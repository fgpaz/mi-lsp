# Task T1: Discovery — confirmar file:line y firmas

## Shared Context
**Goal:** Reconfirmar en HEAD los file:line de los hallazgos antes de mutar, y fijar las firmas cruzadas.
**Stack:** Go; navegación con `mi-lsp nav`.
**Architecture:** Read-only. Produce `discovery.yaml` que las lanes consumen para no inferir ubicaciones.

## Locked Decisions
- Read-only total. No editar, no indexar, no reiniciar daemon.
- Usar `mi-lsp nav search/refs/context` como navegador primario (no Grep crudo sobre `internal/`).
- Antes de cada comando mi-lsp: `$env:MI_LSP_CLIENT_NAME="claude-code"; $env:MI_LSP_SESSION_ID="claude-v050-discovery"`. Workspace alias `axi-smoke`.

## Task Metadata
```yaml
id: T1
depends_on: [T0]
agent_type: ps-explorer
goal_id: G1
github_issues: []
expected_outcome: "discovery.yaml con file:line confirmados por hallazgo y las 3 firmas cruzadas verificadas."
files:
  - read: internal/daemon/server.go
  - read: internal/service/workspace_ops.go
  - read: internal/daemon/state_store.go
  - read: internal/daemon/lifecycle.go
  - read: internal/daemon/admin.go
  - read: internal/worker/protocol.go
  - read: internal/model/types.go
  - read: internal/output/formatter.go
  - read: internal/docgraph/governance.go
complexity: medium
done_when:
  - "discovery.yaml lists confirmed line numbers for AUD-01/02/04/05, SEC-01/02/03/04/05, TOK-01/02/03, PERF-05/06/07/08"
evidence_expected:
  - ".docs/auditoria/2026-06-09-milsp-v050-remediation/discovery.yaml"
stop_if:
  - "a cited symbol/line no longer exists in HEAD — record it as DRIFT and flag the affected lane"
```

## Reference
Auditoría origen: `.docs/auditoria/2026-06-09-post-release-audit-v042/informe-auditoria.md` (secciones 1-4 con file:line).

## Prompt
Sos un explorador read-only. Para cada hallazgo de la auditoría con file:line, abrí el archivo y confirmá que el símbolo/línea citado sigue existiendo y describe lo que dice la auditoría. Donde la línea se haya movido, anotá la línea real. Confirmá específicamente:
- `internal/daemon/server.go`: la llamada a `app.Execute()` con `context.Background()` (AUD-01), `Version="dev"` (AUD-02), check `ProtocolVersion` (SEC-09), `recent_accesses` en system.status (TOK-01), purga al startup (PERF-05).
- `internal/service/workspace_ops.go`: `WithWorkspaceIndexLock` + `IndexWorkspace` (AUD-01/index-lock), parseo de path del root (AUD-05), `docs_ready = ... && !AECanon.Blocking` (governance-ae-canon).
- `internal/daemon/state_store.go`: `SetMaxOpenConns(1)` + retry telemetría (PERF-06), modos 0o644 (SEC-04), columnas workspace_root/entrypoint_path (SEC-05/TOK-03).
- `internal/daemon/lifecycle.go`: `Manager.Call` reenvío de ctx sin deadline (AUD-04), `enforceIdleMemoryBoundsLocked` (PERF-07).
- `internal/daemon/file_watcher.go`: `watcher.Add` por dir + debounce 500ms (PERF-08).
- `internal/daemon/admin.go`: `handleWorkspaceWarm` sin auth/Host/Origin (SEC-02), `handleIndex` sin CSP (SEC-10).
- `internal/worker/protocol.go`: `ReadFrame` lee header de 4 bytes sin bound (SEC-01).
- `internal/model/types.go`: struct `AccessEvent`, `QueryEnvelope`, `GovernanceStatus`, `AECanonStatus` (campos para tokens).
- `internal/output/formatter.go` + `truncator.go`: `compactItems`, `ApplyEnvelopeLimits` (TOK-05, envelope-double-apply).
Verificá las 3 firmas cruzadas: que `Engine`/indexer expone un punto donde insertar `StartBackgroundIndex`; que `StateStore` tiene un método de purga; que `model.AccessEvent` puede recibir `DecisionHash`.

## Execution Procedure
1. Exportá las env vars de attribution.
2. Por cada archivo de la lista, `mi-lsp nav search "<símbolo>" --workspace axi-smoke --include-content --format toon` o `Read` con offset; confirmá línea.
3. Anotá cada confirmación o DRIFT en `discovery.yaml`.
4. Si hay DRIFT, marcá la lane afectada y NO la bloquees: la lane recibe la línea corregida.

## Skeleton
```yaml
# discovery.yaml
confirmed:
  AUD-01: { file: internal/daemon/server.go, line: 235, symbol: "app.Execute(context.Background())", status: confirmed }
  SEC-01: { file: internal/worker/protocol.go, line: 23, symbol: ReadFrame, status: confirmed }
cross_interfaces:
  StartBackgroundIndex_insertion: { file: internal/indexer/indexer.go, near_line: 45 }
drift: []
```

## Verify
`Test-Path .docs/auditoria/2026-06-09-milsp-v050-remediation/discovery.yaml` → True

## Commit
(no commit; read-only discovery, evidencia bajo auditoria)
