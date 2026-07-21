import hashlib
import json
import tempfile
import unittest
from pathlib import Path

from canonical_v2 import payload_digest, token_count
from report_v2 import _comparisons, _items, _manifest_identity, _quality, _sample_nonce, _validate_manifest_bundle, build_report
from security_gate import runtime_evidence_digest


class ReportV2Tests(unittest.TestCase):
    def _write(self, records):
        temp = tempfile.NamedTemporaryFile(mode="w", suffix=".jsonl", delete=False, encoding="utf-8")
        with temp:
            for record in records:
                temp.write(json.dumps(record) + "\n")
        return Path(temp.name)

    def _record(self, repetition, *, status="PASS", payload=None, child_status="PASS", peak=1, case_id="case"):
        payload = payload or {"items": ["stable"], "ok": True, "done": True, "backend": "go", "completeness": "complete", "truncated": False}
        canonical = {
            "schema": "victory-canonical/v2", "operation": "affected", "payload": payload,
            "digest": payload_digest(payload), "token_units": token_count(payload),
        } if status == "PASS" else None
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
            "argv": [], "cwd": "", "env_keys": [], "elapsed_ms": repetition + 1,
            "canonical": canonical, "metrics": {"child": {"status": child_status, "peak_rss_bytes": peak, "tree_peak_rss_bytes": peak, "tree_supported": True, "cleanup_status": "clean", "samples": 1, "timed_out": False, "crashed": False, "failure_class": "none", "exit_code": 0}, "security": {"status": "PASS", "runtime_proof": True, "runtime": runtime, "integrity": {"status": "PASS"}, "source_integrity": {"status": "PASS"}}, "freshness": {"schema": "victory-sample-freshness/v1", "run_id": "d" * 64, "preflight_digest": "e" * 64, "group_id": "g" + hashlib.sha256(f"fake:{case_id}:affected".encode()).hexdigest()[:63], "repetition": repetition, "nonce": _sample_nonce("d" * 64, "e" * 64, f"fake:{case_id}:affected", repetition)}},
            "error": None if status == "PASS" else {"kind": "timeout", "reason_code": "timeout"},
        }

    def test_reports_p50_p95_and_max_from_all_samples(self):
        records = [self._record(repetition, peak=repetition + 1) for repetition in range(30)]
        report = build_report(self._write(records))
        latency = report["groups"]["fake:case:affected"]["latency"]
        self.assertEqual(latency["n"], 30)
        self.assertEqual(latency["max_ms"], 30)
        self.assertGreater(latency["p95_ms"], latency["p50_ms"])
        self.assertTrue(report["anti_gaming"]["all_samples_used"])

    def test_reports_child_peak_metrics_from_all_samples(self):
        records = [self._record(repetition, peak=repetition + 1) for repetition in range(30)]
        for record in records:
            record["metrics"]["child"]["tree_peak_rss_bytes"] = (record["repetition"] + 1) * 2
        report = build_report(self._write(records))
        child = report["groups"]["fake:case:affected"]["child_metrics"]
        self.assertEqual(child["status_counts"], {"PASS": 30})
        self.assertEqual(child["peak_rss_bytes"]["n"], 30)
        self.assertEqual(child["peak_rss_bytes"]["max"], 30)
        self.assertEqual(child["tree_peak_rss_bytes"]["p50"], 31)

    def test_timeout_crash_and_nonzero_with_clean_or_forced_are_fail(self):
        for cleanup, field, value in (("clean", "timed_out", True), ("forced", "crashed", True), ("clean", "failure_class", "exit_nonzero")):
            records = [self._record(i, status="FAIL") for i in range(30)]
            for record in records:
                record["metrics"]["child"][field] = value
                record["metrics"]["child"]["cleanup_status"] = cleanup
            self.assertEqual(build_report(self._write(records))["status"], "FAIL")

    def test_pass_requires_clean_or_forced_successful_child(self):
        for field, value in (("timed_out", True), ("crashed", True), ("failure_class", "timeout"), ("exit_code", 7)):
            records = [self._record(i) for i in range(30)]
            records[0]["metrics"]["child"][field] = value
            with self.assertRaises(ValueError):
                build_report(self._write(records))
        records = [self._record(i) for i in range(30)]
        records[0]["metrics"]["child"]["cleanup_status"] = "not_required"
        self.assertEqual(build_report(self._write(records))["status"], "NOT_COMPARABLE")
        records = [self._record(i) for i in range(30)]
        records[0]["metrics"]["child"]["cleanup_status"] = "failed"
        with self.assertRaises(ValueError):
            build_report(self._write(records))

    def test_pass_requires_samples_and_supported_tree(self):
        for field, value in (("samples", 0), ("tree_supported", False)):
            records = [self._record(i) for i in range(30)]
            records[0]["metrics"]["child"][field] = value
            with self.assertRaises(ValueError):
                build_report(self._write(records))

    def test_runtime_proof_requires_child_metrics_executor_provenance(self):
        records = [self._record(i) for i in range(30)]
        records[0]["metrics"]["security"]["runtime"]["provenance"] = "forged"
        with self.assertRaises(ValueError):
            build_report(self._write(records))

    def test_malformed_metrics_fail_closed_without_type_error(self):
        records = [self._record(i) for i in range(30)]
        records[0]["metrics"] = {"child": {"status": "PASS"}, "security": []}
        with self.assertRaises(ValueError):
            build_report(self._write(records))

    def test_rewriting_repetition_on_cloned_samples_is_rejected_by_freshness(self):
        source = self._record(0)
        records = []
        for repetition in range(30):
            clone = json.loads(json.dumps(source))
            clone["repetition"] = repetition
            records.append(clone)
        with self.assertRaises(ValueError):
            build_report(self._write(records))

    def test_not_required_requires_successful_terminal_state_and_single_process_is_rejected(self):
        for field, value in (("timed_out", True), ("crashed", True), ("failure_class", "exit_nonzero"), ("exit_code", 7)):
            records = [self._record(i) for i in range(30)]
            records[0]["metrics"]["child"]["cleanup_status"] = "not_required"
            records[0]["metrics"]["child"][field] = value
            with self.assertRaises(ValueError):
                build_report(self._write(records))
        records = [self._record(i) for i in range(30)]
        records[0]["metrics"]["child"]["tree_supported"] = False
        with self.assertRaises(ValueError):
            build_report(self._write(records))

    def test_pass_requires_complete_terminal_payload_even_when_digest_and_tokens_match(self):
        records = [self._record(i) for i in range(30)]
        payload = records[0]["canonical"]["payload"]
        for field in ("ok", "done", "backend", "completeness", "truncated"):
            payload.pop(field)
        records[0]["canonical"]["digest"] = payload_digest(payload)
        records[0]["canonical"]["token_units"] = token_count(payload)
        with self.assertRaisesRegex(ValueError, "terminal"):
            build_report(self._write(records))

    def test_token_units_must_equal_recomputed_positive_count(self):
        records = [self._record(i) for i in range(30)]
        records[0]["canonical"]["token_units"] += 1
        with self.assertRaisesRegex(ValueError, "token_units"):
            build_report(self._write(records))

    def test_reproducible_rereport_does_not_consume_samples(self):
        path = self._write([self._record(i) for i in range(30)])
        first = build_report(path)
        second = build_report(path)
        self.assertEqual(first, second)

    def test_report_without_manifest_is_never_pass_and_digest_tampering_is_rejected(self):
        records = [self._record(i) for i in range(30)]
        report = build_report(self._write(records))
        self.assertEqual(report["status"], "NOT_COMPARABLE")
        self.assertFalse(report["anti_gaming"]["manifest_consumed"])
        records[0]["metrics"]["security"]["runtime"]["observed_pids"] = []
        with self.assertRaisesRegex(ValueError, "security"):
            build_report(self._write(records))

    def test_json_round_trip_accepts_valid_pass_and_rejects_canonical_tamper(self):
        records = [json.loads(json.dumps(self._record(i), sort_keys=True)) for i in range(30)]
        report = build_report(self._write(records))
        self.assertEqual(report["status"], "NOT_COMPARABLE")

        tampered = [json.loads(json.dumps(record, sort_keys=True)) for record in records]
        runtime = tampered[0]["metrics"]["security"]["runtime"]
        runtime["observed_network_count"] = 1
        with self.assertRaisesRegex(ValueError, "security"):
            build_report(self._write(tampered))

    def test_json_round_trip_rejects_each_noncanonical_runtime_key(self):
        records = [json.loads(json.dumps(self._record(i), sort_keys=True)) for i in range(30)]
        for extra in ("network_count", "mcp_count", "reason", "unknown_runtime_key"):
            tampered = [json.loads(json.dumps(record, sort_keys=True)) for record in records]
            tampered[0]["metrics"]["security"]["runtime"][extra] = None
            with self.assertRaisesRegex(ValueError, "runtime security projection keys"):
                build_report(self._write(tampered))

    def test_manifest_bundle_requires_matching_sha_path_and_id(self):
        with tempfile.TemporaryDirectory() as temp:
            path = Path(temp) / "manifest.json"
            path.write_text("{\"manifest\": true}\n", encoding="utf-8")
            expected = {"path": str(path.resolve()), "id": _manifest_identity(path), "sha256": hashlib.sha256(path.read_bytes()).hexdigest()}
            metadata = {"manifest": expected["path"], "manifest_path": expected["path"], "manifest_id": expected["id"], "manifest_sha256": expected["sha256"], "manifest_bundle": expected}
            self.assertEqual(_validate_manifest_bundle(metadata, path), expected)
            metadata["manifest_sha256"] = "0" * 64
            with self.assertRaisesRegex(ValueError, "SHA-256"):
                _validate_manifest_bundle(metadata, path)

    def test_manifest_rejects_direct_jsonl_as_authoritative_input(self):
        with self.assertRaisesRegex(ValueError, "runner output directory"):
            build_report(self._write([self._record(0)]), manifest={})

    def test_manifest_rejects_copied_samples_without_run_json(self):
        records = [self._record(i) for i in range(30)]
        with tempfile.TemporaryDirectory() as temp:
            output = Path(temp)
            (output / "samples.jsonl").write_text(
                "".join(json.dumps(record) + "\n" for record in records), encoding="utf-8"
            )
            with self.assertRaisesRegex(ValueError, "missing run.json"):
                build_report(output, manifest={})

    def test_manifest_rejects_tampered_run_json(self):
        records = [self._record(i) for i in range(30)]
        with tempfile.TemporaryDirectory() as temp:
            output = Path(temp)
            samples_path = output / "samples.jsonl"
            samples_path.write_text(
                "".join(json.dumps(record) + "\n" for record in records), encoding="utf-8"
            )
            run = {
                "schema": "victory-run/v2", "run_id": "r" + "a" * 63,
                "status": "PASS", "repetitions": 30, "samples": 30,
                "samples_sha256": "0" * 64,
                "status_counts": {"PASS": 30, "FAIL": 0, "BLOCKED": 0, "NOT_COMPARABLE": 0, "NOT_RUN": 0},
                "runtime_preflight": {
                    "status": "PASS", "require_runtime": True, "fresh_reproduction": True,
                    "evidence_digest": "e" * 64,
                },
            }
            (output / "run.json").write_text(json.dumps(run), encoding="utf-8")
            with self.assertRaisesRegex(ValueError, "SHA-256"):
                build_report(output, manifest={})

    def test_best_of_fields_are_rejected(self):
        records = [self._record(i) for i in range(30)]
        records[0]["best_of"] = True
        with self.assertRaises(ValueError):
            build_report(self._write(records))

    def test_missing_sample_is_rejected(self):
        with self.assertRaises(ValueError):
            build_report(self._write([self._record(0)]))

    def test_duplicate_repetition_is_rejected_even_when_count_is_30(self):
        records = [self._record(0) for _ in range(30)]
        with self.assertRaises(ValueError):
            build_report(self._write(records))

    def test_affected_quality_uses_path_identity_for_all_30_samples(self):
        payload = {
            "items": [{"path": "subject/subject.go", "display": "subject.Normalize"}],
            "ok": True, "done": True, "backend": "go", "completeness": "complete", "truncated": False,
        }
        records = [self._record(i, payload=payload) for i in range(30)]
        quality = _quality(
            records,
            {"id": "case", "operation": "affected"},
            {"oracles": {"case": {"expected_direct": ["subject/subject.go"]}}},
        )
        self.assertEqual(_items(payload, "affected"), ["subject/subject.go"])
        self.assertEqual(quality["status"], "PASS")
        self.assertEqual(quality["matching_samples"], 30)
        self.assertTrue(quality["all_samples_match"])

    def test_threshold_join_uses_current_over_graphify_and_all_metric_samples(self):
        def group(status, tokens, warm, rss):
            return {
                "status": status, "tokens": {"n": 30, "p95": tokens},
                "latency": {"n": 30, "p95_ms": warm},
                "child_metrics": {"tree_peak_rss_bytes": {"n": 30, "p95": rss}},
            }
        manifest = {
            "comparator_pair": {"callers-direct": {"current": "current", "graphify": "graphify", "metrics": ["tokens", "warm_p95", "tree_rss"]}},
            "thresholds": {"current_vs_graphify": {"tokens": 0.70, "warm_p95": 0.80, "tree_rss": 0.50}, "hotpath": {"current_p95_multiplier": 1.10, "baseline_p95_additive_ms": 25}},
        }
        reports = {
            "current": group("PASS", 7, 8, 4), "graphify": group("PASS", 10, 10, 10),
            "current-affected-direct": group("PASS", 1, 10, 1), "baseline-affected-direct-hotpath": group("NOT_COMPARABLE", 1, 20, 1),
        }
        comparisons = _comparisons(reports, manifest)
        self.assertEqual(comparisons["callers-direct"]["status"], "PASS")
        self.assertEqual(comparisons["callers-direct"]["metrics"]["tokens"]["ratio_current_over_graphify"], 0.7)
        self.assertEqual(comparisons["baseline-hotpath"]["status"], "PASS")

    def test_unavailable_comparison_never_passes_threshold(self):
        manifest = {
            "comparator_pair": {"callers-direct": {"current": "current", "graphify": "graphify", "metrics": ["tokens"]}},
            "thresholds": {"current_vs_graphify": {"tokens": 0.70}, "hotpath": {"current_p95_multiplier": 1.10, "baseline_p95_additive_ms": 25}},
        }
        reports = {"current": {"status": "PASS", "tokens": {"n": 30, "p95": 1}}, "graphify": {"status": "NOT_COMPARABLE", "tokens": {"n": 0, "p95": None}}}
        self.assertEqual(_comparisons(reports, manifest)["callers-direct"]["status"], "NOT_COMPARABLE")


if __name__ == "__main__":
    unittest.main()
