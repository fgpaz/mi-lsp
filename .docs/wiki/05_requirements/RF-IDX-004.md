---
wiki_contract: WIKI-CONTRACT-v1
id: RF-IDX-004
kind: requirement
status: active
title: RF-IDX-004 — Ownership y fencing de index jobs
links:
  implements:
  - FL-IDX-01
  code:
  - path: internal/daemon/file_watcher.go
    role: behavior
  - path: internal/indexer/incremental.go
    role: behavior
  - path: internal/indexer/indexer.go
    role: behavior
  - path: internal/service/index_jobs.go
    role: behavior
  - path: internal/service/recall.go
    role: behavior
  - path: internal/store/index_jobs.go
    role: behavior
  - path: internal/store/index_lock.go
    role: behavior
  - path: internal/store/index_publish.go
    role: behavior
---

## Propósito

RF-IDX-004 — Ownership y fencing de index jobs

## Comportamiento ordenado

Enlaces relacionados: [[RF-IDX-001]] · [[CT-INDEX-JOB-OWNERSHIP]] · [[TP-IDX-OWNERSHIP]] · [[FL-IDX-01]]

### 1. Ownership, capacidad y compatibilidad de procesos

- **Marcador estable**: RF-IDX-004.ownership
- **Comprobación prevista**:
  - go test ./internal/store -run 'Test(CreateIndexJobConcurrentStartsReserveOneWorkspace|OwnerBoundIndexJobMutationsRequireExplicitFence|IndexJobWrongOwnerFenceCannotTransition|IndexJobReadCompatibilityWithoutRequestedCancelColumn)'
  - go test ./internal/store -run 'Test(RemoveWorkspaceIndexLockForPIDNeverRemovesLiveProcess)'
- **Evidencia referenciada**:
  - internal/store/index_jobs.go
  - internal/store/index_lock.go
- **Registros**:
  - **id**: RF-IDX-004.ownership.single-reservation
  - **type**: invariant
  - **Regla**: un workspace_root solo puede tener una reserva mutante; la creación del job, la generación candidata y la fila de ownership ocurren en una transacción SQLite.
  - **Oráculo**: dos starts concurrentes producen un único job reservado y un único ActiveIndexJobError; nunca quedan dos reservas activas.
  - **id**: RF-IDX-004.ownership.monotonic-fence
  - **type**: invariant
  - **Regla**: cada reserva conserva owner_token aleatorio y fencing_token monotónico por workspace; un token liberado no se reutiliza.
  - **Oráculo**: una sustitución solo pasa después de un CAS de liberación y el nuevo fencing_token es mayor que el anterior.
  - **id**: RF-IDX-004.ownership.capability-clearing
  - **type**: invariant
  - **Regla**: capability clearing elimina capacidades de ejecución en memoria y en control-plane cuando ownership deja de estar vigente; el control-plane no puede convertir una solicitud externa en owner.
  - **Oráculo**: tras clearing, el worker pierde las capacidades publish/terminal y el control-plane solo puede marcar requested_cancel mediante CAS.
  - **id**: RF-IDX-004.ownership.pid0-modern
  - **type**: compatibility
  - **Regla**: PID 0 se interpreta con la semántica moderna de proceso desconocido/no verificable y no se presume vivo ni muerto sin evidencia de plataforma.
  - **Oráculo**: una fila PID0 moderna se conserva para inspección segura y no permite retirar un lock vivo por heurística.
  - **id**: RF-IDX-004.ownership.pid0-legacy
  - **type**: compatibility
  - **Regla**: las filas legacy con PID0 o formato antiguo se normalizan de forma aditiva; ningún job o generación existente se elimina o mueve durante la migración.
  - **Oráculo**: una base legacy queda legible, su capacidad se limita a status/recovery y solo se reserva después de stale confirmado.
  - **id**: RF-IDX-004.ownership.legacy-concurrent-migration
  - **type**: concurrency
  - **Regla**: la migración legacy concurrente usa CAS/unique workspace_root y no puede crear dos ownership activos ni perder el fence vencedor.
  - **Oráculo**: N migraciones concurrentes convergen en una fila vigente y todas las perdedoras devuelven conflicto recuperable.

### 2. Cancelación y carrera cancelación/publicación

- **Marcador estable**: RF-IDX-004.cancellation
- **Comprobación prevista**:
  - go test ./internal/store -run 'Test(CancelIndexJobForceTimeoutRetainsReservation|MarkIndexJobFailedRejectsRequestedCancellationAndKeepsOwnership|IndexJobCancellationWinsOverStaleFailureWriter)'
  - go test ./internal/indexer -run 'TestCatalogIndexProgressReportsAndCanCancel'
