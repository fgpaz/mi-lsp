import unittest

from canonical_v2 import payload_digest
from schema_v2 import AdapterSpec, RunRecord, SchemaError


class SchemaV2Tests(unittest.TestCase):
    def test_adapter_and_record_are_versioned(self):
        spec = AdapterSpec.from_dict({
            "schema": "victory-adapter-spec/v2", "adapter_id": "fake", "kind": "current",
            "expected_commit": "c" * 40, "expected_executable_sha256": "a" * 64,
            "capabilities": ["affected"], "comparable_operations": ["affected"],
            "normalizable_operations": [], "env_allowlist": ["PATH", "TEMP", "TMP", "MI_LSP_CLIENT_NAME", "MI_LSP_SESSION_ID"],
            "command": ["fake", "{operation}"], "metadata_command": ["fake", "version"],
        })
        self.assertEqual(spec.to_dict()["schema"], "victory-adapter-spec/v2")
        record = RunRecord(
            adapter_id="fake", operation="affected", status="NOT_COMPARABLE",
            error={"kind": "capability", "reason_code": "unavailable"},
        )
        self.assertEqual(record.to_dict()["schema"], "victory-run-record/v2")

    def test_pass_cannot_be_raw_or_missing_canonical(self):
        with self.assertRaises(SchemaError):
            RunRecord(adapter_id="fake", operation="affected", status="PASS").to_dict()
        with self.assertRaises(SchemaError):
            RunRecord(
                adapter_id="fake", operation="affected", status="PASS",
                canonical={"schema": "victory-canonical/v2", "operation": "affected", "payload": {}, "digest": payload_digest({}), "token_units": 0},
                metrics={"stdout": "leak"},
            ).to_dict()

    def test_graphify_affected_requires_declared_normalization(self):
        with self.assertRaises(SchemaError):
            AdapterSpec.from_dict({
                "schema": "victory-adapter-spec/v2", "adapter_id": "graphify", "kind": "graphify",
                "capabilities": ["affected"], "comparable_operations": ["affected"],
                "normalizable_operations": [], "env_allowlist": [], "command": ["python"],
            })

    def test_schema_drift_and_raw_log_are_rejected(self):
        record = {
            "schema": "victory-run-record/v2", "case_id": "case", "adapter_id": "fake", "operation": "affected",
            "status": "PASS", "repetition": 0, "fixture_digest": "a" * 64, "oracle_digest": "b" * 64,
            "capabilities": ["affected"], "argv": [], "cwd": "", "env_keys": [], "elapsed_ms": 1,
            "canonical": {"schema": "victory-canonical/v2", "operation": "affected", "payload": {}, "digest": payload_digest({}), "token_units": 0},
            "metrics": {"child": {"status": "PASS", "peak_rss_bytes": 1}}, "error": None, "unexpected": True,
        }
        with self.assertRaises(SchemaError):
            RunRecord.from_dict(record)
        record.pop("unexpected")
        record["canonical"]["payload"]["stdout"] = "raw log"
        with self.assertRaises(SchemaError):
            RunRecord.from_dict(record)


if __name__ == "__main__":
    unittest.main()
