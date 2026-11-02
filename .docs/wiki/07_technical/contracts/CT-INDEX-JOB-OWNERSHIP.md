---
wiki_contract: WIKI-CONTRACT-v1
id: CT-INDEX-JOB-OWNERSHIP
kind: contract
status: active
title: CT-INDEX-JOB-OWNERSHIP — Contrato de ownership y fencing
links:
  supports:
  - RF-IDX-001
  - RF-IDX-003
  - RF-IDX-004
---

```yaml
harness_protocol: SDD-HARNESS-v1
id: CT-INDEX-JOB-OWNERSHIP
kind: data-lifecycle-contract
audience: dual
imports: [.docs/wiki/07_technical/TECH-GRAPH-NATIVE.md]
exports: [CT-INDEX-JOB-OWNERSHIP]
agent_must_read: [.docs/wiki/07_technical/contracts/CT-INDEX-JOB-OWNERSHIP.md, internal/store/index_jobs.go, internal/indexer/indexer.go]
agent_may_edit: [.docs/wiki/07_technical/contracts/CT-INDEX-JOB-OWNERSHIP.md]
agent_must_not_edit: [internal/store/, internal/indexer/, .mi-lsp/index.db]
verify: ["Leer .docs/wiki/07_technical/contracts/CT-INDEX-JOB-OWNERSHIP.md y .docs/wiki/07_technical/TECH-GRAPH-NATIVE.md; cotejar las afirmaciones con la fuente publicada internal/store/index_jobs.go y sus límites. Registrar únicamente comandos y resultados efectivamente ejecutados; esta especificación no afirma una ejecución."]
stop_if: [Un writer stale/cancelado pueda publicar, o un control-plane adquiera owner/fence/capacidad de publicación.]
evidence: [.docs/wiki/07_technical/contracts/CT-INDEX-JOB-OWNERSHIP.md, .docs/wiki/07_technical/TECH-GRAPH-NATIVE.md, internal/store/index_jobs.go]
```

## Consumidor

CLI, agentes y consumidores locales de las operaciones descritas; no se amplía la autoridad del consumidor.

## Operaciones

Se conservan requests, envelopes, campos, límites y semántica detallados a continuación.

Enlaces relacionados: [[RF-IDX-004]] · [[TP-IDX-OWNERSHIP]] · [[RF-IDX-001]] · [[FL-IDX-01]]

### 1. Reserva y fence

- **Marcador estable**: CT-INDEX-JOB-OWNERSHIP.reserve-fence
- **Comprobación prevista**:
  - go test ./internal/store -run 'Test(CreateIndexJobConcurrentStartsReserveOneWorkspace|IndexJobStaleFenceCannotWinAfterTerminalRelease|OwnerBoundIndexJobMutationsRequireExplicitFence|IndexJobWrongOwnerFenceCannotTransition)'
- **Evidencia referenciada**:
  - internal/store/index_jobs.go
  - internal/store/index_jobs.go
- **Registros**:
  - **id**: CT-IDX-OWNERSHIP-RESERVE-001
  - **type**: operation
  - **name**: reserve
  - **input**: workspace_root, workspace_name, mode, clean
  - **atomicity**: create index_job, index_generation and index_job_ownership in one SQLite transaction
  - **uniqueness**: index_job_ownership.workspace_root is unique
  - **output**: queued job or ActiveIndexJobError
  - **id**: CT-IDX-OWNERSHIP-OWNER-001
  - **type**: record
  - **fields**: workspace_root, job_id, owner_token, fencing_token, pid, acquired_at, released_at
  - **fencing**: monotonically increasing per workspace and never reused after release
  - **id**: CT-IDX-OWNERSHIP-TRANSITION-001
  - **type**: operation
  - **guard**: expected status plus requested_cancel predicate plus active ownership by job_id plus RowsAffected=1
  - **stale_result**: ErrStaleIndexJobOwner
  - **id**: CT-IDX-OWNERSHIP-LEGACY-MIGRATION-001
  - **type**: migration
  - **guard**: IF NOT EXISTS and additive schema/data migration
  - **Oráculo**: concurrent legacy migration preserves existing jobs/generations and converges to one owner row

