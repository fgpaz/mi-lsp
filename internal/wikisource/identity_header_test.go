package wikisource

import "testing"

func TestDeclaredDocIDRequiresBoundedOwnerMetadata(t *testing.T) {
	cases := []struct{ name, content, want string }{
		{"frontmatter", "---\nid: OLD\ndoc_id: CT-OWNER\n---\n", "CT-OWNER"},
		{"unterminated_frontmatter", "---\ntitle: Example\n\n# Body\nid: CT-OTHER\n", ""},
		{"leading_example", "# README\n\n```yaml\nid: CT-EXAMPLE\n```\n", ""},
		{"legacy_harness", "# Baseline\n\n```yaml\nharness_protocol: SDD-HARNESS-v1\nid: baseline\n```\n", "baseline"},
		{"source_contract", "```yaml\nsource_protocol: SDD-WIKI-SOURCE-v1\ndoc_id: CT-OWNER\n```\n", "CT-OWNER"},
		{"unterminated_contract", "```yaml\nharness_protocol: SDD-HARNESS-v1\nid: CT-EXAMPLE\n", ""},
		{"nested_protocol", "```yaml\nexample:\n  harness_protocol: SDD-HARNESS-v1\nid: CT-EXAMPLE\n```\n", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := DeclaredDocID(tc.content); got != tc.want {
				t.Fatalf("identity = %q; want %q", got, tc.want)
			}
		})
	}
}
