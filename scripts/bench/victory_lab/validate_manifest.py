"""Validate Victory Lab v2 manifest pins, hashes, and adapter contracts."""
from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path
from typing import Any, Mapping

try:
    from .manifest_v2 import ManifestError, _validate_measurement_contract, load_manifest, manifest_adapters, resolve_configured_path, sha256_file, git_revision
    from .attestation_v2 import source_artifact_digest, validate_attestation
except ImportError:  # pragma: no cover
    from manifest_v2 import ManifestError, _validate_measurement_contract, load_manifest, manifest_adapters, resolve_configured_path, sha256_file, git_revision
    from attestation_v2 import source_artifact_digest, validate_attestation


def validate_strict_manifest(manifest: Mapping[str, Any], root: Path | None = None, *, check_files: bool = True) -> dict[str, Any]:
    """Apply the v2 evidence gates that a permissive loader cannot omit."""
    if manifest.get("provenance_contract") == "victory-build-attestation/v2" or any(key in manifest for key in ("workloads", "groups", "comparator_pair", "per_metric_comparability")):
        _validate_measurement_contract(manifest)
    adapters = manifest_adapters(manifest)
    if not adapters or any(not spec.comparable_operations for spec in adapters.values()):
        raise ManifestError("manifest has no usable comparator")
    if manifest.get("provenance_contract") == "victory-build-attestation/v2":
        try:
            from .attestation_v2 import AttestationError, validate_attestation
        except ImportError:
            from attestation_v2 import AttestationError, validate_attestation
        for spec in adapters.values():
            if not spec.attestation_path or not spec.expected_attestation_sha256:
                raise ManifestError(f"missing manifested build attestation: {spec.adapter_id}")
            attestation = Path(spec.attestation_path)
            if attestation.is_absolute() or ".." in attestation.parts:
                raise ManifestError(f"unsafe build attestation path: {spec.adapter_id}")
            if check_files:
                if root is None:
                    raise ManifestError("root is required when checking build attestations")
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
                        expected_version=spec.expected_version,
                    )
                except AttestationError as exc:
                    raise ManifestError(f"invalid build attestation: {spec.adapter_id}") from exc
    comparable = {operation for spec in adapters.values() for operation in spec.comparable_operations}
    fixture_hashes = manifest.get("fixture_hashes")
    oracle_hashes = manifest.get("oracle_hashes")
    oracles = manifest.get("oracles")
    if not isinstance(fixture_hashes, Mapping) or not isinstance(oracle_hashes, Mapping) or not isinstance(oracles, Mapping):
        raise ManifestError("manifest evidence maps are required")
    case_ids: set[str] = set()
    for case in manifest.get("cases", []):
        if not isinstance(case, Mapping):
            raise ManifestError("case must be an object")
        case_id = case.get("id")
        if not isinstance(case_id, str) or not case_id or case_id in case_ids:
            raise ManifestError("case id is missing or duplicated")
        operation = case.get("operation")
        if operation not in comparable:
            raise ManifestError(f"no comparable adapter for operation: {operation}")
        golden = case.get("golden")
        golden_path = Path(golden) if isinstance(golden, str) else Path("")
        if not isinstance(golden, str) or not golden or golden_path.is_absolute() or ".." in golden_path.parts:
            raise ManifestError(f"case {case_id} has unsafe golden")
        golden = golden.replace("\\", "/")
        if golden not in oracle_hashes or case_id not in oracles:
            raise ManifestError(f"case {case_id} golden/oracle is not pinned")
        corpus = case.get("corpus")
        if not isinstance(corpus, list) or not corpus:
            raise ManifestError(f"case {case_id} lacks corpus")
        for raw in corpus:
            corpus_path = Path(raw) if isinstance(raw, str) else Path("")
            prefix = raw.replace("\\", "/").rstrip("/") + "/" if isinstance(raw, str) else ""
            if corpus_path.is_absolute() or ".." in corpus_path.parts or not any(str(path).replace("\\", "/").startswith(prefix) for path in fixture_hashes):
                raise ManifestError(f"case {case_id} corpus is not safely pinned")
        case_ids.add(case_id)
    for section, required in (("relation_cases", "golden"), ("mutation_cases", "golden")):
        metadata = manifest.get(section)
        if not isinstance(metadata, Mapping) or not isinstance(metadata.get(required), str):
            raise ManifestError(f"{section} metadata is required")
        golden = metadata[required].replace("\\", "/")
        if golden not in oracle_hashes:
            raise ManifestError(f"{section} golden is not pinned")
    thresholds = manifest.get("thresholds")
    if not isinstance(thresholds, Mapping) or thresholds.get("per_slice") is not True:
        raise ManifestError("per-slice thresholds are required")
    if check_files:
        if root is None:
            raise ManifestError("root is required when checking files")
        for group in (fixture_hashes, oracle_hashes):
            for relative, expected in group.items():
                path = root / str(relative)
                if path.is_symlink() or not path.is_file() or sha256_file(path) != expected:
                    raise ManifestError(f"protected evidence mismatch: {relative}")
    return dict(manifest)


