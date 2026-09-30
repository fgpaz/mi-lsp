#!/usr/bin/env python3
"""Reproducible subprocess benchmark for mi-lsp CLI and native MCP startup."""

from __future__ import annotations

import argparse
import datetime
import hashlib
import json
import os
import platform
import queue
import re
import signal
import statistics
import subprocess
import sys
import threading
import time
from pathlib import Path
from typing import Any


CLI_CASES: dict[str, list[str]] = {
    "find_extract_file_symbols": ["nav", "find", "ExtractFileSymbols", "--exact"],
    "search_graph_repository_invalid": ["nav", "search", "ErrGraphRepositoryInvalid"],
    "multi_read_graph_model": ["nav", "multi-read", "internal/model/graph.go:35-52"],
    "route_navigation": ["nav", "route", "mi-lsp navigation"],
    "intent_navigation": ["nav", "intent", "medir navegación mi-lsp"],
    "affected_graph_model": ["nav", "affected", "internal/model/graph.go"],
}
CONSISTENCY_CASES = (
    "find_extract_file_symbols",
    "search_graph_repository_invalid",
    "multi_read_graph_model",
    "affected_graph_model",
)
MCP_PROTOCOL_VERSION = "2024-11-05"
WORKSPACE_PATTERN = re.compile(r"(?m)^workspace\s*[:=]\s*(.+?)\s*$", re.IGNORECASE)


