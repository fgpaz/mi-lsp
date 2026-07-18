# Roadmap graph-native SDD-A — zero-inference plan

## Purpose, authority and locked decisions

This execution plan turns FL-GPH-01, FL-GPH-02 and FL-GPH-03 into implementation waves without creating RF, TP, TECH, DB or CT identifiers in SDD-A. Human authority remains `.docs/wiki/00_gobierno_documental.md`; scope, architecture and flow authority remain `.docs/wiki/01_alcance_funcional.md`, `.docs/wiki/02_arquitectura.md`, `.docs/wiki/03_FL*` and the FL-GPH documents. This file is execution guidance, not canon.

Locked decisions:

- CLI/core-first, local, read-only at query time; daemon is optional and direct mode is the fallback.
- No MCP, HTTP, remote graph service, implicit network dependency, or extension-owned authority. No push, deploy or publish is authorized by any wave.
- Compiler/semantic evidence is authoritative for supported backends. Text search may locate candidates but may not assert semantic identity, edge, impact or completeness.
- Backend order is C#/Roslyn first, Go second; TypeScript and Python are gated until their semantic evidence and adapters pass the same acceptance contract. Unsupported or partial backends remain explicit omissions.
- Every graph result is generation-aware, provenance-bearing and bounded by depth, result and token budgets.
- Unknown, unsupported, stale, ambiguous or partial work is represented explicitly as unresolved/blocked; it is never silently promoted to fact. Evidence and omission reasons are first-class output.
- G0 alone may establish graph-native names. Later packets reuse the locked names and may not invent RF/TP/TECH/DB/CT IDs.
- NodeKey identity and collision handling are a hard gate: missing identity fields, normalization disagreement or a hash collision produce unresolved/blocked output and stop publication.
- SQLite is the authoritative adjacency store. The core must not materialize the complete graph in RAM and must not use NetworkX as a core dependency; traversal is bounded and storage-backed.
- No worker edits `main`, governance projection, secrets or `.mi-lsp/**`; each packet is isolated and path-bounded.
- No benchmark claim is valid without a pinned fixture, command, baseline, raw result and comparison. Unavailable measurements are BLOCKED.

## Fixed references, comparison and evidence contract

- Exact implementation base: `a251ab1f8db4e96f029926fbef275b078a20a111`.
- Exact Graphify comparator: `9bf14a4931658152969586ace39eb965c010f0d1`.
- Every relevant benchmark runs 30 repetitions per variant, with the same fixture, environment, command and warm/cold protocol. Compare simultaneously against Graphify and the previous mi-lsp baseline; never substitute one comparator for the other.
- Required benchmark outputs: token count, warm p95, peak RSS, incrementality, correctness, precision, recall and determinism. Record raw samples, aggregation method, fixture digest, commit/base, environment, command and unavailable metrics.
- Cross-RID is mandatory: every node, edge, evidence, unresolved result, query and benchmark row carries or deterministically resolves its cross-RID; missing or conflicting cross-RID blocks acceptance.
- Evidence is sanitized, durable and omission-aware under the active audit session. Evidence may prove implementation status but never outrank wiki authority. Screenshots, prose and API-only checks cannot produce PASS.

## Branch, worktree, worker and packet contract

Canonical branch: `worker/w1-sdd-canon`. Implementation branches/worktrees are `worker/gph-g0` through `worker/gph-g10`, created from the exact integration base above and integrated only in this order: G0 → G1 → G2/G3 → G4 → G5/G6 → G7 → G8 → G9 → G10. This wave owns only SDD-A docs and this plan.

Workers must use isolated worktrees, one bounded packet per decision, and no direct edits to `main`. Each packet must contain: one decision; exact input/output paths resolved from the current tree; preconditions; allowed-path list; deterministic test command; evidence path under the active audit session; rollback action; `next_action`; and a stop condition. Workers report a structured result, the orchestrator performs joins before opening the next wave, and commits are atomic and path-bounded. No packet may push, deploy or publish. A packet that cannot resolve an exact path stops rather than guessing. Partial work stays isolated, is marked incomplete, is not active, merged, queried or benchmark truth, and is either completed, reverted or disposed with its sanitized evidence retained.

