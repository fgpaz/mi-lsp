"""Unit tests for the stdlib-only wiki/code bridge campaign runner."""
from __future__ import annotations

import json
import tempfile
import unittest
from pathlib import Path

try:
    import runner
except ModuleNotFoundError:  # pragma: no cover - package discovery path
    from scripts.bench.wiki_code_bridge import runner


class WikiCodeBridgeRunnerTests(unittest.TestCase):
    def test_inventory_covers_exactly_the_37_locked_cases(self):
        self.assertEqual(len(runner.ACCEPTANCE_CASES), 37)
        self.assertEqual(len(set(runner.ACCEPTANCE_CASES)), 37)
        self.assertIn("direct_daemon_parity", runner.ACCEPTANCE_CASES)
        self.assertIn("warm_mixed_neighbors_p95", runner.ACCEPTANCE_CASES)
        self.assertIn("dirty_single_file_overlay_target", runner.ACCEPTANCE_CASES)

    def test_p95_requires_30_samples_and_uses_nearest_rank(self):
        self.assertEqual(runner.p95(range(30)), 28.0)
        with self.assertRaises(ValueError):
            runner.p95(range(29))

    def test_latency_unmet_is_fail_with_residual_risk(self):
        result = runner.latency_result(250.001, 250.0, "dirty_single_file_overlay")
        self.assertEqual(result["status"], "FAIL")
        self.assertEqual(result["samples"], 30)
        self.assertEqual(result["residual_risk"], "dirty_single_file_overlay_target_unmet")

    def test_latency_pass_requires_measured_samples(self):
        blocked = runner.latency_result(None, 100.0, "warm_direct_binding_lookup")
        self.assertEqual(blocked["status"], "BLOCKED")
        self.assertEqual(blocked["samples"], 0)
        passed = runner.latency_result(99.999, 100.0, "warm_direct_binding_lookup")
        self.assertEqual(passed["status"], "PASS")
        self.assertEqual(passed["samples"], 30)

    def test_blocked_summary_is_typed_and_has_no_host_path(self):
        summary = runner.blocked_summary("python_unavailable")
        self.assertEqual(summary["status"], "BLOCKED")
        self.assertEqual(summary["reason_code"], "python_unavailable")
        self.assertEqual(len(summary["acceptance_case_inventory"]), 37)
        self.assertNotIn("/tmp", json.dumps(summary))

    def test_sanitize_value_drops_logs_secrets_and_absolute_paths(self):
        value = runner.sanitize_value({
            "stdout": "raw output",
            "admin_token": "do-not-persist",
            "path": "/tmp/fixture/src/service.mjs",
            "nested": {"stderr": "diagnostic", "status": "PASS"},
        })
        self.assertNotIn("stdout", value)
        self.assertNotIn("admin_token", value)
        self.assertEqual(value["path"], "<absolute_path_omitted>")
        self.assertEqual(value["nested"], {"status": "PASS"})

    def test_sanitize_value_rewrites_campaign_workspace_paths(self):
        value = runner.sanitize_value({"path": "/tmp/fixture/src/service.mjs"}, "/tmp/fixture")
        self.assertEqual(value["path"], "<workspace>/src/service.mjs")

    def test_file_snapshot_detects_db_and_wal_changes(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            state = root / ".mi-lsp"
            state.mkdir()
            (state / "index.db").write_bytes(b"db-v1")
            before = runner.file_snapshot(root)
            (state / "index.db-wal").write_bytes(b"wal-v1")
            after = runner.file_snapshot(root)
            self.assertNotEqual(before, after)

    def test_fixture_tree_digest_is_content_and_relative_path_bound(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / "src").mkdir()
            (root / "src/service.mjs").write_text("export function runDemo() {}\n", encoding="utf-8")
            first = runner.tree_digest(root)
            (root / "src/other.mjs").write_text("export function other() {}\n", encoding="utf-8")
            second = runner.tree_digest(root)
            self.assertNotEqual(first, second)

    def test_cli_prefix_forces_direct_read_only_mode(self):
        command = runner._command_prefix(Path("/tmp/mi-lsp"), Path("workspace"))
        self.assertIn("--no-daemon", command)
        self.assertIn("--no-auto-daemon", command)
        self.assertIn("--format", command)
        self.assertIn("--workspace", command)

    def test_main_writes_typed_blocked_result_without_running_binary(self):
        with tempfile.TemporaryDirectory() as directory:
            output = Path(directory) / "summary.json"
            code = runner.main(["--binary", str(Path(directory) / "missing"), "--output", str(output)])
            self.assertEqual(code, 2)
            summary = json.loads(output.read_text(encoding="utf-8"))
            self.assertEqual(summary["status"], "BLOCKED")
            self.assertEqual(summary["reason_code"], "binary_unavailable")
            self.assertNotIn(str(Path(directory)), output.read_text(encoding="utf-8"))


if __name__ == "__main__":
    unittest.main()
