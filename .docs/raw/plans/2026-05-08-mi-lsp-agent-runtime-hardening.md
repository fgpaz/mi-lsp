# mi-lsp Agent Runtime Hardening Implementation Plan

> **For Hermes/OpenCode:** Use `subagent-driven-development` and `dispatching-parallel-agents` to execute this plan lane-by-lane. Dispatch one subagent per lane or worktree after the serial gates are complete.

**Goal:** Make `mi-lsp` faster and more reliable for agents by improving structured errors, search fallbacks, wiki evidence, truncation continuations, workspace bootstrap UX, runtime telemetry, and Roslyn memory behavior.

**Architecture:** Use a docs-first contract freeze, then parallel implementation lanes with minimal shared-file contention. A short foundation lane owns shared envelope/telemetry schema changes; all other lanes consume those fields and stay within isolated files.

**Tech Stack:** Go CLI/service packages, repo-local SQLite, global daemon telemetry SQLite, TOON/JSON/YAML output formatters, optional .NET Roslyn worker, PowerShell verification on Windows.

---

## Current Evidence Baseline

Telemetry source: `C:\Users\fgpaz\.mi-lsp\daemon\daemon.db`, read-only snapshot from 2026-05-08.

Key signals:

- `nav.search`: 3446 events, 187 failures, p95 `18569ms`, p99 `48548ms`, max `113558ms`.
- `nav.search`: 140 failures from `fork/exec ... rg.exe: Access is denied.`
- `nav.search`: truncation rate `53.7%` for TOON output.
- `nav.wiki.search`: 390 events, truncation rate `76.9%`, p95 `3179ms`.
- `workspace.status` and `nav.governance`: repeated failures from `workspace not found in registry and path does not exist`.
- Active Roslyn runtime snapshots reached about `431MB` for one worker.
- Current repo status: governance valid, but `doc_count=0`, `docs_index_ready=false`, `index_ready=false`.

Code evidence from read-only audit:

- Raw CLI errors bypass envelopes: `cmd/mi-lsp/main.go:12-14`, `internal/cli/root.go:224-227`.
- Stable envelope lacks typed error object: `internal/model/types.go:76-90`.
- `rg` failure has no fallback on `Start` error: `internal/service/search.go:76-104`.
- `search --include-content` does per-match file reads: `internal/service/search_content.go:31-35`, `96-126`.
- Truncator drops items after full envelope materialization: `internal/output/truncator.go:32-48`.
- `nav wiki search` empty docgraph hint only suggests mutating index: `internal/service/wiki_search.go:44-55`.
- Workspace status emits alias-based next steps even when path-resolved alias is not registered: `internal/service/workspace_ops.go:279-288`.
- Roslyn workspace cache is unbounded: `worker-dotnet/MiLsp.Worker/RoslynService.cs:12`, `212-230`.
- RF drift: `.docs/wiki/04_RF.md:62` and `.docs/wiki/06_matriz_pruebas_RF.md:57` include `RF-QRY-016`, but `.docs/wiki/03_FL.md:69` does not.

---

## Execution Rules

Use these rules for every implementation subagent:

- Start with `ps-contexto`, `mi-lsp workspace status . --format toon`, and `mi-lsp nav governance --workspace . --format toon`.
- Do not push to `main`.
- Do not commit unless the orchestrator explicitly authorizes commits for the execution phase.
- Preserve unrelated dirty work.
- Prefer tests first, implementation second, docs sync third.
- Keep each lane within its allowed paths.
- If a lane needs a shared file owned by another lane, stop and escalate instead of editing it.
- Run only the lane-specific tests first, then the integration tests in the final wave.

Recommended worktree model for maximum parallelism:

```powershell
git switch -c mi-lsp-agent-runtime-hardening
git worktree add .worktrees\agent-runtime-contracts -b mi-lsp-hardening-contracts
git worktree add .worktrees\agent-runtime-errors -b mi-lsp-hardening-errors
git worktree add .worktrees\agent-runtime-search -b mi-lsp-hardening-search
git worktree add .worktrees\agent-runtime-wiki -b mi-lsp-hardening-wiki
git worktree add .worktrees\agent-runtime-truncation -b mi-lsp-hardening-truncation
git worktree add .worktrees\agent-runtime-workspace -b mi-lsp-hardening-workspace
git worktree add .worktrees\agent-runtime-telemetry -b mi-lsp-hardening-telemetry
git worktree add .worktrees\agent-runtime-roslyn -b mi-lsp-hardening-roslyn
git worktree add .worktrees\agent-runtime-bench -b mi-lsp-hardening-bench
```

If the orchestrator does not want multiple branches, use one feature branch and dispatch only non-overlapping lanes in parallel.

---

## Dependency Map

Serial gates:

