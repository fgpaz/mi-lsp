"""Reproducible source and runtime attestations for Victory Lab v2."""
from __future__ import annotations

import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import tempfile
from typing import Any, Mapping, Sequence

ATTESTATION_SCHEMA = "victory-build-attestation/v2"
_SOURCE_RECIPE = "git-ls-tree-v1"
_GO_RECIPE = ("go", "build", "-trimpath", "-buildvcs=false", "-o", "{output}", "./cmd/mi-lsp")
_GRAPHIFY_ROLE = "interpreter_plus_source_pythonpath"
_HEX40 = re.compile(r"^[0-9a-f]{40}$")
_HEX64 = re.compile(r"^[0-9a-f]{64}$")
_TARGET_RE = re.compile(r"^[a-z0-9_-]+/[a-z0-9_-]+$")


class AttestationError(ValueError):
    """The attestation is absent, malformed, or not reproducible."""


def canonical_attestation(value: Mapping[str, Any]) -> bytes:
    return (json.dumps(dict(value), sort_keys=True, separators=(",", ":")) + "\n").encode("utf-8")


def _git(source: Path, args: Sequence[str], *, text: bool = True, timeout: float = 20.0) -> subprocess.CompletedProcess:
    try:
        return subprocess.run(
            ["git", "-C", str(source), *args], capture_output=True, text=text, check=False,
            timeout=timeout, env={"PATH": os.environ.get("PATH", "")},
        )
    except (OSError, subprocess.TimeoutExpired) as exc:
        raise AttestationError("source git operation unavailable") from exc


def _git_tree_bytes(source: Path, commit: str, paths: Sequence[str]) -> bytes:
    if not _HEX40.fullmatch(commit) or not source.is_dir():
        raise AttestationError("source tree pin is malformed")
    completed = _git(source, ["ls-tree", "-r", "--full-tree", commit, "--", *paths], text=False)
    if completed.returncode != 0 or not completed.stdout:
        raise AttestationError("source tree listing unavailable")
    return completed.stdout.replace(b"\r\n", b"\n")


def source_artifact_digest(source: str | Path, commit: str) -> str:
    """Hash the pinned Git tree, never the working directory or ``.git``."""
    root = Path(source)
    return hashlib.sha256(_git_tree_bytes(root, commit, ())).hexdigest()


def source_package_digest(source: str | Path, commit: str) -> str:
    return hashlib.sha256(_git_tree_bytes(Path(source), commit, ("pyproject.toml", "graphify"))).hexdigest()


def build_input_digest(source: str | Path, commit: str, command: Sequence[str], toolchain: Mapping[str, str]) -> str:
    """Digest only Go build inputs plus the normalized recipe and toolchain."""
    tree = _git_tree_bytes(Path(source), commit, ("go.mod", "go.sum", "cmd", "internal"))
    payload = tree + b"\n" + json.dumps(
        {"command": list(command), "toolchain": dict(sorted(toolchain.items()))},
        sort_keys=True, separators=(",", ":"),
    ).encode("utf-8")
    return hashlib.sha256(payload).hexdigest()


def _normalized_command(value: Any, *, expected: Sequence[str] = ()) -> list[str]:
    if not isinstance(value, list) or not value or any(not isinstance(item, str) or not item.strip() for item in value):
        raise AttestationError("executed command must be a non-empty normalized list")
    result = list(value)
    for item in result:
        if re.match(r"^[A-Za-z]:[\\/]", item) or item.startswith(("/", "\\\\")):
            raise AttestationError("attestation command must use portable placeholders")
        if "..\\" in item or "../" in item:
            raise AttestationError("attestation command contains a relative escape")
    if expected and result != list(expected):
        raise AttestationError("attestation recipe mismatch")
    return result


def _normalized_map(value: Any, name: str) -> dict[str, str]:
    if not isinstance(value, Mapping) or any(not isinstance(k, str) or not isinstance(v, str) for k, v in value.items()):
        raise AttestationError(f"{name} must be a string map")
    return dict(sorted(value.items()))


def _digest(value: Any, name: str) -> str:
    if not isinstance(value, str) or not _HEX64.fullmatch(value):
        raise AttestationError(f"{name} must be a lowercase hexadecimal 64-character digest")
    return value


def _commit(value: Any, name: str) -> str:
    if not isinstance(value, str) or not _HEX40.fullmatch(value):
        raise AttestationError(f"{name} must be a lowercase hexadecimal 40-character commit")
    return value


