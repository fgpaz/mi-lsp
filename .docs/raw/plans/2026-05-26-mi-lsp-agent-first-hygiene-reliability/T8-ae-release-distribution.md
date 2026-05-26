---
linear_parent: not_applicable
linear_child: not_applicable
anchors: {rs: [], fl: ["FL-DAE-01"], rf: ["RF-DAE-002"], ct: ["CT-CLI-DAEMON-ADMIN"]}
allowed_paths: [".docs/auditoria/2026-05-26-mi-lsp-agent-first-hygiene/**"]
forbidden_paths: [".docs/wiki/_mi-lsp/read-model.toml"]
verify: ["pwsh ./scripts/release/ae-release-binaries.ps1 -SkipBuild -SkipLocalInstall -SkipWslInstall -SkipMirror"]
stop_if: ["release gate skipped without waiver"]
secret_scan: {required: true, expected: "no secrets"}
---
# Task T8: AE Release Distribution

## Shared Context
**Goal:** Close binary-affecting CLI behavior through AE release policy.
**Stack:** PowerShell release script and AE evidence policy.
**Architecture:** This branch changes CLI behavior; dry release gate or waiver is mandatory.

## Locked Decisions
- No publish unless explicitly requested.
- Dry gate is enough for implementation branch closure.

## Task Metadata
```yaml
id: T8
depends_on: ["T7"]
agent_type: codex
goal_id: G3
github_issues: []
expected_outcome: "AE release distribution evidence or waiver exists."
files:
  - create: .docs/auditoria/2026-05-26-mi-lsp-agent-first-hygiene/ae-release-evidence.md
complexity: low
done_when:
  - "AE release dry gate result or waiver is recorded"
evidence_expected:
  - ".docs/auditoria/2026-05-26-mi-lsp-agent-first-hygiene/ae-release-evidence.md"
stop_if:
  - "script missing and no waiver recorded"
```

## Reference
`.docs/wiki/ae/AE-RELEASE-DISTRIBUTION.md`.

## Prompt
Run the dry release gate. If environment blocks it, write an explicit waiver with the command attempted, error, and reason no publish/install occurred.

## Execution Procedure
1. Run the dry gate command.
2. Record result in AE evidence.
3. Do not publish tags or install binaries.

## Skeleton
```markdown
## AE Release Evidence
- command: pwsh ./scripts/release/ae-release-binaries.ps1 ...
- result: PASS|WAIVED
```

## Verify
`pwsh ./scripts/release/ae-release-binaries.ps1 -SkipBuild -SkipLocalInstall -SkipWslInstall -SkipMirror` -> PASS or recorded waiver

## Commit
`chore(ae): record release distribution evidence`