1. Wave 0 must run before any implementation.
2. Wave 1 contract/docs can run in parallel after Wave 0.
3. Wave 2 shared schema foundation must merge before code lanes that need new model fields.
4. Waves 3A through 3G can run in parallel after Wave 2.
5. Wave 4 integration runs after all code lanes merge.

Parallel lanes after Wave 2:

| Lane | Owner | Primary files | Conflict risk |
| --- | --- | --- | --- |
| 3A | Error envelope | `internal/cli`, `internal/output`, `internal/telemetry` | Medium |
| 3B | Search fallback | `internal/service/search*` | Low |
| 3C | Wiki route/search/pack | `internal/service/wiki_search.go`, `internal/service/pack.go` | Medium |
| 3D | Truncation/multi-read | `internal/output/truncator.go`, `internal/service/multi_read.go` | Medium |
| 3E | Workspace UX | `internal/service/workspace_ops.go`, resolution tests | Low |
| 3F | Telemetry/log evidence | `internal/daemon`, `internal/telemetry` | Medium |
| 3G | Roslyn memory | `internal/daemon/lifecycle*`, `worker-dotnet` | Medium |
| 3H | Benchmarks | `*_bench_test.go`, perf smoke tests | Low |

---

## Wave 0: Readiness Gate, Serial

### Task 0.1: Confirm Governance And Branch Safety

**Objective:** Ensure implementation does not start from stale governance or accidental main push state.

**Files:** None.

**Step 1: Check workspace status**

Run: `mi-lsp workspace status . --format toon`

Expected: `governance_blocked=false`, `governance_sync=in_sync`.

**Step 2: Check governance**

Run: `mi-lsp nav governance --workspace . --format toon`

Expected: `blocked=false`, `sync=in_sync`.

**Step 3: Check git state**

Run: `git status --short --branch`

Expected: no unrelated tracked modifications in files this plan will touch.

**Step 4: Stop condition**

Stop if governance is blocked, if the projection is stale, or if a same-scope dirty change already exists.

### Task 0.2: Rebuild Docs Index Before Wiki/Harness Work

**Objective:** Make wiki search, harness validation, and source validation usable before changing contracts.

**Files:** `.mi-lsp/index.db` local operational state only.

**Step 1: Rebuild docs-only index**

Run: `mi-lsp index --workspace . --docs-only`

Expected: command succeeds and docs are indexed.

**Step 2: Verify status**

Run: `mi-lsp workspace status . --format toon`

Expected: `docs_index_ready=true`, `doc_count>0`.

**Step 3: Validate harness**

Run: `mi-lsp nav wiki validate-harness --workspace . --format toon`

Expected: no governance blocker. If `harness_verdict=BLOCKED`, capture blockers and route to the docs lane before code.

**Step 4: Validate source**

Run: `mi-lsp nav wiki validate-source --workspace . --format toon`

Expected: no navigation blocker that prevents plan execution.

---

## Wave 1: Contract And Docs Freeze, Parallel

### Task 1A: Repair RF-QRY-016 Traceability Drift

**Objective:** Make the FL -> RF index agree with the RF and TP layers before code work.

**Files:**

- Modify: `.docs/wiki/03_FL.md:63-71`
- Review: `.docs/wiki/04_RF.md:55-62`
- Review: `.docs/wiki/06_matriz_pruebas_RF.md:50-57`

**Step 1: Write the failing traceability check**

Run: `mi-lsp nav trace RF-QRY-016 --workspace . --format toon`

Expected before fix: trace or manual evidence shows `.docs/wiki/03_FL.md:69` omits `RF-QRY-016`.

**Step 2: Update FL map**

Add `RF-QRY-016` to the `FL-QRY-01` row in `.docs/wiki/03_FL.md`.

**Step 3: Verify**

Run: `mi-lsp nav trace RF-QRY-016 --workspace . --format toon`

Expected: `RF-QRY-016` maps through `FL-QRY-01` and `TP-QRY` without missing-FL warning.

### Task 1B: Define Structured Error Envelope Contract

**Objective:** Decide the public failure shape before changing CLI behavior.

**Files:**

- Modify: `.docs/wiki/09_contratos_tecnicos.md:70-90`
- Modify: `.docs/wiki/09_contratos/CT-CLI-DAEMON-ADMIN.md:136-147`
- Modify: `.docs/wiki/08_db/DB-STATE-Y-TELEMETRIA.md:110-168`
- Review: `.docs/wiki/06_pruebas/TP-QRY.md`

**Required contract decision:**

```yaml
error:
  kind: string
  code: string
  message: string
  stage: string
  hint_code: string
  retryable: bool
```

**Step 1: Document `ok=false` envelope**

Add a contract rule: command failures in machine formats must render an envelope with `ok=false`, `items=[]`, stable `error`, `hint`, `warnings`, and `stats` when available.

**Step 2: Document stderr compatibility**

State that human stderr may remain for classic mode only, but agent formats should prefer structured envelope output.

