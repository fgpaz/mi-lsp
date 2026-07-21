"""Aggregate and verify complete Victory Lab v2 evidence."""
from __future__ import annotations

import argparse
import json
import statistics
from pathlib import Path
from typing import Any, Iterable, Mapping

try:
    from .canonical_v2 import payload_digest
    from .manifest_v2 import load_manifest, sha256_bytes
    from .schema_v2 import RunRecord
    from .validate_manifest import validate_strict_manifest
except ImportError:  # pragma: no cover
    from canonical_v2 import payload_digest
    from manifest_v2 import load_manifest, sha256_bytes
    from schema_v2 import RunRecord
    from validate_manifest import validate_strict_manifest

AUTHORITATIVE_REPETITIONS = 30
_FORBIDDEN_KEYS = frozenset({"best", "best_of", "minimum", "fastest", "selected_sample", "summary"})


def percentile(values: Iterable[float], p: float) -> float | None:
    values = sorted(float(value) for value in values)
    if not values:
        return None
    rank = (len(values) - 1) * p / 100
    low, high = int(rank), min(int(rank) + 1, len(values) - 1)
    return values[low] + (values[high] - values[low]) * (rank - low)


def _stats(values: Iterable[float]) -> dict[str, Any]:
    values = [float(value) for value in values]
    return {"n": len(values), "p50": percentile(values, 50), "p95": percentile(values, 95), "max": max(values) if values else None, "mean": statistics.fmean(values) if values else None}


def latency_stats(records: list[dict[str, Any]]) -> dict[str, Any]:
    values = [float(record["elapsed_ms"]) for record in records if isinstance(record.get("elapsed_ms"), (int, float)) and not isinstance(record.get("elapsed_ms"), bool)]
    stats = _stats(values)
    return {"n": stats["n"], "p50_ms": stats["p50"], "p95_ms": stats["p95"], "max_ms": stats["max"], "mean_ms": stats["mean"]}


def _metric_stats(records: list[dict[str, Any]], key: str) -> dict[str, Any]:
    values: list[float] = []
    for record in records:
        child = record.get("metrics", {}).get("child", {})
        value = child.get(key) if isinstance(child, dict) else None
        if isinstance(value, (int, float)) and not isinstance(value, bool):
            values.append(float(value))
    stats = _stats(values)
    return {"n": stats["n"], "p50": stats["p50"], "p95": stats["p95"], "max": stats["max"]}


def _contains_forbidden_key(value: Any) -> bool:
    if isinstance(value, dict):
        return any(str(key).lower() in _FORBIDDEN_KEYS or _contains_forbidden_key(child) for key, child in value.items())
    if isinstance(value, list):
        return any(_contains_forbidden_key(child) for child in value)
    return False


def _validate_sample_metrics(record: dict[str, Any]) -> None:
    metrics = record.get("metrics")
    child = metrics.get("child") if isinstance(metrics, dict) else None
    if not isinstance(child, dict) or child.get("status") not in {"PASS", "NOT_COMPARABLE"}:
        raise ValueError("missing or invalid child metric status")
    if child["status"] == "PASS":
        value = child.get("peak_rss_bytes")
        if isinstance(value, bool) or not isinstance(value, (int, float)) or value < 0:
            raise ValueError("PASS sample is missing peak_rss_bytes")
    if record["status"] == "PASS" and not isinstance(record.get("elapsed_ms"), (int, float)):
        raise ValueError("PASS sample is missing elapsed_ms")


def child_metric_stats(records: list[dict[str, Any]]) -> dict[str, Any]:
    statuses: dict[str, int] = {}
    reasons: dict[str, int] = {}
    for record in records:
        child = record.get("metrics", {}).get("child", {})
        if not isinstance(child, dict):
            continue
        status = str(child.get("status", "NOT_COMPARABLE"))
        statuses[status] = statuses.get(status, 0) + 1
        reason = child.get("reason_code")
        if reason:
            reasons[str(reason)] = reasons.get(str(reason), 0) + 1
    return {"status_counts": dict(sorted(statuses.items())), "reason_counts": dict(sorted(reasons.items())), "peak_rss_bytes": _metric_stats(records, "peak_rss_bytes"), "tree_peak_rss_bytes": _metric_stats(records, "tree_peak_rss_bytes")}


