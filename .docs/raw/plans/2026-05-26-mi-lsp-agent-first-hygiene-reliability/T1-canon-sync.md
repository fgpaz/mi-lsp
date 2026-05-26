---
linear_parent: not_applicable
linear_child: not_applicable
anchors: {rs: [], fl: ["FL-BOOT-01", "FL-QRY-01"], rf: ["RF-WKS-004", "RF-WKS-005", "RF-QRY-002"], ct: ["CT-CLI-DAEMON-ADMIN", "CT-NAV-GOVERNANCE"]}
allowed_paths: [".docs/wiki/**"]
forbidden_paths: [".docs/wiki/_mi-lsp/read-model.toml"]
verify: ["mi-lsp nav wiki validate-source --workspace mi-lsp --format toon"]
stop_if: ["governance_blocked=true"]
secret_scan: {required: true, expected: "no secrets"}
---
# Task T1: Canon Sync

## Shared Context
**Goal:** Keep SDD/AE canon aligned with the new CLI behavior.
**Stack:** Markdown wiki with SDD-HARNESS-v1 and SDD-WIKI-SOURCE-v1.
**Architecture:** Public CLI changes require RF, TP, CT, and technical baseline sync.

## Locked Decisions
- New public command is `workspace hygiene`.
- `--apply-safe` may only invoke existing registry stale prune behavior.

## Task Metadata
```yaml
id: T1
depends_on: ["T0"]
agent_type: codex
goal_id: G1
github_issues: []
expected_outcome: "Wiki canon names the hygiene command, safe-apply behavior, stale diagnostics, and telemetry recommendations."
files:
  - modify: .docs/wiki/04_RF/RF-WKS-004.md
  - modify: .docs/wiki/06_pruebas/TP-WKS.md
  - modify: .docs/wiki/09_contratos_tecnicos.md
  - modify: .docs/wiki/07_baseline_tecnica.md
complexity: medium
done_when:
  - "rg -n \"workspace hygiene|registry-hygiene|apply-safe\" .docs/wiki returns matches"
evidence_expected:
  - "wiki validation command output"
stop_if:
  - "target wiki files are missing"
```

## Reference
`.docs/wiki/04_RF/RF-WKS-004.md` section 5 already owns workspace doctor/prune behavior.

## Prompt
Update the existing canon in place. Do not introduce a competing governance layer. Add concise entries for `workspace hygiene`, safe apply, stale index diagnostics, and telemetry recommendations.

## Execution Procedure
1. Open the four target docs.
2. Add the new command to the same sections that describe `workspace doctor` and `workspace prune`.
3. Add TP rows for dry-run and apply-safe behavior.
4. Add technical baseline bullets for agent-first hygiene and telemetry recommendations.

## Skeleton
```markdown
- `workspace hygiene [--apply-safe]`: agent-first hygiene summary over doctor/prune.
```

## Verify
`mi-lsp nav wiki validate-source --workspace mi-lsp --format toon` -> no new BLOCKED result caused by this change

## Commit
`docs: sync workspace hygiene canon`
