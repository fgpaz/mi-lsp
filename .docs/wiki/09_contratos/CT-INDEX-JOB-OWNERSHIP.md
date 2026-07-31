# CT-INDEX-JOB-OWNERSHIP — Contrato de ownership y fencing

```yaml
harness_protocol: SDD-HARNESS-v1
source_protocol: SDD-WIKI-SOURCE-v1
doc_id: CT-INDEX-JOB-OWNERSHIP
id: CT-INDEX-JOB-OWNERSHIP
kind: technical-contract
audience: llm-first
imports:
  - '[[00_gobierno_documental]]'
  - '[[FL-IDX-01]]'
  - '[[RF-IDX-001]]'
  - '[[RF-IDX-004]]'
  - '[[TP-IDX-OWNERSHIP]]'
exports:
  - CT-INDEX-JOB-OWNERSHIP
  - CT-INDEX-JOB-OWNERSHIP.reserve-fence
  - CT-INDEX-JOB-OWNERSHIP.control-plane
  - CT-INDEX-JOB-OWNERSHIP.publication
  - CT-INDEX-JOB-OWNERSHIP.watcher-lock
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/03_FL/FL-IDX-01.md
  - .docs/wiki/04_RF/RF-IDX-001.md
  - .docs/wiki/04_RF/RF-IDX-004.md
  - .docs/wiki/06_pruebas/TP-IDX-OWNERSHIP.md
agent_may_edit:
  - internal/store/index_jobs.go
  - internal/store/queries_incremental.go
  - internal/store/index_publish.go
  - internal/indexer/incremental.go
  - internal/indexer/indexer.go
  - internal/service/index_jobs.go
  - internal/daemon/file_watcher.go
  - internal/store/index_jobs_ownership_test.go
  - internal/store/index_jobs_test.go
  - internal/store/index_lock_test.go
  - internal/daemon/file_watcher_test.go
  - internal/service/index_jobs_test.go
  - .docs/wiki/09_contratos/CT-INDEX-JOB-OWNERSHIP.md
  - .docs/wiki/04_RF/RF-IDX-004.md
  - .docs/wiki/06_pruebas/TP-IDX-OWNERSHIP.md
agent_must_not_edit:
  - .docs/wiki/04_RF/RF-IDX-003.md
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - MI_LSP_CLIENT_NAME=batch-a-doc-writer MI_LSP_SESSION_ID=2026-07-30-index-safety-probe go run ./cmd/mi-lsp nav wiki validate-harness --workspace . --ids CT-INDEX-JOB-OWNERSHIP --format toon --no-daemon
  - MI_LSP_CLIENT_NAME=batch-a-doc-writer MI_LSP_SESSION_ID=2026-07-30-index-safety-probe go run ./cmd/mi-lsp nav wiki validate-source --workspace . --ids CT-INDEX-JOB-OWNERSHIP --format toon --no-daemon
  - go test ./internal/store ./internal/service ./internal/daemon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
  - wiki_source_verdict=BLOCKED
  - owner_fence_not_cas=true
  - rows_affected != 1_on_terminal_transition=true
evidence:
  - .docs/wiki/09_contratos/CT-INDEX-JOB-OWNERSHIP.md
  - .docs/wiki/04_RF/RF-IDX-004.md
  - .docs/wiki/06_pruebas/TP-IDX-OWNERSHIP.md
  - internal/store/index_jobs.go
  - internal/store/queries_incremental.go
  - internal/store/index_publish.go
  - internal/store/index_jobs_ownership_test.go
  - internal/store/index_jobs_test.go
  - internal/store/index_lock.go
  - internal/store/index_lock_test.go
  - internal/indexer/incremental.go
  - internal/indexer/indexer.go
  - internal/indexer/incremental_test.go
  - internal/service/index_jobs.go
  - internal/service/index_jobs_test.go
  - internal/daemon/file_watcher.go
  - internal/daemon/file_watcher_test.go
```

Enlaces relacionados: [[RF-IDX-004]] · [[TP-IDX-OWNERSHIP]] · [[RF-IDX-001]] · [[FL-IDX-01]]

## 1. Reserva y fence