def _target(value: Any) -> str:
    if not isinstance(value, str) or not _TARGET_RE.fullmatch(value):
        raise AttestationError("target must be a normalized OS/architecture pair")
    return value


def _head(source: Path) -> str:
    result = _git(source, ["rev-parse", "HEAD"])
    value = result.stdout.strip()
    if result.returncode != 0 or not _HEX40.fullmatch(value):
        raise AttestationError("source checkout HEAD is unavailable")
    return value


def _source_clean_for_go(source: Path) -> None:
    result = _git(source, ["status", "--porcelain", "--untracked-files=all", "--", "go.mod", "go.sum", "cmd", "internal"])
    if result.returncode != 0:
        raise AttestationError("source build-input status is unavailable")
    if result.stdout.strip():
        raise AttestationError("build-relevant Go source is dirty")


def _effective_go_toolchain(toolchain: Mapping[str, str]) -> dict[str, str]:
    try:
        version = subprocess.run(["go", "version"], capture_output=True, text=True, check=False, timeout=20)
        env = subprocess.run(["go", "env", "GOOS", "GOARCH"], capture_output=True, text=True, check=False, timeout=20)
    except (OSError, subprocess.TimeoutExpired) as exc:
        raise AttestationError("Go toolchain is unavailable") from exc
    if version.returncode != 0 or env.returncode != 0:
        raise AttestationError("Go toolchain probe failed")
    tokens = version.stdout.strip().split()
    go_version = next((token for token in tokens[2:] if token.startswith("go")), "")
    if not go_version:
        raise AttestationError("Go toolchain version is unavailable")
    target_lines = env.stdout.splitlines()
    effective = {"go": go_version, "target": "/".join(target_lines[:2]) if len(target_lines) >= 2 else ""}
    expected = dict(sorted((str(k), str(v)) for k, v in toolchain.items()))
    if effective != expected:
        raise AttestationError("effective Go toolchain does not match attestation")
    return effective


def _sanitized_log_digest(stdout: bytes | str, stderr: bytes | str) -> str:
    text = (stdout.decode("utf-8", "replace") if isinstance(stdout, bytes) else str(stdout))
    text += "\n" + (stderr.decode("utf-8", "replace") if isinstance(stderr, bytes) else str(stderr))
    text = re.sub(r"(?i)[A-Z]:[\\/][^\r\n ]+", "<PATH>", text)
    text = re.sub(r"(?i)(?:^|[\s])/(?:[^\s]+)", " <PATH>", text)
    text = re.sub(r"\b\d{4}-\d{2}-\d{2}[T ][^\s]+", "<TIME>", text)
    return hashlib.sha256(text.encode("utf-8", "replace")).hexdigest()


def _run_go_reproduction(source: Path, commit: str, expected_sha: str, command: Sequence[str], toolchain: Mapping[str, str]) -> dict[str, Any]:
    if tuple(command) != _GO_RECIPE:
        raise AttestationError("Go attestation must use the deterministic build recipe")
    if _head(source) != commit:
        raise AttestationError("source checkout HEAD mismatch")
    _source_clean_for_go(source)
    effective = _effective_go_toolchain(toolchain)
    before = build_input_digest(source, commit, command, effective)
    with tempfile.TemporaryDirectory(prefix="victory-go-reproduce-") as raw:
        output = Path(raw) / "mi-lsp.exe"
        actual_command = [str(output) if item == "{output}" else item for item in command]
        env = dict(os.environ)
        target_os, target_arch = effective["target"].split("/", 1)
        env.update({"GOOS": target_os, "GOARCH": target_arch})
        try:
            completed = subprocess.run(actual_command, cwd=str(source), env=env, capture_output=True, check=False, timeout=300)
        except (OSError, subprocess.TimeoutExpired) as exc:
            raise AttestationError("reproducible Go build did not complete") from exc
        if completed.returncode != 0 or not output.is_file():
            raise AttestationError("reproducible Go build failed")
        produced = hashlib.sha256(output.read_bytes()).hexdigest()
        after = build_input_digest(source, commit, command, effective)
        if before != after:
            raise AttestationError("Go build inputs changed during reproduction")
    return {
        "status": "reproduced" if produced == expected_sha else "mismatch",
        "executed_command": list(command),
        "source_head": commit,
        "toolchain": effective,
        "target": effective["target"],
        "build_input_digest": before,
        "sanitized_log_digest": _sanitized_log_digest(completed.stdout, completed.stderr),
        "produced_sha256": produced,
        "matches_expected": produced == expected_sha,
    }