**Step 3: Document telemetry mapping**

Map `error.kind`, `error.code`, `error.stage`, and `error.hint_code` to `access_events.error_kind`, `error_code`, `failure_stage`, and `hint_code`.

**Step 4: Verify**

Run: `mi-lsp nav wiki search "structured error envelope ok=false" --workspace . --layer CT,DB,TP --format toon`

Expected: updated CT/DB/TP anchors appear in the top results.

### Task 1C: Define Wiki Evidence And Pack Anchor Contract

**Objective:** Specify that SDD anchor discovery must return line evidence and stable expansion paths.

**Files:**

- Modify: `.docs/wiki/09_contratos/CT-NAV-WIKI.md`
- Modify: `.docs/wiki/09_contratos/CT-NAV-PACK.md`
- Modify: `.docs/wiki/04_RF/RF-QRY-016.md`
- Modify: `.docs/wiki/06_pruebas/TP-QRY.md`

**Contract additions:**

- `nav wiki search` results include `evidence_start_line` and `evidence_end_line` when source text is available.
- `nav wiki pack --full --doc X` must include `X` as `docs[0]` or otherwise return `ok=false` with a typed contract failure.
- `truncated=true` must include a machine-readable continuation.

**Step 1: Add anchor-first invariant**

Document: explicit `--doc` and route primary docs are never displaced by governance/support docs.

**Step 2: Add line evidence contract**

Document result fields and fallback behavior when line ranges cannot be computed.

**Step 3: Add TP cases**

Add TP references for anchor-first pack, exact doc line evidence, and truncation continuation.

**Step 4: Verify**

Run: `mi-lsp nav wiki search "anchor-first line evidence truncation continuation" --workspace . --layer CT,RF,TP --format toon`

Expected: CT/RF/TP docs are discoverable.

### Task 1D: Define Performance And Memory SLO Contracts

**Objective:** Turn observed telemetry into explicit budgets and measurement surfaces.

**Files:**

- Modify: `.docs/wiki/07_baseline_tecnica.md`
- Modify: `.docs/wiki/07_tech/TECH-DAEMON-GOBERNANZA.md`
- Modify: `.docs/wiki/08_db/DB-STATE-Y-TELEMETRIA.md`
- Modify: `.docs/wiki/09_contratos/CT-DAEMON-WORKER.md`
- Modify: `.docs/wiki/06_pruebas/TP-DAE.md`
- Modify: `.docs/wiki/06_pruebas/TP-QRY.md`

**Initial SLO targets:**

- `nav.search`: p95 under `1500ms` on large indexed workspaces after fallback hardening.
- `nav.wiki.search`: p95 under `500ms` for indexed docs with bounded content.
- `nav.ask` docs-first preview: p95 under `1500ms`.
- `nav.pack` preview: p95 under `750ms`.
- `nav.refs` Roslyn warm: p95 under `1000ms`, cold p95 under `5000ms`.
- daemon baseline private bytes under `150MB`.
- additional warm Roslyn runtime private bytes under `350MB` unless explicitly waived by solution size.

**Step 1: Document metrics**

Add fields for `response_bytes`, `serialize_ms`, `format_ms`, `runtime_memory_bytes`, `runtime_created`, `runtime_cold_start_ms`, and `sqlite_write_ms`.

**Step 2: Document memory surfaces**

Separate daemon process memory, runtime memory, Roslyn worker memory, and historical sample memory.

**Step 3: Add TP cases**

Add test cases for p95 export, truncation metadata presence, runtime snapshot memory, and Roslyn duplicate-alias runtime reuse.

---

## Wave 2: Shared Schema Foundation, Serial

### Task 2.1: Add Shared Model Fields For All Lanes

**Objective:** Create additive types so parallel lanes do not fight over `internal/model/types.go`.

**Files:**

- Modify: `internal/model/types.go:76-90`
- Modify: `internal/output/formatter_test.go`
- Modify: `internal/output/truncator_test.go`

**Step 1: Write failing formatter tests**

Test expectations:

```go
env := model.Envelope{
    Ok: false,
    Backend: "test",
    Items: []any{},
    Error: &model.EnvelopeError{
        Kind: "workspace",
        Code: "workspace_unresolved",
        Message: "workspace not found",
        Stage: "selector_validation",
        HintCode: "workspace_unresolved",
    },
}
```

Run: `go test ./internal/output -run 'Test.*Error.*Envelope' -count=1`

Expected: FAIL before fields exist.

**Step 2: Add additive structs**

Add these model types only, with `omitempty` tags:

