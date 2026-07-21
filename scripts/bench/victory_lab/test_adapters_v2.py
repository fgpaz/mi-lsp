import copy
import hashlib
import json
import tempfile
import unittest
from pathlib import Path

from adapters.v2 import AdapterError, CommandResult, VictoryAdapter, _adapt_mi_lsp_terminal, _graphify_affected_payload, _graphify_graph_path, _normalize_set_payload, materialize_fixture
from manifest_v2 import load_manifest
from schema_v2 import AdapterSpec


class FakeExecutor:
    def __init__(self, *, timeout=False, crash=False, commit="11ac8af870d4110b6b4333199b8a8343c52ce784", items=None, runtime_proof=None):
        self.calls = []
        self.timeout = timeout
        self.crash = crash
        self.commit = commit
        self.items = items or [{"display": "callers.Direct"}, {"display": "subject.Validate"}]
        self.runtime_proof = runtime_proof

    def run(self, argv, *, cwd, env, timeout_seconds):
        self.calls.append((list(argv), Path(cwd), dict(env), timeout_seconds))
        if "version" in argv:
            out = {"items": [{"vcs_revision": self.commit, "version": "(devel)"}]}
            return CommandResult(list(argv), str(cwd), sorted(env), 0, json.dumps(out), runtime_proof=self.runtime_proof)
        if "index" in argv:
            return CommandResult(list(argv), str(cwd), sorted(env), 0, json.dumps({"ok": True, "backend": "go", "completeness": "complete", "truncated": False, "items": []}), runtime_proof=self.runtime_proof)
        if "find" in argv:
            return CommandResult(list(argv), str(cwd), sorted(env), 0, json.dumps({"ok": True, "backend": "go", "completeness": "complete", "truncated": False, "items": [{"display": "subject.Normalize"}]}), runtime_proof=self.runtime_proof)
        if self.timeout:
            return CommandResult(list(argv), str(cwd), sorted(env), 124, timed_out=True)
        if self.crash:
            return CommandResult(list(argv), str(cwd), sorted(env), -1, crashed=True)
        out = {"ok": True, "backend": "go", "completeness": "complete", "truncated": False, "items": self.items}
        return CommandResult(list(argv), str(cwd), sorted(env), 0, json.dumps(out), elapsed_ms=4.0, runtime_proof=self.runtime_proof)


