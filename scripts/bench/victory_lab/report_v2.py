"""Aggregate complete Victory Lab v2 samples without best-of selection."""
from __future__ import annotations

import argparse
import json
import statistics
from pathlib import Path
from typing import Any, Iterable

try:
    from .canonical_v2 import payload_digest
    from .schema_v2 import RunRecord
except ImportError:  # pragma: no cover
    from canonical_v2 import payload_digest
    from schema_v2 import RunRecord

AUTHORITATIVE_REPETITIONS = 30
_FORBIDDEN_KEYS = frozenset({"best", "best_of", "minimum", "fastest", "selected_sample", "summary"})


def percentile(values: Iterable[float], p: float) -> float | None:
    values = sorted(float(value) for value in values)
    if not values:
        return None
    rank = (len(values) - 1) * p / 100
    low, high = int(rank), min(int(rank) + 1, len(values) - 1)
    return values[low] + (values[high] - values[low]) * (rank - low)


def latency_stats(records: list[dict[str, Any]]) -> dict[str, Any]:
    values = [float(record["elapsed_ms"]) for record in records if isinstance(record.get("elapsed_ms"), (int, float))]
    return {
        "n": len(values), "p50_ms": percentile(values, 50), "p95_ms": percentile(values, 95),
        "max_ms": max(values) if values else None, "mean_ms": statistics.fmean(values) if values else None,
    }


def _metric_stats(records: list[dict[str, Any]], key: str) -> dict[str, Any]:
    values: list[float] = []
    for record in records:
        child = record.get("metrics", {}).get("child", {})
        value = child.get(key) if isinstance(child, dict) else None
        if isinstance(value, (int, float)) and not isinstance(value, bool):
            values.append(float(value))
    return {
        "n": len(values), "p50": percentile(values, 50), "p95": percentile(values, 95),
        "max": max(values) if values else None,
    }


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
        if isinstance(value, bool) or not isinstance(value, (int, float)):
            raise ValueError("PASS sample is missing peak_rss_bytes")
        if value < 0:
            raise ValueError("peak_rss_bytes must be non-negative")
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
    return {
        "status_counts": dict(sorted(statuses.items())),
        "reason_counts": dict(sorted(reasons.items())),
        "peak_rss_bytes": _metric_stats(records, "peak_rss_bytes"),
        "tree_peak_rss_bytes": _metric_stats(records, "tree_peak_rss_bytes"),
    }


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


def build_report(samples: str | Path, *, expected_repetitions: int = AUTHORITATIVE_REPETITIONS) -> dict[str, Any]:
    if expected_repetitions != AUTHORITATIVE_REPETITIONS:
        raise ValueError("Victory Lab reports require exactly 30 repetitions")
    records = _read_samples(Path(samples))
    if any(not record.get("case_id") for record in records):
        raise ValueError("sample case_id is required")
    groups = _group(records, "adapter_id", "case_id", "operation")
    group_reports = {}
    seen_samples: set[tuple[Any, ...]] = set()
    for (adapter_id, case_id, operation), group in sorted(groups.items()):
        if len(group) != expected_repetitions:
            raise ValueError(f"{adapter_id}/{case_id}/{operation}: expected {expected_repetitions} samples, got {len(group)}")
        repetitions = [record.get("repetition") for record in group]
        sample_keys = [(adapter_id, case_id, operation, record.get("repetition")) for record in group]
        if len(set(sample_keys)) != len(sample_keys) or seen_samples.intersection(sample_keys):
            raise ValueError("duplicate or replayed sample")
        fingerprints = [
            json.dumps({key: value for key, value in record.items() if key != "repetition"}, sort_keys=True, separators=(",", ":"))
            for record in group
        ]
        if len(set(fingerprints)) != len(fingerprints):
            raise ValueError("duplicate or replayed sample")
        seen_samples.update(sample_keys)
        if sorted(repetitions) != list(range(expected_repetitions)):
            raise ValueError(f"{adapter_id}/{case_id}/{operation}: repetitions must be exactly 0..{expected_repetitions - 1}")
        pass_digests: set[str] = set()
        for record in group:
            if record["status"] == "PASS":
                canonical = record.get("canonical")
                if not isinstance(canonical, dict) or canonical.get("digest") != payload_digest(canonical.get("payload")):
                    raise ValueError("canonical digest does not match payload")
                pass_digests.add(canonical["digest"])
        if len(pass_digests) > 1:
            raise ValueError(f"nondeterministic canonical digest: {adapter_id}/{case_id}/{operation}")
        statuses = {status: sum(record["status"] == status for record in group) for status in ("PASS", "FAIL", "BLOCKED", "NOT_COMPARABLE", "NOT_RUN")}
        child_unavailable = any(record["metrics"]["child"]["status"] == "NOT_COMPARABLE" for record in group)
        if statuses["BLOCKED"]:
            status = "BLOCKED"
        elif statuses["FAIL"]:
            status = "FAIL"
        elif child_unavailable or statuses["NOT_COMPARABLE"]:
            status = "NOT_COMPARABLE"
        elif statuses["PASS"] == len(group):
            status = "PASS"
        else:
            status = "NOT_RUN"
        group_reports[f"{adapter_id}:{case_id}:{operation}"] = {
            "adapter_id": adapter_id, "case_id": case_id, "operation": operation, "status": status,
            "status_counts": statuses, "latency": latency_stats(group), "child_metrics": child_metric_stats(group),
            "all_samples": len(group), "canonical_digests": sorted(pass_digests),
        }
    counts = {status: sum(record["status"] == status for record in records) for status in ("PASS", "FAIL", "BLOCKED", "NOT_COMPARABLE", "NOT_RUN")}
    child_unavailable = any(record["metrics"]["child"]["status"] == "NOT_COMPARABLE" for record in records)
    if counts["BLOCKED"]:
        status = "BLOCKED"
    elif counts["FAIL"]:
        status = "FAIL"
    elif child_unavailable or counts["NOT_COMPARABLE"]:
        status = "NOT_COMPARABLE"
    elif counts["PASS"] == len(records):
        status = "PASS"
    else:
        status = "NOT_RUN"
    return {
        "schema": "victory-report/v2", "status": status, "samples": len(records),
        "status_counts": counts, "groups": group_reports,
        "anti_gaming": {"all_samples_used": True, "best_of": False, "p50_p95_max_reported": True},
    }


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--input", required=True)
    parser.add_argument("--output", required=True)
    parser.add_argument("--repetitions", type=int, default=30)
    args = parser.parse_args(argv)
    try:
        report = build_report(args.input, expected_repetitions=args.repetitions)
    except (OSError, ValueError, json.JSONDecodeError) as exc:
        print(f"BLOCKED: {exc}")
        return 2
    Path(args.output).write_text(json.dumps(report, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    print(json.dumps(report, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
