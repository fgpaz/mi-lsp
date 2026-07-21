"""Authoritative Victory Lab v2 runner with serialized, complete sample retention."""
from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path
from typing import Any, Mapping

try:
    from .adapters.v2 import VictoryAdapter
    from .manifest_v2 import ManifestError, load_manifest, manifest_adapters, sha256_bytes
except ImportError:  # pragma: no cover
    from adapters.v2 import VictoryAdapter
    from manifest_v2 import ManifestError, load_manifest, manifest_adapters, sha256_bytes


def _digest_group(manifest: Mapping[str, Any], key: str) -> str:
    value = json.dumps(manifest.get(key, {}), sort_keys=True, separators=(",", ":")).encode()
    return sha256_bytes(value)


def run_manifest(manifest_path: str | Path, output: str | Path, *, repetitions: int = 30, mode: str = "authoritative", executor: Any = None) -> dict[str, Any]:
    path = Path(manifest_path).resolve()
    manifest = load_manifest(path)
    if mode == "authoritative" and repetitions != 30:
        raise ManifestError("authoritative Victory Lab runs require exactly 30 repetitions")
    if repetitions < 1 or repetitions > 30:
        raise ManifestError("repetitions must be between 1 and 30")
    manifest = dict(manifest)
    manifest["fixture_digest"] = _digest_group(manifest, "fixture_hashes")
    manifest["oracle_digest"] = _digest_group(manifest, "oracle_hashes")
    output_path = Path(output).resolve()
    output_path.mkdir(parents=True, exist_ok=True)
    adapters = manifest_adapters(manifest)
    samples: list[dict[str, Any]] = []
    # Deliberately serial: comparator processes and fixture materialization must not overlap.
    for adapter_id in sorted(adapters):
        adapter = VictoryAdapter(adapters[adapter_id], manifest, path.parent, executor=executor)
        for case in manifest["cases"]:
            for repetition in range(repetitions):
                record = adapter.run_case(case, repetition=repetition)
                samples.append(record.to_dict())
    expected_samples = len(adapters) * len(manifest["cases"]) * repetitions
    if len(samples) != expected_samples:
        raise ManifestError(f"authoritative runner retained {len(samples)} samples, expected {expected_samples}")
    samples_path = output_path / "samples.jsonl"
    with samples_path.open("w", encoding="utf-8", newline="\n") as stream:
        for sample in samples:
            stream.write(json.dumps(sample, ensure_ascii=False, sort_keys=True, separators=(",", ":")) + "\n")
    summary = {
        "schema": "victory-run/v2",
        "manifest": str(path),
        "manifest_schema": manifest["schema"],
        "mode": mode,
        "repetitions": repetitions,
        "authoritative": mode == "authoritative",
        "adapters": sorted(adapters),
        "cases": [case["id"] for case in manifest["cases"]],
        "samples": len(samples),
        "samples_path": str(samples_path),
        "status_counts": {status: sum(sample["status"] == status for sample in samples) for status in ("PASS", "FAIL", "BLOCKED", "NOT_COMPARABLE", "NOT_RUN")},
        "fixture_digest": manifest["fixture_digest"],
        "oracle_digest": manifest["oracle_digest"],
        "anti_gaming": {"serialized_variants": True, "all_samples_retained": True, "best_of_rejected": True},
    }
    (output_path / "run.json").write_text(json.dumps(summary, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    return summary


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--manifest", required=True)
    parser.add_argument("--output", required=True)
    parser.add_argument("--repetitions", type=int, default=30)
    parser.add_argument("--mode", choices=("authoritative", "exploratory"), default="authoritative")
    args = parser.parse_args(argv)
    try:
        summary = run_manifest(args.manifest, args.output, repetitions=args.repetitions, mode=args.mode)
    except (ManifestError, OSError, ValueError) as exc:
        print(f"BLOCKED: {exc}", file=sys.stderr)
        return 2
    print(json.dumps(summary, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
