# PATHS

This file is a projection of `.docs/wiki/ae/AE-PROJECTION-POLICY.md`.
The wiki remains the source of truth; this file gives agents a compact path map.

## AE Programa Gateway (MANDATORY)

`ae-programa` is the mandatory gateway before any non-trivial, mutating, policy, harness, shared-skill, or multi-step work in this repository.

Before functional work, validate the local AE layer is complete:

- `.docs/wiki/ae/README.md`
- `.docs/wiki/ae/AE-PHASES.md`
- `.docs/wiki/ae/AE-HARNESS-MANIFEST.md`
- `.docs/wiki/ae/AE-HARNESS-ORCHESTRATION.md`
- `.docs/wiki/ae/AE-WORK-MODES.md`
- `.docs/wiki/ae/AE-SESSION-CONTRACT.md`
- `.docs/wiki/ae/AE-PROJECTION-POLICY.md`
- `.docs/wiki/ae/AE-EVIDENCE-POLICY.md`
- `.docs/wiki/ae/AE-RELEASE-DISTRIBUTION.md`
- `AGENTS.md`, `CLAUDE.md`, `PATHS.md`
- `scripts/ae/pre-push-guard.ps1`

If any required AE file is missing or contradicted, enter `manifest_repair` mode and repair the AE layer before doing functional work. `AGENTS.md`, `CLAUDE.md`, and `PATHS.md` are projections; `.docs/wiki/ae/**` is the AE source of truth.

Every mutating or non-trivial task must create/update `.docs/auditoria/<YYYY-MM-DD>-<task-slug>/session-contract.yaml` with an `ae_contract` block before edits. The contract must name selected mode, decision lock, adapter, orchestration depth, `worker_decision`, independent axes, launch/join evidence or blocker, allowed paths, forbidden paths, required evidence, stop conditions, and cleanup policy.

Adapter selection is manifest-first: discover global `ae-adapter-*` skills, read `adapter_manifest.schema=ae-harness-adapter/v1`, prefer an explicit user-requested harness, then current/project harness fit. If a usable adapter exists for required worker scope, it must launch real workers with `worker_decision=spawned`; fall back to `simulated_packets` with `missing_ae_adapter_manifest` only when no adapter satisfies evidence and isolation. `ae-adapter-hermes` and `ae-adapter-claude-code` are proof-gated partial seeds until a native `ae-adapter-proof/v1` proves spawn, monitor, join, fallback, evidence, and sanitization.

Audit hygiene is mandatory for non-trivial `.docs/auditoria/<YYYY-MM-DD>-<task-slug>/`: write `audit-manifest.yaml` with `schema: ae-audit-hygiene/v1`, `retention_ttl_days: 14`, `hash_algorithm: sha256`, artifact classes, promoted summary/verdict, and cleanup status. Raw logs, screenshots, transcripts, prompts, and plans are temporary unless promoted or explicitly held; durable evidence cannot be deleted without replacement summary, hash, path, date, owner, and reason.

Governed AE work must also record `mi_lsp_preflight` in the session contract before worker launch, historical audit, closure, push, or PR-ready claims. Set `MI_LSP_CLIENT_NAME` and `MI_LSP_SESSION_ID` before every `mi-lsp` command; `client_name=manual-cli`, a default `session_id` like `cli-<pid>`, `governance_blocked=true`, `docs_ready=false`, `doc_count=0`, or `ae_canon.status` in `missing|mismatch|projection_only` is a hard blocker, not a warning.

Subagents or worker lanes are mandatory for AE-governed T2+, mutating, multi-step, policy/harness/shared-skill, runtime/deployable, or independent-axis work. Zero-subagent execution is compliant only for `C0_INLINE_NO_DIFF` true read-only/no-diff work with no independent axes; any `why_no_worker` outside that case is blocker evidence, not authorization for local execution. Default recursion depth is `v0_shadow`: two active levels, third-level task orchestrators are shadow-only until a later supervised pilot.

