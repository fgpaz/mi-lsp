# RF-IDX-004 — Ownership y fencing de index jobs

```yaml
harness_protocol: SDD-HARNESS-v1
source_protocol: SDD-WIKI-SOURCE-v1
doc_id: RF-IDX-004
id: RF-IDX-004
kind: requirement
audience: llm-first
imports:
  - '[[00_gobierno_documental]]'
  - '[[FL-IDX-01]]'
  - '[[RF-IDX-001]]'
  - '[[CT-INDEX-JOB-OWNERSHIP]]'
  - '[[TP-IDX-OWNERSHIP]]'
exports:
  - RF-IDX-004
  - RF-IDX-004.ownership
  - RF-IDX-004.cancellation
  - RF-IDX-004.publication
  - RF-IDX-004.watcher-locks
  - RF-IDX-004.ownership.single-reservation
  - RF-IDX-004.cancellation.cancel-publish-race
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/03_FL/FL-IDX-01.md
  - .docs/wiki/04_RF/RF-IDX-001.md
  - .docs/wiki/09_contratos/CT-INDEX-JOB-OWNERSHIP.md
  - .docs/wiki/06_pruebas/TP-IDX-OWNERSHIP.md
agent_may_edit:
  - .docs/wiki/04_RF/RF-IDX-004.md
agent_must_not_edit:
  - .docs/wiki/04_RF/RF-IDX-003.md
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - MI_LSP_CLIENT_NAME=batch-a-doc-writer MI_LSP_SESSION_ID=2026-07-30-index-safety-probe go run ./cmd/mi-lsp nav wiki validate-harness --workspace . --ids RF-IDX-004 --format toon --no-daemon
  - MI_LSP_CLIENT_NAME=batch-a-doc-writer MI_LSP_SESSION_ID=2026-07-30-index-safety-probe go run ./cmd/mi-lsp nav wiki validate-source --workspace . --ids RF-IDX-004 --format toon --no-daemon
  - go test ./internal/store ./internal/service ./internal/daemon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
  - wiki_source_verdict=BLOCKED
  - terminal_transition_without_rows_affected=true
  - stale_owner_can_commit_terminal=true
  - live_process_lock_removed=true
evidence:
  - .docs/wiki/04_RF/RF-IDX-004.md
  - .docs/wiki/09_contratos/CT-INDEX-JOB-OWNERSHIP.md
  - .docs/wiki/06_pruebas/TP-IDX-OWNERSHIP.md
  - internal/store/index_jobs_ownership_test.go
  - internal/store/index_lock_test.go
  - internal/store/store_test.go
  - internal/service/index_jobs_test.go
  - internal/daemon/file_watcher_test.go
```

Enlaces relacionados: [[RF-IDX-001]] · [[CT-INDEX-JOB-OWNERSHIP]] · [[TP-IDX-OWNERSHIP]] · [[FL-IDX-01]]

## 1. Ownership, capacidad y compatibilidad de procesos

```toon
harness_protocol: SDD-HARNESS-v1
source_protocol: SDD-WIKI-SOURCE-v1
doc_id: RF-IDX-004
block_id: RF-IDX-004.ownership
kind: normative-requirement
source_of_truth: this fenced toon block
imports:
  - .docs/wiki/03_FL/FL-IDX-01.md
  - .docs/wiki/04_RF/RF-IDX-001.md
  - .docs/wiki/09_contratos/CT-INDEX-JOB-OWNERSHIP.md
exports:
  - RF-IDX-004.ownership
verify:
  - go test ./internal/store -run 'Test(CreateIndexJobConcurrentStartsReserveOneWorkspace|OwnerBoundIndexJobMutationsRequireExplicitFence|IndexJobWrongOwnerFenceCannotTransition|IndexJobReadCompatibilityWithoutRequestedCancelColumn)'
  - go test ./internal/store -run 'Test(RemoveWorkspaceIndexLockForPIDNeverRemovesLiveProcess)'
evidence:
  - internal/store/index_jobs_ownership_test.go
  - internal/store/index_lock_test.go
records:
  - id: RF-IDX-004.ownership.single-reservation
    type: invariant
    statement: un workspace_root solo puede tener una reserva mutante; la creación del job, la generación candidata y la fila de ownership ocurren en una transacción SQLite.
    oracle: dos starts concurrentes producen un único job reservado y un único ActiveIndexJobError; nunca quedan dos reservas activas.
  - id: RF-IDX-004.ownership.monotonic-fence
    type: invariant
    statement: cada reserva conserva owner_token aleatorio y fencing_token monotónico por workspace; un token liberado no se reutiliza.
    oracle: una sustitución solo pasa después de un CAS de liberación y el nuevo fencing_token es mayor que el anterior.
  - id: RF-IDX-004.ownership.capability-clearing
    type: invariant
    statement: capability clearing elimina capacidades de ejecución en memoria y en control-plane cuando ownership deja de estar vigente; el control-plane no puede convertir una solicitud externa en owner.
    oracle: tras clearing, el worker pierde las capacidades publish/terminal y el control-plane solo puede marcar requested_cancel mediante CAS.
  - id: RF-IDX-004.ownership.pid0-modern
    type: compatibility
    statement: PID 0 se interpreta con la semántica moderna de proceso desconocido/no verificable y no se presume vivo ni muerto sin evidencia de plataforma.
    oracle: una fila PID0 moderna se conserva para inspección segura y no permite retirar un lock vivo por heurística.
  - id: RF-IDX-004.ownership.pid0-legacy
    type: compatibility
    statement: las filas legacy con PID0 o formato antiguo se normalizan de forma aditiva; ningún job o generación existente se elimina o mueve durante la migración.
    oracle: una base legacy queda legible, su capacidad se limita a status/recovery y solo se reserva después de stale confirmado.
  - id: RF-IDX-004.ownership.legacy-concurrent-migration
    type: concurrency
    statement: la migración legacy concurrente usa CAS/unique workspace_root y no puede crear dos ownership activos ni perder el fence vencedor.
    oracle: N migraciones concurrentes convergen en una fila vigente y todas las perdedoras devuelven conflicto recuperable.
```

