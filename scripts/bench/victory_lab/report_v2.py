"""Aggregate complete Victory Lab v2 samples without best-of selection."""
from __future__ import annotations

import argparse
import json
import statistics
from pathlib import Path
from typing import Any, Iterable

try:
    from .schema_v2 import RunRecord
except ImportError:  # pragma: no cover
    from schema_v2 import RunRecord


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
        RunRecord.from_dict(record)
        if any(key.lower() in {"best", "best_of", "minimum", "fastest"} for key in record):
            raise ValueError("best-of summaries are forbidden")
    return records


def _group(records: list[dict[str, Any]], *keys: str) -> dict[tuple[Any, ...], list[dict[str, Any]]]:
    groups: dict[tuple[Any, ...], list[dict[str, Any]]] = {}
    for record in records:
        groups.setdefault(tuple(record.get(key) for key in keys), []).append(record)
    return groups


def build_report(samples: str | Path, *, expected_repetitions: int = 30) -> dict[str, Any]:
    records = _read_samples(Path(samples))
    groups = _group(records, "adapter_id", "operation")
    group_reports = {}
    for (adapter_id, operation), group in sorted(groups.items()):
        if len(group) != expected_repetitions:
            raise ValueError(f"{adapter_id}/{operation}: expected {expected_repetitions} samples, got {len(group)}")
        repetitions = [record.get("repetition") for record in group]
        if sorted(repetitions) != list(range(expected_repetitions)):
            raise ValueError(f"{adapter_id}/{operation}: repetitions must be exactly 0..{expected_repetitions - 1}")
        statuses = {status: sum(record["status"] == status for record in group) for status in ("PASS", "FAIL", "BLOCKED", "NOT_COMPARABLE", "NOT_RUN")}
        if statuses["BLOCKED"]:
            status = "BLOCKED"
        elif statuses["FAIL"]:
            status = "FAIL"
        elif statuses["PASS"] == len(group):
            status = "PASS"
        elif statuses["NOT_COMPARABLE"] == len(group):
            status = "NOT_COMPARABLE"
        else:
            status = "NOT_RUN"
        group_reports[f"{adapter_id}:{operation}"] = {
            "adapter_id": adapter_id, "operation": operation, "status": status,
            "status_counts": statuses, "latency": latency_stats(group), "child_metrics": child_metric_stats(group),
            "all_samples": len(group), "canonical_digests": sorted({record["canonical"]["digest"] for record in group if record.get("canonical")}),
        }
    counts = {status: sum(record["status"] == status for record in records) for status in ("PASS", "FAIL", "BLOCKED", "NOT_COMPARABLE", "NOT_RUN")}
    if counts["BLOCKED"]:
        status = "BLOCKED"
    elif counts["FAIL"]:
        status = "FAIL"
    elif counts["PASS"] and counts["PASS"] + counts["NOT_COMPARABLE"] == len(records):
        status = "PASS" if not counts["NOT_COMPARABLE"] else "NOT_COMPARABLE"
    elif counts["NOT_COMPARABLE"] == len(records):
        status = "NOT_COMPARABLE"
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
