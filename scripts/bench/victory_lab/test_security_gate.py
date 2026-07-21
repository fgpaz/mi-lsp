from unittest.mock import patch
import json
import tempfile
from pathlib import Path
import subprocess
import unittest

try:
    from . import security_gate as security_gate_module
    from .security_gate import RuntimeProofProbe, SecurityGate, runtime_evidence_digest
except ImportError:
    import security_gate as security_gate_module
    from security_gate import RuntimeProofProbe, SecurityGate, runtime_evidence_digest


class RuntimeProofFocusedTests(unittest.TestCase):
    def _probe(self, pid_provider, process_observer):
        probe = RuntimeProofProbe(
            101,
            pid_provider=pid_provider,
            process_observer=process_observer,
            provenance="child_metrics_executor",
        )
        probe.available = True
        return probe

    def test_root_metadata_is_observed_before_netstat(self):
        events = []

        def observe(pids):
            events.append("metadata")
            return {101: {"image": "child.exe"}}

        def run(*args, **kwargs):
            events.append("netstat")
            return subprocess.CompletedProcess(args[0], 0, "", "")

        probe = self._probe(lambda: {101}, observe)
        with patch(f"{security_gate_module.__name__}.subprocess.run", side_effect=run):
            probe._sample()

        self.assertEqual(events, ["metadata", "netstat"])
        self.assertEqual(probe.samples, 1)

    def test_missing_root_metadata_never_counts_as_a_sample(self):
        probe = self._probe(lambda: {101}, lambda pids: {})
        with patch(f"{security_gate_module.__name__}.subprocess.run") as netstat:
            probe._sample()
            result = probe.finish()

        netstat.assert_not_called()
        self.assertEqual(result["status"], "NOT_COMPARABLE")
        self.assertEqual(result["reason_code"], "root_metadata_missing")
        self.assertEqual(result["sample_count"], 0)

    def test_start_retries_transient_root_metadata_before_background_observer(self):
        calls = []

        def observe(pids):
            calls.append(tuple(sorted(pids)))
            return {} if len(calls) == 1 else {101: {"image": "child.exe"}}

        probe = self._probe(lambda: {101}, observe)
        with patch(
            f"{security_gate_module.__name__}.subprocess.run",
            return_value=subprocess.CompletedProcess(["netstat", "-ano"], 0, "", ""),
        ):
            probe.start()
            result = probe.finish()

        self.assertGreaterEqual(len(calls), 2)
        self.assertEqual(result["status"], "PASS")
        self.assertTrue(result["runtime_proof"])
        self.assertEqual(result["metadata_observed_pids"], [101])
        self.assertGreater(result["sample_count"], 0)

    def test_all_observed_pids_with_metadata_can_produce_runtime_pass(self):
        probe = self._probe(
            lambda: {101, 202},
            lambda pids: {
                101: {"image": "child.exe"},
                202: {"image": "worker.exe"},
            },
        )
        with patch(f"{security_gate_module.__name__}.subprocess.run", return_value=subprocess.CompletedProcess(["netstat", "-ano"], 0, "", "")):
            probe._sample()
            result = probe.finish()

        self.assertEqual(result["status"], "PASS")
        self.assertTrue(result["runtime_proof"])
        self.assertEqual(result["observed_pids"], [101, 202])
        self.assertEqual(result["metadata_observed_pids"], [101, 202])

    def test_missing_child_metadata_cannot_produce_runtime_pass(self):
        probe = self._probe(
            lambda: {101, 202},
            lambda pids: {101: {"image": "child.exe"}},
        )
        with patch(f"{security_gate_module.__name__}.subprocess.run", return_value=subprocess.CompletedProcess(["netstat", "-ano"], 0, "", "")):
            probe._sample()
            result = probe.finish()

        self.assertEqual(result["status"], "NOT_COMPARABLE")
        self.assertFalse(result["runtime_proof"])
        self.assertEqual(result["reason_code"], "metadata_missing")
        self.assertEqual(result["observed_pids"], [101, 202])
        self.assertEqual(result["metadata_observed_pids"], [101])

    def test_bounded_metadata_reread_recovers_transient_pid_lookup_race(self):
        responses = [
            {101: {"image": "child.exe"}},
            {101: {"image": "child.exe"}, 202: {"image": "worker.exe"}},
        ]
        probe = self._probe(lambda: {101, 202}, lambda pids: responses.pop(0))
        with patch(f"{security_gate_module.__name__}.subprocess.run", return_value=subprocess.CompletedProcess(["netstat", "-ano"], 0, "", "")):
            probe._sample()
            result = probe.finish()

        self.assertEqual(result["status"], "PASS")
        self.assertTrue(result["runtime_proof"])
        self.assertEqual(result["observed_pids"], [101, 202])
        self.assertEqual(result["metadata_observed_pids"], [101, 202])

    def test_pid_disappearance_is_reported_as_sanitized_observer_race(self):
        probe = self._probe(lambda: set(), lambda pids: {101: {"image": "child.exe"}})
        probe._sample()
        result = probe.finish()

        self.assertEqual(result["status"], "NOT_COMPARABLE")
        self.assertEqual(result["reason_code"], "observer_race")
        self.assertEqual(result["sample_count"], 0)

    def test_observer_exception_does_not_escape_or_leak_diagnostic_text(self):
        def observe(_pids):
            raise RuntimeError("C:\\private\\patient\\secret@example.com")

        probe = self._probe(lambda: {101}, observe)
        probe._sample()
        result = probe.finish()

        self.assertEqual(result["reason_code"], "observer_race")
        self.assertNotIn("patient", repr(result))
        self.assertNotIn("secret@example.com", repr(result))


