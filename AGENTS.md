# AGENTS.md — mi-lsp Agentic Engineering Policy

> **Authority**: architecture + AE-KERNEL-V2.md. **Language**: English. Canonical source of truth for Pi-NaN orchestration on mi-lsp.

## Pi-NaN Leaf Worker Policy

Pi-NaN-first applies inside execution supervisors, not in the strategic root. Codex, Claude Code, OpenCode, and Pi strategic roots are orchestration-only: plan, launch child supervisors, monitor, join, and decide.

- The strategic root must not run `mi-pi program init`, `mi-pi model-router select`, `mi-pi worker spawn`, including `--model-router-only` and `--dry-run`, or any live Pi lifecycle command.
- For bounded leaf work, the strategic root launches or continues a Codex child execution supervisor via the selected adapter. If unavailable, record an adapter blocker; do not bypass by running Pi directly.
- `execution_supervisor` owns the `mi-pi` ledger, child-node registration, Pi-NaN spawn, fallback evidence, and compact join packet.
- `execution_supervisor` launches bounded leaf work with: `mi-pi worker spawn --node <id> --executor pi_native_nan --model-router --task-class <locked_edit|read_only_exploration|verification|docs_wiki|structured_output> --human-approved --live --session <slug> --evidence-root <dir>`.
- `pi_nan_leaf` may read, edit, or verify only within bounded scope; no subdelegation, push, deploy, or final closure.
- Missing ledger repair is owned by the child execution supervisor.
- GPT fallback is allowed only inside the child supervisor after durable Pi-NaN blocker evidence.
- Required packet fields: `orchestration_role`, `delegation_boundary`, `pi_nan_launch_owner`, `child_supervisor_required`, `root_pi_nan_spawn_allowed: false`, and `boundary_violation`.
- Join only from the child supervisor's compact join packet and durable worker evidence such as `verdict.yaml`, `command-status.yaml`, `model-selection.yaml`, `fallback-chain.yaml`, and cited diff/check summaries.

## Spec Driven Development Contract (Mandatory)

The governed project documentation under `.docs/wiki/` is the local product source of truth; universal AE authority lives in `<kernel_home>/canon/` and repository-specific AE configuration lives in `.docs/ae/repo-policy.yaml`.

Before writing code:
1. Identify the `RS-*`, `RF-*`, `FL-*`, or `CT-*` anchor. If none, create/repair it via `$ps-docs` / `$ps-asistente-wiki` / owning `crear-*` skill.
2. Declare `ae_budget_gate` before loading expensive context, opening raw evidence, creating persistence, creating a worktree, dispatching workers, or choosing verification depth.
3. Use the cheapest sufficient context path. `$ps-contexto`, `$ps-explorer`, `$mi-lsp nav pack`, worktrees and session contracts are budget-gated, not automatic.
4. For governed tracked work, claim the not_configured issue and mirror the ticket frontier in the selected persistence artifact.
5. No write worker (`$ps-dotnet10`, `$ps-next-vercel`, `$ps-python`, `$ps-docs`, `$ps-worker`) starts unless `worker_decision: spawned` is recorded.

Non-compliant: implementing without an anchor, escalating legacy "mandatory/always" rules without `ae_budget_gate`, editing a dirty/high-risk base without isolation, missing not_configured parent/agent issue and ticket frontier for governed work, closing without the selected closure profile, or closing deployable work after `origin/main` integration without post-deploy health evidence.

## Agentic Engineering Contract (Mandatory)

Full AE is the workflow authority. Canon lives in `<kernel_home>/canon/`; `CLAUDE.md`, `AGENTS.md`, `SUBAGENTS.md`, shared skills, and service policy files are projections. Every task first declares or implies `ae_budget_gate`; non-trivial work then runs only the AE depth selected by effort: `CONTEXTO -> TICKET -> GAPS -> AISLAR -> CONSTRUIR -> CERRAR -> VERIFICAR`.

