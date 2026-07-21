import unittest

try:
    from .sanitize_v2 import sanitize_env, sanitize_error, sanitize_metrics, sanitize_process_result
    from .child_metrics import ChildMetrics, ChildRunResult
except ImportError:
    from sanitize_v2 import sanitize_env, sanitize_error, sanitize_metrics, sanitize_process_result
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
