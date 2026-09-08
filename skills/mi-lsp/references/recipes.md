# Recipes

Use this reference when the task is goal-shaped instead of command-shaped.

## Bounded intent-first recipes

These recipes compose existing `mi-lsp` commands; they do not add another
router or recipe abstraction. Keep the returned envelope, warnings, omissions,
and canonical lane visible. Use the smallest bounded expansion that answers the
intent, and treat the canonical wiki as the final authority.

### R2 — Evidence reentry

Use this before loading a large prompt, transcript, log, or screenshot. Start
with the metadata-only inventory and follow its recommended canonical/evidence
path:

```powershell
mi-lsp nav evidence inventory <query>
```

The envelope is `backend: evidence.inventory` with one
`evidenceInventoryResult` containing `query`, `mode`, `recommended_read_path`,
`context_loading_profile`, `evidence_loading_profile`, `canonical`, optional
`lookup_status`, `evidence_roots`, and `next_queries`. Each root may expose its
artifact type, verdict, summary, `files`/`bytes`/`estimated_raw_tokens` stats,
and authority. Raw prompt, transcript, log, screenshot, secret, and PHI
content is omitted.

Keep `truncated=true` visible and use `continuation.next` to continue the same
inventory operation in full mode; do not silently broaden the query. This is a
direct/no-daemon read. If a root or lookup is unavailable, preserve its
`lookup_status`, omission, warning, and next query rather than treating
absence as proof. Inventory metadata recommends a read; it does not replace
the canonical artifact or authorize a write.

### R4 — Governance validation chain

Run the governance check before the two named wiki validators, in this order:

```powershell
mi-lsp nav governance
mi-lsp nav wiki validate-harness
mi-lsp nav wiki validate-source
```

The governance result is `GovernanceStatus`: preserve `blocked`, `sync`,
`index_sync`, `issues`, `warnings`, `allowed_actions`, `next_steps`, and any
returned `index_sync_details`. The harness result is
`HarnessValidationResult`: `harness_protocol`, `harness_readiness`,
`harness_verdict`, `harness_blockers`, `harness_warnings`,
`harness_contracts_reviewed`, `harness_links_reviewed`,
`harness_evidence_required`, `harness_evidence_found`,
`harness_docs_missing_contract`, and `harness_docs_unknown_audience`. The
source result is `WikiSourceValidationResult`: `wiki_source_protocol`,
`index_freshness`, `governance_sync`, `wiki_source_readiness`,
`wiki_source_verdict`, `wiki_source_blockers`, `wiki_source_warnings`,
`wiki_source_artifacts_reviewed`, `wiki_source_blocks_reviewed`,
`wiki_source_records_reviewed`, `wiki_source_tables_reviewed`,
`navigation_readiness`, `navigation_blockers`, and `documents`. There is no
`in_sync` boolean contract.

Proceed only when `blocked=false`, the reported sync/read-model state is
acceptable, and both validator verdicts are not `BLOCKED` with no unowned
blocker. Otherwise stop the dependent read, retain the diagnostics, and route
to the returned repair/next-step guidance. Keep direct execution, warnings,
and truncation visible; this chain validates the canonical route and source
contracts but does not turn a validator result into permission to mutate
protected files.

### R5 — Change-impact pack

Use this for a bounded change-context packet before expanding into affected
surfaces, tests, and wiki reads:

```powershell
mi-lsp nav change-pack [ref] [--path <path>] [--limit N]
```

The result is a `ChangePackPacket` with `ref`, `changed_files`,
`changed_paths`, `changed_symbols`, `affected`, `read_first`, `hub_risk`,
`wiki_must_read`, `suggested_tests`, `batch_next`, `generation_id`, `backend`,
optional `determinism_digest`, `impact_files`, `impact_symbols`,
`classification`, `classifications`, and `next_queries`. `hub_risk` is an
optional advisory object; do not invent a flat `hub_risk_score`.

Keep changed-path, affected, test, and wiki evidence bounded by `--limit`.
When a diff and generation are available, preserve their digest and coverage;
when either is missing, keep the warning or omission explicit instead of
claiming determinism or complete impact. Follow `batch_next`/`next_queries`
only as bounded continuations. The packet is impact guidance, not a canonical
authority and not a write plan.

### R1 — Skill-plan routing

Use this when a request needs more than one skill and role-aware selection is
required:

