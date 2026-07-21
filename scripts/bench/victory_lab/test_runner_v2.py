import json
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

import runner_v2
from schema_v2 import AdapterSpec, RunRecord


class FakeAdapter:
    calls = []

    def __init__(self, spec, manifest, root, executor=None):
        self.spec = spec

    def run_case(self, case, *, repetition):
        FakeAdapter.calls.append((self.spec.adapter_id, case["id"], repetition))
        return RunRecord(
            adapter_id=self.spec.adapter_id, operation=case["operation"], status="PASS", repetition=repetition,
            canonical={"digest": f"{self.spec.adapter_id}-{case['id']}-{repetition}"}, elapsed_ms=float(repetition + 1),
        )


class RunnerV2Tests(unittest.TestCase):
    def test_authoritative_requires_exactly_30(self):
        with self.assertRaises(ValueError):
            runner_v2.run_manifest("ignored", tempfile.mkdtemp(), repetitions=29)

    def test_serializes_adapters_cases_and_retains_all_samples(self):
        manifest = {
            "schema": "victory-lab-manifest/v2", "version": 2,
            "fixture_hashes": {"fixture": "a" * 64}, "oracle_hashes": {"oracle": "b" * 64},
            "adapters": [{"schema": "victory-adapter-spec/v2", "adapter_id": "a", "kind": "current", "command": ["fake"], "capabilities": ["affected"], "comparable_operations": ["affected"], "env_allowlist": []}],
            "cases": [{"id": "c", "operation": "affected", "corpus": ["x"], "golden": "o", "changed_paths": ["x"]}],
        }
        spec = AdapterSpec.from_dict(manifest["adapters"][0])
        FakeAdapter.calls = []
        with tempfile.TemporaryDirectory() as temp, patch.object(runner_v2, "load_manifest", return_value=manifest), patch.object(runner_v2, "manifest_adapters", return_value={"a": spec}), patch.object(runner_v2, "VictoryAdapter", FakeAdapter):
            summary = runner_v2.run_manifest("ignored", temp, repetitions=30)
            lines = (Path(temp) / "samples.jsonl").read_text().splitlines()
        self.assertEqual(summary["samples"], 30)
        self.assertEqual(len(lines), 30)
        self.assertEqual([call[2] for call in FakeAdapter.calls], list(range(30)))
        self.assertTrue(summary["anti_gaming"]["all_samples_retained"])


if __name__ == "__main__":
    unittest.main()
