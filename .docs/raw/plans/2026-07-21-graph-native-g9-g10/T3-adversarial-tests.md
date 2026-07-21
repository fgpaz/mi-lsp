---
linear_parent: 2026-07-21-graph-native-g9-g10
linear_child: T3
anchors: [RF-GPH-011, TP-GPH]
allowed_paths: [scripts/bench/victory_lab/test_adversarial_v2.py, scripts/bench/victory_lab/validate_manifest.py]
forbidden_paths: [benchmarks/victory-lab/v2/corpus/**, comparator source trees]
verify: [python -B -m unittest scripts.bench.victory_lab.test_adversarial_v2]
stop_if: [a false PASS cannot be reproduced]
secret_scan: required_no_values
---
# Task T3: Anti-gaming tests
## Shared Context
The benchmark must reject incomplete, mutable, replayed or selectively reported data.
## Locked Decisions
- No best-of, no zero substitution, no hidden NOT_COMPARABLE.
- Exactly 30 raw repetitions per measured variant.
## Task Metadata
```yaml
id: T3
depends_on: [T2]
agent_type: claudex-writer
goal_id: G9
github_issues: []
expected_outcome: Every known benchmark-gaming path fails closed.
files: [{create: scripts/bench/victory_lab/test_adversarial_v2.py}, {create: scripts/bench/victory_lab/validate_manifest.py}]
complexity: medium
done_when: [all adversarial tests pass]
evidence_expected: [atomic commit SHA, rejected-case inventory]
stop_if: [production fixture must be mutated to test a case]
```
## Reference
Use temporary copies only; never mutate pinned comparators or canonical v2 fixture.
## Prompt
Test pin/hash mismatch, <30, missing comparator, timeout/crash, schema drift, duplicate/replayed samples, best-of selection, unavailable metric as PASS, nondeterminism and raw-log leakage.
## Execution Procedure
1. Create temp-only adversarial fixtures.
2. Add one fail-closed test per condition.
3. Run focused tests.
4. Run full Victory suite.
5. Commit atomically.
## Skeleton
```python
def test_rejects_fewer_than_thirty_samples(self): ...
```
## Verify
`python -B -m unittest scripts.bench.victory_lab.test_adversarial_v2` -> PASS
## Commit
`test(bench): reject victory lab gaming and incomplete evidence`
