"""Authoritative Victory Lab v2 runner with serialized, complete sample retention."""
from __future__ import annotations

import argparse
import hashlib
import json
import math
import secrets
import sys
from pathlib import Path
from typing import Any, Mapping

try:
    from .adapters.v2 import VictoryAdapter
    from .canonical_v2 import common_token_units, payload_digest, validate_terminal_state
    from .manifest_v2 import ManifestError, canonical_file_hash, load_manifest, manifest_adapters, sha256_bytes, sha256_file
    from .schema_v2 import RunRecord
    from .validate_manifest import validate_runtime, validate_strict_manifest
    from .security_gate import runtime_evidence_digest
    from .sanitize_v2 import validate_runtime_security_keys
except ImportError:  # pragma: no cover
    from adapters.v2 import VictoryAdapter
    from canonical_v2 import common_token_units, payload_digest, validate_terminal_state
    from manifest_v2 import ManifestError, canonical_file_hash, load_manifest, manifest_adapters, sha256_bytes, sha256_file
    from schema_v2 import RunRecord
    from validate_manifest import validate_runtime, validate_strict_manifest
    from security_gate import runtime_evidence_digest
    from sanitize_v2 import validate_runtime_security_keys


def _digest_group(manifest: Mapping[str, Any], key: str) -> str:
    value = json.dumps(manifest.get(key, {}), sort_keys=True, separators=(",", ":")).encode()
    return sha256_bytes(value)


def _runtime_preflight_digest(manifest_path: Path, manifest: Mapping[str, Any]) -> str:
    """Bind the fresh runtime reproduction to this exact measurement manifest."""
    evidence = {
        "manifest_sha256": sha256_file(manifest_path),
        "adapters": [
            {
                "adapter_id": adapter.get("adapter_id"),
                "attestation_sha256": adapter.get("expected_attestation_sha256"),
                "executable_sha256": adapter.get("expected_executable_sha256"),
                "interpreter_sha256": adapter.get("interpreter_sha256"),
                "source_commit": adapter.get("expected_commit"),
                "source_sha256": adapter.get("expected_source_sha256"),
            }
            for adapter in manifest.get("adapters", [])
        ],
    }
    return sha256_bytes(json.dumps(evidence, sort_keys=True, separators=(",", ":")).encode())


def _sample_nonce(run_id: str, preflight_digest: str, group_key: str, repetition: int) -> str:
    material = "\\0".join((run_id, preflight_digest, group_key, str(repetition)))
    return hashlib.sha256(("victory-sample/v1\\0" + material).encode("utf-8")).hexdigest()


def _freshness(run_id: str, preflight_digest: str, group_key: str, repetition: int) -> dict[str, Any]:
    return {
        "schema": "victory-sample-freshness/v1",
        "run_id": run_id,
        "preflight_digest": preflight_digest,
        "group_id": "g" + hashlib.sha256(group_key.encode("utf-8")).hexdigest()[:63],
        "repetition": repetition,
        "nonce": _sample_nonce(run_id, preflight_digest, group_key, repetition),
    }


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


_RUNNER_ERROR_SCHEMA = "victory-runner-error/v2"
_RUN_ARTIFACTS = ("run.json", "samples.jsonl", "runner-error.json")


def _effective_status_counts(status_counts: Mapping[str, int], child_statuses: set[str]) -> str:
    statuses = {status for status, count in status_counts.items() if count}
    if "BLOCKED" in statuses:
        return "BLOCKED"
    if "FAIL" in statuses:
        return "FAIL"
    if "NOT_COMPARABLE" in statuses or "NOT_COMPARABLE" in child_statuses:
        return "NOT_COMPARABLE"
    if statuses == {"PASS"}:
        return "PASS"
    return "NOT_RUN"


def _runner_reason_code(exc: BaseException) -> str:
    """Map an abort to the closed, durable reason-code catalog without leaking it."""
    if isinstance(exc, (ManifestError, ValueError)):
        return "blocked"
    return "native_error"


def _write_runner_error(
    output_path: Path,
    *,
    run_id: str,
    manifest_sha256: str,
    runtime_preflight_digest: str,
    completed_samples: int,
    expected_samples: int,
    reason_code: str,
) -> None:
    payload = {
        "schema": _RUNNER_ERROR_SCHEMA,
        "status": "BLOCKED",
        "reason_code": reason_code,
        "completed_samples": completed_samples,
        "expected_samples": expected_samples,
        "run_id": run_id,
        "manifest_sha256": manifest_sha256,
        "runtime_preflight_digest": runtime_preflight_digest,
    }
    error_path = output_path / "runner-error.json"
    with error_path.open("x", encoding="utf-8", newline="\n") as stream:
        stream.write(json.dumps(payload, indent=2, sort_keys=True) + "\n")


