"""Validate Victory Lab v2 manifest pins, hashes, and adapter contracts."""
from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

try:
    from .manifest_v2 import ManifestError, load_manifest, manifest_adapters, resolve_configured_path, sha256_file, git_revision
except ImportError:  # pragma: no cover
    from manifest_v2 import ManifestError, load_manifest, manifest_adapters, resolve_configured_path, sha256_file, git_revision


def validate_runtime(manifest: dict, *, require_runtime: bool = False) -> list[str]:
    blockers: list[str] = []
    for spec in manifest_adapters(manifest).values():
        try:
            executable = resolve_configured_path("graphify_python" if spec.kind == "graphify" else spec.kind, spec.executable or None)
            if not executable.is_file():
                raise ManifestError(f"missing executable: {executable}")
            if spec.expected_executable_sha256 and sha256_file(executable) != spec.expected_executable_sha256:
                raise ManifestError(f"executable sha mismatch: {spec.adapter_id}")
            if spec.kind == "graphify":
                source = resolve_configured_path("graphify_source", spec.source or None)
                if not source.is_dir() or git_revision(source) != spec.expected_commit:
                    raise ManifestError(f"Graphify source pin mismatch: {spec.adapter_id}")
                if spec.source_digest_path and spec.expected_source_sha256 and sha256_file(source / spec.source_digest_path) != spec.expected_source_sha256:
                    raise ManifestError(f"Graphify source digest mismatch: {spec.adapter_id}")
        except ManifestError as exc:
            if require_runtime:
                blockers.append(str(exc))
    return blockers


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--manifest", required=True)
    parser.add_argument("--skip-files", action="store_true")
    parser.add_argument("--require-runtime", action="store_true")
    args = parser.parse_args(argv)
    try:
        manifest = load_manifest(args.manifest, check_files=not args.skip_files)
        blockers = validate_runtime(manifest, require_runtime=args.require_runtime)
        if blockers:
            raise ManifestError("; ".join(blockers))
    except (ManifestError, OSError, ValueError) as exc:
        print(json.dumps({"schema": "victory-manifest-validation/v2", "status": "BLOCKED", "error": str(exc)}, sort_keys=True))
        return 2
    print(json.dumps({"schema": "victory-manifest-validation/v2", "status": "PASS", "manifest": str(Path(args.manifest).resolve())}, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
