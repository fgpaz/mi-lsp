"""Fail-closed subprocess adapters for the Victory Lab v2 comparators."""
from __future__ import annotations

import json
import os
import re
import shutil
import subprocess
import tempfile
import time
from contextlib import contextmanager
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Callable, Iterator, Mapping

try:
    from ..attestation_v2 import AttestationError, validate_attestation, source_artifact_digest, probe_graphify_runtime
    from ..canonical_v2 import canonical_payload, parse_json_output, validate_terminal_state
    from ..child_metrics import ChildMetrics, ChildMetricsExecutor
    from ..manifest_v2 import ManifestError, git_revision, resolve_configured_path, sha256_file, validate_current_metadata
    from ..sanitize_v2 import sanitize_env, sanitize_error, sanitize_metrics
    from ..schema_v2 import AdapterSpec, RunRecord
    from ..security_gate import SecurityGate
except ImportError:  # pragma: no cover
    from attestation_v2 import AttestationError, validate_attestation, source_artifact_digest, probe_graphify_runtime
    from canonical_v2 import canonical_payload, parse_json_output, validate_terminal_state
    from child_metrics import ChildMetrics, ChildMetricsExecutor
    from manifest_v2 import ManifestError, git_revision, resolve_configured_path, sha256_file, validate_current_metadata
    from sanitize_v2 import sanitize_env, sanitize_error, sanitize_metrics
    from schema_v2 import AdapterSpec, RunRecord
    from security_gate import SecurityGate


class AdapterError(RuntimeError):
    """An adapter cannot safely produce a comparable sample."""


@dataclass
class CommandResult:
    argv: list[str]
    cwd: str
    env_keys: list[str]
    returncode: int
    stdout: str = ""
    stderr: str = ""
    elapsed_ms: float = 0.0
    timed_out: bool = False
    crashed: bool = False
    metrics: ChildMetrics | None = None
    runtime_proof: dict[str, object] | None = None


class SubprocessExecutor:
    """Real child-process boundary with native metrics and tree cleanup."""

    def __init__(self) -> None:
        self._executor = ChildMetricsExecutor()

    def run(self, argv: list[str], *, cwd: Path, env: Mapping[str, str], timeout_seconds: float) -> CommandResult:
        result = self._executor.run(argv, cwd=cwd, env=env, timeout_seconds=timeout_seconds)
        return CommandResult(
            argv=result.argv, cwd=result.cwd, env_keys=result.env_keys, returncode=result.returncode,
            stdout=result.stdout, stderr=result.stderr, elapsed_ms=result.elapsed_ms,
            timed_out=result.timed_out, crashed=result.crashed, metrics=result.metrics,
            runtime_proof=result.runtime_proof,
        )


def _safe_error(kind: str, message: str) -> dict[str, str]:
    return sanitize_error(kind, message)


def _copy_fixture(source: Path, destination: Path) -> None:
    if not source.is_dir():
        raise AdapterError(f"fixture source missing: {source}")
    shutil.copytree(source, destination, dirs_exist_ok=True)


def _project_toml(repository_identity: str) -> str:
    return "\n".join([
        '[project]', 'name = "victory-lab-v2"', 'languages = ["go"]', 'kind = "single"',
        'default_repo = "go"', 'default_entrypoint = "go.mod"',
        '', '[[repo]]', 'id = "go"', 'name = "go"', 'root = "go"',
        'languages = ["go"]', 'default_entrypoint = "go.mod"',
        f'repository_identity = "{repository_identity}"', '',
    ])


@contextmanager
def materialize_fixture(fixture_source: Path, repository_identity: str) -> Iterator[Path]:
    """Copy the corpus to TEMP and create local project state there only."""
    with tempfile.TemporaryDirectory(prefix="victory-lab-v2-") as raw:
        root = Path(raw)
        _copy_fixture(fixture_source, root)
        state = root / ".mi-lsp"
        state.mkdir(parents=True, exist_ok=True)
        (state / "project.toml").write_text(_project_toml(repository_identity), encoding="utf-8", newline="\n")
        try:
            yield root
        finally:
            # TemporaryDirectory performs the actual recursive cleanup.  The
            # postcondition makes cleanup a measured gate, including returns
            # from provenance/timeout/error paths.
            pass
    if root.exists():
        raise AdapterError("fixture cleanup was not proven")