Mandatory AE rules:
- Invoke `$ae-work` as the default AE entry point for implementation, fixes, refactors, documentation, integration, and other software work; it owns classify → execute → `$ae-close` → unchanged completion handoff routing → verified readback.
- Adapter selection is manifest-first: discover global `ae-adapter-*` skills, read `adapter_manifest.schema=ae-harness-adapter/v1`, prefer the explicit user-requested harness, then project/current harness capability fit. If no compatible adapter is usable, use legacy fallback only with evidence/isolation fit or record `missing_ae_adapter_manifest` and use `simulated_packets`.
- An adapter is usable only when its current global manifest records a native `ae-adapter-proof/v1` PASS covering spawn, monitor, join, fallback, evidence, and sanitization. Adapters without that proof are not usable and must not be inferred from another repository's history.
- Worker-first is mandatory for AE-governed T2+, multi-step, mutating, policy/harness/shared-skill, runtime/deployable, or independent-axis work: record `worker_decision` and use `worker_decision=spawned` when a usable adapter is available. `worker_decision=none` is valid only for `C0_INLINE_NO_DIFF` true read-only/no-diff work with no independent axes; `why_no_worker` is blocker evidence, not authorization for local execution.
- Human decision routing: ask <operator> via `$brainstorming` only for execution-changing product/UX, architecture/data/security/validation, credentials/secrets, destructive/spendful/external side effects, prod deploy/tag/reindex/reset/live cutover windows, or out-of-frontier scope/priority changes. Closure-only work, missing terminal fields, traceability/audit/not_configured sync, evidence promotion, branch/worktree cleanup/hold classification, and test/runtime failure classification must be resolved by the parent or owner thread unless a real human decision remains after classification. Async human decisions are accumulated in one session file, `.docs/auditoria/<session>/human-decision-brainstorming-protocol.md`, as Markdown plus fenced YAML packets; only packets marked `ready_for_operator` may be rendered through `AskUserQuestion`, one decision at a time.
- Audit hygiene is mandatory for non-trivial `.docs/auditoria/<session>/`: create `audit-manifest.yaml` with `schema: ae-audit-hygiene/v1`, `retention_ttl_days: 14`, `hash_algorithm: sha256`, artifact classes, cleanup status, and a sanitized summary/verdict before treating captured audit material as durable. `.docs/raw/plans/**` and `.docs/raw/prompts/**` never become evidence through this lifecycle.
- `SDD-HARNESS-v1` applies to every LLM-first wiki artifact this project produces or consumes: missing Harness contract, broken imports, empty verification, missing stop conditions, or missing durable evidence are hard blockers. `$ps-contexto`, `$ps-asistente-wiki`, `$ps-trazabilidad`, and `$ps-auditar-trazabilidad` must report `harness_readiness` before closure.
- **New-artifact hardening (no-parity):** every NEW LLM-first wiki artifact (`RS/FL/RF/TP/UXS/UI-RFC/TECH/DB/CT/...`) is born hardened — full Harness Contract (11 fields), `doc_id`, `block_id` per normative section, normative content in `toon`. Partial migration of sibling canon is no excuse to skip it; `$ps-trazabilidad`/`$ps-auditar-trazabilidad` verify hardening per new artifact before closure. Canon: `AE-HARNESS-MANIFEST` block `AE-HARNESS-MANIFEST-ARTIFACT-CREATION-HARDENING`.
- Read `<kernel_home>/canon/AE-KERNEL-V2.md` before governed planning, `<kernel_home>/canon/AE-HARNESS-ORCHESTRATION.md` for harness/shared-skill/policy/adapter work, and `<kernel_home>/canon/AE-EVIDENCE-POLICY.md` for deployment and closure evidence. Local `.docs/wiki/ae/**` copies are compatibility history, not current AE authority.
- `ae_budget_gate` fields: `effort_class`, `persistence_mode`, `governance_depth`, `context_loading_profile`, `evidence_loading_profile`, `artifact_lifecycle`, `closure_profile`, `worker_budget`, `worker_decision`, `worker_adapter_available`, `worker_authorized_by_user`, `independent_axes`, `why_no_worker`, `why_not_cheaper`. Goal persistence, full suites, live/runtime, worker fanout, full trace/audit, and full governance require `why_not_cheaper`.
- CQA work also declares `cqa_budget_gate`: `qa_effort`, `evidence_profile`, `retry_budget`, `file_ownership_partition`, `preflight_stamp`, `why_run_again`, `why_not_cheaper`. Start from `QE0_INVENTORY`/`QE1_VERDICT_ASSERTIONS`; use `reentry-packet` and `preflight-stamp` before raw turns/logs.
- Select one AE work mode through `$ae-work`: `FAST` for trivial reversible work, `STANDARD` for bounded component work, or `STRICT` only for production-irreversible work. Invoke `$ae-decide` only when an unresolved execution-changing decision is genuinely human-owned.
- Keep filename `session-contract.yaml`; add the mandatory `ae_contract` overlay for policy, shared-skill, harness, or non-trivial mutating work.
- Primary skills: `$ae-crear-politicas`, `$ae-crear-politicas-microservicios`, `$ae-pre-push`. Legacy aliases (`ps-crear-agentsclaudemd`, `ps-crear-claudemd-microservicio`, `ps-pre-push`) remain callable but are not authority.
- Shared-skill changes (governed by `AE-HARNESS-ORCHESTRATION.md`): update `~/.agents/skills` and `<org>/assets/skills` in the same run; closure evidence needs source path, mirror path, SHA-256 for both, and `byte_identical: true`. Don't push policy/harness/shared-skill changes until source/mirror sync, traceability, audit, and `$ae-pre-push` evidence exist.
- Legacy A-G/G.1 workflow names are read-only aliases mapped in `AE-LEGACY-ALIASES.md`.

