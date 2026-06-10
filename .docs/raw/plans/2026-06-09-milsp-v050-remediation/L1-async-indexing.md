# Task L1: Async-first indexing engine

## Shared Context
**Goal:** workspace.add/init devuelve <2s indexando en background; usar incremental git-aware; lock con timeout; path canon; publicar memory_pointer atómico.
**Stack:** Go, SQLite, git.
**Architecture:** Worktree `C:/wt/v050-l1-async-indexing`, branch `v050/l1-async-indexing`. Implementa los stubs de `internal/indexer/job.go` creados en T2.

## Locked Decisions
- `StartBackgroundIndex` arranca una goroutine con `context.WithTimeout` por etapa (default 5min, configurable `MI_LSP_INDEX_TIMEOUT`); devuelve jobID inmediato. El estado se persiste para que `IndexJobStatus` y `workspace.status` lo lean.
- `workspace.add`/`init` por defecto async; conservar `--no-index` y agregar `--wait` para el comportamiento bloqueante anterior.
- Auto-index usa incremental por git-diff cuando hay `.git` y existe índice previo; full solo en primer index o sin git.
- Lock de índice (`index_lock.go`) gana `AcquireWithTimeout(d time.Duration)`; el auto-index usa 30s y degrada a "ya en progreso" en vez de bloquear.
- Path canon en workspace.add: `filepath.Clean` + `filepath.Abs` + detectar segmento duplicado final; error accionable, no CreateFile crudo.

## Task Metadata
```yaml
id: L1
depends_on: [T2]
agent_type: general-purpose
goal_id: G1
github_issues: []
expected_outcome: "workspace.add async <2s; incremental en re-index; lock con timeout; path malformado da error claro."
files:
  - modify: internal/indexer/indexer.go
  - modify: internal/indexer/job.go
  - modify: internal/store/index_lock.go
  - modify: internal/store/queries_incremental.go
  - modify: internal/store/index_publish.go
  - modify: internal/service/workspace_ops.go
  - modify: internal/cli/index.go
  - modify: internal/cli/init.go
complexity: high
done_when:
  - "go build ./... exits 0"
  - "go test ./internal/indexer/... ./internal/store/... ./internal/service/... passes"
  - "workspace.add on a large dir returns status=indexing before 5s (manual smoke noted in verdict)"
evidence_expected:
  - ".docs/auditoria/2026-06-09-milsp-v050-remediation/L1-verdict.yaml"
stop_if:
  - "cancelling an in-flight index leaves index.db partially written (must be atomic/rollback)"
  - "a file outside the owned set needs editing — STOP and report cross-lane coupling"
```

## Reference
`discovery.yaml` AUD-01/AUD-05/index-lock líneas. `internal/store/queries_incremental.go` (lógica git-diff ya existente, hoy solo usada por nav.diff-context).

## Prompt
Implementá el indexado async-first SOLO en los archivos de tu set. Workspace mi-lsp con `mi-lsp nav refs/context` para entender callers antes de cambiar firmas. No toques `internal/daemon/server.go` (es de L2): exponé `StartBackgroundIndex`/`IndexJobStatus` y dejá que L2 los llame; el wiring server-side es de L2.
Cambios:
1. `job.go`: implementá `StartBackgroundIndex` (goroutine + `context.WithTimeout` por etapa + persistencia de estado del job vía store) y `IndexJobStatus`.
2. `workspace_ops.go`: `workspace.add/init` llama `StartBackgroundIndex` por defecto; respeta `--no-index` y `--wait`. Agregá canonicalización de path con detección de segmento duplicado y error accionable (AUD-05). Publicá `memory_pointer` atómicamente con el índice (AUD-07): mové la escritura de memory snapshot a la misma transacción/commit que `index_publish`.
3. `index_lock.go`: agregá `AcquireWithTimeout`; auto-index usa 30s.
4. `indexer.go`: en auto-index, si hay `.git` e índice previo, llamá el camino incremental de `queries_incremental.go`; si no, full.
5. `cli/index.go`, `cli/init.go`: flags `--wait` (bloqueante) y mantené `--no-index`.

## Execution Procedure
1. `cd C:/wt/v050-l1-async-indexing`; `git merge --no-edit main` (trae schema T2).
2. Implementá los 8 archivos del set.
3. `go build ./... && go vet ./... && go test ./internal/indexer/... ./internal/store/... ./internal/service/...`.
4. Smoke manual: indexá un dir grande, confirmá retorno <5s con estado.
5. Commit. Escribí `L1-verdict.yaml` (verdict, comandos+exit, residual risk).

## Skeleton
```go
// funcs de PAQUETE (no receiver Engine). Registro de jobs a nivel paquete con mutex.
var jobs = newJobRegistry()
func StartBackgroundIndex(ctx context.Context, root string, clean bool, mode IndexMode) (string, error) {
    jobID := newJobID(root)
    jobs.set(jobID, IndexJobState{JobID: jobID, Phase: "queued"})
    go func() {
        ic, cancel := context.WithTimeout(context.WithoutCancel(ctx), indexTimeout())
        defer cancel()
        var err error
        if mode == IndexModeIncremental { _, err = IndexWorkspace(ic, root, false) /* incremental path */ } else { _, err = IndexWorkspace(ic, root, clean) }
        jobs.finish(jobID, err)
    }()
    return jobID, nil
}
```
> NOTA discovery: el cuerpo real reusa `indexer.IndexWorkspace(ctx, root, clean)` (indexer.go:37), etapando con chequeos `ic.Err()`. `workspace_ops.go:56-58` hoy llama `indexer.IndexWorkspace` dentro de `WithWorkspaceIndexLock`; cambialo para usar `StartBackgroundIndex` por defecto.

## Verify
`go test ./internal/indexer/... ./internal/store/... ./internal/service/...` → PASS

## Commit
`feat(index): async-first background indexing with incremental, lock timeout, path canon (AUD-01/05/07)`