def _display(item: Any) -> str:
    if isinstance(item, str):
        return item
    if not isinstance(item, Mapping):
        return ""
    for key in ("display", "name", "symbol", "qualified_name", "owner_path"):
        value = item.get(key)
        if isinstance(value, str) and value:
            return value
    return ""


def _path(item: Any) -> str:
    """Return the public file identity for an affected result item."""
    if not isinstance(item, Mapping):
        return ""
    value = item.get("path")
    return value.replace("\\", "/") if isinstance(value, str) and value else ""


def _comparison_key(item: Any, operation: str) -> str:
    """Use file paths for affected and qualified displays for callers."""
    return _path(item) if operation == "affected" else _display(item)


def _items(payload: Any) -> list[Any]:
    if isinstance(payload, Mapping):
        value = payload.get("items", payload.get("nodes", payload.get("results", [])))
        return value if isinstance(value, list) else []
    return payload if isinstance(payload, list) else []


def _qualify_native_items(payload: Any, operation: str = "callers") -> Any:
    """Make caller displays comparable without changing affected file identities."""
    if not isinstance(payload, Mapping) or not isinstance(payload.get("items"), list):
        return payload
    normalized = dict(payload)
    items: list[Any] = []
    for item in payload["items"]:
        if not isinstance(item, Mapping):
            items.append(item)
            continue
        value = dict(item)
        if operation != "affected":
            display = _display(item)
            owner_path = item.get("owner_path")
            if display and isinstance(owner_path, str) and "." not in display:
                package = Path(owner_path.replace("\\", "/")).parent.as_posix().replace("/", ".")
                if package and package != ".":
                    value["display"] = f"{package}.{display}"
        elif _path(item):
            value["path"] = _path(item)
        items.append(value)
    normalized["items"] = items
    return normalized


def _canonical_set_key(item: Any, operation: str, original_index: int) -> tuple[str, int]:
    """Order result sets by their operation's canonical identity."""
    return (_comparison_key(item, operation), original_index)


def _normalize_set_payload(payload: Any, operation: str) -> Any:
    """Normalize result sets without consulting oracle data.

    Callers are symbol sets ordered by qualified display. Affected is a
    file-level set: ``item.path`` is the identity, and duplicate paths are
    collapsed before lexicographic ordering. Path-query order is untouched.
    """
    normalized = _qualify_native_items(payload, operation)
    if operation not in {"callers", "affected"}:
        return normalized
    if not isinstance(normalized, Mapping) or not isinstance(normalized.get("items"), list):
        return normalized
    indexed = list(enumerate(normalized["items"]))
    indexed.sort(key=lambda pair: _canonical_set_key(pair[1], operation, pair[0]))
    result = dict(normalized)
    if operation == "affected":
        seen: set[str] = set()
        unique: list[Any] = []
        for _, item in indexed:
            identity = _path(item)
            if identity and identity not in seen:
                seen.add(identity)
                unique.append(item)
        result["items"] = unique
    else:
        result["items"] = [item for _, item in indexed]
    return result


def _normalize_path_payload(payload: Any, case: Mapping[str, Any]) -> Any:
    """Preserve native path order while making explicit endpoints observable."""
    normalized = _qualify_native_items(payload)
    if not isinstance(normalized, Mapping) or not isinstance(normalized.get("items"), list):
        return normalized
    items = list(normalized["items"])
    from_display = str(case.get("from", ""))
    to_display = str(case.get("to", ""))
    if from_display and (not items or _display(items[0]) != from_display):
        items.insert(0, {"display": from_display})
    if to_display and (not items or _display(items[-1]) != to_display):
        items.append({"display": to_display})
    result = dict(normalized)
    result["items"] = items
    return result


def _normalize_payload(payload: Any, case: Mapping[str, Any]) -> Any:
    """Apply operation-specific normalization without consulting the oracle."""
    if case["operation"] == "path":
        return _normalize_path_payload(payload, case)
    return _normalize_set_payload(payload, case["operation"])


