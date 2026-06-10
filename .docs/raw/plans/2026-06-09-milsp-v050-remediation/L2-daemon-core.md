# Task L2: Daemon core (server.go)

## Shared Context
**Goal:** Wiring server-side del index async; propagar versión real al daemon; ProtocolVersion requerido; ticker de retención; backpressure uniforme; recent_accesses default 5/opt-in.
**Stack:** Go.
**Architecture:** Worktree `C:/wt/v050-l2-daemon-core`, branch `v050/l2-daemon-core`. Único dueño de `internal/daemon/server.go` y `options.go`. Consume `StartBackgroundIndex` (L1), `PurgeAndVacuum` (L3).

## Locked Decisions
- Reemplazar `context.Background()` en el path de ejecución de index por el llamado a `StartBackgroundIndex` (L1) cuando la op sea workspace.add/init; las ops read siguen igual.
- `Version` del daemon se setea desde build info (no hardcode "dev"): leer la misma fuente que `internal/cli/version.go`.
- `ProtocolVersion` vacío ahora es inválido: rechazar con error claro (SEC-09).
- Ticker de retención cada 6h llama `PurgeAndVacuum(retentionDays, maxBytes)` (L3); default 30 días / 50MB.
- Backpressure (max_inflight) aplica también a nav.search/nav.find (PERF-10).
- `recent_accesses` default 5 (era 20); 20 solo con opt-in `--telemetry`/full (TOK-01).

## Task Metadata
```yaml
id: L2
depends_on: [T2]
agent_type: general-purpose
goal_id: G1
github_issues: []
expected_outcome: "daemon reporta versión real; status async; protocolo estricto; retención automática; backpressure uniforme; status liviano por defecto."
files:
  - modify: internal/daemon/server.go
  - modify: internal/daemon/options.go
complexity: high
done_when:
  - "go build ./... exits 0"
  - "go test ./internal/daemon/... passes"
  - "daemon status shows version != dev after rebuild"
evidence_expected:
  - ".docs/auditoria/2026-06-09-milsp-v050-remediation/L2-verdict.yaml"
stop_if:
  - "StartBackgroundIndex or PurgeAndVacuum signatures differ from T2 stubs — STOP, do not redefine them"
```

## Reference
`discovery.yaml` AUD-01(235)/AUD-02(63)/SEC-09(192)/PERF-05(91-95)/TOK-01(207). `internal/cli/version.go` para la fuente de versión.

## Prompt
Editá SOLO `server.go` y `options.go`. No definas `StartBackgroundIndex` ni `PurgeAndVacuum` (son stubs de L1/L3); solo llamalos. Cambios:
1. AUD-01: donde hoy se ejecuta el index con `context.Background()`, derivá a `StartBackgroundIndex` para workspace.add/init; mantené timeout solo en ops síncronas que lo requieran.
2. AUD-02/version: setá `Version` del daemon desde build info (misma fuente que cli/version.go), nunca "dev".
3. SEC-09: `ProtocolVersion == ""` ahora falla con error explícito.
4. PERF-05: agregá un ticker 6h que llama `store.PurgeAndVacuum(30, 50*1024*1024)`.
5. PERF-10: extendé la lista de ops con backpressure para incluir nav.search/nav.find.
6. TOK-01: `recent_accesses` default 5; flag/opción para 20.

## Execution Procedure
1. `cd C:/wt/v050-l2-daemon-core`; `git merge --no-edit main`.
2. Aplicá los 6 cambios.
3. `go build ./... && go vet ./... && go test ./internal/daemon/...`.
4. Commit. `L2-verdict.yaml`.

## Skeleton
```go
// version from build info
srv.Version = buildVersionString() // shared with cli/version.go, not "dev"
// retention ticker
go func(){ t := time.NewTicker(6*time.Hour); for range t.C { _,_,_ = store.PurgeAndVacuum(retentionDays, maxBytes) } }()
// strict protocol
if req.ProtocolVersion == "" || req.ProtocolVersion != model.ProtocolVersion { return protocolMismatchErr(req.ProtocolVersion) }
```

## Verify
`go test ./internal/daemon/...` → PASS

## Commit
`feat(daemon): async index wiring, real version, strict protocol, retention ticker, uniform backpressure (AUD-01/02 SEC-09 PERF-05/10 TOK-01)`