### 2. Control-plane, cancelación y capacidades

- **Marcador estable**: CT-INDEX-JOB-OWNERSHIP.control-plane
- **Comprobación prevista**:
  - go test ./internal/store -run 'Test(CancelIndexJobForceTimeoutRetainsReservation|MarkIndexJobFailedRejectsRequestedCancellationAndKeepsOwnership|IndexJobCancellationWinsOverStaleFailureWriter|CancelIndexJobForceTerminatesProcessAndMarksCanceled)'
  - go test ./internal/indexer -run 'TestCatalogIndexProgressReportsAndCanCancel'
- **Evidencia referenciada**:
  - internal/store/index_jobs.go
  - internal/store/index_jobs.go
  - internal/indexer/indexer.go
- **Registros**:
  - **id**: CT-IDX-CONTROL-PLANE-001
  - **type**: operation
  - **name**: request-cancel
  - **input**: job_id only
  - **guard**: CAS against active job with ownership vigente
  - **output**: requested_cancel=1; never returns or adopts a publication fence
  - **id**: CT-IDX-CONTROL-PLANE-002
  - **type**: state-machine
  - **name**: cancel
  - **states**: queued -> cancel_requested -> canceled; running -> cancel_requested -> terminating|canceled
  - **rule**: cancel_requested is never terminal
  - **id**: CT-IDX-CONTROL-PLANE-003
  - **type**: capability
  - **name**: clear
  - **memory**: clear publish and terminal capabilities as soon as ownership is stale, canceled, or released
  - **control_plane**: retain only status and cancel CAS; never mint owner_token or fencing_token
  - **id**: CT-IDX-CONTROL-PLANE-004
  - **type**: process
  - **name**: pid0
  - **modern**: PID0 is unknown/not-verifiable and cannot be treated as a live-process proof
  - **legacy**: PID0 or missing PID in legacy rows is recoverable only after conservative stale evidence
  - **id**: CT-IDX-CONTROL-PLANE-005
  - **type**: cancellation
  - **name**: force-timeout
  - **Oráculo**: if waitForProcessExit does not confirm exit, phase=terminating, reservation and lock remain
  - **id**: CT-IDX-CONTROL-PLANE-006
  - **type**: race
  - **name**: cancellation-publication
  - **Oráculo**: canceled or succeeded is the single terminal winner; a valid cancellation race never ends failed

### 3. Publicación y estado runtime

- **Marcador estable**: CT-INDEX-JOB-OWNERSHIP.publication
- **Comprobación prevista**:
  - go test ./internal/store -run 'Test(ReplaceWorkspaceIndexPublishesGenerationMetadata|StageGraphGenerationIsAtomicAndInitiallyInvisible|PublishGraphObservationBatchesIdempotentPathPreservesPointerCAS)'
  - go test ./internal/indexer -run 'Test(FullIndexPublishesLocalGoGraph|IncrementalIndexPublishesNewGraphGenerationAfterFileChange|IncrementalIndexObservationFailureLeavesGraphStale)'
  - go test ./internal/service -run 'TestIndexStartNoChangeGraphRepairPreservesCatalogBinding|TestIndexStartPopulatesWikiChunkEmbeddings|TestRecall_.*Embedding'
- **Evidencia referenciada**:
  - internal/store/index_publish.go
  - internal/store/index_publish.go
  - internal/indexer/graph_pipeline.go
  - internal/indexer/incremental.go
  - internal/service/index_jobs.go
  - internal/service/recall.go