def _is_sha256(value: Any) -> bool:
    return isinstance(value, str) and len(value) == 64 and all(char in "0123456789abcdef" for char in value)


def _is_finite_number(value: Any) -> bool:
    if not isinstance(value, (int, float)) or isinstance(value, bool):
        return False
    try:
        return math.isfinite(float(value))
    except (OverflowError, ValueError):
        return False


def _valid_pid_list(value: Any, *, nonempty: bool) -> bool:
    return (
        isinstance(value, list)
        and (bool(value) or not nonempty)
        and all(isinstance(pid, int) and not isinstance(pid, bool) and pid > 0 for pid in value)
        and len(set(value)) == len(value)
    )


def _runtime_digest_matches(runtime: Mapping[str, Any]) -> bool:
    try:
        validate_runtime_security_keys(runtime)
        return runtime_evidence_digest(runtime) == runtime.get("evidence_digest")
    except (TypeError, ValueError):
        return False


def _validate_runtime_projection(sample: Mapping[str, Any]) -> None:
    metrics = sample.get("metrics")
    security = metrics.get("security") if isinstance(metrics, Mapping) else None
    runtime = security.get("runtime") if isinstance(security, Mapping) else None
    if runtime is not None:
        if not isinstance(runtime, Mapping):
            raise ManifestError("runtime security projection must be an object")
        try:
            validate_runtime_security_keys(runtime)
        except ValueError as exc:
            raise ManifestError(str(exc)) from exc


def _manifest_identity(path: Path) -> str:
    canonical_path = path.resolve().as_posix().casefold()
    return "m" + sha256_bytes(canonical_path.encode("utf-8"))[:63]


def _validate_pass_sample(sample: Mapping[str, Any]) -> None:
    """Reject a forged PASS before it can enter durable runner evidence."""
    canonical = sample.get("canonical")
    if not isinstance(canonical, Mapping) or not isinstance(canonical.get("payload"), Mapping):
        raise ManifestError("adapter returned PASS without a canonical payload")
    payload = canonical["payload"]
    operation = canonical.get("operation")
    if operation != sample.get("operation"):
        raise ManifestError("adapter returned PASS with canonical operation drift")
    validate_terminal_state(payload)
    if canonical.get("digest") != payload_digest(payload):
        raise ManifestError("adapter returned PASS with a canonical digest mismatch")
    token_units = canonical.get("token_units")
    try:
        expected_token_units = common_token_units(operation, payload)
    except ValueError as exc:
        raise ManifestError("adapter returned PASS with an invalid common semantic projection") from exc
    if (
        isinstance(token_units, bool)
        or not _is_finite_number(token_units)
        or token_units <= 0
        or token_units != expected_token_units
    ):
        raise ManifestError("adapter returned PASS with invalid common-projection token_units")

    if not _is_finite_number(sample.get("elapsed_ms")):
        raise ManifestError("adapter returned PASS without finite elapsed_ms")
    metrics = sample.get("metrics")
    child = metrics.get("child") if isinstance(metrics, Mapping) else None
    if not isinstance(child, Mapping) or child.get("status") != "PASS":
        raise ManifestError("adapter returned PASS without successful child metrics")
    if not _is_finite_number(child.get("peak_rss_bytes")) or child["peak_rss_bytes"] < 0:
        raise ManifestError("adapter returned PASS without measured child RSS")
    if not _is_finite_number(child.get("tree_peak_rss_bytes")) or child["tree_peak_rss_bytes"] < 0:
        raise ManifestError("adapter returned PASS without measured tree RSS")
    if child.get("tree_supported") is not True:
        raise ManifestError("adapter returned PASS without process-tree support")
    if not isinstance(child.get("samples"), int) or isinstance(child.get("samples"), bool) or child["samples"] <= 0:
        raise ManifestError("adapter returned PASS without child samples")
    if child.get("timed_out") is not False or child.get("crashed") is not False:
        raise ManifestError("adapter returned PASS with an unsuccessful child terminal state")
    if child.get("failure_class") != "none" or not isinstance(child.get("exit_code"), int) or isinstance(child.get("exit_code"), bool) or child.get("exit_code") != 0:
        raise ManifestError("adapter returned PASS with an unsuccessful child exit")
    if child.get("cleanup_status") not in {"not_required", "clean", "forced"}:
        raise ManifestError("adapter returned PASS without successful child cleanup")
    if (
        child.get("timed_out") is not False
        or child.get("crashed") is not False
        or child.get("failure_class") != "none"
        or not isinstance(child.get("exit_code"), int)
        or isinstance(child.get("exit_code"), bool)
        or child.get("exit_code") != 0
    ):
        raise ManifestError("adapter returned PASS with an unsuccessful child terminal state")

    security = metrics.get("security") if isinstance(metrics, Mapping) else None
    runtime = security.get("runtime") if isinstance(security, Mapping) else None
    integrity = security.get("integrity") if isinstance(security, Mapping) else None
    source_integrity = security.get("source_integrity") if isinstance(security, Mapping) else None
    _validate_runtime_projection(sample)
    if (
        not isinstance(security, Mapping)
        or security.get("status") != "PASS"
        or security.get("runtime_proof") is not True
        or not isinstance(runtime, Mapping)
        or runtime.get("status") != "PASS"
        or runtime.get("runtime_proof") is not True
        or runtime.get("provenance") != "child_metrics_executor"
        or not isinstance(runtime.get("probe_mode"), str)
        or not runtime.get("probe_mode")
        or runtime.get("observed_network_count") != 0
        or runtime.get("observed_mcp_count") != 0
        or runtime.get("reason_code") is not None
        or not isinstance(runtime.get("sample_count"), int)
        or isinstance(runtime.get("sample_count"), bool)
        or runtime.get("sample_count", 0) <= 0
        or not _valid_pid_list(runtime.get("observed_pids"), nonempty=True)
        or not _valid_pid_list(runtime.get("metadata_observed_pids"), nonempty=False)
        or not set(runtime.get("observed_pids", [])) <= set(runtime.get("metadata_observed_pids", []))
        or not _is_sha256(runtime.get("evidence_digest"))
        or not _runtime_digest_matches(runtime)
        or not isinstance(integrity, Mapping)
        or integrity.get("status") != "PASS"
        or not isinstance(source_integrity, Mapping)
        or source_integrity.get("status") != "PASS"
    ):
        raise ManifestError("adapter returned PASS without complete security/provenance evidence")


