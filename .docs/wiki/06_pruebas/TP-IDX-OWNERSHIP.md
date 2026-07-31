# TP-IDX-OWNERSHIP — Pruebas de ownership, fencing y exclusión

```yaml
harness_protocol: SDD-HARNESS-v1
source_protocol: SDD-WIKI-SOURCE-v1
doc_id: TP-IDX-OWNERSHIP
id: TP-IDX-OWNERSHIP
kind: test-contract
audience: llm-first
imports:
  - '[[00_gobierno_documental]]'
  - '[[FL-IDX-01]]'
  - '[[RF-IDX-001]]'
  - '[[RF-IDX-004]]'
  - '[[CT-INDEX-JOB-OWNERSHIP]]'
  - '[[TP-IDX]]'
exports:
  - TP-IDX-OWNERSHIP
  - TP-IDX-OWNERSHIP.lifecycle
  - TP-IDX-OWNERSHIP.publication
  - TP-IDX-OWNERSHIP.watcher-lock-races
  - TC-IDX-OWNERSHIP-001
  - TC-IDX-OWNERSHIP-002
  - TC-IDX-OWNERSHIP-003
  - TC-IDX-OWNERSHIP-004
  - TC-IDX-OWNERSHIP-005
  - TC-IDX-OWNERSHIP-006
  - TC-IDX-OWNERSHIP-007
  - TC-IDX-OWNERSHIP-008
  - TC-IDX-OWNERSHIP-009
  - TC-IDX-OWNERSHIP-010
  - TC-IDX-OWNERSHIP-011
  - TC-IDX-OWNERSHIP-012
  - TC-IDX-OWNERSHIP-013
  - TC-IDX-OWNERSHIP-014
  - TC-IDX-OWNERSHIP-015
  - TC-IDX-OWNERSHIP-016
  - TC-IDX-OWNERSHIP-017
  - TC-IDX-OWNERSHIP-018
  - TC-IDX-OWNERSHIP-019
  - TC-IDX-OWNERSHIP-020
  - TC-IDX-OWNERSHIP-021
  - TC-IDX-OWNERSHIP-022
  - TC-IDX-OWNERSHIP-023
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/03_FL/FL-IDX-01.md
  - .docs/wiki/04_RF/RF-IDX-001.md
  - .docs/wiki/04_RF/RF-IDX-004.md
  - .docs/wiki/09_contratos/CT-INDEX-JOB-OWNERSHIP.md
  - .docs/wiki/06_pruebas/TP-IDX.md
agent_may_edit:
  - .docs/wiki/06_pruebas/TP-IDX-OWNERSHIP.md
agent_must_not_edit:
  - .docs/wiki/04_RF/RF-IDX-003.md
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - MI_LSP_CLIENT_NAME=batch-a-doc-writer MI_LSP_SESSION_ID=2026-07-30-index-safety-probe go run ./cmd/mi-lsp nav wiki validate-harness --workspace . --ids TP-IDX-OWNERSHIP --format toon --no-daemon
  - MI_LSP_CLIENT_NAME=batch-a-doc-writer MI_LSP_SESSION_ID=2026-07-30-index-safety-probe go run ./cmd/mi-lsp nav wiki validate-source --workspace . --ids TP-IDX-OWNERSHIP --format toon --no-daemon
  - GOMAXPROCS=2 go test -count=1 -p 1 ./internal/store
  - GOMAXPROCS=2 go test -count=1 -p 1 ./internal/indexer
  - GOMAXPROCS=2 go test -count=1 -p 1 ./internal/service
  - GOMAXPROCS=2 go test -count=1 -p 1 ./internal/daemon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
  - wiki_source_verdict=BLOCKED
  - any_fault_injection_is_shared_workspace=true
  - stale_terminal_write_succeeded=true
  - live_lock_missing_after_timeout=true
evidence:
  - .docs/wiki/06_pruebas/TP-IDX-OWNERSHIP.md
  - .docs/wiki/04_RF/RF-IDX-004.md
  - .docs/wiki/09_contratos/CT-INDEX-JOB-OWNERSHIP.md
  - internal/store/index_jobs_ownership_test.go
  - internal/store/index_jobs_test.go
  - internal/store/index_lock_test.go
  - internal/store/store_test.go
  - internal/indexer/indexer_progress_test.go
  - internal/indexer/incremental_test.go
  - internal/daemon/file_watcher_test.go
  - internal/service/recall_test.go
  - internal/service/index_jobs_test.go
```

