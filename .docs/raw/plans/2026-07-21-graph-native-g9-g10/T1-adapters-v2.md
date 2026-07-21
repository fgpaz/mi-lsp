---
linear_parent: 2026-07-21-graph-native-g9-g10
linear_child: T1
anchors: [RF-GPH-011, CT-GRAPH-CLI]
allowed_paths: [scripts/bench/victory_lab/adapters/**, scripts/bench/victory_lab/schema_v2.py, scripts/bench/victory_lab/manifest_v2.py, scripts/bench/victory_lab/runner_v2.py, scripts/bench/victory_lab/report_v2.py, scripts/bench/victory_lab/canonical_v2.py, benchmarks/victory-lab/v2/manifest.json]
forbidden_paths: [scripts/bench/victory_lab/runner.py, scripts/bench/victory_lab/report.py, comparator source trees]
verify: [python -B -m unittest discover -s scripts/bench/victory_lab -p test_*.py]
stop_if: [pin mismatch, adapter requires comparator mutation]
secret_scan: required_no_values
---
# Task T1: Real adapters and provenance v2
## Shared Context
Run current, baseline and Graphify as child processes through a versioned fail-closed schema.
## Locked Decisions
- Pins: baseline `a251ab1...111`, Graphify `9bf14a4...d1` v0.9.19.
- Baseline graph-only is `NOT_COMPARABLE`; hot-path affected remains comparable.
## Task Metadata
```yaml
id: T1
depends_on: [T0]
agent_type: claudex-writer
goal_id: G9
github_issues: []
expected_outcome: Real adapters produce normalized RunRecord v2 with exact provenance.
files: [{create: scripts/bench/victory_lab/adapters/**}, {create: scripts/bench/victory_lab/runner_v2.py}, {create: benchmarks/victory-lab/v2/manifest.json}]
complexity: high
done_when: [adapter/provenance tests pass]
evidence_expected: [atomic commit SHA, command matrix]
stop_if: [native outputs cannot be normalized without dropping required evidence]
```
## Reference
`CT-GRAPH-CLI` for envelope semantics; keep v1 files unchanged.
## Prompt
Implement AdapterSpec, RunRecord, pin/digest validation, exact argv/cwd/env allowlist, current/baseline/Graphify adapters, canonical payload normalization, NOT_COMPARABLE reasons, exactly-30 enforcement and all-sample aggregation.
## Execution Procedure
1. Read T0 fixture and command help from pinned executables.
2. Create versioned schema and manifest validator.
3. Implement adapters and v2 runner/report.
4. Add focused tests under `scripts/bench/victory_lab/test_*_v2.py` only if path ownership remains exclusive.
5. Commit atomically.
## Skeleton
```python
@dataclass(frozen=True)
class RunRecord:
    system_id: str
    task_id: str
    status: str
    elapsed_ms: float | None
    canonical_digest: str | None
```
## Verify
`python -B -m unittest discover -s scripts/bench/victory_lab -p "test_*.py"` -> PASS
## Commit
`feat(bench): add pinned competitive adapters v2`
