# AGENTS.md — mi-lsp Agentic Engineering Policy

> **Authority**: Architecture + AE-KERNEL-V2.md. **Language**: English. Canonical source of truth for worker-runtime orchestration on mi-lsp.

## worker-runtime Leaf Worker Policy

Worker-runtime-neutral applies inside execution supervisors, not in the strategic root. Codex, Claude Code, OpenCode, and Pi strategic roots are orchestration-only: plan, launch child supervisors, monitor, join, and decide.

- The strategic root must not run the worker runtime initialization command, the worker runtime model selection command, the worker runtime spawn command, including `--model-router-only` and `--dry-run`, or any live Pi lifecycle command.
- For bounded leaf work, the strategic root launches or continues a Codex child execution supervisor via the selected adapter. If unavailable, record an adapter blocker; do not bypass by running Pi directly.
- `execution_supervisor` owns the worker runtime ledger, child-node registration, worker-runtime spawn, fallback evidence, and compact join packet.
- `execution_supervisor` launches bounded leaf work with: `worker-runtime spawn --node <id> --executor native_worker_runtime --task-class <locked_edit|read_only_exploration|verification|docs_wiki|structured_output> --approved --live --session <slug> --evidence-root <dir>`.
- `worker_leaf` may read, edit, or verify only within bounded scope; no subdelegation, push, deploy, or final closure.
- Missing ledger repair is owned by the child execution supervisor.
- GPT fallback is allowed only inside the child supervisor after durable worker-runtime blocker evidence.
- Required packet fields: `orchestration_role`, `delegation_boundary`, `worker_launch_owner`, `child_supervisor_required`, `root_worker_spawn_allowed: false`, and `boundary_violation`.
- Join only from the child supervisor's compact join packet and durable worker evidence such as `verdict.yaml`, `command-status.yaml`, `model-selection.yaml`, `fallback-chain.yaml`, and cited diff/check summaries.

## Spec Driven Development Contract (Mandatory)

The wiki (the canonical documentation set) + governed `context/` annexes is the ONLY source of truth.

Before writing code:
1. Identify the `RS-*`, `RF-*`, `FL-*`, or `CT-*` anchor. If none, create/repair it via `$documentation-worker` / `$wiki-assistant` / owning `crear-*` skill.
2. Declare `ae_budget_gate` before loading expensive context, opening raw evidence, creating persistence, creating a worktree, dispatching workers, or choosing verification depth.
3. Use the cheapest sufficient context path. `$context-loader`, `$exploration-worker`, `$project-context-cli nav pack`, worktrees and session contracts are budget-gated, not automatic.
4. For governed tracked work, claim the external tracker issue when tracker.mode is external; otherwise mirror the local session-contract frontier in the selected persistence artifact.
5. No write worker (`$backend-worker`, `$frontend-worker`, `$python-worker`, `$documentation-worker`, `$general-worker`) starts unless `worker_decision: spawned` is recorded.

Non-compliant: implementing without an anchor, escalating legacy "mandatory/always" rules without `ae_budget_gate`, editing a dirty/high-risk base without isolation, missing parent/agent issue or ticket frontier when tracker.mode is external, closing without the selected closure profile, or closing deployable work after `origin/main` integration without post-deploy health evidence.

## Agentic Engineering Contract (Mandatory)

Full AE is the workflow authority. Canon lives in `<kernel_home>/canon/`; `CLAUDE.md`, `AGENTS.md`, `SUBAGENTS.md`, shared skills, and service policy files are projections. Every task first declares or implies `ae_budget_gate`; non-trivial work then runs only the AE depth selected by effort: `CONTEXTO -> TICKET -> GAPS -> AISLAR -> CONSTRUIR -> CERRAR -> VERIFICAR`.

### Tracker Applicability

- tracker.mode external: claim/ticket/parent-agent/workflow clauses apply.
- tracker.mode none: the local session-contract frontier owns the workflow.
- Traceability, audit, and pre-push remain mandatory in both modes.
- This override governs any generic tracker wording below.

