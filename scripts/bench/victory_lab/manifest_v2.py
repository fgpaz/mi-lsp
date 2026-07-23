"""Manifest loading, pin validation, and fixture/oracle hashing for Victory Lab v2."""
from __future__ import annotations

import hashlib
import json
import os
import re
import subprocess
from pathlib import Path
from typing import Any, Mapping

try:
    from .attestation_v2 import AttestationError, validate_attestation
    from .schema_v2 import ADAPTER_SCHEMA, MANIFEST_SCHEMA, OPERATIONS, AdapterSpec, SchemaError
except ImportError:  # pragma: no cover - direct script compatibility
    from attestation_v2 import AttestationError, validate_attestation
    from schema_v2 import ADAPTER_SCHEMA, MANIFEST_SCHEMA, OPERATIONS, AdapterSpec, SchemaError

BASELINE_COMMIT = "a251ab1f8db4e96f029926fbef275b078a20a111"
GRAPHIFY_COMMIT = "9bf14a4931658152969586ace39eb965c010f0d1"
GRAPHIFY_VERSION = "0.9.19"
DEFAULT_PATHS = {
    "current": r"C:\tmp\milsp-g9-bin\mi-lsp-current.exe",
    "baseline": r"C:\tmp\milsp-g9-bin\mi-lsp-baseline.exe",
    "current_source": "",
    "baseline_source": "",
    "graphify_source": "",
    "graphify_python": r"C:\tmp\graphify-bench-venv\Scripts\python.exe",
}
_PATH_ENV = {
    "current": "VICTORY_LAB_CURRENT_EXE",
    "baseline": "VICTORY_LAB_BASELINE_EXE",
    "current_source": "VICTORY_LAB_CURRENT_SOURCE",
    "baseline_source": "VICTORY_LAB_BASELINE_SOURCE",
    "graphify_source": "VICTORY_LAB_GRAPHIFY_SOURCE",
    "graphify_python": "VICTORY_LAB_GRAPHIFY_PYTHON",
}
_SHA_RE = re.compile(r"^[0-9a-f]{64}$")


class ManifestError(ValueError):
    """Raised when the v2 manifest cannot be trusted."""


