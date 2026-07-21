import copy
import hashlib
import json
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

import runner_v2
from canonical_v2 import payload_digest
from manifest_v2 import BASELINE_COMMIT, GRAPHIFY_COMMIT, GRAPHIFY_VERSION, ManifestError, validate_manifest
from report_v2 import build_report
from validate_manifest import validate_runtime, validate_strict_manifest
from schema_v2 import AdapterSpec, RunRecord, SchemaError


class _StableAdapter:
    def __init__(self, spec, manifest, root, executor=None):
        self.spec = spec

    def run_case(self, case, *, repetition):
        payload = {"items": ["stable"]}
        return RunRecord(
            adapter_id=self.spec.adapter_id, operation=case["operation"], status="PASS", repetition=repetition,
            canonical={"schema": "victory-canonical/v2", "operation": case["operation"], "payload": payload,
                       "digest": payload_digest(payload), "token_units": 1},
            elapsed_ms=1.0, metrics={"child": {"status": "PASS", "peak_rss_bytes": 1}},
        )


class _MutatingAdapter(_StableAdapter):
    target = "fixture"

    def __init__(self, spec, manifest, root, executor=None):
        super().__init__(spec, manifest, root, executor=executor)
        self.root = Path(root)
        self.mutated = False

    def run_case(self, case, *, repetition):
        if not self.mutated:
            relative = "corpus/fixture.go" if self.target == "fixture" else "goldens/case.json"
            (self.root / relative).write_text("tampered", encoding="utf-8")
            self.mutated = True
        return super().run_case(case, repetition=repetition)


