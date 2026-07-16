# README redesign verdict

```yaml
harness_protocol: SDD-HARNESS-v1
id: README-REDESIGN-VERDICT
kind: support-doc
audience: dual
imports:
  - '[[00_gobierno_documental]]'
  - '[[AE-RELEASE-DISTRIBUTION]]'
exports:
  - README-REDESIGN-VERDICT
agent_must_read:
  - .docs/auditoria/2026-07-16-readme-redesign/verdict.md
  - .docs/auditoria/2026-07-16-readme-redesign/verification.log
agent_may_edit:
  - .docs/auditoria/2026-07-16-readme-redesign/**
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - sh scripts/tests/install-platform-mapping.sh
  - go test ./...
stop_if:
  - governance_blocked=true
  - required validation fails
evidence:
  - .docs/auditoria/2026-07-16-readme-redesign/verification.log
  - .docs/auditoria/2026-07-16-readme-redesign/traceability-closure.yaml
```

## Verdict

**APPROVED** for the README redesign and all requested follow-ups.

## Passed

- README reduced from 436 to 185 physical lines.
- Public narrative follows the approved order and audience.
- Installation presents CLI + skill first and CLI-only second.
- Demo uses `init`, `nav search --include-content`, and `nav multi-read`.
- GIF is animated, accessible through descriptive alt text, and backed by an updated SVG source.
- Windows, Linux, and macOS platform claims match the v0.5.12 release assets.
- The Darwin bundle was inspected and contains the expected `workers/osx-arm64` runtime path.
- The new shell regression test verifies Darwin archive-to-worker mapping, Linux support, and invalid-RID rejection before network access.
- Go documentation now consistently describes a native AST catalog with optional `gopls` enrichment, without claiming Roslyn parity.
- Commands, local links, assets, shell syntax, CI YAML, Go tests, Git hygiene, and governance passed.
- The task-owned Harness Contract is recognized and the changed AE release source contract passes; full-workspace validators remain blocked only by documented preexisting kernel-v2 debt outside this diff.
- No task change touched the four preexisting `internal/**` modifications.

## Corrected finding

The earlier recommendation to reject Darwin was based on stale evidence. Public Darwin assets already exist in release `v0.5.12`, and `install.sh` already maps `darwin-*` archives to `osx-*` workers. The final change preserves that support and adds deterministic regression coverage instead of introducing a false platform restriction.

## Release gate note

The release orchestration script passed its clean-tree provenance gate, then stopped because the skipped build left no `dist/win-arm64/mi-lsp.exe` artifact to inspect. No binary-producing source changed, and the session contract waives rebuild and local install refresh. Live GitHub assets and archive contents were verified independently.

## Integration state

The implementation is committed on `docs/readme-redesign` at `1c3379ada2559f3e34348536cfeacaa2b59034e6`. The user explicitly authorized guarded integration into `origin/main`; push, PR, CI, merge, and remote readback remain the active completion steps. Release publication and cleanup remain out of scope.