def _graphify_affected_payload(stdout: str) -> dict[str, Any]:
    """Convert Graphify's human-readable affected output into adapter JSON."""
    if not stdout.strip():
        raise ValueError("Graphify returned empty output")
    items: list[dict[str, str]] = []
    line_pattern = re.compile(r"^\s*-\s*(?P<name>.+?)\s+\[[^]]+\]\s+(?P<path>[^:]+):L\d+")
    for line in stdout.splitlines():
        match = line_pattern.match(line)
        if not match:
            continue
        name = match.group("name").removesuffix("()")
        source_path = Path(match.group("path").replace("\\\\", "/"))
        package = source_path.parent.as_posix().replace("/", ".")
        items.append({"display": f"{package}.{name}" if package else name})
    if not items:
        if "No unique node match" in stdout:
            raise ValueError("Graphify could not resolve selector")
        raise ValueError("Graphify output contained no affected items")
    return {
        "ok": True, "backend": "graphify", "completeness": "complete",
        "truncated": False, "done": True, "items": items,
    }


def _adapt_mi_lsp_terminal(native: Any, result: CommandResult) -> Any:
    """Materialize terminality only from a complete, successful mi-lsp process."""
    if (
        not isinstance(native, Mapping)
        or result.returncode != 0
        or result.timed_out
        or result.crashed
    ):
        return native
    if "done" in native:
        return native
    if (
        native.get("ok") is True
        and native.get("truncated") is False
        and native.get("error") in (None, "", {}, [])
        and native.get("errors") in (None, "", {}, [])
        and native.get("partial") in (None, False)
    ):
        adapted = dict(native)
        adapted["done"] = True
        return adapted
    return native


