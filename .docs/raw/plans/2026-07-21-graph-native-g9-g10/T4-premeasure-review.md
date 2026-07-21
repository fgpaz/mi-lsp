---
linear_parent: 2026-07-21-graph-native-g9-g10
linear_child: T4
anchors: [RF-GPH-011, TP-GPH, CT-GRAPH-CLI, CT-MILX-V1]
allowed_paths: []
forbidden_paths: ['**']
verify: [python -B -m unittest discover -s scripts/bench/victory_lab -p test_*.py]
stop_if: [review finds semantic mismatch or benchmark gaming]
secret_scan: required_no_values
---
# Task T4: Independent pre-measure review
## Shared Context
Read-only adversarial review after T0–T3 join and before authoritative measurement.
## Locked Decisions
- Reviewer shares no implementation authority.
- Any material finding blocks measurement.
## Task Metadata
```yaml
id: T4
depends_on: [T3]
agent_type: claudex-verifier
goal_id: G9
github_issues: []
expected_outcome: Independent PASS that harness semantics, pins, hashes and anti-gaming gates are correct.
files: [{read: scripts/bench/victory_lab/**}, {read: benchmarks/victory-lab/v2/**}]
complexity: high
done_when: [review verdict PASS]
evidence_expected: [review verdict with file/line evidence]
stop_if: [any required variant or oracle is missing]
```
## Reference
RF-GPH-011 and TP-GPH are acceptance authority.
## Prompt
Attempt to refute fairness, comparability, exact-30, determinism, RSS, sanitizer, no-write and provenance claims. Return PASS only if every issue is closed.
## Execution Procedure
1. Inspect joined diff and tests.
2. Run focused suite read-only.
3. Verify manifest pins/hashes.
4. Emit structured verdict.
## Skeleton
```yaml
verdict: PASS|BLOCKED
findings: []
```
## Verify
`python -B -m unittest discover -s scripts/bench/victory_lab -p "test_*.py"` -> PASS
## Commit
`not_applicable_read_only`
