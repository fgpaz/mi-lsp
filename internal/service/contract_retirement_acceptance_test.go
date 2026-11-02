package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/fgpaz/mi-lsp/internal/model"
)

// These are observable validation oracles, not claims about hidden inventory
// state, execution on other platforms, or acceptance of unrelated criteria.
func TestRetirementCriterionFiveValidationOracles(t *testing.T) {
	const path = ".docs/wiki/00_retirements.md"
	const archive = ".docs/wiki/_retired/requirement/RF-OLD-01.md"
	row := "  - id: RF-OLD-01\n    old_path: .docs/wiki/05_requirements/RF-OLD-01.md\n    archived_path: " + archive + "\n    reason: Retired\n    terminal: true\n    superseded_by: null\n"
	validate := func(t *testing.T, body string) model.WikiContractValidationResult {
		t.Helper()
		root := t.TempDir()
		writeContractTestWikiFile(t, root, archive, "# Archived\n")
		writeContractTestWikiFile(t, root, path, body)
		return compileWikiContractValidation(root, []model.DocRecord{{Path: path}}, nil)
	}
	for _, key := range []string{"entries", "retirements"} {
		t.Run(key, func(t *testing.T) {
			good := "```yaml\n" + key + ":\n" + row + "```\n"
			got := validate(t, good)
			if got.ContractVerdict != "PASS" || len(got.ContractWarnings) != 0 {
				t.Fatalf("valid inventory: %+v", got)
			}
			for _, field := range []string{"id", "old_path", "archived_path", "reason", "terminal"} {
				t.Run("missing_"+field, func(t *testing.T) {
					lines := strings.Split(good, "\n")
					for i, line := range lines {
						trimmed := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "- "))
						if strings.HasPrefix(trimmed, field+":") {
							if field == "id" {
								lines[i] = "  - {}"
							} else {
								lines[i] = ""
							}
						}
					}
					// Removing the first key must preserve the remaining mapping, not
					// accidentally turn the fixture into invalid YAML.
					body := strings.Join(lines, "\n")
					if field == "id" {
						body = strings.Replace(good, "  - id: RF-OLD-01\n    old_path:", "  - old_path:", 1)
					}
					bad := validate(t, body)
					if bad.ContractVerdict != "ADVISORY" || !strings.Contains(strings.Join(bad.ContractWarnings, "\n"), "missing "+field) {
						t.Fatalf("missing %s must remain specifically advisory: %+v", field, bad)
					}
				})
			}
		})
	}
	for name, body := range map[string]string{
		"prose":         "# Registro de retiros\n\nNo hay retiros registrados.\n",
		"entries_empty": "```yaml\nentries: []\n```\n",
		"legacy_empty":  "```yaml\nretirements: []\n```\n",
	} {
		t.Run(name, func(t *testing.T) {
			got := validate(t, body)
			if got.ContractVerdict != "PASS" || got.ContractsReviewed != 1 || len(got.ContractWarnings) != 0 {
				t.Fatalf("explicit empty inventory: %+v", got)
			}
			data, err := json.Marshal(got)
			if err != nil {
				t.Fatal(err)
			}
			// This asserts no fabricated RF identifier in the PUBLIC validation
			// result. It does not purport to inspect an unexposed rows structure.
			if strings.Contains(string(data), "RF-") {
				t.Fatalf("fabricated RF identifier in empty-inventory result: %s", data)
			}
		})
	}
	for name, body := range map[string]string{
		"ambiguous":     "# Registro\nSin novedades por ahora.\n",
		"malformed":     "```yaml\nentries: [\n```\n",
		"contradictory": "No hay retiros registrados.\n\n```yaml\nentries:\n" + row + "```\n",
		"wrong_boolean": "```yaml\nentries:\n" + strings.Replace(row, "terminal: true", "terminal: 'true'", 1) + "```\n",
	} {
		t.Run(name, func(t *testing.T) {
			got := validate(t, body)
			if got.ContractVerdict != "ADVISORY" || len(got.ContractWarnings) == 0 {
				t.Fatalf("invalid/ambiguous inventory must stay advisory: %+v", got)
			}
		})
	}
}