```go
type EnvelopeError struct {
    Kind string `json:"kind,omitempty"`
    Code string `json:"code,omitempty"`
    Message string `json:"message,omitempty"`
    Stage string `json:"stage,omitempty"`
    HintCode string `json:"hint_code,omitempty"`
    Retryable bool `json:"retryable,omitempty"`
}

type EnvelopeOmission struct {
    Input string `json:"input,omitempty"`
    Path string `json:"path,omitempty"`
    Reason string `json:"reason,omitempty"`
    ErrorCode string `json:"error_code,omitempty"`
    RequestedRange string `json:"requested_range,omitempty"`
}

type EnvelopeMetrics struct {
    ResponseBytes int `json:"response_bytes,omitempty"`
    SerializeMs int64 `json:"serialize_ms,omitempty"`
    FormatMs int64 `json:"format_ms,omitempty"`
    RuntimeMemoryBytes int64 `json:"runtime_memory_bytes,omitempty"`
    RuntimeCreated bool `json:"runtime_created,omitempty"`
    RuntimeColdStartMs int64 `json:"runtime_cold_start_ms,omitempty"`
    SQLiteWriteMs int64 `json:"sqlite_write_ms,omitempty"`
}
```

Extend `Envelope` additively:

```go
Error *EnvelopeError `json:"error,omitempty"`
Omissions []EnvelopeOmission `json:"omissions,omitempty"`
Metrics *EnvelopeMetrics `json:"metrics,omitempty"`
```

**Step 3: Verify**

Run: `go test ./internal/model ./internal/output -count=1`

Expected: PASS.

**Step 4: Merge gate**

No other lane may edit `internal/model/types.go` until this task is merged into the integration branch.

---

## Wave 3: Parallel Implementation Lanes

### Task 3A: Structured CLI Error Envelopes

**Objective:** Make command failures parseable by agents and telemetry.

**Allowed files:**

- Modify: `internal/cli/root.go:190-228`
- Modify: `cmd/mi-lsp/main.go:10-15`
- Modify: `internal/cli/root_test.go`
- Modify: `internal/cli/nav_test.go`
- Modify: `internal/telemetry/access_events_test.go`

**Forbidden files:** `internal/service/search.go`, `internal/service/wiki_search.go`, `worker-dotnet/**`.

**Step 1: Write failing CLI tests**

Scenarios:

- invalid workspace returns `ok=false` envelope in `--format toon`.
- error includes `error.kind`, `error.code`, `error.stage`, `hint_code`.
- telemetry records `error_kind`, `error_code`, `failure_stage`.
- stderr does not become the only machine-readable output for agent formats.

Run: `go test ./internal/cli ./internal/telemetry -run 'Test.*Error|Test.*Telemetry.*Error' -count=1`

Expected: FAIL.

**Step 2: Implement error normalization**

Add a small function near `executeOperation`:

```go
func errorEnvelope(request model.CommandRequest, route string, err error) model.Envelope {
    return model.Envelope{
        Ok: false,
        Workspace: request.Context.Workspace,
        Backend: route,
        Items: []any{},
        Truncated: false,
        Error: classifyOperationError(err),
        Hint: hintForOperationError(err),
    }
}
```

Keep classification minimal first: workspace unresolved, repo selector invalid, daemon backpressure, backend runtime, and unknown.

**Step 3: Render instead of returning raw error**

In `internal/cli/root.go`, when `err != nil`, build the envelope, record telemetry with that envelope, then call `printEnvelope`.

**Step 4: Keep process exit non-zero**

Preserve non-zero exit for failed commands. If cobra/main needs this, return a sentinel after printing only in human/classic mode. Do not print duplicate JSON/TOON.

**Step 5: Verify**

Run: `go test ./internal/cli ./internal/output ./internal/telemetry -count=1`

Expected: PASS.

### Task 3B: Robust `rg` Fallback And Faster Content Enrichment

**Objective:** Prevent `rg.exe` access failures from breaking `nav.search` and reduce N+1 file reads.

**Allowed files:**

- Modify: `internal/service/search.go:76-168`
- Modify: `internal/service/search_content.go:15-127`
- Modify: `internal/service/app_test.go`
- Add or modify: `internal/service/search_test.go`

**Step 1: Write failing fallback tests**

Scenarios:

- `rg` missing uses Go fallback.
- `rg` `Access is denied` uses Go fallback with warning/hint code.
- `rg` sees `.mi-lsp/index.db-wal` or `.mi-lsp/index.db-shm` errors and does not fail the whole query.
- fallback item shape matches rg item shape.

Run: `go test ./internal/service -run 'TestSearch.*Rg|TestSearch.*Fallback' -count=1`

Expected: FAIL.

**Step 2: Add fallback-eligible error detection**

Add a helper in `search.go`:

```go
func isRgFallbackEligible(err error) bool {
    if err == nil {
        return false
    }
    msg := strings.ToLower(err.Error())
    return strings.Contains(msg, "access is denied") ||
        strings.Contains(msg, "permission denied") ||
        strings.Contains(msg, "cannot find the file") ||
        strings.Contains(msg, "locked")
}
```

**Step 3: Use fallback on `Start` and selected `Wait` errors**

