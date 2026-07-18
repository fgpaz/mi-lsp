# Victory Lab v1

Victory Lab is a deterministic, dependency-free graph benchmark foundation.
The manifest pins `graphify_revision: victory-graphify-v1`, SHA-256 hashes, nine
C#/Go/TypeScript/Python/wiki/mixed/cross-workspace/extension/relationship cases,
and a default of 30 repetitions.

## Run

    PYTHONDONTWRITEBYTECODE=1 python -m unittest discover -s scripts/bench/victory_lab -p "test_*.py"
    PYTHONDONTWRITEBYTECODE=1 python scripts/bench/victory_lab/runner.py --manifest benchmarks/victory-lab/v1/manifest.json --smoke --output .tmp/victory-smoke
    PYTHONDONTWRITEBYTECODE=1 python scripts/bench/victory_lab/report.py --input .tmp/victory-smoke --output .tmp/victory-report.json

`--smoke` performs one repetition; the default is 30 and `--repetitions` is
configurable. JSONL retains every case/repetition. Reports include symbol and
relation precision/recall/F1, p50/p95, MAD, deterministic bootstrap 95% CIs,
and RSS/disk/output/token units.

Goldens are hand-authored fixtures. Relations explicitly distinguish positive,
negative, ambiguous, unresolved, and not-comparable expectations. Quality can
therefore be measured as non-perfect without treating every execution as a
failure or deriving truth from the same extraction under test.

The incremental section measures a full initial state, deterministic
create/change/delete/rename mutations, a mutated candidate, and a separate
clean full rebuild. Its stale rate is the measured graph-record difference,
not a constant. The report separates the measured Graphify result from
mi-lsp (not run by this dependency-free lab) and unsupported/not-comparable
claims.

Canonical JSON normalizes line endings and ordering and removes volatile process
fields. OS/architecture and unavailable RSS are explicit. Manifest hashes and
Graphify revision are verified before execution; every case is emitted, with no
best-of selection, network, installation, or model judge.
