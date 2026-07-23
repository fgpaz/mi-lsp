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

    def test_authoritative_universe_is_exactly_eight_explicit_groups(self):
        groups = self.manifest["groups"]
        self.assertEqual(len(groups), 8)
        self.assertEqual({item["group_id"] for item in groups}, {
            "current-callers-direct", "current-callers-transitive",
            "graphify-callers-direct", "graphify-callers-transitive",
            "current-affected-direct", "current-affected-transitive",
            "baseline-affected-direct-hotpath", "current-path-shortest",
        })
        self.assertTrue(all(item["repetitions"] == 30 for item in groups))
        self.assertEqual(len({(item["adapter_id"], item["case_id"], item["operation"]) for item in groups}), 8)

    def test_manifest_rejects_cartesian_or_unsupported_graphify_group(self):
        broken = copy.deepcopy(self.manifest)
        broken["groups"].append({"group_id": "graphify-affected-direct", "adapter_id": "graphify-0.9.19-v2", "workload_id": "affected-direct", "case_id": "affected-direct", "operation": "affected", "repetitions": 30, "authoritative": True})
        with self.assertRaises(ManifestError):
            validate_manifest(broken, self.path.parent, check_files=False)
        broken = copy.deepcopy(self.manifest)
        broken["comparator_pair"]["callers-direct"]["metrics"] = ["tokens", "unknown"]
        with self.assertRaises(ManifestError):
            validate_manifest(broken, self.path.parent, check_files=False)

    def test_unavailable_scopes_are_explicit_not_comparable(self):
        for item in self.manifest["scopes"]:
            if item["scope"] in {"incremental", "build", "index"}:
                self.assertEqual(item["status"], "NOT_COMPARABLE")
                self.assertTrue(item["reason"])

    def test_group_contract_rejects_path_replacement_and_cross_operation_substitution(self):
        broken = copy.deepcopy(self.manifest)
        path_group = next(item for item in broken["groups"] if item["group_id"] == "current-path-shortest")
        path_group["adapter_id"] = "mi-lsp-baseline-v2"
        with self.assertRaises(ManifestError):
            validate_manifest(broken, self.path.parent, check_files=False)

        broken = copy.deepcopy(self.manifest)
        affected_group = next(item for item in broken["groups"] if item["group_id"] == "current-affected-direct")
        affected_group.update({"workload_id": "callers-direct", "case_id": "callers-direct", "operation": "callers"})
        with self.assertRaises(ManifestError):
            validate_manifest(broken, self.path.parent, check_files=False)

        broken = copy.deepcopy(self.manifest)
        broken["groups"] = [item for item in broken["groups"] if item["group_id"] != "current-path-shortest"]
        broken["groups"].append({
            "group_id": "baseline-path-shortest",
            "adapter_id": "mi-lsp-baseline-v2",
            "workload_id": "path-shortest",
            "case_id": "path-shortest",
            "operation": "path",
            "repetitions": 30,
            "authoritative": True,
        })
        with self.assertRaises(ManifestError):
            validate_manifest(broken, self.path.parent, check_files=False)

    def test_pair_contract_rejects_cross_mode_cross_operation_and_nonreciprocal_pairs(self):
        broken = copy.deepcopy(self.manifest)
        broken["comparator_pair"]["callers-direct"]["graphify"] = "graphify-callers-transitive"
        with self.assertRaises(ManifestError):
            validate_manifest(broken, self.path.parent, check_files=False)

        broken = copy.deepcopy(self.manifest)
        broken["comparator_pair"]["callers-direct"]["current"] = "current-affected-direct"
        with self.assertRaises(ManifestError):
            validate_manifest(broken, self.path.parent, check_files=False)

        broken = copy.deepcopy(self.manifest)
        broken["hotpath_pair"]["baseline"] = "current-affected-direct"
        with self.assertRaises(ManifestError):
            validate_manifest(broken, self.path.parent, check_files=False)

        broken = copy.deepcopy(self.manifest)
        broken.pop("hotpath_pair")
        with self.assertRaises(ManifestError):
            validate_manifest(broken, self.path.parent, check_files=False)

    def test_pair_contract_rejects_graphify_affected_and_scope_expansion(self):
        broken = copy.deepcopy(self.manifest)
        broken["comparator_pair"]["affected-direct"] = {
            "current": "current-affected-direct",
            "graphify": "graphify-callers-direct",
            "metrics": ["warm_p95"],
        }
        with self.assertRaises(ManifestError):
            validate_manifest(broken, self.path.parent, check_files=False)

        broken = copy.deepcopy(self.manifest)
        broken["scopes"].append({"scope": "query", "status": "PASS", "reason": "measured"})
        with self.assertRaises(ManifestError):
            validate_manifest(broken, self.path.parent, check_files=False)


if __name__ == "__main__":
    unittest.main()
