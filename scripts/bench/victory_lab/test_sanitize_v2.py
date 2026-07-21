import json
import unittest

try:
    from .sanitize_v2 import RUNTIME_SECURITY_KEYS, project_runtime_security, sanitize_env, sanitize_error, sanitize_metrics, sanitize_process_result, validate_runtime_security_keys
    from .security_gate import runtime_evidence_digest
    from .child_metrics import ChildMetrics, ChildRunResult
except ImportError:
    from sanitize_v2 import RUNTIME_SECURITY_KEYS, project_runtime_security, sanitize_env, sanitize_error, sanitize_metrics, sanitize_process_result, validate_runtime_security_keys
    from security_gate import runtime_evidence_digest
    from child_metrics import ChildMetrics, ChildRunResult


class SanitizeV2Tests(unittest.TestCase):
    def test_env_allowlist_persists_names_not_values(self):
        self.assertEqual(
            sanitize_env({"PATH": "safe", "TOKEN": "do-not-store", "UNDECLARED": "x"}, ["PATH", "TOKEN"]),
            ["PATH"],
        )

    def test_metrics_drop_raw_payloads_secrets_and_paths(self):
        metrics = sanitize_metrics({
            "peak_rss_bytes": 123,
            "fixture_digest": "a" * 64,
            "request_id": "rid-1",
            "reason_code": "counter unavailable",
            "stdout": "secret output",
            "source_payload": "source",
            "absolute_path": "C:\\Users\\alice\\fixture",
            "api_token": "value",
            "nan": float("nan"),
        })
        self.assertEqual(metrics["peak_rss_bytes"], 123)
        self.assertEqual(metrics["fixture_digest"], "a" * 64)
        self.assertEqual(metrics["request_id"], "rid-1")
        self.assertNotIn("stdout", metrics)
        self.assertNotIn("source_payload", metrics)
        self.assertNotIn("absolute_path", metrics)
        self.assertNotIn("api_token", metrics)
        self.assertNotIn("nan", metrics)

    def test_nested_path_and_secret_values_are_dropped(self):
        metrics = sanitize_metrics({
            "security": {
                "status": "PASS",
                "changed_path_ids": ["C:\\Users\\alice\\fixture"],
                "details": {"absolute_path": "C:\\Users\\alice\\fixture"},
            },
            "error_reason": "C:\\Users\\alice\\secret-token.txt",
        })
        self.assertEqual(metrics["security"]["status"], "PASS")
        self.assertNotIn("changed_path_ids", metrics["security"])
        self.assertNotIn("details", metrics["security"])
        self.assertNotIn("error_reason", metrics)
        self.assertEqual(sanitize_error("spawn", "C:\\Users\\alice\\secret-token.txt"), {"kind": "spawn", "reason_code": "spawn"})

    def test_runtime_projection_keeps_only_bounded_contract_and_rehashes_after_projection(self):
        raw = {
            "status": "PASS", "runtime_proof": True, "provenance": "child_metrics_executor",
            "probe_mode": "windows_netstat_child_tree_observation", "observed_pids": [202, 101, 101, -1],
            "metadata_observed_pids": [101, 202, 0], "sample_count": 2,
            "observed_network_count": 0, "observed_mcp_count": 0, "reason_code": None,
            "argv": ["C:\\Users\\alice\\tool.exe", "--token", "secret"],
            "stdout": "alice@example.com", "stderr": "patient record", "paths": ["C:\\private\\x"],
            "env": {"TOKEN": "secret"}, "probe_errors": 99,
        }
        projected = project_runtime_security(raw)
        self.assertEqual(set(projected), RUNTIME_SECURITY_KEYS)
        self.assertEqual(projected["observed_pids"], [101, 202])
        self.assertEqual(projected["metadata_observed_pids"], [101, 202])
        self.assertEqual(projected["reason_code"], None)
        self.assertEqual(projected["evidence_digest"], runtime_evidence_digest(projected))
        self.assertNotIn("alice@example.com", repr(projected))
        self.assertNotIn("private", repr(projected))
        self.assertNotIn("TOKEN", repr(projected))

    def test_runtime_projection_round_trip_keeps_only_canonical_names(self):
        projected = project_runtime_security({
            "status": "PASS", "runtime_proof": True, "provenance": "child_metrics_executor",
            "probe_mode": "windows_netstat_child_tree_observation", "observed_pids": [101],
            "metadata_observed_pids": [101], "sample_count": 1,
            "observed_network_count": 0, "observed_mcp_count": 0, "reason_code": None,
            "network_count": 99, "mcp_count": 99, "reason": "tampered",
        })
        round_tripped = json.loads(json.dumps(projected, sort_keys=True))
        self.assertEqual(set(round_tripped), RUNTIME_SECURITY_KEYS)
        self.assertNotIn("network_count", round_tripped)
        self.assertNotIn("mcp_count", round_tripped)
        self.assertNotIn("reason", round_tripped)
        self.assertEqual(runtime_evidence_digest(round_tripped), round_tripped["evidence_digest"])

    def test_runtime_projection_requires_exact_keys_after_json_round_trip(self):
        projected = project_runtime_security({
            "status": "PASS", "runtime_proof": True, "provenance": "child_metrics_executor",
            "probe_mode": "windows_netstat_child_tree_observation", "observed_pids": [101],
            "metadata_observed_pids": [101], "sample_count": 1, "observed_network_count": 0,
            "observed_mcp_count": 0, "reason_code": None,
        })
        round_tripped = json.loads(json.dumps(projected, sort_keys=True))
        validate_runtime_security_keys(round_tripped)
        self.assertIsNone(round_tripped["reason_code"])
        for extra in ("network_count", "mcp_count", "reason", "unknown_runtime_key"):
            invalid = dict(round_tripped)
            invalid[extra] = None
            with self.assertRaisesRegex(ValueError, "runtime security projection keys"):
                validate_runtime_security_keys(invalid)
        missing_reason = dict(round_tripped)
        del missing_reason["reason_code"]
        with self.assertRaisesRegex(ValueError, "runtime security projection keys"):
            validate_runtime_security_keys(missing_reason)

    def test_runtime_projection_forces_nc_for_unproven_child(self):
        projected = project_runtime_security({
            "status": "PASS", "runtime_proof": True, "provenance": "other",
            "probe_mode": "unknown", "observed_pids": [101], "metadata_observed_pids": [],
            "sample_count": 1, "observed_network_count": 0, "observed_mcp_count": 0,
            "reason_code": None,
        })
        self.assertEqual(projected["status"], "NOT_COMPARABLE")
        self.assertFalse(projected["runtime_proof"])
        self.assertEqual(projected["reason_code"], "metadata_missing")

    def test_process_projection_hashes_output_and_omits_values_and_paths(self):
        result = ChildRunResult(
            argv=["C:\\Users\\alice\\tool.exe", "--token", "secret"], cwd="C:\\Users\\alice",
            env_keys=["PATH", "TOKEN"], returncode=0, stdout="raw stdout", stderr="raw stderr",
            metrics=ChildMetrics(peak_rss_bytes=42, status="PASS"),
        )
        sanitized = sanitize_process_result(result, env_allowlist=["PATH", "TOKEN"])
        text = repr(sanitized)
        self.assertNotIn("raw stdout", text)
        self.assertNotIn("raw stderr", text)
        self.assertNotIn("Users", text)
        self.assertEqual(sanitized["env_keys"], ["PATH"])
        self.assertEqual(sanitized["metrics"]["peak_rss_bytes"], 42)
        self.assertEqual(len(sanitized["stdout_sha256"]), 64)


if __name__ == "__main__":
    unittest.main()
