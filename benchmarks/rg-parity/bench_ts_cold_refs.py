#!/usr/bin/env python3
"""Mide latencia fría de nav refs TS y calidad semántica posterior."""

import argparse
import json
import os
from pathlib import Path
import shutil
import statistics
import subprocess
import sys
import tempfile
import time


LATENCY_TARGET_MS = 2000
DEFAULT_REGISTRY = Path.home() / ".mi-lsp" / "registry.toml"


def normalize_location(item, workspace_root):
    file_name = item.get("file") or item.get("file_path") or item.get("path")
    line = item.get("line")
    if not isinstance(file_name, str) or not isinstance(line, (int, float)):
        return None
    path = Path(file_name)
    if path.is_absolute():
        try:
            path = path.relative_to(workspace_root)
        except ValueError:
            return None
    return f"{path.as_posix()}:{int(line)}"


def parse_expected_refs(values):
    expected = set()
    for value in values:
        file_name, separator, line = value.rpartition(":")
        if not separator or not file_name or not line.isdigit():
            raise ValueError(f"invalid --expected-ref {value!r}; expected file:line")
        expected.add(f"{Path(file_name).as_posix()}:{int(line)}")
    return expected


def resolve_binary(value):
    resolved = shutil.which(value)
    if resolved:
        return resolved
    candidate = Path(value).expanduser().resolve()
    if candidate.is_file():
        return str(candidate)
    raise FileNotFoundError(f"mi-lsp binary not found: {value}")


def find_tsserver(workspace_root, env):
    roots = [workspace_root, *workspace_root.parents]
    local_candidates = [root / "node_modules" / "typescript" / "lib" / "tsserver.js" for root in roots]
    for candidate in local_candidates:
        if candidate.is_file():
            return candidate, None

    npm_root = subprocess.run(
        ["npm", "root", "-g"], capture_output=True, text=True, env=env, timeout=10, check=False
    )
    global_root = Path(npm_root.stdout.strip()) if npm_root.returncode == 0 else None
    global_candidate = global_root / "typescript" / "lib" / "tsserver.js" if global_root else None
    if global_candidate and global_candidate.is_file():
        return global_candidate, global_root.parent.parent
    checked = ", ".join(str(candidate) for candidate in local_candidates)
    if global_candidate:
        checked += f", {global_candidate}"
    raise FileNotFoundError(f"tsserver.js not found in production lookup paths: {checked}")


def run_command(command, cwd, env, timeout):
    started = time.perf_counter()
    try:
        completed = subprocess.run(
            command, cwd=cwd, text=True, capture_output=True, env=env, timeout=timeout, check=False
        )
        return {
            "elapsed_ms": round((time.perf_counter() - started) * 1000, 2),
            "exit_code": completed.returncode,
            "stdout": completed.stdout,
            "stderr": completed.stderr,
            "timed_out": False,
        }
    except subprocess.TimeoutExpired as exc:
        stdout = exc.stdout or ""
        stderr = exc.stderr or ""
        if isinstance(stdout, bytes):
            stdout = stdout.decode("utf-8", errors="replace")
        if isinstance(stderr, bytes):
            stderr = stderr.decode("utf-8", errors="replace")
        return {
            "elapsed_ms": round((time.perf_counter() - started) * 1000, 2),
            "exit_code": None,
            "stdout": stdout,
            "stderr": stderr,
            "timed_out": True,
        }


def parse_envelope(result):
    try:
        envelope = json.loads(result["stdout"])
    except json.JSONDecodeError:
        return None
    if not isinstance(envelope, dict):
        return None
    items = envelope.get("items")
    if not isinstance(items, list) or any(not isinstance(item, dict) for item in items):
        return None
    return envelope


def valid_envelope(result, envelope):
    return (
        not result["timed_out"]
        and result["exit_code"] == 0
        and envelope is not None
        and envelope.get("ok") is True
        and all(item.get("origin") in {"semantic", "catalog", "text"} for item in envelope["items"])
    )


def is_not_degraded(envelope):
    # RF-QRY-001 defines degraded=false as omitted (omitempty); null is not false.
    return envelope is not None and (
        "degraded" not in envelope or envelope["degraded"] is False
    )