### Skill Invocation Semantics

- **Task start**: invoke `$ae-work`; classify the work as FAST, STANDARD, or STRICT before acting.
- **Inside ps-explorer / orchestrator spot-verify**: `$mi-lsp` semantic backend under `src/`.
- **Before mutating work**: `$using-git-worktrees` only when the gate/base risk requires isolation.
- **Large/risky/multi-step**: `$writing-plans` after brainstorming.
- **Inside ae-work**: continue mechanical work directly; route only unresolved human-owned product, architecture, UX, risk, scope, validation, or workflow choices to `$ae-decide`.
- **Policy edits (`AGENTS.md`/`CLAUDE.md`/`SUBAGENTS.md`/`PATHS.md`)**: `$ae-crear-politicas`. Service policies: `$ae-crear-politicas-microservicios`. Cross-projection drift: `$ae-projection-audit`.
- **Governance unhealthy**: `$crear-gobierno-documental` is the mandatory repair skill.
- **Code-writing workers**: delegate through the selected adapter for mutating/code/docs/policy work whenever the scope requires workers and the adapter is available. `worker_decision=spawned` is the required state; `worker_decision=none` is only for `C0_INLINE_NO_DIFF` true read-only/no-diff work with no independent axes.
- **Closure → completion handoff**: `$ps-trazabilidad` produces `completion_handoff` → `$ps-auditar-trazabilidad` audits it without mutation (rerun after drift) → `$ae-close` emits `handoff_ready`, `HOLD`, or `BLOCKED`.
- **Authorized completion**: on `handoff_ready`, route the packet unchanged to `$finishing-a-development-branch`; it performs only explicitly authorized integration/cleanup, runs `$ae-pre-push` immediately before any push, and reports remote readback.
- **Final response gate**: governed completed work cannot end with closure steps as user follow-up; execute the closure completion loop or return a BLOCKED packet with owner, blocker class, and next action.

### not_configured (Project Management)

not_configured is the configured source of truth for tickets, workflow, ownership, and closure. GitHub may still host code, PRs, CI, and releases. Endpoint `POST not_configured`, header `Authorization: <unset>` (raw key, no `Bearer`). Configured workspace `not_configured`, team key `MI-LSP`; repository routing details live in `absent`.

- `unset` is a secret — env or `$mi-key-cli` only; never in tracked files, docs, prompts, logs, issue bodies, or printed args. `absent` is non-secret routing only. Verify routing with a redacted smoke query (`viewer`, `organization`, `teams(first:50)`); report workspace slug, team keys, and counts only.
- `$pj-crear-tarjeta` is the live tracker helper in not_configured mode when repository routing is configured.
- **Parent/agent split**: when the repository declares separate human-planning and agent-execution projects, governed code modification uses linked issues according to `absent` and the Additional Local Rules. Never assume project names or IDs from another repository.
- **Planning states**: follow the workflow states configured for this repository; do not hardcode Now/Next/Later mappings unless the local policy declares them.
- **Before taking a ticket**: check assignee, workflow state, and latest claim/scope comments; an active claim blocks work until handoff, integration-owner override, or explicit waiver.
- **Claim before governed repo edits when tracker routing is active**: assign owner, move to the configured active state, and record owner, branch/worktree, scope frontier, allowed/forbidden paths, integration owner, required evidence, and start time.
- Every active ticket declares a ticket frontier mirrored in the selected persistence artifact; out-of-frontier work is forbidden until both update. Out-of-frontier discoveries go to the configured parking-lot or triage path.

