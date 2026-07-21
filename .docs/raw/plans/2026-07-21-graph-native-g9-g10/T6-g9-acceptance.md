---
linear_parent: 2026-07-21-graph-native-g9-g10
linear_child: T6
anchors: [RF-GPH-011, TP-GPH]
allowed_paths: [.docs/auditoria/2026-07-21-milsp-graph-native-g9-g10/review-index.yaml, .docs/auditoria/2026-07-21-milsp-graph-native-g9-g10/g9-verdict.md]
forbidden_paths: [scripts/**, benchmarks/**, comparator source trees]
verify: [git diff --check]
stop_if: [sample inventory incomplete, threshold failure hidden]
secret_scan: required_no_values
---
# Task T6: Independent G9 acceptance
## Shared Context
Read-only verification of every measured variant and then minimal verdict artifacts.
## Locked Decisions
- A failed target is BLOCKED, never softened.
- No global superiority claim from partial slices.
## Task Metadata
```yaml
id: T6
depends_on: [T5]
agent_type: verifier-adversarial
goal_id: G9
github_issues: []
expected_outcome: Independent scoped conclusion for Graphify and baseline comparisons.
files: [{create: .docs/auditoria/2026-07-21-milsp-graph-native-g9-g10/g9-verdict.md}]
complexity: high
done_when: [verdict is PASS or exact irreducible BLOCKED]
evidence_expected: [review-index, signed-off metric inventory]
stop_if: [raw and aggregate evidence disagree]
```
## Reference
RF-GPH-011 thresholds and baseline hot-path guard.
## Prompt
Verify exactly 30, pins, hashes, all metrics, determinism, negative violations, baseline guard, no-MCP/no-write and truthful NOT_COMPARABLE scopes. Write only the scoped verdict artifacts.
## Execution Procedure
1. Inspect raw inventory and sanitized report.
2. Recompute sample counts and selected aggregates.
3. Verify claims against thresholds.
4. Write verdict and review index.
## Skeleton
```toon
block_id: g9-verdict
verdict: PASS|BLOCKED
```
## Verify
`git diff --check` -> PASS
## Commit
`docs(audit): record G9 competitive verdict`