```powershell
mi-lsp skills plan --role <parent|leaf> --task <text> [--token-budget N] [--max-skills N]
```

The envelope is `backend: skills` with one `items[0]` matching
`mi-lsp-skill-plan/v1`: `schema`, `role`, `task`,
`budget{max_skills,token_budget}`, `always`, `routers`, `selected`,
`bundles_optional`, `deny_families`, `warnings`, and `why_not_cheaper`.
There is no `estimated_tokens` field. Selected IDs must resolve in the
catalog; parent routers belong only on parent plans, and leaf plans exclude
parent routers. Bounds and forced-intent exceptions follow `BuildPlan`.

Keep catalog/build warnings and any bounded error envelope visible. Do not
silently replace the planner with a daemon, external subagent, or invented
schema, and do not broaden the selected set when a catalog item is omitted.
The plan constrains routing cost; it does not supersede canonical wiki,
governance, or task-specific authority.

### R3 — Knowledge map then conditional recall

Orient a knowledge wiki with its bounded hub map first, then use semantic recall
only when its optional embedding dependency is available:

```powershell
mi-lsp nav wiki map --workspace <alias>
mi-lsp nav recall <query> --workspace <alias> --intent <formula|evidence|route|explore|learning> [--map]
```

The first envelope is `backend: wiki.map` with deterministically ordered hubs.
The map may return empty items or hubs; keep any surfaced warnings or hint
visible.
The second is `backend: recall` for semantic results or `backend: recall+lexical`
when an embedding request fails. Each `RecallResult` carries
`query`, `intent`, `archivo` (the result path), `heading`, `score`, `snippet`,
`start_line`, `end_line`, and `why`.

Keep the map-first step bounded and run recall conditionally. When embeddings
are not configured, preserve the direct `recall` response with empty items and
its hint to use `nav wiki search`; when an endpoint fails after configuration,
preserve the `recall+lexical` backend, warning/hint, and
`why: lexical_fallback`. Recall is direct and has no governance or daemon
requirement. It is candidate discovery only: verify any candidate against the
canonical wiki before relying on it.

### R6 — Authorized isolated graph QA

Use this recipe only when the operator explicitly authorizes graph QA in an
isolated sandbox. It is not a universal preflight for simple queries, and its
setup belongs at the start of authorized verification rather than hidden in
BUILD/C2. Missing optional plans, progress files, or artifacts are not blockers.
Leave closure ownership to the existing ae-work/QA owners; this recipe adds
only mi-lsp-specific QA guidance.

1. **Shared identity and backend-specific entrypoint.** For every authorized
   run, record the exact sandbox `cwd`, applicable documentary root, source
   identity, effective config, backend, and current CLI help. Use the source
   identity appropriate to the selected backend and scope; do not infer one
   backend's requirements from another. Only when the scope invokes Go code
   observation or indexing, additionally record the explicit repository
   identity and the runtime-selected, supported `go.mod` entrypoint. A present
   `go.mod` alone is insufficient: confirm that the entrypoint is supported by
   the CLI/catalog/config and observed runtime response. Use only flags shown by
   current help. The supported mode enum includes `full`, `docs`, and `catalog`;
   for example, use `index start --mode full --wait` only when current help
   confirms that syntax. A supported `full` run without `--clean` may dispatch
   incremental/no-change work internally, but `--mode incremental` is not a
   valid invented flag. A wiki/documentary-only scope requires only its
   documentary root/profile and source identity; it does not require a Go
   toolchain, Go entrypoint, Go caches, or a Git repository merely because
   compiler QA does. Other backends follow their own actual requirements; do
   not impose Go requirements on them.

2. **Offline-safe isolation when needed.** Only when Go code observation or
   indexing is in scope, resolve absolute host paths for populated `GOMODCACHE`,
   `GOCACHE`, and `GOPATH` before replacing `HOME`, using explicit variable
   names, and point the isolated process at those caches. Never dump the full
   environment or hardcode a host-specific path. Apply storage-capacity checks,
   native OS paths in fixtures, and secret redaction whenever the actual
   authorized work needs them; these are not gates for a simple query. Before
   an expensive check, capacity-check `TMPDIR`/`GOTMPDIR` on suitable disk.
   Do not purge caches or install dependencies automatically. Classify missing
   cache/env prerequisites separately from a product failure. Cross-compiling
   is not native execution.