## 2. Cancelación y carrera cancelación/publicación

```toon
harness_protocol: SDD-HARNESS-v1
source_protocol: SDD-WIKI-SOURCE-v1
doc_id: RF-IDX-004
block_id: RF-IDX-004.cancellation
kind: normative-requirement
source_of_truth: this fenced toon block
imports:
  - .docs/wiki/04_RF/RF-IDX-001.md
  - .docs/wiki/09_contratos/CT-INDEX-JOB-OWNERSHIP.md
  - .docs/wiki/06_pruebas/TP-IDX-OWNERSHIP.md
exports:
  - RF-IDX-004.cancellation
verify:
  - go test ./internal/store -run 'Test(CancelIndexJobForceTimeoutRetainsReservation|MarkIndexJobFailedRejectsRequestedCancellationAndKeepsOwnership|IndexJobCancellationWinsOverStaleFailureWriter)'
  - go test ./internal/indexer -run 'TestCatalogIndexProgressReportsAndCanCancel'
evidence:
  - internal/store/index_jobs_ownership_test.go
  - internal/store/index_jobs_test.go
  - internal/indexer/indexer_progress_test.go
records:
  - id: RF-IDX-004.cancellation.queued-immediate
    type: state-machine
    statement: una cancelación de un job queued pasa inmediatamente a canceled sin spawn; la reserva se libera solo con el terminal owner-bound.
    oracle: no aparece proceso worker, el lock no se elimina antes de la transición válida y el job no queda failed.
  - id: RF-IDX-004.cancellation.running-cooperative
    type: state-machine
    statement: la cancelación cooperativa escribe requested_cancel y el worker confirma canceled solo después de observar la solicitud y alcanzar una frontera sin publicación.
    oracle: requested_cancel no es terminal y el estado final es canceled con un único ganador.
  - id: RF-IDX-004.cancellation.force-timeout
    type: state-machine
    statement: un force cancel que no confirma la salida del PID conserva requested_cancel, fase terminating, reserva y lock.
    oracle: el timeout devuelve resultado operativo de terminación y no libera ownership ni marca failed.
  - id: RF-IDX-004.cancellation.stale-no-pointers
    type: fencing
    statement: un fence stale o canceled no puede escribir terminal, runtime, active generation ni memory pointer.
    oracle: RowsAffected es cero o ErrStaleIndexJobOwner y todos los punteros y runtime conservan el snapshot vencedor.
  - id: RF-IDX-004.cancellation.cancel-publish-race
    type: concurrency
    statement: la carrera entre cancelación y publicación termina en canceled o succeeded según el primer commit owner-bound; nunca termina failed por una cancelación válida.
    oracle: existe exactamente un terminal inmutable, no hay publicación parcial y el job no se clasifica failed por perder la carrera de cancelación.
```

## 3. Publicación, fencing y embeddings