def sha256_bytes(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def canonical_file_hash(path: Path) -> str:
    """Hash fixture bytes exactly; line ending normalization is not authority."""
    return sha256_file(path)


def inventory_entries(root: Path) -> list[dict[str, Any]]:
    """Return the recursively enumerated raw-byte corpus/golden universe."""
    root = Path(root).resolve()
    entries: list[dict[str, Any]] = []
    for base_name in ("corpus", "goldens"):
        base = root / base_name
        if not base.is_dir():
            raise ManifestError(f"inventory root is missing: {base_name}")
        for path in sorted((item for item in base.rglob("*") if item.is_file()), key=lambda item: item.relative_to(root).as_posix()):
            relative = path.relative_to(root).as_posix()
            if relative == "goldens/inventory.json":
                continue
            if path.is_symlink():
                raise ManifestError(f"inventory cannot include symlink: {relative}")
            data = path.read_bytes()
            entries.append({"bytes": len(data), "path": relative, "sha256": sha256_bytes(data)})
    return entries


def validate_inventory(root: Path, inventory: Mapping[str, Any]) -> None:
    """Require an exact recursive raw-byte inventory, including no omissions."""
    if inventory.get("schema") != "victory-inventory/v2" or inventory.get("byte_semantics") != "raw-byte":
        raise ManifestError("unsupported or non-raw inventory")
    files = inventory.get("files")
    if not isinstance(files, list) or files != sorted(files, key=lambda item: str(item.get("path", ""))):
        raise ManifestError("inventory files must be an ordered list")
    expected = inventory_entries(root)
    normalized: list[dict[str, Any]] = []
    for item in files:
        if not isinstance(item, Mapping) or set(item) != {"bytes", "path", "sha256"}:
            raise ManifestError("inventory entry schema drift")
        relative = item.get("path")
        if not isinstance(relative, str) or Path(relative).is_absolute() or ".." in Path(relative).parts:
            raise ManifestError("inventory contains unsafe path")
        if not isinstance(item.get("bytes"), int) or item["bytes"] < 0:
            raise ManifestError("inventory bytes must be a non-negative integer")
        digest = item.get("sha256")
        if not isinstance(digest, str) or not _SHA_RE.fullmatch(digest):
            raise ManifestError("inventory contains invalid digest")
        normalized.append({"bytes": item["bytes"], "path": relative.replace("\\", "/"), "sha256": digest})
    if normalized != expected:
        raise ManifestError("recursive raw-byte inventory mismatch")


def resolve_configured_path(name: str, value: str | None = None) -> Path:
    if name not in DEFAULT_PATHS:
        raise ManifestError(f"unknown configurable path: {name}")
    configured = value or os.environ.get(_PATH_ENV[name]) or DEFAULT_PATHS[name]
    if isinstance(configured, str) and configured.startswith("${") and configured.endswith("}"):
        env_name = configured[2:-1]
        if env_name not in _PATH_ENV.values():
            raise ManifestError(f"unknown portable path variable: {env_name}")
        configured = os.environ.get(env_name, "")
    if not configured or "\x00" in configured:
        raise ManifestError(f"invalid configured path: {name}")
    return Path(configured).expanduser()


def _require_sha_map(value: Any, name: str) -> dict[str, str]:
    if not isinstance(value, Mapping) or not value:
        raise ManifestError(f"{name} must be a non-empty map")
    result = {}
    for rel, digest in value.items():
        if not isinstance(rel, str) or Path(rel).is_absolute() or ".." in Path(rel).parts:
            raise ManifestError(f"{name} contains unsafe path: {rel!r}")
        if not isinstance(digest, str) or not _SHA_RE.fullmatch(digest):
            raise ManifestError(f"{name} contains invalid sha256 for {rel!r}")
        result[rel.replace("\\", "/")] = digest
    return result


def _validate_operation_case(case: Mapping[str, Any]) -> None:
    if not isinstance(case.get("id"), str) or not case["id"]:
        raise ManifestError("case id is required")
    operation = case.get("operation")
    if operation not in OPERATIONS:
        raise ManifestError(f"unsupported case operation: {operation}")
    if not isinstance(case.get("golden"), str):
        raise ManifestError(f"case {case['id']} lacks golden")
    corpus = case.get("corpus")
    if not isinstance(corpus, list) or not corpus:
        raise ManifestError(f"case {case['id']} lacks corpus")
    for path in corpus:
        if not isinstance(path, str) or Path(path).is_absolute() or ".." in Path(path).parts:
            raise ManifestError(f"case {case['id']} has unsafe corpus path")
    if operation == "callers" and not case.get("selector"):
        raise ManifestError(f"case {case['id']} lacks selector")
    if operation == "affected" and not case.get("changed_paths"):
        raise ManifestError(f"case {case['id']} lacks changed_paths")
    if operation == "path" and not case.get("from") or operation == "path" and not case.get("to"):
        raise ManifestError(f"case {case['id']} lacks path endpoints")


_CANONICAL_WORKLOADS: dict[str, dict[str, str]] = {
    "callers-direct": {"workload_id": "callers-direct", "case_id": "callers-direct", "operation": "callers", "mode": "direct"},
    "callers-transitive": {"workload_id": "callers-transitive", "case_id": "callers-transitive", "operation": "callers", "mode": "transitive"},
    "affected-direct": {"workload_id": "affected-direct", "case_id": "affected-direct", "operation": "affected", "mode": "direct"},
    "affected-transitive": {"workload_id": "affected-transitive", "case_id": "affected-transitive", "operation": "affected", "mode": "transitive"},
    "path-shortest": {"workload_id": "path-shortest", "case_id": "path-shortest", "operation": "path"},
}
_CANONICAL_GROUPS: dict[str, dict[str, Any]] = {
    "current-callers-direct": {"adapter_id": "mi-lsp-current-v2", "workload_id": "callers-direct", "case_id": "callers-direct", "operation": "callers"},
    "current-callers-transitive": {"adapter_id": "mi-lsp-current-v2", "workload_id": "callers-transitive", "case_id": "callers-transitive", "operation": "callers"},
    "graphify-callers-direct": {"adapter_id": "graphify-0.9.19-v2", "workload_id": "callers-direct", "case_id": "callers-direct", "operation": "callers"},
    "graphify-callers-transitive": {"adapter_id": "graphify-0.9.19-v2", "workload_id": "callers-transitive", "case_id": "callers-transitive", "operation": "callers"},
    "current-affected-direct": {"adapter_id": "mi-lsp-current-v2", "workload_id": "affected-direct", "case_id": "affected-direct", "operation": "affected"},
    "current-affected-transitive": {"adapter_id": "mi-lsp-current-v2", "workload_id": "affected-transitive", "case_id": "affected-transitive", "operation": "affected"},
    "baseline-affected-direct-hotpath": {"adapter_id": "mi-lsp-baseline-v2", "workload_id": "affected-direct", "case_id": "affected-direct", "operation": "affected"},
    "current-path-shortest": {"adapter_id": "mi-lsp-current-v2", "workload_id": "path-shortest", "case_id": "path-shortest", "operation": "path"},
}
_CANONICAL_GRAPHIFY_PAIRS: dict[str, tuple[str, str]] = {
    "callers-direct": ("current-callers-direct", "graphify-callers-direct"),
    "callers-transitive": ("current-callers-transitive", "graphify-callers-transitive"),
}
_CANONICAL_HOTPATH_PAIR = ("current-affected-direct", "baseline-affected-direct-hotpath")
_COMPARISON_METRICS = ("tokens", "warm_p95", "tree_rss")
_NON_COMPARABLE_SCOPES = frozenset({"incremental", "build", "index"})


def _validate_measurement_contract(manifest: Mapping[str, Any]) -> None:
    """Validate the explicit, finite G9 measurement universe; never infer a Cartesian product."""
    workloads = manifest.get("workloads")
    groups = manifest.get("groups")
    adapters = manifest.get("adapters")
    cases = manifest.get("cases")
    pairs = manifest.get("comparator_pair")
    hotpath_pair = manifest.get("hotpath_pair")
    per_metric = manifest.get("per_metric_comparability")
    thresholds = manifest.get("thresholds")
    if not isinstance(workloads, list) or len(workloads) != len(_CANONICAL_WORKLOADS):
        raise ManifestError("manifest must declare exactly the canonical workloads")
    workload_by_id: dict[str, Mapping[str, Any]] = {}
    for workload in workloads:
        if not isinstance(workload, Mapping) or not isinstance(workload.get("workload_id"), str):
            raise ManifestError("workload must declare workload_id")
        workload_id = str(workload["workload_id"])
        expected = _CANONICAL_WORKLOADS.get(workload_id)
        if expected is None or dict(workload) != expected:
            raise ManifestError(f"workload {workload_id} is not a canonical semantic workload")
        if workload_id in workload_by_id:
            raise ManifestError(f"duplicate workload: {workload_id}")
        workload_by_id[workload_id] = workload
    if set(workload_by_id) != set(_CANONICAL_WORKLOADS):
        raise ManifestError("workload set is not canonical")

    if not isinstance(cases, list):
        raise ManifestError("manifest must declare cases")
    case_by_id = {case.get("id"): case for case in cases if isinstance(case, Mapping)}
    if len(case_by_id) != len(cases):
        raise ManifestError("cases must have unique object ids")
    for workload_id, workload in workload_by_id.items():
        case = case_by_id.get(workload["case_id"])
        if not isinstance(case, Mapping) or case.get("operation") != workload["operation"]:
            raise ManifestError(f"workload {workload_id} is not coherent with its case")
        if workload.get("mode") is not None and case.get("mode") != workload["mode"]:
            raise ManifestError(f"workload {workload_id} mode does not match its case")

    if not isinstance(adapters, list):
        raise ManifestError("manifest must declare adapters")
    adapter_by_id = {item.get("adapter_id"): item for item in adapters if isinstance(item, Mapping)}
    if len(adapter_by_id) != len(adapters):
        raise ManifestError("adapters must have unique object ids")
    if not isinstance(groups, list) or len(groups) != len(_CANONICAL_GROUPS):
        raise ManifestError("authoritative measurement universe must contain exactly 8 canonical groups")
    if {group.get("group_id") for group in groups if isinstance(group, Mapping)} != set(_CANONICAL_GROUPS):
        raise ManifestError("group_id set must be exactly the canonical eight groups")

    group_by_id: dict[str, Mapping[str, Any]] = {}
    seen_tuples: set[tuple[str, str, str]] = set()
    required_group_keys = {"group_id", "adapter_id", "workload_id", "case_id", "operation", "repetitions", "authoritative"}
    for group in groups:
        if not isinstance(group, Mapping) or set(group) != required_group_keys:
            raise ManifestError("groups must use the exact authoritative group schema")
        group_id = group.get("group_id")
        expected = _CANONICAL_GROUPS.get(group_id)
        if expected is None:
            raise ManifestError(f"unknown canonical group: {group_id}")
        for field, expected_value in expected.items():
            if group.get(field) != expected_value:
                raise ManifestError(f"group {group_id} has non-canonical {field}")
        if group.get("repetitions") != 30 or group.get("authoritative") is not True:
            raise ManifestError(f"group {group_id} must be authoritative with exactly 30 repetitions")
        adapter = adapter_by_id.get(group["adapter_id"])
        workload = workload_by_id.get(group["workload_id"])
        case = case_by_id.get(group["case_id"])
        if not isinstance(adapter, Mapping) or not isinstance(workload, Mapping) or not isinstance(case, Mapping):
            raise ManifestError(f"group {group_id} references an unknown adapter, workload, or case")
        if group["operation"] not in adapter.get("comparable_operations", []):
            raise ManifestError(f"group {group_id} uses an operation the adapter did not declare comparable")
        if group["operation"] != workload["operation"] or group["case_id"] != workload["case_id"] or case.get("operation") != group["operation"]:
            raise ManifestError(f"group {group_id} does not match its workload semantics")
        tuple_key = (group["adapter_id"], group["case_id"], group["operation"])
        if tuple_key in seen_tuples:
            raise ManifestError("measurement groups must be explicit and unique; Cartesian duplicates are forbidden")
        seen_tuples.add(tuple_key)
        group_by_id[group_id] = group

    if not isinstance(pairs, Mapping) or set(pairs) != set(_CANONICAL_GRAPHIFY_PAIRS):
        raise ManifestError("comparator_pair must declare exactly the two canonical callers pairs")
    for pair_id, (current_id, graphify_id) in _CANONICAL_GRAPHIFY_PAIRS.items():
        pair = pairs[pair_id]
        if not isinstance(pair, Mapping) or set(pair) != {"current", "graphify", "metrics"}:
            raise ManifestError(f"comparator pair {pair_id} schema is not exact")
        if pair["current"] != current_id or pair["graphify"] != graphify_id or pair["metrics"] != list(_COMPARISON_METRICS):
            raise ManifestError(f"comparator pair {pair_id} is not reciprocal/coherent with its canonical groups")
        current = group_by_id[current_id]
        graphify = group_by_id[graphify_id]
        current_adapter = adapter_by_id[current["adapter_id"]]
        graphify_adapter = adapter_by_id[graphify["adapter_id"]]
        if current_adapter.get("kind") != "current" or graphify_adapter.get("kind") != "graphify":
            raise ManifestError(f"Graphify pair {pair_id} must be current versus Graphify")
        if current["operation"] != graphify["operation"] or current["operation"] != "callers" or current["workload_id"] != graphify["workload_id"] or current["case_id"] != graphify["case_id"]:
            raise ManifestError(f"Graphify pair {pair_id} must compare the same callers semantic workload")
        if workload_by_id[current["workload_id"]].get("mode") != workload_by_id[graphify["workload_id"]].get("mode"):
            raise ManifestError(f"Graphify pair {pair_id} must compare the same callers mode")

    if not isinstance(hotpath_pair, Mapping) or set(hotpath_pair) != {"current", "baseline", "metrics"}:
        raise ManifestError("hotpath_pair must declare current affected-direct versus baseline affected-direct")
    if hotpath_pair["current"] != _CANONICAL_HOTPATH_PAIR[0] or hotpath_pair["baseline"] != _CANONICAL_HOTPATH_PAIR[1] or hotpath_pair["metrics"] != ["warm_p95"]:
        raise ManifestError("hotpath_pair is not the canonical affected-direct pair")
    hot_current = group_by_id[_CANONICAL_HOTPATH_PAIR[0]]
    hot_baseline = group_by_id[_CANONICAL_HOTPATH_PAIR[1]]
    if hot_current["operation"] != hot_baseline["operation"] or hot_current["operation"] != "affected" or hot_current["workload_id"] != hot_baseline["workload_id"] or hot_current["case_id"] != hot_baseline["case_id"]:
        raise ManifestError("hotpath_pair must compare the same affected-direct semantic workload")
    if adapter_by_id[hot_current["adapter_id"]].get("kind") != "current" or adapter_by_id[hot_baseline["adapter_id"]].get("kind") != "baseline":
        raise ManifestError("hotpath_pair must use current and baseline adapters")

    expected_per_metric = {metric: list(_CANONICAL_GRAPHIFY_PAIRS) for metric in _COMPARISON_METRICS}
    if per_metric != expected_per_metric:
        raise ManifestError("per_metric_comparability must cover exactly the canonical Graphify pairs")

    if not isinstance(thresholds, Mapping):
        raise ManifestError("manifest must declare thresholds")
    ratios = thresholds.get("current_vs_graphify")
    if not isinstance(ratios, Mapping) or ratios.get("tokens") != 0.70 or ratios.get("warm_p95") != 0.80 or ratios.get("tree_rss") != 0.50:
        raise ManifestError("thresholds must declare current-vs-Graphify targets .70/.80/.50")
    hotpath = thresholds.get("hotpath")
    if not isinstance(hotpath, Mapping) or hotpath.get("current_p95_multiplier") != 1.10 or hotpath.get("baseline_p95_additive_ms") != 25:
        raise ManifestError("thresholds must declare the hotpath 1.10x + 25ms guard")

    scopes = manifest.get("scopes")
    if not isinstance(scopes, list) or {item.get("scope") for item in scopes if isinstance(item, Mapping)} != _NON_COMPARABLE_SCOPES or len(scopes) != len(_NON_COMPARABLE_SCOPES):
        raise ManifestError("incremental, build, and index must be the complete explicit scope set")
    for item in scopes:
        if not isinstance(item, Mapping) or set(item) != {"scope", "status", "reason"} or item.get("status") != "NOT_COMPARABLE" or not isinstance(item.get("reason"), str) or not item["reason"].strip():
            raise ManifestError("unavailable scopes must remain NOT_COMPARABLE with a reason")


def validate_manifest(manifest: Mapping[str, Any], root: Path | None = None, *, check_files: bool = True) -> dict[str, Any]:
    if manifest.get("schema") != MANIFEST_SCHEMA:
        raise ManifestError("unsupported manifest schema")
    if manifest.get("version") != 2:
        raise ManifestError("manifest version must be 2")
    if manifest.get("baseline_commit") != BASELINE_COMMIT:
        raise ManifestError("baseline commit pin mismatch")
    graphify = manifest.get("graphify", {})
    if graphify.get("commit") != GRAPHIFY_COMMIT or graphify.get("version") != GRAPHIFY_VERSION:
        raise ManifestError("Graphify pin mismatch")
    current_commit = manifest.get("current", {}).get("commit")
    if not isinstance(current_commit, str) or not re.fullmatch(r"[0-9a-f]{40}", current_commit):
        raise ManifestError("current commit must be a full manifest pin")
    if not isinstance(manifest.get("default_repetitions"), int) or manifest["default_repetitions"] != 30:
        raise ManifestError("authoritative repetitions must be exactly 30")
    _require_sha_map(manifest.get("fixture_hashes"), "fixture_hashes")
    _require_sha_map(manifest.get("oracle_hashes"), "oracle_hashes")
    adapters = manifest.get("adapters")
    if not isinstance(adapters, list) or not adapters:
        raise ManifestError("manifest must declare adapters")
    ids = set()
    specs: list[AdapterSpec] = []
    for raw in adapters:
        if raw.get("schema") != ADAPTER_SCHEMA:
            raise ManifestError("adapter schema missing")
        try:
            spec = AdapterSpec.from_dict(raw)
        except SchemaError as exc:
            raise ManifestError(f"invalid adapter specification: {exc}") from exc
        if spec.adapter_id in ids:
            raise ManifestError(f"duplicate adapter: {spec.adapter_id}")
        ids.add(spec.adapter_id)
        specs.append(spec)
    by_kind = {kind: [spec for spec in specs if spec.kind == kind] for kind in ("current", "baseline", "graphify")}
    for kind in ("current", "baseline"):
        if len(by_kind[kind]) != 1:
            raise ManifestError(f"manifest must contain exactly one {kind} adapter")
    if by_kind["current"][0].expected_commit != manifest["current"]["commit"]:
        raise ManifestError("current commit pin is not linked to current adapter")
    if by_kind["baseline"][0].expected_commit != manifest["baseline_commit"]:
        raise ManifestError("baseline commit pin is not linked to baseline adapter")
    if manifest.get("provenance_contract") == "victory-build-attestation/v2":
        for spec in specs:
            if not spec.attestation_path or not spec.expected_attestation_sha256:
                raise ManifestError(f"{spec.adapter_id} lacks manifested build attestation")
            attestation = Path(spec.attestation_path)
            if attestation.is_absolute() or ".." in attestation.parts:
                raise ManifestError(f"{spec.adapter_id} has unsafe attestation path")
            if check_files:
                if root is None:
                    raise ManifestError("root is required for build attestation checks")
                try:
                    validate_attestation(
                        root / attestation,
                        expected_commit=spec.expected_commit,
                        expected_executable_sha256=spec.expected_executable_sha256,
                        expected_source_sha256=spec.expected_source_sha256,
                        expected_file_sha256=spec.expected_attestation_sha256,
                        expected_kind=spec.kind,
                        expected_build_command=spec.build_command,
                        expected_toolchain=spec.toolchain,
                        expected_package_provenance=spec.package_provenance,
                        expected_runtime_role=spec.runtime_role or None,
                        expected_interpreter_sha256=spec.interpreter_sha256,
                        expected_module_name=spec.module_name or "graphify",
                        expected_distribution_name=spec.distribution_name or "graphifyy",
                        expected_source_package_sha256=spec.expected_source_package_sha256,
                        expected_metadata_pyproject_sha256=spec.expected_metadata_pyproject_sha256,
                        expected_version=spec.expected_version,
                    )
                except AttestationError as exc:
                    raise ManifestError(f"invalid build attestation for {spec.adapter_id}: {exc}") from exc
    cases = manifest.get("cases")
    if not isinstance(cases, list) or not cases:
        raise ManifestError("manifest must declare cases")
    case_ids = set()
    for case in cases:
        _validate_operation_case(case)
        if case["id"] in case_ids:
            raise ManifestError(f"duplicate case: {case['id']}")
        case_ids.add(case["id"])
    if manifest.get("provenance_contract") == "victory-build-attestation/v2" or any(key in manifest for key in ("workloads", "groups", "comparator_pair", "per_metric_comparability")):
        _validate_measurement_contract(manifest)
    if check_files:
        if root is None:
            raise ManifestError("root is required when check_files=true")
        inventory_path = root / "goldens" / "inventory.json"
        if not inventory_path.is_file():
            raise ManifestError("recursive raw-byte inventory is missing")
        try:
            validate_inventory(root, json.loads(inventory_path.read_text(encoding="utf-8")))
        except (OSError, json.JSONDecodeError) as exc:
            raise ManifestError(f"cannot read inventory: {exc}") from exc
        for group in ("fixture_hashes", "oracle_hashes"):
            for rel, expected in manifest[group].items():
                path = root / rel
                if not path.is_file():
                    raise ManifestError(f"missing {group} file: {rel}")
                actual = canonical_file_hash(path)
                if actual != expected:
                    raise ManifestError(f"{group} mismatch: {rel}")
    return dict(manifest)


def load_manifest(path: str | Path, *, check_files: bool = True) -> dict[str, Any]:
    manifest_path = Path(path).resolve()
    try:
        value = json.loads(manifest_path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as exc:
        raise ManifestError(f"cannot read manifest: {exc}") from exc
    return validate_manifest(value, manifest_path.parent, check_files=check_files)


def manifest_adapters(manifest: Mapping[str, Any]) -> dict[str, AdapterSpec]:
    return {spec.adapter_id: spec for spec in (AdapterSpec.from_dict(raw) for raw in manifest["adapters"])}


def validate_current_metadata(
    metadata: Mapping[str, Any],
    manifest: Mapping[str, Any],
    *,
    expected_commit: str | None = None,
    expected_version: str | None = None,
    expected_executable_sha256: str | None = None,
    require_observed: bool = True,
) -> str:
    """Require observed adapter provenance; pins are not observations."""
    expected = expected_commit or manifest["current"]["commit"]
    item = metadata.get("items", [{}])[0] if isinstance(metadata.get("items"), list) else metadata
    if not isinstance(item, Mapping):
        raise ManifestError("provenance metadata item must be an object")
    observed = item.get("vcs_revision") or item.get("commit") or item.get("revision")
    observed_executable = item.get("executable_sha256")
    if expected_executable_sha256 and observed_executable and observed_executable != expected_executable_sha256:
        raise ManifestError("CLI executable digest mismatch")
    if require_observed and not observed and not (expected_executable_sha256 and observed_executable == expected_executable_sha256):
        raise ManifestError("observed CLI provenance is missing")
    if observed and observed != expected:
        raise ManifestError(f"CLI revision mismatch: {observed} != {expected}")
    if expected_version:
        observed_version = item.get("version")
        if require_observed and not observed_version:
            raise ManifestError("observed CLI version is missing")
        if observed_version and observed_version != expected_version:
            raise ManifestError(f"CLI version mismatch: {observed_version} != {expected_version}")
    return observed or expected


def git_revision(source: Path) -> str:
    try:
        completed = subprocess.run(
            ["git", "-C", str(source), "rev-parse", "HEAD"], capture_output=True, text=True,
            check=False, timeout=10, env={"PATH": os.environ.get("PATH", "")},
        )
    except (OSError, subprocess.TimeoutExpired) as exc:
        raise ManifestError(f"cannot read source revision: {exc}") from exc
    revision = completed.stdout.strip()
    if completed.returncode or not re.fullmatch(r"[0-9a-f]{40}", revision):
        raise ManifestError("source revision unavailable")
    return revision