Enlaces relacionados: [[RF-IDX-004]] · [[CT-INDEX-JOB-OWNERSHIP]] · [[TP-IDX]] · [[FL-IDX-01]]

## 1. Ciclo de vida y cancelación

```toon
harness_protocol: SDD-HARNESS-v1
source_protocol: SDD-WIKI-SOURCE-v1
doc_id: TP-IDX-OWNERSHIP
block_id: TP-IDX-OWNERSHIP.lifecycle
kind: executable-test-plan
source_of_truth: this fenced toon block
imports:
  - .docs/wiki/04_RF/RF-IDX-004.md
  - .docs/wiki/09_contratos/CT-INDEX-JOB-OWNERSHIP.md
exports:
  - TP-IDX-OWNERSHIP.lifecycle
verify:
  - go test -v -count=1 -p 1 ./internal/store -run '^Test(MarkIndexJobProgressUpdatesCountersStageAndTimestamp|CancelIndexJobForceTimeoutRetainsReservation|CancelIndexJobForceTerminatesProcessAndMarksCanceled|AcquireWithTimeoutReportsContentionAndPreservesLiveLock|IndexJobCancellationWinsOverStaleFailureWriter)$'
  - go test -v -count=1 -p 1 ./internal/service -run '^TestRunIndexJob(HonorsCooperativeCancelDuringProgress|ReconcilesPublicationWinsDuringCancel)$'
  - go test -v -count=1 -p 1 ./internal/store -run '^TestIndexJobRead(LoadersPreserveCancelRequestedAfterStaleFailureCAS|PropagatesMarkFailedDatabaseError|PropagatesReloadErrorAfterStaleCASRace)$'
evidence:
  - internal/store/index_jobs_test.go
  - internal/store/index_jobs_ownership_test.go
  - internal/indexer/indexer_progress_test.go
records:
  - id: TC-IDX-OWNERSHIP-001
    type: liveness
    scenario: progress/liveness
    command: go test -v -count=1 -p 1 ./internal/store -run '^TestMarkIndexJobProgressUpdatesCountersStageAndTimestamp$'
    test: TestMarkIndexJobProgressUpdatesCountersStageAndTimestamp
    setup: t.TempDir with a local SQLite database and a cancellable index loop
    oracle: updated_at, stage, current_path, files_total and counters advance while the worker is alive; cancellation is observed at a loop boundary
    stop_if: progress_timestamp_stalls_before_completion=true
  - id: TC-IDX-OWNERSHIP-002
    type: cancellation
    scenario: queued immediate cancel
    command: go test -v -count=1 -p 1 ./internal/store -run '^TestCancelIndexJobQueuedImmediate$'
    test: TestCancelIndexJobQueuedImmediate
    setup: create a queued job with an ownership row but do not spawn a worker
    oracle: cancel transitions queued to canceled without spawn; ownership is released only by the owner-bound terminal transition; result is never failed
    stop_if: queued_cancel_spawns_worker=true
  - id: TC-IDX-OWNERSHIP-003
    type: cancellation
    scenario: cooperative running cancel
    command: go test -v -count=1 -p 1 ./internal/service -run '^TestRunIndexJobHonorsCooperativeCancelDuringProgress$'
    test: TestRunIndexJobHonorsCooperativeCancelDuringProgress
    setup: running worker, requested_cancel CAS and a publication barrier that has not committed
    oracle: worker confirms canceled only after no-publication boundary; requested_cancel is not terminal and exactly one terminal winner remains
    stop_if: canceled_written_before_worker_exit=true
  - id: TC-IDX-OWNERSHIP-004
    type: cancellation
    scenario: force timeout
    command: go test -v -count=1 -p 1 ./internal/store -run '^TestCancelIndexJobForceTimeoutRetainsReservation$'
    test: TestCancelIndexJobForceTimeoutRetainsReservation
    setup: process that ignores termination and a matching workspace lock
    oracle: timeout leaves requested_cancel=1, phase=terminating, reservation and live lock intact; a confirmed dead PID may be cleaned safely
    stop_if: live_lock_missing_after_timeout=true
  - id: TC-IDX-OWNERSHIP-005
    type: capability
    scenario: capability clearing in memory/control-plane
    command: go test -v -count=1 -p 1 ./internal/store -run '^TestControlPlaneProjectionCannotExerciseOwnerRights$'
    test: TestControlPlaneProjectionCannotExerciseOwnerRights
    setup: worker fence invalidated by stale, canceled or released ownership
    oracle: in-memory publish/terminal capabilities are cleared; control-plane retains only status and cancel CAS and cannot mint or adopt a fence
    stop_if: control_plane_publishes_without_owner=true
```