def _verify_pyproject(source: Path, *, module_name: str, distribution_name: str, version: str) -> str:
    pyproject = source / "pyproject.toml"
    package = source / module_name
    if not pyproject.is_file() or not package.is_dir():
        raise AttestationError("Graphify source metadata or package is missing")
    try:
        import tomllib
        metadata = tomllib.loads(pyproject.read_text(encoding="utf-8"))
    except (OSError, ValueError, ModuleNotFoundError) as exc:
        raise AttestationError("Graphify pyproject metadata cannot be verified") from exc
    project = metadata.get("project")
    if not isinstance(project, Mapping) or project.get("name") != distribution_name or project.get("version") != version:
        raise AttestationError("Graphify pyproject metadata mismatch")
    packages = metadata.get("tool", {}).get("setuptools", {}).get("packages", [])
    if not isinstance(packages, list) or module_name not in packages:
        raise AttestationError("Graphify pyproject does not package the pinned module")
    return hashlib.sha256(pyproject.read_bytes()).hexdigest()


def graphify_runtime_command() -> list[str]:
    return [
        "{interpreter}", "-c",
        "import graphify,json,pathlib,tomllib; p=pathlib.Path(graphify.__file__).parents[1]/'pyproject.toml'; v=getattr(graphify,'__version__',None) or tomllib.loads(p.read_text())['project']['version']; print(json.dumps({'module_file':graphify.__file__,'version':v,'distribution':'graphifyy'},sort_keys=True))",
    ]


def probe_graphify_runtime(
    interpreter: str | Path, source: str | Path, *, expected_commit: str, module_name: str = "graphify",
    distribution_name: str = "graphifyy", expected_version: str = "0.9.19", toolchain: Mapping[str, str] | None = None,
) -> dict[str, Any]:
    root = Path(source).resolve()
    interpreter_path = Path(interpreter).resolve()
    if _head(root) != expected_commit:
        raise AttestationError("Graphify source HEAD mismatch")
    metadata_sha = _verify_pyproject(root, module_name=module_name, distribution_name=distribution_name, version=expected_version)
    if not interpreter_path.is_file():
        raise AttestationError("Graphify interpreter is missing")
    expected_toolchain = dict(sorted((str(k), str(v)) for k, v in (toolchain or {}).items()))
    try:
        version_probe = subprocess.run([str(interpreter_path), "--version"], capture_output=True, text=True, check=False, timeout=20)
    except (OSError, subprocess.TimeoutExpired) as exc:
        raise AttestationError("Graphify interpreter probe failed") from exc
    if version_probe.returncode != 0 or expected_toolchain.get("python") and expected_toolchain["python"] not in version_probe.stdout:
        raise AttestationError("Graphify interpreter toolchain mismatch")
    command = graphify_runtime_command()
    actual = [str(interpreter_path) if item == "{interpreter}" else item for item in command]
    env = dict(os.environ)
    env["PYTHONPATH"] = str(root)
    try:
        completed = subprocess.run(actual, cwd=str(root), env=env, capture_output=True, text=True, check=False, timeout=30)
    except (OSError, subprocess.TimeoutExpired) as exc:
        raise AttestationError("Graphify runtime probe did not complete") from exc
    if completed.returncode != 0 or len(completed.stdout.splitlines()) != 1 or completed.stderr.strip():
        raise AttestationError("Graphify runtime probe was not sanitized JSON")
    try:
        observed = json.loads(completed.stdout)
    except json.JSONDecodeError as exc:
        raise AttestationError("Graphify runtime probe returned invalid JSON") from exc
    if not isinstance(observed, Mapping) or set(observed) != {"distribution", "module_file", "version"}:
        raise AttestationError("Graphify runtime metadata schema drift")
    module_file = Path(str(observed.get("module_file"))).resolve()
    try:
        module_file.relative_to(root)
    except ValueError as exc:
        raise AttestationError("Graphify module file is outside the pinned source") from exc
    expected_file = root / module_name / "__init__.py"
    if module_file != expected_file.resolve():
        raise AttestationError("Graphify runtime imported an unexpected module file")
    if observed.get("distribution") != distribution_name:
        raise AttestationError("Graphify distribution name mismatch")
    return {
        "status": "reproduced",
        "executed_command": command,
        "source_head": expected_commit,
        "module_file": f"{module_name}/__init__.py",
        "module_file_under_source": True,
        "module_name": module_name,
        "distribution_name": distribution_name,
        "version": expected_version,
        "metadata_pyproject_sha256": metadata_sha,
        "sanitized_log_digest": _sanitized_log_digest(json.dumps({"module_file": f"{module_name}/__init__.py", "version": observed.get("version"), "distribution": observed.get("distribution")}, sort_keys=True), ""),
        "observed_version": observed.get("version"),
    }


