import copy
import hashlib
import json
import os
import subprocess
import sys
import tempfile
import unittest
from types import SimpleNamespace
from pathlib import Path
from unittest.mock import patch

import runner_v2
from adapters.v2 import _adapt_mi_lsp_terminal, _graphify_affected_payload
from attestation_v2 import AttestationError, build_input_digest, canonical_attestation, source_artifact_digest, validate_attestation
from child_metrics import run_child
from canonical_v2 import canonical_payload, canonicalize, payload_digest, token_count, validate_terminal_state
from manifest_v2 import BASELINE_COMMIT, GRAPHIFY_COMMIT, GRAPHIFY_VERSION, ManifestError, load_manifest, validate_manifest
from report_v2 import _sample_nonce, build_report
from security_gate import SecurityGate, runtime_evidence_digest
from validate_manifest import validate_runtime, validate_strict_manifest
from schema_v2 import AdapterSpec, RunRecord, SchemaError
from durable_v2 import validate_durable


class _StableAdapter:
    def __init__(self, spec, manifest, root, executor=None):
        self.spec = spec

    def run_case(self, case, *, repetition):
        payload = {"items": ["stable"], "ok": True, "done": True, "backend": "go", "completeness": "complete", "truncated": False}
        runtime = {
            "status": "PASS", "runtime_proof": True, "provenance": "child_metrics_executor",
            "probe_mode": "windows_netstat_child_tree_observation", "observed_pids": [101],
            "metadata_observed_pids": [101], "sample_count": 1,
            "observed_network_count": 0, "observed_mcp_count": 0, "reason_code": None,
        }
        runtime["evidence_digest"] = runtime_evidence_digest(runtime)
        return RunRecord(
            adapter_id=self.spec.adapter_id, operation=case["operation"], status="PASS", repetition=repetition,
            canonical={"schema": "victory-canonical/v2", "operation": case["operation"], "payload": payload,
                       "digest": payload_digest(payload), "token_units": token_count(payload)},
            elapsed_ms=1.0, metrics={
                "child": {"status": "PASS", "peak_rss_bytes": 1, "tree_peak_rss_bytes": 1, "tree_supported": True,
                          "cleanup_status": "clean", "samples": 1, "timed_out": False, "crashed": False,
                          "failure_class": "none", "exit_code": 0},
                "security": {"status": "PASS", "runtime_proof": True, "runtime": runtime,
                             "integrity": {"status": "PASS"}, "source_integrity": {"status": "PASS"}},
            },
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
                "expected_commit": "c" * 40, "expected_executable_sha256": "a" * 64,
                "capabilities": ["affected"], "comparable_operations": ["affected"],
                "normalizable_operations": [], "env_allowlist": [], "command": ["fake"], "metadata_command": ["fake", "version"],
            }, {
                "schema": "victory-adapter-spec/v2", "adapter_id": "baseline", "kind": "baseline",
                "expected_commit": BASELINE_COMMIT, "expected_executable_sha256": "b" * 64,
                "capabilities": ["affected"], "comparable_operations": ["affected"],
                "normalizable_operations": [], "env_allowlist": [], "command": ["fake"], "metadata_command": ["fake", "version"],
            }],
            "cases": [{"id": "case", "operation": "affected", "corpus": ["corpus"], "golden": "goldens/case.json", "changed_paths": ["corpus/fixture.go"]}],
            "oracles": {"case": {"expected_direct": ["stable"]}},
            "repository_identity": "temporary-test-fixture",
        }
        path = root / "manifest.json"
        path.write_text(json.dumps(manifest), encoding="utf-8")
        return temp, root, path, manifest

    def _record(self, repetition, *, status="PASS", child_status="PASS", peak=1, payload=None, case_id="case"):
        payload = payload or {"items": ["stable"], "ok": True, "done": True, "backend": "go", "completeness": "complete", "truncated": False}
        passed = status == "PASS"
        runtime = {
            "status": "PASS", "runtime_proof": True, "provenance": "child_metrics_executor",
            "probe_mode": "windows_netstat_child_tree_observation", "observed_pids": [101],
            "metadata_observed_pids": [101], "sample_count": 1,
            "observed_network_count": 0, "observed_mcp_count": 0, "reason_code": None,
        }
        runtime["evidence_digest"] = runtime_evidence_digest(runtime)
        return {
            "schema": "victory-run-record/v2", "case_id": case_id, "adapter_id": "fake", "operation": "affected",
            "status": status, "repetition": repetition, "fixture_digest": "a" * 64, "oracle_digest": "b" * 64,
            "executable_sha256": "", "source_sha256": "", "commit": "", "version": "", "capabilities": ["affected"],
            "argv": [], "cwd": "", "env_keys": [], "elapsed_ms": repetition + 1.0,
            "canonical": {"schema": "victory-canonical/v2", "operation": "affected", "payload": payload,
                          "digest": payload_digest(payload), "token_units": token_count(payload)} if passed else None,
            "metrics": {"child": {"status": child_status, "peak_rss_bytes": peak, "tree_peak_rss_bytes": peak, "tree_supported": True, "cleanup_status": "clean", "samples": 1, "timed_out": False, "crashed": False, "failure_class": "none", "exit_code": 0}, "security": {"status": "PASS", "runtime_proof": True, "runtime": runtime, "integrity": {"status": "PASS"}, "source_integrity": {"status": "PASS"}}, "freshness": {"schema": "victory-sample-freshness/v1", "run_id": "d" * 64, "preflight_digest": "e" * 64, "group_id": "g" + hashlib.sha256(f"fake:{case_id}:affected".encode()).hexdigest()[:63], "repetition": repetition, "nonce": _sample_nonce("d" * 64, "e" * 64, f"fake:{case_id}:affected", repetition)}},
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

    def test_missing_current_source_without_env_is_operational_preflight_blocker(self):
        temp, root, path, manifest = self._manifest_tree()
        before = copy.deepcopy(manifest)
        try:
            with patch.dict(os.environ, {}, clear=False):
                os.environ.pop("VICTORY_LAB_CURRENT_SOURCE", None)
                blockers = validate_runtime(manifest, require_runtime=True, manifest_root=root)
            self.assertTrue(blockers)
            self.assertEqual(manifest, before)
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
        records[1] = self._record(1, payload={"items": ["changed"], "ok": True, "done": True, "backend": "go", "completeness": "complete", "truncated": False})
        self.assertEqual(build_report(self._write_samples(records))["status"], "FAIL")
        leaked = self._record(0)
        leaked["canonical"]["payload"]["stdout"] = "raw native log"
        with self.assertRaises(SchemaError):
            RunRecord.from_dict(leaked)

    def test_terminal_state_is_checked_before_any_pass_claim(self):
        valid = {"ok": True, "backend": "go", "truncated": False, "done": True, "items": []}
        self.assertIs(validate_terminal_state(valid), valid)
        with self.assertRaises(ValueError):
            validate_terminal_state({key: value for key, value in valid.items() if key != "done"})
        for field, value in (("done", False), ("phase", "running"), ("partial", True), ("truncated", True)):
            broken = dict(valid)
            broken[field] = value
            with self.assertRaises(ValueError):
                validate_terminal_state(broken)

    def test_pass_requires_positive_token_units_even_when_payload_is_stable(self):
        record = self._record(0)
        record["canonical"]["token_units"] = 0
        with self.assertRaises(SchemaError):
            RunRecord.from_dict(record)
        records = self._samples()
        records[0]["canonical"]["token_units"] = 0
        with self.assertRaises(SchemaError):
            build_report(self._write_samples(records))

    def test_closed_catalogs_do_not_reject_domain_status_but_reject_metric_drift(self):
        domain_status = self._record(0, payload={"items": [{"display": "stable", "status": "active"}], "ok": True, "done": True, "backend": "go", "completeness": "complete", "truncated": False})
        RunRecord.from_dict(domain_status)
        metric_drift = self._record(0)
        metric_drift["metrics"]["child"]["status"] = "MAYBE"
        with self.assertRaises(SchemaError):
            RunRecord.from_dict(metric_drift)

    def test_value_redaction_covers_email_phi_ssn_and_relative_paths(self):
        values = ["alice@example.com", "patient record", "SSN 123-45-6789", "relative/path/file.go"]
        for value in values:
            self.assertEqual(canonicalize(value), "<REDACTED>")
        rendered = repr([canonicalize(value) for value in values])
        for value in values:
            self.assertNotIn(value, rendered)

    def test_attestation_binds_source_checkout_recipe_and_executable(self):
        root = Path(__file__).resolve().parents[3]
        source = root
        commit = subprocess.check_output(["git", "-C", str(source), "rev-parse", "HEAD"], text=True).strip()
        source_sha = source_artifact_digest(source, commit)
        executable_sha = "a" * 64
        command = ["go", "build", "-trimpath", "-buildvcs=false", "-o", "{output}", "./cmd/mi-lsp"]
        toolchain = {"go": "test", "target": "windows/arm64"}
        value = {"schema": "victory-build-attestation/v2", "adapter_kind": "current", "source_tree_commit": commit,
                 "source_artifact_digest": source_sha, "source_digest_recipe": "git-ls-tree-v1",
                 "build_command": command, "toolchain": toolchain, "executable_sha256": executable_sha,
                 "package_provenance": {}, "reproducible": True,
                 "build_execution": {"status": "reproduced", "executed_command": command, "source_head": commit,
                   "toolchain": toolchain, "target": "windows/arm64",
                   "build_input_digest": build_input_digest(source, commit, command, toolchain),
                   "sanitized_log_digest": "b" * 64, "produced_sha256": executable_sha, "matches_expected": True}}
        with tempfile.TemporaryDirectory() as temp:
            path = Path(temp) / "attestation.json"; path.write_bytes(canonical_attestation(value))
            digest = hashlib.sha256(path.read_bytes()).hexdigest()
            self.assertEqual(validate_attestation(path, expected_commit=commit, expected_executable_sha256=executable_sha,
                expected_source_sha256=source_sha, expected_file_sha256=digest, source_root=source,
                expected_kind="current", expected_build_command=value["build_command"], expected_toolchain=value["toolchain"],
                expected_package_provenance={})["source_tree_commit"], commit)
            for changed in (dict(value, source_tree_commit="0" * 40), dict(value, build_command=["go", "test"])):
                path.write_bytes(canonical_attestation(changed))
                with self.assertRaises(AttestationError):
                    validate_attestation(path, expected_commit=commit, expected_executable_sha256=executable_sha, expected_source_sha256=source_sha, source_root=source, expected_kind="current", expected_build_command=value["build_command"], expected_toolchain=value["toolchain"], expected_package_provenance={})
            path.write_text(json.dumps({"schema": "victory-build-attestation/v1", "commit": commit, "executable_sha256": executable_sha, "source_sha256": source_sha, "binding_sha256": "0" * 64, "reproducible": True}), encoding="utf8")
            with self.assertRaises(AttestationError):
                validate_attestation(path, expected_commit=commit, expected_executable_sha256=executable_sha, expected_source_sha256=source_sha)

    def test_manifest_attestation_digest_mismatch_is_blocked(self):
        root = Path(__file__).resolve().parents[3]
        path = root / "benchmarks/victory-lab/v2/manifest.json"
        manifest = load_manifest(path)
        broken = copy.deepcopy(manifest)
        broken["adapters"][0]["expected_attestation_sha256"] = "0" * 64
        with self.assertRaises(ManifestError):
            validate_strict_manifest(broken, path.parent, check_files=True)

    def test_durable_rejects_email_adapter_id(self):
        with self.assertRaises((SchemaError, ValueError)):
            RunRecord(adapter_id="alice@example.com", case_id="case", operation="affected", status="NOT_COMPARABLE", error={"kind": "capability", "reason_code": "unavailable"}).to_dict()

    def test_durable_rejects_windows_patient_path_case_id(self):
        with self.assertRaises((SchemaError, ValueError)):
            RunRecord(adapter_id="adapter", case_id=r"C:\\patient\\record.json", operation="affected", status="NOT_COMPARABLE", error={"kind": "capability", "reason_code": "unavailable"}).to_dict()

    def test_durable_rejects_group_key_email_and_path(self):
        with self.assertRaises(ValueError):
            validate_durable({"groups": {"alice@example.com:case:affected": {}}})
        with self.assertRaises(ValueError):
            validate_durable({"groups": {r"adapter:C:\\patient\\record.json:affected": {}}})

    def test_source_input_mutation_after_verification_is_blocked(self):
        from security_gate import SecurityGate
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            (root / ".git").mkdir()
            with patch("security_gate._git_head", return_value="a" * 40):
                source = root / "internal"; source.mkdir()
                input_file = root / "go.mod"; input_file.write_text("module stable", encoding="utf-8")
                gate = SecurityGate(source_inputs={"go": (root, ("go.mod", "internal"))})
                gate.start()
                input_file.write_text("module mutated", encoding="utf-8")
                result = gate.finish()
        self.assertEqual(result["status"], "BLOCKED")
        self.assertEqual(result["integrity"]["reason_code"], "protected_path_changed")

    def test_graphify_payload_preserves_native_order_instead_of_oracle_sorting(self):
        payload = _graphify_affected_payload(
            "Affected nodes for Normalize()\n"
            "- Validate() [calls] subject/subject.go:L9\n"
            "- Direct() [calls] callers/callers.go:L6\n"
        )
        self.assertEqual([item["display"] for item in payload["items"]], ["subject.Validate", "callers.Direct"])

    def test_real_adapter_terminal_fields_survive_canonicalization(self):
        native = {
            "ok": True, "done": True, "backend": "go", "completeness": "complete",
            "truncated": False, "items": [],
        }
        adapted = _adapt_mi_lsp_terminal(
            native,
            SimpleNamespace(returncode=0, timed_out=False, crashed=False),
        )
        self.assertIs(adapted, native)
        canonical = canonical_payload("affected", adapted)
        self.assertEqual(
            {key: canonical["payload"][key] for key in ("ok", "done", "backend", "completeness", "truncated")},
            {"ok": True, "done": True, "backend": "go", "completeness": "complete", "truncated": False},
        )
        graphified = _graphify_affected_payload(
            "Affected nodes for Normalize()\n"
            "- Validate() [calls] subject/subject.go:L9\n"
        )
        self.assertEqual(
            {key: graphified[key] for key in ("ok", "done", "backend", "completeness", "truncated")},
            {"ok": True, "done": True, "backend": "graphify", "completeness": "complete", "truncated": False},
        )

    def test_injected_runtime_observation_without_executor_provenance_is_not_comparable(self):
        with tempfile.TemporaryDirectory() as temp:
            fixture = Path(temp) / "fixture.txt"
            fixture.write_text("stable", encoding="utf-8")
            gate = SecurityGate({"fixture": fixture})
            gate.start(["safe-tool"], {"PATH": "redacted"})
            comparable = gate.finish(
                ["safe-tool"], {"PATH": "redacted"},
                runtime_observation={
                    "status": "PASS", "runtime_proof": True, "sample_count": 1,
                    "observed_network_count": 0, "observed_mcp_count": 0,
                    "evidence_digest": "c" * 64,
                },
            )
            self.assertEqual(comparable["status"], "NOT_COMPARABLE")
            self.assertFalse(comparable["runtime_proof"])
            self.assertEqual(comparable["runtime"]["provenance"], None)

    def test_real_child_tree_passes_only_with_child_metrics_executor_evidence(self):
        if os.name != "nt":
            self.skipTest("native runtime proof is Windows-only")
        env = {key: os.environ.get(key, "") for key in ("PATH", "TEMP", "TMP")}
        with tempfile.TemporaryDirectory() as temp:
            result = run_child(
                [sys.executable, "-c", "import time; time.sleep(1.2)"],
                cwd=Path(temp), env=env, timeout_seconds=3,
            )
            self.assertEqual(result.metrics.status, "PASS")
            self.assertTrue(result.metrics.tree_supported)
            observed_pids = result.metrics.observed_pids
            self.assertTrue(observed_pids)
            self.assertEqual(len(observed_pids), len(set(observed_pids)))
            self.assertTrue(all(isinstance(pid, int) and pid > 0 for pid in observed_pids))
            if len(observed_pids) == 1:
                self.assertEqual(len(observed_pids), 1)
                self.assertEqual(result.metrics.tree_peak_rss_bytes, result.metrics.peak_rss_bytes)
            else:
                self.assertGreater(len(observed_pids), 1)
                self.assertGreaterEqual(result.metrics.tree_peak_rss_bytes, result.metrics.peak_rss_bytes)
            self.assertIsInstance(result.runtime_proof, dict)
            self.assertEqual(result.runtime_proof["status"], "PASS")
            self.assertEqual(result.runtime_proof["provenance"], "child_metrics_executor")
            self.assertTrue(result.runtime_proof["runtime_proof"])
            gate = SecurityGate({"fixture": Path(temp)})
            gate.start(result.argv, env)
            comparable = gate.finish(result.argv, env, runtime_observation=result.runtime_proof)
            self.assertEqual(comparable["status"], "PASS")
            self.assertTrue(comparable["runtime_proof"])


if __name__ == "__main__":
    unittest.main()