- **Evidencia referenciada**:
  - internal/store/index_jobs.go
  - internal/store/index_jobs.go
  - internal/indexer/indexer.go
- **Registros**:
  - **id**: RF-IDX-004.cancellation.queued-immediate
  - **type**: state-machine
  - **Regla**: una cancelación de un job queued pasa inmediatamente a canceled sin spawn; la reserva se libera solo con el terminal owner-bound.
  - **Oráculo**: no aparece proceso worker, el lock no se elimina antes de la transición válida y el job no queda failed.
  - **id**: RF-IDX-004.cancellation.running-cooperative
  - **type**: state-machine
  - **Regla**: la cancelación cooperativa escribe requested_cancel y el worker confirma canceled solo después de observar la solicitud y alcanzar una frontera sin publicación.
  - **Oráculo**: requested_cancel no es terminal y el estado final es canceled con un único ganador.
  - **id**: RF-IDX-004.cancellation.force-timeout
  - **type**: state-machine
  - **Regla**: un force cancel que no confirma la salida del PID conserva requested_cancel, fase terminating, reserva y lock.
  - **Oráculo**: el timeout devuelve resultado operativo de terminación y no libera ownership ni marca failed.
  - **id**: RF-IDX-004.cancellation.stale-no-pointers
  - **type**: fencing
  - **Regla**: un fence stale o canceled no puede escribir terminal, runtime, active generation ni memory pointer.
  - **Oráculo**: RowsAffected es cero o ErrStaleIndexJobOwner y todos los punteros y runtime conservan el snapshot vencedor.
  - **id**: RF-IDX-004.cancellation.cancel-publish-race
  - **type**: concurrency
  - **Regla**: la carrera entre cancelación y publicación termina en canceled o succeeded según el primer commit owner-bound; nunca termina failed por una cancelación válida.
  - **Oráculo**: existe exactamente un terminal inmutable, no hay publicación parcial y el job no se clasifica failed por perder la carrera de cancelación.

### 3. Publicación, fencing y embeddings

- **Marcador estable**: RF-IDX-004.publication
- **Comprobación prevista**:
  - go test ./internal/store -run 'Test(ReplaceWorkspaceIndexPublishesGenerationMetadata|StageGraphGenerationIsAtomicAndInitiallyInvisible|PublishGraphObservationBatchesIdempotentPathPreservesPointerCAS)'
  - go test ./internal/indexer -run 'Test(IncrementalIndexPublishesNewGraphGenerationAfterFileChange|IncrementalIndexObservationFailureLeavesGraphStale)'
  - go test ./internal/service -run 'TestIndexStartPopulatesWikiChunkEmbeddings|TestRecall_.*Embedding'
- **Evidencia referenciada**:
  - internal/store/index_publish.go
  - internal/store/index_publish.go
  - internal/indexer/incremental.go
  - internal/service/recall.go
- **Registros**:
  - **id**: RF-IDX-004.publication.fenced-modes
  - **type**: publication
  - **Regla**: full, docs, catalog, graph e incremental construyen una generación candidata y publican solo con el fence vigente del job.
  - **Oráculo**: cada modo actualiza únicamente sus superficies declaradas y no activa una generación de otro modo.
  - **id**: RF-IDX-004.publication.atomic-terminal
  - **type**: atomicity
  - **Regla**: la publicación y la transición terminal son una unidad durable; si falla el commit, los punteros y el runtime permanecen en el snapshot previo.
  - **Oráculo**: nunca se observa pointer nuevo con job failed/canceled ni job succeeded sin generación activa válida.
  - **id**: RF-IDX-004.publication.stale-canceled
  - **type**: fencing
  - **Regla**: una publicación stale o cancelada no modifica active_catalog_generation_id, active_docs_generation_id, active_graph_generation_id, memory_pointer ni runtime metadata.
  - **Oráculo**: el snapshot anterior es byte/valor estable después del intento rechazado.
  - **id**: RF-IDX-004.publication.embeddings-best-effort
  - **type**: post-publication
  - **Regla**: embeddings se ejecutan después de publicar y son best-effort; su timeout o indisponibilidad no revierte una publicación válida ni convierte el job en failed.
  - **Oráculo**: el job sigue succeeded, se registra warning sanitizado y los embeddings incompletos no se presentan como completos.

### 4. Watcher y lock interproceso

