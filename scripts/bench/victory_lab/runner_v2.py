"""Authoritative Victory Lab v2 runner with serialized, complete sample retention."""
from __future__ import annotations

import argparse
import hashlib
import json
import sys
from pathlib import Path
from typing import Any, Mapping

try:
    from .adapters.v2 import VictoryAdapter
    from .manifest_v2 import ManifestError, canonical_file_hash, load_manifest, manifest_adapters, sha256_bytes, sha256_file
    from .schema_v2 import RunRecord
    from .validate_manifest import validate_strict_manifest
except ImportError:  # pragma: no cover
    from adapters.v2 import VictoryAdapter
    from manifest_v2 import ManifestError, canonical_file_hash, load_manifest, manifest_adapters, sha256_bytes, sha256_file
    from schema_v2 import RunRecord
    from validate_manifest import validate_strict_manifest


def _digest_group(manifest: Mapping[str, Any], key: str) -> str:
    value = json.dumps(manifest.get(key, {}), sort_keys=True, separators=(",", ":")).encode()
    return sha256_bytes(value)


def _tree_digest(path: Path) -> str:
    digest = hashlib.sha256()
    for item in sorted((candidate for candidate in path.rglob("*") if candidate.is_file()), key=lambda candidate: candidate.relative_to(path).as_posix()):
        digest.update(item.relative_to(path).as_posix().encode("utf-8"))
        digest.update(sha256_file(item).encode("ascii"))
    return digest.hexdigest()


def _content_snapshot(manifest_path: Path, manifest: Mapping[str, Any]) -> dict[str, str]:
    root = manifest_path.parent
    snapshot = {
        "__manifest__": sha256_file(manifest_path),
        "__corpus_tree__": _tree_digest(root / "corpus"),
        "__goldens_tree__": _tree_digest(root / "goldens"),
    }
    for group in ("fixture_hashes", "oracle_hashes"):
        for relative in manifest[group]:
            path = root / relative
            if path.is_symlink() or not path.is_file():
                raise ManifestError(f"protected {group} file is missing or linked: {relative}")
            snapshot[f"{group}:{relative}"] = canonical_file_hash(path)
    return snapshot


def _assert_content_unchanged(before: Mapping[str, str], manifest_path: Path, manifest: Mapping[str, Any]) -> None:
    after = _content_snapshot(manifest_path, manifest)
    if dict(before) != after:
        raise ManifestError("fixture, golden, or manifest content changed during run")


def _effective_status(samples: list[dict[str, Any]]) -> str:
    statuses = {str(sample.get("status")) for sample in samples}
    child_statuses = {
        str(sample.get("metrics", {}).get("child", {}).get("status"))
        for sample in samples
        if isinstance(sample.get("metrics"), Mapping) and isinstance(sample["metrics"].get("child"), Mapping)
    }
    if "BLOCKED" in statuses:
        return "BLOCKED"
    if "FAIL" in statuses:
        return "FAIL"
    if "NOT_COMPARABLE" in statuses or "NOT_COMPARABLE" in child_statuses:
        return "NOT_COMPARABLE"
    if statuses == {"PASS"}:
        return "PASS"
    return "NOT_RUN"


def run_manifest(manifest_path: str | Path, output: str | Path, *, repetitions: int = 30, mode: str = "authoritative", executor: Any = None) -> dict[str, Any]:
    path = Path(manifest_path).resolve()
    manifest = load_manifest(path)
    validate_strict_manifest(manifest, path.parent, check_files=True)
    if repetitions != 30:
        raise ManifestError("Victory Lab v2 evidence requires exactly 30 repetitions")
    if mode not in {"authoritative", "exploratory"}:
        raise ManifestError(f"unsupported run mode: {mode}")
    manifest = dict(manifest)
    manifest["fixture_digest"] = _digest_group(manifest, "fixture_hashes")
    manifest["oracle_digest"] = _digest_group(manifest, "oracle_hashes")
    content_before = _content_snapshot(path, manifest)
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
                sample = record.to_dict()
                if sample.get("adapter_id") != adapter_id or sample.get("operation") != case["operation"]:
                    raise ManifestError("adapter returned a sample for the wrong comparator or operation")
                sample["case_id"] = case["id"]
                sample["fixture_digest"] = manifest["fixture_digest"]
                sample["oracle_digest"] = manifest["oracle_digest"]
                RunRecord.from_dict(sample)
                samples.append(sample)
    _assert_content_unchanged(content_before, path, manifest)
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
        "status": _effective_status(samples),
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