class SecurityGateFocusedTests(unittest.TestCase):
    def _runtime(self):
        runtime = {
            "status": "PASS", "runtime_proof": True, "provenance": "child_metrics_executor",
            "probe_mode": "windows_netstat_child_tree_observation", "observed_pids": [101],
            "metadata_observed_pids": [101], "sample_count": 1,
            "observed_network_count": 0, "observed_mcp_count": 0, "reason_code": None,
        }
        runtime["evidence_digest"] = runtime_evidence_digest(runtime)
        return runtime

    def test_runtime_digest_is_recomputed_and_pid_coverage_is_required(self):
        with tempfile.TemporaryDirectory() as temp:
            fixture = Path(temp) / "fixture.txt"
            fixture.write_text("stable", encoding="utf-8")
            gate = SecurityGate({"fixture": fixture})
            gate.start()
            runtime = self._runtime()
            result = gate.finish(runtime_observation=runtime)
            self.assertEqual(result["status"], "PASS")
            runtime["observed_network_count"] = 1
            self.assertEqual(gate.finish(runtime_observation=runtime)["status"], "NOT_COMPARABLE")
            runtime = self._runtime()
            runtime["metadata_observed_pids"] = []
            runtime["evidence_digest"] = runtime_evidence_digest(runtime)
            self.assertEqual(gate.finish(runtime_observation=runtime)["status"], "NOT_COMPARABLE")

    def test_fallback_preserves_sanitized_reason_and_counters(self):
        with tempfile.TemporaryDirectory() as temp:
            fixture = Path(temp) / "fixture.txt"
            fixture.write_text("stable", encoding="utf-8")
            gate = SecurityGate({"fixture": fixture})
            gate.start()
            result = gate.finish(
                runtime_observation={
                    "status": "PASS",
                    "runtime_proof": True,
                    "reason_code": "observer_race",
                    "sample_count": 4,
                    "observed_network_count": 2,
                    "observed_mcp_count": 1,
                    "probe_errors": 3,
                    "observed_pids": [101, 202],
                    "metadata_observed_pids": [101, 202],
                },
            )

        self.assertEqual(result["status"], "NOT_COMPARABLE")
        self.assertEqual(result["runtime"]["reason_code"], "runtime_proof_unavailable")
        self.assertEqual(result["runtime"]["sample_count"], 4)
        self.assertEqual(result["runtime"]["observed_network_count"], 2)
        self.assertEqual(result["runtime"]["observed_mcp_count"], 1)
        self.assertNotIn("probe_errors", result["runtime"])
        self.assertEqual(result["runtime"]["observed_pids"], [101, 202])

    def test_gate_rejects_runtime_pass_when_child_metadata_is_missing(self):
        with tempfile.TemporaryDirectory() as temp:
            fixture = Path(temp) / "fixture.txt"
            fixture.write_text("stable", encoding="utf-8")
            gate = SecurityGate({"fixture": fixture})
            gate.start()
            result = gate.finish(
                runtime_observation={
                    "status": "PASS",
                    "runtime_proof": True,
                    "provenance": "child_metrics_executor",
                    "sample_count": 1,
                    "observed_network_count": 0,
                    "observed_mcp_count": 0,
                    "observed_pids": [101, 202],
                    "metadata_observed_pids": [101],
                    "evidence_digest": "a" * 64,
                },
            )

        self.assertEqual(result["status"], "NOT_COMPARABLE")
        self.assertFalse(result["runtime_proof"])
        self.assertEqual(result["runtime"]["reason_code"], "metadata_missing")
        self.assertEqual(result["runtime"]["observed_pids"], [101, 202])
        self.assertEqual(result["runtime"]["metadata_observed_pids"], [101])

    def test_fallback_rejects_unbounded_reason_without_leaking_it(self):
        with tempfile.TemporaryDirectory() as temp:
            fixture = Path(temp) / "fixture.txt"
            fixture.write_text("stable", encoding="utf-8")
            gate = SecurityGate({"fixture": fixture})
            gate.start()
            result = gate.finish(
                runtime_observation={
                    "reason_code": "C:\\private\\patient\\secret@example.com",
                    "sample_count": 1,
                    "observed_network_count": 0,
                    "observed_mcp_count": 0,
                },
            )

        self.assertEqual(result["runtime"]["reason_code"], "runtime_proof_unavailable")
        self.assertNotIn("patient", repr(result))
        self.assertNotIn("secret@example.com", repr(result))

    def test_fallback_reason_and_digest_survive_json_round_trip(self):
        with tempfile.TemporaryDirectory() as temp:
            fixture = Path(temp) / "fixture.txt"
            fixture.write_text("stable", encoding="utf-8")
            gate = SecurityGate({"fixture": fixture})
            gate.start()
            result = gate.finish(
                runtime_observation={
                    "status": "PASS",
                    "runtime_proof": True,
                    "reason_code": "runtime_proof_unavailable",
                    "sample_count": 1,
                    "observed_network_count": 0,
                    "observed_mcp_count": 0,
                    "observed_pids": [101],
                    "metadata_observed_pids": [101],
                },
            )

        runtime = json.loads(json.dumps(result["runtime"], sort_keys=True))
        self.assertEqual(runtime["reason_code"], "runtime_proof_unavailable")
        self.assertEqual(runtime_evidence_digest(runtime), runtime["evidence_digest"])


if __name__ == "__main__":
    unittest.main()