- **Registros**:
  - **id**: CT-IDX-PUBLISH-001
  - **type**: mode
  - **modes**: full, docs, catalog, graph, incremental
  - **rule**: each mode fences its candidate generation and publishes only its declared surfaces
  - **id**: CT-IDX-PUBLISH-002
  - **type**: atomicity
  - **rule**: publication and terminal transition commit atomically; a failed commit leaves prior pointers and runtime metadata unchanged
  - **id**: CT-IDX-PUBLISH-003
  - **type**: fencing
  - **rule**: stale or canceled writer cannot change active_*_generation_id, memory_pointer, runtime metadata, files, symbols, or docs
  - **id**: CT-IDX-PUBLISH-004
  - **type**: embeddings
  - **rule**: embeddings run post-publication as best-effort and cannot roll back a valid publication or turn it into failed
  - **id**: CT-IDX-PUBLISH-005
  - **type**: terminal
  - **rule**: terminal update requires expected state, active owner fence and RowsAffected=1; zero rows is a stale rejection
  - **id**: CT-IDX-PUBLISH-006
  - **type**: no-op
  - **rule**: owner-bound full-to-incremental completion with no file or document mutations marks its candidate generation skipped and never rewrites the active catalog pointer; it preserves a valid graph binding or activates only an explicitly repaired graph snapshot under the same fence, while cancellation, failure, and fence loss remain non-publishing outcomes
  - **id**: CT-IDX-PUBLISH-007
  - **type**: readiness
  - **name**: catalog-generation-snapshot
  - **source**: active_catalog_generation_id OR atomic ReplaceCatalog metadata indexed_at + total_files
  - **rule**: WorkspaceCatalogReady accepts an active versioned catalog pointer or the complete metadata pair written by replaceCatalogTx (including total_files=0); schema-only and partial metadata are not ready, and readiness never inspects row counts. Without either signal nav.find returns index_not_ready; a workspace DB open/read failure returns the existing workspace_db_open_failed error. A transient unregistered root is readable only when a real catalog publication is ready.

### 4. Watcher y lock interproceso

- **Marcador estable**: CT-INDEX-JOB-OWNERSHIP.watcher-lock
- **Comprobación prevista**:
  - go test ./internal/daemon -run 'TestWatcherBatchRetry(UsesThreeImmediateRetries|StopsAfterOneDeferredFailure|NewEventRearmsExhaustedCycle)'
  - go test ./internal/store -run 'Test(WithWorkspaceIndexLockRejectsConcurrentIndexRun|WithWorkspaceIndexLockRemovesStaleLock|AcquireWithTimeoutReportsContentionAndPreservesLiveLock|WorkspaceIndexLockDoesNotRemoveReplacementOwner|WorkspaceIndexLockCleanupPreservesReplacementClaimedAfterQuarantine|RemoveWorkspaceIndexLockForOwnerPreservesReplacementAfterQuarantine|RemoveWorkspaceIndexLockForPIDNeverRemovesLiveProcess)'
- **Evidencia referenciada**:
  - internal/daemon/file_watcher.go
  - internal/daemon/file_watcher.go
  - internal/store/index_lock.go
  - internal/store/index_lock.go
- **Registros**:
  - **id**: CT-IDX-WATCHER-001
  - **type**: interprocess-barrier
  - **path**: <workspace>/.mi-lsp/index.lock
  - **owners**: index job and file watcher
  - **rule**: watcher contends on the same lock and never writes symbols in parallel with an index job
  - **id**: CT-IDX-WATCHER-002
  - **type**: retry
  - **rule**: three immediate attempts plus one deferred attempt per coalesced event, with a new event rearming exactly one cycle and no timer storm
  - **id**: CT-IDX-WATCHER-003
  - **type**: quarantine
  - **rule**: stale lock is quarantined before replacement; cleanup compares PID, owner_token and started_at
  - **id**: CT-IDX-WATCHER-004
  - **type**: owner-bound-release
  - **rule**: an exiting callback cannot remove a lock whose owner token, PID or started_at belongs to a replacement owner

## Errores

Se mantienen las razones, warnings, rechazos y fallbacks explícitos de cada operación.

## Compatibilidad

La migración cambia el contrato documental, no el protocolo runtime ni sus versiones. Los esquemas legacy que la herramienta consume continúan como compatibilidad de entrada, no como formato de autoría de esta wiki.
