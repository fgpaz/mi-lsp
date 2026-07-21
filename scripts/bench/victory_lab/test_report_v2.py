import json
import tempfile
import unittest
from pathlib import Path

from canonical_v2 import payload_digest
from report_v2 import build_report


class ReportV2Tests(unittest.TestCase):
    def _write(self, records):
        temp = tempfile.NamedTemporaryFile(mode="w", suffix=".jsonl", delete=False, encoding="utf-8")
        with temp:
            for record in records:
                temp.write(json.dumps(record) + "\n")
        return Path(temp.name)

    def _record(self, repetition, *, status="PASS", payload=None, child_status="PASS", peak=1, case_id="case"):
        payload = payload or {"items": ["stable"]}
        canonical = {
            "schema": "victory-canonical/v2", "operation": "affected", "payload": payload,
            "digest": payload_digest(payload), "token_units": 1,
        } if status == "PASS" else None
        return {
            "schema": "victory-run-record/v2", "case_id": case_id, "adapter_id": "fake", "operation": "affected",
            "status": status, "repetition": repetition, "fixture_digest": "a" * 64, "oracle_digest": "b" * 64,
            "executable_sha256": "", "source_sha256": "", "commit": "", "version": "", "capabilities": ["affected"],
            "argv": [], "cwd": "", "env_keys": [], "elapsed_ms": repetition + 1,
            "canonical": canonical, "metrics": {"child": {"status": child_status, "peak_rss_bytes": peak, "tree_peak_rss_bytes": peak, "tree_supported": True, "cleanup_status": "clean"}, "security": {"status": "PASS", "runtime_proof": True, "runtime": {"status": "PASS", "runtime_proof": True, "sample_count": 1, "observed_network_count": 0, "observed_mcp_count": 0, "evidence_digest": "c" * 64}, "integrity": {"status": "PASS"}}},
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


if __name__ == "__main__":
    unittest.main()