def validate_attestation(
    path: str | Path, *, expected_commit: str, expected_executable_sha256: str = "", expected_source_sha256: str = "",
    expected_file_sha256: str = "", source_root: str | Path | None = None, expected_kind: str | None = None,
    expected_build_command: Sequence[str] = (), expected_toolchain: Mapping[str, str] | None = None,
    expected_package_provenance: Mapping[str, str] | None = None, expected_runtime_role: str | None = None,
    expected_interpreter_sha256: str = "", expected_module_name: str = "graphify", expected_distribution_name: str = "graphifyy",
    expected_source_package_sha256: str = "", expected_metadata_pyproject_sha256: str = "", expected_version: str = "",
    require_runtime: bool = False, interpreter_path: str | Path | None = None,
) -> dict[str, Any]:
    attestation_path = Path(path)
    try:
        value = json.loads(attestation_path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as exc:
        raise AttestationError("build attestation cannot be read") from exc
    if not isinstance(value, Mapping) or value.get("schema") != ATTESTATION_SCHEMA:
        raise AttestationError("claim-only or unsupported build attestation schema")
    if expected_kind is not None and value.get("adapter_kind") != expected_kind:
        raise AttestationError("adapter kind mismatch")
    if value.get("source_digest_recipe") != _SOURCE_RECIPE:
        raise AttestationError("unsupported source digest recipe")
    commit = _commit(value.get("source_tree_commit"), "source_tree_commit")
    source_sha = _digest(value.get("source_artifact_digest"), "source artifact digest")
    if commit != expected_commit or source_sha != expected_source_sha256:
        raise AttestationError("source pin mismatch")
    toolchain = _normalized_map(value.get("toolchain"), "toolchain")
    if expected_toolchain is not None and toolchain != dict(sorted(expected_toolchain.items())):
        raise AttestationError("toolchain provenance mismatch")

    if value.get("adapter_kind") in {"current", "baseline"}:
        required = {"adapter_kind", "build_command", "build_execution", "executable_sha256", "package_provenance", "reproducible", "schema", "source_artifact_digest", "source_digest_recipe", "source_tree_commit", "toolchain"}
        if set(value) != required or value.get("reproducible") is not True:
            raise AttestationError("Go build attestation schema drift or non-reproducible claim")
        executable_sha = _digest(value.get("executable_sha256"), "executable digest")
        if executable_sha != expected_executable_sha256:
            raise AttestationError("executable digest pin mismatch")
        command = _normalized_command(value.get("build_command"), expected=_GO_RECIPE)
        if expected_build_command and command != list(expected_build_command):
            raise AttestationError("build recipe mismatch")
        package = _normalized_map(value.get("package_provenance"), "package_provenance")
        if expected_package_provenance is not None and package != dict(sorted(expected_package_provenance.items())):
            raise AttestationError("package provenance mismatch")
        execution = value.get("build_execution")
        if not isinstance(execution, Mapping) or set(execution) != {"status", "executed_command", "source_head", "toolchain", "target", "build_input_digest", "sanitized_log_digest", "produced_sha256", "matches_expected"}:
            raise AttestationError("build_execution schema drift")
        if execution.get("status") != "reproduced" or execution.get("source_head") != commit or execution.get("executed_command") != command:
            raise AttestationError("build execution is not a reproduced pinned recipe")
        _target(execution.get("target"))
        if execution.get("toolchain") != toolchain:
            raise AttestationError("build execution toolchain mismatch")
        _digest(execution.get("build_input_digest"), "build_input_digest")
        _digest(execution.get("sanitized_log_digest"), "sanitized_log_digest")
        if execution.get("produced_sha256") != executable_sha or execution.get("matches_expected") is not True:
            raise AttestationError("reproduced executable does not match expected hash")
        if source_root is not None:
            root = Path(source_root)
            if _head(root) != commit or source_artifact_digest(root, commit) != source_sha:
                raise AttestationError("source checkout provenance mismatch")
            _source_clean_for_go(root)
            if build_input_digest(root, commit, command, toolchain) != execution.get("build_input_digest"):
                raise AttestationError("build input digest mismatch")
            if require_runtime:
                reproduced = _run_go_reproduction(root, commit, executable_sha, command, toolchain)
                if reproduced != dict(execution):
                    raise AttestationError("runtime reproduction evidence mismatch")
    elif value.get("adapter_kind") == "graphify":
        required = {"adapter_kind", "distribution_name", "interpreter_sha256", "metadata_pyproject_sha256", "module_name", "runtime_execution", "runtime_role", "schema", "source_artifact_digest", "source_digest_recipe", "source_package_digest", "source_tree_commit", "toolchain"}
        if set(value) != required:
            raise AttestationError("Graphify runtime attestation schema drift")
        if value.get("runtime_role") != (expected_runtime_role or _GRAPHIFY_ROLE):
            raise AttestationError("Graphify runtime role mismatch")
        if value.get("module_name") != expected_module_name or value.get("distribution_name") != expected_distribution_name:
            raise AttestationError("Graphify module/distribution mismatch")
        interpreter_sha = _digest(value.get("interpreter_sha256"), "interpreter digest")
        if expected_interpreter_sha256 and interpreter_sha != expected_interpreter_sha256:
            raise AttestationError("interpreter digest pin mismatch")
        source_package_sha = _digest(value.get("source_package_digest"), "source_package_digest")
        metadata_sha = _digest(value.get("metadata_pyproject_sha256"), "metadata_pyproject_sha256")
        if expected_source_package_sha256 and source_package_sha != expected_source_package_sha256:
            raise AttestationError("Graphify source package digest pin mismatch")
        if expected_metadata_pyproject_sha256 and metadata_sha != expected_metadata_pyproject_sha256:
            raise AttestationError("Graphify pyproject metadata pin mismatch")
        execution = value.get("runtime_execution")
        if not isinstance(execution, Mapping) or set(execution) != {"status", "executed_command", "source_head", "module_file", "module_file_under_source", "module_name", "distribution_name", "version", "metadata_pyproject_sha256", "sanitized_log_digest", "observed_version"}:
            raise AttestationError("runtime_execution schema drift")
        if execution.get("status") != "reproduced" or execution.get("source_head") != commit or execution.get("executed_command") != graphify_runtime_command() or execution.get("module_file_under_source") is not True:
            raise AttestationError("Graphify runtime is not a reproduced pinned import")
        if execution.get("module_name") != expected_module_name or execution.get("distribution_name") != expected_distribution_name or execution.get("version") != expected_version:
            raise AttestationError("Graphify runtime metadata mismatch")
        _digest(execution.get("sanitized_log_digest"), "sanitized_log_digest")
        if execution.get("metadata_pyproject_sha256") != value.get("metadata_pyproject_sha256"):
            raise AttestationError("Graphify metadata digest mismatch")
        if source_root is not None:
            root = Path(source_root)
            if _head(root) != commit or source_artifact_digest(root, commit) != source_sha:
                raise AttestationError("Graphify source provenance mismatch")
            if source_package_digest(root, commit) != value.get("source_package_digest"):
                raise AttestationError("Graphify source package digest mismatch")
            if hashlib.sha256((root / "pyproject.toml").read_bytes()).hexdigest() != value.get("metadata_pyproject_sha256"):
                raise AttestationError("Graphify pyproject digest mismatch")
            if require_runtime:
                if interpreter_path is None:
                    raise AttestationError("Graphify interpreter path is required for runtime proof")
                actual = probe_graphify_runtime(
                    interpreter_path, root,
                    expected_commit=commit, module_name=expected_module_name, distribution_name=expected_distribution_name,
                    expected_version=expected_version, toolchain=toolchain,
                )
                # The portable attestation deliberately excludes the host path.
                actual.pop("interpreter_path", None)
                if any(actual.get(key) != execution.get(key) for key in execution if key != "interpreter_path"):
                    raise AttestationError("Graphify runtime reproduction evidence mismatch")
    else:
        raise AttestationError("unsupported attestation adapter kind")
    if expected_file_sha256 and hashlib.sha256(attestation_path.read_bytes()).hexdigest() != expected_file_sha256:
        raise AttestationError("build attestation digest mismatch")
    return dict(value)


__all__ = [
    "ATTESTATION_SCHEMA", "AttestationError", "build_input_digest", "canonical_attestation", "graphify_runtime_command",
    "probe_graphify_runtime", "source_artifact_digest", "source_package_digest", "validate_attestation",
]
