import concurrent.futures
import tempfile
import unittest
from pathlib import Path
from types import SimpleNamespace

from scripts.bench.harness_first.runner import (
    HarnessError,
    MARKER_NAME,
    SECTIONS,
    _RSSSampler,
    _command,
    _rss_report,
    claim_global_marker,
    parse_go_version_m,
    query_check,
    to_yaml,
    validate_manifest,
    worker_status_check,
    write_marker,
)


def measured_query_check(query, payload):
    return query_check(query, payload, output_bytes=1, estimated_tokens=1)


class RunnerContractTests(unittest.TestCase):
    def manifest(self):
        return {
            "schema": "harness-first-campaign/v1",
            "campaign_id": "dry-run",
            "queries": [{
                "id": "explain-change",
                "kind": "explain_change",
                "args": ["nav", "explain-change", "--path", "internal/service/intent.go"],
                "preview_required": True,
                "modes": ["direct"],
            }],
        }

    def preview_payload(self, *, explain=True):
        omissions = [
            {"section": section, "reason": f"no {section} evidence was available"}
            for section in ("callers", "callees", "tests")
        ]
        plan = {
            "preview": [
                {"section": "change", "items": [{"path": "src/change.go"}], "count": 1},
                {"section": "affected", "items": [{"path": "src/affected.go"}], "count": 1},
                {"section": "callers", "items": [], "count": 0},
                {"section": "callees", "items": [], "count": 0},
                {"section": "tests", "items": [], "count": 0},
                {"section": "contracts", "items": [{"path": "CT-EXAMPLE"}], "count": 1},
                {"section": "wiki", "items": [{"path": ".docs/wiki/00_gobierno_documental.md"}], "count": 1},
            ],
            "omissions": omissions,
            "expansions": [{"command": "mi-lsp nav affected --full", "reason": "expand affected evidence"}],
        }
        return {"ok": True, "items": [plan]}

    def test_manifest_locks_seven_sections_and_expansions(self):
        contract = validate_manifest(self.manifest())
        self.assertEqual(contract["budgets"]["preview_sections"], list(SECTIONS))
        self.assertEqual(contract["budgets"]["preview_expansions"], ["command", "reason"])

    def test_direct_only_queries_cannot_be_labeled_daemon(self):
        manifest = self.manifest()
        manifest["queries"][0]["modes"] = ["direct", "daemon"]
        with self.assertRaises(HarnessError):
            validate_manifest(manifest)

    def test_command_routes_direct_and_daemon_modes_explicitly(self):
        query = {"args": ["nav", "related", "BuildIntentPlan"]}
        direct = _command(Path("mi-lsp"), Path("workspace"), "campaign", query, "direct")
        daemon = _command(Path("mi-lsp"), Path("workspace"), "campaign", query, "daemon")
        self.assertIn("--no-daemon", direct)
        self.assertNotIn("--no-auto-daemon", direct)
        self.assertNotIn("--no-daemon", daemon)
        self.assertEqual(direct[-3:], ["nav", "related", "BuildIntentPlan"])
        self.assertEqual(daemon[-3:], ["nav", "related", "BuildIntentPlan"])

    def test_budget_drift_is_rejected(self):
        manifest = self.manifest()
        manifest["budgets"] = {"parity": False}
        with self.assertRaises(HarnessError):
            validate_manifest(manifest)

    def test_manifest_rejects_unknown_kind(self):
        manifest = self.manifest()
        manifest["queries"][0]["kind"] = "unknown_kind"
        with self.assertRaises(HarnessError):
            validate_manifest(manifest)

    def test_explain_change_requires_preview(self):
        manifest = self.manifest()
        manifest["queries"][0].pop("preview_required")
        with self.assertRaises(HarnessError):
            validate_manifest(manifest)

    def test_related_requires_freshness_rank_flag(self):
        manifest = {
            "schema": "harness-first-campaign/v1",
            "campaign_id": "related-contract",
            "queries": [{
                "id": "related",
                "kind": "related",
                "args": ["nav", "related", "BuildIntentPlan"],
                "modes": ["direct"],
            }],
        }
        with self.assertRaises(HarnessError):
            validate_manifest(manifest)

    def test_workspace_map_requires_freshness_rank_flag(self):
        manifest = {
            "schema": "harness-first-campaign/v1",
            "campaign_id": "workspace-contract",
            "queries": [{
                "id": "workspace-map",
                "kind": "workspace_map",
                "args": ["nav", "workspace-map"],
                "modes": ["direct"],
                "freshness_rank_required": False,
            }],
        }
        with self.assertRaises(HarnessError):
            validate_manifest(manifest)

    def test_stale_graph_requires_flag_and_cannot_be_passed_operationally(self):
        manifest = {
            "schema": "harness-first-campaign/v1",
            "campaign_id": "stale-contract",
            "queries": [{
                "id": "related",
                "kind": "related",
                "args": ["nav", "related", "BuildIntentPlan"],
                "modes": ["direct"],
            }],
        }
        with self.assertRaises(HarnessError):
            validate_manifest(manifest)

    def test_freshness_not_required_is_not_a_failure(self):
        result = measured_query_check(self.manifest()["queries"][0], self.preview_payload())
        self.assertEqual(result["status"], "PASS")
        self.assertEqual(result["freshness_rank"]["status"], "NOT_REQUIRED")

    def test_freshness_required_passes_only_with_evidence(self):
        query = dict(self.manifest()["queries"][0], freshness_rank_required=True)
        payload = self.preview_payload()
        payload["items"][0]["freshness_rank"] = 1
        payload["items"][0]["graph_freshness"] = {"state": "current"}
        result = measured_query_check(query, payload)
        self.assertEqual(result["status"], "PASS")
        self.assertEqual(result["freshness_rank"]["status"], "PASS")

        payload.pop("items")[0] if False else None
        missing = self.preview_payload()
        failed = measured_query_check(query, missing)
        self.assertEqual(failed["status"], "FAIL")
        self.assertEqual(failed["freshness_rank"]["status"], "FAIL")

    def test_non_current_freshness_states_never_pass(self):
        query = {"id": "related", "kind": "related", "args": ["nav", "related", "BuildIntentPlan"], "freshness_rank_required": True}
        for state in ("lagging", "stale", "invalid", "unknown"):
            with self.subTest(state=state):
                payload = {"ok": True, "items": [{"graph_freshness": {"state": state}, "graph_ranks": [1]}]}
                result = measured_query_check(query, payload)
                self.assertEqual(result["status"], "FAIL")
                self.assertEqual(result["freshness_rank"]["status"], "FAIL")

    def test_mixed_freshness_states_fail_closed(self):
        query = {"id": "related", "kind": "related", "args": ["nav", "related", "BuildIntentPlan"], "freshness_rank_required": True}
        payload = {"ok": True, "items": [{"graph_freshness": {"state": "current"}, "graph_ranks": [1]}], "metadata": {"graph_freshness": {"state": "lagging"}}}
        result = measured_query_check(query, payload)
        self.assertEqual(result["freshness_rank"]["status"], "FAIL")
        self.assertEqual(result["status"], "FAIL")

    def test_stale_state_at_list_index_64_fails(self):
        query = {"id": "related", "kind": "related", "args": ["nav", "related", "BuildIntentPlan"], "freshness_rank_required": True}
        current = {"graph_freshness": {"state": "current"}, "graph_ranks": []}
        stale = {"graph_freshness": {"state": "stale"}, "graph_ranks": []}
        payload = {"ok": True, "items": [current.copy() for _ in range(64)] + [stale]}
        result = measured_query_check(query, payload)
        self.assertEqual(result["freshness_rank"]["status"], "FAIL")
        self.assertIn("__freshness_traversal_truncated__", result["freshness_rank"]["graph_states"])

    def test_nesting_beyond_freshness_limit_fails_closed(self):
        query = {"id": "related", "kind": "related", "args": ["nav", "related", "BuildIntentPlan"], "freshness_rank_required": True}
        nested = {"graph_freshness": {"state": "current"}}
        for _ in range(9):
            nested = {"nested": nested}
        payload = {"ok": True, "items": [{"graph_freshness": {"state": "current"}, "graph_ranks": []}], "metadata": nested}
        result = measured_query_check(query, payload)
        self.assertEqual(result["freshness_rank"]["status"], "FAIL")
        self.assertIn("__freshness_traversal_truncated__", result["freshness_rank"]["graph_states"])

    def test_preview_requires_each_section_to_be_available_or_explained(self):
        result = measured_query_check(self.manifest()["queries"][0], self.preview_payload())
        self.assertEqual(result["status"], "PASS")
        self.assertEqual(result["preview"]["sections"], list(SECTIONS))
        self.assertEqual(result["preview"]["omissions"]["callers"], "no callers evidence was available")
        self.assertEqual(result["preview"]["expansions"]["valid"], 1)

    def test_preview_rejects_split_command_and_reason_fields(self):
        payload = self.preview_payload()
        payload["items"][0]["expansions"] = [
            {"command": "mi-lsp nav affected --full"},
            {"reason": "split fields must not be accepted"},
        ]
        result = measured_query_check(self.manifest()["queries"][0], payload)
        self.assertEqual(result["status"], "FAIL")
        self.assertEqual(result["preview"]["expansions"]["complete"], False)
        self.assertEqual(len(result["preview"]["expansions"]["invalid"]), 2)

    def test_preview_rejects_non_nav_expansion_command(self):
        payload = self.preview_payload()
        payload["items"][0]["expansions"] = [{"command": "next", "reason": "wrong command family"}]
        result = measured_query_check(self.manifest()["queries"][0], payload)
        self.assertEqual(result["status"], "FAIL")

    def test_sample_measurements_are_fail_closed(self):
        query = self.manifest()["queries"][0]
        for output_bytes, estimated_tokens in ((None, None), (None, 1), (1, None), (0, 1), (1, 0), (-1, 1), (1, -1)):
            with self.subTest(output_bytes=output_bytes, estimated_tokens=estimated_tokens):
                result = query_check(query, self.preview_payload(), output_bytes=output_bytes, estimated_tokens=estimated_tokens)
                self.assertEqual(result["status"], "FAIL")
                self.assertEqual(result["sample_measurements"]["status"], "FAIL")

        passed = query_check(query, self.preview_payload(), output_bytes=1, estimated_tokens=1)
        self.assertEqual(passed["status"], "PASS")
        self.assertEqual(passed["sample_measurements"]["status"], "PASS")

    def test_kind_schema_checks_are_not_ok_only(self):
        wiki = {"ok": True, "items": [{"docs": []}]}
        wiki_query = {"id": "wiki", "kind": "wiki_pack", "args": ["nav", "wiki", "pack", "task"]}
        self.assertEqual(measured_query_check(wiki_query, wiki)["status"], "FAIL")

        related = {"ok": True, "items": [{"symbol": "BuildIntentPlan", "graph_freshness": {"state": "current"}}]}
        related_query = {"id": "related", "kind": "related", "args": ["nav", "related", "BuildIntentPlan"], "freshness_rank_required": True}
        self.assertEqual(measured_query_check(related_query, related)["status"], "FAIL")

    def test_real_freshness_and_rank_shape_passes_for_related(self):
        payload = {
            "ok": True,
            "items": [{"symbol": "BuildIntentPlan", "graph_freshness": {"state": "current"}, "graph_ranks": []}],
        }
        query = {"id": "related", "kind": "related", "args": ["nav", "related", "BuildIntentPlan"], "freshness_rank_required": True}
        result = measured_query_check(query, payload)
        self.assertEqual(result["status"], "PASS")
        self.assertEqual(result["freshness_rank"]["status"], "PASS")

    def test_graph_shape_and_freshness_are_separate_fail_closed_gates(self):
        for kind, args in (("related", ["nav", "related", "BuildIntentPlan"]), ("workspace_map", ["nav", "workspace-map"])):
            query = {"id": kind, "kind": kind, "args": args, "freshness_rank_required": True}
            unknown = {"ok": True, "items": [{"graph_freshness": {"state": "unknown"}, "graph_ranks": []}]}
            unknown_result = measured_query_check(query, unknown)
            self.assertEqual(unknown_result["kind_schema"]["status"], "PASS")
            self.assertEqual(unknown_result["freshness_rank"]["status"], "FAIL")
            self.assertEqual(unknown_result["status"], "FAIL")

            current = {"ok": True, "items": [{"graph_freshness": {"state": "current"}, "graph_ranks": []}]}
            current_result = measured_query_check(query, current)
            self.assertEqual(current_result["status"], "PASS")
            self.assertEqual(current_result["kind_schema"]["status"], "PASS")
            self.assertEqual(current_result["freshness_rank"]["status"], "PASS")

            missing = {"ok": True, "items": [{}]}
            missing_result = measured_query_check(query, missing)
            self.assertEqual(missing_result["kind_schema"]["status"], "FAIL")
            self.assertEqual(missing_result["status"], "FAIL")

    def test_worker_status_requires_real_usable_evidence(self):
        usable = {"ok": True, "backend": "worker", "items": [{"selected_compatible": True, "selected_source": "bundle", "selected_path": "worker"}]}
        self.assertEqual(worker_status_check(usable)["status"], "PASS")
        terminal = {"ok": True, "backend": "worker", "items": [{"selected_compatible": False, "selected_error": "protocol mismatch"}]}
        self.assertEqual(worker_status_check(terminal)["status"], "FAIL")
        invented = {"ok": True, "backend": "worker", "items": [{"terminal_state": "ready"}]}
        self.assertEqual(worker_status_check(invented)["status"], "FAIL")
        bad = {"ok": True, "backend": "catalog", "items": []}
        self.assertEqual(worker_status_check(bad)["status"], "FAIL")

    def test_marker_is_exclusive(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / MARKER_NAME
            value = {"schema": "harness-first-run-marker/v1", "source_revision": "a" * 40}
            write_marker(path, value)
            with self.assertRaises(HarnessError) as raised:
                write_marker(path, value)
            self.assertEqual(raised.exception.reason_code, "marker_exists")

    def test_global_marker_uses_candidate_tuple_not_output_path(self):
        with tempfile.TemporaryDirectory() as directory:
            source = Path(directory)
            candidate = {"campaign_id": "campaign", "source_revision": "a" * 40, "binary_sha256": "b" * 64}
            claim_global_marker(source, candidate)
            with self.assertRaises(HarnessError) as raised:
                claim_global_marker(source, candidate)
            self.assertEqual(raised.exception.reason_code, "marker_exists")

    def test_global_marker_claim_is_atomic_under_threads(self):
        with tempfile.TemporaryDirectory() as directory:
            source = Path(directory)
            candidate = {"campaign_id": "campaign", "source_revision": "a" * 40, "binary_sha256": "b" * 64}

            def attempt(_index):
                try:
                    claim_global_marker(source, candidate)
                    return "claimed"
                except HarnessError as error:
                    return error.reason_code

            with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:
                results = list(pool.map(attempt, range(32)))
            self.assertEqual(results.count("claimed"), 1)
            self.assertEqual(results.count("marker_exists"), 31)

    def test_go_version_metadata_parser_is_fail_closed(self):
        text = "path example\nbuild\tvcs.revision=" + "a" * 40 + "\nbuild\tvcs.modified=false\n"
        parsed = parse_go_version_m(text)
        self.assertEqual(parsed["revision"], "a" * 40)
        self.assertEqual(parsed["modified"], False)
        self.assertTrue(parsed["parsed"])
        self.assertFalse(parse_go_version_m("build vcs.revision " + "a" * 40)["parsed"])

    def test_rss_gate_includes_worker_and_fails_closed(self):
        samples = [{"peak_rss_bytes": 10}]
        worker = {"peak_rss_bytes": None}
        report = _rss_report(samples, worker, 100)
        self.assertEqual(report["status"], "NOT_RUN")
        self.assertTrue(report["includes_worker_status"])

    def test_rss_sampler_invalidates_children_and_rss_errors(self):
        class AccessDenied(Exception):
            pass

        class NoSuchProcess(Exception):
            pass

        class FakePsutil:
            pass

        FakePsutil.NoSuchProcess = NoSuchProcess
        FakePsutil.AccessDenied = AccessDenied

        class ProcessHandle:
            def __init__(self, *, children_error=None, rss_error=None, child=None):
                self.children_error = children_error
                self.rss_error = rss_error
                self.child = child

            def children(self, recursive=True):
                if self.children_error is not None:
                    raise self.children_error
                return [self.child] if self.child is not None else []

            def memory_info(self):
                if self.rss_error is not None:
                    raise self.rss_error
                return SimpleNamespace(rss=10)

        for root in (
            ProcessHandle(children_error=AccessDenied()),
            ProcessHandle(child=ProcessHandle(rss_error=AccessDenied())),
        ):
            with self.subTest(error="observation"):
                sampler = _RSSSampler(SimpleNamespace(pid=123))
                sampler.psutil = FakePsutil
                sampler.psutil.Process = lambda _pid, root=root: root
                sampler._sample()
                self.assertTrue(sampler.failure)
                self.assertIsNone(sampler.stop())

        root = ProcessHandle(child=ProcessHandle(rss_error=NoSuchProcess()))
        sampler = _RSSSampler(SimpleNamespace(pid=123))
        sampler.psutil = FakePsutil
        original_children = root.children

        def one_observation(*, recursive=True):
            result = original_children(recursive=recursive)
            sampler.stop_event.set()
            return result

        root.children = one_observation
        sampler.psutil.Process = lambda _pid: root
        sampler._sample()
        self.assertFalse(sampler.failure)
        self.assertEqual(sampler.stop(), 10)

    def test_release_post_gate_reads_remote_assets_in_clean_dirs(self):
        workflow = (Path(__file__).parents[4] / ".github" / "workflows" / "release.yml").read_text(encoding="utf-8")
        post_gate = workflow.split("Re-verify published-build metadata and worker bundles from release assets", 1)[1]
        self.assertIn('gh release download "$GITHUB_REF_NAME"', post_gate)
        self.assertIn('published_dir="$(mktemp -d)"', post_gate)
        self.assertIn('extracted_dir="$(mktemp -d)"', post_gate)
        self.assertIn("sha256sum -c", post_gate)
        self.assertIn("go version -m", post_gate)
        self.assertIn("worker_root", post_gate)
        self.assertIn("declare -A expected_set=()", post_gate)
        self.assertIn("declare -A seen_names=()", post_gate)
        self.assertIn('test -z "${seen_names[$checksum_name]+present}"', post_gate)
        self.assertIn('test -n "${expected_set[$checksum_name]+present}"', post_gate)
        self.assertIn('test "$checksum_count" -eq 6', post_gate)
        self.assertIn('for expected_name in "${expected_names[@]}"', post_gate)
        self.assertIn('test "$checksum_name" = "$(basename -- "$checksum_name")"', post_gate)
        self.assertIn('[[ "$checksum_name" != /* && "$checksum_name" != */* && "$checksum_name" != *..* ]]', post_gate)
        self.assertNotIn("find dist", post_gate)

    def test_checksum_file_is_regular_and_inside_download_directory_before_read(self):
        workflow = (Path(__file__).parents[4] / ".github" / "workflows" / "release.yml").read_text(encoding="utf-8")
        post_gate = workflow.split("Re-verify published-build metadata and worker bundles from release assets", 1)[1]
        selected = post_gate.index('checksum_file="${checksum_files[0]}"')
        regular = post_gate.index('test -f "$checksum_file"')
        nonsymlink = post_gate.index('test ! -L "$checksum_file"')
        resolved = post_gate.index('checksum_real="$(realpath -e -- "$checksum_file")"')
        read_loop = post_gate.index('while IFS= read -r checksum_line; do')
        self.assertLess(selected, regular)
        self.assertLess(regular, nonsymlink)
        self.assertLess(nonsymlink, resolved)
        self.assertLess(resolved, read_loop)
        self.assertIn('[[ "$checksum_real" == "$published_real/"* ]]', post_gate)

    def test_release_checksum_gate_rejects_duplicate_or_missing_names(self):
        workflow = (Path(__file__).parents[4] / ".github" / "workflows" / "release.yml").read_text(encoding="utf-8")
        post_gate = workflow.split("Re-verify published-build metadata and worker bundles from release assets", 1)[1]
        self.assertIn('test -z "${seen_names[$checksum_name]+present}"', post_gate)
        self.assertIn('test -n "${expected_set[$checksum_name]+present}"', post_gate)
        self.assertIn('test "$checksum_count" -eq 6', post_gate)
        self.assertIn('for expected_name in "${expected_names[@]}"', post_gate)

    def test_release_post_gate_accepts_equals_and_whitespace_metadata(self):
        workflow = (Path(__file__).parents[4] / ".github" / "workflows" / "release.yml").read_text(encoding="utf-8")
        self.assertIn(r'vcs\.revision(=|[[:space:]]+)', workflow)
        self.assertIn(r'vcs\.modified(=|[[:space:]]+)', workflow)

    def test_yaml_sanitizes_native_output_and_payload(self):
        rendered = to_yaml({"status": "PASS", "stdout": "must not be persisted", "payload": {"secret": "no"}})
        self.assertIn('status: "PASS"', rendered)
        self.assertNotIn("stdout", rendered)
        self.assertNotIn("payload", rendered)


if __name__ == "__main__":
    unittest.main()
