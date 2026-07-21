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
    from .schema_v2 import ADAPTER_SCHEMA, MANIFEST_SCHEMA, OPERATIONS, AdapterSpec, SchemaError
except ImportError:  # pragma: no cover - direct script compatibility
    from schema_v2 import ADAPTER_SCHEMA, MANIFEST_SCHEMA, OPERATIONS, AdapterSpec, SchemaError

BASELINE_COMMIT = "a251ab1f8db4e96f029926fbef275b078a20a111"
GRAPHIFY_COMMIT = "9bf14a4931658152969586ace39eb965c010f0d1"
GRAPHIFY_VERSION = "0.9.19"
DEFAULT_PATHS = {
    "current": r"C:\tmp\milsp-g9-bin\mi-lsp-current.exe",
    "baseline": r"C:\tmp\milsp-g9-bin\mi-lsp-baseline.exe",
    "graphify_source": r"C:\tmp\pi-github-repos\Graphify-Labs\graphify",
    "graphify_python": r"C:\tmp\graphify-bench-venv\Scripts\python.exe",
}
_PATH_ENV = {
    "current": "VICTORY_LAB_CURRENT_EXE",
    "baseline": "VICTORY_LAB_BASELINE_EXE",
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


def resolve_configured_path(name: str, value: str | None = None) -> Path:
    if name not in DEFAULT_PATHS:
        raise ManifestError(f"unknown configurable path: {name}")
    configured = value or os.environ.get(_PATH_ENV[name]) or DEFAULT_PATHS[name]
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
) -> str:
    """Require adapter provenance from CLI when present, otherwise its manifest pin."""
    expected = expected_commit or manifest["current"]["commit"]
    item = metadata.get("items", [{}])[0] if isinstance(metadata.get("items"), list) else metadata
    if not isinstance(item, Mapping):
        raise ManifestError("provenance metadata item must be an object")
    observed = item.get("vcs_revision") or item.get("commit") or item.get("revision")
    if observed and observed != expected:
        raise ManifestError(f"CLI revision mismatch: {observed} != {expected}")
    if expected_version:
        observed_version = item.get("version")
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
