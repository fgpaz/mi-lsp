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
            canonical={"schema": "victory-canonical/v2", "operation": case["operation"], "payload": {"stable": True}, "digest": "" + "0" * 64, "token_units": 1},
            elapsed_ms=float(repetition + 1), metrics={"child": {"status": "PASS", "peak_rss_bytes": 1}},
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

    def test_serializes_adapters_cases_and_retains_all_samples(self):
        manifest = {
            "schema": "victory-lab-manifest/v2", "version": 2,
            "fixture_hashes": {"fixture": "a" * 64}, "oracle_hashes": {"oracle": "b" * 64},
            "adapters": [{"schema": "victory-adapter-spec/v2", "adapter_id": "a", "kind": "current", "expected_commit": "c" * 40, "expected_executable_sha256": "a" * 64, "metadata_command": ["fake", "version"], "command": ["fake"], "capabilities": ["affected"], "comparable_operations": ["affected"], "env_allowlist": []}],
            "cases": [{"id": "c", "operation": "affected", "corpus": ["x"], "golden": "o", "changed_paths": ["x"]}],
        }
        spec = AdapterSpec.from_dict(manifest["adapters"][0])
        FakeAdapter.calls = []
        with tempfile.TemporaryDirectory() as temp, patch.object(runner_v2, "load_manifest", return_value=manifest), patch.object(runner_v2, "validate_strict_manifest"), patch.object(runner_v2, "validate_runtime", return_value=[]), patch.object(runner_v2, "manifest_adapters", return_value={"a": spec}), patch.object(runner_v2, "VictoryAdapter", FakeAdapter):
            root = Path(temp)
            (root / "manifest.json").write_text("{}", encoding="utf-8")
            (root / "fixture").write_text("fixture", encoding="utf-8")
            (root / "oracle").write_text("oracle", encoding="utf-8")
            summary = runner_v2.run_manifest(str(root / "manifest.json"), str(root / "output"), repetitions=30)
            lines = (root / "output" / "samples.jsonl").read_text().splitlines()
        self.assertEqual(summary["samples"], 30)
        self.assertEqual(len(lines), 30)
        self.assertEqual([call[2] for call in FakeAdapter.calls], list(range(30)))
        self.assertTrue(summary["anti_gaming"]["all_samples_retained"])


if __name__ == "__main__":
    unittest.main()
