# Final integration drift closure - traceability and audit

Date: 2026-05-19

## Scope

- Close the drift found during final `$ps-trazabilidad` / `$ps-auditar-trazabilidad` after PR #15 integration.
- Preserve the already-merged telemetry hardening on `origin/main`.
- Repair technical-doc traceability so `nav wiki trace` can resolve exact `TECH-*`, `DB-*`, and `CT-*` doc IDs from frontmatter `implements` / `tests`.
- Promote the useful loose `mi-lsp` skill reference `references/landing-presentation.md` from `.agents` into `C:/repos/buho/assets` and `.codex`.

## Anchors

- `RF-QRY-016` owns `nav wiki trace` as part of the public wiki navigation surface.
- `CT-NAV-WIKI` owns the command contract for `nav wiki trace`.
- `CT-CLI-DAEMON-ADMIN`, `DB-STATE-Y-TELEMETRIA`, and `TECH-DAEMON-GOBERNANZA` are the regression targets for technical-doc trace evidence.

## Changes

- `internal/docgraph/docgraph.go` now indexes frontmatter `implements` and `tests` for all governed docs, not only RF docs.
- `internal/service/trace.go` now resolves unknown/non-RF doc IDs by exact `doc_id` before falling back to RF heuristics.
- Regression tests cover technical-doc frontmatter extraction and exact CT doc resolution.
- `RF-QRY-016` and `CT-NAV-WIKI` were synchronized with the implementation behavior.
- `C:/repos/buho/assets` includes the promoted `skills/mi-lsp/references/landing-presentation.md` and is pushed to `origin/main` at `d1d6407db21c13c0377e4ed7ff526cd3e2e56fce`.

## Verification

- `mi-lsp workspace status mi-lsp --format toon`: PASS, governance ready, index ready.
- `mi-lsp nav governance --workspace mi-lsp --format toon`: PASS, `governance_blocked=false`, projection in sync.
- `go test ./internal/docgraph ./internal/service -run Trace`: PASS.
- `go test ./...`: PASS.
- `dotnet test worker-dotnet/MiLsp.Worker.sln`: PASS.
- `mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon`: PASS.
- `mi-lsp nav wiki validate-source --workspace mi-lsp --format toon`: PASS.
- `go run ./cmd/mi-lsp nav wiki trace CT-CLI-DAEMON-ADMIN --workspace mi-lsp --format toon`: PASS, `status=implemented`, `coverage=1`.
- `go run ./cmd/mi-lsp nav wiki trace DB-STATE-Y-TELEMETRIA --workspace mi-lsp --format toon`: PASS, `status=implemented`, `coverage=1`.
- `go run ./cmd/mi-lsp nav wiki trace TECH-DAEMON-GOBERNANZA --workspace mi-lsp --format toon`: PASS, `status=implemented`, `coverage=1`.

## Drift inventory

- `mi-lsp` deprecated worktree drift: closed; only the main repo worktree remains before this PR branch.
- `mi-lsp` branch drift: no stale same-scope local or remote branches remained before this PR branch.
- `buho/assets` skill drift: closed; assets, `.agents`, and `.codex` trees match after the landing reference promotion.
- Original stashes remain classified as `superseded-retained` because dropping stashes is destructive and not required for branch/worktree cleanup.

## Verdict

Approved for PR integration after final branch gates pass. After merge, rebuild and redistribute the installed binary and skill binaries from the merge commit, then delete the feature branch and verify `main...origin/main = 0/0`.
