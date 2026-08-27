"""Unit tests for the stdlib-only wiki/code bridge campaign runner."""
from __future__ import annotations

import json
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

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

    def test_item_file_accepts_direct_nested_and_catalog_shapes(self):
        self.assertEqual(runner._item_file({"file": "src/direct.mjs"}), "src/direct.mjs")
        self.assertEqual(runner._item_file({"definition": {"file": "src/nested.mjs"}}), "src/nested.mjs")
        self.assertEqual(runner._item_file({"file_path": "src/catalog.mjs"}), "src/catalog.mjs")
        self.assertEqual(
            runner._item_file({"file": "src/direct.mjs", "definition": {"file": "src/nested.mjs"}}),
            "src/direct.mjs",
        )
        self.assertIsNone(runner._item_file({"definition": {"name": "missing"}}))

    def test_operation_stats_ms_extracts_and_validates_envelope_value(self):
        self.assertEqual(runner.operation_stats_ms({"stats": {"ms": 12}}), 12.0)
        self.assertEqual(runner.operation_stats_ms({"stats": {"ms": 0.5}}), 0.5)
        with self.assertRaises(runner.RunnerBlocked):
            runner.operation_stats_ms({"stats": {}})
        with self.assertRaises(runner.RunnerFailure):
            runner.operation_stats_ms({"stats": {"ms": -1}})
        with self.assertRaises(runner.RunnerFailure):
            runner.operation_stats_ms({"stats": {"ms": True}})

    def test_build_batch_operations_constructs_exactly_30_identical_requests(self):
        operations = runner.build_batch_operations("nav.wiki.trace", {"rf": "RF-DEMO-001"})
        self.assertEqual(len(operations), runner.RUNS)
        self.assertEqual([item["id"] for item in operations], [f"warm-{index:02d}" for index in range(30)])
        self.assertEqual({item["op"] for item in operations}, {"nav.wiki.trace"})
        self.assertEqual({json.dumps(item["params"], sort_keys=True) for item in operations}, {json.dumps({"rf": "RF-DEMO-001"}, sort_keys=True)})
        with self.assertRaises(runner.RunnerFailure):
            runner.build_batch_operations("nav.batch", {})

    def test_extract_batch_stats_returns_exact_samples_and_p95(self):
        operations = runner.build_batch_operations("nav.related", {"symbol": "runDemo", "depth": "definition"})
        batch = {
            "ok": True,
            "truncated": False,
            "items": [
                {
                    "id": operation["id"],
                    "op": operation["op"],
                    "duration_ms": value,
                    "envelope": {"ok": True, "stats": {"ms": value}},
                }
                for operation, value in zip(operations, range(runner.RUNS))
            ],
        }
        samples = runner.extract_batch_stats(batch, operations)
        self.assertEqual(len(samples), runner.RUNS)
        self.assertEqual(samples, [float(value) for value in range(30)])
        self.assertEqual(runner.p95(samples), 28.0)

    def test_measured_stats_uses_one_sequential_batch_and_returns_samples(self):
        operations = runner.build_batch_operations("nav.neighbors", {"selector": "RF-DEMO-001", "depth": 1, "limit": 20})
        batch = {
            "ok": True,
            "truncated": False,
            "items": [
                {
                    "id": operation["id"],
                    "op": operation["op"],
                    "duration_ms": value,
                    "envelope": {"ok": True, "stats": {"ms": value}},
                }
                for operation, value in zip(operations, range(runner.RUNS))
            ],
        }
        with patch.object(runner, "run_command", return_value=batch) as run_command:
            measured_p95, samples = runner.measured_stats(
                Path("/tmp/mi-lsp"),
                Path("/tmp/workspace"),
                "nav.neighbors",
                {"selector": "RF-DEMO-001", "depth": 1, "limit": 20},
            )
        self.assertEqual(measured_p95, 28.0)
        self.assertEqual(samples, [float(value) for value in range(30)])
        run_command.assert_called_once()
        call = run_command.call_args
        self.assertEqual(
            call.args[2],
            [
                "nav",
                "batch",
                "--sequential",
                "--max-items",
                "30",
                "--max-chars",
                "10485760",
            ],
        )
        self.assertEqual(json.loads(call.kwargs["input_data"]), operations)

    def test_extract_batch_stats_rejects_typed_missing_invalid_and_mismatched_results(self):
        operations = runner.build_batch_operations("nav.wiki.trace", {"rf": "RF-DEMO-001"})
        valid_items = [
            {
                "id": operation["id"],
                "op": operation["op"],
                "duration_ms": 1,
                "envelope": {"ok": True, "stats": {"ms": 1}},
            }
            for operation in operations
        ]

        with self.assertRaises(runner.RunnerBlocked):
            runner.extract_batch_stats({"items": []}, operations)
        with self.assertRaises(runner.RunnerBlocked):
            runner.extract_batch_stats({"ok": True}, operations)
        with self.assertRaises(runner.RunnerFailure):
            runner.extract_batch_stats({"ok": False, "items": []}, operations)
        with self.assertRaises(runner.RunnerFailure):
            runner.extract_batch_stats({"ok": True, "items": {}}, operations)
        with self.assertRaises(runner.RunnerFailure):
            runner.extract_batch_stats({"ok": True, "items": valid_items[:-1]}, operations)

        wrong_order = list(valid_items)
        wrong_order[1] = dict(wrong_order[1], id="warm-wrong")
        with self.assertRaises(runner.RunnerFailure):
            runner.extract_batch_stats({"ok": True, "items": wrong_order}, operations)

        item_error = list(valid_items)
        item_error[0] = dict(item_error[0], error="operation failed")
        with self.assertRaises(runner.RunnerFailure):
            runner.extract_batch_stats({"ok": True, "items": item_error}, operations)

        missing_envelope = list(valid_items)
        missing_envelope[0] = {"id": operations[0]["id"], "op": operations[0]["op"], "duration_ms": 1}
        with self.assertRaises(runner.RunnerBlocked):
            runner.extract_batch_stats({"ok": True, "items": missing_envelope}, operations)

        invalid_envelope = list(valid_items)
        invalid_envelope[0] = dict(invalid_envelope[0], envelope=[])
        with self.assertRaises(runner.RunnerFailure):
            runner.extract_batch_stats({"ok": True, "items": invalid_envelope}, operations)

        missing_envelope_ok = list(valid_items)
        missing_envelope_ok[0] = dict(missing_envelope_ok[0], envelope={})
        with self.assertRaises(runner.RunnerBlocked):
            runner.extract_batch_stats({"ok": True, "items": missing_envelope_ok}, operations)

        failed_envelope = list(valid_items)
        failed_envelope[0] = dict(failed_envelope[0], envelope={"ok": False})
        with self.assertRaises(runner.RunnerFailure):
            runner.extract_batch_stats({"ok": True, "items": failed_envelope}, operations)

        missing_duration = list(valid_items)
        missing_duration[0] = dict(missing_duration[0])
        del missing_duration[0]["duration_ms"]
        with self.assertRaises(runner.RunnerBlocked):
            runner.extract_batch_stats({"ok": True, "items": missing_duration}, operations)

        for invalid_duration in ("slow", True, -1, float("nan"), float("inf")):
            with self.subTest(invalid_duration=invalid_duration):
                invalid_items = list(valid_items)
                invalid_items[0] = dict(invalid_items[0], duration_ms=invalid_duration)
                with self.assertRaises(runner.RunnerFailure):
                    runner.extract_batch_stats({"ok": True, "items": invalid_items}, operations)

        nested_stats_omitted = list(valid_items)
        nested_stats_omitted[0] = dict(nested_stats_omitted[0], envelope={"ok": True})
        self.assertEqual(
            runner.extract_batch_stats({"ok": True, "items": nested_stats_omitted}, operations),
            [1.0] * runner.RUNS,
        )

        nested_stats_lower = list(valid_items)
        nested_stats_lower[0] = dict(nested_stats_lower[0], envelope={"ok": True, "stats": {"ms": 0}})
        self.assertEqual(
            runner.extract_batch_stats({"ok": True, "items": nested_stats_lower}, operations),
            [1.0] * runner.RUNS,
        )

        invalid_stats = list(valid_items)
        invalid_stats[0] = dict(invalid_stats[0], envelope={"ok": True, "stats": {"ms": "slow"}})
        with self.assertRaises(runner.RunnerFailure):
            runner.extract_batch_stats({"ok": True, "items": invalid_stats}, operations)

        mismatched_stats = list(valid_items)
        mismatched_stats[0] = dict(mismatched_stats[0], envelope={"ok": True, "stats": {"ms": 2}})
        with self.assertRaises(runner.RunnerFailure):
            runner.extract_batch_stats({"ok": True, "items": mismatched_stats}, operations)

        for omission_code in ("max_items", "char_budget"):
            with self.subTest(omission_code=omission_code):
                with self.assertRaises(runner.RunnerFailure):
                    runner.extract_batch_stats(
                        {
                            "ok": True,
                            "truncated": False,
                            "omissions": [{"error_code": omission_code}],
                            "items": valid_items,
                        },
                        operations,
                    )

        with self.assertRaises(runner.RunnerFailure):
            runner.extract_batch_stats(
                {"ok": True, "truncated": True, "items": valid_items},
                operations,
            )

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
