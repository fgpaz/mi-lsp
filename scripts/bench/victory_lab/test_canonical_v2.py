import unittest
from pathlib import Path

from canonical_v2 import common_result_projection, common_token_units, canonical_json, canonical_payload, parse_json_output, payload_digest, validate_terminal_state


class CanonicalV2Tests(unittest.TestCase):
    @staticmethod
    def _terminal(**extra):
        return {
            "ok": True,
            "done": True,
            "backend": "go",
            "completeness": "complete",
            "truncated": False,
            **extra,
        }

    def test_volatile_paths_and_secrets_are_not_authoritative(self):
        root = Path("C:/tmp/victory-fixture")
        value = self._terminal(items=[{"display": str(root / "go" / "x.go")}], elapsed_ms=99, api_token="secret")
        text = canonical_json(value, root)
        self.assertNotIn("secret", text)
        self.assertNotIn("elapsed_ms", text)
        self.assertIn("go/x.go", text)

    def test_payload_digest_is_stable_and_raw_fields_are_removed(self):
        left = canonical_payload("callers", self._terminal(items=[{"display": "A"}], stdout="native"))
        right = canonical_payload("callers", self._terminal(items=[{"display": "A"}], stderr="different"))
        self.assertEqual(left["digest"], right["digest"])
        self.assertEqual(left["schema"], "victory-canonical/v2")

    def test_workspace_fixture_identity_accepts_case_insensitive_basename_and_full_path(self):
        root = Path("C:/tmp/victory-fixture")
        for workspace in (root.name.upper(), r"c:\\TMP\\VICTORY-FIXTURE"):
            payload = canonical_payload("callers", self._terminal(workspace=workspace, items=[]), root)
            self.assertEqual(payload["payload"]["workspace"], "<FIXTURE_ROOT>")

    def test_fixture_paths_are_redacted_after_relativization_but_safe_paths_survive(self):
        fixture = Path(__file__).resolve().parents[3] / "benchmarks/victory-lab/v2"
        payload = canonical_payload(
            "affected",
            self._terminal(
                items=[
                    {"path": "subject/subject.go"},
                    {"path": str(fixture / "corpus/go/subject/subject.go")},
                    {"path": str(fixture / "corpus/go/patient/record.json")},
                    {"path": "alice@example.com"},
                    {"path": "SSN 123-45-6789"},
                ],
            ),
            fixture,
        )
        self.assertEqual(
            payload["payload"]["items"],
            [
                {"path": "subject/subject.go"},
                {"path": "corpus/go/subject/subject.go"},
                {"path": "<REDACTED>"},
                {"path": "<REDACTED>"},
                {"path": "<REDACTED>"},
            ],
        )

    def test_unrecognized_workspace_pii_and_path_are_redacted(self):
        root = Path("C:/tmp/victory-fixture")
        for workspace in (
            "alice@example.com",
            r"C:\\other\\patient\\record.json",
            str(root / "private" / "record.json"),
        ):
            payload = canonical_payload("callers", self._terminal(workspace=workspace, items=[]), root)
            self.assertEqual(payload["payload"]["workspace"], "<REDACTED>")

    def test_semantic_workspace_name_is_preserved(self):
        root = Path("C:/tmp/victory-fixture")
        payload = canonical_payload("callers", self._terminal(workspace="domain-workspace", items=[]), root)
        self.assertEqual(payload["payload"]["workspace"], "domain-workspace")

    def test_common_projection_equalizes_rich_current_and_minimal_graphify(self):
        current = self._terminal(
            backend="go",
            workspace="<FIXTURE_ROOT>",
            request_id="req-123",
            stats={"scanned": 99, "elapsed_ms": 12},
            evidence={"source": "current", "trace_id": "trace-current"},
            items=[{"display": "callers.Direct", "id": "symbol-1", "path": "callers/callers.go"}],
        )
        graphify = {
            "ok": True,
            "done": True,
            "backend": "graphify",
            "completeness": "exact",
            "truncated": False,
            "items": [{"display": "callers.Direct"}],
        }
        current_canonical = canonical_payload("callers", current)
        graphify_canonical = canonical_payload("callers", graphify)
        self.assertEqual(current_canonical["token_units"], graphify_canonical["token_units"])
        self.assertEqual(current_canonical["payload"]["evidence"], current["evidence"])
        self.assertEqual(current_canonical["payload"]["request_id"], "req-123")
        self.assertEqual(current_canonical["digest"], payload_digest(current_canonical["payload"]))
        self.assertNotEqual(current_canonical["digest"], graphify_canonical["digest"])

    def test_common_projection_contains_only_shared_semantics(self):
        payload = self._terminal(
            workspace="workspace",
            backend="go",
            ids={"symbol": "s1"},
            stats={"count": 1},
            items=[{"display": "b"}, {"display": "a"}],
        )
        projection = common_result_projection("callers", payload)
        self.assertEqual(
            projection,
            {
                "ok": True,
                "done": True,
                "completeness": "complete",
                "truncated": False,
                "operation": "callers",
                "items": [{"display": "a"}, {"display": "b"}],
            },
        )
        self.assertNotIn("backend", projection)
        self.assertNotIn("workspace", projection)
        self.assertNotIn("ids", projection)
        self.assertNotIn("stats", projection)

    def test_display_and_normalized_path_changes_change_token_units(self):
        callers_a = self._terminal(items=[{"display": "A"}])
        callers_b = self._terminal(items=[{"display": "Longer.Display"}])
        affected_a = self._terminal(items=[{"path": "src\\a.go"}])
        affected_b = self._terminal(items=[{"path": "src\\nested\\very\\long\\name.go"}])
        self.assertNotEqual(common_token_units("callers", callers_a), common_token_units("callers", callers_b))
        self.assertNotEqual(common_token_units("affected", affected_a), common_token_units("affected", affected_b))
        self.assertEqual(
            common_result_projection("affected", self._terminal(items=[{"path": "src/./a.go"}]))["items"],
            [{"path": "src/a.go"}],
        )

    def test_incomplete_terminal_cannot_be_projected_as_pass(self):
        with self.assertRaisesRegex(ValueError, "done=true"):
            common_result_projection("callers", self._terminal(done=False, items=[]))
        with self.assertRaisesRegex(ValueError, "completeness"):
            common_result_projection("callers", self._terminal(completeness="partial", items=[]))

    def test_catalog_backend_is_allowed_but_arbitrary_backend_is_not(self):
        native = {"ok": True, "done": True, "backend": "catalog", "completeness": "complete", "truncated": False, "items": []}
        self.assertIs(validate_terminal_state(native), native)
        with self.assertRaisesRegex(ValueError, "invalid backend"):
            validate_terminal_state(dict(native, backend="catalogue"))

    def test_json_parser_rejects_empty(self):
        with self.assertRaises(ValueError):
            parse_json_output("  ")


if __name__ == "__main__":
    unittest.main()
