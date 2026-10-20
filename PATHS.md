# PATHS

This file is a compact path projection. It does not replace the universal AE kernel.

## Authority and compatibility

- Universal AE authority: `<kernel_home>/canon/`.
- Repository AE policy authority: `.docs/ae/repo-policy.yaml`.
- Product and governance authority: `.docs/wiki/00_governance.md` and the governed wiki layers below.
- Executable governance projection: `.docs/wiki/_mi-lsp/read-model.toml`.
- `.docs/wiki/ae/**` is compatibility history only. It is not current AE authority and must not supply active workflow instructions.
- `AGENTS.md`, `CLAUDE.md`, and `PATHS.md` are generated or projected policy surfaces; repair their source authority and regenerate rather than hand-editing generated policy twins.

## Kernel v2 binding

- Canon mode: `kernel_v2`.
- Required repository policy: `.docs/ae/repo-policy.yaml`.
- Required anchor: `.docs/wiki/07_technical/contracts/CT-GOVERNANCE-AE-KERNEL-V2.md`.
- Canon validation is fail-closed for missing or mismatched modules, absolute paths, parent traversal, and symlinked roots, components, or files.
- Canon reads must resolve beneath `<kernel_home>/canon/`; repository-specific slots must resolve beneath this repository's `.docs/ae/repo-policy.yaml`.

## Repository map

- Governance and body language (`language: es`): `.docs/wiki/00_governance.md`
- Scope: `.docs/wiki/01_scope.md`
- Outcomes: `.docs/wiki/02_outcomes/`
- Architecture and ADR: `.docs/wiki/03_architecture.md` (Mermaid C4 context and containers), `.docs/wiki/03_architecture/`
- Flows: `.docs/wiki/04_flows/` (sequence Mermaid in each FL's Secuencia section)
- Requirements: `.docs/wiki/05_requirements/`
- Data model: `.docs/wiki/05_requirements/data-model.md`
- Test plans: `.docs/wiki/06_tests/`; RF×TP matrix: `mi-lsp nav wiki matrix` (on demand)
- Technical baseline: `.docs/wiki/07_technical.md`, `.docs/wiki/07_technical/`
- Physical data model: `.docs/wiki/07_technical/db/DB-PHYSICAL-MODEL.md`, `.docs/wiki/07_technical/db/`
- Contracts: `.docs/wiki/07_technical/contracts/`
- UI journeys and modules: `.docs/wiki/10_ux/`; visual and scope projections: `DESIGN.md`, `PRODUCT.md`
- Terminal path registry: `.docs/wiki/00_retirements.md`; source snapshots: `.docs/history/wiki-v0/` (historical only)
- Repository AE policy: `.docs/ae/repo-policy.yaml`
- Non-canonical roadmap fallback: `.docs/roadmap/roadmap.md`; thin RS-backed items, not a wiki layer or normative index
- Durable audit evidence: `.docs/auditoria/2026-07-13-ae-kernel-v2-integration/**` for this integration scope only

## Operational evidence

- Session contract: `.docs/auditoria/<session>/session-contract.yaml`
- Audit hygiene manifest: `.docs/auditoria/<session>/audit-manifest.yaml`
- Traceability closure: `.docs/auditoria/<session>/traceability-closure.yaml`
- Raw inputs under `.docs/raw/plans/**` and `.docs/raw/prompts/**` are disposable, excluded from normal versioning, and never closure evidence.
- This task does not edit, move, promote, or delete `.docs/raw/**`.

## Verification surfaces

- Governance status: `MI_LSP_CLIENT_NAME=<client> MI_LSP_SESSION_ID=<session> go run ./cmd/mi-lsp workspace status . --format toon`
- Governance navigation: `MI_LSP_CLIENT_NAME=<client> MI_LSP_SESSION_ID=<session> go run ./cmd/mi-lsp nav governance --workspace milsp-harness-first --format toon`
- Runtime help: `go run ./cmd/mi-lsp nav --help` and the affected subcommand helps
- Intent/explain-change smoke: `go run ./cmd/mi-lsp nav intent <question> --workspace <alias> --format toon` and `go run ./cmd/mi-lsp nav explain-change --path <path> --workspace <alias> --format toon`
- Wiki validator smoke: `go run ./cmd/mi-lsp nav wiki validate-source --workspace <alias> --paths <path> --format toon` and `go run ./cmd/mi-lsp nav wiki validate-source --workspace <alias> --ids <doc-id> --format toon`
- Tests: `go test ./...`
- Formatting check: `git diff --check`
- Policy renderer: `node C:/repos/mios/ae-kernel/skills/ae-crear-politicas/scripts/render-policy.mjs --canon C:/repos/mios/ae-kernel/canon/AE-POLICY-PROJECTION.md --repo-policy .docs/ae/repo-policy.yaml --out-dir C:/wt/mi-lsp-ae-kernel-v2-final`
- Policy deprecation scan: run the kernel deprecation scanner against active projections; compatibility history is excluded from active authority.

## Forbidden paths and actions

- Never edit `.env`, `.env.*`, `.mi-lsp/**`, `node_modules/**`, `.next/**`, or secret material.
- Never edit `.docs/raw/**` in this scope.
- Never modify `C:/repos/mios/ae-kernel/` or any other repository.
- No commit, push, deploy, integration, branch deletion, worktree cleanup, or evidence cleanup is authorized in this worker scope.
