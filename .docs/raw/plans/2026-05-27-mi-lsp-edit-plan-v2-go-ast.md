# mi-lsp edit-plan-v2 Go AST Backend Plan

## Summary

Add `edit-plan-v2` to the existing `mi-lsp nav edit-plan` command. V2 defines a multi-language structural-edit contract, implements the first AST backend for Go, and returns actionable `language_not_supported` errors for C#, TypeScript, and Python AST operations in this wave. `edit-plan-v1` remains compatible and textual.

## AE Decisions

- selected_mode: `orquestado_deterministico`
- branch: `codex/mi-lsp-edit-plan-v2-go-ast`
- worktree: `C:\repos\mios\mi-lsp-edit-plan-v2-go-ast`
- base_ref: `main`
- base_sha: `3bd4680f8611862d6f0e8645a7190906871e3ca1`
- disposition: `integrate-main`
- cleanup: `auto-after-successful-integration`
- release_gate: required
- external_issues: []

## Scope

- Keep `mi-lsp nav edit-plan` as the only public command.
- Accept `version: "edit-plan-v1"` with existing behavior.
- Accept `version: "edit-plan-v2"` with multi-language target metadata.
- Implement Go AST operations:
  - `replace_go_function`
  - `replace_go_function_body`
  - `insert_go_function_after`
  - `ensure_go_import`
  - `remove_go_import`
- Recognize `csharp`, `typescript`, and `python` target languages and reject AST operations with `language_not_supported`.
- Reuse v1 dry-run/apply guardrails, path safety, hash checks, temp-file writes, rollback behavior, and no-stage/no-commit policy.

## Non-Goals

- No C# Roslyn AST editing.
- No TypeScript compiler API AST editing.
- No Python `ast` or `libcst` editing.
- No semantic rename or cross-file refactor.
- No formatter side effects outside Go AST in-memory formatting.

## Verification

- `go test ./internal/model ./internal/service ./internal/cli ./internal/indexer`
- Go fixture dogfood:
  - v2 dry-run no-write
  - v2 apply writes only expected Go file
  - `go test ./...` in fixture
- Multi-language compatibility fixtures:
  - v2 C#/TS/Python AST packets return `language_not_supported`
  - v1 textual packets still dry-run/apply in C#/TS/Python fixtures
- `mi-lsp nav governance --workspace mi-lsp --format toon`
- `mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon`
- `.\scripts\release\ae-release-binaries.ps1 -SkipWslInstall -SkipMirror`
- `mi-lsp index --clean --workspace mi-lsp --format toon`
- `mi-lsp workspace hygiene --format toon`
- `ps-trazabilidad`
- `ps-auditar-trazabilidad`

## Task Packets

- `T0-plan-contract-decision-lock.md`
- `T1-canon-sync.md`
- `T2-model-contract-v2.md`
- `T3-go-ast-engine.md`
- `T4-service-dispatch-output.md`
- `T5-tests-dogfood.md`
- `T6-release-trace-audit.md`
- `T7-integrate-main-cleanup.md`
