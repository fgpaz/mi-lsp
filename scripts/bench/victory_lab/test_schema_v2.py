import unittest

from schema_v2 import AdapterSpec, RunRecord, SchemaError


class SchemaV2Tests(unittest.TestCase):
    def test_adapter_and_record_are_versioned(self):
        spec = AdapterSpec.from_dict({
            "schema": "victory-adapter-spec/v2", "adapter_id": "fake", "kind": "current",
            "capabilities": ["affected"], "comparable_operations": ["affected"],
            "normalizable_operations": [], "env_allowlist": ["PATH", "TEMP", "TMP", "MI_LSP_CLIENT_NAME", "MI_LSP_SESSION_ID"],
            "command": ["fake", "{operation}"], "metadata_command": [],
        })
        self.assertEqual(spec.to_dict()["schema"], "victory-adapter-spec/v2")
        record = RunRecord(adapter_id="fake", operation="affected", status="NOT_COMPARABLE")
        self.assertEqual(record.to_dict()["schema"], "victory-run-record/v2")

    def test_pass_cannot_be_raw_or_missing_canonical(self):
        with self.assertRaises(SchemaError):
            RunRecord(adapter_id="fake", operation="affected", status="PASS").to_dict()
        with self.assertRaises(SchemaError):
            RunRecord(adapter_id="fake", operation="affected", status="PASS", canonical={}, metrics={"stdout": "leak"}).to_dict()

    def test_graphify_affected_requires_declared_normalization(self):
        with self.assertRaises(SchemaError):
            AdapterSpec.from_dict({
                "schema": "victory-adapter-spec/v2", "adapter_id": "graphify", "kind": "graphify",
                "capabilities": ["affected"], "comparable_operations": ["affected"],
                "normalizable_operations": [], "env_allowlist": [], "command": ["python"],
            })


if __name__ == "__main__":
    unittest.main()