```toon
harness_protocol: SDD-HARNESS-v1
source_protocol: SDD-WIKI-SOURCE-v1
doc_id: RF-IDX-004
block_id: RF-IDX-004.publication
kind: normative-requirement
source_of_truth: this fenced toon block
imports:
  - .docs/wiki/04_RF/RF-IDX-001.md
  - .docs/wiki/08_db/DB-DOC-INDEX.md
  - .docs/wiki/06_pruebas/TP-IDX-OWNERSHIP.md
exports:
  - RF-IDX-004.publication
verify:
  - go test ./internal/store -run 'Test(ReplaceWorkspaceIndexPublishesGenerationMetadata|StageGraphGenerationIsAtomicAndInitiallyInvisible|PublishGraphObservationBatchesIdempotentPathPreservesPointerCAS)'
  - go test ./internal/indexer -run 'Test(IncrementalIndexPublishesNewGraphGenerationAfterFileChange|IncrementalIndexObservationFailureLeavesGraphStale)'
  - go test ./internal/service -run 'TestIndexStartPopulatesWikiChunkEmbeddings|TestRecall_.*Embedding'
evidence:
  - internal/store/store_test.go
  - internal/store/graph_store_test.go
  - internal/indexer/incremental_test.go
  - internal/service/recall_test.go
records:
  - id: RF-IDX-004.publication.fenced-modes
    type: publication
    statement: full, docs, catalog, graph e incremental construyen una generación candidata y publican solo con el fence vigente del job.
    oracle: cada modo actualiza únicamente sus superficies declaradas y no activa una generación de otro modo.
  - id: RF-IDX-004.publication.atomic-terminal
    type: atomicity
    statement: la publicación y la transición terminal son una unidad durable; si falla el commit, los punteros y el runtime permanecen en el snapshot previo.
    oracle: nunca se observa pointer nuevo con job failed/canceled ni job succeeded sin generación activa válida.
  - id: RF-IDX-004.publication.stale-canceled
    type: fencing
    statement: una publicación stale o cancelada no modifica active_catalog_generation_id, active_docs_generation_id, active_graph_generation_id, memory_pointer ni runtime metadata.
    oracle: el snapshot anterior es byte/valor estable después del intento rechazado.
  - id: RF-IDX-004.publication.embeddings-best-effort
    type: post-publication
    statement: embeddings se ejecutan después de publicar y son best-effort; su timeout o indisponibilidad no revierte una publicación válida ni convierte el job en failed.
    oracle: el job sigue succeeded, se registra warning sanitizado y los embeddings incompletos no se presentan como completos.
```

## 4. Watcher y lock interproceso

```toon
harness_protocol: SDD-HARNESS-v1
source_protocol: SDD-WIKI-SOURCE-v1
doc_id: RF-IDX-004
block_id: RF-IDX-004.watcher-locks
kind: normative-requirement
source_of_truth: this fenced toon block
imports:
  - .docs/wiki/09_contratos/CT-INDEX-JOB-OWNERSHIP.md
  - .docs/wiki/06_pruebas/TP-IDX-OWNERSHIP.md
exports:
  - RF-IDX-004.watcher-locks
verify:
  - go test ./internal/daemon -run 'TestWatcherBatchRetry(UsesThreeImmediateRetries|StopsAfterOneDeferredFailure|NewEventRearmsExhaustedCycle)'
  - go test ./internal/store -run 'Test(WithWorkspaceIndexLockRejectsConcurrentIndexRun|AcquireWithTimeoutReportsContentionAndPreservesLiveLock|WorkspaceIndexLockDoesNotRemoveReplacementOwner|WorkspaceIndexLockCleanupPreservesReplacementClaimedAfterQuarantine|RemoveWorkspaceIndexLockForOwnerPreservesReplacementAfterQuarantine)'
evidence:
  - internal/daemon/file_watcher_test.go
  - internal/store/index_lock_test.go
records:
  - id: RF-IDX-004.watcher-locks.retry-budget
    type: watcher
    statement: un batch del watcher realiza tres reintentos inmediatos y como máximo una reintentada diferida; un evento nuevo rearma el ciclo sin storm.
    oracle: cuatro intentos totales por ciclo, una sola diferida y ningún timer duplicado para el mismo evento coalescido.
  - id: RF-IDX-004.watcher-locks.quarantine
    type: lock
    statement: un lock stale se pone en quarantine antes de reclamarlo; la limpieza compara PID, owner_token y started_at y nunca retira un owner reemplazado.
    oracle: el owner replacement sobrevive a cualquier callback stale, incluso después de quarantine.
  - id: RF-IDX-004.watcher-locks.pid0-safety
    type: lock
    statement: RemoveWorkspaceIndexLockForPID nunca retira un PID vivo y trata PID0 moderno y legacy como no verificables hasta contar con evidencia segura.
    oracle: un PID0 o PID vivo conserva el lock y devuelve contención/diagnóstico, no eliminación.
```
