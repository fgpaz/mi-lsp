package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

type wikiResolution struct {
	Schema         string   `json:"schema"`
	State          string   `json:"state"`
	Pending        bool     `json:"pending"`
	Mode           string   `json:"mode"`
	CanonRoot      string   `json:"canon_root"`
	AERoot         string   `json:"ae_root"`
	GovernanceFile string   `json:"governance_file"`
	External       any      `json:"external"`
	Source         string   `json:"source"`
	Notes          []string `json:"notes"`
}

func newWikiResolveCommand() *cobra.Command {
	var root string
	command := &cobra.Command{Use: "resolve", Short: "Resolve the local wiki contract", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if strings.TrimSpace(root) == "" {
			return fmt.Errorf("--root is required")
		}
		result := resolveWiki(root)
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		if err := enc.Encode(result); err != nil {
			return err
		}
		if result.Pending {
			return ErrExitCode(2)
		}
		return nil
	}}
	command.Flags().StringVar(&root, "root", "", "Workspace root")
	command.Flags().Bool("json", false, "Emit sandinas-wiki-resolution/v1 JSON")
	return command
}

func resolveWiki(root string) wikiResolution {
	abs, err := filepath.Abs(root)
	result := wikiResolution{Schema: "sandinas-wiki-resolution/v1", State: "pending", Pending: true, Mode: "local", External: false, Source: "default", Notes: []string{}}
	if err != nil {
		result.Notes = append(result.Notes, err.Error())
		return result
	}
	governance := ""
	for _, rel := range []string{".docs/wiki/00_governance.md", ".docs/wiki/governance.md", ".docs/wiki/00_governance.yaml", ".docs/wiki/00_governance.yml"} {
		p := filepath.Join(abs, filepath.FromSlash(rel))
		if _, e := os.Stat(p); e == nil {
			governance = p
			break
		}
	}
	if governance == "" {
		result.Notes = append(result.Notes, "governance_file_not_found")
		return result
	}
	content, err := os.ReadFile(governance)
	if err != nil {
		result.Notes = append(result.Notes, "governance_read_failed: "+err.Error())
		return result
	}
	var doc map[string]any
	text := string(content)
	// Contracts are either a JSON block or YAML in a fenced/whole-document form.
	start := strings.Index(text, "{")
	if start >= 0 {
		if end := balancedJSONObjectEnd(text, start); end > start {
			text = text[start:end]
		}
	}
	if err := yaml.Unmarshal([]byte(text), &doc); err != nil {
		result.Notes = append(result.Notes, "governance_parse_failed: "+err.Error())
		return result
	}
	wiki := mapValue(doc, "wiki")
	if len(wiki) == 0 {
		wiki = mapValue(doc, "wiki_resolution")
	}
	rootValue := firstString(wiki, "canon_root", "wiki_root", "root")
	if rootValue == "" {
		rootValue = firstString(doc, "wiki_root")
	}
	if rootValue == "" {
		rootValue = ".docs/wiki"
		result.Notes = append(result.Notes, "wiki_root_defaulted")
	}
	result.CanonRoot = resolvePath(abs, rootValue)
	result.AERoot = resolvePath(abs, firstString(wiki, "ae_root", "ae_canon_root"))
	result.GovernanceFile = governance
	result.Mode = firstString(wiki, "mode")
	if result.Mode == "" {
		result.Mode = firstString(doc, "mode")
	}
	if result.Mode == "" {
		result.Mode = "local"
	}
	if ext, ok := wiki["external"]; ok {
		result.External = ext
	} else if result.Mode == "repo" || result.Mode == "external" {
		result.External = true
	}
	result.Source = firstString(doc, "schema", "version")
	if result.Source == "" {
		result.Source = "governance"
	}
	if _, err := os.Stat(result.CanonRoot); err != nil {
		result.Notes = append(result.Notes, "canon_root_unavailable")
	}
	result.Pending = false
	result.State = "resolved"
	return result
}
func mapValue(m map[string]any, k string) map[string]any {
	if v, ok := m[k].(map[string]any); ok {
		return v
	}
	return map[string]any{}
}
func firstString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if s, ok := m[k].(string); ok && strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}
func resolvePath(root, value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	if filepath.IsAbs(value) {
		return filepath.Clean(value)
	}
	return filepath.Clean(filepath.Join(root, filepath.FromSlash(value)))
}
func balancedJSONObjectEnd(text string, start int) int {
	depth := 0
	quoted, escape := false, false
	for i := start; i < len(text); i++ {
		c := text[i]
		if quoted {
			if escape {
				escape = false
				continue
			}
			if c == '\\' {
				escape = true
				continue
			}
			if c == '"' {
				quoted = false
			}
			continue
		}
		if c == '"' {
			quoted = true
			continue
		}
		if c == '{' {
			depth++
		}
		if c == '}' {
			depth--
			if depth == 0 {
				return i + 1
			}
		}
	}
	return -1
}
