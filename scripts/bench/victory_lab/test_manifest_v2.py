import copy
import unittest
from pathlib import Path

from manifest_v2 import ManifestError, load_manifest, validate_manifest


class ManifestV2Tests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.root = Path(__file__).resolve().parents[3]
        cls.path = cls.root / "benchmarks/victory-lab/v2/manifest.json"
        cls.manifest = load_manifest(cls.path)

    def test_manifest_hashes_and_required_pins_validate(self):
        self.assertEqual(self.manifest["baseline_commit"], "a251ab1f8db4e96f029926fbef275b078a20a111")
        self.assertEqual(self.manifest["graphify"]["version"], "0.9.19")
        self.assertEqual(self.manifest["default_repetitions"], 30)

    def test_baseline_cannot_claim_graph_only_operations(self):
        broken = copy.deepcopy(self.manifest)
        baseline = next(item for item in broken["adapters"] if item["kind"] == "baseline")
        baseline["comparable_operations"] = ["path"]
        baseline["capabilities"] = ["path"]
        with self.assertRaises(ManifestError):
            validate_manifest(broken, self.path.parent, check_files=False)

    def test_current_commit_must_be_full_pin(self):
        broken = copy.deepcopy(self.manifest)
        broken["current"]["commit"] = "short"
        with self.assertRaises(ManifestError):
            validate_manifest(broken, self.path.parent, check_files=False)


if __name__ == "__main__":
    unittest.main()