```toon
harness_protocol: SDD-HARNESS-v1
source_protocol: SDD-WIKI-SOURCE-v1
doc_id: CT-INDEX-JOB-OWNERSHIP
block_id: CT-INDEX-JOB-OWNERSHIP.reserve-fence
kind: contract
source_of_truth: this fenced toon block
imports:
  - .docs/wiki/04_RF/RF-IDX-004.md
  - .docs/wiki/04_RF/RF-IDX-001.md
exports:
  - CT-INDEX-JOB-OWNERSHIP.reserve-fence
verify:
  - go test ./internal/store -run 'Test(CreateIndexJobConcurrentStartsReserveOneWorkspace|IndexJobStaleFenceCannotWinAfterTerminalRelease|OwnerBoundIndexJobMutationsRequireExplicitFence|IndexJobWrongOwnerFenceCannotTransition)'
evidence:
  - internal/store/index_jobs.go
  - internal/store/index_jobs_ownership_test.go
records:
  - id: CT-IDX-OWNERSHIP-RESERVE-001
    type: operation
    name: reserve
    input: workspace_root, workspace_name, mode, clean
    atomicity: create index_job, index_generation and index_job_ownership in one SQLite transaction
    uniqueness: index_job_ownership.workspace_root is unique
    output: queued job or ActiveIndexJobError
  - id: CT-IDX-OWNERSHIP-OWNER-001
    type: record
    fields: workspace_root, job_id, owner_token, fencing_token, pid, acquired_at, released_at
    fencing: monotonically increasing per workspace and never reused after release
  - id: CT-IDX-OWNERSHIP-TRANSITION-001
    type: operation
    guard: expected status plus requested_cancel predicate plus active ownership by job_id plus RowsAffected=1
    stale_result: ErrStaleIndexJobOwner
  - id: CT-IDX-OWNERSHIP-LEGACY-MIGRATION-001
    type: migration
    guard: IF NOT EXISTS and additive schema/data migration
    oracle: concurrent legacy migration preserves existing jobs/generations and converges to one owner row
```

## 2. Control-plane, cancelación y capacidades

```toon
harness_protocol: SDD-HARNESS-v1
source_protocol: SDD-WIKI-SOURCE-v1
doc_id: CT-INDEX-JOB-OWNERSHIP
block_id: CT-INDEX-JOB-OWNERSHIP.control-plane
kind: contract
source_of_truth: this fenced toon block
imports:
  - .docs/wiki/04_RF/RF-IDX-004.md
  - .docs/wiki/06_pruebas/TP-IDX-OWNERSHIP.md
exports:
  - CT-INDEX-JOB-OWNERSHIP.control-plane
verify:
  - go test ./internal/store -run 'Test(CancelIndexJobForceTimeoutRetainsReservation|MarkIndexJobFailedRejectsRequestedCancellationAndKeepsOwnership|IndexJobCancellationWinsOverStaleFailureWriter|CancelIndexJobForceTerminatesProcessAndMarksCanceled)'
  - go test ./internal/indexer -run 'TestCatalogIndexProgressReportsAndCanCancel'
evidence:
  - internal/store/index_jobs_ownership_test.go
  - internal/store/index_jobs_test.go
  - internal/indexer/indexer_progress_test.go
records:
  - id: CT-IDX-CONTROL-PLANE-001
    type: operation
    name: request-cancel
    input: job_id only
    guard: CAS against active job with ownership vigente
    output: requested_cancel=1; never returns or adopts a publication fence
  - id: CT-IDX-CONTROL-PLANE-002
    type: state-machine
    name: cancel
    states: queued -> cancel_requested -> canceled; running -> cancel_requested -> terminating|canceled
    rule: cancel_requested is never terminal
  - id: CT-IDX-CONTROL-PLANE-003
    type: capability
    name: clear
    memory: clear publish and terminal capabilities as soon as ownership is stale, canceled, or released
    control_plane: retain only status and cancel CAS; never mint owner_token or fencing_token
  - id: CT-IDX-CONTROL-PLANE-004
    type: process
    name: pid0
    modern: PID0 is unknown/not-verifiable and cannot be treated as a live-process proof
    legacy: PID0 or missing PID in legacy rows is recoverable only after conservative stale evidence
  - id: CT-IDX-CONTROL-PLANE-005
    type: cancellation
    name: force-timeout
    oracle: if waitForProcessExit does not confirm exit, phase=terminating, reservation and lock remain
  - id: CT-IDX-CONTROL-PLANE-006
    type: race
    name: cancellation-publication
    oracle: canceled or succeeded is the single terminal winner; a valid cancellation race never ends failed
```

## 3. Publicación y estado runtime