## 2. PID y migración legacy

```toon
harness_protocol: SDD-HARNESS-v1
source_protocol: SDD-WIKI-SOURCE-v1
doc_id: TP-IDX-OWNERSHIP
block_id: TP-IDX-OWNERSHIP.legacy
kind: executable-test-plan
source_of_truth: this fenced toon block
imports:
  - .docs/wiki/04_RF/RF-IDX-004.md
  - .docs/wiki/09_contratos/CT-INDEX-JOB-OWNERSHIP.md
exports:
  - TP-IDX-OWNERSHIP.legacy
verify:
  - go test -v -count=1 -p 1 ./internal/store -run '^Test(LegacyPID0ModernSemantics|LegacyPID0Compatibility|IndexJobReadCompatibilityWithoutRequestedCancelColumn|ConcurrentLegacyOwnershipMigration|RemoveWorkspaceIndexLockForPIDNeverRemovesLiveProcess|Open_MigratesLegacyRepoColumns)$'
evidence:
  - internal/store/index_lock_test.go
  - internal/store/store_test.go
  - internal/store/index_jobs_ownership_test.go
records:
  - id: TC-IDX-OWNERSHIP-006
    type: process-safety
    scenario: PID0 moderno
    command: go test -v -count=1 -p 1 ./internal/store -run '^TestLegacyPID0ModernSemantics$'
    test: TestLegacyPID0ModernSemantics
    setup: ownership/lock row with PID=0 in the current schema
    oracle: PID0 is unknown/not-verifiable; status and recovery remain safe and no live lock is removed by PID heuristic
    stop_if: pid0_assumed_dead=true
  - id: TC-IDX-OWNERSHIP-007
    type: compatibility
    scenario: PID0 legacy
    command: go test -v -count=1 -p 1 ./internal/store -run '^TestLegacyPID0Compatibility$'
    test: TestLegacyPID0Compatibility
    setup: pre-migration schema/row with PID0 or absent requested_cancel
    oracle: additive migration preserves jobs and generations, exposes a conservative status, and permits reservation only after confirmed stale evidence
    stop_if: legacy_job_deleted=true
  - id: TC-IDX-OWNERSHIP-008
    type: concurrency
    scenario: migración legacy concurrente
    command: go test -v -count=1 -p 1 ./internal/store -run '^TestConcurrentLegacyOwnershipMigration$'
    test: TestConcurrentLegacyOwnershipMigration
    setup: N goroutines open and migrate the same legacy index.db in t.TempDir
    oracle: unique workspace_root and CAS converge to one ownership row; no duplicate active owner and no lost fence
    stop_if: duplicate_active_ownership=true
```

## 3. Fencing y publicación por modo

