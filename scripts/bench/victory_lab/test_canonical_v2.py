import unittest
from pathlib import Path

from canonical_v2 import canonical_json, canonical_payload, parse_json_output


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

    def test_json_parser_rejects_empty(self):
        with self.assertRaises(ValueError):
            parse_json_output("  ")


if __name__ == "__main__":
    unittest.main()