def validate_runtime(manifest: dict, *, require_runtime: bool = False, manifest_root: Path | None = None) -> list[str]:
    blockers: list[str] = []
    attestation_root = manifest_root or Path(__file__).resolve().parents[3] / "benchmarks" / "victory-lab" / "v2"
    for spec in manifest_adapters(manifest).values():
        try:
            executable = resolve_configured_path("graphify_python" if spec.kind == "graphify" else spec.kind, spec.executable or None)
            if not executable.is_file():
                raise ManifestError(f"missing executable: {spec.adapter_id}")
            if spec.kind == "graphify":
                if sha256_file(executable) != spec.interpreter_sha256:
                    raise ManifestError(f"interpreter sha mismatch: {spec.adapter_id}")
            elif spec.expected_executable_sha256 and sha256_file(executable) != spec.expected_executable_sha256:
                raise ManifestError(f"executable sha mismatch: {spec.adapter_id}")
            source_name = "graphify_source" if spec.kind == "graphify" else f"{spec.kind}_source"
            source = resolve_configured_path(source_name, spec.source or None)
            if not source.is_dir() or git_revision(source) != spec.expected_commit:
                raise ManifestError(f"source pin mismatch: {spec.adapter_id}")
            if spec.expected_source_sha256 and source_artifact_digest(source, spec.expected_commit) != spec.expected_source_sha256:
                raise ManifestError(f"source artifact digest mismatch: {spec.adapter_id}")
            validate_attestation(
                Path(spec.attestation_path) if Path(spec.attestation_path).is_absolute() else attestation_root / spec.attestation_path,
                expected_commit=spec.expected_commit,
                expected_executable_sha256=spec.expected_executable_sha256,
                expected_source_sha256=spec.expected_source_sha256,
                source_root=source,
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
                require_runtime=require_runtime,
                interpreter_path=executable,
            )
        except (ManifestError, ValueError, OSError) as exc:
            if require_runtime or "provenance" in str(exc).lower() or "pin" in str(exc).lower() or "sha" in str(exc).lower():
                blockers.append(f"{spec.adapter_id}: {exc}")
    return blockers


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--manifest", required=True)
    parser.add_argument("--skip-files", action="store_true")
    parser.add_argument("--require-runtime", action="store_true")
    args = parser.parse_args(argv)
    try:
        manifest = load_manifest(args.manifest, check_files=not args.skip_files)
        validate_strict_manifest(manifest, Path(args.manifest).resolve().parent, check_files=not args.skip_files)
        blockers = validate_runtime(manifest, require_runtime=args.require_runtime, manifest_root=Path(args.manifest).resolve().parent)
        if blockers:
            raise ManifestError("; ".join(blockers))
    except (ManifestError, OSError, ValueError) as exc:
        print(json.dumps({"schema": "victory-manifest-validation/v2", "status": "BLOCKED", "error": str(exc)}, sort_keys=True))
        return 2
    print(json.dumps({"schema": "victory-manifest-validation/v2", "status": "PASS", "manifest": str(Path(args.manifest).resolve())}, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
