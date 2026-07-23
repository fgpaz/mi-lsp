# PATHS

This file is a compact path projection. It does not replace the universal AE kernel.

## Authority and compatibility

- Universal AE authority: `<kernel_home>/canon/`.
- Repository AE policy authority: `.docs/ae/repo-policy.yaml`.
- Product and governance authority: `.docs/wiki/00_gobierno_documental.md` and the governed wiki layers below.
- Executable governance projection: `.docs/wiki/_mi-lsp/read-model.toml`.
- `.docs/wiki/ae/**` is compatibility history only. It is not current AE authority and must not supply active workflow instructions.
- `AGENTS.md`, `CLAUDE.md`, and `PATHS.md` are generated or projected policy surfaces; repair their source authority and regenerate rather than hand-editing generated policy twins.

## Kernel v2 binding

- Canon mode: `kernel_v2`.
- Required repository policy: `.docs/ae/repo-policy.yaml`.
- Required anchor: `.docs/wiki/09_contratos/CT-GOVERNANCE-AE-KERNEL-V2.md`.
- Canon validation is fail-closed for missing or mismatched modules, absolute paths, parent traversal, and symlinked roots, components, or files.
- Canon reads must resolve beneath `<kernel_home>/canon/`; repository-specific slots must resolve beneath this repository's `.docs/ae/repo-policy.yaml`.

## Repository map

- Governance: `.docs/wiki/00_gobierno_documental.md`
- Scope: `.docs/wiki/01_alcance_funcional.md`
- Architecture: `.docs/wiki/02_arquitectura.md`
- Flows: `.docs/wiki/03_FL.md`, `.docs/wiki/03_FL/`
- Requirements: `.docs/wiki/04_RF.md`, `.docs/wiki/04_RF/`
- Data model: `.docs/wiki/05_modelo_datos.md`
- Test matrix and plans: `.docs/wiki/06_matriz_pruebas_RF.md`, `.docs/wiki/06_pruebas/`
- Technical baseline: `.docs/wiki/07_baseline_tecnica.md`, `.docs/wiki/07_tech/`
- Physical data model: `.docs/wiki/08_modelo_fisico_datos.md`, `.docs/wiki/08_db/`
- Contracts: `.docs/wiki/09_contratos_tecnicos.md`, `.docs/wiki/09_contratos/`
- Repository AE policy: `.docs/ae/repo-policy.yaml`
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
