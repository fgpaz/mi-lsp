---
linear_parent: 2026-07-21-graph-native-g9-g10
linear_child: T2
anchors: [RF-GPH-011, CT-MILX-V1]
allowed_paths: [scripts/bench/victory_lab/child_metrics.py, scripts/bench/victory_lab/sanitize_v2.py, scripts/bench/victory_lab/security_gate.py, scripts/bench/victory_lab/test_child_metrics_v2.py]
forbidden_paths: [scripts/bench/victory_lab/wrappers.py, comparator source trees]
verify: [python -B -m unittest scripts.bench.victory_lab.test_child_metrics_v2]
stop_if: [Windows child RSS cannot be measured truthfully]
secret_scan: required_no_values
---
# Task T2: Child metrics and security
## Shared Context
Measure child peak RSS on Windows ARM64 and sanitize process evidence; unavailable counters remain explicit.
## Locked Decisions
- Never substitute Python harness RSS.
- Durable evidence excludes raw stdout/stderr, full env and source payloads.
## Task Metadata
```yaml
id: T2
depends_on: [T1]
agent_type: claudex-writer
goal_id: G9
github_issues: []
expected_outcome: Child metrics, timeout/crash cleanup and sanitizer are fail-closed.
files: [{create: scripts/bench/victory_lab/child_metrics.py}, {create: scripts/bench/victory_lab/security_gate.py}]
complexity: high
done_when: [child metrics/security tests pass]
evidence_expected: [atomic commit SHA, platform capability packet]
stop_if: [process tree cannot be cleaned or evidence leaks secrets]
```
## Reference
`process_metrics.py` is historical self-RSS and must not be reused as child proof.
## Prompt
Use Windows process APIs available without new runtime dependencies where practical. Track child/tree peak working set, timeout and exit; mark unsupported as NOT_COMPARABLE. Add env allowlist, bounded output digests, redaction, no-network/MCP indicators and before/after file digest checks.
## Execution Procedure
1. Implement child sampler and process-tree cleanup.
2. Implement sanitizer/security gate.
3. Add deterministic tests using short child processes.
4. Run focused and full Victory tests.
5. Commit atomically.
## Skeleton
```python
@dataclass(frozen=True)
class ChildMetrics:
    peak_rss_bytes: int | None
    status: str
    reason: str | None
```
## Verify
`python -B -m unittest scripts.bench.victory_lab.test_child_metrics_v2` -> PASS
## Commit
`feat(bench): measure and sanitize child process metrics`
