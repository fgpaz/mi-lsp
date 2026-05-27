# PATHS

This file is a projection of `.docs/wiki/ae/AE-PROJECTION-POLICY.md`.
The wiki remains the source of truth; this file gives agents a compact path map.

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
