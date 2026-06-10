# Task L5: Daemon runtime (Roslyn timeout, memoria, watcher)

## Shared Context
**Goal:** Timeout per-call al worker + caché de fallo de solution; política de reclamo de memoria; cap de dirs por watcher; debounce con batching.
**Stack:** Go, fsnotify.
**Architecture:** Worktree `C:/wt/v050-l5-daemon-runtime`, branch `v050/l5-daemon-runtime`. Único dueño de `lifecycle.go` y `file_watcher.go`.

## Locked Decisions
- `Manager.Call` envuelve el ctx con `context.WithTimeout(ctx, callTimeout)` (default 30s, `MI_LSP_WORKER_CALL_TIMEOUT_SECONDS`).
- Fallo de solution-load se cachea por `runtime_key` con TTL (ej. 5min): la próxima query devuelve el error inmediato con hint, sin re-spawnear MSBuild 70s.
- Memoria: si `private_bytes` supera umbral (`MI_LSP_DAEMON_SOFT_MEMORY_MB`, default 500), reducir idle timeout y disparar `runtime.GC()`; loguear warning.
- Watcher: cap por-watcher de dirs (`MI_LSP_WATCHER_MAX_DIRS`, default 10000); batching de eventos por ventana en vez de un timer por archivo.

## Task Metadata
```yaml
id: L5
depends_on: [T2]
agent_type: general-purpose
goal_id: G1
github_issues: []
expected_outcome: "nav.refs contra solution rota falla <=30s y cachea; memoria acotada; watcher con cap y batching."
files:
  - modify: internal/daemon/lifecycle.go
  - modify: internal/daemon/file_watcher.go
complexity: high
done_when:
  - "go build ./... exits 0"
  - "go test ./internal/daemon/... passes"
  - "a test asserts Manager.Call honors a context timeout and caches a failed solution load"
evidence_expected:
  - ".docs/auditoria/2026-06-09-milsp-v050-remediation/L5-verdict.yaml"
stop_if:
  - "failure cache would also cache transient errors (network blips) — only cache deterministic solution-config failures"
```

## Reference
`discovery.yaml` AUD-04(73-84)/PERF-07/PERF-08. La auditoría: 2 nav.refs de 70s c/u por el mismo `.sln` roto.

## Prompt
Editá SOLO `lifecycle.go` y `file_watcher.go`. Cambios:
1. AUD-04: en `Manager.Call`, envolvé el contexto con `context.WithTimeout` (default 30s configurable). Cacheá el fallo de carga de solution por `runtime_key` con TTL; distinguí fallos determinísticos (config de solution, proyecto duplicado) de transitorios (solo cacheá los determinísticos).
2. PERF-07: en el loop de reap/idle, si la memoria privada supera el umbral soft, bajá idle timeout y llamá `runtime.GC()`, con warning logueado.
3. PERF-08: cap de dirs por watcher; al exceder, no agregar más y loguear; batching de eventos por ventana (coalescing) en vez de un timer por archivo.

## Execution Procedure
1. `cd C:/wt/v050-l5-daemon-runtime`; `git merge --no-edit main`.
2. Aplicá los cambios + tests.
3. `go build ./... && go vet ./... && go test ./internal/daemon/...`.
4. Commit. `L5-verdict.yaml`.

## Skeleton
```go
func (m *Manager) Call(ctx context.Context, rk string, req Request) (Response, error) {
    if e, ok := m.failCache.get(rk); ok { return Response{}, fmt.Errorf("cached backend failure: %w", e) }
    cctx, cancel := context.WithTimeout(ctx, m.callTimeout); defer cancel()
    resp, err := m.client(rk).Call(cctx, req)
    if err != nil && isDeterministicSolutionFailure(err) { m.failCache.set(rk, err, 5*time.Minute) }
    return resp, err
}
```

## Verify
`go test ./internal/daemon/...` → PASS

## Commit
`feat(daemon): per-call worker timeout + failure cache, memory reclaim, watcher cap+batching (AUD-04 PERF-07/08)`
