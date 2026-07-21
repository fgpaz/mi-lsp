---
linear_parent: 2026-07-21-graph-native-g9-g10
linear_child: T5
anchors: [RF-GPH-011, TP-GPH]
allowed_paths: [.docs/auditoria/2026-07-21-milsp-graph-native-g9-g10/**]
forbidden_paths: [comparator source trees, .docs/wiki/**, .mi-lsp/**]
verify: [python scripts/bench/victory_lab/report_v2.py --verify]
stop_if: [fewer than 30 samples, pin/hash drift, competing performance process]
secret_scan: required_no_values
---
# Task T5: Serialized authoritative measurement
## Shared Context
Run current, baseline and Graphify serially with identical fixture/protocol; raw evidence stays external first.
## Locked Decisions
- Never parallelize measurement processes.
- Rebuild current/baseline from exact SHAs before measuring.
## Task Metadata
```yaml
id: T5
depends_on: [T4]
agent_type: ps-worker
goal_id: G9
github_issues: []
expected_outcome: Complete raw 30x matrix and sanitized report.
files: [{create: .docs/auditoria/2026-07-21-milsp-graph-native-g9-g10/benchmark-summary.yaml}]
complexity: high
done_when: [report verifier PASS and sample inventory exact]
evidence_expected: [raw external inventory, sanitized summary, binary/fixture hashes]
stop_if: [correctness, determinism or threshold failure]
```
## Reference
Run DAG N0–N9 from the main plan; preserve native output only outside durable audit.
## Prompt
Rebuild binaries, verify pins/digests, then run current cold/warm/incremental, baseline comparable slices and Graphify corresponding slices, exactly 30 each, serially. Generate p50/p95/max, correctness/P/R/F1, negative violations, token count, child RSS and determinism digests.
## Execution Procedure
1. Verify clean comparators and rebuild binaries.
2. Verify manifest and fixture hashes.
3. Run all variants serially.
4. Verify report and promote sanitized numeric evidence.
5. Stop on any failed threshold; do not repair in this task.
## Skeleton
```yaml
variant: current-warm-affected
sample_count: 30
status: PASS|BLOCKED|NOT_COMPARABLE
```
## Verify
`python scripts/bench/victory_lab/report_v2.py --input <raw> --output <sanitized> --verify` -> PASS
## Commit
`bench(graph): record sanitized G9 competitive evidence`