In `searchPatternScoped`, if `searchPatternRg` returns fallback-eligible error, call `searchPatternFallback` and attach a warning through caller plumbing.

If current function shape cannot return warnings, add a small internal result struct only inside service package. Do not change public output shape until Task 3A fields exist.

**Step 4: Batch content enrichment by file**

Group matches by file, read each file once, and slice all requested ranges from the in-memory lines. Keep symbol lookup optional and bounded.

**Step 5: Verify**

Run: `go test ./internal/service -run 'TestSearch|TestNavSearch' -count=1`

Expected: PASS.

### Task 3C: Wiki Search Evidence And Pack Anchor-First

**Objective:** Make SDD anchor discovery return authoritative line ranges and ensure pack full mode never drops the anchor.

**Allowed files:**

- Modify: `internal/service/wiki_search.go:81-122`
- Modify: `internal/service/pack.go:250-300`
- Modify: `internal/service/wiki_search_test.go`
- Modify: `internal/service/pack_test.go`
- Modify: `internal/service/route_test.go`

**Step 1: Write failing pack invariant tests**

Scenarios:

- `nav wiki pack --doc .docs/wiki/03_FL/FL-QRY-01.md --full` includes that doc as first pack doc.
- route primary doc remains first in preview and full modes.
- governance docs can appear as support, never replace explicit anchor.

Run: `go test ./internal/service -run 'Test.*Pack.*Anchor|Test.*Pack.*Full' -count=1`

Expected: FAIL.

**Step 2: Write failing wiki evidence tests**

Create a fixture doc with a match at known lines. Assert result includes `evidence_start_line`, `evidence_end_line`, and bounded snippet/content.

Run: `go test ./internal/service -run 'TestWikiSearch.*Evidence' -count=1`

Expected: FAIL.

**Step 3: Implement anchor-first item selection**

Before iterating stage specs in `pack.go`, append the primary doc once with `Stage: "anchor"` when available. Skip it later via `seen`.

**Step 4: Implement line evidence**

When building a wiki result, compute the first matched line range from loaded doc content or source block metadata. Prefer source blocks when available; fall back to scanning normalized content.

**Step 5: Verify**

Run: `go test ./internal/service -run 'TestWikiSearch|Test.*Pack|Test.*Route' -count=1`

Expected: PASS.

### Task 3D: Truncation Continuations And Multi-Read Omissions

**Objective:** Make dropped/omitted content explicit and actionable for agents.

**Allowed files:**

- Modify: `internal/output/truncator.go:11-58`
- Modify: `internal/service/multi_read.go`
- Modify: `internal/output/truncator_test.go`
- Modify: `internal/service/multi_read_test.go`

**Step 1: Write failing truncator tests**

Scenarios:

- char budget truncation records omitted item count/path when possible.
- single bulky item gets item-level continuation or next hint.
- telemetry truncation reason is explicit, not inferred only from `count >= maxItems`.

Run: `go test ./internal/output -run 'Test.*Truncat' -count=1`

Expected: FAIL.

**Step 2: Write failing multi-read tests**

Scenarios:

- three requested ranges, two emitted, one omitted due to budget.
- invalid newline path becomes typed omission or typed error without OS-specific raw leakage.
- access denied/missing file records safe omission metadata.

Run: `go test ./internal/service -run 'Test.*MultiRead.*Omission|Test.*MultiRead.*Budget' -count=1`

Expected: FAIL.

**Step 3: Implement omissions**

Populate `Envelope.Omissions` for known dropped file/range inputs. Do not include sensitive absolute paths unless already in the safe workspace-relative item.

**Step 4: Implement continuation**

For omitted ranges, set `continuation.next` to `op=nav.multi-read` with the remaining ranges where the current model supports it. If model shape is too small, use `next_hint` plus `omissions`.

**Step 5: Verify**

Run: `go test ./internal/output ./internal/service -run 'Test.*Truncat|Test.*MultiRead' -count=1`

Expected: PASS.

### Task 3E: Workspace Path/Alias Agent UX

**Objective:** Stop agents from following invalid alias next steps or falling back to unrelated `last_workspace`.

**Allowed files:**

- Modify: `internal/service/workspace_ops.go:279-288`
- Modify: `internal/service/workspace_resolution_test.go`
- Modify: `internal/cli/root_test.go`
- Modify: `skills/mi-lsp/SKILL.md` only if the implementation changes agent guidance

**Step 1: Write failing tests**

Scenarios:

- `workspace status .` path-resolves current repo and emits next steps that work in read-only mode.
- if alias is not registered, next steps use `--workspace .` or include an explicit alias-not-registered warning.
- omitted workspace from cwd with `.docs/wiki` does not silently use unrelated `last_workspace`.

Run: `go test ./internal/service ./internal/cli -run 'Test.*Workspace.*Path|Test.*LastWorkspace|Test.*NextSteps' -count=1`

