import unittest
from pathlib import Path

from canonical_v2 import canonical_json, canonical_payload, parse_json_output, validate_terminal_state


class CanonicalV2Tests(unittest.TestCase):
    def test_volatile_paths_and_secrets_are_not_authoritative(self):
        root = Path("C:/tmp/victory-fixture")
        value = {"items": [{"display": str(root / "go" / "x.go")}], "elapsed_ms": 99, "api_token": "secret"}
        text = canonical_json(value, root)
        self.assertNotIn("secret", text)
        self.assertNotIn("elapsed_ms", text)
        self.assertIn("go/x.go", text)

    def test_payload_digest_is_stable_and_raw_fields_are_removed(self):
        left = canonical_payload("callers", {"items": [{"display": "A"}], "stdout": "native"})
        right = canonical_payload("callers", {"items": [{"display": "A"}], "stderr": "different"})
        self.assertEqual(left["digest"], right["digest"])
        self.assertEqual(left["schema"], "victory-canonical/v2")

    def test_workspace_fixture_identity_accepts_case_insensitive_basename_and_full_path(self):
        root = Path("C:/tmp/victory-fixture")
        for workspace in (root.name.upper(), r"c:\\TMP\\VICTORY-FIXTURE"):
            payload = canonical_payload("callers", {"workspace": workspace, "items": []}, root)
            self.assertEqual(payload["payload"]["workspace"], "<FIXTURE_ROOT>")

    def test_fixture_paths_are_redacted_after_relativization_but_safe_paths_survive(self):
        fixture = Path(__file__).resolve().parents[3] / "benchmarks/victory-lab/v2"
        payload = canonical_payload(
            "affected",
            {
                "items": [
                    {"path": "subject/subject.go"},
                    {"path": str(fixture / "corpus/go/subject/subject.go")},
                    {"path": str(fixture / "corpus/go/patient/record.json")},
                    {"path": "alice@example.com"},
                    {"path": "SSN 123-45-6789"},
                ]
            },
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
            payload = canonical_payload("callers", {"workspace": workspace, "items": []}, root)
            self.assertEqual(payload["payload"]["workspace"], "<REDACTED>")

    def test_semantic_workspace_name_is_preserved(self):
        root = Path("C:/tmp/victory-fixture")
        payload = canonical_payload("callers", {"workspace": "domain-workspace", "items": []}, root)
        self.assertEqual(payload["payload"]["workspace"], "domain-workspace")

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