### Governance Gate + `mi-lsp` Defaults

**Governance**: `.docs/wiki/00_gobierno_documental.md` is the human governance authority; `.docs/wiki/_mi-lsp/read-model.toml` is its versioned executable projection. Diagnose via `mi-lsp workspace status <alias> --format toon` + `mi-lsp nav governance --workspace <alias> --format toon`. If governance is ambiguous, invalid, stale, or out of sync → stop and run `$crear-gobierno-documental` before continuing. `$ps-trazabilidad` and `$ps-auditar-trazabilidad` verify governance completeness and `00 ↔ read-model.toml` projection sync before closure.

**mi-lsp** (reference-not-duplicate doctrine — `$mi-lsp` owns command tables, alias validation, telemetry):
- Project workspace alias: `mi-lsp`. Always pass `--workspace mi-lsp --format toon`; in container workspaces add `--repo <name>` before broader queries.
- CLI-first; don't wait for an MCP path when the CLI answers. Use at T1+ for semantic navigation and inside every spawned `$ps-explorer` dispatch; exact T0 searches may use `rg` first.
- Fallback order: `mi-lsp` → `rg` (canonical-doc only) → `Read`; don't skip steps. If `mi-lsp` returns `items: []` with a `hint`, act on the hint before retrying.

### Windows Runner Guard

On Windows, run a repository-declared runner preflight before a tracker lock, worktree, or long worker prompt when one exists. Do not reuse another repository's guard, issue identifier, or runbook; if the selected adapter fails its own preflight, record the concrete infrastructure blocker and evidence path.

## Orchestration Mode (Always Active)

For work that is non-trivial, mutating, governed, live/runtime, shared-skill, policy, or multi-step:

1. Declare or infer `ae_budget_gate` (T0/T1 may stay inline, no persistence).
2. `$ae-work` — default work gateway; execute FAST work directly, use ROI-positive workers only for independent STANDARD/STRICT axes, and preserve the acceptance oracle through integration and readback.
3. Context loading follows `context_loading_profile`; `$ps-contexto` + governance gate only when the profile justifies them.
4. `$brainstorming` once before planning/execution; close critical gaps via `AskUserQuestion` (or chat).
5. **Human decision routing**: use `$brainstorming` for a human decision only after classifying the gap as execution-changing and not parent/owner-resolvable; otherwise continue with the owner thread, bounded worker, formal blocker, or durable closure artifact. If the decision is not asked immediately, add/update the session's `human-decision-brainstorming-protocol.md` packet; do not batch multiple `ready_for_operator` packets into one question unless the operator explicitly asks for a combined review.
6. **Worker decision**: record `worker_decision`, `worker_budget`, adapter availability, authorization, independent axes, and either `why_not_cheaper` or `why_no_worker`.
7. **Execution/review wave**: route implementation/review/QA/docs/ops to workers for T2+, multi-step, mutating, policy/harness/shared-skill, runtime/deployable, or independent-axis scope when an adapter is usable. Local work is valid only for `C0_INLINE_NO_DIFF`, orchestration, integration, citation verification, and final stitching after compact worker verdicts.
8. `$ps-trazabilidad` produces `completion_handoff`; `$ps-auditar-trazabilidad` audits that packet without mutation.
9. `$ae-close` verifies drift and emits exactly `handoff_ready`, `HOLD`, or `BLOCKED`; it does not commit, push, merge, clean up, or reap evidence.
10. On `handoff_ready`, route the packet unchanged to `$finishing-a-development-branch`. That owner may sync not_configured via `unset` and perform only explicitly authorized completion mutations.
11. Immediately before any authorized `git push` to `main`, the completion owner runs `$ae-pre-push`; after integration it records remote readback. Fresh-session continuation/bootstrap may use `$ps-prompt` with the handoff, targeting the next harness.