class GraphifyPrepareExecutor:
    def __init__(self, *, write_graph=True, help_text=None):
        self.calls = []
        self.write_graph = write_graph
        self.help_text = help_text or (
            '  affected "X"             reverse traversal\n'
            '    --relation R            edge relation to traverse in reverse (repeatable)\n'
        )

    def run(self, argv, *, cwd, env, timeout_seconds):
        self.calls.append((list(argv), Path(cwd), dict(env), timeout_seconds))
        if "--help" in argv:
            return CommandResult(list(argv), str(cwd), sorted(env), 0, self.help_text)
        if self.write_graph:
            graph_path = _graphify_graph_path(Path(cwd))
            graph_path.parent.mkdir(parents=True, exist_ok=True)
            graph_path.write_text("{}", encoding="utf-8")
        return CommandResult(list(argv), str(cwd), sorted(env), 0, "", elapsed_ms=4.0)


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
        # This unit test isolates subprocess sequencing with a synthetic
        # executable.  Attestation binding is exercised separately with real
        # manifested artifacts; a synthetic binary must not be mistaken for
        # that provenance evidence.
        raw.pop("attestation_path", None)
        raw.pop("expected_attestation_sha256", None)
        test_manifest = copy.deepcopy(self.manifest)
        test_manifest.pop("provenance_contract", None)
        spec = AdapterSpec.from_dict(raw)
        return VictoryAdapter(spec, test_manifest, self.root / "benchmarks/victory-lab/v2", executor=executor)

    def _graphify_adapter(self, executor):
        with tempfile.NamedTemporaryFile(delete=False) as handle:
            handle.write(b"fake python")
            executable = Path(handle.name)
        raw = next(item for item in self.manifest["adapters"] if item["kind"] == "graphify").copy()
        raw["executable"] = str(executable)
        raw["expected_executable_sha256"] = hashlib.sha256(executable.read_bytes()).hexdigest()
        raw.pop("attestation_path", None)
        raw.pop("expected_attestation_sha256", None)
        test_manifest = copy.deepcopy(self.manifest)
        test_manifest.pop("provenance_contract", None)
        return VictoryAdapter(AdapterSpec.from_dict(raw), test_manifest, self.root / "benchmarks/victory-lab/v2", executor=executor)

    def _baseline_adapter(self, executor):
        with tempfile.NamedTemporaryFile(delete=False) as handle:
            handle.write(b"fake executable")
            executable = Path(handle.name)
        raw = next(item for item in self.manifest["adapters"] if item["kind"] == "baseline").copy()
        raw["executable"] = str(executable)
        raw["expected_executable_sha256"] = hashlib.sha256(executable.read_bytes()).hexdigest()
        raw.pop("attestation_path", None)
        raw.pop("expected_attestation_sha256", None)
        test_manifest = copy.deepcopy(self.manifest)
        test_manifest.pop("provenance_contract", None)
        spec = AdapterSpec.from_dict(raw)
        return VictoryAdapter(spec, test_manifest, self.root / "benchmarks/victory-lab/v2", executor=executor)

    def test_materializes_project_state_only_in_temp(self):
        source = self.root / "benchmarks/victory-lab/v2/corpus"
        with materialize_fixture(source, self.manifest["repository_identity"]) as fixture:
            self.assertTrue((fixture / ".mi-lsp/project.toml").is_file())
            self.assertTrue((fixture / "corpus/go/subject/subject.go").is_file())
            project = (fixture / ".mi-lsp/project.toml").read_text(encoding="utf-8")
            self.assertIn('root = "corpus/go"', project)
        self.assertFalse((source / ".mi-lsp").exists())

    def test_fake_subprocess_produces_pass_without_real_process(self):
        fake = FakeExecutor()
        record = self._adapter(fake).run_case(self.manifest["cases"][0])
        self.assertEqual(record.status, "NOT_COMPARABLE")
        self.assertEqual(len(fake.calls), 4)
        self.assertEqual(fake.calls[2][0][1:3], ["nav", "find"])
        self.assertEqual(fake.calls[1][0][1], "index")
        self.assertIn("--format", fake.calls[-1][0])
        self.assertNotIn("stdout", record.to_dict())
        self.assertEqual(record.metrics["security"]["status"], "NOT_COMPARABLE")
        self.assertFalse(record.metrics["security"]["runtime_proof"])

    def test_timeout_and_crash_fail_closed(self):
        timeout = self._adapter(FakeExecutor(timeout=True)).run_case(self.manifest["cases"][0])
        crash = self._adapter(FakeExecutor(crash=True)).run_case(self.manifest["cases"][0])
        self.assertEqual(timeout.status, "NOT_COMPARABLE")
        self.assertEqual(timeout.error["kind"], "timeout")
        self.assertEqual(crash.status, "NOT_COMPARABLE")
        self.assertEqual(crash.error["kind"], "crash")

    def test_path_command_has_both_endpoints_depth_and_no_unsupported_repo_flag(self):
        fake = FakeExecutor()
        record = self._adapter(fake).run_case(next(case for case in self.manifest["cases"] if case["operation"] == "path"))
        self.assertEqual(record.status, "NOT_COMPARABLE")
        argv = fake.calls[-1][0]
        self.assertNotIn("--repo", argv)
        path_start = argv.index("path") + 1
        self.assertEqual(argv[path_start:path_start + 2], ["Run", "Normalize"])
        self.assertEqual(argv[argv.index("--depth") + 1], "2")

    def test_set_normalization_qualifies_and_sorts_without_oracle(self):
        payload = {
            "items": [
                {"display": "Validate", "owner_path": "subject/subject.go"},
                {"display": "Run", "owner_path": "app/app.go"},
                {"display": "Direct", "owner_path": "callers/callers.go"},
            ]
        }
        normalized = _normalize_set_payload(payload, "callers")
        self.assertEqual(
            [item["display"] for item in normalized["items"]],
            ["app.Run", "callers.Direct", "subject.Validate"],
        )

    def test_path_normalization_preserves_native_route_order(self):
        payload = {
            "items": [
                {"display": "Run", "owner_path": "app/app.go"},
                {"display": "Direct", "owner_path": "callers/callers.go"},
                {"display": "Normalize", "owner_path": "subject/subject.go"},
            ]
        }
        normalized = _normalize_set_payload(payload, "path")
        self.assertEqual(
            [item["display"] for item in normalized["items"]],
            ["app.Run", "callers.Direct", "subject.Normalize"],
        )

    def test_affected_normalization_uses_path_identity_and_deduplicates_files(self):
        payload = {
            "items": [
                {"path": "subject\\subject.go", "display": "subject.Uncalled"},
                {"path": "callers/callers.go", "display": "callers.Direct"},
                {"path": "subject/subject.go", "display": "subject.Validate"},
                {"path": "app/app.go", "display": "app.Run"},
            ]
        }
        normalized = _normalize_set_payload(payload, "affected")
        self.assertEqual(
            [item["path"] for item in normalized["items"]],
            ["app/app.go", "callers/callers.go", "subject/subject.go"],
        )
        self.assertEqual(
            [item["display"] for item in normalized["items"]],
            ["app.Run", "callers.Direct", "subject.Uncalled"],
        )

    def test_affected_comparison_uses_paths_not_symbol_displays(self):
        fake = FakeExecutor(
            items=[
                {"path": "subject/subject.go", "display": "subject.Uncalled"},
                {"path": "callers/callers.go", "display": "callers.Direct"},
                {"path": "subject/subject.go", "display": "subject.Validate"},
            ],
            runtime_proof={"status": "PASS", "runtime_proof": True, "sample_count": 1},
        )
        record = self._adapter(fake).run_case(
            next(case for case in self.manifest["cases"] if case["id"] == "affected-direct")
        )
        self.assertEqual(record.status, "NOT_COMPARABLE")

    def test_baseline_affected_executes_hot_path_but_is_not_comparable(self):
        fake = FakeExecutor(commit="a251ab1f8db4e96f029926fbef275b078a20a111")
        record = self._baseline_adapter(fake).run_case(
            next(case for case in self.manifest["cases"] if case["id"] == "affected-direct")
        )
        self.assertEqual(record.status, "NOT_COMPARABLE")
        self.assertEqual(record.error, {"kind": "comparability", "reason_code": "comparability"})
        self.assertGreaterEqual(len(fake.calls), 3)
        self.assertEqual(fake.calls[1][0][1], "index")
        self.assertEqual(fake.calls[-1][0][1:3], ["nav", "affected"])
        self.assertIn("subject/subject.go", fake.calls[-1][0])
        self.assertIn("security", record.metrics)

    def test_affected_transitive_uses_transitive_path_oracle(self):
        items = [
            {"path": "subject/subject.go", "display": "subject.Validate"},
            {"path": "callers/callers.go", "display": "callers.Indirect"},
            {"path": "app/app.go", "display": "app.Run"},
            {"path": "callers/callers.go", "display": "callers.Direct"},
        ]
        fake = FakeExecutor(items=items, runtime_proof={"status": "PASS", "runtime_proof": True, "sample_count": 1})
        record = self._adapter(fake).run_case(
            next(case for case in self.manifest["cases"] if case["id"] == "affected-transitive")
        )
        self.assertEqual(record.status, "NOT_COMPARABLE")
        self.assertEqual(len(record.canonical["payload"]["items"]), 3)

    def test_baseline_graph_only_cases_are_not_comparable(self):
        raw = next(item for item in self.manifest["adapters"] if item["kind"] == "baseline")
        baseline = VictoryAdapter(AdapterSpec.from_dict(raw), self.manifest, self.root / "benchmarks/victory-lab/v2")
        for case in self.manifest["cases"]:
            if case["operation"] != "affected":
                self.assertEqual(baseline.run_case(case).status, "NOT_COMPARABLE")

    def test_graphify_prepare_probes_top_level_help_and_pins_single_graph_path(self):
        executor = GraphifyPrepareExecutor()
        adapter = self._graphify_adapter(executor)
        case = next(case for case in self.manifest["cases"] if case["operation"] == "callers")
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            result = adapter._prepare_graphify(root, case, {})
            self.assertEqual(result.returncode, 0)
            self.assertEqual(executor.calls[0][0][1:], ["-m", "graphify", "--help"])
            extract_argv = executor.calls[1][0]
            self.assertIn("extract", extract_argv)
            out_index = extract_argv.index("--out")
            self.assertEqual(Path(extract_argv[out_index + 1]), root)
            self.assertTrue(_graphify_graph_path(root).is_file())
            self.assertFalse((root / "graphify-out" / "graphify-out" / "graph.json").exists())

    def test_graphify_prepare_rejects_subcommand_help_without_top_level_contract(self):
        executor = GraphifyPrepareExecutor(help_text="Run 'graphify --help' for full usage.")
        adapter = self._graphify_adapter(executor)
        case = next(case for case in self.manifest["cases"] if case["operation"] == "callers")
        with tempfile.TemporaryDirectory() as temp:
            with self.assertRaisesRegex(AdapterError, "affected --relation"):
                adapter._prepare_graphify(Path(temp), case, {})
        self.assertEqual(len(executor.calls), 1)

    def test_graphify_prepare_rejects_success_without_pinned_graph(self):
        executor = GraphifyPrepareExecutor(write_graph=False)
        adapter = self._graphify_adapter(executor)
        case = next(case for case in self.manifest["cases"] if case["operation"] == "callers")
        with tempfile.TemporaryDirectory() as temp:
            with self.assertRaisesRegex(AdapterError, "graphify-out/graph.json"):
                adapter._prepare_graphify(Path(temp), case, {})

    def test_graphify_text_is_normalized_to_qualified_names(self):
        payload = _graphify_affected_payload(
            "Affected nodes for Normalize()\n- Validate() [calls] subject/subject.go:L9\n- Direct() [calls] callers/callers.go:L6\n"
        )
        self.assertTrue(payload["done"])
        self.assertEqual([item["display"] for item in payload["items"]], ["subject.Validate", "callers.Direct"])

    def test_graphify_empty_stdout_fails_closed(self):
        for stdout in ("  \r\n", "Affected nodes for Normalize()\n"):
            with self.assertRaises(ValueError):
                _graphify_affected_payload(stdout)

    def test_mi_lsp_terminality_is_adapted_only_after_complete_process(self):
        native = {"ok": True, "backend": "go", "truncated": False, "error": "", "items": []}
        result = CommandResult([], "", [], 0, "")
        adapted = _adapt_mi_lsp_terminal(native, result)
        self.assertTrue(adapted["done"])
        partial = dict(native, partial=True)
        self.assertNotIn("done", _adapt_mi_lsp_terminal(partial, result))
        failed = _adapt_mi_lsp_terminal(native, CommandResult([], "", [], 1, ""))
        self.assertNotIn("done", failed)
        timed_out = _adapt_mi_lsp_terminal(native, CommandResult([], "", [], 0, "", timed_out=True))
        self.assertNotIn("done", timed_out)


if __name__ == "__main__":
    unittest.main()