def first_response_valid(result, envelope):
    if not valid_envelope(result, envelope):
        return False
    items = envelope["items"]
    if envelope.get("degraded") is True:
        return (
            envelope.get("fallback_used") == "text"
            and bool(envelope.get("reason"))
            and all(item.get("origin") == "text" for item in items)
        )
    return (
        envelope.get("backend") == "tsserver"
        and is_not_degraded(envelope)
        and bool(items)
        and all(item.get("origin") == "semantic" for item in items)
    )


def semantic_warmup_success(result, envelope):
    return (
        valid_envelope(result, envelope)
        and envelope.get("backend") == "tsserver"
        and is_not_degraded(envelope)
        and bool(envelope["items"])
        and all(item.get("origin") == "semantic" for item in envelope["items"])
    )


def origins(envelope):
    if envelope is None:
        return []
    return sorted({item.get("origin") for item in envelope["items"] if item.get("origin") is not None})


def semantic_quality(result, envelope, expected_refs, workspace_root):
    items = envelope["items"] if envelope is not None else []
    locations = {
        location
        for item in items
        if (location := normalize_location(item, workspace_root)) is not None
    }
    missing = sorted(expected_refs - locations)
    all_semantic = bool(items) and all(item.get("origin") == "semantic" for item in items)
    semantic_backend = envelope is not None and envelope.get("backend") == "tsserver"
    not_degraded = is_not_degraded(envelope)
    failures = []
    if not valid_envelope(result, envelope):
        failures.append("invalid_envelope")
    if not semantic_backend:
        failures.append("backend_not_tsserver")
    if not not_degraded:
        failures.append("degraded_or_invalid_degraded_flag")
    if not all_semantic:
        failures.append("items_missing_or_not_all_semantic")
    if missing:
        failures.append("expected_semantic_references_missing")
    return {
        "passed": not failures,
        "failures": failures,
        "backend": envelope.get("backend") if envelope else None,
        "degraded": envelope.get("degraded", False) if envelope is not None else None,
        "degraded_flag_present": envelope is not None and "degraded" in envelope,
        "reason": envelope.get("reason") if envelope else None,
        "origins": origins(envelope),
        "semantic_item_count": sum(item.get("origin") == "semantic" for item in items),
        "expected_refs": sorted(expected_refs),
        "observed_locations": sorted(locations),
        "missing_expected_refs": missing,
        "latency_ms": result["elapsed_ms"],
    }


def query_command(binary, workspace, symbol, file_name, session_id):
    return [
        binary,
        "--format",
        "json",
        "--workspace",
        workspace,
        "--client-name",
        "bench",
        "--session-id",
        session_id,
        "--no-auto-daemon",
        "nav",
        "refs",
        symbol,
        "--file",
        file_name,
    ]


