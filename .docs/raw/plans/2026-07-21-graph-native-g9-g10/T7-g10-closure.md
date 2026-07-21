---
linear_parent: 2026-07-21-graph-native-g9-g10
linear_child: T7
anchors: [RF-GPH-001..011, TP-GPH, TECH-GRAPH-NATIVE, DB-SYMBOL-EDGE-GRAPH, CT-GRAPH-CLI, CT-MILX-V1, AE-RELEASE-DISTRIBUTION]
allowed_paths: [docs/benchmarks/VICTORY_LAB.md, .docs/wiki/ae/AE-RELEASE-DISTRIBUTION.md, .docs/auditoria/2026-07-21-milsp-graph-native-g9-g10/**]
forbidden_paths: [.docs/wiki/00_gobierno_documental.md, .docs/wiki/_mi-lsp/read-model.toml, .mi-lsp/**]
verify: [go test ./..., go vet ./..., gofmt -l cmd internal, git diff --check]
stop_if: [G9 not PASS, missing release provenance, forbidden external mutation]
secret_scan: required_no_values
---
# Task T7: G10 documentation and local closure
## Shared Context
Reconcile reviewer-proven gaps only, then prepare local-only closure evidence.
## Locked Decisions
- No global install, push, PR, merge, deploy or publish.
- `go test -race` is NOT_RUN_HOST_UNSUPPORTED on Windows ARM64.
## Task Metadata
```yaml
id: T7
depends_on: [T6]
agent_type: ps-docs
goal_id: G10
github_issues: []
expected_outcome: Complete traceability, security, hygiene and local release disposition.
files: [{modify: docs/benchmarks/VICTORY_LAB.md}, {create: .docs/auditoria/2026-07-21-milsp-graph-native-g9-g10/closure-packet.yaml}]
complexity: high
done_when: [audit manifest hashes final and closure packet complete]
evidence_expected: [closure packet, traceability closure, release disposition]
stop_if: [canon requires an unapproved semantic change]
```
## Reference
AE-RELEASE-DISTRIBUTION graph-native gate; active audit manifest TTL 14/SHA-256.
## Prompt
Update benchmark docs with scoped results and NOT_COMPARABLE slices. Record binary/source provenance, install paths if any, worker status, rollback note, exact test results and explicit no push/deploy/publish. Finalize audit hashes and cleanup status.
## Execution Procedure
1. Consume six G10 read-only audits.
2. Edit only proven gaps.
3. Run full fixed command set.
4. Finalize SHA-256 manifest and closure packet.
5. Hand off to ps-trazabilidad, ps-auditar-trazabilidad and ae-close.
## Skeleton
```yaml
release_disposition: local_only
push: not_performed
deploy: not_performed
publish: not_performed
```
## Verify
`go test ./... && go vet ./... && git diff --check` -> PASS
## Commit
`docs(audit): close graph-native G10 locally`