Mandatory AE rules:
- Invoke `$ae-work` as the default AE entry point for non-trivial, mutating, policy/harness/shared-skill, live/runtime, or multi-step work; it owns classification through closure routing, adapter selection, persistence, evidence expectations, and completion handoff. Use `$ae-decide` only for genuinely human-owned execution-changing decisions.
- Adapter selection is manifest-first: discover global `ae-adapter-*` skills, read `adapter_manifest.schema=ae-harness-adapter/v1`, prefer the explicit user-requested harness, then project/current harness capability fit. If no compatible adapter is usable, use legacy fallback only with evidence/isolation fit or record `missing_ae_adapter_manifest` and use `simulated_packets`.
- Usable manifest-backed adapters are `ae-adapter-codex`, `ae-adapter-pi`, and `ae-adapter-claude-code` (native `ae-adapter-proof/v1` PASS, verified adapter evidence, recorded in its global manifest). `ae-adapter-hermes` remains a proof-gated partial seed: not usable until a native `ae-adapter-proof/v1` proves spawn, monitor, join, fallback, evidence, and sanitization.
- Worker-first is mandatory for AE-governed T2+, multi-step, mutating, policy/harness/shared-skill, runtime/deployable, or independent-axis work: record `worker_decision` and use `worker_decision=spawned` when a usable adapter is available. `worker_decision=none` is valid only for `C0_INLINE_NO_DIFF` true read-only/no-diff work with no independent axes; `why_no_worker` is blocker evidence, not authorization for local execution.
- Human decision routing: ask <operator> via `$brainstorming` only for execution-changing product/UX, architecture/data/security/validation, credentials/secrets, destructive/spendful/external side effects, prod deploy/tag/reindex/reset/live cutover windows, or out-of-frontier scope/priority changes. Closure-only work, missing terminal fields, traceability/audit/external tracker sync, evidence promotion, branch/worktree cleanup/hold classification, and test/runtime failure classification must be resolved by the parent or owner thread unless a real human decision remains after classification. Async human decisions are accumulated in one session file, `.docs/auditoria/<session>/human-decision-brainstorming-protocol.md`, as Markdown plus fenced YAML packets; only packets marked `ready_for_operator` may be rendered through `AskUserQuestion`, one decision at a time.
- Audit hygiene is mandatory for non-trivial `.docs/auditoria/<session>/`: create `audit-manifest.yaml` with `schema: ae-audit-hygiene/v1`, `retention_ttl_days: 14`, `hash_algorithm: sha256`, artifact classes, cleanup status, and a sanitized summary/verdict before treating raw evidence as durable.
- `SDD-HARNESS-v1` applies to every LLM-first wiki artifact this project produces or consumes: missing Harness contract, broken imports, empty verification, missing stop conditions, or missing durable evidence are hard blockers. `$context-loader`, `$wiki-assistant`, `$traceability-check`, and `$traceability-audit` must report `harness_readiness` before closure.
- **New-artifact hardening (no-parity):** every NEW LLM-first wiki artifact (`RS/FL/RF/TP/UXS/UI-RFC/TECH/DB/CT/...`) is born hardened — full Harness Contract (11 fields), `doc_id`, `block_id` per normative section, normative content in `toon`. Partial migration of sibling canon is no excuse to skip it; `$traceability-check`/`$traceability-audit` verify hardening per new artifact before closure. Canon: the harness manifest block `AE-HARNESS-MANIFEST-ARTIFACT-CREATION-HARDENING`.
- Read the canonical AE guidance and phase definitions before planning; the harness orchestration canon for harness/shared-skill/policy/adapter work. For any deployable task (diff touching `src/**`, `deployment configuration/**`, or the tracked environment template), the post-deployment smoke policy governs G1 (business-endpoint smoke), G2 (env-var diff guard), G3 (ExpectedSha mandatory), G4 (deploy vs redeploy evidence), plus auto-rollback (DL-008) and drift sentinel (DL-010).
- `ae_budget_gate` fields: `effort_class`, `persistence_mode`, `governance_depth`, `context_loading_profile`, `evidence_loading_profile`, `artifact_lifecycle`, `closure_profile`, `worker_budget`, `worker_decision`, `worker_adapter_available`, `worker_authorized_by_user`, `independent_axes`, `why_no_worker`, `why_not_cheaper`. Goal persistence, full suites, live/runtime, worker fanout, full trace/audit, and full governance require `why_not_cheaper`.
- CQA work also declares `cqa_budget_gate`: `qa_effort`, `evidence_profile`, `retry_budget`, `file_ownership_partition`, `preflight_stamp`, `why_run_again`, `why_not_cheaper`. Start from `QE0_INVENTORY`/`QE1_VERDICT_ASSERTIONS`; use `reentry-packet` and `preflight-stamp` before raw turns/logs.
- Select and record one AE work mode: FAST | STANDARD | STRICT. Route genuinely human-owned execution-changing decisions through `$ae-decide` before deterministic execution.
- Keep filename `session-contract.yaml`; add the mandatory `ae_contract` overlay for policy, shared-skill, harness, or non-trivial mutating work.
- Primary skills: `$ae-crear-politicas`, the service-policy skill, `$ae-pre-push`. Legacy aliases (`legacy-policy-creator`, `legacy-service-policy-creator`, `legacy-pre-push`) remain callable but are not authority.
- Shared-skill changes (governed by the harness orchestration canon): update the configured shared skills directory and `<org>/assets/skills` in the same run; closure evidence needs source path, mirror path, SHA-256 for both, and `byte_identical: true`. Don't push policy/harness/shared-skill changes until source/mirror sync, traceability, audit, and `$ae-pre-push` evidence exist.
- Legacy A-G/G.1 workflow names are read-only aliases mapped in the legacy alias registry.