## Graph contract locked by G0/G1

- `GraphGeneration`: immutable generation id, workspace identity, schema version, source fingerprint, compiler/backend versions, created-at metadata and status (`staged|active|retired|invalid`).
- `NodeKey`: canonical tuple `{workspace_root, backend_type, project_or_module, symbol_kind, canonical_name, declaration_uri, declaration_range}`; serialization is normalized and hashed. Missing identity fields, non-deterministic normalization or collisions produce unresolved records, not guessed keys.
- Node record: `node_key`, kind, display name, declaration location, source fingerprint, backend/compiler provenance, confidence/status, cross-RID and generation id.
- Edge record: `from`, `to`, relation, source location/evidence, provenance, confidence/status, cross-RID and generation id. Inferred edges are visibly marked and never presented as compiler facts.
- Evidence record: source URI/range, backend, extractor version, digest, observed claim, cross-RID and generation id. Unresolved record: reason code, selector/input, affected generation, cross-RID and recovery hint.
- SQLite adjacency tables are authoritative for reads and bounded traversal. No complete in-memory graph and no NetworkX core. Schema migration is versioned and additive/transactional: dual-read/dual-write during the compatibility window, read old version, stage new version, validate, atomically activate, retain rollback metadata, then retire old writes only after evidence; no destructive in-place migration and no query-time migration.
- Generations are staged, validated and atomically published. Readers see the prior active generation until the new pointer is committed; crash recovery restores the prior pointer and preserves evidence.
- `MILX-v1` is an isolated, capability-allowlisted host/pack/result contract. Packs are bounded, serialized and provenance-bearing; MILX cannot write the primary graph or become authority.

## Wave map and milestones

| Wave | Milestones | Decision and dependency | Allowed scope | Join / stop |
|---|---|---|---|---|
| W0 authority and evidence freeze | G0 | Freeze authority, references, names, paths, baselines and evidence schema before implementation. | Audit/session packet paths only; no implementation and no canonical wiki mutation. | Join G0 manifest and decision lock. Stop on governance block, collision, missing base/comparator, missing baseline or scope drift. |
| W1 graph model | G1 | Define identity, records, provenance, omission, cross-RID and migration contract. | Existing `cmd/`, `internal/` graph/model/storage roots and matching existing tests, exact files listed by G0. | Join model tests and schema fixture. Stop on NodeKey ambiguity, collision or authority drift. |
| W2 compiler evidence | G2, G3 | Implement C#/Roslyn then Go adapters, with TS/Python gated; stage and validate generations. | Existing backend integration and graph/index roots plus mirrored tests, exact files predeclared by G0/G1. | Join adapter and staging evidence. Stop on text-only semantic claim, silent omission or invalid staged output. |
| W3 persistence and queries | G4, G5, G6 | Publish SQLite adjacency atomically; add bounded query and explain/impact. | Existing index/storage, CLI/core and query roots plus matching tests. | Join crash/migration/query evidence. Stop on active-pointer corruption, unbounded RAM traversal or authority mutation. |
| W4 MILX and compatibility | G7, G8 | Add isolated MILX-v1 packs/results and stable no-MCP CLI envelopes. | Existing extension/host and CLI roots plus matching tests; no network capability. | Join security, compatibility and daemon-absent evidence. Stop on capability escape, MCP/network dependency or daemon requirement. |
| W5 quality and benchmark lab | G9 | Freeze and execute correctness/performance comparison against both exact references. | Pinned benchmark lab/fixture paths resolved by G0; no unpinned result. | Join raw 30-repetition evidence and verdict. Stop on failed test, missing baseline, unavailable metric or threshold failure. |
| W6 closure and local disposition | G10 | Traceability, audits, release-distribution readiness and rollback note without external publication. | Active audit/closure paths and approved packet paths only. | Join `ae-close`, cross-ref and traceability verdicts. Stop on FAIL/BLOCKED, forbidden path or any push/deploy/publish attempt. |