def percentile95(values: list[float]) -> float | None:
    """Nearest-rank p95, matching an explicit rank convention across runs."""
    if not values:
        return None
    ordered = sorted(values)
    rank = max(1, (95 * len(ordered) + 99) // 100)
    return ordered[rank - 1]


def summary(values: list[float]) -> dict[str, float | int | None]:
    if not values:
        return {"samples": 0, "median_ms": None, "p95_ms": None, "max_ms": None}
    return {
        "samples": len(values),
        "median_ms": statistics.median(values),
        "p95_ms": percentile95(values),
        "max_ms": max(values),
    }


def sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for block in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(block)
    return digest.hexdigest()


def reported_workspace(stdout: bytes) -> str | None:
    text = stdout.decode("utf-8", errors="replace")
    match = WORKSPACE_PATTERN.search(text)
    if match:
        return match.group(1).strip().strip('"')
    try:
        payload = json.loads(text)
    except (json.JSONDecodeError, UnicodeDecodeError):
        return None
    if isinstance(payload, dict) and payload.get("workspace") is not None:
        return str(payload["workspace"])
    return None


def git_revision(cwd: Path) -> str | None:
    try:
        result = subprocess.run(
            ["git", "rev-parse", "HEAD"], cwd=cwd, capture_output=True,
            text=True, timeout=5, check=False,
        )
    except (OSError, subprocess.TimeoutExpired):
        return None
    return result.stdout.strip() if result.returncode == 0 else None


def terminate_process(process: subprocess.Popen[bytes]) -> None:
    if process.poll() is not None:
        return
    try:
        if os.name == "nt":
            process.terminate()
        else:
            os.killpg(process.pid, signal.SIGTERM)
        process.wait(timeout=0.5)
    except (OSError, subprocess.TimeoutExpired):
        try:
            if os.name == "nt":
                process.kill()
            else:
                os.killpg(process.pid, signal.SIGKILL)
        except OSError:
            process.kill()
        process.wait()


def run_cli(
    binary: Path, argv: list[str], cwd: Path, environment: dict[str, str], timeout: float,
) -> dict[str, Any]:
    started = time.perf_counter_ns()
    process: subprocess.Popen[bytes] | None = None
    timed_out = False
    try:
        process = subprocess.Popen(
            [str(binary), *argv], cwd=cwd, env=environment,
            stdout=subprocess.PIPE, stderr=subprocess.PIPE,
            start_new_session=(os.name != "nt"),
            creationflags=subprocess.CREATE_NEW_PROCESS_GROUP if os.name == "nt" else 0,
        )
        try:
            stdout, stderr = process.communicate(timeout=timeout)
        except subprocess.TimeoutExpired:
            timed_out = True
            terminate_process(process)
            stdout, stderr = process.communicate()
    except OSError as exc:
        stdout, stderr = b"", str(exc).encode("utf-8", errors="replace")
    elapsed = (time.perf_counter_ns() - started) / 1_000_000
    out_text = stdout.decode("utf-8", errors="replace")
    record = {
        "elapsed_ms": elapsed,
        "timed_out": timed_out,
        "exit_code": process.returncode if process is not None else None,
        "stdout_bytes": len(stdout),
        "stderr_bytes": len(stderr),
        "workspace_reported": reported_workspace(stdout),
        "error": stderr.decode("utf-8", errors="replace")[:1000] if (process is None or process.returncode or timed_out) else None,
    }
    if "--format" in argv and argv[argv.index("--format") + 1:argv.index("--format") + 2] == ["json"]:
        try:
            parsed = json.loads(out_text)
            record["json_contract"] = {"valid": True, "root_type": type(parsed).__name__}
        except json.JSONDecodeError as exc:
            record["json_contract"] = {"valid": False, "error": str(exc)[:300]}
    return record


class LineReader:
    """Drain child stdout without blocking writes or losing partial lines."""

    def __init__(self, stream: Any) -> None:
        self.lines: queue.Queue[bytes | None] = queue.Queue()
        self.bytes_read = 0
        self.thread = threading.Thread(target=self._read, args=(stream,), daemon=True)
        self.thread.start()

    def _read(self, stream: Any) -> None:
        try:
            while True:
                line = stream.readline()
                if not line:
                    break
                self.bytes_read += len(line)
                self.lines.put(line)
        finally:
            self.lines.put(None)

    def response(self, request_id: int, deadline: float) -> dict[str, Any]:
        while True:
            remaining = deadline - time.monotonic()
            if remaining <= 0:
                raise TimeoutError("MCP response deadline exceeded")
            try:
                line = self.lines.get(timeout=remaining)
            except queue.Empty as exc:
                raise TimeoutError("MCP response deadline exceeded") from exc
            if line is None:
                raise EOFError("MCP server closed stdout before response")
            try:
                message = json.loads(line)
            except (UnicodeDecodeError, json.JSONDecodeError):
                continue
            if isinstance(message, dict) and message.get("id") == request_id:
                return message


def write_rpc(process: subprocess.Popen[bytes], message: dict[str, Any]) -> None:
    if process.stdin is None:
        raise BrokenPipeError("MCP stdin is unavailable")
    process.stdin.write((json.dumps(message, separators=(",", ":")) + "\n").encode())
    process.stdin.flush()


def run_mcp(
    binary: Path, cwd: Path, environment: dict[str, str], workspace: str,
    pattern: str, timeout: float,
) -> dict[str, Any]:
    started = time.perf_counter_ns()
    process: subprocess.Popen[bytes] | None = None
    timed_out = False
    result: dict[str, Any] = {"initialized": False, "tool_response": False}
    stderr_chunks: list[bytes] = []
    try:
        process = subprocess.Popen(
            [str(binary), "mcp", "--workspace", workspace], cwd=cwd, env=environment,
            stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
            start_new_session=(os.name != "nt"),
            creationflags=subprocess.CREATE_NEW_PROCESS_GROUP if os.name == "nt" else 0,
        )
        assert process.stdout is not None
        reader = LineReader(process.stdout)
        if process.stderr is not None:
            def drain_stderr() -> None:
                while True:
                    chunk = process.stderr.read(4096)
                    if not chunk:
                        break
                    stderr_chunks.append(chunk)
            threading.Thread(target=drain_stderr, daemon=True).start()
        deadline = time.monotonic() + timeout
        write_rpc(process, {
            "jsonrpc": "2.0", "id": 1, "method": "initialize",
            "params": {
                "protocolVersion": MCP_PROTOCOL_VERSION,
                "capabilities": {},
                "clientInfo": {"name": "mi-lsp-navigation-benchmark", "version": "1"},
            },
        })
        initialized = reader.response(1, deadline)
        result["initialized"] = "result" in initialized and "error" not in initialized
        write_rpc(process, {"jsonrpc": "2.0", "method": "notifications/initialized"})
        write_rpc(process, {
            "jsonrpc": "2.0", "id": 2, "method": "tools/call",
            "params": {"name": "nav_search", "arguments": {"pattern": pattern, "workspace": workspace}},
        })
        response = reader.response(2, deadline)
        result["tool_response"] = "result" in response and "error" not in response
        if "error" in response:
            result["protocol_error"] = str(response["error"])[:1000]
        elif isinstance(response.get("result"), dict):
            result["is_error"] = response["result"].get("isError")
            result["content_blocks"] = len(response["result"].get("content", []))
    except TimeoutError as exc:
        timed_out = True
        result["error"] = str(exc)
    except (OSError, EOFError, BrokenPipeError, AssertionError) as exc:
        result["error"] = str(exc)[:1000]
    finally:
        if process is not None:
            terminate_process(process)
    stderr = b"".join(stderr_chunks)[:65536]
    elapsed = (time.perf_counter_ns() - started) / 1_000_000
    result.update({
        "elapsed_ms": elapsed,
        "timed_out": timed_out,
        "exit_code_after_cleanup": process.returncode if process is not None else None,
        "stdout_bytes": reader.bytes_read if "reader" in locals() else 0,
        "stderr_bytes_captured": len(stderr),
        "stderr_excerpt": stderr.decode("utf-8", errors="replace")[:1000] or None,
    })
    return result


def command_for(name: str, workspace_selector: str | None, explicit_json: bool = False) -> list[str]:
    argv = list(CLI_CASES[name])
    if workspace_selector is not None:
        argv.extend(["--workspace", workspace_selector])
    if explicit_json:
        argv.extend(["--format", "json"])
    return argv


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", default="mi-lsp", help="mi-lsp executable")
    parser.add_argument("--workspace", required=True, help="workspace alias or absolute path")
    parser.add_argument("--cwd", default=".", help="working directory used for child processes")
    parser.add_argument("--samples", type=int, default=30, help="warm and MCP repetitions (default: 30)")
    parser.add_argument("--timeout", type=float, default=120.0, help="firm timeout in seconds per child")
    parser.add_argument("--output", required=True, help="JSON result path")
    args = parser.parse_args(argv)
    campaign_started = time.perf_counter_ns()
    started_utc = datetime.datetime.now(datetime.timezone.utc).isoformat()
    if not 1 <= args.samples <= 1000:
        parser.error("--samples must be between 1 and 1000")
    if not 0 < args.timeout <= 3600:
        parser.error("--timeout must be greater than 0 and at most 3600 seconds")
    for name in ("MI_LSP_CLIENT_NAME", "MI_LSP_SESSION_ID"):
        if not os.environ.get(name):
            parser.error(f"{name} must be set and non-empty")

    cwd = Path(args.cwd).resolve()
    binary_arg = Path(args.binary).expanduser()
    found = str(binary_arg.resolve()) if binary_arg.parent != Path(".") or binary_arg.is_absolute() else __import__("shutil").which(args.binary)
    if not found:
        parser.error(f"mi-lsp binary not found: {args.binary}")
    binary = Path(found).resolve()
    if not binary.is_file():
        parser.error(f"mi-lsp binary is not a file: {binary}")
    environment = os.environ.copy()
    environment["MI_LSP_CLIENT_NAME"] = os.environ["MI_LSP_CLIENT_NAME"]
    environment["MI_LSP_SESSION_ID"] = os.environ["MI_LSP_SESSION_ID"]

    commands: dict[str, Any] = {}
    for name, base in CLI_CASES.items():
        samples: list[dict[str, Any]] = []
        selectors: list[str | None] = [args.workspace]
        samples.append({
            "phase": "cold", "repetition": 0,
            **run_cli(binary, command_for(name, selectors[0]), cwd, environment, args.timeout),
        })
        for repetition in range(1, args.samples + 1):
            samples.append({
                "phase": "warm", "repetition": repetition,
                **run_cli(binary, command_for(name, args.workspace), cwd, environment, args.timeout),
            })
        successful = [item for item in samples[1:] if item["exit_code"] == 0 and not item["timed_out"]]
        warm = [item["elapsed_ms"] for item in successful]
        commands[name] = {
            "base_argv": [str(binary), *base],
            "cold": samples[0], "warm_summary": summary(warm),
            "failed_warm_samples": len(samples) - 1 - len(successful), "samples": samples,
        }
        print(f"completed CLI case: {name} ({len(warm)} warm samples)", file=sys.stderr, flush=True)

    consistency: dict[str, Any] = {}
    selectors = [
        ("cwd", None, cwd),
        ("alias", args.workspace, cwd),
        ("absolute_path", str(cwd), cwd),
    ]
    for name in CONSISTENCY_CASES:
        consistency[name] = {}
        for label, selector, sample_cwd in selectors:
            observed = run_cli(
                binary, command_for(name, selector), sample_cwd, environment, args.timeout,
            )
            observed["workspace_resolution"] = (
                "reported" if observed["workspace_reported"] else "unresolved_or_output_omits_workspace"
            )
            consistency[name][label] = observed
        names = {
            consistency[name][label]["workspace_reported"]
            for label in ("cwd", "alias", "absolute_path")
            if consistency[name][label]["workspace_reported"] is not None
        }
        complete = all(
            consistency[name][label]["workspace_reported"] is not None
            for label in ("cwd", "alias", "absolute_path")
        )
        consistency[name]["comparison"] = {
            "all_reported": complete,
            "same_workspace": complete and len(names) == 1,
            "reported_names": sorted(names),
        }
        print(f"completed workspace consistency: {name}", file=sys.stderr, flush=True)

    json_probe = run_cli(
        binary, command_for("search_graph_repository_invalid", args.workspace, explicit_json=True),
        cwd, environment, args.timeout,
    )
    # Probe parsing is intentionally summarized; response content is not copied to the artifact.
    json_probe["contract_probe"] = "stdout JSON validity and root type recorded"
    print("completed explicit JSON contract probe", file=sys.stderr, flush=True)

    mcp_samples = []
    for repetition in range(1, args.samples + 1):
        mcp_samples.append({
            "repetition": repetition,
            **run_mcp(binary, cwd, environment, args.workspace, "ErrGraphRepositoryInvalid", args.timeout),
        })
    successful_mcp = [
        sample for sample in mcp_samples
        if sample["initialized"] and sample["tool_response"] and not sample["timed_out"]
    ]
    mcp_times = [sample["elapsed_ms"] for sample in successful_mcp]
    print(f"completed MCP case ({len(mcp_samples)} samples)", file=sys.stderr, flush=True)

    artifact = {
        "schema": "milsp-navigation-benchmark/v1",
        "conditions": {
            "started_utc": started_utc,
            "git_revision": git_revision(cwd),
            "binary": str(binary), "binary_sha256": sha256_file(binary),
            "platform": platform.platform(), "python": platform.python_version(),
            "cwd": str(cwd), "workspace_argument": args.workspace,
            "client_name": environment["MI_LSP_CLIENT_NAME"],
            "session_id": environment["MI_LSP_SESSION_ID"],
            "samples_per_warm_and_mcp_case": args.samples,
            "timeout_seconds_per_process": args.timeout,
            "clock": "perf_counter_ns; elapsed includes child spawn and complete response",
        },
        "campaign_elapsed_ms": (time.perf_counter_ns() - campaign_started) / 1_000_000,
        "cli": commands,
        "workspace_consistency": consistency,
        "explicit_json_probe": json_probe,
        "mcp_cold_initialize_and_nav_search": {
            "tool": "nav_search", "pattern": "ErrGraphRepositoryInvalid",
            "status": "available" if successful_mcp else "unavailable",
            "successful_samples": len(successful_mcp),
            "failed_samples": len(mcp_samples) - len(successful_mcp),
            "summary": summary(mcp_times), "samples": mcp_samples,
        },
    }
    output = Path(args.output).expanduser()
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(json.dumps(artifact, ensure_ascii=False, sort_keys=True, indent=2) + "\n", encoding="utf-8")
    print(json.dumps({
        "schema": artifact["schema"], "output": str(output.resolve()),
        "git_revision": artifact["conditions"]["git_revision"],
        "binary_sha256": artifact["conditions"]["binary_sha256"],
        "mcp_cold": artifact["mcp_cold_initialize_and_nav_search"]["summary"],
    }, ensure_ascii=False, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