- **Marcador estable**: RF-IDX-004.watcher-locks
- **Comprobación prevista**:
  - go test ./internal/daemon -run 'TestWatcherBatchRetry(UsesThreeImmediateRetries|StopsAfterOneDeferredFailure|NewEventRearmsExhaustedCycle)'
  - go test ./internal/store -run 'Test(WithWorkspaceIndexLockRejectsConcurrentIndexRun|AcquireWithTimeoutReportsContentionAndPreservesLiveLock|WorkspaceIndexLockDoesNotRemoveReplacementOwner|WorkspaceIndexLockCleanupPreservesReplacementClaimedAfterQuarantine|RemoveWorkspaceIndexLockForOwnerPreservesReplacementAfterQuarantine)'
- **Evidencia referenciada**:
  - internal/daemon/file_watcher.go
  - internal/store/index_lock.go
- **Registros**:
  - **id**: RF-IDX-004.watcher-locks.retry-budget
  - **type**: watcher
  - **Regla**: un batch del watcher realiza tres reintentos inmediatos y como máximo una reintentada diferida; un evento nuevo rearma el ciclo sin storm.
  - **Oráculo**: cuatro intentos totales por ciclo, una sola diferida y ningún timer duplicado para el mismo evento coalescido.
  - **id**: RF-IDX-004.watcher-locks.quarantine
  - **type**: lock
  - **Regla**: un lock stale se pone en quarantine antes de reclamarlo; la limpieza compara PID, owner_token y started_at y nunca retira un owner reemplazado.
  - **Oráculo**: el owner replacement sobrevive a cualquier callback stale, incluso después de quarantine.
  - **id**: RF-IDX-004.watcher-locks.pid0-safety
  - **type**: lock
  - **Regla**: RemoveWorkspaceIndexLockForPID nunca retira un PID vivo y trata PID0 moderno y legacy como no verificables hasta contar con evidencia segura.
  - **Oráculo**: un PID0 o PID vivo conserva el lock y devuelve contención/diagnóstico, no eliminación.

## Superficies

CLI `mi-lsp`, con operaciones y selectores detallados en el comportamiento del requisito.

## Entradas/salidas tipadas

Las entradas, salidas, enums y límites forman parte del contrato de comportamiento ordenado de este documento; los campos conservan sus nombres de protocolo.

## Errores

Se aplican las condiciones de rechazo y los warnings descritos en el comportamiento ordenado y sus variantes. No se añade ningún fallback silencioso.

## Criterios de aceptación (numerados)

