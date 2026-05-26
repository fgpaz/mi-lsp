---
linear_parent: not_applicable
linear_child: not_applicable
anchors: {rs: [], fl: ["FL-BOOT-01", "FL-QRY-01"], rf: ["RF-WKS-004", "RF-QRY-002"], ct: ["CT-CLI-DAEMON-ADMIN"]}
allowed_paths: [".docs/auditoria/2026-05-26-mi-lsp-agent-first-hygiene/**"]
forbidden_paths: [".docs/wiki/_mi-lsp/read-model.toml"]
verify: ["go test ./internal/workspace ./internal/service ./internal/daemon ./internal/docgraph ./internal/store"]
stop_if: ["focused tests fail"]
secret_scan: {required: true, expected: "no secrets"}
---
# Task T7: Integration Verification

## Shared Context
**Goal:** Verify the implementation before AE release and traceability closure.
**Stack:** Go tests, mi-lsp CLI smoke.
**Architecture:** This task verifies; it should not add new behavior unless a test exposes a direct bug.

## Locked Decisions
- Run focused package tests before broader closure.
- Capture command outputs under audit evidence when practical.

## Task Metadata
```yaml
id: T7
depends_on: ["T5", "T6"]
agent_type: codex
goal_id: G3
github_issues: []
expected_outcome: "Focused verification passes for changed subsystems."
files:
  - create: .docs/auditoria/2026-05-26-mi-lsp-agent-first-hygiene/verification-notes.md
complexity: low
done_when:
  - "go test ./internal/workspace ./internal/service ./internal/daemon ./internal/docgraph ./internal/store exits 0"
evidence_expected:
  - ".docs/auditoria/2026-05-26-mi-lsp-agent-first-hygiene/verification-notes.md"
stop_if:
  - "tests fail"
```

## Reference
Plan main verify section.

## Prompt
Run the focused test set and CLI smoke commands. Record pass/fail and exact command names in verification notes.

## Execution Procedure
1. Run focused Go tests.
2. Run `go test ./internal/cli` if CLI command wiring changed.
3. Run `mi-lsp workspace hygiene --format toon` from repo root.
4. Record results.

## Skeleton
```markdown
## Verification
- command: go test ...
- result: PASS
```

## Verify
`go test ./internal/workspace ./internal/service ./internal/daemon ./internal/docgraph ./internal/store` -> PASS

## Commit
`test: verify agent-first hygiene reliability`
