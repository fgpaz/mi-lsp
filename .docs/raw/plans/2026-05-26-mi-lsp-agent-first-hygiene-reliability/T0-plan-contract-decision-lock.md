---
linear_parent: not_applicable
linear_child: not_applicable
anchors: {rs: [], fl: ["FL-BOOT-01"], rf: ["RF-WKS-004"], ct: ["CT-CLI-DAEMON-ADMIN"]}
allowed_paths: [".docs/raw/plans/**", ".docs/auditoria/2026-05-26-mi-lsp-agent-first-hygiene/**"]
forbidden_paths: [".docs/wiki/_mi-lsp/read-model.toml", ".env*", "secrets/**"]
verify: ["mi-lsp nav governance --workspace mi-lsp --format toon"]
stop_if: ["governance_blocked=true"]
secret_scan: {required: true, expected: "no secrets"}
---
# Task T0: Plan Contract Decision Lock

## Shared Context
**Goal:** Persist the implementation plan and AE session contract.
**Stack:** Markdown, YAML, git.
**Architecture:** This task creates planning evidence only; it does not implement runtime behavior.

## Locked Decisions
- Selected AE mode is `orquestado_deterministico`.
- Work must happen on branch `codex/mi-lsp-agent-first-hygiene-reliability`.

## Task Metadata
```yaml
id: T0
depends_on: []
agent_type: codex
goal_id: G3
github_issues: []
expected_outcome: "Plan and session contract exist before code changes."
files:
  - create: .docs/raw/plans/2026-05-26-mi-lsp-agent-first-hygiene-reliability.md
  - create: .docs/auditoria/2026-05-26-mi-lsp-agent-first-hygiene/session-contract.yaml
complexity: low
done_when:
  - "Test-Path .docs/raw/plans/2026-05-26-mi-lsp-agent-first-hygiene-reliability.md returns True"
evidence_expected:
  - "git commit containing plan artifacts"
stop_if:
  - "governance_blocked=true"
```

## Reference
`.docs/wiki/ae/AE-HARNESS-MANIFEST.md` - follow required session contract fields.

## Prompt
Create the main plan, task folder, task subdocuments, and session contract exactly under the paths listed above. Do not edit code in this task.

## Execution Procedure
1. Verify governance with `mi-lsp nav governance --workspace mi-lsp --format toon`.
2. Create the plan files.
3. Stage only plan/session-contract files.
4. Commit with `docs(plan): add agent-first hygiene reliability plan`.

## Skeleton
```yaml
ae_contract:
  selected_mode: orquestado_deterministico
```

## Verify
`git show --stat --oneline -1` -> includes plan artifacts

## Commit
`docs(plan): add agent-first hygiene reliability plan`
