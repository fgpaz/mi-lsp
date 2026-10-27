package model

import "testing"

func TestA14LanguageFrontmatterBeforeFence(t *testing.T) {
	for _, tc := range []struct {
		name, body      string
		declared, valid bool
		language        WikiContractLanguage
	}{
		{"spanish_frontmatter", "---\nlanguage: es\n---\n```yaml\ntraits: {}\n```\n", true, true, WikiContractLanguageES},
		{"frontmatter_wins_conflict", "---\nlanguage: es\n---\n```yaml\nlanguage: en\n```\n", true, true, WikiContractLanguageES},
		{"english_frontmatter", "---\nlanguage: en\n---\n", true, true, WikiContractLanguageEN},
		{"missing", "---\nversion: 1\n---\n", false, true, WikiContractLanguageES},
		{"invalid_value", "---\nlanguage: fr\n---\n", true, false, ""},
		{"malformed", "---\nlanguage: [\n---\n", false, false, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := ResolveWikiContractLanguage("wiki/00_governance.md", tc.body)
			if got.Declared != tc.declared || got.Valid != tc.valid || got.Language != tc.language {
				t.Fatalf("got %+v", got)
			}
			if !tc.declared && tc.valid && len(got.Warnings) == 0 {
				t.Fatal("missing declaration must remain advisory")
			}
		})
	}
}
