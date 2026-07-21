import hashlib
import json
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

import runner_v2
from canonical_v2 import payload_digest, token_count
from schema_v2 import AdapterSpec, RunRecord
from security_gate import runtime_evidence_digest


class FakeAdapter:
    calls = []
    case_override = None
    repetition_override = None
    token_units_override = None
    terminal_fields_override = True

    def __init__(self, spec, manifest, root, executor=None):
        self.spec = spec

    @classmethod
    def _payload(cls):
        payload = {"stable": True}
        if cls.terminal_fields_override:
            payload.update({"ok": True, "done": True, "backend": "go", "completeness": "complete", "truncated": False})
        return payload

    def run_case(self, case, *, repetition):
        FakeAdapter.calls.append((self.spec.adapter_id, case["id"], repetition))
        payload = FakeAdapter._payload()
        runtime = {
            "status": "PASS", "runtime_proof": True, "provenance": "child_metrics_executor",
            "probe_mode": "windows_netstat_child_tree_observation", "observed_pids": [101],
            "metadata_observed_pids": [101], "sample_count": 1, "network_count": 0, "mcp_count": 0,
            "observed_network_count": 0, "observed_mcp_count": 0, "reason": None,
        }
        runtime["evidence_digest"] = runtime_evidence_digest(runtime)
        return RunRecord(
            adapter_id=self.spec.adapter_id, case_id=FakeAdapter.case_override or "", operation=case["operation"], status="PASS", repetition=repetition if FakeAdapter.repetition_override is None else FakeAdapter.repetition_override,
            canonical={"schema": "victory-canonical/v2", "operation": case["operation"], "payload": payload, "digest": payload_digest(payload), "token_units": token_count(payload) if FakeAdapter.token_units_override is None else FakeAdapter.token_units_override},
            elapsed_ms=float(repetition + 1), metrics={
                "child": {"status": "PASS", "peak_rss_bytes": 1, "tree_peak_rss_bytes": 1, "tree_supported": True,
                          "cleanup_status": "clean", "samples": 1, "timed_out": False, "crashed": False,
                          "failure_class": "none", "exit_code": 0},
                "security": {"status": "PASS", "runtime_proof": True, "runtime": runtime,
                             "integrity": {"status": "PASS"}, "source_integrity": {"status": "PASS"}},
            },
        )