class VictoryAdapter:
    """One adapter instance, with provenance checked before every authoritative run."""

    def __init__(self, spec: AdapterSpec, manifest: Mapping[str, Any], manifest_root: Path, *, executor: Any = None):
        self.spec = spec
        self.manifest = manifest
        self.manifest_root = Path(manifest_root).resolve()
        self.executor = executor or SubprocessExecutor()
        self._metadata: dict[str, Any] | None = None
        self._commit: str = ""
        self._executable = self._resolve_executable()
        # Source checkouts are execution-time provenance inputs.  Resolving one
        # while constructing an adapter would make capability-NC slices and
        # executor fakes depend on unrelated host configuration.
        self._source: Path | None = None
        self._executable_sha = ""
        self._source_sha = ""
        self._security_gate: SecurityGate | None = None

    def _resolve_executable(self) -> Path:
        if self.spec.kind == "graphify":
            return resolve_configured_path("graphify_python", self.spec.executable or None)
        return resolve_configured_path(self.spec.kind, self.spec.executable or None)

    def _resolve_source(self) -> Path | None:
        if self.spec.kind not in {"current", "baseline", "graphify"}:
            return None
        name = "graphify_source" if self.spec.kind == "graphify" else f"{self.spec.kind}_source"
        return resolve_configured_path(name, self.spec.source or None)

    def _env(self, fixture_root: Path) -> dict[str, str]:
        allowed = set(self.spec.env_allowlist)
        source = os.environ
        env = {key: source[key] for key in sorted(allowed) if key in source}
        env.update({
            "MI_LSP_CLIENT_NAME": "victory-lab-v2",
            "MI_LSP_SESSION_ID": f"victory-lab-v2-{self.spec.adapter_id}",
        })
        if "MI_LSP_CLIENT_NAME" not in allowed or "MI_LSP_SESSION_ID" not in allowed:
            raise AdapterError("manifest env allowlist must include MI_LSP provenance keys")
        if self.spec.kind == "graphify":
            env["PYTHONPATH"] = str(self._source)
        env["VICTORY_LAB_FIXTURE_ROOT"] = str(fixture_root)
        if not {"PATH", "TEMP", "TMP"}.issubset(allowed):
            raise AdapterError("manifest env allowlist must include PATH, TEMP, and TMP")
        return env

    def _format_command(self, template: tuple[str, ...], *, root: Path, case: Mapping[str, Any]) -> list[str]:
        selector = str(case.get("selector", ""))
        if case["operation"] == "affected" and not selector:
            selector = ",".join(str(path) for path in case.get("changed_paths", []))
        if self.spec.kind in {"current", "baseline"} and case["operation"] in {"callers", "path"} and "." in selector:
            selector = selector.rsplit(".", 1)[-1]
        if self.spec.kind == "graphify" and case["operation"] == "callers" and "." in selector:
            selector = f"{selector.rsplit('.', 1)[-1]}()"
        mode = str(case.get("mode", "direct"))
        depth = int(case.get("depth", 1 if mode == "direct" else 2))
        if depth < 1:
            raise AdapterError("case depth must be positive")
        from_value = str(case.get("from", ""))
        to_value = str(case.get("to", ""))
        if self.spec.kind in {"current", "baseline"}:
            from_value = from_value.rsplit(".", 1)[-1]
            to_value = to_value.rsplit(".", 1)[-1]
        values = {
            "executable": str(self._executable), "python": str(self._executable), "source": str(self._source or ""),
            "root": str(root), "graph": str(root / "graphify-out" / "graph.json"), "depth": str(depth),
            "operation": str(case["operation"]), "selector": selector,
            "from": from_value, "to": to_value, "mode": mode,
            "changed_paths": ",".join(case.get("changed_paths", [])),
        }
        optional = {"{selector}", "{from}", "{to}", "{changed_paths}", "{graph}"}
        rendered: list[str] = []
        for part in template:
            value = part.format(**values)
            if not value and part in optional:
                continue
            if part in {"--depth", "{depth}"} and case["operation"] not in {"callers", "affected", "path"}:
                continue
            if part in {"--mode", "{mode}"} and case["operation"] != "affected":
                continue
            rendered.append(value)
        # A baseline whose command has no mode flag cannot make a transitive
        # claim; run_case rejects that slice before spawning it.
        return rendered

    def _verify_provenance(self, env: Mapping[str, str], root: Path) -> None:
        if not self._executable.is_file():
            raise AdapterError(f"executable missing: {self._executable}")
        self._executable_sha = sha256_file(self._executable)
        if self.spec.expected_executable_sha256 and self._executable_sha != self.spec.expected_executable_sha256:
            raise AdapterError("executable sha256 mismatch")
        if self.spec.kind in {"current", "baseline"} and self._metadata is not None:
            self._commit = validate_current_metadata(
                self._metadata, self.manifest, expected_commit=self.spec.expected_commit,
                expected_version=self.spec.expected_version, expected_executable_sha256=self.spec.expected_executable_sha256,
                require_observed=True,
            )
        source_required = (
            self.manifest.get("provenance_contract") == "victory-build-attestation/v2"
            or bool(self.spec.source and self.spec.source_digest_path and self.spec.expected_source_sha256)
        )
        if source_required:
            # Resolve only at the provenance boundary.  Construction and
            # capability-NC paths remain independent of source environment.
            self._source = self._resolve_source()
            if self._source is None or not self._source.is_dir():
                raise AdapterError("source checkout is missing")
            self._commit = git_revision(self._source)
            if self._commit != self.spec.expected_commit:
                raise AdapterError("source revision mismatch")
            self._source_sha = source_artifact_digest(self._source, self._commit)
            if self.spec.expected_source_sha256 and self._source_sha != self.spec.expected_source_sha256:
                raise AdapterError("source artifact digest mismatch")
            if self.spec.kind in {"current", "baseline"}:
                status = subprocess.run(
                    ["git", "-C", str(self._source), "status", "--porcelain", "--untracked-files=all", "--", "go.mod", "go.sum", "cmd", "internal"],
                    capture_output=True, text=True, check=False, timeout=10,
                )
                if status.returncode != 0 or status.stdout.strip():
                    raise AdapterError("build-relevant Go source is dirty")
            elif self.spec.kind == "graphify":
                probe_graphify_runtime(
                    self._executable, self._source, expected_commit=self.spec.expected_commit,
                    module_name=self.spec.module_name, distribution_name=self.spec.distribution_name,
                    expected_version=self.spec.expected_version, toolchain=self.spec.toolchain,
                )
        if self.manifest.get("provenance_contract") == "victory-build-attestation/v2":
            if not self.spec.attestation_path or not self.spec.expected_attestation_sha256:
                raise AdapterError("manifested build attestation is missing")
            try:
                validate_attestation(
                    self.manifest_root / self.spec.attestation_path,
                    expected_commit=self._commit,
                    expected_executable_sha256=self._executable_sha,
                    expected_source_sha256=self._source_sha,
                    expected_file_sha256=self.spec.expected_attestation_sha256,
                    source_root=self._source,
                    expected_kind=self.spec.kind,
                    expected_build_command=self.spec.build_command,
                    expected_toolchain=self.spec.toolchain,
                    expected_package_provenance=self.spec.package_provenance,
                    expected_runtime_role=self.spec.runtime_role or None,
                    expected_interpreter_sha256=self.spec.interpreter_sha256,
                    expected_module_name=self.spec.module_name or "graphify",
                    expected_distribution_name=self.spec.distribution_name or "graphifyy",
                    expected_source_package_sha256=self.spec.expected_source_package_sha256,
                    expected_metadata_pyproject_sha256=self.spec.expected_metadata_pyproject_sha256,
                    expected_version=self.spec.expected_version,
                    interpreter_path=self._executable if self.spec.kind == "graphify" else None,
                )
            except (AttestationError, OSError) as exc:
                raise AdapterError("build attestation does not bind executable to commit") from exc

    def _metadata_once(self, root: Path, env: Mapping[str, str]) -> None:
        if self._metadata is not None:
            return
        if not self.spec.metadata_command:
            if self.spec.kind in {"current", "baseline"} and not (
                self.spec.source and self.spec.source_digest_path and self.spec.expected_source_sha256
            ):
                raise AdapterError("observed provenance or explicit source-to-binary proof is required")
            self._metadata = None
            self._verify_provenance(env, root)
            return
        result = self.executor.run(self._format_command(self.spec.metadata_command, root=root, case={"operation": "callers"}), cwd=root, env=env, timeout_seconds=self.spec.timeout_seconds)
        if result.timed_out or result.returncode != 0:
            raise AdapterError("provenance command failed")
        try:
            self._metadata = parse_json_output(result.stdout)
        except ValueError as exc:
            raise AdapterError("provenance command returned invalid JSON") from exc
        self._commit = validate_current_metadata(
            self._metadata,
            self.manifest,
            expected_commit=self.spec.expected_commit or self.manifest["current"]["commit"],
            expected_version=self.spec.expected_version,
            expected_executable_sha256=self.spec.expected_executable_sha256,
        ) if self.spec.kind != "graphify" else self.spec.expected_commit
        self._verify_provenance(env, root)

    def _graphify_input(self, root: Path, case: Mapping[str, Any]) -> Path:
        corpus = Path(str(case["corpus"][0]))
        if corpus.parts and corpus.parts[0] == "corpus":
            corpus = Path(*corpus.parts[1:])
        return root / corpus

    def _prepare_graphify(self, root: Path, case: Mapping[str, Any], env: Mapping[str, str]) -> CommandResult:
        command = (
            str(self._executable), "-m", "graphify", "extract", str(self._graphify_input(root, case)),
            "--code-only", "--no-cluster", "--out", str(root),
        )
        return self.executor.run(list(command), cwd=root, env=env, timeout_seconds=self.spec.timeout_seconds)

    def _prepare_runtime(self, root: Path, case: Mapping[str, Any], env: Mapping[str, str]) -> CommandResult | None:
        if self.spec.kind == "graphify":
            return self._prepare_graphify(root, case, env)
        if self.spec.kind in {"current", "baseline"}:
            command = (
                str(self._executable), "index", "--workspace", str(root),
                "--format", "json", "--no-auto-daemon",
                "--client-name", "victory-lab-v2", "--session-id", f"victory-lab-v2-{self.spec.adapter_id}",
            )
            return self.executor.run(list(command), cwd=root, env=env, timeout_seconds=self.spec.timeout_seconds)
        return None

    def _record(
        self, case: Mapping[str, Any], repetition: int, status: str, *, root: Path,
        env: Mapping[str, str], canonical: Any = None, result: CommandResult | None = None,
        error: dict[str, str] | None = None, security: Mapping[str, Any] | None = None,
    ) -> RunRecord:
        child_metrics = result.metrics.to_dict() if result is not None and result.metrics is not None else {
            "status": "NOT_COMPARABLE", "reason_code": "not_measured"
        }
        record = RunRecord(
            adapter_id=self.spec.adapter_id, operation=case["operation"], status=status, repetition=repetition,
            fixture_digest=str(self.manifest.get("fixture_digest", "")), oracle_digest=str(self.manifest.get("oracle_digest", "")),
            executable_sha256=self._executable_sha, source_sha256=self._source_sha, commit=self._commit or self.spec.expected_commit,
            version=self.spec.expected_version, capabilities=list(self.spec.capabilities),
            # Process paths and raw argv are intentionally not durable evidence.
            argv=[], cwd="", env_keys=sanitize_env({key: None for key in env}, self.spec.env_allowlist),
            elapsed_ms=result.elapsed_ms if result else None, canonical=canonical,
            metrics=sanitize_metrics({
                "sample": repetition, "child": child_metrics,
                "security": security or {"status": "NOT_COMPARABLE", "reason_code": "not_checked"},
            }), error=error,
        )
        record.validate()
        return record

    def _finish_record(
        self, gate: SecurityGate, case: Mapping[str, Any], repetition: int, status: str, *,
        root: Path, env: Mapping[str, str], argv: list[str] | None = None,
        canonical: Any = None, result: CommandResult | None = None,
        error: dict[str, str] | None = None,
    ) -> RunRecord:
        try:
            security = gate.finish(
                argv or [], env,
                runtime_observation=result.runtime_proof if result is not None else None,
            )
        except OSError:
            return self._record(
                case, repetition, "BLOCKED", root=root, env=env, canonical=canonical,
                result=result, error=_safe_error("security", "integrity snapshot unavailable"),
            )
        if security["status"] != "PASS":
            codes = set(security.get("advisory_scan", {}).get("reason_codes", []))
            runtime_reason = security.get("runtime", {}).get("reason_code") if isinstance(security.get("runtime"), Mapping) else None
            if {"network_indicator", "mcp_indicator", "runtime_proof_unavailable"} & (codes | {str(runtime_reason)}):
                # Runtime proof gates authority, but must not erase the child
                # terminal classification (timeout/crash/oracle) from a sample
                # that is already known not to be PASS.
                status = "NOT_COMPARABLE"
                if error is None:
                    error = _safe_error("security", "runtime proof unavailable for network or MCP")
            else:
                error = _safe_error("security", "protected input changed or advisory scan found an indicator")
                status = "BLOCKED"
        elif status == "PASS" and not bool(security.get("runtime_proof", False)):
            status = "NOT_COMPARABLE"
            error = _safe_error("security", "runtime proof unavailable")
        return self._record(
            case, repetition, status, root=root, env=env, canonical=canonical,
            result=result, error=error, security=security,
        )

    def run_case(self, case: Mapping[str, Any], *, repetition: int = 0) -> RunRecord:
        if case["operation"] not in self.spec.capabilities:
            return RunRecord(adapter_id=self.spec.adapter_id, operation=case["operation"], status="NOT_COMPARABLE", repetition=repetition, error=_safe_error("capability", "operation not declared by adapter"))
        if case["operation"] not in self.spec.comparable_operations:
            return RunRecord(adapter_id=self.spec.adapter_id, operation=case["operation"], status="NOT_COMPARABLE", repetition=repetition, error=_safe_error("comparability", "operation is not declared comparable"))
        fixture_source = self.manifest_root / "corpus"
        protected_paths = {
            "fixture_source": fixture_source,
            "goldens": self.manifest_root / "goldens",
            "manifest": self.manifest_root / "manifest.json",
            "attestation": self.manifest_root / self.spec.attestation_path if self.spec.attestation_path else self.manifest_root / "manifest.json",
            "executable": self._executable,
        }
        source_inputs: dict[str, tuple[Path, tuple[str, ...]]] = {}
        source_required = self.manifest.get("provenance_contract") == "victory-build-attestation/v2" or bool(self.spec.source and self.spec.source_digest_path and self.spec.expected_source_sha256)
        if source_required:
            self._source = self._resolve_source()
            if self._source is None:
                raise AdapterError("source checkout is missing")
            if self.spec.kind in {"current", "baseline"}:
                source_inputs[self.spec.kind] = (self._source, ("go.mod", "go.sum", "cmd", "internal"))
            elif self.spec.kind == "graphify":
                source_inputs[self.spec.kind] = (self._source, ("pyproject.toml", self.spec.source_package_path or self.spec.module_name))
        with materialize_fixture(fixture_source, self.manifest["repository_identity"]) as root:
            env = self._env(root)
            gate = SecurityGate(protected_paths, source_inputs=source_inputs)
            try:
                gate.start()
            except OSError:
                return self._record(case, repetition, "BLOCKED", root=root, env=env, error=_safe_error("security", "integrity snapshot unavailable"))
            self._security_gate = gate
            try:
                try:
                    self._metadata_once(root, env)
                    self._verify_provenance(env, root)
                except (AdapterError, ManifestError, OSError) as exc:
                    return self._finish_record(
                        gate, case, repetition, "BLOCKED", root=root, env=env,
                        error=_safe_error("provenance", str(exc)),
                    )
                preparation = self._prepare_runtime(root, case, env)
                if preparation is not None:
                    if preparation.timed_out:
                        return self._finish_record(
                            gate, case, repetition, "FAIL", root=root, env=env,
                            argv=preparation.argv, result=preparation,
                            error=_safe_error("timeout", "adapter preparation timed out"),
                        )
                    if preparation.crashed or preparation.returncode != 0:
                        return self._finish_record(
                            gate, case, repetition, "FAIL", root=root, env=env,
                            argv=preparation.argv, result=preparation,
                            error=_safe_error("prepare", "adapter preparation exited unsuccessfully"),
                        )
                argv = self._format_command(self.spec.command, root=root, case=case)
                result = self.executor.run(argv, cwd=root, env=env, timeout_seconds=self.spec.timeout_seconds)
                if result.timed_out:
                    return self._finish_record(
                        gate, case, repetition, "FAIL", root=root, env=env,
                        argv=argv, result=result, error=_safe_error("timeout", "adapter timed out"),
                    )
                if result.crashed or result.returncode != 0:
                    return self._finish_record(
                        gate, case, repetition, "FAIL", root=root, env=env,
                        argv=argv, result=result, error=_safe_error("crash", "adapter exited unsuccessfully"),
                    )
                try:
                    native = _graphify_affected_payload(result.stdout) if self.spec.kind == "graphify" else parse_json_output(result.stdout)
                    if self.spec.kind in {"current", "baseline"}:
                        native = _adapt_mi_lsp_terminal(native, result)
                    native = _normalize_payload(native, case)
                    validate_terminal_state(native)
                    canonical = canonical_payload(case["operation"], native, root)
                except ValueError as exc:
                    return self._finish_record(
                        gate, case, repetition, "FAIL", root=root, env=env,
                        argv=argv, result=result, error=_safe_error("decode", str(exc)),
                    )
                if self.spec.kind == "baseline" and case["operation"] == "affected":
                    # The baseline is a heuristic hot-path guard.  Its process,
                    # terminal output, and security evidence remain measured,
                    # but symbol-level oracle comparison is not claimed.
                    return self._finish_record(
                        gate, case, repetition, "NOT_COMPARABLE", root=root, env=env,
                        argv=argv, canonical=canonical, result=result,
                        error=_safe_error("comparability", "not_comparable"),
                    )
                expected = self.manifest.get("oracles", {}).get(case["id"], {})
                actual = [_comparison_key(item, case["operation"]) for item in _items(native)]
                if case["operation"] == "path":
                    target = expected.get("expected_shortest_path", [])
                elif case["operation"] in {"callers", "affected"}:
                    target = expected.get(
                        "expected_direct" if case.get("mode", "direct") == "direct" else "expected_transitive", []
                    )
                else:
                    target = []
                target = [str(item) for item in target]
                status = "PASS" if actual == target else "FAIL"
                return self._finish_record(
                    gate, case, repetition, status, root=root, env=env,
                    argv=argv, canonical=canonical, result=result,
                    error=None if status == "PASS" else _safe_error("oracle", "canonical result differs from oracle"),
                )
            finally:
                self._security_gate = None