def _read_samples(path: Path) -> list[dict[str, Any]]:
    if path.is_file() and path.suffix == ".jsonl":
        lines = path.read_text(encoding="utf-8").splitlines()
    elif path.is_dir() and (path / "samples.jsonl").is_file():
        lines = (path / "samples.jsonl").read_text(encoding="utf-8").splitlines()
    else:
        raise ValueError("input must be samples.jsonl or a runner output directory")
    records = [json.loads(line) for line in lines if line.strip()]
    if not records:
        raise ValueError("no samples")
    for record in records:
        if _contains_forbidden_key(record):
            raise ValueError("best-of summaries are forbidden")
        RunRecord.from_dict(record)
        _validate_sample_metrics(record)
    return records


def _group(records: list[dict[str, Any]], *keys: str) -> dict[tuple[Any, ...], list[dict[str, Any]]]:
    groups: dict[tuple[Any, ...], list[dict[str, Any]]] = {}
    for record in records:
        groups.setdefault(tuple(record.get(key) for key in keys), []).append(record)
    return groups


def _digest_group(manifest: Mapping[str, Any], key: str) -> str:
    return sha256_bytes(json.dumps(manifest.get(key, {}), sort_keys=True, separators=(",", ":")).encode())


def _items(payload: Any) -> list[str]:
    if not isinstance(payload, Mapping):
        return []
    values = payload.get("items", payload.get("nodes", payload.get("results", [])))
    if not isinstance(values, list):
        return []
    result: list[str] = []
    for value in values:
        if isinstance(value, str):
            result.append(value)
        elif isinstance(value, Mapping):
            for key in ("display", "name", "symbol", "qualified_name", "owner_path"):
                if isinstance(value.get(key), str) and value[key]:
                    result.append(value[key])
                    break
    return result


def _expected(case: Mapping[str, Any], manifest: Mapping[str, Any]) -> list[str]:
    oracle = manifest.get("oracles", {}).get(case["id"], {})
    if case.get("operation") == "path":
        return [str(item) for item in oracle.get("expected_shortest_path", [])]
    key = "expected_direct" if case.get("mode", "direct") == "direct" else "expected_transitive"
    return [str(item) for item in oracle.get(key, [])]


def _negative(case: Mapping[str, Any], manifest: Mapping[str, Any]) -> set[str]:
    values = manifest.get("oracles", {}).get(case["id"], {}).get("negative_exclusions", [])
    return {str(item) for item in values} if isinstance(values, list) else set()


def _quality(records: list[dict[str, Any]], case: Mapping[str, Any], manifest: Mapping[str, Any]) -> dict[str, Any]:
    expected = _expected(case, manifest)
    expected_set, negative = set(expected), _negative(case, manifest)
    actuals = [_items(record.get("canonical", {}).get("payload", {})) for record in records if record.get("status") == "PASS"]
    actual = actuals[0] if actuals else []
    actual_set = set(actual)
    tp, fp, fn = len(actual_set & expected_set), len(actual_set - expected_set), len(expected_set - actual_set)
    negative_violations = len(actual_set & negative)
    precision = tp / (tp + fp) if tp + fp else 0.0
    recall = tp / (tp + fn) if tp + fn else 0.0
    return {
        "correctness": {"numerator": int(actual == expected), "denominator": 1, "value": 1.0 if actual == expected else 0.0},
        "precision": {"tp": tp, "fp": fp, "denominator": tp + fp, "value": precision},
        "recall": {"tp": tp, "fn": fn, "denominator": tp + fn, "value": recall},
        "f1": {"numerator": 2 * tp, "denominator": 2 * tp + fp + fn, "value": 2 * precision * recall / (precision + recall) if precision + recall else 0.0},
        "negative_violations": {"count": negative_violations, "denominator": len(negative), "value": negative_violations},
    }