class RunnerV2Tests(unittest.TestCase):
    def test_authoritative_requires_exactly_30(self):
        with self.assertRaises(ValueError):
            runner_v2.run_manifest("ignored", tempfile.mkdtemp(), repetitions=29)

    def test_authoritative_runner_requires_fresh_runtime_preflight(self):
        manifest = {
            "schema": "victory-lab-manifest/v2", "version": 2,
            "fixture_hashes": {"fixture": "a" * 64}, "oracle_hashes": {"oracle": "b" * 64},
            "adapters": [], "cases": [],
        }
        with tempfile.TemporaryDirectory() as temp, patch.object(runner_v2, "load_manifest", return_value=manifest), patch.object(runner_v2, "validate_strict_manifest"), patch.object(runner_v2, "validate_runtime", return_value=["reproduction mismatch"]):
            path = Path(temp) / "manifest.json"
            path.write_text("{}", encoding="utf-8")
            with self.assertRaisesRegex(ValueError, "runtime preflight failed"):
                runner_v2.run_manifest(path, Path(temp) / "output", repetitions=30)

    def _run_manifest_with_fake(self, *, case_override=None, repetition_override=None):
        manifest = {
            "schema": "victory-lab-manifest/v2", "version": 2,
            "fixture_hashes": {"fixture": "a" * 64}, "oracle_hashes": {"oracle": "b" * 64},
            "adapters": [{"schema": "victory-adapter-spec/v2", "adapter_id": "a", "kind": "current", "expected_commit": "c" * 40, "expected_executable_sha256": "a" * 64, "metadata_command": ["fake", "version"], "command": ["fake"], "capabilities": ["affected"], "comparable_operations": ["affected"], "env_allowlist": []}],
            "cases": [{"id": "c", "operation": "affected", "corpus": ["x"], "golden": "o", "changed_paths": ["x"]}],
            "workloads": [{"workload_id": "w", "case_id": "c", "operation": "affected", "mode": "direct"}],
            "groups": [{"group_id": "g", "adapter_id": "a", "workload_id": "w", "case_id": "c", "operation": "affected", "repetitions": 30, "authoritative": True}],
        }
        spec = AdapterSpec.from_dict(manifest["adapters"][0])
        FakeAdapter.case_override = case_override
        FakeAdapter.repetition_override = repetition_override
        with tempfile.TemporaryDirectory() as temp, patch.object(runner_v2, "load_manifest", return_value=manifest), patch.object(runner_v2, "validate_strict_manifest"), patch.object(runner_v2, "validate_runtime", return_value=[]), patch.object(runner_v2, "manifest_adapters", return_value={"a": spec}), patch.object(runner_v2, "VictoryAdapter", FakeAdapter):
            root = Path(temp)
            (root / "manifest.json").write_text("{}", encoding="utf-8")
            (root / "fixture").write_text("fixture", encoding="utf-8")
            (root / "oracle").write_text("oracle", encoding="utf-8")
            return runner_v2.run_manifest(root / "manifest.json", root / "output", repetitions=30)

    def test_runner_rejects_wrong_returned_case_and_repetition(self):
        try:
            with self.assertRaises(ValueError):
                self._run_manifest_with_fake(case_override="wrong")
            with self.assertRaises(ValueError):
                self._run_manifest_with_fake(repetition_override=29)
        finally:
            FakeAdapter.case_override = None
            FakeAdapter.repetition_override = None

    def test_runner_recomputes_positive_token_units_before_persisting(self):
        FakeAdapter.token_units_override = 1
        try:
            with self.assertRaises(ValueError):
                self._run_manifest_with_fake()
        finally:
            FakeAdapter.token_units_override = None

    def test_runner_rejects_pass_without_complete_terminal_state(self):
        FakeAdapter.terminal_fields_override = False
        try:
            with self.assertRaisesRegex(ValueError, "terminal"):
                self._run_manifest_with_fake()
        finally:
            FakeAdapter.terminal_fields_override = True

    def test_serializes_adapters_cases_and_retains_all_samples(self):
        manifest = {
            "schema": "victory-lab-manifest/v2", "version": 2,
            "fixture_hashes": {"fixture": "a" * 64}, "oracle_hashes": {"oracle": "b" * 64},
            "adapters": [{"schema": "victory-adapter-spec/v2", "adapter_id": "a", "kind": "current", "expected_commit": "c" * 40, "expected_executable_sha256": "a" * 64, "metadata_command": ["fake", "version"], "command": ["fake"], "capabilities": ["affected"], "comparable_operations": ["affected"], "env_allowlist": []}],
            "cases": [{"id": "c", "operation": "affected", "corpus": ["x"], "golden": "o", "changed_paths": ["x"]}],
            "workloads": [{"workload_id": "w", "case_id": "c", "operation": "affected", "mode": "direct"}],
            "groups": [{"group_id": "g", "adapter_id": "a", "workload_id": "w", "case_id": "c", "operation": "affected", "repetitions": 30, "authoritative": True}],
        }
        spec = AdapterSpec.from_dict(manifest["adapters"][0])
        FakeAdapter.calls = []
        with tempfile.TemporaryDirectory() as temp, patch.object(runner_v2, "load_manifest", return_value=manifest), patch.object(runner_v2, "validate_strict_manifest"), patch.object(runner_v2, "validate_runtime", return_value=[]), patch.object(runner_v2, "manifest_adapters", return_value={"a": spec}), patch.object(runner_v2, "VictoryAdapter", FakeAdapter):
            root = Path(temp)
            (root / "manifest.json").write_text("{}", encoding="utf-8")
            (root / "fixture").write_text("fixture", encoding="utf-8")
            (root / "oracle").write_text("oracle", encoding="utf-8")
            manifest_sha256 = hashlib.sha256((root / "manifest.json").read_bytes()).hexdigest()
            summary = runner_v2.run_manifest(str(root / "manifest.json"), str(root / "output"), repetitions=30)
            lines = (root / "output" / "samples.jsonl").read_text().splitlines()
        self.assertEqual(summary["samples"], 30)
        self.assertEqual(len(lines), 30)
        self.assertEqual([call[2] for call in FakeAdapter.calls], list(range(30)))
        self.assertTrue(summary["anti_gaming"]["all_samples_retained"])
        self.assertEqual(summary["manifest_path"], str((root / "manifest.json").resolve()))
        self.assertEqual(summary["manifest_sha256"], manifest_sha256)
        self.assertEqual(summary["manifest_bundle"]["path"], summary["manifest_path"])
        self.assertEqual(summary["manifest_bundle"]["id"], summary["manifest_id"])


if __name__ == "__main__":
    unittest.main()