## Wave task packets — zero inference

### W0 — authority and evidence freeze

#### G0 authority-freeze

- Preconditions: governance status is not blocked; `.docs/wiki/00_gobierno_documental.md` and `.docs/wiki/_mi-lsp/read-model.toml` are synchronized; exact base `a251ab1f8db4e96f029926fbef275b078a20a111` and Graphify `9bf14a4931658152969586ace39eb965c010f0d1` resolve; active audit session and its `audit-manifest.yaml` exist or are created under `.docs/auditoria/<session>/` with `schema: ae-audit-hygiene/v1`, `retention_ttl_days: 14`, `hash_algorithm: sha256`.
- Inputs: `.docs/wiki/00_gobierno_documental.md`, `.docs/wiki/01_alcance_funcional.md`, `.docs/wiki/02_arquitectura.md`, `.docs/wiki/03_FL.md`, `.docs/wiki/03_FL/FL-GPH-01.md`, `.docs/wiki/03_FL/FL-GPH-02.md`, `.docs/wiki/03_FL/FL-GPH-03.md`, `.docs/raw/plans/2026-07-18-milsp-graph-native-roadmap.md`, the two pinned commits and the benchmark lab paths discovered from the current tree.
- Outputs: only `.docs/auditoria/<session>/g0-manifest.yaml`, sanitized baseline/reference records, decision lock and packet evidence; do not edit any input wiki, implementation file, `.mi-lsp/**` or governance projection.
- Steps: validate wiki authority and cross-references; inventory existing graph names without inventing RF/TP/TECH/DB/CT IDs; verify exact commits and worktree roots; pin fixture, command, environment and baseline identifiers; define cross-RID/evidence/omission fields; record allowed paths and forbidden paths; create the wave packet and worker join record.
- Tests: `mi-lsp nav governance --workspace C:\repos\mios\mi-lsp-w1-sdd --format toon`; ID/cross-reference scan; `git diff --check`; exact commit and path checks; baseline availability check.
- Acceptance: manifest is reproducible, authority is explicit, both comparators and previous baseline are pinned, no invented SDD IDs exist, every later path is exact or explicitly gated, and evidence records omissions instead of guessing.
- Rollback: discard the W0 packet and retain only sanitized failure evidence; do not proceed to G1.
- Next action: join G0, then create isolated `worker/gph-g1` from the exact base. Stop on any failed precondition.

### W1 — graph model

#### G1 graph-model

- Preconditions: G0 join is PASS; exact model/storage/test paths are listed in the packet; no NodeKey collision or naming gap remains unresolved.
- Allowed paths: only the exact existing graph model, storage schema and matching test files under `cmd/` and `internal/` declared by G0; no `.mi-lsp/**`, wiki authority or governance projection.
- Steps: define `GraphGeneration`, `NodeKey`, node/edge/evidence/unresolved records; implement normalized serialization, hash and collision detection; define cross-RID derivation; define SQLite adjacency schema and indexes; define additive transactional migration with dual-read/write, validation, atomic activation and rollback metadata; define omission/status taxonomy; add schema fixtures.
- Tests: deterministic identity/property tests; normalization and collision tests; provenance/cross-RID tests; unresolved/omission tests; SQLite adjacency and bounded traversal fixtures; migration dual-read/write, rollback and old-version read tests.
- Acceptance: same input produces byte-identical identity and cross-RID; all required fields are present; collisions block publication; SQLite is authoritative; no complete graph is held in RAM; NetworkX is absent from core dependencies; migration is additive, transactional and reversible.
- Rollback: revert only the atomic G1 packet and remove staged schema artifacts; preserve active prior schema and evidence.
- Next action: join G1 and dispatch W2 adapters. Stop on non-determinism, guessed identity, missing rollback or authority movement.

### W2 — compiler evidence and generation staging

#### G2 compiler-adapters

