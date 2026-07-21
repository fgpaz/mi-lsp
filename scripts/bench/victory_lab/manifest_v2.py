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
