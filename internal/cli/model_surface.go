package cli

import (
	"encoding/json"

	"github.com/spf13/cobra"
)

// Model discovery is deliberately smaller than nav --help and provider-manifest.
// The provider owns names, budgets, schema and invocation; this is only a view.
func newModelSurfaceCommand() *cobra.Command {
	return &cobra.Command{
		Use: "models", Short: "Show the small model surface (full catalog: nav --help / provider-manifest)", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			type entry struct {
				Name        string         `json:"name"`
				Description string         `json:"description"`
				InputSchema map[string]any `json:"input_schema"`
			}
			entries := []entry{}
			for _, op := range newProviderManifest("unknown").Operations {
				if op.ModelAlias != nil {
					entries = append(entries, entry{Name: op.ModelAlias.Namespace + "." + op.ModelAlias.Name, Description: op.Description, InputSchema: op.InputSchema})
				}
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{"operations": entries, "default_token_budget": 2000, "default_max_items": 20, "advanced": "mi-lsp nav --help; mi-lsp provider-manifest"})
		},
	}
}