3. **Evidence and readback.** Require a positive existing relationship only
   when the acceptance contract explicitly promises a positive flow. For a
   negative-only security oracle, assert refusal and no mutation instead; do
   not require a positive relationship. Require graph/catalog binding
   coherence only when the selected backend and scope actually expose or use a
   catalog; wiki/documentary-only work does not require a code-catalog binding.
   `ok=true`, an empty selector, or counts alone is not acceptance. When the
   acceptance contract requires before/after counts and stable digests, compare
   them; otherwise do not imply that those comparisons were required. Preserve
   backend, reason, and omissions; classify historical or external unresolved
   debt against the active scope instead of making it a universal blocker or
   universal pass. For daemon status, forward only documented scalar fields
   (for example status, pid, root, backend, entrypoint, and generation IDs).
   Never forward nested state objects, `admin_token`, raw tokens, or full
   environment data. A canonical shell command invoking `mi-lsp` is valid tool
   use.

4. **Recovery discipline.** Reuse checks that already passed and recover only
   the invalidated oracle. Do not loop the full suite, purge state, or retry
   unchanged commands. Preserve every labeled omission or cache/environment
   limitation in the report.

## Canonical wiki / traceability discovery

```powershell
mi-lsp nav route "how does login work?" --workspace <alias> --format compact
mi-lsp nav wiki search "RF-AUTH login" --workspace <alias> --layer RF,TP,CT --format compact
mi-lsp nav wiki pack "how does login work?" --workspace <alias> --format compact
mi-lsp nav wiki trace RF-AUTH-001 --workspace <alias> --format compact
```

Use this when the task is about canonical docs, requirements, tests, contracts, or traceability.
If AXI preview is trimmed, rerun the same wiki command with `--full` before broadening the search.
If a later `nav search` returns prompts, audits, `.docs/raw`, or other support artifacts, treat that as non-canonical evidence and keep the wiki lane as the source of truth.

## Intent-first graph and change recipes

Start with the goal in `nav intent`; it is mandatory for every supported goal-shaped request. Routing is automatic and has no routing opt-out. `explain-change` is an intent/operation of `nav intent`, not a required literal alias in the request. The planner returns a bounded preview with the information currently available, graph/wiki/evidence, explicit candidates, fallbacks, and omissions. Preserve the seven sections `change`, `affected`, `callers`, `callees`, `tests`, `contracts`, and `wiki`; use the exact `expansions[].command` as the second query and read its reason. Follow the emitted command rather than translating it by hand.

```powershell
# Explain a working-tree or explicitly supplied change.
mi-lsp nav intent "explain the change and its impact" --workspace <alias> --format toon
mi-lsp nav explain-change --path internal/service/intent.go --workspace <alias> --format toon

# Impact, callers, and callees.
mi-lsp nav intent "what is affected by this change?" --workspace <alias> --format toon
mi-lsp nav affected internal/service/intent.go --include-tests --include-docs --workspace <alias> --format toon
mi-lsp nav callers MySymbol --workspace <alias> --format toon
mi-lsp nav callees MySymbol --workspace <alias> --format toon

# A bounded path, exact edge explanation, and neighborhood.
mi-lsp nav path FromSymbol ToSymbol --workspace <alias> --format toon
mi-lsp nav explain <edge-cross-rid> --workspace <alias> --format toon
mi-lsp nav neighbors MySymbol --workspace <alias> --format toon
```

Natural-language “neighborhood” routes to the exposed `nav neighbors` command. Use `nav related MySymbol --depth definition,callers,implementors,tests` only when that specific four-lane summary is wanted. For every preview, preserve available evidence and read `expansions[].reason`; use the exact `expansions[].command` for the next bounded read. A timeout, silence, `DONE`, or `PASS` without fresh evidence is incomplete evidence, not PASS and not a silent fallback.

Only these external fallback reasons are allowed: `unsupported_operation`, `unavailable_binary`, `invalid_workspace`, and `explicit_incomplete`. If none is visible, stay with `mi-lsp` and report the limitation. Use 180/300-second soft/hard watchdogs, at most two same-context recoveries with a smaller packet and no unchanged retry, at most six practical lanes with exclusive `allowed_paths`, fail-closed joins, fresh verification, and redacted evidence without prompts, transcripts, secrets, PII, PHI, argv, or raw patterns. Do not invent model/provider metadata.

