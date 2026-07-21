import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

try:
    from .child_metrics import ChildMetrics, run_child
except ImportError:
    from child_metrics import ChildMetrics, run_child


class ChildMetricsV2Tests(unittest.TestCase):
    def _env(self):
        return {"PATH": os.environ.get("PATH", ""), "TEMP": os.environ.get("TEMP", ""), "TMP": os.environ.get("TMP", "")}

    def test_short_real_child_has_truthful_status(self):
        with tempfile.TemporaryDirectory() as temp:
            result = run_child(
                [sys.executable, "-c", "import time; time.sleep(0.12); print('transient-output')"],
                cwd=Path(temp), env=self._env(), timeout_seconds=2,
            )
        self.assertIsInstance(result.metrics, ChildMetrics)
        self.assertEqual(result.returncode, 0)
        self.assertNotIn("transient-output", str(result.metrics.to_dict()))
        if os.name == "nt":
            self.assertEqual(result.metrics.status, "PASS")
            self.assertIsInstance(result.metrics.peak_rss_bytes, int)
            self.assertGreater(result.metrics.peak_rss_bytes, 0)
        else:
            self.assertEqual(result.metrics.status, "NOT_COMPARABLE")
            self.assertIsNone(result.metrics.peak_rss_bytes)

    def test_timeout_terminates_child_tree_and_preserves_class(self):
        with tempfile.TemporaryDirectory() as temp:
            result = run_child(
                [sys.executable, "-c", "import time; time.sleep(10)"],
                cwd=Path(temp), env=self._env(), timeout_seconds=0.08,
            )
        self.assertTrue(result.timed_out)
        self.assertEqual(result.metrics.failure_class, "timeout")
        self.assertIn(result.metrics.cleanup_status, {"clean", "forced"})
        self.assertIsNotNone(result.metrics.exit_code)

    def test_nonzero_exit_is_not_misreported_as_harness_crash(self):
        with tempfile.TemporaryDirectory() as temp:
            result = run_child(
                [sys.executable, "-c", "raise SystemExit(23)"],
                cwd=Path(temp), env=self._env(), timeout_seconds=2,
            )
        self.assertEqual(result.returncode, 23)
        self.assertFalse(result.timed_out)
        self.assertEqual(result.metrics.failure_class, "exit_nonzero")
        self.assertEqual(result.metrics.exit_code, 23)

    def test_crash_preserves_crash_class(self):
        with tempfile.TemporaryDirectory() as temp:
            result = run_child(
                [sys.executable, "-c", "import os; os.abort()"],
                cwd=Path(temp), env=self._env(), timeout_seconds=2,
            )
        self.assertFalse(result.timed_out)
        self.assertTrue(result.metrics.crashed)
        self.assertEqual(result.metrics.failure_class, "crash")

    def test_spawn_error_has_distinct_failure_class(self):
        with tempfile.TemporaryDirectory() as temp:
            result = run_child(
                [str(Path(temp) / "missing-child.exe")], cwd=Path(temp), env=self._env(), timeout_seconds=2,
            )
        self.assertEqual(result.returncode, 127)
        self.assertEqual(result.metrics.failure_class, "spawn_error")
        self.assertTrue(result.metrics.crashed)

    def test_unavailable_counter_never_falls_back_to_harness_rss(self):
        with tempfile.TemporaryDirectory() as temp:
            result = run_child(
                [sys.executable, "-c", "pass"], cwd=Path(temp), env=self._env(), timeout_seconds=2,
            )
        if os.name != "nt":
            self.assertEqual(result.metrics.reason, "unsupported_platform")
            self.assertIsNone(result.metrics.peak_rss_bytes)


if __name__ == "__main__":
    unittest.main()