- Preconditions: G1 contract and fixtures PASS; exact adapter roots are resolved. C#/Roslyn adapter is implemented and tested before Go; TS/Python remain gated unless their semantic backends and fixtures are available.
- Allowed paths: exact existing C#/Roslyn and Go backend integration files first, then explicitly approved TS/Python adapter files only if G0 evidence proves support, plus mirrored tests and fixture paths. No text-search implementation may assert semantic edges.
- Steps: extract compiler-backed declarations/relations with source ranges, versions, digests, cross-RIDs and coverage; emit unsupported/partial backend omissions; compare C#/Roslyn then Go deterministic reruns; gate TS/Python behind the same evidence contract; never claim completeness from text search.
- Tests: C#/Roslyn fixture tests; Go fixture tests; unsupported TS/Python tests; provenance, cross-RID, omission and deterministic rerun tests; text-only negative tests.
- Acceptance: C#/Roslyn and Go pass authoritative evidence checks; TS/Python are either gated with explicit reason or pass the same contract; no silent backend omission or semantic assertion from text search.
- Rollback: remove only the failing adapter packet and retain G1 model; do not downgrade compiler evidence to text evidence.
- Next action: join G2 and dispatch G3. Stop on ambiguous compiler output, missing provenance or unsupported backend presented as complete.

#### G3 generation-staging

- Preconditions: G2 adapter join is PASS; exact staging, validator and unresolved-record paths are declared under existing graph/index roots.
- Allowed paths: those exact staging/validation files and matching tests only.
- Steps: stage all nodes, edges and evidence in a new `GraphGeneration`; validate NodeKey uniqueness, edge endpoints, source fingerprints, backend versions, cross-RIDs and omission states; mark partial generations; keep staging invisible to readers; produce fixture digest and staging evidence.
- Tests: invalid node/edge, duplicate/collision, cancellation, partial backend, stale source, missing cross-RID, malformed evidence and deterministic staging tests.
- Acceptance: invalid, stale, partial or ambiguous work is unresolved/blocked; no staged generation is active; all accepted records are provenance-bearing and cross-RID-resolvable.
- Rollback: delete staged generation only, preserve the active generation and retain sanitized validation evidence.
- Next action: join G3 and create W3 persistence packet. Stop on any staged record leaking to query.

### W3 — persistence and bounded queries

#### G4 atomic-publish-rollback

- Preconditions: G3 staged generation PASS; SQLite schema and migration fixtures PASS; exact persistence/migration paths are declared; no `.mi-lsp/**` edits.
- Allowed paths: existing index/storage implementation and matching tests only.
- Steps: write staged adjacency to SQLite; dual-read/dual-write during migration; validate counts, keys, edges, evidence and cross-RIDs; atomically switch the active-generation pointer; retain immutable backup and rollback metadata; exercise crash recovery and concurrent readers; never migrate at query time.
- Tests: crash-window, pointer atomicity, concurrent-reader, restore, old/new dual-read/write, migration rollback, staged cleanup and active-generation preservation.
- Acceptance: readers see either old or new valid generation; crash restores prior active pointer; SQLite remains authoritative; no in-memory full graph or NetworkX core; rollback is deterministic.
- Rollback: atomically restore prior pointer, delete staged rows, retain backup metadata and failure evidence.
- Next action: join G4 and dispatch G5/G6. Stop on corruption, query-time migration or non-atomic visibility.

#### G5 graph-query

- Preconditions: G4 PASS with an active SQLite generation; exact command/service/envelope paths are declared.
- Allowed paths: existing CLI/core query files and matching tests only.
- Steps: implement deterministic selectors, bounded depth/result/token limits, generation selector, direct-mode fallback and explicit unresolved/omission output; read only from SQLite adjacency; do not mutate the index.
- Tests: selector missing/ambiguous, retired generation, depth/result/token limits, daemon absent, direct mode, cross-RID and deterministic output tests.
- Acceptance: bounded generation-aware read-only query returns provenance and omissions; daemon is optional; no query-time write or full graph materialization.
- Rollback: revert the query packet and leave the active generation unchanged.
- Next action: join G5, then validate G6. Stop on unbounded traversal or silent omission.