Expected: FAIL.

**Step 2: Make next steps path-safe**

When workspace source is path-based, prefer `--workspace .` in emitted next steps or add a stable warning that alias registration is unavailable.

**Step 3: Fail closed on unsafe omitted workspace**

If cwd has `.git`, `.mi-lsp`, or `.docs/wiki` but cannot resolve a registered alias, return a typed hint to use `--workspace .` instead of using unrelated `last_workspace`.

**Step 4: Verify**

Run: `go test ./internal/service ./internal/cli -run 'Test.*Workspace|Test.*Root' -count=1`

Expected: PASS.

### Task 3F: Access Telemetry, Log Evidence, And Runtime Metrics

**Objective:** Make telemetry explain latency, truncation, response size, and runtime/memory attribution.

**Allowed files:**

- Modify: `internal/model/types.go` only if Wave 2 did not already add required fields
- Modify: `internal/telemetry/access_diagnostics.go:41-70`
- Modify: `internal/daemon/state_store.go:224-260`
- Modify: `internal/daemon/export.go:54-74`
- Modify: `internal/daemon/admin.go`
- Modify: `internal/daemon/state_store_test.go`
- Modify: `internal/daemon/export_test.go`
- Modify: `internal/telemetry/access_events_test.go`

**Step 1: Write failing DB migration tests**

Assert new nullable columns round-trip:

- `response_bytes`
- `serialize_ms`
- `format_ms`
- `runtime_memory_bytes`
- `runtime_created`
- `runtime_cold_start_ms`
- `sqlite_write_ms`

Run: `go test ./internal/daemon ./internal/telemetry -run 'Test.*Access|Test.*Export|Test.*StateStore' -count=1`

Expected: FAIL.

**Step 2: Add idempotent migrations**

Extend `access_events` with nullable/default columns. Keep legacy reads null-safe.

**Step 3: Record response and format metrics**

Capture serialized response bytes and formatting duration in the CLI/output path. Keep overhead low and best-effort.

**Step 4: Export metrics**

Add p50/p95 summaries for response bytes and format latency where useful. Do not include raw patterns or payloads.

**Step 5: Verify**

Run: `go test ./internal/daemon ./internal/telemetry ./internal/cli -count=1`

Expected: PASS.

### Task 3G: Roslyn Runtime Memory Bounds

**Objective:** Prevent warm Roslyn workers from growing without visibility or eviction pressure.

**Allowed files:**

- Modify: `internal/daemon/lifecycle.go`
- Modify: `internal/daemon/lifecycle_windows.go`
- Modify: `internal/daemon/lifecycle_unix.go`
- Modify: `internal/daemon/lifecycle_runtime_key_test.go`
- Modify: `worker-dotnet/MiLsp.Worker/RoslynService.cs`
- Modify: `worker-dotnet/MiLsp.Worker.Tests/*` if test project exists

**Step 1: Write fake runtime memory tests**

Scenarios:

- runtime snapshot records memory.
- duplicate aliases for same root do not create duplicate runtime memory rows.
- over-budget idle runtime is evicted before lower-memory active runtime.

Run: `go test ./internal/daemon -run 'Test.*Runtime|Test.*Memory|Test.*Evict' -count=1`

Expected: FAIL for new budget behavior.

**Step 2: Add daemon memory thresholds**

Add config/env defaults only after contract docs are updated. Suggested names:

- `MI_LSP_DAEMON_MAX_RUNTIME_MEMORY_MB`
- `MI_LSP_DAEMON_MAX_TOTAL_RUNTIME_MEMORY_MB`

**Step 3: Add memory-aware eviction**

Evict idle LRU runtimes when total runtime memory exceeds threshold. Do not kill active runtime mid-request.

**Step 4: Bound Roslyn workspace cache**

Inside the .NET worker, add a small cache bound or explicit clear/evict mechanism. If this is too risky, expose cache count and memory evidence first, then defer hard eviction.

**Step 5: Verify**

Run: `go test ./internal/daemon -count=1`

Run: `dotnet test worker-dotnet/MiLsp.Worker.sln`

Expected: PASS.

### Task 3H: Benchmark And Regression Harness

**Objective:** Add repeatable measurements so future agents know whether hardening worked.

**Allowed files:**

- Add: `internal/service/search_bench_test.go`
- Add: `internal/service/wiki_search_bench_test.go`
- Add: `internal/service/pack_bench_test.go`
- Add: `internal/daemon/telemetry_bench_test.go`
- Modify: `internal/daemon/perf_smoke.go`
- Modify: `scripts/release/regression-smoke.ps1`

**Step 1: Add service benchmarks**

Benchmarks:

- `BenchmarkNavSearchSmallLiteral`
- `BenchmarkNavSearchMediumIncludeContent`
- `BenchmarkNavWikiSearchExactDocID`
- `BenchmarkNavWikiSearchContentQuery`
- `BenchmarkNavPackPreview`
- `BenchmarkNavPackFull`