def run_one(args, run_index, expected_refs):
    registry_path = args.registry.expanduser().resolve()
    if not registry_path.is_file():
        raise FileNotFoundError(f"workspace registry not found: {registry_path}")
    with tempfile.TemporaryDirectory(prefix="mi-lsp-ts-cold-") as temporary_home:
        home = Path(temporary_home)
        isolated_registry = home / ".mi-lsp" / "registry.toml"
        isolated_registry.parent.mkdir(mode=0o700, parents=True)
        shutil.copyfile(registry_path, isolated_registry)
        isolated_registry.chmod(0o600)

        env = os.environ.copy()
        env["HOME"] = str(home)
        if os.name == "nt":
            env["USERPROFILE"] = str(home)
        env["MI_LSP_AUTOINDEX"] = "0"
        env["MI_LSP_NO_AUTO_REGISTER"] = "1"
        env["MI_LSP_CLIENT_NAME"] = "bench"
        if args.npm_prefix:
            env["npm_config_prefix"] = str(args.npm_prefix.expanduser().resolve())

        env["MI_LSP_SESSION_ID"] = f"ts-cold-refs-{run_index}-daemon-start"
        daemon_start = run_command(
            [args.milsp, "--format", "json", "daemon", "start", "--idle-timeout", "5m"],
            args.workspace_root,
            env,
            args.command_timeout,
        )
        if daemon_start["exit_code"] != 0 or daemon_start["timed_out"]:
            env["MI_LSP_SESSION_ID"] = f"ts-cold-refs-{run_index}-daemon-stop-after-start-error"
            stop = run_command(
                [args.milsp, "--format", "json", "--no-auto-daemon", "daemon", "stop"],
                args.workspace_root,
                env,
                min(args.command_timeout, 15),
            )
            return {
                "run": run_index + 1,
                "setup_error": "isolated_daemon_start_failed",
                "daemon_start_exit_code": daemon_start["exit_code"],
                "daemon_start_timed_out": daemon_start["timed_out"],
                "daemon_start_stderr": daemon_start["stderr"].strip(),
                "daemon_stopped_after_setup_error": stop["exit_code"] == 0 and not stop["timed_out"],
            }

        record = None
        try:
            env["MI_LSP_SESSION_ID"] = f"ts-cold-refs-{run_index}-cold"
            cold = run_command(
                query_command(args.milsp, args.workspace, args.symbol, args.file, env["MI_LSP_SESSION_ID"]),
                args.workspace_root,
                env,
                args.command_timeout,
            )
            cold_envelope = parse_envelope(cold)
            cold_valid = valid_envelope(cold, cold_envelope)
            cold_items = cold_envelope["items"] if cold_envelope else []
            cold_origins = origins(cold_envelope)
            if cold_envelope and cold_envelope.get("backend") == "tsserver" and is_not_degraded(cold_envelope) and cold_origins == ["semantic"]:
                first_state = "semantic_on_first_call"
            elif cold_valid and cold_envelope.get("degraded") is True and cold_origins == ["text"]:
                first_state = "degraded_text_fallback"
            else:
                first_state = "other_or_unavailable"

            if args.warmup_wait_seconds:
                time.sleep(args.warmup_wait_seconds)

            env["MI_LSP_SESSION_ID"] = f"ts-cold-refs-{run_index}-quality"
            quality_result = run_command(
                query_command(args.milsp, args.workspace, args.symbol, args.file, env["MI_LSP_SESSION_ID"]),
                args.workspace_root,
                env,
                args.command_timeout,
            )
            quality_envelope = parse_envelope(quality_result)
            quality = semantic_quality(quality_result, quality_envelope, expected_refs, args.workspace_root)
            warmup_success = semantic_warmup_success(quality_result, quality_envelope)
            first_latency_under_2s = (
                first_response_valid(cold, cold_envelope)
                and cold["elapsed_ms"] < LATENCY_TARGET_MS
            )
            record = {
                "run": run_index + 1,
                "daemon_start_ms": daemon_start["elapsed_ms"],
                "first_latency_under_2s": first_latency_under_2s,
                "cold_latency": {
                    "ms": cold["elapsed_ms"],
                    "under_2s": first_latency_under_2s,
                    "valid_envelope": cold_valid,
                    "valid_for_latency_contract": first_response_valid(cold, cold_envelope),
                    "timed_out": cold["timed_out"],
                    "exit_code": cold["exit_code"],
                },
                "warmup_observation": {
                    "state": first_state,
                    "backend": cold_envelope.get("backend") if cold_envelope else None,
                    "degraded": cold_envelope.get("degraded") if cold_envelope else None,
                    "fallback_used": cold_envelope.get("fallback_used") if cold_envelope else None,
                    "reason": cold_envelope.get("reason") if cold_envelope else None,
                    "origins": cold_origins,
                    "item_count": len(cold_items),
                    "stderr": cold["stderr"].strip() if not cold_valid else "",
                },
                "warmup_success": warmup_success,
                "semantic_quality": quality,
                "accepted": first_latency_under_2s and warmup_success and quality["passed"],
            }
        finally:
            env["MI_LSP_SESSION_ID"] = f"ts-cold-refs-{run_index}-daemon-stop"
            stop = run_command(
                [args.milsp, "--format", "json", "--no-auto-daemon", "daemon", "stop"],
                args.workspace_root,
                env,
                min(args.command_timeout, 15),
            )
            if record is not None:
                record["daemon_stopped"] = stop["exit_code"] == 0 and not stop["timed_out"]
                if not record["daemon_stopped"]:
                    record["accepted"] = False
                    record["cleanup_failure"] = {
                        "exit_code": stop["exit_code"],
                        "timed_out": stop["timed_out"],
                    }
            if stop["exit_code"] != 0 or stop["timed_out"]:
                print(
                    f"run {run_index + 1}: isolated daemon stop failed (exit={stop['exit_code']}, timeout={stop['timed_out']})",
                    file=sys.stderr,
                )
        return record


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--milsp", required=True, help="mi-lsp executable")
    parser.add_argument("--workspace", required=True, help="already registered TypeScript workspace alias")
    parser.add_argument("--workspace-root", required=True, type=Path, help="repo root used to start the isolated daemon")
    parser.add_argument("--symbol", required=True, help="symbol with a catalog definition")
    parser.add_argument("--file", required=True, help="TS/JS source file anchoring the symbol")
    parser.add_argument("--expected-ref", required=True, action="append", help="expected semantic reference file:line; repeatable")
    parser.add_argument("--registry", type=Path, default=DEFAULT_REGISTRY, help="read-only source registry copied into temporary HOME")
    parser.add_argument("--npm-prefix", type=Path, help="npm global prefix containing the temporary TypeScript runtime")
    parser.add_argument("--runs", type=int, default=5)
    parser.add_argument("--warmup-wait-seconds", type=float, default=3.0)
    parser.add_argument("--command-timeout", type=float, default=30.0)
    args = parser.parse_args()
    if args.runs < 1:
        parser.error("--runs must be positive")
    if args.warmup_wait_seconds < 0:
        parser.error("--warmup-wait-seconds must be non-negative")
    if args.command_timeout <= 0:
        parser.error("--command-timeout must be positive")

    try:
        expected_refs = parse_expected_refs(args.expected_ref)
        args.milsp = resolve_binary(args.milsp)
        args.workspace_root = args.workspace_root.expanduser().resolve()
        if not args.workspace_root.is_dir():
            raise FileNotFoundError(f"workspace root not found: {args.workspace_root}")
        base_env = os.environ.copy()
        if args.npm_prefix:
            args.npm_prefix = args.npm_prefix.expanduser().resolve()
            base_env["npm_config_prefix"] = str(args.npm_prefix)
        args.registry = args.registry.expanduser().resolve()
        tsserver_path, detected_npm_prefix = find_tsserver(args.workspace_root, base_env)
        if not args.npm_prefix:
            args.npm_prefix = detected_npm_prefix
    except (ValueError, FileNotFoundError, subprocess.SubprocessError) as exc:
        parser.error(str(exc))

    records = [run_one(args, index, expected_refs) for index in range(args.runs)]
    latencies = [record["cold_latency"]["ms"] for record in records if "cold_latency" in record]
    result = {
        "benchmark": "ts-cold-refs-v2",
        "workspace": args.workspace,
        "workspace_root": str(args.workspace_root),
        "symbol": args.symbol,
        "file": args.file,
        "tsserver_path": str(tsserver_path),
        "runs": records,
        "median_cold_latency_ms": round(statistics.median(latencies), 2) if latencies else None,
        "max_cold_latency_ms": round(max(latencies), 2) if latencies else None,
        "first_latency_under_2s": len(records) == args.runs and all(
            record.get("first_latency_under_2s") is True for record in records
        ),
        "warmup_success": len(records) == args.runs and all(
            record.get("warmup_success") is True for record in records
        ),
        "semantic_quality": len(records) == args.runs and all(
            "semantic_quality" in record and record["semantic_quality"]["passed"] is True
            for record in records
        ),
        "accepted_all_runs": len(records) == args.runs and all(
            record.get("accepted") is True for record in records
        ),
    }
    print(json.dumps(result, indent=2, sort_keys=True))
    return 0 if result["accepted_all_runs"] else 1


if __name__ == "__main__":
    raise SystemExit(main())
