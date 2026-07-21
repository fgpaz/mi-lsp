---
linear_parent: 2026-07-21-graph-native-g9-g10
linear_child: T0
anchors: [RF-GPH-011, TP-GPH]
allowed_paths: [benchmarks/victory-lab/v2/**]
forbidden_paths: [.mi-lsp/**, comparator source trees, .docs/wiki/**]
verify: [python -B -m unittest discover -s scripts/bench/victory_lab -p test_*.py]
stop_if: [fixture requires absolute user paths, oracle cannot be hand-authored]
secret_scan: required_no_values
---
# Task T0: Fixture and oracle v2
## Shared Context
Create a deterministic Go fixture for callers/affected/path with explicit repository identity represented as fixture data, plus positive/negative/ambiguous/unresolved goldens.
## Locked Decisions
- New `benchmarks/victory-lab/v2/**`; do not edit v1.
- Hash raw bytes with SHA-256; no absolute paths.
## Task Metadata
```yaml
id: T0
depends_on: []
agent_type: claudex-writer
goal_id: G9
github_issues: []
expected_outcome: Stable fixture and exact oracle for comparable tasks.
files: [{create: benchmarks/victory-lab/v2/corpus/**}, {create: benchmarks/victory-lab/v2/goldens/**}]
complexity: high
done_when: [all fixture/oracle hashes are deterministic]
evidence_expected: [atomic commit SHA, hash inventory]
stop_if: [required semantic expectation is ambiguous]
```
## Reference
`benchmarks/victory-lab/v1/` for layout only; `RF-GPH-011` and `TP-GPH` own semantics.
## Prompt
Create v2 corpus/goldens only. Include stable Go module, `subject.go`, callers, unrelated negative cases, one deterministic file mutation, expected callers/affected/path sets, normalization fields and repository identity metadata. Never create a repo-root `.mi-lsp` file.
## Execution Procedure
1. Read v1 layout and RF/TP oracle.
2. Create v2 fixture and goldens.
3. Add a hash inventory file under v2.
4. Run focused Python tests if present.
5. Commit atomically.
## Skeleton
```json
{"task":"affected","expected":["caller.go","subject.go"],"negative_violations":[]}
```
## Verify
`python -B -m unittest discover -s scripts/bench/victory_lab -p "test_*.py"` -> PASS
## Commit
`test(bench): add graph-native victory fixture v2`
