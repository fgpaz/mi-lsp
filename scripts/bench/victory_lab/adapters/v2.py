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
    from ..canonical_v2 import canonical_payload, parse_json_output
    from ..child_metrics import ChildMetrics, ChildMetricsExecutor
    from ..manifest_v2 import ManifestError, git_revision, resolve_configured_path, sha256_file, validate_current_metadata
    from ..sanitize_v2 import sanitize_env, sanitize_error, sanitize_metrics
    from ..schema_v2 import AdapterSpec, RunRecord
    from ..security_gate import SecurityGate
except ImportError:  # pragma: no cover
    from canonical_v2 import canonical_payload, parse_json_output
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
        )


def _safe_error(kind: str, message: str) -> dict[str, str]:
    return sanitize_error(kind, message)


def _copy_fixture(source: Path, destination: Path) -> None:
    if not source.is_dir():
        raise AdapterError(f"fixture source missing: {source}")
    shutil.copytree(source, destination, dirs_exist_ok=True)


def _project_toml(repository_identity: str) -> str:
    return "\n".join([
        '[project]', 'name = "victory-lab-v2"', 'languages = ["go"]', 'kind = "single"', 'default_repo = "go"',
        '', '[[repo]]', 'id = "go"', 'name = "go"', 'root = "go"',
        'languages = ["go"]', f'repository_identity = "{repository_identity}"', '',
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
        yield root


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


def _items(payload: Any) -> list[Any]:
    if isinstance(payload, Mapping):
        value = payload.get("items", payload.get("nodes", payload.get("results", [])))
        return value if isinstance(value, list) else []
    return payload if isinstance(payload, list) else []


def _qualify_native_items(payload: Any) -> Any:
    """Make mi-lsp's display-only symbols comparable across repository packages."""
    if not isinstance(payload, Mapping) or not isinstance(payload.get("items"), list):
        return payload
    normalized = dict(payload)
    items: list[Any] = []
    for item in payload["items"]:
        if not isinstance(item, Mapping):
            items.append(item)
            continue
        value = dict(item)
        display = _display(item)
        owner_path = item.get("owner_path")
        if display and isinstance(owner_path, str) and "." not in display:
            package = Path(owner_path.replace("\\", "/")).parent.as_posix().replace("/", ".")
            if package and package != ".":
                value["display"] = f"{package}.{display}"
        items.append(value)
    normalized["items"] = items
    return normalized


def _graphify_affected_payload(stdout: str) -> dict[str, Any]:
    """Convert Graphify's human-readable affected output into adapter JSON."""
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
    if not items and "No unique node match" in stdout:
        raise ValueError("Graphify could not resolve selector")
    return {"items": items}


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
        self._source = self._resolve_source()
        self._executable_sha = ""
        self._source_sha = ""
        self._security_gate: SecurityGate | None = None

    def _resolve_executable(self) -> Path:
        if self.spec.kind == "graphify":
            return resolve_configured_path("graphify_python", self.spec.executable or None)
        return resolve_configured_path(self.spec.kind, self.spec.executable or None)

    def _resolve_source(self) -> Path | None:
        if self.spec.kind != "graphify":
            return None
        return resolve_configured_path("graphify_source", self.spec.source or None)

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
        depth = 1 if case.get("mode", "direct") == "direct" else 2
        from_value = str(case.get("from", ""))
        to_value = str(case.get("to", ""))
        if self.spec.kind in {"current", "baseline"}:
            from_value = from_value.rsplit(".", 1)[-1]
            to_value = to_value.rsplit(".", 1)[-1]
        values = {
            "executable": str(self._executable), "python": str(self._executable), "source": str(self._source or ""),
            "root": str(root), "graph": str(root / "graphify-out" / "graph.json"), "depth": str(depth),
            "operation": str(case["operation"]), "selector": selector,
            "from": from_value, "to": to_value,
            "changed_paths": ",".join(case.get("changed_paths", [])),
        }
        optional = {"{selector}", "{from}", "{to}", "{changed_paths}", "{graph}"}
        return [rendered for part in template if (rendered := part.format(**values)) or part not in optional]

    def _verify_provenance(self, env: Mapping[str, str], root: Path) -> None:
        if not self._executable.is_file():
            raise AdapterError(f"executable missing: {self._executable}")
        self._executable_sha = sha256_file(self._executable)
        if self.spec.expected_executable_sha256 and self._executable_sha != self.spec.expected_executable_sha256:
            raise AdapterError("executable sha256 mismatch")
        if self.spec.kind == "graphify":
            if self._source is None or not self._source.is_dir():
                raise AdapterError("Graphify source missing")
            self._commit = git_revision(self._source)
            if self._commit != self.spec.expected_commit:
                raise AdapterError("Graphify source revision mismatch")
            if self.spec.source_digest_path:
                source_digest_path = self._source / self.spec.source_digest_path
                self._source_sha = sha256_file(source_digest_path)
                if self.spec.expected_source_sha256 and self._source_sha != self.spec.expected_source_sha256:
                    raise AdapterError("Graphify source sha256 mismatch")
        elif self._metadata is not None:
            self._commit = validate_current_metadata(
                self._metadata,
                self.manifest,
                expected_commit=self.spec.expected_commit or self.manifest["current"]["commit"],
                expected_version=self.spec.expected_version,
            )

    def _metadata_once(self, root: Path, env: Mapping[str, str]) -> None:
        if self._metadata is not None:
            return
        if not self.spec.metadata_command:
            self._metadata = {}
            self._commit = self.spec.expected_commit
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
            security = gate.finish(argv or [], env)
        except OSError:
            return self._record(
                case, repetition, "BLOCKED", root=root, env=env, canonical=canonical,
                result=result, error=_safe_error("security", "integrity snapshot unavailable"),
            )
        if security["status"] != "PASS":
            error = _safe_error("security", "protected input changed or advisory scan found an indicator")
            status = "BLOCKED"
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
        protected_paths = {"fixture_source": fixture_source, "manifest": self.manifest_root / "manifest.json"}
        with materialize_fixture(fixture_source, self.manifest["repository_identity"]) as root:
            env = self._env(root)
            gate = SecurityGate(protected_paths)
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
                    native = _graphify_affected_payload(result.stdout) if self.spec.kind == "graphify" else _qualify_native_items(parse_json_output(result.stdout))
                    canonical = canonical_payload(case["operation"], native, root)
                except ValueError as exc:
                    return self._finish_record(
                        gate, case, repetition, "FAIL", root=root, env=env,
                        argv=argv, result=result, error=_safe_error("decode", str(exc)),
                    )
                expected = self.manifest.get("oracles", {}).get(case["id"], {})
                actual = [_display(item) for item in _items(native)]
                if case["operation"] == "path":
                    target = expected.get("expected_shortest_path", [])
                elif case["operation"] == "callers":
                    target = expected.get("expected_direct", []) if case.get("mode", "direct") == "direct" else expected.get("expected_transitive", [])
                else:
                    target = expected.get("expected_direct", [])
                target = [str(item) for item in target]
                status = "PASS" if actual == target or set(actual) == set(target) else "FAIL"
                return self._finish_record(
                    gate, case, repetition, status, root=root, env=env,
                    argv=argv, canonical=canonical, result=result,
                    error=None if status == "PASS" else _safe_error("oracle", "canonical result differs from oracle"),
                )
            finally:
                self._security_gate = None
