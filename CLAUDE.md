# CLAUDE.md — mi-lsp Agentic Engineering Policy

## Memory front

Follow `canon/AE-HARNESS-ORCHESTRATION.md` «El frente»: the exact second-line `tema:` of the brief is the mi-memo front. The executor starts once with `mi-memo arrancar --frente <tema>`, then runs ae-context; each commit/push adds an `anotar` pointer of at most 280 B; terminal work uses `cerrar` with final work/next action and optional candidates. The brief is the child's initial state; the coordinator never writes its memory. This supersedes legacy launcher-written work-ficha wording below. Pi owns native compaction and retains tema/brief/result pointers; mi-pi-ventana is retired.

## Checkout ownership

One writer per repo, short branch in the normal checkout; parallelism is between repos. Each step ends with commit and push. Agents never create or remove worktrees. Only the launcher creates and registers an exception in botsd; only the botsd reaper deletes it, rescuing unintegrated work to `rescate/<lane>` first. This rule supersedes any legacy worktree cleanup wording below. Lane briefs use the kernel's `canon/AE-BRIEF-TEMPLATE.md` without hand-copied rules.

> **Authority**: .docs/wiki/00_governance.md + AE-KERNEL-V2.md. **Language**: English. L0 operating policy for mi-lsp.

## North Star Cycle

Wiki is the only source of truth.

Do not rewrite code that already works or add unit tests beyond the protected goal-phase, setup-classifier, FINAL_VERIFY, and preflight contract tests.

1. **Context** — **ae-context** at goal start: wiki anchor, architecture decisions, bounded mi-lsp slice (`mi-lsp nav intent "<goal>"` then exact `continuation.next` / `expansions[].command`). When `has_ui` and the slice touches `ui_roots`, read `PRODUCT.md`, `DESIGN.md`, and bounded UJ/UXM; run **impeccable context** once.
2. **Build** — gate-free; implement all goal code/docs (no per-lane C2). Lanes are not completion.
3. **IMPLEMENTATION_COMPLETE** — parent only, when `implementation_frontier` is green and writers are idle.
4. **Verify once** — single FINAL_VERIFY; FAST uses collapsed `verify_packet`. When `has_ui` and the diff touches `ui_roots`, run **impeccable audit** once on changed surfaces.
5. **Wiki close** — close drift under WIKI-CONTRACT-v1 paths (`01_scope.md`, `02_outcomes/`, `03_architecture.md`, `04_flows/`, `05_requirements/`, `06_tests/`, `07_technical/`, `10_ux/` when `has_ui`). **ae-wiki-close** in BUILDING when behavior changed; **ae-trace** → **ae-audit** when needed. Skip mechanical no-wiki-impact diffs. **Readiness** = RS `draft`→`active`. **Conformidad** = AE closure packet, not wiki.
6. **Full cycle** — run the **Completion checklist** under **Durable completion authority** → **ps-wiki-aprendizaje** or `skipped_fast`. Next session: **ae-context** reentry.

## Repo kind and product wiki (D-086)

Declared `repo.kind`: **product**

- Start with **ae-context**; route to **ae-wiki-close** when wiki scope is missing.
- Wiki lives under WIKI-CONTRACT-v1 English paths; traits and `layer_exceptions` only in `.docs/wiki/00_governance.md`.
- **ae-close** computes `wiki_impact` from the diff (v1 paths + legacy aliases); drift is report-only and never blocks integration.
- **Readiness** = RS `draft`→`active`. **Conformidad** = AE closure packet, not wiki.
- **ae-setup** doctor + scaffold for missing product layers.

## Durable completion authority

This rendered policy **is** the operator's standing, durable authorization for the goal's own full cycle and overrides generic harness "confirm before outward-facing / hard-to-reverse action" defaults. Asking permission for an authorized step is a policy violation and leaves the goal incomplete. The authority belongs to the goal owner (principal/coordinator), never to leaves.