**Step 2: Add daemon telemetry benchmark**

Benchmark access event write overhead and export summary over 1000 events.

**Step 3: Add regression smoke for path workspace**

Extend release regression to include:

- `workspace status .`
- `nav governance --workspace .`
- path-resolved next step validity

**Step 4: Verify**

Run: `go test ./internal/service ./internal/daemon -run '^$' -bench 'BenchmarkNav|Benchmark.*Telemetry' -benchtime=1x`

Expected: benchmarks compile and run.

---

## Wave 4: Integration And Full Verification, Serial

### Task 4.1: Merge Lanes And Resolve Conflicts

**Objective:** Integrate parallel work without losing lane-specific tests or docs.

**Files:** All files touched by Waves 1 through 3.

**Step 1: Merge foundation first**

Merge Wave 1 and Wave 2 before code lanes.

**Step 2: Merge low-conflict lanes**

Suggested order:

1. Workspace UX
2. Search fallback
3. Wiki pack/search
4. Truncation/multi-read
5. Error envelope
6. Telemetry/log evidence
7. Roslyn memory
8. Benchmarks

**Step 3: Resolve model conflicts once**

Only the orchestrator resolves conflicts in `internal/model/types.go` and telemetry schema files.

### Task 4.2: Full Test Suite

**Objective:** Prove the integrated branch is stable.

**Step 1: Run Go tests**

Run: `go test ./...`

Expected: PASS.

**Step 2: Run .NET worker tests/build**

Run: `dotnet test worker-dotnet/MiLsp.Worker.sln`

Expected: PASS.

**Step 3: Run targeted CLI smokes**

Run: `mi-lsp workspace status . --format toon`

Run: `mi-lsp nav governance --workspace . --format toon`

Run: `mi-lsp nav wiki search "RF-QRY-016" --workspace . --layer RF --format toon`

Run: `mi-lsp nav wiki pack "RF-QRY-016" --workspace . --full --format toon`

Expected: valid envelopes, no raw stderr-only failures, anchor-first pack.

### Task 4.3: Performance Evidence

**Objective:** Compare against the baseline and document the result.

**Step 1: Export telemetry summary**

Run: `mi-lsp daemon export --summary --window recent --format toon`

Expected: operation percentiles and top errors are visible.

**Step 2: Run perf smoke**

Run: `mi-lsp daemon perf-smoke --callers 16 --watch-mode lazy --max-working-set-mb 250 --max-private-mb 300 --max-handles 5000`

Expected: PASS or documented waiver if Roslyn runtime memory is intentionally out of idle-daemon scope.

**Step 3: Run benchmarks**

Run: `go test ./internal/service ./internal/daemon -run '^$' -bench 'BenchmarkNav|Benchmark.*Telemetry' -benchtime=3x`

Expected: benchmark output captured in audit evidence.

### Task 4.4: Traceability Closure

**Objective:** Close SDD and runtime-sensitive traceability before PR.

**Step 1: Run traceability**

Use `ps-trazabilidad`.

Required chain:

- `00 -> FL-QRY-01 -> RF-QRY-001/RF-QRY-010/RF-QRY-012/RF-QRY-014/RF-QRY-016 -> CT-NAV-* -> TP-QRY`
- `00 -> FL-DAE-01 -> RF-DAE-002 -> TECH-DAEMON-GOBERNANZA -> DB-STATE-Y-TELEMETRIA -> CT-CLI-DAEMON-ADMIN/CT-DAEMON-WORKER -> TP-DAE`
- `00 -> FL-CS-01 -> RF-CS-001 -> CT-DAEMON-WORKER -> TP-CS`

**Step 2: Run audit**

Use `ps-auditar-trazabilidad` because this is runtime-sensitive and multi-module.

**Step 3: Run pre-push gate before any push to main**

Use `ps-pre-push` if a push to `main` is intended. If no tracker card exists, capture explicit waiver or create a card before push.

---

## Subagent Dispatch Packets

Use these prompts after Wave 0 and the relevant dependencies are complete.

### Packet: Contracts Lane

Task: Contract freeze for mi-lsp agent/runtime hardening.
Scope: `.docs/wiki/03_FL.md`, `.docs/wiki/09_contratos_tecnicos.md`, `.docs/wiki/09_contratos/*.md`, `.docs/wiki/08_db/DB-STATE-Y-TELEMETRIA.md`, `.docs/wiki/07_tech/TECH-DAEMON-GOBERNANZA.md`, `.docs/wiki/06_pruebas/TP-*.md`.
Goal: Make docs define structured errors, wiki evidence ranges, pack anchor-first behavior, telemetry fields, and performance/memory SLOs.
Constraints: Do not edit Go or C# code. Preserve governance. Use TOON validation commands.
Return: changed docs, anchor map, validation commands, residual blockers.

