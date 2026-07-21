import json
import tempfile
import unittest
from pathlib import Path

from report_v2 import build_report


class ReportV2Tests(unittest.TestCase):
    def _write(self, records):
        temp = tempfile.NamedTemporaryFile(mode="w", suffix=".jsonl", delete=False, encoding="utf-8")
        with temp:
            for record in records:
                temp.write(json.dumps(record) + "\n")
        return Path(temp.name)

    def test_reports_p50_p95_and_max_from_all_samples(self):
        records = []
        for repetition in range(30):
            records.append({
                "schema": "victory-run-record/v2", "adapter_id": "fake", "operation": "affected", "status": "PASS", "repetition": repetition,
                "canonical": {"digest": str(repetition)}, "metrics": {}, "elapsed_ms": repetition + 1,
            })
        report = build_report(self._write(records))
        latency = report["groups"]["fake:affected"]["latency"]
        self.assertEqual(latency["n"], 30)
        self.assertEqual(latency["max_ms"], 30)
        self.assertGreater(latency["p95_ms"], latency["p50_ms"])
        self.assertTrue(report["anti_gaming"]["all_samples_used"])

    def test_best_of_fields_are_rejected(self):
        records = [{
            "schema": "victory-run-record/v2", "adapter_id": "fake", "operation": "affected", "status": "PASS", "repetition": i,
            "canonical": {"digest": str(i)}, "metrics": {}, "elapsed_ms": 1, "best_of": True,
        } for i in range(30)]
        with self.assertRaises(ValueError):
            build_report(self._write(records))

    def test_missing_sample_is_rejected(self):
        record = {"schema": "victory-run-record/v2", "adapter_id": "fake", "operation": "affected", "status": "PASS", "repetition": 0, "canonical": {}, "metrics": {}, "elapsed_ms": 1}
        with self.assertRaises(ValueError):
            build_report(self._write([record]))

    def test_duplicate_repetition_is_rejected_even_when_count_is_30(self):
        records = [{
            "schema": "victory-run-record/v2", "adapter_id": "fake", "operation": "affected", "status": "PASS", "repetition": 0,
            "canonical": {"digest": str(i)}, "metrics": {}, "elapsed_ms": 1,
        } for i in range(30)]
        with self.assertRaises(ValueError):
            build_report(self._write(records))


if __name__ == "__main__":
    unittest.main()