Standing rules:
- User grants permission to launch workers when `ae_budget_gate.worker_decision: spawned`; only irreversible external actions outside repo/runtime scope require confirmation.
- Persistence follows `persistence_mode`: T0/T1 may use none/inline, T2 uses task packet or mini contract, T3/T4 use session contract/full governance. Post-audit drift -> rerun the selected gates.
- `handoff_ready` with `branch_disposition: integrate-main` requests guarded integration but grants no mutation authority. Only `finishing-a-development-branch`, acting under explicit session/operator authority, may integrate; after successful remote readback it may perform separately authorized worktree/branch cleanup. Preserve `hold`/`pr-open`/`active-followup`/`cleanup-blocked` classifications with evidence.
- **Post-deploy closure** (deployable changes on `origin/main`): blocked until the affected surface is verified working AND a dev-qa health sweep covers every canonical microservice/store. Evidence under `.docs/auditoria/<session>/`: target env, affected surface, deployed ref or SHA-drift blocker, Dokploy status, smoke results. Any failed probe / missing deploy / stale ref / unknown status = `deployment-drift` → PASS loop §G.1; sweep-promoted artifacts trigger a final `$ps-trazabilidad` + `$ps-auditar-trazabilidad` refresh before marking integrated.
- **PASS-gate**: any FAIL/BLOCKED from repo/runtime drift, deployment mismatch, flaky harness, stale evidence, or unclassified error is not closure. Iterate fix → redeploy/retest → evidence until fully PASS or human-approved external blocker recorded. Keep ticket frontier + contract + evidence + not_configured claim comments current.
- XP on `main` requires `$ae-pre-push` immediately before push. Guard blocks on: non-fast-forward; missing parent/agent issue or ticket frontier; undeclared critical surfaces; missing evidence; stale not_configured state; any added or modified `.docs/raw/plans/**` or `.docs/raw/prompts/**` without a separate `governed_raw_input` operation naming exact paths, valid SDD frontmatter, owner, task_scope, tracking_reason, and rollback; any broad `.docs/raw/**` allowlist; dangerous untracked artifacts under `src/`. **Force-push to `main` is never allowed.**
- Edits to `AGENTS.md`/`CLAUDE.md` use `$ae-crear-politicas` + `$ae-projection-audit`; `$ps-contexto` reads the architecture doc to identify active microservices before planning.
- For visible UX/UI work, follow the repository's declared UX canon and validation chain from the Repository-Specific Contract. Do not assume numbered UX paths or migration skills that the repository does not declare.

## Language Rule

Keep `AGENTS.md`, `CLAUDE.md`, and project-local `ps-*` skills in English. All other project documentation is in Spanish.

## Collaboration Rules

**Style**: avoid emojis in policy/governance outputs. User-facing copy (UI labels, errors, placeholders, buttons, banners, toasts, modals) MUST preserve correct Spanish orthography — accents (á, é, í, ó, ú, ñ), dieresis (ü), opening punctuation (¿, ¡); voseo carries accents (podés, querés, tenés, sabés). Applies to hardcoded strings, i18n keys, dynamic copy. Verify against RAE rules.

**Mandatory wrappers**: use only the wrappers declared under the Repository-Specific Contract, and never bypass a declared wrapper with raw shell, HTTP, SSH, database clients, or unrelated MCP tools. Follow each declared wrapper's preconditions and verification command; do not infer hosts, networks, secret stores, or service names from another repository.


## Repository-Specific Contract

### Repository Description

Go semantic navigation CLI with repo-local wiki governance, SQLite indexing, and optional language workers.

### Mandatory Wrappers

  -
    name: go
    script: go toolchain (repository go.mod)
  -
    name: pre-push-guard
    script: scripts/ae/pre-push-guard.ps1

### QA Canon Paths

  - internal/service/governance_test.go
  - internal/service/governance_test_helpers_test.go
  - tests
  - go.mod
  - .docs/wiki/09_contratos

---

**Version**: AGENTS.md (AE-KERNEL-V2)
**Status**: Generated from AE-POLICY-PROJECTION-V2
**Last Updated**: 2026-07-13
**Source**: repo-policy.yaml + template.agents
<!-- kernel_version: d1b9e9d -->