### Packet: Error Envelope Lane

Task: Implement structured `ok=false` envelopes.
Scope: `internal/cli`, `cmd/mi-lsp`, `internal/output`, `internal/telemetry`.
Goal: Machine formats render typed error envelopes and telemetry persists typed fields.
Constraints: Do not edit search/wiki/Roslyn files. Depend on Wave 2 model fields.
Return: tests added, behavior changed, compatibility risk, verification output.

### Packet: Search Lane

Task: Harden `nav.search` fallback and content performance.
Scope: `internal/service/search.go`, `internal/service/search_content.go`, search tests.
Goal: `rg` access failures degrade to fallback and include-content reads files once per file.
Constraints: Do not edit output formatter or wiki search.
Return: root cause, tests, latency expectation, residual risk.

### Packet: Wiki Lane

Task: Harden `nav wiki search` evidence and `nav wiki pack` anchor behavior.
Scope: `internal/service/wiki_search.go`, `internal/service/pack.go`, wiki/pack/route tests.
Goal: line evidence is available and `--full` never drops primary anchor.
Constraints: Do not edit search fallback or telemetry schema.
Return: tests, line-range behavior, pack invariant evidence.

### Packet: Truncation Lane

Task: Make truncation and multi-read omissions actionable.
Scope: `internal/output/truncator.go`, `internal/service/multi_read.go`, related tests.
Goal: omitted ranges/items are explicit and continuations are machine-readable.
Constraints: Do not edit CLI error rendering.
Return: omission schema usage, test output, known limitations.

### Packet: Workspace UX Lane

Task: Fix path workspace and last-workspace agent UX.
Scope: workspace resolution, `workspace status`, next steps, related docs if needed.
Goal: read-only agents can use `--workspace .` safely and are not routed to stale aliases.
Constraints: Do not run `mi-lsp init` unless explicitly allowed.
Return: path/alias cases covered, warnings/hints added, tests.

### Packet: Telemetry Lane

Task: Add response/runtime telemetry and export summaries.
Scope: `internal/telemetry`, `internal/daemon`, admin/export tests.
Goal: explain latency/truncation/memory with structured local telemetry.
Constraints: No raw payload/pattern logging. Keep migrations idempotent.
Return: schema diff, privacy review, export examples, tests.

### Packet: Roslyn Memory Lane

Task: Add runtime memory budgets and Roslyn cache visibility/bounds.
Scope: `internal/daemon/lifecycle*`, `worker-dotnet/MiLsp.Worker`, daemon/Roslyn tests.
Goal: avoid unbounded worker memory and expose budget evidence.
Constraints: Do not break warm semantic queries. Prefer observability before aggressive eviction.
Return: memory behavior, tests, cold/warm risk, follow-up if needed.

### Packet: Bench Lane

Task: Add benchmark and release regression coverage.
Scope: `*_bench_test.go`, `perf_smoke`, `scripts/release/regression-smoke.ps1`.
Goal: make performance and path-workspace regressions visible.
Constraints: Benchmarks must not be required in normal unit test CI unless explicitly configured.
Return: benchmark commands, sample output, thresholds.

---

## Definition Of Done

Implementation is done only when all are true:

- Governance remains unblocked and in sync.
- Docs index is ready or the final evidence explains why only source checkout state is being reported.
- `RF-QRY-016` traceability drift is fixed.
- Structured `ok=false` envelope exists for machine formats.
- `rg` access denied no longer causes hard `nav.search` failure when fallback can scan.
- `nav wiki pack --full --doc X` includes `X`.
- `nav wiki search` can provide file/line evidence for matches.
- Truncated outputs include actionable continuation or omissions.
- Workspace path mode emits safe next steps.
- Access telemetry captures enough fields to explain slow/truncated/error-heavy operations without storing raw payloads.
- Roslyn runtime memory has at least visibility and preferably budget-aware eviction for idle runtimes.
- `go test ./...` passes.
- `dotnet test worker-dotnet/MiLsp.Worker.sln` passes or the absence of test project is documented with a build substitute.
- `ps-trazabilidad` passes.
- `ps-auditar-trazabilidad` is approved or approved with explicitly accepted follow-ups.

---

## Known Risks

- Error envelope changes can break scripts that expect stderr-only failures. Keep exit codes and document compatibility.
- `rg` fallback can be slower than ripgrep. Cache `rg` health briefly and expose the fallback in telemetry.
- Anchor-first pack may change output order for users who relied on governance-first packs. Treat this as intended for agent workflows.
- Line evidence from normalized Markdown may drift if source blocks and raw file lines diverge. Prefer raw file line scanning when possible.
- Memory-aware eviction can increase cold-start latency. Evict only idle runtimes first and expose cold/warm telemetry.
- Bench thresholds should start as guardrails, not hard CI blockers, until enough local telemetry exists.