### Skill Invocation Semantics

- **Task start**: declare `ae_budget_gate`; invoke `$ae-work` for non-trivial, mutating, policy/harness/shared-skill, live/runtime, or multi-step work.
- **Inside exploration-worker / orchestrator spot-verify**: `$project-context-cli` semantic backend under `src/`.
- **Before mutating work**: `$using-git-worktrees` only when the gate/base risk requires isolation.
- **Large/risky/multi-step**: `$writing-plans` after brainstorming.
- **Inside ae-work**: continue mechanical work directly; route unresolved human-owned execution-changing decisions to `$ae-decide`.
- **Policy edits (`AGENTS.md`/`CLAUDE.md`/`SUBAGENTS.md`/`PATHS.md`)**: `$ae-crear-politicas`. Service policies: the service-policy skill. Cross-projection drift: `$ae-projection-audit`.
- **Governance unhealthy**: `$crear-gobierno-documental` is the mandatory repair skill.
- **Code-writing workers**: delegate through the selected adapter for mutating/code/docs/policy work whenever the scope requires workers and the adapter is available. `worker_decision=spawned` is the required state; `worker_decision=none` is only for `C0_INLINE_NO_DIFF` true read-only/no-diff work with no independent axes.
- **Closure → push**: `$traceability-check` → `$traceability-audit` (rerun after drift) → external tracker sync only when tracker.mode is external and the task touched the tracker → `$ae-pre-push` before any `git push` to `main`.
- **After integration / PR-open / hold / cleanup**: `$finishing-a-development-branch`.
- **Final response gate**: governed completed work cannot end with closure steps as user follow-up; execute the closure completion loop or return a BLOCKED packet with owner, blocker class, and next action.

### Project Management / Tracker Applicability

- When tracker.mode is external, the configured tracker provider is the source of truth for tickets, workflow, ownership, and closure.
- When tracker.mode is none, the local session contract, git history, and evidence artifacts own the workflow.
- Tracker-specific issue, claim, parent-agent, ticket-frontier, sync, key, endpoint, URL, and lock language applies only in external mode.
- In external mode, the configured tracker adapter/helper owns endpoint, auth, state mapping, and project routing; never assume a CLI name, GraphQL shape, or shared state vocabulary.
- In external mode, governed code modification uses the provider adapter's parent/execution work-item model; the only exception is a bootstrap/emergency waiver in `session-contract.yaml`, and the waiver reference is the configured external issue or identifier when one exists.
- In external mode, use the provider adapter's canonical state mapping for planning, execution, triage, WIP, and closure; never translate through assumed Planning/Execution states.
- In external mode, before taking a ticket or claiming repo edits, check assignee, workflow state, latest claim/scope comments, and update the ticket frontier in `session-contract.yaml`.
- In none mode, the local session-contract frontier owns out-of-frontier scope; parking-lot comments still capture discoveries.
- Traceability, audit, and pre-push remain mandatory in both modes.
- This section governs any generic tracker wording below.