## Preparation and guarded editing

```powershell
mi-lsp nav prepare "review the routing change" --affected internal/service/intent.go --workspace <alias> --format toon
mi-lsp nav edit-plan --packet .mi-lsp/plan.json --workspace <alias> --format toon
```

`nav prepare` is read-only semantic preparation. `nav edit-plan` is a separate deterministic patch packet surface: dry-run by default; writing requires `--apply --experimental-apply`, a clean Git workspace, safe paths, and matching hashes. Neither command is a generic fallback for the other.

## Wiki pack, trace, and validator scopes

Use the actual wiki subcommands and scopes exposed by the runtime:

```powershell
mi-lsp nav wiki pack "understand the routing contract" --workspace <alias> --format toon
mi-lsp nav wiki pack "indexing docs" --workspace <alias> --rf RF-IDX-001 --format toon
mi-lsp nav wiki trace RF-QRY-003 --workspace <alias> --format toon
mi-lsp nav wiki trace --all --summary --workspace <alias> --format toon
mi-lsp nav wiki validate-harness --workspace <alias> --ids CT-NAV-INTENT --format toon
mi-lsp nav wiki validate-harness --workspace <alias> --paths .docs/wiki/09_contratos/CT-NAV-INTENT.md --format toon
mi-lsp nav wiki validate-source --workspace <alias> --ids CT-NAV-INTENT --format toon
mi-lsp nav wiki validate-source --workspace <alias> --paths .docs/wiki/09_contratos/CT-NAV-INTENT.md --format toon
```

`nav wiki search` accepts `--layer RS,RF,FL,TP,CT,TECH,DB`. `validate-harness` validates SDD-HARNESS-v1 contracts and `validate-source` validates SDD-WIKI-SOURCE-v1 source blocks; both accept comma-separated `--ids` or `--paths`. Use the two named wiki validator subcommands; do not infer a combined validator surface.

## Service audit

```powershell
mi-lsp nav service <service-path> --workspace <alias> --format compact
mi-lsp nav context <file>:<line> --workspace <alias> --format compact
mi-lsp nav search "IConsumer<|PublishAsync<" --workspace <alias> --format compact
mi-lsp nav overview <service-path> --workspace <alias> --format compact
```

Use this before claiming a service is incomplete.
For Go packages, `nav service` reports `profile=go-package` and uses Go-aware evidence (`net/http`, router-style calls, Cobra, worker signals) instead of .NET-only endpoint/event patterns.

## Completeness check for `.NET` minimal APIs

```powershell
mi-lsp nav service src/backend/<service> --workspace <alias> --format compact
mi-lsp nav context src/backend/<service>/Program.cs:<line> --workspace <alias> --format compact
mi-lsp nav search "Map(Get|Post|Put|Delete|Patch)" --workspace <alias> --format compact
```

Do not infer "not implemented" only because a guessed command or handler class is absent.

## Workspace orientation

```powershell
mi-lsp nav governance --workspace <alias> --format compact
mi-lsp nav ask "how is this workspace organized?" --workspace <alias>
mi-lsp nav workspace-map --workspace <alias> --axi --format compact
mi-lsp nav related <important-symbol> --workspace <alias> --format compact
```

If governance is blocked, stop normal exploration and repair the governance document/projection first.

## PR review / impact analysis

```powershell
mi-lsp nav diff-context HEAD~1 --workspace <alias> --format compact
mi-lsp nav diff-context main --include-content --workspace <alias> --format compact
```

## Batch exploration

```powershell
mi-lsp nav search "PublishAsync" --include-content --workspace <alias> --format compact
mi-lsp nav multi-read src/Service.cs:1-100 src/Controller.cs:50-150 src/Model.cs:1-80 --workspace <alias> --format compact
```

If that still implies too many separate calls, switch to `nav batch`.

## Portable preparation recipe

```powershell
mi-lsp prepare create --workspace <alias> --output <validated-evidence-root>
mi-lsp prepare verify --workspace <alias> --input <packet>
mi-lsp prepare refresh --workspace <alias> --input <packet> --output <validated-evidence-root>
```

Keep seed receipts and catalogs isolated with explicit roots. Treat typed drift as repair guidance only: preparation evidence never authorizes writes, protected mutations, or promotions.