```toon
harness_protocol: SDD-HARNESS-v1
source_protocol: SDD-WIKI-SOURCE-v1
doc_id: CT-INDEX-JOB-OWNERSHIP
block_id: CT-INDEX-JOB-OWNERSHIP.publication
kind: contract
source_of_truth: this fenced toon block
imports:
  - .docs/wiki/04_RF/RF-IDX-001.md
  - .docs/wiki/04_RF/RF-IDX-004.md
  - .docs/wiki/08_db/DB-DOC-INDEX.md
exports:
  - CT-INDEX-JOB-OWNERSHIP.publication
verify:
  - go test ./internal/store -run 'Test(ReplaceWorkspaceIndexPublishesGenerationMetadata|StageGraphGenerationIsAtomicAndInitiallyInvisible|PublishGraphObservationBatchesIdempotentPathPreservesPointerCAS)'
  - go test ./internal/indexer -run 'Test(FullIndexPublishesLocalGoGraph|IncrementalIndexPublishesNewGraphGenerationAfterFileChange|IncrementalIndexObservationFailureLeavesGraphStale)'
  - go test ./internal/service -run 'TestIndexStartPopulatesWikiChunkEmbeddings|TestRecall_.*Embedding'
evidence:
  - internal/store/store_test.go
  - internal/store/graph_store_test.go
  - internal/indexer/graph_pipeline_test.go
  - internal/indexer/incremental_test.go
  - internal/service/recall_test.go
records:
  - id: CT-IDX-PUBLISH-001
    type: mode
    modes: full, docs, catalog, graph, incremental
    rule: each mode fences its candidate generation and publishes only its declared surfaces
  - id: CT-IDX-PUBLISH-002
    type: atomicity
    rule: publication and terminal transition commit atomically; a failed commit leaves prior pointers and runtime metadata unchanged
  - id: CT-IDX-PUBLISH-003
    type: fencing
    rule: stale or canceled writer cannot change active_*_generation_id, memory_pointer, runtime metadata, files, symbols, or docs
  - id: CT-IDX-PUBLISH-004
    type: embeddings
    rule: embeddings run post-publication as best-effort and cannot roll back a valid publication or turn it into failed
  - id: CT-IDX-PUBLISH-005
    type: terminal
    rule: terminal update requires expected state, active owner fence and RowsAffected=1; zero rows is a stale rejection
```

## 4. Watcher y lock interproceso

```toon
harness_protocol: SDD-HARNESS-v1
source_protocol: SDD-WIKI-SOURCE-v1
doc_id: CT-INDEX-JOB-OWNERSHIP
block_id: CT-INDEX-JOB-OWNERSHIP.watcher-lock
kind: contract
source_of_truth: this fenced toon block
imports:
  - .docs/wiki/04_RF/RF-IDX-004.md
  - .docs/wiki/06_pruebas/TP-IDX-OWNERSHIP.md
exports:
  - CT-INDEX-JOB-OWNERSHIP.watcher-lock
verify:
  - go test ./internal/daemon -run 'TestWatcherBatchRetry(UsesThreeImmediateRetries|StopsAfterOneDeferredFailure|NewEventRearmsExhaustedCycle)'
  - go test ./internal/store -run 'Test(WithWorkspaceIndexLockRejectsConcurrentIndexRun|WithWorkspaceIndexLockRemovesStaleLock|AcquireWithTimeoutReportsContentionAndPreservesLiveLock|WorkspaceIndexLockDoesNotRemoveReplacementOwner|WorkspaceIndexLockCleanupPreservesReplacementClaimedAfterQuarantine|RemoveWorkspaceIndexLockForOwnerPreservesReplacementAfterQuarantine|RemoveWorkspaceIndexLockForPIDNeverRemovesLiveProcess)'
evidence:
  - internal/daemon/file_watcher.go
  - internal/daemon/file_watcher_test.go
  - internal/store/index_lock.go
  - internal/store/index_lock_test.go
records:
  - id: CT-IDX-WATCHER-001
    type: interprocess-barrier
    path: <workspace>/.mi-lsp/index.lock
    owners: index job and file watcher
    rule: watcher contends on the same lock and never writes symbols in parallel with an index job
  - id: CT-IDX-WATCHER-002
    type: retry
    rule: three immediate attempts plus one deferred attempt per coalesced event, with a new event rearming exactly one cycle and no timer storm
  - id: CT-IDX-WATCHER-003
    type: quarantine
    rule: stale lock is quarantined before replacement; cleanup compares PID, owner_token and started_at
  - id: CT-IDX-WATCHER-004
    type: owner-bound-release
    rule: an exiting callback cannot remove a lock whose owner token, PID or started_at belongs to a replacement owner
```
