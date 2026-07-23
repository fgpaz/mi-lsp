import concurrent.futures
from collections.abc import Mapping
import json
import tempfile
import unittest
from pathlib import Path
from types import SimpleNamespace
from unittest.mock import patch

from scripts.bench.harness_first.runner import (
    HarnessError,
    MARKER_NAME,
    ProcessResult,
    SECTIONS,
    _RSSSampler,
    _bounded_diff_value,
    _bounded_structural_diff,
    _candidate_preflight,
    _command,
    _daemon_identity_check,
    _daemon_preflight,
    _find_preview_contract,
    _freshness_records,
    _rss_report,
    _select_latest_telemetry_event,
    claim_global_marker,
    digest,
    finite,
    normalize,
    parse_go_version_m,
    query_check,
    run_campaign,
    sanitize,
    to_yaml,
    validate_manifest,
    worker_status_check,
    write_marker,
)


def measured_query_check(query, payload):
    return query_check(query, payload, output_bytes=1, estimated_tokens=1)


class CountingMapping(Mapping):
    def __init__(self, values):
        self.values = values
        self.iterated = 0
        self.lookups = 0

    def __iter__(self):
        for key in self.values:
            self.iterated += 1
            yield key

    def __getitem__(self, key):
        self.lookups += 1
        return self.values[key]

    def __len__(self):
        return len(self.values)


