# Harness-first final campaign

The only authorized campaign runner is `scripts/bench/harness_first/runner.py`.
It is not a comparator and does not invoke Victory Lab.

## Dry-run gate

From the repository root, validate the schema without starting a candidate:

```text
python -m scripts.bench.harness_first.runner \
  --manifest docs/benchmarks/HARNESS_FIRST_CAMPAIGN.json \
  --output .artifacts/harness-first-final \
  --dry-run
```

Dry-run validates the seven preview sections, command shapes, direct/daemon
labels, and the single worker-status invocation. It does not start a candidate,
create a marker, read `.env`, or read secrets. Route probes are reported in the
dry-run count but are only used after a real query has run.

## Authorized campaign shape

A real run requires `--run`, one candidate `--binary`, a clean source worktree,
and a new output directory. Each manifest mode is executed once; there is no
retry path and `attempts` must remain one. The output directory is bounded to
`report.json`, `report.yaml`, and its marker. A machine-local registry outside
the source worktree also claims `(campaign_id, source_revision, binary_sha256)`
so a second output directory cannot rerun the same candidate.

The manifest intentionally labels `wiki-pack`, `explain-change`, and
`workspace-map` as direct-only functional probes because the CLI forces those
operations direct. `related` is the daemon-capable semantic probe and is run in
both modes. Its direct mode must be observed as `route=direct`; its daemon mode
must be observed as `route=daemon`, with a non-empty backend and never
`direct_fallback`. Route observation reads only allowlisted telemetry fields in
memory; it never persists telemetry stdout, stderr, or payload data.

Correctness is kind-specific rather than `ok=true` alone. Wiki packs require
usable docs/evidence; explain-change requires the real `items[].preview[]` shape
with all seven sections, and every empty section must have an explicit omission
or fallback. Every expansion is validated as one object containing both
`command` and `reason`, with a command beginning `mi-lsp nav `. Workspace-map
and related require their real `graph_freshness.state` and `graph_ranks` fields.
Expected digests are optional and are not populated with unstable goldens.

The report measures stdout byte count and a deterministic token estimate
`ceil(UTF-8 output bytes / 4)` for every query, including per-preview token
counts, byte counts, and a bounded utility ratio based on section and expansion
coverage. It stores only allowlisted measurements, statuses, digests, and
sanitized diagnostics; native stdout/stderr and raw payloads are never written.

RSS is a mandatory fail-closed gate for all query processes and the worker-status
process. The sampler uses pinned `psutil` from
`scripts/bench/harness_first/requirements.txt`; missing or unusable sampling is
`NOT_RUN` and fails the campaign. The benchmark campaign is not run in CI.

Provenance is also fail-closed before candidate spawn. The source revision must
be a 40- or 64-hex Git revision and the source tree must be clean. The binary
SHA-256 is recorded, `go version -m` is parsed, `vcs.revision` must equal the
source revision, and `vcs.modified` must be exactly `false`. `NOT_RUN`, `FAIL`,
parse failure, or mismatch blocks execution.