#### G6 explain-impact

- Preconditions: G5 PASS; exact explain/impact files and tests are declared.
- Allowed paths: existing query/explain roots and matching tests only.
- Steps: add read-only evidence-chain, explain and impact traversal with explicit inferred status, bounded depth/token budgets, unresolved/ambiguous handling and wiki links as references only; record cross-RIDs.
- Tests: evidence chain, bounded traversal, ambiguity, unresolved, inferred-edge labeling, budget and fixed-fixture token comparison against G5 baseline.
- Acceptance: every claim has evidence or explicit omission; inferred edges are never compiler facts; traversal is storage-backed and bounded; wiki authority is not mutated.
- Rollback: return a bounded warning, revert only G6 and keep the graph unchanged.
- Next action: join G6 and create W4 packet. Stop on unsupported impact presented as fact.

### W4 — MILX-v1 and CLI compatibility

#### G7 MILX host

- Preconditions: G4 active generation and G5/G6 read-only contracts PASS; exact extension/host paths are declared; no MCP/network capability is available.
- Allowed paths: existing extension/host, pack/result and matching test roots only.
- Steps: implement `MILX-v1` isolated process contract; enforce capability allowlist, timeout, serialized pack/result schema, cross-RID/provenance, cleanup budget and host-only termination; prohibit host writes to the primary graph and prohibit network/MCP.
- Tests: allowlist denial, timeout, malformed result, crash, cleanup, no-network, no-MCP, host RSS/termination and pack determinism tests.
- Acceptance: host is isolated, bounded and disposable; malformed/crashed MILX yields a warning and preserves the graph; packs are provenance-bearing and cannot become authority.
- Rollback: terminate only the host, preserve active graph, discard staged host output and retain bounded warning evidence.
- Next action: join G7 and dispatch G8. Stop on capability escape, network/MCP use or graph write.

#### G8 CLI/output/no-MCP

- Preconditions: G7 PASS; exact CLI flag, envelope and compatibility paths are declared.
- Allowed paths: existing CLI/output/compatibility files and matching tests only.
- Steps: define stable flags/envelopes, generation/cross-RID/omission fields, daemon-optional direct path, pack boundaries and explicit no-MCP/no-network checks; preserve compatibility or record a blocked incompatibility.
- Tests: golden envelopes, compatibility, daemon absent, exit codes, pack bounds, no-MCP/network scans and query read-only checks.
- Acceptance: stable bounded output is available without daemon, MCP or network; compatibility behavior is explicit; output tokens are measurable.
- Rollback: revert output packet if compatibility is undefined or daemon becomes required; keep prior CLI behavior.
- Next action: join G8 and dispatch W5 benchmark lab. Stop on implicit network/MCP or hidden schema change.

### W5 — quality and benchmark lab

#### G9 quality-bench

- Preconditions: G0 pinned the fixture, command, environment and previous mi-lsp baseline; Graphify commit is exactly `9bf14a4931658152969586ace39eb965c010f0d1`; implementation base is exactly `a251ab1f8db4e96f029926fbef275b078a20a111`; G1-G8 joins are PASS; exact benchmark harness paths are resolved.
- Allowed paths: only the pinned benchmark harness, fixture, oracle and evidence paths declared by G0; no source mutation during measurement.
- Steps: execute each relevant benchmark 30 times per variant for current mi-lsp, Graphify and previous mi-lsp baseline, using identical cold/warm and incremental protocols; calculate token count, warm p95, peak RSS, incrementality, correctness, precision, recall and determinism; record raw samples, fixture digest, environment, command, commits and unavailable metrics; run security/no-MCP scans.
- Tests: `gofmt -l cmd internal`; `go vet ./...`; `go test ./...`; targeted graph tests; fixed-fixture token counts; 30-repetition cold/warm p95; peak RSS for index/query/MILX; incremental fixture; correctness/precision/recall/determinism; cross-RID; crash recovery, migration rollback and security/no-MCP checks.
- Acceptance: both comparisons are present, all relevant metrics have raw evidence, determinism and correctness or precision/recall oracles pass, and no unavailable measurement is represented as PASS. Existing hot paths must remain within baseline × 1.10 with the +25 ms p95 allowance; any unmeasured or failed gate is BLOCKED.
- Rollback: revert the offending packet or benchmark harness change; preserve raw evidence and rerun only after the fixture/baseline issue is resolved.
- Next action: join G9 and create W6 closure packet. Stop on missing baseline, fewer than 30 repetitions, invented result or threshold failure.