### Governance Gate + `project-context-cli` Defaults

**Governance**: the canonical governance document is the human governance authority; the versioned context-tool read model is its versioned executable projection. Diagnose via `project-context-cli workspace status <alias> --format toon` + `project-context-cli nav governance --workspace <alias> --format toon`. If governance is ambiguous, invalid, stale, or out of sync → stop and run `$crear-gobierno-documental` before continuing. `$traceability-check` and `$traceability-audit` verify governance completeness and `00 ↔ read-model.toml` projection sync before closure.

**project-context-cli** (reference-not-duplicate doctrine — `$project-context-cli` owns command tables, alias validation, telemetry):
- Project workspace alias: `mi-lsp`. Always pass `--workspace mi-lsp --format toon`; in container workspaces add `--repo <name>` before broader queries.
- CLI-first; don't wait for an MCP path when the CLI answers. Use at T1+ for semantic navigation and inside every spawned `$exploration-worker` dispatch; exact T0 searches may use `rg` first.
- Fallback order: `project-context-cli` → `rg` (canonical-doc only) → `Read`; don't skip steps. If `project-context-cli` returns `items: []` with a `hint`, act on the hint before retrying.

### Platform Runner Guard

Codex Desktop workers on Windows MUST pass `<repo-scripts>/Test-PlatformRunner.ps1` (or `platform_runner_guard.py --workspace mi-lsp`) before an external tracker lock, worktree, or long prompt. `BLOCKED_PLATFORM_RUNNER` → follow `<repo-docs>/runbooks/platform-runner.md`. Evidence: `.docs/auditoria/<task>/platform-runner-guard.{json,md}`.

## Orchestration Mode (Always Active)

For work that is non-trivial, mutating, governed, live/runtime, shared-skill, policy, or multi-step:

1. Declare or infer `ae_budget_gate` (T0/T1 may stay inline, no persistence).
2. `$ae-work` — default gateway for non-trivial scope; execute FAST work directly and use ROI-positive workers only for independent STANDARD/STRICT axes.
3. Context loading follows `context_loading_profile`; `$context-loader` + governance gate only when the profile justifies them.
4. `$brainstorming` once before planning/execution; close critical gaps via `AskUserQuestion` (or chat).
5. **Human decision routing**: use `$brainstorming` for a human decision only after classifying the gap as execution-changing and not parent/owner-resolvable; otherwise continue with the owner thread, bounded worker, formal blocker, or durable closure artifact. If the decision is not asked immediately, add/update the session's `human-decision-brainstorming-protocol.md` packet; do not batch multiple `ready_for_operator` packets into one question unless the operator explicitly asks for a combined review.
6. **Worker decision**: record `worker_decision`, `worker_budget`, adapter availability, authorization, independent axes, and either `why_not_cheaper` or `why_no_worker`.
7. **Execution/review wave**: route implementation/review/QA/docs/ops to workers for T2+, multi-step, mutating, policy/harness/shared-skill, runtime/deployable, or independent-axis scope when an adapter is usable. Local work is valid only for `C0_INLINE_NO_DIFF`, orchestration, integration, citation verification, and final stitching after compact worker verdicts.
8. `$traceability-check` then `$traceability-audit` before marking complete.
9. external tracker sync before closure only when tracker.mode is external and the task touched the tracker.
10. Before any `git push` to `main`: `$ae-pre-push` after traceability/audit + external tracker sync when tracker.mode is external.
11. Fresh-session continuation/bootstrap: `$continuation-prompt` after traceability, targeting the next harness.