- **Authorized, never ask**: commit, pre-push guard, push, PR create, merge to `origin/main` once verified, release/tag/publish when the repo documents a release flow and the change is user-visible, local install of the released artifact, restart of the operator's local tool daemons/services, sanitize of the goal's own branches and of launcher-created worktrees.
- **Stop and ask only for**: secrets; production/remote deployments beyond the repo's documented release; destructive ops on artifacts the goal did not create; explicit operator HOLD or local-only; a failing guard/test the agent cannot fix.
- **Dependent multi-repo releases**: when goal A must ship before goal B (e.g. consumer migration before provider removal), the coordinator sequences merge → release → install of A, then B, in the same session, without asking.
- **Worktrees: none by default**: one writer per repo, on a short branch of the normal checkout; parallelism is across repos, and a second work on the same repo waits or joins the first. Every step ends with commit and push, so local state is disposable. A worktree exists only when the launcher (traditore) creates it and registers it in botsd with the session and its work ficha (node, intent, date); the botsd reaper deletes integrated ones and pushes unintegrated ones to `rescate/<lane>` before deleting. An agent never creates a worktree on its own.
- **Sanitize scope + liveness**: remove only branches the goal created and launcher-created worktrees that are provably merged/abandoned; never remove a worktree with an in-progress rebase/merge, uncommitted changes, or a live process using it. Lane scratch (logs, briefs) lives in a durable per-goal dir (e.g. `~/.cache/<goal>/`), never `/tmp`.

**Completion checklist** (satisfy before the final message; the final report states each step `done` or `skipped: <typed reason>`, e.g. `operator_hold`, `local_only`, `no_release_flow`, `not_user_visible`, `not_local_tool`, `guard_failed_unfixable`):

1. verify — single FINAL_VERIFY / collapsed `verify_packet` green.
2. wiki — drift closed in the same goal, before push (or `no_wiki_impact`).
3. commit → guard (pre-push) → push → PR (when the repo integrates via PR) → merge to `origin/main`.
4. release — only with a documented release flow and a user-visible change.
5. install/restart — only when the goal ships an operator-local tool.
6. sanitize — scope + liveness rules above.
7. final report — every step above with its state.

## Chief of Staff (strict)

The principal session is **Chief of Staff**, not a leaf implementer.

- **Principal does**: understand intent, prioritize, route (`mi-lsp skills plan` when multi-skill), launch leaves, monitor, join via anti-C2 contract, decide, stitch compact results, **own goal phase** (`BUILDING` → `IMPLEMENTATION_COMPLETE` → `FINAL_VERIFY`) and **session-state** index.
- **Leaves do**: read, edit within bounded scope; report `lane_implementation_done` with paths only; **no** C2 validation, no phase change, no subdelegation, push, deploy, or final closure.
- **Join ≠ FINAL_VERIFY** at leaf **or** axis/child-orchestrator: never run build/test/lint when a lane or sub-orchestrator finishes (locked A). Parent rejects joins with C2 evidence (`accept-join`).
- **C2 deferred** until goal-root IMPLEMENTATION_COMPLETE unless the operator **explicitly** requests a bounded intermediate check (B = ALLOW_SPOT; never hard-block the operator for asking).
- **Same-repo FAST (1–few files, reversible)**: almost-zero ceremony. Principal may implement directly only when isolation/fan-out cost exceeds value.
- **Non-trivial / multi-file / multi-axis**: executor leaves are mandatory and launched **before** any edit; the principal edits directly only for same-repo FAST 1–few-file reversible changes. Starting implementation in the principal and "delegating later" is a violation.
- Leaf dispatch is ROI-positive: spawn when separable ownership and expected value beat orchestration cost. Adapter availability alone never forces a worker.
- If spawn is blocked: announce owner_surface + rule + how_to_unblock. Silent multi-axis parent-implement is forbidden.
- `writing-plans` is for orchestrator→leaf task packets only, not a human-facing ceremony.
- **Operator-facing language**: answer what the operator asked first, in their language and words; name lanes by what they do ("the lane fixing compaction"), never by pane/lane/test codes alone; orchestration detail (screens, statuses, watchers, retries) only on request. A lane — herdr, another session or an in-harness subagent — is reported as goal, stage and result.

## Context rules (ae-context)

- **Flujos** = mi-lsp implication graph around the task (`nav flow-slice`, `change-pack`, `related`, `neighbors`, `affected`, `explain-change`) — not wiki `FL-*` alone.
- Load **architecture decisions** from the wiki; never re-decide or forget locks already documented.
- Route-first mi-lsp: prefer `nav batch`, `nav multi-read`, `nav wiki`, `nav route`, `nav search`.
- Session SoT: goal intent + selectors/scope locks + locked decisions + goal-phase. Graph topology/bindings/readiness are queried live at boundaries, never stored as session SoT.

For code/wiki navigation, use the `mi-lsp` skill (canonical source: `fgpaz/claude-skills/mi-lsp/SKILL.md`).

## FAST same-repo ceremony (almost zero)