class VictoryAdversarialV2Tests(unittest.TestCase):
    def _manifest_tree(self):
        temp = tempfile.TemporaryDirectory()
        root = Path(temp.name)
        (root / "corpus").mkdir()
        (root / "goldens").mkdir()
        (root / "corpus/fixture.go").write_text("package fixture\n", encoding="utf-8")
        (root / "goldens/case.json").write_text('{"expected_direct": ["stable"]}\n', encoding="utf-8")
        digest = lambda path: hashlib.sha256(path.read_bytes()).hexdigest()
        manifest = {
            "schema": "victory-lab-manifest/v2", "version": 2,
            "baseline_commit": BASELINE_COMMIT,
            "graphify": {"commit": GRAPHIFY_COMMIT, "version": GRAPHIFY_VERSION},
            "current": {"commit": "c" * 40}, "default_repetitions": 30,
            "fixture_hashes": {"corpus/fixture.go": digest(root / "corpus/fixture.go")},
            "oracle_hashes": {"goldens/case.json": digest(root / "goldens/case.json")},
            "adapters": [{
                "schema": "victory-adapter-spec/v2", "adapter_id": "fake", "kind": "current",
                "capabilities": ["affected"], "comparable_operations": ["affected"],
                "normalizable_operations": [], "env_allowlist": [], "command": ["fake"],
            }],
            "cases": [{"id": "case", "operation": "affected", "corpus": ["corpus"], "golden": "goldens/case.json", "changed_paths": ["corpus/fixture.go"]}],
            "oracles": {"case": {"expected_direct": ["stable"]}},
            "repository_identity": "temporary-test-fixture",
        }
        path = root / "manifest.json"
        path.write_text(json.dumps(manifest), encoding="utf-8")
        return temp, root, path, manifest

    def _record(self, repetition, *, status="PASS", child_status="PASS", peak=1, payload=None, case_id="case"):
        payload = payload or {"items": ["stable"]}
        passed = status == "PASS"
        return {
            "schema": "victory-run-record/v2", "case_id": case_id, "adapter_id": "fake", "operation": "affected",
            "status": status, "repetition": repetition, "fixture_digest": "a" * 64, "oracle_digest": "b" * 64,
            "executable_sha256": "", "source_sha256": "", "commit": "", "version": "", "capabilities": ["affected"],
            "argv": [], "cwd": "", "env_keys": [], "elapsed_ms": repetition + 1.0,
            "canonical": {"schema": "victory-canonical/v2", "operation": "affected", "payload": payload,
                          "digest": payload_digest(payload), "token_units": 1} if passed else None,
            "metrics": {"child": {"status": child_status, "peak_rss_bytes": peak}},
            "error": None if passed else {"kind": "timeout", "reason_code": "timeout"},
        }

    def _samples(self, **kwargs):
        return [self._record(i, **kwargs) for i in range(30)]

    def _write_samples(self, records):
        handle = tempfile.NamedTemporaryFile(mode="w", suffix=".jsonl", delete=False, encoding="utf-8")
        with handle:
            for record in records:
                handle.write(json.dumps(record) + "\n")
        return Path(handle.name)

    def test_sha_and_pin_mismatch_are_blocked_in_temporary_manifest(self):
        temp, root, path, manifest = self._manifest_tree()
        try:
            broken = copy.deepcopy(manifest)
            broken["baseline_commit"] = "0" * 40
            with self.assertRaises(ManifestError):
                validate_manifest(broken, root, check_files=False)
            broken = copy.deepcopy(manifest)
            broken["fixture_hashes"]["corpus/fixture.go"] = "0" * 64
            with self.assertRaises(ManifestError):
                validate_manifest(broken, root, check_files=True)
        finally:
            temp.cleanup()

    def test_runtime_executable_sha_mismatch_is_blocked(self):
        temp, root, path, manifest = self._manifest_tree()
        try:
            executable = root / "fake.exe"
            executable.write_bytes(b"fake")
            broken = copy.deepcopy(manifest)
            broken["adapters"][0]["executable"] = str(executable)
            broken["adapters"][0]["expected_executable_sha256"] = "0" * 64
            spec = AdapterSpec.from_dict(broken["adapters"][0])
            blockers = validate_runtime({**broken, "adapters": [spec.to_dict()]}, require_runtime=True)
            self.assertTrue(blockers)
            self.assertIn("sha mismatch", blockers[0])
        finally:
            temp.cleanup()

    def test_fixture_and_golden_mutation_is_detected_after_run(self):
        for target in ("fixture", "golden"):
            temp, root, path, manifest = self._manifest_tree()
            try:
                spec = AdapterSpec.from_dict(manifest["adapters"][0])
                _MutatingAdapter.target = target
                with patch.object(runner_v2, "VictoryAdapter", _MutatingAdapter), patch.object(
                    runner_v2, "manifest_adapters", return_value={"fake": spec}
                ):
                    with self.assertRaises(ManifestError):
                        runner_v2.run_manifest(path, root / "output", repetitions=30)
            finally:
                temp.cleanup()

    def test_missing_comparator_is_blocked(self):
        temp, root, path, manifest = self._manifest_tree()
        try:
            broken = copy.deepcopy(manifest)
            broken["adapters"][0]["capabilities"] = []
            broken["adapters"][0]["comparable_operations"] = []
            with self.assertRaises(ManifestError):
                validate_strict_manifest(broken, root, check_files=False)
        finally:
            temp.cleanup()

    def test_less_than_thirty_is_blocked_by_runner_and_report(self):
        temp, root, path, manifest = self._manifest_tree()
        try:
            with self.assertRaises(ManifestError):
                runner_v2.run_manifest(path, root / "output", repetitions=29)
            with self.assertRaises(ValueError):
                build_report(self._write_samples(self._samples()[:29]), expected_repetitions=29)
        finally:
            temp.cleanup()

    def test_timeout_and_crash_are_not_pass(self):
        timeout = self._samples(status="FAIL")
        crash = self._samples(status="FAIL")
        crash[0]["error"] = {"kind": "crash", "reason_code": "crash"}
        self.assertEqual(build_report(self._write_samples(timeout))["status"], "FAIL")
        self.assertEqual(build_report(self._write_samples(crash))["status"], "FAIL")

    def test_schema_drift_duplicate_replay_and_best_of_are_blocked(self):
        records = self._samples()
        drift = copy.deepcopy(records[0])
        drift["unexpected"] = True
        with self.assertRaises(SchemaError):
            RunRecord.from_dict(drift)
        duplicate = records[:-1] + [copy.deepcopy(records[0])]
        with self.assertRaises(ValueError):
            build_report(self._write_samples(duplicate))
        best_of = self._samples()
        best_of[0]["best_of"] = True
        with self.assertRaises(ValueError):
            build_report(self._write_samples(best_of))

    def test_missing_metric_and_unavailable_metric_cannot_be_pass(self):
        missing = self._samples()
        del missing[0]["metrics"]["child"]["peak_rss_bytes"]
        with self.assertRaises(ValueError):
            build_report(self._write_samples(missing))
        unavailable = self._samples(child_status="NOT_COMPARABLE", peak=None)
        report = build_report(self._write_samples(unavailable))
        self.assertEqual(report["status"], "NOT_COMPARABLE")
        self.assertNotEqual(report["status"], "PASS")

    def test_nondeterministic_digest_and_raw_log_leak_are_blocked(self):
        records = self._samples()
        records[1] = self._record(1, payload={"items": ["changed"]})
        with self.assertRaises(ValueError):
            build_report(self._write_samples(records))
        leaked = self._record(0)
        leaked["canonical"]["payload"]["stdout"] = "raw native log"
        with self.assertRaises(SchemaError):
            RunRecord.from_dict(leaked)


if __name__ == "__main__":
    unittest.main()