class CountingList(list):
    def __init__(self, values):
        super().__init__(values)
        self.indexed = 0
        self.iterated = 0

    def __iter__(self):
        for item in super().__iter__():
            self.iterated += 1
            yield item

    def __getitem__(self, index):
        self.indexed += 1
        return super().__getitem__(index)


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

    def test_manifest_worker_status_args_are_exactly_fixed(self):
        manifest = self.manifest()
        manifest["worker_status_args"] = ["worker", "install"]
        with self.assertRaises(HarnessError):
            validate_manifest(manifest)
        manifest["worker_status_args"] = ["admin", "export"]
        with self.assertRaises(HarnessError):
            validate_manifest(manifest)
        self.assertEqual(validate_manifest(self.manifest())["worker_status_args"], ["worker", "status"])

    def test_command_routes_direct_and_daemon_modes_explicitly(self):
        query = {"args": ["nav", "related", "BuildIntentPlan"]}
        direct = _command(Path("mi-lsp"), Path("workspace"), "campaign", query, "direct")
        daemon = _command(Path("mi-lsp"), Path("workspace"), "campaign", query, "daemon")
        self.assertIn("--no-daemon", direct)
        self.assertNotIn("--no-auto-daemon", direct)
        self.assertNotIn("--no-daemon", daemon)
        self.assertEqual(direct[-3:], ["nav", "related", "BuildIntentPlan"])
        self.assertEqual(daemon[-3:], ["nav", "related", "BuildIntentPlan"])

    def test_daemon_identity_check_requires_exact_candidate_hash_and_compatible_metadata(self):
        candidate_sha = "a" * 64
        payload = {"ok": True, "items": [{"state": {"executable_sha256": candidate_sha, "protocol_version": "mi-lsp-v1.1", "version": "(devel)"}}]}
        result = _daemon_identity_check(payload, candidate_sha)
        self.assertEqual(result["status"], "PASS")
        self.assertTrue(result["daemon_identity_match"])
        self.assertTrue(result["protocol_compatible"])
        self.assertTrue(result["version_compatible"])

        mismatch = _daemon_identity_check(payload, "b" * 64)
        self.assertEqual(mismatch["status"], "FAIL")
        self.assertFalse(mismatch["daemon_identity_match"])
        self.assertEqual(mismatch["reason_code"], "daemon_identity_mismatch")

    def test_daemon_version_spoof_fails_closed_even_when_hash_and_protocol_match(self):
        candidate_sha = "a" * 64
        candidate = {"version": "(devel)", "protocol_version": "mi-lsp-v1.1", "executable_sha256": candidate_sha}
        payload = {"ok": True, "items": [{"state": {"executable_sha256": candidate_sha, "protocol_version": "mi-lsp-v1.1", "version": "unrelated-build"}}]}
        result = _daemon_identity_check(payload, candidate_sha, candidate)
        self.assertEqual(result["status"], "FAIL")
        self.assertFalse(result["version_match"])
        self.assertEqual(result["reason_code"], "candidate_version_mismatch")

    def test_daemon_preflight_fails_closed_on_identity_mismatch_without_retry(self):
        candidate_sha = "a" * 64
        calls = []
        responses = iter([
            ProcessResult(0, 1.0, payload={"ok": True}),
            ProcessResult(0, 2.0, payload={"ok": True}),
            ProcessResult(0, 3.0, payload={"ok": True, "items": [{"state": {"executable_sha256": "b" * 64, "protocol_version": "mi-lsp-v1.1", "version": "(devel)"}}]}),
        ])

        def fake_run_process(argv, timeout):
            calls.append(list(argv))
            return next(responses)

        with patch("scripts.bench.harness_first.runner.run_process", side_effect=fake_run_process):
            result = _daemon_preflight(Path("candidate"), candidate_sha, 10.0)

        self.assertEqual(result["status"], "BLOCKED")
        self.assertEqual(result["reason_code"], "daemon_identity_mismatch")
        self.assertFalse(result["daemon_identity_match"])
        self.assertEqual(len(calls), 3)
        self.assertTrue(calls[0][-2:] == ["daemon", "stop"])
        self.assertTrue(calls[1][-2:] == ["daemon", "start"])
        self.assertTrue(calls[2][-2:] == ["daemon", "status"])

    def test_candidate_preflight_runs_worker_once_before_direct_sample_and_excludes_preflight_from_latency(self):
        manifest = {
            "schema": "harness-first-campaign/v1",
            "campaign_id": "preflight-test",
            "queries": [{
                "id": "wiki",
                "kind": "wiki_pack",
                "args": ["nav", "wiki", "pack", "task"],
                "modes": ["direct"],
            }],
        }
        worker_payload = {"ok": True, "backend": "worker", "items": [{"selected_compatible": True, "selected_source": "bundle", "selected_path": "worker"}]}
        query_payload = {"ok": True, "items": [{"docs": [{"path": "doc"}]}]}
        responses = iter([
            ProcessResult(0, 1.0, payload={"ok": True, "items": [{"version": "(devel)", "protocol_version": "mi-lsp-v1.1", "executable_sha256": "b" * 64}]}),
            ProcessResult(0, 3.0, payload=worker_payload),
            ProcessResult(0, 7.0, payload=query_payload),
            ProcessResult(0, 1.0, payload=[]),
        ])
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            binary = root / "candidate"
            binary.write_bytes(b"candidate")
            output = root / "output"
            calls = []

            def fake_run_process(argv, timeout):
                calls.append(list(argv))
                return next(responses)

            with (
                patch("scripts.bench.harness_first.runner.run_process", side_effect=fake_run_process),
                patch("scripts.bench.harness_first.runner.source_revision", return_value="a" * 40),
                patch("scripts.bench.harness_first.runner.provenance", return_value={"status": "PASS", "binary_sha256": "b" * 64}),
                patch("scripts.bench.harness_first.runner.claim_global_marker"),
            ):
                report = run_campaign(manifest, binary=binary, source_root=root, output=output)

        worker_calls = [call for call in calls if "worker" in call and "status" in call]
        self.assertEqual(len(worker_calls), 1)
        self.assertEqual(calls[0][-1], "version")
        self.assertIn("worker", calls[1])
        self.assertIn("--no-daemon", calls[1])
        self.assertIn("wiki", calls[2])
        self.assertEqual(report["samples"][0]["elapsed_ms"], 7.0)
        self.assertEqual(report["candidate_preflight"]["worker_elapsed_ms"], 3.0)
        self.assertEqual(report["candidate_preflight"]["daemon"]["status"], "NOT_REQUIRED")

    def test_blocked_candidate_preflight_does_not_burn_global_claim(self):
        manifest = self.manifest()
        responses = iter([
            ProcessResult(0, 1.0, payload={"ok": True, "items": [{"version": "(devel)", "protocol_version": "mi-lsp-v1.1", "executable_sha256": "b" * 64}]}),
            ProcessResult(0, 1.0, payload={"ok": True, "backend": "catalog", "items": []}),
        ])
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            binary = root / "candidate"
            binary.write_bytes(b"candidate")
            output = root / "output"
            with (
                patch("scripts.bench.harness_first.runner.run_process", side_effect=lambda _argv, _timeout: next(responses)),
                patch("scripts.bench.harness_first.runner.source_revision", return_value="a" * 40),
                patch("scripts.bench.harness_first.runner.provenance", return_value={"status": "PASS", "binary_sha256": "b" * 64}),
                patch("scripts.bench.harness_first.runner.claim_global_marker") as claim,
            ):
                report = run_campaign(manifest, binary=binary, source_root=root, output=output)
        self.assertEqual(report["status"], "BLOCKED")
        self.assertEqual(report["candidate_preflight"]["reason_code"], "worker_preflight_failed")
        claim.assert_not_called()

    def test_candidate_preflight_requires_daemon_setup_once_for_daemon_samples(self):
        candidate_sha = "a" * 64
        worker_payload = {"ok": True, "backend": "worker", "items": [{"selected_compatible": True, "selected_source": "bundle", "selected_path": "worker"}]}
        status_payload = {"ok": True, "items": [{"state": {"executable_sha256": candidate_sha, "protocol_version": "mi-lsp-v1.1", "version": "(devel)"}}]}
        responses = iter([
            ProcessResult(0, 1.0, payload={"ok": True, "items": [{"version": "(devel)", "protocol_version": "mi-lsp-v1.1", "executable_sha256": candidate_sha}]}),
            ProcessResult(0, 1.0, payload=worker_payload),
            ProcessResult(0, 2.0, payload={"ok": True}),
            ProcessResult(0, 3.0, payload={"ok": True}),
            ProcessResult(0, 4.0, payload=status_payload),
        ])
        calls = []

        def fake_run_process(argv, timeout):
            calls.append(list(argv))
            return next(responses)

        with patch("scripts.bench.harness_first.runner.run_process", side_effect=fake_run_process):
            result = _candidate_preflight(Path("candidate"), Path("source"), "campaign", ["worker", "status"], candidate_sha, 10.0, daemon_required=True)

        self.assertEqual(result["status"], "PASS")
        self.assertTrue(result["daemon_identity_match"])
        self.assertEqual(len(calls), 5)
        self.assertEqual(calls[0][-1], "version")
        self.assertEqual(result["worker_elapsed_ms"], 1.0)
        self.assertEqual(result["daemon"]["start"]["elapsed_ms"], 3.0)
        self.assertEqual(result["daemon"]["status_probe"]["elapsed_ms"], 4.0)
        self.assertGreaterEqual(result["daemon"]["elapsed_ms"], 0.0)

    def test_bounded_structural_diff_is_redacted_and_limited(self):
        left = {"items": [{f"field_{index}": f"C:/private/{index}.cs"} for index in range(5)], "token": "secret-value"}
        right = {"items": [{f"field_{index}": f"C:/other/{index}.cs"} for index in range(5)], "token": "other-secret"}
        result = _bounded_structural_diff(left, right, max_diffs=2)
        self.assertEqual(result["count"], 2)
        self.assertTrue(result["truncated"])
        self.assertNotIn("private", json.dumps(result))
        self.assertNotIn("secret-value", json.dumps(result))

    def test_projection_redacts_path_sensitive_phi_but_keeps_technical_symbols(self):
        value = {
            "C:/Users/Alice/notes.txt": "private path",
            "patient_name": "Alice Example",
            "diagnosis": "rare condition",
            "MRN": "MRN-123456",
            "email": "alice@example.com",
            "symbol": "BuildIntentPlan",
            "name": "BuildIntentPlan",
        }
        rendered = json.dumps(sanitize(value))
        self.assertNotIn("C:/Users/Alice/notes.txt", rendered)
        self.assertNotIn("Alice Example", rendered)
        self.assertNotIn("rare condition", rendered)
        self.assertNotIn("MRN-123456", rendered)
        self.assertNotIn("alice@example.com", rendered)
        self.assertIn("BuildIntentPlan", rendered)
        self.assertIn('"name": "BuildIntentPlan"', rendered)

    def test_neutral_keys_redact_known_token_formats_and_high_entropy_values(self):
        values = [
            "ghp_" + "A" * 36,
            "github_pat_" + "A" * 30,
            "glpat-" + "A" * 20,
            "xoxb-1234567890-1234567890-abcdefghijkl",
            "sk_live_" + "A" * 24,
            "sk_test_" + "A" * 24,
            "AKIA" + "A" * 16,
            "Bearer " + "A" * 32,
            "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c",
            "Aa1+/Bb2-_Cc3+/Dd4-_Ee5+/Ff6-_Gg7+/Hh8-_Ii9+/Jj0-",
        ]
        for value in values:
            with self.subTest(value=value[:12]):
                rendered = json.dumps(sanitize({"metadata": value}), sort_keys=True)
                self.assertNotIn(value, rendered)
                self.assertIn("[REDACTED:", rendered)
                self.assertEqual(sanitize(value), _bounded_diff_value(value))
                self.assertNotIn(value, to_yaml({"metadata": value}))

                diff = _bounded_structural_diff({"metadata": value}, {"metadata": "other-value"})
                self.assertNotIn(value, json.dumps(diff, sort_keys=True))

    def test_neutral_keys_keep_sha_revisions_digests_and_technical_symbols(self):
        sha256 = "a" * 64
        revision = "0123456789abcdef0123456789abcdef01234567"
        semantic_id = "BuildIntentPlanWithManySemanticSegmentsAndTypes20260722"
        value = {"sha256": sha256, "revision": revision, "semantic_id": semantic_id}
        rendered = json.dumps(sanitize(value), sort_keys=True)
        self.assertIn(sha256, rendered)
        self.assertIn(revision, rendered)
        self.assertIn(semantic_id, rendered)
        self.assertEqual(_bounded_diff_value(sha256), sha256)
        self.assertEqual(_bounded_diff_value(revision), revision)
        self.assertEqual(_bounded_diff_value(semantic_id), semantic_id)

    def test_credential_detection_is_bounded_by_projection_string_limit(self):
        oversized = "Aa1+/" * 200
        self.assertNotIn(oversized, json.dumps(sanitize({"metadata": oversized})))

    def test_projection_truncation_fails_query_closed(self):
        payload = {"ok": True, "items": [{"name": "BuildIntentPlan"}], "values": list(range(100000))}
        result = measured_query_check(self.manifest()["queries"][0], payload)
        self.assertEqual(result["status"], "FAIL")
        self.assertEqual(result["semantic_projection"]["reason_code"], "projection_truncated")
        self.assertIsNone(result["normalized_digest"])

    def test_discovery_and_freshness_budget_stop_custom_mapping_before_contract(self):
        values = {f"junk_{index}": {"nested": index} for index in range(100000)}
        payload = CountingMapping(values)
        self.assertIsNone(_find_preview_contract(payload))
        self.assertLess(payload.iterated, 256)
        payload = CountingMapping(values)
        _ranks, graph, _rank_lists = _freshness_records(payload)
        self.assertIn("__freshness_traversal_truncated__", [item.get("state") for item in graph])
        self.assertLess(payload.iterated, 256)

    def test_discovery_budget_stops_custom_list_before_final_contract(self):
        payload = CountingList([{"junk": index} for index in range(100000)] + [{"preview": []}])
        self.assertIsNone(_find_preview_contract(payload))
        self.assertLess(payload.iterated, 256)

    def test_bounded_diff_stops_before_final_difference_in_huge_list(self):
        left = list(range(100000))
        right = list(range(100000))
        right[-1] = -1
        result = _bounded_structural_diff(left, right)
        self.assertTrue(result["truncated"])
        self.assertNotEqual(result["status"], "PASS")
        self.assertLessEqual(result["count"], 32)

    def test_giant_mapping_key_fails_closed_without_raw_key_or_digest(self):
        giant_key = "k" * 100000
        payload = {"ok": True, "items": [{"name": "BuildIntentPlan"}], giant_key: "value"}
        result = measured_query_check(self.manifest()["queries"][0], payload)
        rendered = json.dumps(result, sort_keys=True)
        self.assertEqual(result["status"], "FAIL")
        self.assertEqual(result["semantic_projection"]["reason_code"], "projection_truncated")
        self.assertIsNone(result["normalized_digest"])
        self.assertNotIn(giant_key, rendered)

    def test_finite_rejects_bool_and_arbitrary_precision_integer_without_crash(self):
        self.assertFalse(finite(True))
        self.assertTrue(finite(1))
        self.assertTrue(finite(1.0))
        self.assertFalse(finite(10**1000))
        result = measured_query_check(self.manifest()["queries"][0], {"ok": True, "items": [{"value": 10**1000}]})
        self.assertEqual(result["status"], "FAIL")
        self.assertEqual(result["semantic_projection"]["reason_code"], "projection_truncated")

    def test_diff_redacts_relative_paths_and_clinical_text_after_difference(self):
        left = {"src/private/patient_notes.txt": "Patient Alice has rare condition"}
        right = {"src\\private\\patient_notes.txt": "Patient Bob has rare condition"}
        result = _bounded_structural_diff(left, right)
        rendered = json.dumps(result, sort_keys=True)
        self.assertEqual(result["status"], "DIFF")
        self.assertNotIn("src/private/patient_notes.txt", rendered)
        self.assertNotIn("Patient Alice has rare condition", rendered)
        self.assertNotIn("Patient Bob has rare condition", rendered)
        self.assertNotIn("private", rendered)

    def test_stats_ms_does_not_change_normalized_digest(self):
        base = {"stats": {"ms": 10, "symbols": 4, "files": 2}}
        changed = {"stats": {"ms": 900, "symbols": 4, "files": 2}}
        self.assertEqual(digest(normalize(base)), digest(normalize(changed)))

    def test_normalized_stats_preserve_semantic_fields(self):
        normalized = normalize({"stats": {"ms": 10, "symbols": 4, "files": 2, "references": 7}})
        self.assertNotIn("ms", normalized["stats"])
        self.assertEqual(normalized["stats"]["symbols"], 4)
        self.assertEqual(normalized["stats"]["files"], 2)
        self.assertEqual(normalized["stats"]["references"], 7)

    def test_query_check_direct_and_daemon_ignore_route_and_stats_ms_for_digest(self):
        query = {"id": "search", "kind": "search", "args": ["nav", "search", "BuildIntentPlan"]}
        direct = {
            "ok": True,
            "route": "direct",
            "backend": "catalog",
            "stats": {"ms": 10, "symbols": 4, "files": 2},
            "items": [{"name": "BuildIntentPlan"}],
        }
        daemon = {
            "ok": True,
            "route": "daemon",
            "backend": "roslyn",
            "stats": {"ms": 900, "symbols": 4, "files": 2},
            "items": [{"name": "BuildIntentPlan"}],
        }
        direct_result = query_check(query, direct, output_bytes=1, estimated_tokens=1)
        daemon_result = query_check(query, daemon, output_bytes=1, estimated_tokens=1)
        self.assertEqual(direct_result["status"], "PASS")
        self.assertEqual(daemon_result["status"], "PASS")
        self.assertEqual(direct_result["normalized_digest"], daemon_result["normalized_digest"])

    def test_telemetry_selection_prefers_highest_id_in_newest_first_export(self):
        events = [
            {"id": 42, "route": "daemon", "backend": "roslyn", "operation": "nav.related", "client_name": "harness-first", "session_id": "campaign"},
            {"id": 41, "route": "direct", "backend": "catalog", "operation": "nav.related", "client_name": "harness-first", "session_id": "campaign"},
        ]
        selected = _select_latest_telemetry_event(
            events,
            campaign_id="campaign",
            client_name="harness-first",
            operation="nav.related",
        )
        self.assertIsNotNone(selected)
        self.assertEqual(selected["route"], "daemon")

    def test_telemetry_selection_prefers_highest_id_when_export_order_is_inverted(self):
        events = [
            {"id": 41, "route": "direct", "backend": "catalog", "operation": "nav.related", "client_name": "harness-first", "session_id": "campaign"},
            {"id": 42, "route": "daemon", "backend": "roslyn", "operation": "nav.related", "client_name": "harness-first", "session_id": "campaign"},
        ]
        selected = _select_latest_telemetry_event(
            events,
            campaign_id="campaign",
            client_name="harness-first",
            operation="nav.related",
        )
        self.assertIsNotNone(selected)
        self.assertEqual(selected["route"], "daemon")

    def test_telemetry_selection_falls_back_for_missing_or_malformed_ids(self):
        timestamped = [
            {"id": "not-a-number", "occurred_at": "2026-07-22T12:01:00Z", "route": "daemon", "operation": "nav.related", "client_name": "harness-first", "session_id": "campaign"},
            {"occurred_at": "2026-07-22T12:00:00Z", "route": "direct", "operation": "nav.related", "client_name": "harness-first", "session_id": "campaign"},
        ]
        selected = _select_latest_telemetry_event(
            timestamped,
            campaign_id="campaign",
            client_name="harness-first",
            operation="nav.related",
        )
        self.assertIsNotNone(selected)
        self.assertEqual(selected["route"], "daemon")

        export_order_fallback = [
            {"route": "daemon", "operation": "nav.related", "client_name": "harness-first", "session_id": "campaign"},
            {"id": "malformed", "route": "direct", "operation": "nav.related", "client_name": "harness-first", "session_id": "campaign"},
        ]
        selected = _select_latest_telemetry_event(
            export_order_fallback,
            campaign_id="campaign",
            client_name="harness-first",
            operation="nav.related",
        )
        self.assertIsNotNone(selected)
        self.assertEqual(selected["route"], "daemon")

    def test_telemetry_selection_filters_campaign_client_and_operation_before_ordering(self):
        events = [
            {"id": 99, "route": "daemon", "operation": "nav.related", "client_name": "other-client", "session_id": "campaign"},
            {"id": 98, "route": "daemon", "operation": "nav.search", "client_name": "harness-first", "session_id": "campaign"},
            {"id": 97, "route": "daemon", "operation": "nav.related", "client_name": "harness-first", "session_id": "other-campaign"},
            {"id": 42, "route": "daemon", "backend": "roslyn", "operation": "nav.related", "client_name": "harness-first", "session_id": "campaign"},
            {"id": 41, "route": "direct", "backend": "catalog", "operation": "nav.related", "client_name": "harness-first", "session_id": "campaign"},
        ]
        selected = _select_latest_telemetry_event(
            events,
            campaign_id="campaign",
            client_name="harness-first",
            operation="nav.related",
        )
        self.assertIsNotNone(selected)
        self.assertEqual(selected["route"], "daemon")
        self.assertEqual(selected["id"], 42)

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
