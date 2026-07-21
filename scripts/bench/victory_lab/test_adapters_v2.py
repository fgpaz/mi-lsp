import hashlib
import json
import tempfile
import unittest
from pathlib import Path

from adapters.v2 import CommandResult, VictoryAdapter, _graphify_affected_payload, materialize_fixture
from manifest_v2 import load_manifest
from schema_v2 import AdapterSpec


class FakeExecutor:
    def __init__(self, *, timeout=False, crash=False):
        self.calls = []
        self.timeout = timeout
        self.crash = crash

    def run(self, argv, *, cwd, env, timeout_seconds):
        self.calls.append((list(argv), Path(cwd), dict(env), timeout_seconds))
        if "version" in argv:
            out = {"items": [{"vcs_revision": "cc8207f9a89201066f70343524db755d7b196c81"}]}
            return CommandResult(list(argv), str(cwd), sorted(env), 0, json.dumps(out))
        if "index" in argv:
            return CommandResult(list(argv), str(cwd), sorted(env), 0, json.dumps({"ok": True}))
        if self.timeout:
            return CommandResult(list(argv), str(cwd), sorted(env), 124, timed_out=True)
        if self.crash:
            return CommandResult(list(argv), str(cwd), sorted(env), -1, crashed=True)
        out = {"items": [{"display": "callers.Direct"}, {"display": "subject.Validate"}]}
        return CommandResult(list(argv), str(cwd), sorted(env), 0, json.dumps(out), elapsed_ms=4.0)


class AdapterV2Tests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.root = Path(__file__).resolve().parents[3]
        cls.manifest = load_manifest(cls.root / "benchmarks/victory-lab/v2/manifest.json")

    def _adapter(self, executor):
        with tempfile.NamedTemporaryFile(delete=False) as handle:
            handle.write(b"fake executable")
            executable = Path(handle.name)
        raw = next(item for item in self.manifest["adapters"] if item["adapter_id"] == "mi-lsp-current-v2").copy()
        raw["executable"] = str(executable)
        raw["expected_executable_sha256"] = hashlib.sha256(executable.read_bytes()).hexdigest()
        spec = AdapterSpec.from_dict(raw)
        return VictoryAdapter(spec, self.manifest, self.root / "benchmarks/victory-lab/v2", executor=executor)

    def test_materializes_project_state_only_in_temp(self):
        source = self.root / "benchmarks/victory-lab/v2/corpus"
        with materialize_fixture(source, self.manifest["repository_identity"]) as fixture:
            self.assertTrue((fixture / ".mi-lsp/project.toml").is_file())
            self.assertTrue((fixture / "go/subject/subject.go").is_file())
        self.assertFalse((source / ".mi-lsp").exists())

    def test_fake_subprocess_produces_pass_without_real_process(self):
        fake = FakeExecutor()
        record = self._adapter(fake).run_case(self.manifest["cases"][0])
        self.assertEqual(record.status, "PASS")
        self.assertEqual(len(fake.calls), 3)
        self.assertEqual(fake.calls[1][0][1], "index")
        self.assertIn("--format", fake.calls[-1][0])
        self.assertNotIn("stdout", record.to_dict())

    def test_timeout_and_crash_fail_closed(self):
        timeout = self._adapter(FakeExecutor(timeout=True)).run_case(self.manifest["cases"][0])
        crash = self._adapter(FakeExecutor(crash=True)).run_case(self.manifest["cases"][0])
        self.assertEqual(timeout.status, "FAIL")
        self.assertEqual(timeout.error["kind"], "timeout")
        self.assertEqual(crash.status, "FAIL")
        self.assertEqual(crash.error["kind"], "crash")

    def test_path_command_has_both_endpoints_and_no_unsupported_repo_flag(self):
        fake = FakeExecutor()
        record = self._adapter(fake).run_case(next(case for case in self.manifest["cases"] if case["operation"] == "path"))
        self.assertEqual(record.status, "FAIL")
        argv = fake.calls[-1][0]
        self.assertNotIn("--repo", argv)
        self.assertEqual(argv[argv.index("path") + 1:argv.index("--workspace")], ["Run", "Normalize"])

    def test_baseline_graph_only_cases_are_not_comparable(self):
        raw = next(item for item in self.manifest["adapters"] if item["kind"] == "baseline")
        baseline = VictoryAdapter(AdapterSpec.from_dict(raw), self.manifest, self.root / "benchmarks/victory-lab/v2")
        for case in self.manifest["cases"]:
            if case["operation"] != "affected":
                self.assertEqual(baseline.run_case(case).status, "NOT_COMPARABLE")

    def test_graphify_text_is_normalized_to_qualified_names(self):
        payload = _graphify_affected_payload(
            "Affected nodes for Normalize()\n- Validate() [calls] subject/subject.go:L9\n- Direct() [calls] callers/callers.go:L6\n"
        )
        self.assertEqual([item["display"] for item in payload["items"]], ["subject.Validate", "callers.Direct"])


if __name__ == "__main__":
    unittest.main()
