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

Every mutating or non-trivial task must create/update `.docs/auditoria/<YYYY-MM-DD>-<task-slug>/session-contract.yaml` with an `ae_contract` block before edits. The contract must name selected mode, decision lock, adapter, orchestration depth, allowed paths, forbidden paths, required evidence, stop conditions, and cleanup policy.

Adapter selection is manifest-first: discover global `ae-adapter-*` skills, read `adapter_manifest.schema=ae-harness-adapter/v1`, prefer an explicit user-requested harness, then current/project harness fit, and fall back to `simulated_packets` with `missing_ae_adapter_manifest` when no adapter satisfies evidence and isolation.

Governed AE work must also record `mi_lsp_preflight` in the session contract before worker launch, historical audit, closure, push, or PR-ready claims. Set `MI_LSP_CLIENT_NAME` and `MI_LSP_SESSION_ID` before every `mi-lsp` command; `client_name=manual-cli`, a default `session_id` like `cli-<pid>`, `governance_blocked=true`, `docs_ready=false`, `doc_count=0`, or `ae_canon.status` in `missing|mismatch|projection_only` is a hard blocker, not a warning.

Subagents or worker lanes are mandatory for future non-trivial work. Zero-subagent execution is non-compliant unless the session contract records a trivial/read-only waiver. Default recursion depth is `v0_shadow`: two active levels, third-level task orchestrators are shadow-only until a later supervised pilot.

WSL/subagent/worker execution audits must be read-only first and must produce a worker/session attribution matrix, admin export summary, manual-cli exception review, and WSL evidence-handling note. Do not mutate WSL filesystems, rewrite histories/logs, dump raw shell history, or treat telemetry/transcripts as canon.

Before any push or PR-ready claim, run `scripts/ae/pre-push-guard.ps1` with the active session contract, then close with `ps-trazabilidad` and `ps-auditar-trazabilidad`. If any diff, branch, evidence, scope, or tracker state changes after audit, rerun both closure gates.

## Subagent Orchestration Protocol

- Every non-trivial task must launch subagents or worker lanes after `ae-programa` locks the session contract.
- First wave is read-only exploration; implementation writes go to specialized implementation or worker lanes.
- Minimum lanes: 1 for trivial/read-only checks, 3 for medium work, 5 for complex or cross-layer work.
- Delegated tasks must be atomic, path-bounded, and evidence-bounded; subagents return summaries with file/line or command evidence, not raw dumps.
- The orchestrator must verify cited paths/results before integrating. Contradictory subagent results trigger another bounded verification lane.
- Zero-subagent execution requires an explicit trivial/read-only waiver in the session contract.

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