```toon
harness_protocol: SDD-HARNESS-v1
source_protocol: SDD-WIKI-SOURCE-v1
doc_id: TP-IDX-OWNERSHIP
block_id: TP-IDX-OWNERSHIP.publication
kind: executable-test-plan
source_of_truth: this fenced toon block
imports:
  - .docs/wiki/04_RF/RF-IDX-001.md
  - .docs/wiki/04_RF/RF-IDX-004.md
  - .docs/wiki/08_db/DB-DOC-INDEX.md
exports:
  - TP-IDX-OWNERSHIP.publication
verify:
  - go test -v -count=1 -p 1 ./internal/store -run '^Test(IndexJobStaleFenceCannotWinAfterTerminalRelease|IndexJobTerminalRaceHasSingleImmutableWinner|FencedPublicationModeMatrix|ControlPlaneProjectionCannotExerciseOwnerRights|FencedGraphPointerActivation|FencedGraphExpectedPriorActivation|StaleOrCanceledGraphFencePreservesPointerAndRuntime|PublicationAndTerminalCommitAtomically|IncrementalPublicationCanceledFencePreservesFilesAndSymbols|IncrementalPublicationStaleFencePreservesFilesAndSymbols)$'
  - go test -v -count=1 -p 1 ./internal/indexer -run '^Test(FullIndexPublishesLocalGoGraph|IncrementalIndexPublishesNewGraphGenerationAfterFileChange|IncrementalIndexObservationFailureLeavesGraphStale)$'
  - go test -v -count=1 -p 1 ./internal/service -run '^Test(EmbeddingFailureAfterPublicationLeavesSucceededGeneration|EmbeddingIndexPersistsSuccessfulBatchesAndResumes|Recall_BilingualEStoEN|Recall_EmbeddingsImplicitlyActiveWithoutEnabled)$'
evidence:
  - internal/store/index_jobs_ownership_test.go
  - internal/store/store_test.go
  - internal/store/graph_store_test.go
  - internal/indexer/graph_pipeline_test.go
  - internal/indexer/incremental_test.go
  - internal/service/recall_test.go
  - internal/service/index_jobs_test.go
records:
  - id: TC-IDX-OWNERSHIP-009
    type: publication
    scenario: publicación fenced full/docs/catalog/graph/incremental
    command: go test -v -count=1 -p 1 ./internal/store -run '^TestFencedGraphPointerActivation$'
    test: TestFencedGraphPointerActivation
    setup: candidate generations for modes full, docs, catalog, graph and incremental with independent fences
    oracle: each mode publishes only its declared surfaces and rejects a fence belonging to another mode
    stop_if: cross_mode_pointer_changed=true
  - id: TC-IDX-OWNERSHIP-010
    type: fencing
    scenario: stale/canceled fence sin cambios de pointers/runtime
    command: go test -v -count=1 -p 1 ./internal/store -run '^TestStaleOrCanceledGraphFencePreservesPointerAndRuntime$'
    test: TestStaleOrCanceledGraphFencePreservesPointerAndRuntime
    setup: snapshot with active catalog/docs/graph pointers and a stale or canceled candidate
    oracle: ErrStaleIndexJobOwner or zero RowsAffected; active_*_generation_id, memory_pointer, runtime metadata, files and symbols remain unchanged
    stop_if: stale_writer_mutated_runtime=true
  - id: TC-IDX-OWNERSHIP-011
    type: atomicity
    scenario: atomicidad publication-terminal
    command: go test -v -count=1 -p 1 ./internal/store -run '^TestPublicationAndTerminalCommitAtomically$'
    test: TestPublicationAndTerminalCommitAtomically
    setup: inject a commit failure between candidate activation and terminal transition
    oracle: transaction rollback leaves prior pointers and terminal state; no succeeded job points to an invisible generation and no failed job points to a new active generation
    stop_if: partial_publication_visible=true
  - id: TC-IDX-OWNERSHIP-012
    type: best-effort
    scenario: embeddings post-publicación
    command: go test -v -count=1 -p 1 ./internal/service -run '^TestEmbeddingFailureAfterPublicationLeavesSucceededGeneration$'
    test: TestEmbeddingFailureAfterPublicationLeavesSucceededGeneration
    setup: valid published generation and an embedding provider that times out or returns partial batches
    oracle: publication remains succeeded, warning is sanitized, and incomplete embeddings are not advertised as complete
    stop_if: embedding_failure_rolled_back_valid_publication=true
```

## 4. Watcher, locks y carreras