1. un workspace_root solo puede tener una reserva mutante; la creación del job, la generación candidata y la fila de ownership ocurren en una transacción SQLite. Evidencia esperada: dos starts concurrentes producen un único job reservado y un único ActiveIndexJobError; nunca quedan dos reservas activas.
2. cada reserva conserva owner_token aleatorio y fencing_token monotónico por workspace; un token liberado no se reutiliza. Evidencia esperada: una sustitución solo pasa después de un CAS de liberación y el nuevo fencing_token es mayor que el anterior.
3. capability clearing elimina capacidades de ejecución en memoria y en control-plane cuando ownership deja de estar vigente; el control-plane no puede convertir una solicitud externa en owner. Evidencia esperada: tras clearing, el worker pierde las capacidades publish/terminal y el control-plane solo puede marcar requested_cancel mediante CAS.
4. PID 0 se interpreta con la semántica moderna de proceso desconocido/no verificable y no se presume vivo ni muerto sin evidencia de plataforma. Evidencia esperada: una fila PID0 moderna se conserva para inspección segura y no permite retirar un lock vivo por heurística.
5. las filas legacy con PID0 o formato antiguo se normalizan de forma aditiva; ningún job o generación existente se elimina o mueve durante la migración. Evidencia esperada: una base legacy queda legible, su capacidad se limita a status/recovery y solo se reserva después de stale confirmado.
6. la migración legacy concurrente usa CAS/unique workspace_root y no puede crear dos ownership activos ni perder el fence vencedor. Evidencia esperada: N migraciones concurrentes convergen en una fila vigente y todas las perdedoras devuelven conflicto recuperable.
7. una cancelación de un job queued pasa inmediatamente a canceled sin spawn; la reserva se libera solo con el terminal owner-bound. Evidencia esperada: no aparece proceso worker, el lock no se elimina antes de la transición válida y el job no queda failed.
8. la cancelación cooperativa escribe requested_cancel y el worker confirma canceled solo después de observar la solicitud y alcanzar una frontera sin publicación. Evidencia esperada: requested_cancel no es terminal y el estado final es canceled con un único ganador.
9. un force cancel que no confirma la salida del PID conserva requested_cancel, fase terminating, reserva y lock. Evidencia esperada: el timeout devuelve resultado operativo de terminación y no libera ownership ni marca failed.
10. un fence stale o canceled no puede escribir terminal, runtime, active generation ni memory pointer. Evidencia esperada: RowsAffected es cero o ErrStaleIndexJobOwner y todos los punteros y runtime conservan el snapshot vencedor.
11. la carrera entre cancelación y publicación termina en canceled o succeeded según el primer commit owner-bound; nunca termina failed por una cancelación válida. Evidencia esperada: existe exactamente un terminal inmutable, no hay publicación parcial y el job no se clasifica failed por perder la carrera de cancelación.
12. full, docs, catalog, graph e incremental construyen una generación candidata y publican solo con el fence vigente del job. Evidencia esperada: cada modo actualiza únicamente sus superficies declaradas y no activa una generación de otro modo.
13. la publicación y la transición terminal son una unidad durable; si falla el commit, los punteros y el runtime permanecen en el snapshot previo. Evidencia esperada: nunca se observa pointer nuevo con job failed/canceled ni job succeeded sin generación activa válida.
14. una publicación stale o cancelada no modifica active_catalog_generation_id, active_docs_generation_id, active_graph_generation_id, memory_pointer ni runtime metadata. Evidencia esperada: el snapshot anterior es byte/valor estable después del intento rechazado.
15. embeddings se ejecutan después de publicar y son best-effort; su timeout o indisponibilidad no revierte una publicación válida ni convierte el job en failed. Evidencia esperada: el job sigue succeeded, se registra warning sanitizado y los embeddings incompletos no se presentan como completos.
16. un batch del watcher realiza tres reintentos inmediatos y como máximo una reintentada diferida; un evento nuevo rearma el ciclo sin storm. Evidencia esperada: cuatro intentos totales por ciclo, una sola diferida y ningún timer duplicado para el mismo evento coalescido.
17. un lock stale se pone en quarantine antes de reclamarlo; la limpieza compara PID, owner_token y started_at y nunca retira un owner reemplazado. Evidencia esperada: el owner replacement sobrevive a cualquier callback stale, incluso después de quarantine.
18. RemoveWorkspaceIndexLockForPID nunca retira un PID vivo y trata PID0 moderno y legacy como no verificables hasta contar con evidencia segura. Evidencia esperada: un PID0 o PID vivo conserva el lock y devuelve contención/diagnóstico, no eliminación.

```yaml
harness_protocol: SDD-HARNESS-v1
id: RF-IDX-004
kind: requirement
audience: llm-first
imports:
- .docs/wiki/04_flows/FL-IDX-01.md
exports:
- RF-IDX-004
- RF-IDX-004 — Ownership y fencing de index jobs
agent_must_read:
- .docs/wiki/05_requirements/RF-IDX-004.md
- .docs/wiki/04_flows/FL-IDX-01.md
- .docs/wiki/06_tests/TP-IDX-OWNERSHIP.md
agent_may_edit:
- .docs/wiki/05_requirements/RF-IDX-004.md
agent_must_not_edit:
- .docs/wiki/00_governance.md
- AGENTS.md
- internal/daemon/file_watcher.go
- internal/indexer/incremental.go
- internal/indexer/indexer.go
- internal/service/index_jobs.go
- internal/service/recall.go
- internal/store/index_jobs.go
- internal/store/index_lock.go
- internal/store/index_publish.go
verify:
- 'Para los casos de este requisito en .docs/wiki/06_tests/TP-IDX-OWNERSHIP.md, ejecutar
  el comando publicado: go test -v -count=1 -p 1 ./internal/store -run ''^Test(MarkIndexJobProgressUpdatesCountersStageAndTimestamp|CancelIndexJobForceTimeoutRetainsReservation|CancelIndexJobForceTerminatesProcessAndMarksCanceled|AcquireWithTimeoutReportsContentionAndPreservesLiveLock|IndexJobCancellationWinsOverStaleFailureWriter)$''
  (la evidencia de ejecución debe registrarse por separado).'
stop_if:
- Detenerse si la propuesta requiere alterar criterios, decisiones o comportamiento
  más allá de RF-IDX-004.
evidence:
- .docs/wiki/05_requirements/RF-IDX-004.md
- .docs/wiki/06_tests/TP-IDX-OWNERSHIP.md
```