Standing rules:
- User grants permission to launch workers when `ae_budget_gate.worker_decision: spawned`; only irreversible external actions outside repo/runtime scope require confirmation.
- Persistence follows `persistence_mode`: T0/T1 may use none/inline, T2 uses task packet or mini contract, T3/T4 use session contract/full governance. Post-audit drift -> rerun the selected gates.
- Approved closure with `branch_disposition: integrate-main` → guarded integration, then sanitize deprecated worktrees/branches. Preserve only `hold`/`pr-open`/`active-followup`/`cleanup-blocked` with evidence.
- **Post-deploy closure** (deployable changes on `origin/main`): blocked until the affected surface is verified working AND a non-production health sweep covers every canonical microservice/store. Evidence under `.docs/auditoria/<session>/`: target env, affected surface, deployed ref or SHA-drift blocker, deployment platform status, smoke results. Any failed probe / missing deploy / stale ref / unknown status = `deployment-drift` → PASS loop §G.1; sweep-promoted artifacts trigger a final `$traceability-check` + `$traceability-audit` refresh before marking integrated.
- **PASS-gate**: any FAIL/BLOCKED from repo/runtime drift, deployment mismatch, flaky harness, stale evidence, or unclassified error is not closure. Iterate fix → redeploy/retest → evidence until fully PASS or human-approved external blocker recorded. Keep ticket frontier + contract + evidence + external tracker claim comments current when tracker.mode is external.
- XP on `main` requires `$ae-pre-push` immediately before push. Guard blocks on: non-fast-forward; missing parent/agent issue or ticket frontier when tracker.mode is external; undeclared critical surfaces; missing evidence; stale tracker state only in external mode; `.docs/raw/` outside `plans/`+`prompts/` allowlist; missing SDD frontmatter; dangerous untracked artifacts under `src/`. **Force-push to `main` is never allowed.**
- Edits to `AGENTS.md`/`CLAUDE.md` use `$ae-crear-politicas` + `$ae-projection-audit`; `$context-loader` reads the architecture doc to identify active microservices before planning.
- Legacy UX/UI canon paths → `$ux-canon-migrator` before any new UX/UI task. Do not generate `docs/ux/ui-rfc/*` before the validation matrix + validation evidence exist. Do not start visible implementation before the operational UX handoff chain exists.

## Language Rule

Keep `AGENTS.md`, `CLAUDE.md`, and project-local skills in English. All other project documentation is in Spanish.

## Collaboration Rules

**Style**: avoid emojis in policy/governance outputs. User-facing copy (UI labels, errors, placeholders, buttons, banners, toasts, modals) MUST preserve correct Spanish orthography — locale-specific characters, punctuation, and inflection. Applies to hardcoded strings, i18n keys, dynamic copy. Verify against the configured language standard.

**Mandatory wrappers** (never bypass with raw `curl`/`ssh`/`psql`/`sqlcmd`/MCPs when the wrapper expresses the action):
  - name: mi-lsp
    script: mi-lsp
    precondition: Set MI_LSP_CLIENT_NAME and MI_LSP_SESSION_ID before governed navigation.
    authority: repository policy

**private network precondition**: before `deployment-cli` or `remote-cli`, confirm private access to the target environment (ask the user if not already confirmed).


## Repository-Specific Contract

### Repository Description

Local semantic CLI for large .NET/C# and TypeScript workspaces.

### Repository Structure Rules

  - Go CLI and daemon code lives under cmd/ and internal/.
  - The Roslyn worker lives under worker-dotnet/.
  - .docs/wiki is the canonical repository documentation authority.
  - .docs/auditoria/<session>/ stores durable sanitized evidence.
  - .docs/raw/ remains non-canonical local input.

### UX Orthography Rules

  - Preserve Spanish orthography and diacritics in documentation.

### Mandatory Wrappers

  - name: mi-lsp
    script: mi-lsp
    precondition: Set MI_LSP_CLIENT_NAME and MI_LSP_SESSION_ID before governed navigation.
    authority: repository policy

### QA Canon Paths

  - .docs/wiki/06_matriz_pruebas_RF.md
  - .docs/wiki/06_pruebas/

---

**Version**: AGENTS.md (AE-KERNEL-V2)
**Status**: Generated from AE-POLICY-PROJECTION-V2
**Last Updated**: 2026-07-24
**Source**: repo-policy.yaml + template.agents
<!-- kernel_version: 88a02363 -->