### W6 — closure and local release disposition

#### G10 closure-rollout

- Preconditions: G9 PASS; all packet joins are PASS; active audit session is complete enough for closure; no external publication is requested or authorized.
- Allowed paths: active `.docs/auditoria/<session>/` closure/evidence files and approved packet paths only. Never modify governance projection, secrets or `.mi-lsp/**`.
- Steps: run traceability, cross-reference and security audits; validate wiki authority/evidence/omission links; run `ae-close`; produce closure packet, sanitized verdict, release-distribution provenance, install paths, worker status, rollback note and explicit no-push/no-deploy/no-publish disposition; verify atomic commit boundaries, worktree cleanup and worker joins.
- Tests: final full command set; governance validation; graph ID scan; cross-ref scan; traceability audit; `git diff --check`; forbidden-path check; evidence manifest/audit hygiene validation; release-distribution review and `ae-close` verdict.
- Acceptance: all gates are ordered and reproducible; `ae-close` is `APPROVED`; audits/cross-ref/traceability are PASS; evidence is sanitized and omission-aware; release readiness is documented locally without publishing; no push, deploy or publish occurred.
- Rollback: do not integrate on FAIL/BLOCKED; preserve evidence, revert the bad atomic packet, return to the failed gate and route to owner triage. Do not admin-merge or bypass failing checks.
- Next action: only after closure approval may the integration owner perform the separately governed local integration decision; this plan itself stops and authorizes no push/deploy/publish.

## Fixed verification command set

`mi-lsp nav governance --workspace C:\repos\mios\mi-lsp-w1-sdd --format toon`

`go test ./...`

`go vet ./...`

`gofmt -l cmd internal`

`rg -n "FL-GPH-(01|02|03)|GraphGeneration|NodeKey|MILX-v1|SQLite|adjacency|NetworkX|cross-RID|precision|recall|determinism|incremental|peak RSS|p95|30 repetitions|Graphify|a251ab1f8db4e96f029926fbef275b078a20a111|9bf14a4931658152969586ace39eb965c010f0d1|no-MCP|crash recovery|dual-read|dual-write|rollback|ae-close|no push|no deploy|no publish" .docs/wiki .docs/raw/plans`

`rg -n --glob '!2026-07-18-milsp-graph-native-roadmap.md' "RF-GPH|TP-GPH|TECH-GPH|DB-GPH|CT-GPH" .docs/wiki .docs/raw/plans`

`git diff --check`

The graph ID scan must show no invented graph RF/TP/TECH/DB/CT IDs. G10 repeats the complete command set plus all targeted graph tests and the pinned benchmark suite; prose, screenshots or API-only checks cannot produce PASS.

## Release, closure and disposition rules

Release distribution remains local and gated: no binary, worker, install or publication artifact may be released until G9 PASS and G10 `ae-close` approval, and this plan still authorizes no push, deploy or publish. Record provenance, exact base/comparator commits, install paths, worker status, rollback note, audit manifest and any explicit release waiver under the active audit session. Query never mutates the index. A failed or interrupted packet is `partial`: it is not active, not counted as complete, not merged and not used as benchmark truth; its staged files are deleted or reverted while sanitized evidence and the reason remain. Full completion requires all waves and gates in order, final full tests, governance/source validation, security/no-MCP checks, traceability/cross-reference checks, release-distribution evidence, `ae-close` approval and a clean path-bounded diff.