def run_manifest(manifest_path: str | Path, output: str | Path, *, repetitions: int = 30, mode: str = "authoritative", executor: Any = None) -> dict[str, Any]:
    path = Path(manifest_path).resolve()
    manifest = load_manifest(path)
    validate_strict_manifest(manifest, path.parent, check_files=True)
    runtime_blockers = validate_runtime(manifest, require_runtime=True, manifest_root=path.parent)
    if runtime_blockers:
        raise ManifestError("runtime preflight failed: " + "; ".join(runtime_blockers))
    runtime_preflight_digest = _runtime_preflight_digest(path, manifest)
    manifest_sha256 = sha256_file(path)
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
    existing_artifacts = [name for name in _RUN_ARTIFACTS if (output_path / name).exists()]
    if existing_artifacts:
        raise ManifestError("output directory already contains run artifacts")
    adapters = manifest_adapters(manifest)
    cases = {case["id"]: case for case in manifest["cases"]}
    groups = manifest.get("groups")
    if not isinstance(groups, list) or not groups:
        raise ManifestError("manifest must declare explicit measurement groups")
    workloads = {item["workload_id"]: item for item in manifest.get("workloads", []) if isinstance(item, Mapping)}
    expected_samples = len(groups) * repetitions
    run_id = "r" + hashlib.sha256(secrets.token_bytes(32)).hexdigest()[:63]
    samples_path = output_path / "samples.jsonl"
    completed_samples = 0
    status_counts = {status: 0 for status in ("PASS", "FAIL", "BLOCKED", "NOT_COMPARABLE", "NOT_RUN")}
    child_statuses: set[str] = set()
    seen_sample_keys: set[tuple[str, str, str, int]] = set()
    # Deliberately serial: only the exact manifest groups are authoritative.
    # Open the stream before invoking the first adapter so validated samples survive an abort.
    with samples_path.open("x", encoding="utf-8", newline="\n") as stream:
        try:
            for group in groups:
                group_id = group.get("group_id")
                adapter_id = group.get("adapter_id")
                workload = workloads.get(group.get("workload_id"))
                case = cases.get(group.get("case_id"))
                if not isinstance(group_id, str) or not isinstance(adapter_id, str) or workload is None or case is None:
                    raise ManifestError("group references an unknown adapter, workload, or case")
                if adapter_id not in adapters or group.get("operation") != case.get("operation") or workload.get("case_id") != case.get("id"):
                    raise ManifestError(f"group {group_id} is not an exact workload declaration")
                adapter = VictoryAdapter(adapters[adapter_id], manifest, path.parent, executor=executor)
                for repetition in range(repetitions):
                    record = adapter.run_case(case, repetition=repetition)
                    sample = record.to_dict()
                    returned_case_id = sample.get("case_id")
                    returned_repetition = sample.get("repetition")
                    if sample.get("adapter_id") != adapter_id or sample.get("operation") != case["operation"]:
                        raise ManifestError("adapter returned a sample for the wrong comparator or operation")
                    if not (isinstance(returned_case_id, str) and returned_case_id in ("", case["id"])):
                        raise ManifestError("adapter returned a sample for the wrong case_id")
                    if not isinstance(returned_repetition, int) or isinstance(returned_repetition, bool) or returned_repetition != repetition:
                        raise ManifestError("adapter returned a sample for the wrong repetition")
                    sample["case_id"] = case["id"]
                    sample["fixture_digest"] = manifest["fixture_digest"]
                    sample["oracle_digest"] = manifest["oracle_digest"]
                    sample_key = (group_id, case["id"], case["operation"], repetition)
                    if sample_key in seen_sample_keys:
                        raise ManifestError("adapter returned a duplicate explicit group sample key")
                    seen_sample_keys.add(sample_key)
                    metrics = sample.get("metrics")
                    if not isinstance(metrics, dict):
                        raise ManifestError("adapter returned malformed metrics")
                    metrics = dict(metrics)
                    sample["metrics"] = metrics
                    _validate_runtime_projection(sample)
                    metrics["freshness"] = _freshness(run_id, runtime_preflight_digest, group_id, repetition)
                    sample["metrics"] = metrics
                    RunRecord.from_dict(sample)
                    if sample.get("status") == "PASS":
                        _validate_pass_sample(sample)

                    stream.write(json.dumps(sample, ensure_ascii=False, sort_keys=True, separators=(",", ":")) + "\n")
                    stream.flush()
                    completed_samples += 1
                    status = str(sample.get("status"))
                    if status in status_counts:
                        status_counts[status] += 1
                    child = metrics.get("child")
                    if isinstance(child, Mapping) and isinstance(child.get("status"), str):
                        child_statuses.add(child["status"])

            _assert_content_unchanged(content_before, path, manifest)
            if completed_samples != expected_samples:
                raise ManifestError(f"authoritative runner retained {completed_samples} samples, expected {expected_samples}")
        except Exception as exc:
            try:
                stream.flush()
            except OSError:
                pass
            try:
                _write_runner_error(
                    output_path,
                    run_id=run_id,
                    manifest_sha256=manifest_sha256,
                    runtime_preflight_digest=runtime_preflight_digest,
                    completed_samples=completed_samples,
                    expected_samples=expected_samples,
                    reason_code=_runner_reason_code(exc),
                )
            except OSError:
                pass
            raise
    samples_sha256 = sha256_file(samples_path)
    manifest_path = str(path)
    manifest_id = _manifest_identity(path)
    summary = {
        "schema": "victory-run/v2",
        "run_id": run_id,
        "manifest": manifest_path,
        "manifest_path": manifest_path,
        "manifest_id": manifest_id,
        "manifest_sha256": manifest_sha256,
        "manifest_bundle": {"path": manifest_path, "id": manifest_id, "sha256": manifest_sha256},
        "manifest_schema": manifest["schema"],
        "status": _effective_status_counts(status_counts, child_statuses),
        "mode": mode,
        "repetitions": repetitions,
        "authoritative": mode == "authoritative",
        "adapters": sorted(adapters),
        "cases": [case["id"] for case in manifest["cases"]],
        "groups": [group["group_id"] for group in groups],
        "expected_groups": len(groups),
        "samples": completed_samples,
        "samples_path": str(samples_path),
        "samples_sha256": samples_sha256,
        "status_counts": status_counts,
        "fixture_digest": manifest["fixture_digest"],
        "oracle_digest": manifest["oracle_digest"],
        "runtime_preflight": {
            "status": "PASS",
            "require_runtime": True,
            "fresh_reproduction": True,
            "evidence_digest": runtime_preflight_digest,
        },
        "anti_gaming": {"serialized_variants": True, "all_samples_retained": True, "best_of_rejected": True, "cartesian_expansion": False, "explicit_groups_only": True},
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
    except Exception as exc:
        print(f"BLOCKED: {exc}", file=sys.stderr)
        return 2
    print(json.dumps(summary, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