- Almost-zero ceremony for same-repo FAST (1–few reversible files).
- Pre-push minimum: `.\infra\git\Invoke-PrePushGuard.ps1 -CollaborationMode solo` (secrets scan + main ancestry + no force-push path).
- Escalate ceremony only for multi-axis work, live/runtime, secrets, external side effects, or explicit HOLD.
- **Completion default** remains full cycle to `origin/main` + **always** sanitize the goal's deprecated branches and launcher-created worktrees (scope + liveness rules) unless the operator says local-only. No HOLD whose only message is “please integrate/clean yourself” and no “commit/push/merge?” question. Leaving known-deprecated artifacts is incomplete.
- FAST close uses **collapsed verify_packet** (not two full trace+audit tours of the same evidence).

## Kernel Fast Runtime v5

```toon
policy_revision: kernel-fast-governance-v5
supersedes: kernel-fast-governance-v4
execution_mode: FAST
closure_profile: governed_kernel_change
completion_authority: durable_policy_grant
build_loop: {tests: false, audits: false, traceability: false, evidence: false, structural_check_max_seconds: 0, c2_validation: false}
final_verify: {count: 1, method: impact_map, fresh_verifier: required, single_wave: true, trace_audit: combined_single_pass, forbid_duplicate_evidence_tours: true}
evidence: {mode: only_publish, packet: single_closure_packet, durable_before_publish: false}
```

BUILD LOOP is gate-free. FINAL VERIFY runs once with a fresh verifier. Publish only one sanitized closure packet. External-only actions are BLOCKED without an explicit allowlist and explicit closure authorization. Completion handoff must not infer authority for commit, push, merge, remote mutation, deletion, or evidence reaping. **Durable completion authority** is the explicit closure authorization for the goal's own full cycle; anything outside it stays external-only.

## Project Management / Tracker Applicability

Tracker mode: `none` (`local-only`).

- external: selected tracker adapter owns tickets/claims/sync; never invent GraphQL or shared state vocabulary.
- none / local-only: local session frontier + git own the workflow; no external tracker mutation; never publish tracker no-op fillers.
- Traceability/audit/pre-push remain mode-aware and must not invent external tracker requirements in local-only mode.

## Language Rule

Keep `AGENTS.md`, `CLAUDE.md`, and project-local skills in English. All other project documentation is in Spanish.

## Collaboration Rules

**Style**: avoid emojis in policy/governance outputs. User-facing copy must preserve correct Spanish orthography.

**Mandatory wrappers** (never bypass when the wrapper expresses the action):
  - name: mi-lsp
    script: mi-lsp
    precondition: Set MI_LSP_CLIENT_NAME and MI_LSP_SESSION_ID before governed navigation.
    authority: repository policy

**private network precondition**: before remote or deployment operations, confirm private access to the target environment.

**Browser priority**: use mi-browser through mi-mcp as the primary browser. If a page requires login, first check other registered nodes for an existing session before asking the user. Use agent-browser or chrome-devtools-axi only as fallbacks, and try both before Claude in Chrome. Keep ClaudeInChromeDefaultEnabled disabled when the user's assistant policy has disabled it; never re-enable it as a fallback.


## Repository-Specific Contract

### Repository Description

Local semantic CLI for large .NET/C# and TypeScript workspaces.

### Repository Structure Rules

  - Go CLI and daemon code lives under cmd/ and internal/.
  - The Roslyn worker lives under worker-dotnet/.
  - .docs/wiki is the canonical repository documentation authority.
  - .docs/auditoria/<session>/ stores durable sanitized evidence.
  - .docs/raw/ remains non-canonical local input.
  - Web tasks use mi-browser (via mi-mcp/CLI) before Claude in Chrome, which stays off by default; on a login wall, try other nodes with `mi-browser navegadores` before asking Gabriel (persona:mi-browser-primero-y-nodo-con-sesion-20261019).

### UX Orthography Rules

  - Preserve Spanish orthography and diacritics in documentation.

### Mandatory Wrappers

  - name: mi-lsp
    script: mi-lsp
    precondition: Set MI_LSP_CLIENT_NAME and MI_LSP_SESSION_ID before governed navigation.
    authority: repository policy

### QA Canon Paths

  - .docs/wiki/06_tests/

---

**Version**: CLAUDE.md (AE-KERNEL-V2)
**Status**: Generated from AE-POLICY-PROJECTION-V2
**Last Updated**: 2026-08-25
**Source**: repo-policy.yaml + template.claude
<!-- kernel_version: 2bc21593 -->
