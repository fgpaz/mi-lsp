# AGENTS.md — mi-lsp Agentic Engineering Policy

> **Authority**: Architecture + AE-KERNEL-V2.md. **Language**: English. L0 operating policy for mi-lsp.

## North Star Cycle

Wiki is the only source of truth.

1. **Context** — wiki anchor + mi-lsp implication graph + architecture decisions from wiki (reuse session context-packet).
2. **Build** — gate-free; implement **all** code/docs for the goal (no C2 tests/builds/lints). Lanes are not completion.
3. **IMPLEMENTATION_COMPLETE** — parent only, when inventory is green.
4. **Verify once** — single FINAL_VERIFY wave (build/tests/impact map); not per lane. FAST local: collapsed `verify_packet`.
5. **Wiki close** — update wiki if drift. Drift is wiki vs shipped behavior, not omitted `.docs/wiki` files. Product-visible shipped work without RS/FL/RF/TP/TECH coverage is drift; FAST still runs `ps-asistente-wiki` plus targeted `ps-trazabilidad` then `ps-auditar-trazabilidad` at FAST depth when that drift exists or the operator asked for ciclo AE / `full_cycle_to_origin_main`. Collapsed `verify_packet` compresses C2; it does not replace wiki routing or FAST-depth trace/audit. Skip targeted wiki+trace only for mechanical no-wiki-impact diffs (typo/format/no user-visible behavior). Do not wait to be re-asked.
6. **Full cycle default** — verify_packet (collapsed or full trace/audit) → close drifts → pre-push → integrate `origin/main` → **always** sanitize deprecated branches/worktrees → learning route (`ps-wiki-aprendizaje` only with pilot/qa inputs; else friction/skipped_fast). Next session: `ps-contexto` reentry of last classification + open friction (do not re-ask).

## Chief of Staff (strict)

The principal session is **Chief of Staff**, not a leaf implementer.

- **Principal does**: understand intent, prioritize, route (`mi-lsp skills plan` when multi-skill), launch leaves, monitor, join via anti-C2 contract, decide, stitch compact results, **own goal phase** (`BUILDING` → `IMPLEMENTATION_COMPLETE` → `FINAL_VERIFY`) and **session-state** index.
- **Leaves do**: read, edit within bounded scope; report `lane_implementation_done` with paths only; **no** C2 validation, no phase change, no subdelegation, push, deploy, or final closure.
- **Join ≠ FINAL_VERIFY** at leaf **or** axis/child-orchestrator: never run build/test/lint when a lane or sub-orchestrator finishes (locked A). Parent rejects joins with C2 evidence (`accept-join`).
- **C2 deferred** until goal-root IMPLEMENTATION_COMPLETE unless the operator **explicitly** requests a bounded intermediate check (B = ALLOW_SPOT; never hard-block the operator for asking).
- **Same-repo FAST (1–few files, reversible)**: almost-zero ceremony — no preflight ritual, no session-contract required. Principal may implement directly only when isolation/fan-out cost exceeds value.
- **Non-trivial / multi-file / multi-axis**: launch leaves; do not expand the principal into full implementation.
- Leaf dispatch is ROI-positive: spawn when separable ownership and expected value beat orchestration cost. Adapter availability alone never forces a worker.
- If spawn is blocked: announce owner_surface + rule + how_to_unblock. Silent multi-axis parent-implement is forbidden.
- `$writing-plans` is for orchestrator→leaf task packets only, not a human-facing ceremony.

## Context rules (ps-contexto)

- **Flujos** = mi-lsp implication graph around the task (`nav flow-slice`, `change-pack`, `related`, `neighbors`, `affected`, `explain-change`) — not wiki `FL-*` alone.
- Load **architecture decisions** from the wiki; never re-decide or forget locks already documented.
- Route-first mi-lsp: prefer `nav batch` + `nav multi-read`; depth `policy` for kernel/skill-only (skip product/UX/negocio gates).
- Session SoT: context-packet + goal-phase (indexed by session-state). `mi-lsp prepare` is not session SoT.

## mi-lsp diet (token-cheap nav)

Workspace: `mi-lsp`. Default `--format toon` (leaves: prefer `--profile harness-micro --compress`).

Preferred commands only:
1. `mi-lsp nav search <pattern>`
2. `mi-lsp nav find <symbol>`
3. `mi-lsp nav related <symbol>` / `nav flow-slice --selector <s>` / `nav neighbors <s>`
4. `mi-lsp nav route <task>` / `nav change-pack` when diff-scoped
5. `mi-lsp nav batch` / `nav multi-read` to cut process starts
6. `mi-lsp skills plan --role parent --task "..."` when multi-skill routing is unclear
7. `mi-lsp nav evidence inventory` at close reentry; `nav governance` only when governance matters

Fallback: mi-lsp → rg → Read. Do not treat mi-lsp as a governance ritual on every task.

## FAST same-repo ceremony (almost zero)

- No mandatory session-contract for same-repo FAST.
- No mandatory preflight suite for local reversible work.
- Pre-push minimum: `.\infra\git\Invoke-PrePushGuard.ps1 -CollaborationMode solo` (secrets scan + main ancestry + no force-push path).
- Escalate ceremony only for multi-axis work, live/runtime, secrets, external side effects, or explicit HOLD.
- **Completion default** remains full cycle to `origin/main` + **always** sanitize deprecated worktrees/branches unless the operator says local-only. No HOLD whose only message is “please integrate/clean yourself”. Leaving known-deprecated artifacts is incomplete.
- FAST close uses **collapsed verify_packet** (not two full trace+audit tours of the same evidence).

## Kernel Fast Runtime v3

```toon
policy_revision: kernel-fast-governance-v3
execution_mode: FAST
closure_profile: governed_kernel_change
build_loop: {tests: false, audits: false, traceability: false, evidence: false, structural_check_max_seconds: 0, c2_validation: false}
final_verify: {count: 1, method: impact_map, fresh_verifier: required, single_wave: true}
evidence: {mode: only_publish, packet: single_closure_packet, durable_before_publish: false}
```

BUILD LOOP is gate-free. FINAL VERIFY runs once with a fresh verifier. Publish only one sanitized closure packet. External-only actions are BLOCKED without an explicit allowlist and explicit closure authorization. Completion handoff must not infer authority for commit, push, merge, remote mutation, deletion, or evidence reaping.

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
**Last Updated**: 2026-08-25
**Source**: repo-policy.yaml + template.agents
<!-- kernel_version: 6f4da8b4 -->
