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
    runtime_extra = None
    abort_on_repetition = None

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
        if FakeAdapter.abort_on_repetition == repetition:
            raise RuntimeError("simulated adapter abort with sensitive path C:\\secret")
        payload = FakeAdapter._payload()
        runtime = {
            "status": "PASS", "runtime_proof": True, "provenance": "child_metrics_executor",
            "probe_mode": "windows_netstat_child_tree_observation", "observed_pids": [101],
            "metadata_observed_pids": [101], "sample_count": 1,
            "observed_network_count": 0, "observed_mcp_count": 0, "reason_code": None,
        }
        if FakeAdapter.runtime_extra is not None:
            runtime[FakeAdapter.runtime_extra] = None if FakeAdapter.runtime_extra == "reason" else 1
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

    def _fake_manifest(self):
        return {
            "schema": "victory-lab-manifest/v2", "version": 2,
            "fixture_hashes": {"fixture": "a" * 64}, "oracle_hashes": {"oracle": "b" * 64},
            "adapters": [{"schema": "victory-adapter-spec/v2", "adapter_id": "a", "kind": "current", "expected_commit": "c" * 40, "expected_executable_sha256": "a" * 64, "metadata_command": ["fake", "version"], "command": ["fake"], "capabilities": ["affected"], "comparable_operations": ["affected"], "env_allowlist": []}],
            "cases": [{"id": "c", "operation": "affected", "corpus": ["x"], "golden": "o", "changed_paths": ["x"]}],
            "workloads": [{"workload_id": "w", "case_id": "c", "operation": "affected", "mode": "direct"}],
            "groups": [{"group_id": "g", "adapter_id": "a", "workload_id": "w", "case_id": "c", "operation": "affected", "repetitions": 30, "authoritative": True}],
        }

    def _run_manifest_in_root(self, root, *, case_override=None, repetition_override=None, manifest=None):
        manifest = manifest or self._fake_manifest()
        spec = AdapterSpec.from_dict(manifest["adapters"][0])
        FakeAdapter.case_override = case_override
        FakeAdapter.repetition_override = repetition_override
        (root / "manifest.json").write_text("{}", encoding="utf-8")
        (root / "fixture").write_text("fixture", encoding="utf-8")
        (root / "oracle").write_text("oracle", encoding="utf-8")
        with patch.object(runner_v2, "load_manifest", return_value=manifest), patch.object(runner_v2, "validate_strict_manifest"), patch.object(runner_v2, "validate_runtime", return_value=[]), patch.object(runner_v2, "manifest_adapters", return_value={"a": spec}), patch.object(runner_v2, "VictoryAdapter", FakeAdapter):
            return runner_v2.run_manifest(root / "manifest.json", root / "output", repetitions=30)

    def _run_manifest_with_fake(self, *, case_override=None, repetition_override=None):
        with tempfile.TemporaryDirectory() as temp:
            return self._run_manifest_in_root(Path(temp), case_override=case_override, repetition_override=repetition_override)

    def test_runner_rejects_wrong_returned_case_and_repetition(self):
        try:
            with self.assertRaises(ValueError):
                self._run_manifest_with_fake(case_override="wrong")
            with self.assertRaises(ValueError):
                self._run_manifest_with_fake(repetition_override=29)
        finally:
            FakeAdapter.case_override = None
            FakeAdapter.repetition_override = None

    def test_runner_rejects_each_noncanonical_runtime_key_before_digest(self):
        for extra in ("network_count", "mcp_count", "reason", "unknown_runtime_key"):
            FakeAdapter.runtime_extra = extra
            try:
                with tempfile.TemporaryDirectory() as temp:
                    root = Path(temp)
                    with self.assertRaisesRegex(ValueError, "runtime security projection keys"):
                        self._run_manifest_in_root(root)
                    output = root / "output"
                    self.assertEqual((output / "samples.jsonl").read_text(encoding="utf-8"), "")
                    self.assertFalse((output / "run.json").exists())
            finally:
                FakeAdapter.runtime_extra = None

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

    def test_abort_on_second_sample_preserves_exactly_first_and_sanitizes_error(self):
        FakeAdapter.abort_on_repetition = 1
        try:
            with tempfile.TemporaryDirectory() as temp:
                root = Path(temp)
                with self.assertRaisesRegex(RuntimeError, "simulated adapter abort"):
                    self._run_manifest_in_root(root)
                output = root / "output"
                lines = (output / "samples.jsonl").read_text(encoding="utf-8").splitlines()
                error_text = (output / "runner-error.json").read_text(encoding="utf-8")
                error = json.loads(error_text)
                self.assertEqual(len(lines), 1)
                self.assertEqual(json.loads(lines[0])["repetition"], 0)
                self.assertEqual(error, {
                    "schema": "victory-runner-error/v2",
                    "status": "BLOCKED",
                    "reason_code": "native_error",
                    "completed_samples": 1,
                    "expected_samples": 30,
                    "run_id": error["run_id"],
                    "manifest_sha256": hashlib.sha256((root / "manifest.json").read_bytes()).hexdigest(),
                    "runtime_preflight_digest": error["runtime_preflight_digest"],
                })
                self.assertNotIn("simulated adapter abort", error_text)
                self.assertNotIn("secret", error_text)
                self.assertNotIn(str(root), error_text)
                self.assertFalse((output / "run.json").exists())
        finally:
            FakeAdapter.abort_on_repetition = None

    def test_invalid_first_sample_leaves_empty_samples_and_error(self):
        FakeAdapter.terminal_fields_override = False
        try:
            with tempfile.TemporaryDirectory() as temp:
                root = Path(temp)
                with self.assertRaisesRegex(ValueError, "terminal"):
                    self._run_manifest_in_root(root)
                output = root / "output"
                self.assertEqual((output / "samples.jsonl").read_text(encoding="utf-8"), "")
                error = json.loads((output / "runner-error.json").read_text(encoding="utf-8"))
                self.assertEqual(error["status"], "BLOCKED")
                self.assertEqual(error["reason_code"], "blocked")
                self.assertEqual(error["completed_samples"], 0)
                self.assertEqual(error["expected_samples"], 30)
                self.assertFalse((output / "run.json").exists())
        finally:
            FakeAdapter.terminal_fields_override = True

    def test_rerun_nonempty_output_is_blocked_without_reuse(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            first = self._run_manifest_in_root(root)
            output = root / "output"
            original_samples = (output / "samples.jsonl").read_bytes()
            with self.assertRaisesRegex(ValueError, "already contains run artifacts"):
                self._run_manifest_in_root(root)
            self.assertEqual((output / "samples.jsonl").read_bytes(), original_samples)
            self.assertEqual(json.loads((output / "run.json").read_text(encoding="utf-8"))["run_id"], first["run_id"])
            self.assertFalse((output / "runner-error.json").exists())

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
            samples_bytes = (root / "output" / "samples.jsonl").read_bytes()
            lines = samples_bytes.splitlines()
            run_json = json.loads((root / "output" / "run.json").read_text(encoding="utf-8"))
            round_tripped_sample = json.loads(lines[0].decode("utf-8"))
        runner_v2._validate_pass_sample(round_tripped_sample)
        self.assertEqual(summary["samples"], 30)
        self.assertEqual(len(lines), 30)
        self.assertEqual([call[2] for call in FakeAdapter.calls], list(range(30)))
        self.assertTrue(summary["anti_gaming"]["all_samples_retained"])
        self.assertEqual(summary["manifest_path"], str((root / "manifest.json").resolve()))
        self.assertEqual(summary["manifest_sha256"], manifest_sha256)
        self.assertEqual(summary["manifest_bundle"]["path"], summary["manifest_path"])
        self.assertEqual(summary["manifest_bundle"]["id"], summary["manifest_id"])
        samples_sha256 = hashlib.sha256(samples_bytes).hexdigest()
        self.assertEqual(summary["samples_sha256"], samples_sha256)
        self.assertEqual(run_json["samples_sha256"], samples_sha256)
        self.assertEqual(run_json["samples"], 30)
        self.assertEqual(run_json["status_counts"], summary["status_counts"])
        self.assertFalse((root / "output" / "runner-error.json").exists())


if __name__ == "__main__":
    unittest.main()