```toon
harness_protocol: SDD-HARNESS-v1
source_protocol: SDD-WIKI-SOURCE-v1
doc_id: TP-IDX-OWNERSHIP
block_id: TP-IDX-OWNERSHIP.watcher-lock-races
kind: executable-test-plan
source_of_truth: this fenced toon block
imports:
  - .docs/wiki/04_RF/RF-IDX-004.md
  - .docs/wiki/09_contratos/CT-INDEX-JOB-OWNERSHIP.md
exports:
  - TP-IDX-OWNERSHIP.watcher-lock-races
verify:
  - go test -v -count=1 -p 1 ./internal/daemon -run '^TestWatcherBatchRetry(UsesThreeImmediateRetries|StopsAfterOneDeferredFailure|NewEventRearmsExhaustedCycle)$'
  - go test -v -count=1 -p 1 ./internal/store -run '^Test(WithWorkspaceIndexLockRejectsConcurrentIndexRun|AcquireWithTimeoutReportsContentionAndPreservesLiveLock|WorkspaceIndexLockDoesNotRemoveReplacementOwner|WorkspaceIndexLockCleanupPreservesReplacementClaimedAfterQuarantine|RemoveWorkspaceIndexLockForOwnerPreservesReplacementAfterQuarantine|RemoveWorkspaceIndexLockForPIDNeverRemovesLiveProcess)$'
  - go test -v -count=1 -p 1 ./internal/store -run '^Test(CancelPublicationRaceNeverEndsFailed|IndexJobCancellationWinsOverStaleFailureWriter)$'
  - go test -v -count=1 -p 1 ./internal/store -run '^TestForegroundIncrementalPublicationUpdatesFilesAndSymbolsTogether$'
  - go test -v -count=1 -p 1 ./internal/daemon -run '^TestWatcherForegroundIncrementalUpdatesFilesAndSymbolsTogether$'
evidence:
  - internal/daemon/file_watcher_test.go
  - internal/store/index_lock_test.go
  - internal/store/index_jobs_ownership_test.go
records:
  - id: TC-IDX-OWNERSHIP-013
    type: watcher
    scenario: watcher tres inmediatos y una única diferida sin storm
    command: go test -v -count=1 -p 1 ./internal/daemon -run '^TestWatcherBatchRetryUsesThreeImmediateRetries$'
    test: TestWatcherBatchRetryUsesThreeImmediateRetries
    setup: coalesced watcher event with deterministic contention and one new event after exhaustion
    oracle: three immediate retries plus one deferred retry, exactly one timer, and a new event rearms one fresh cycle without storm
    stop_if: retry_timer_count > 1
  - id: TC-IDX-OWNERSHIP-014
    type: lock
    scenario: lock quarantine y replacement owner-bound
    command: go test -v -count=1 -p 1 ./internal/store -run '^TestWorkspaceIndexLockDoesNotRemoveReplacementOwner$'
    test: TestWorkspaceIndexLockDoesNotRemoveReplacementOwner
    setup: stale lock quarantined, replacement owner claims the same path, then the stale callback runs
    oracle: cleanup compares PID, owner_token and started_at; replacement lock survives and a live PID is never removed
    stop_if: replacement_owner_lock_removed=true
  - id: TC-IDX-OWNERSHIP-015
    type: race
    scenario: carrera cancelación/publicación que no puede acabar failed
    command: go test -v -count=1 -p 1 ./internal/store -run '^TestCancelPublicationRaceAtCommitBoundary$'
    test: TestCancelPublicationRaceAtCommitBoundary
    setup: synchronize cancellation CAS, publication commit and stale failure writer at the same barrier
    oracle: exactly one terminal canceled or succeeded wins; a valid cancellation loser is stale/recoverable and the final job is never failed
    stop_if: cancellation_publication_race_failed=true
  - id: TC-IDX-OWNERSHIP-016
    type: atomicity
    scenario: incremental cancelado no deriva files/symbols
    command: go test -v -count=1 -p 1 ./internal/store -run '^TestIncrementalPublicationCanceledFencePreservesFilesAndSymbols$'
    test: TestIncrementalPublicationCanceledFencePreservesFilesAndSymbols
    setup: catálogo incremental previo, cancel_requested confirmado y cambios de files/symbols preparados en memoria
    oracle: el fence cancelado se rechaza antes de escribir y files/symbols permanecen byte/row-equivalentes al snapshot previo
    stop_if: canceled_incremental_catalog_drift=true
  - id: TC-IDX-OWNERSHIP-017
    type: atomicity
    scenario: incremental stale no deriva files/symbols
    command: go test -v -count=1 -p 1 ./internal/store -run '^TestIncrementalPublicationStaleFencePreservesFilesAndSymbols$'
    test: TestIncrementalPublicationStaleFencePreservesFilesAndSymbols
    setup: catálogo incremental previo y fence liberado por un terminal succeeded antes del intento stale
    oracle: el owner stale se rechaza antes de escribir y files/symbols permanecen equivalentes al snapshot previo
    stop_if: stale_incremental_catalog_drift=true
  - id: TC-IDX-OWNERSHIP-018
    type: race
    scenario: publication-wins reconciliada por runIndexJob
    command: go test -v -count=1 -p 1 ./internal/service -run '^TestRunIndexJobReconcilesPublicationWinsDuringCancel$'
    test: TestRunIndexJobReconcilesPublicationWinsDuringCancel
    setup: cancel_requested y publicación terminal succeeded sincronizados antes del CAS de cancelación owner-bound
    oracle: MarkIndexJobCanceled stale se reconcilia como succeeded; runIndexJob no propaga ErrStaleIndexJobOwner ni marca failed
    stop_if: publication_wins_propagates_stale_error=true
  - id: TC-IDX-OWNERSHIP-019
    type: atomicity
    scenario: publicaci�n incremental foreground
    command: go test -v -count=1 -p 1 ./internal/store -run '^TestForegroundIncrementalPublicationUpdatesFilesAndSymbolsTogether$'
    test: TestForegroundIncrementalPublicationUpdatesFilesAndSymbolsTogether
    setup: cambios de files y symbols preparados y publicados por la ruta foreground sin job
    oracle: content_hash y symbols reflejan el mismo cambio y el runtime graph queda stale de forma coherente cuando no hay graph publication
    stop_if: foreground_files_symbols_diverge=true
  - id: TC-IDX-OWNERSHIP-020
    type: atomicity
    scenario: publicaci�n incremental del watcher
    command: go test -v -count=1 -p 1 ./internal/daemon -run '^TestWatcherForegroundIncrementalUpdatesFilesAndSymbolsTogether$'
    test: TestWatcherForegroundIncrementalUpdatesFilesAndSymbolsTogether
    setup: workspace TypeScript real, cambio detectado por git y callback watcher que usa la ruta incremental foreground
    oracle: el callback actualiza content_hash y symbols juntos; el s�mbolo anterior no permanece y el nuevo s�
    stop_if: watcher_files_symbols_diverge=true
  - id: TC-IDX-OWNERSHIP-021
    type: terminal-truth
    scenario: stale read con cancel_requested
    command: go test -v -count=1 -p 1 ./internal/store -run '^TestIndexJobReadLoadersPreserveCancelRequestedAfterStaleFailureCAS$'
    test: TestIndexJobReadLoadersPreserveCancelRequestedAfterStaleFailureCAS
    setup: fila moderna con PID muerto y cancel_requested confirmado antes de la reconciliación de lectura
    oracle: MarkIndexJobFailed puede perder el CAS, pero GetIndexJob, LatestIndexJob y ActiveIndexJob recargan el estado SQLite real y nunca sintetizan failed
    stop_if: stale_cancel_read_synthesizes_failed=true
  - id: TC-IDX-OWNERSHIP-022
    type: error-propagation
    scenario: error de MarkIndexJobFailed durante reconciliación
    command: go test -v -count=1 -p 1 ./internal/store -run '^TestIndexJobReadPropagatesMarkFailedDatabaseError$'
    test: TestIndexJobReadPropagatesMarkFailedDatabaseError
    setup: seam de MarkIndexJobFailed que devuelve un error de base de datos
    oracle: el loader devuelve el error envuelto y la fila conserva su estado no terminal en SQLite
    stop_if: mark_failed_database_error_is_ignored=true
  - id: TC-IDX-OWNERSHIP-023
    type: api-boundary
    scenario: publicación incremental sin bypass por commit independiente
    command: go test -v -count=1 -p 1 ./internal/store -run '^TestForegroundIncrementalPublicationUpdatesFilesAndSymbolsTogether$'
    test: TestForegroundIncrementalPublicationUpdatesFilesAndSymbolsTogether
    setup: cambios de files y symbols publicados mediante PublishIncrementalGenerationWithChanges
    oracle: las APIs públicas de publicación mantienen files y symbols en una transacción atómica; ReplaceFileSymbols y DeleteFileSymbols no son APIs exportadas ni tienen callers productivos
    stop_if: incremental_private_query_bypass=true
```