def _effective_status(group: list[dict[str, Any]]) -> str:
    statuses = {str(record.get("status")) for record in group}
    if "BLOCKED" in statuses:
        return "BLOCKED"
    if "FAIL" in statuses:
        return "FAIL"
    if "NOT_COMPARABLE" in statuses or any(record.get("metrics", {}).get("child", {}).get("status") == "NOT_COMPARABLE" for record in group):
        return "NOT_COMPARABLE"
    return "PASS" if statuses == {"PASS"} else "NOT_RUN"


def _load_manifest(manifest: str | Path | Mapping[str, Any] | None) -> tuple[dict[str, Any] | None, Path | None]:
    if manifest is None:
        return None, None
    if isinstance(manifest, Mapping):
        return dict(manifest), None
    path = Path(manifest).resolve()
    value = load_manifest(path)
    validate_strict_manifest(value, path.parent, check_files=True)
    return value, path


def build_report(samples: str | Path, *, manifest: str | Path | Mapping[str, Any] | None = None, expected_repetitions: int = AUTHORITATIVE_REPETITIONS) -> dict[str, Any]:
    if expected_repetitions != AUTHORITATIVE_REPETITIONS:
        raise ValueError("Victory Lab reports require exactly 30 repetitions")
    records = _read_samples(Path(samples))
    if any(not record.get("case_id") for record in records):
        raise ValueError("sample case_id is required")
    loaded_manifest, manifest_path = _load_manifest(manifest)
    expected_keys = None
    case_map: dict[tuple[str, str], Mapping[str, Any]] = {}
    if loaded_manifest is not None:
        fixture_digest, oracle_digest = _digest_group(loaded_manifest, "fixture_hashes"), _digest_group(loaded_manifest, "oracle_hashes")
        for record in records:
            if record.get("fixture_digest") != fixture_digest or record.get("oracle_digest") != oracle_digest:
                raise ValueError("sample fixture/oracle digest does not match manifest")
        expected_keys = {(adapter["adapter_id"], case["id"], case["operation"]) for adapter in loaded_manifest["adapters"] for case in loaded_manifest["cases"]}
        case_map = {(case["id"], case["operation"]): case for case in loaded_manifest["cases"]}
    groups = _group(records, "adapter_id", "case_id", "operation")
    if expected_keys is not None and set(groups) != expected_keys:
        raise ValueError(f"sample universe mismatch; missing={sorted(expected_keys - set(groups))}, extra={sorted(set(groups) - expected_keys)}")
    group_reports: dict[str, Any] = {}
    seen_samples: set[tuple[Any, ...]] = set()
    for (adapter_id, case_id, operation), group in sorted(groups.items()):
        if len(group) != 30:
            raise ValueError(f"{adapter_id}/{case_id}/{operation}: expected exactly 30 samples, got {len(group)}")
        sample_keys = [(adapter_id, case_id, operation, record.get("repetition")) for record in group]
        if len(set(sample_keys)) != 30 or seen_samples.intersection(sample_keys):
            raise ValueError("duplicate or replayed sample")
        seen_samples.update(sample_keys)
        if sorted(record.get("repetition") for record in group) != list(range(30)):
            raise ValueError(f"{adapter_id}/{case_id}/{operation}: repetitions must be exactly 0..29")
        fingerprints = [json.dumps({key: value for key, value in record.items() if key != "repetition"}, sort_keys=True, separators=(",", ":")) for record in group]
        if len(set(fingerprints)) != 30:
            raise ValueError("duplicate or replayed sample")
        digests = []
        for record in group:
            if record.get("status") == "PASS":
                canonical = record.get("canonical")
                if not isinstance(canonical, Mapping) or canonical.get("digest") != payload_digest(canonical.get("payload")):
                    raise ValueError("canonical digest does not match payload")
                digests.append(canonical["digest"])
        determinism = {"reruns": 30, "unique_digests": len(set(digests)), "outputs_identical": bool(digests) and len(set(digests)) == 1, "status": "PASS" if digests and len(set(digests)) == 1 else "FAIL"}
        case = case_map.get((case_id, operation), {"id": case_id, "operation": operation})
        quality = _quality(group, case, loaded_manifest) if loaded_manifest else {"status": "NOT_COMPARABLE"}
        tokens = [record["canonical"].get("token_units") for record in group if record.get("status") == "PASS" and isinstance(record.get("canonical"), Mapping) and isinstance(record["canonical"].get("token_units"), (int, float))]
        stale = [record.get("metrics", {}).get("stale_rate") for record in group if isinstance(record.get("metrics", {}).get("stale_rate"), (int, float))]
        incremental = [record.get("metrics", {}).get("incrementality_ms") for record in group if isinstance(record.get("metrics", {}).get("incrementality_ms"), (int, float))]
        status = _effective_status(group)
        if loaded_manifest is not None and status == "PASS" and (not incremental or not stale):
            status = "NOT_COMPARABLE"
        if status == "PASS" and determinism["status"] != "PASS":
            status = "FAIL"
        latency = latency_stats(group)
        group_reports[f"{adapter_id}:{case_id}:{operation}"] = {
            "adapter_id": adapter_id, "case_id": case_id, "operation": operation, "status": status,
            "status_counts": {item: sum(record["status"] == item for record in group) for item in ("PASS", "FAIL", "BLOCKED", "NOT_COMPARABLE", "NOT_RUN")},
            "latency": latency, "warm_p95_ms": latency["p95_ms"], "tokens": _stats(tokens), "child_metrics": child_metric_stats(group),
            "incrementality": {"status": "PASS" if incremental else "NOT_COMPARABLE", "samples": _stats(incremental), "stale_rate": _stats(stale) if stale else {"status": "NOT_COMPARABLE"}},
            "quality": quality, "determinism": determinism, "all_samples": 30, "canonical_digests": sorted(set(digests)),
        }
    counts = {item: sum(record["status"] == item for record in records) for item in ("PASS", "FAIL", "BLOCKED", "NOT_COMPARABLE", "NOT_RUN")}
    statuses = [item["status"] for item in group_reports.values()]
    status = "BLOCKED" if "BLOCKED" in statuses else "FAIL" if "FAIL" in statuses else "NOT_COMPARABLE" if "NOT_COMPARABLE" in statuses or (loaded_manifest is not None and (loaded_manifest.get("thresholds", {}).get("status") != "PASS")) else "PASS" if statuses and all(item == "PASS" for item in statuses) else "NOT_RUN"
    return {"schema": "victory-report/v2", "status": status, "samples": len(records), "status_counts": counts, "manifest": str(manifest_path) if manifest_path else None, "expected_repetitions": 30, "expected_universe": len(expected_keys) if expected_keys is not None else None, "groups": group_reports, "thresholds": (loaded_manifest or {}).get("thresholds", {"status": "NOT_COMPARABLE", "reason_code": "thresholds_not_declared"}), "anti_gaming": {"all_samples_used": True, "best_of": False, "exactly_30": all(len(group) == 30 for group in groups.values()), "manifest_consumed": loaded_manifest is not None}}


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--input", required=True)
    parser.add_argument("--output", required=True)
    parser.add_argument("--manifest", required=True)
    parser.add_argument("--repetitions", type=int, default=30)
    args = parser.parse_args(argv)
    try:
        report = build_report(args.input, manifest=args.manifest, expected_repetitions=args.repetitions)
    except (OSError, ValueError, json.JSONDecodeError) as exc:
        print(f"BLOCKED: {exc}")
        return 2
    Path(args.output).write_text(json.dumps(report, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    print(json.dumps(report, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