WSL/subagent/worker execution audits must be read-only first and must produce a worker/session attribution matrix, admin export summary, manual-cli exception review, and WSL evidence-handling note. Do not mutate WSL filesystems, rewrite histories/logs, dump raw shell history, or treat telemetry/transcripts as canon.

Before any push or PR-ready claim, run `scripts/ae/pre-push-guard.ps1` with the active session contract, then close with `ps-trazabilidad` and `ps-auditar-trazabilidad`. If any diff, branch, evidence, scope, or tracker state changes after audit, rerun both closure gates.

## Subagent Orchestration Protocol

- Every required worker scope must launch subagents or worker lanes after `ae-programa` locks the session contract and selects a usable adapter.
- First wave is read-only exploration; implementation writes go to specialized implementation or worker lanes.
- Minimum lanes: 0 only for `C0_INLINE_NO_DIFF`; 1+ for T2+/mutating/multi-step/policy/harness/shared-skill/runtime/deployable scope; 3+ for medium independent axes; 5+ for complex or cross-layer work.
- Delegated tasks must be atomic, path-bounded, and evidence-bounded; subagents return summaries with file/line or command evidence, not raw dumps.
- The orchestrator must verify cited paths/results before integrating. Contradictory subagent results trigger another bounded verification lane.
- Zero-subagent execution requires `C0_INLINE_NO_DIFF` true read-only/no-diff/no independent axes in the session contract.

## Canon

- Governance authority: `.docs/wiki/00_gobierno_documental.md`
- Governance projection: `.docs/wiki/_mi-lsp/read-model.toml`
- Functional canon: `.docs/wiki/01_alcance_funcional.md`, `.docs/wiki/02_arquitectura.md`, `.docs/wiki/03_FL.md`, `.docs/wiki/03_FL/`, `.docs/wiki/04_RF.md`, `.docs/wiki/04_RF/`, `.docs/wiki/05_modelo_datos.md`, `.docs/wiki/06_matriz_pruebas_RF.md`, `.docs/wiki/06_pruebas/`
- Technical canon: `.docs/wiki/07_baseline_tecnica.md`, `.docs/wiki/07_tech/`, `.docs/wiki/08_modelo_fisico_datos.md`, `.docs/wiki/08_db/`, `.docs/wiki/09_contratos_tecnicos.md`, `.docs/wiki/09_contratos/`
- AE canon: `.docs/wiki/ae/`

## AE Required Docs

- `.docs/wiki/ae/README.md`
- `.docs/wiki/ae/AE-PHASES.md`
- `.docs/wiki/ae/AE-HARNESS-MANIFEST.md`
- `.docs/wiki/ae/AE-HARNESS-ORCHESTRATION.md`
- `.docs/wiki/ae/AE-WORK-MODES.md`
- `.docs/wiki/ae/AE-SESSION-CONTRACT.md`
- `.docs/wiki/ae/AE-PROJECTION-POLICY.md`
- `.docs/wiki/ae/AE-EVIDENCE-POLICY.md`
- `.docs/wiki/ae/AE-RELEASE-DISTRIBUTION.md`

## Operational Evidence

- Session contract: `.docs/auditoria/<YYYY-MM-DD>-<task-slug>/session-contract.yaml`
- Operational ledgers: `.docs/auditoria/<YYYY-MM-DD>-<task-slug>/orchestrator-registry.yaml`, `decision-ledger.yaml`, `recursion-learning-log.yaml`, `evidence-index.yaml`
- Traceability closure: `.docs/auditoria/<YYYY-MM-DD>-<task-slug>/traceability-closure.yaml`
- Audit report: `.docs/auditoria/<YYYY-MM-DD>-<task-slug>/audit-report.yaml`

## Scripts

- Release gate: `scripts/release/ae-release-binaries.ps1`
- AE pre-push guard: `scripts/ae/pre-push-guard.ps1`
- Skill mirror check: `scripts/compare-skill-mirrors.ps1`

## Forbidden Defaults

- `.git/**`
- `.mi-lsp/**`
- `.docs/wiki/_mi-lsp/read-model.toml`
- `.env`, `.env.*`
- `dist/**`
- binaries and secret material
