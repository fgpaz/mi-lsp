import tempfile
from pathlib import Path
import unittest

try:
    from .security_gate import SecurityGate, compare_snapshots, scan_command_env, snapshot_paths
except ImportError:
    from security_gate import SecurityGate, compare_snapshots, scan_command_env, snapshot_paths


class SecurityGateV2Tests(unittest.TestCase):
    def test_before_after_fixture_hashes_detect_no_write_and_write(self):
        with tempfile.TemporaryDirectory() as temp:
            fixture = Path(temp) / "fixture.txt"
            fixture.write_text("stable", encoding="utf-8")
            before = snapshot_paths({"fixture": fixture})
            after = snapshot_paths({"fixture": fixture})
            self.assertEqual(compare_snapshots(before, after)["status"], "PASS")
            fixture.write_text("changed", encoding="utf-8")
            changed = compare_snapshots(before, snapshot_paths({"fixture": fixture}))
            self.assertEqual(changed["status"], "FAIL")
            self.assertEqual(changed["reason_code"], "protected_path_changed")

    def test_command_and_env_advisory_indicators_are_bounded(self):
        result = scan_command_env(["tool", "https://example.invalid", "mcp"], {"API_TOKEN": "secret"})
        self.assertEqual(result["status"], "FAIL")
        self.assertEqual(result["scan_mode"], "static_advisory")
        self.assertFalse(result["runtime_proof"])
        self.assertEqual(set(result["reason_codes"]), {"mcp_indicator", "network_indicator", "secret_indicator"})
        self.assertNotIn("example.invalid", repr(result))
        self.assertNotIn("API_TOKEN", repr(result))

    def test_gate_is_fail_closed_for_missing_start(self):
        with self.assertRaises(RuntimeError):
            SecurityGate().finish()

    def test_gate_reports_advisory_not_runtime_proof(self):
        with tempfile.TemporaryDirectory() as temp:
            fixture = Path(temp) / "fixture.txt"
            fixture.write_text("stable", encoding="utf-8")
            gate = SecurityGate({"fixture": fixture})
            start = gate.start(["safe-tool"], {"PATH": "redacted"})
            finish = gate.finish(["safe-tool", "mcp"], {"PATH": "redacted"})
        self.assertEqual(finish["status"], "PASS")
        self.assertEqual(finish["advisory_scan"]["status"], "FAIL")
        self.assertFalse(start["advisory_scan"]["runtime_proof"])
        self.assertFalse(finish["runtime_proof"])
        self.assertNotIn(str(fixture), repr(start))
        self.assertNotIn(str(fixture), repr(finish))


if __name__ == "__main__":
    unittest.main()
