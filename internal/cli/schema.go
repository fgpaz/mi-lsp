package cli

import (
	"encoding/json"
	"os"

	"github.com/spf13/cobra"
)

func newSchemaCommand(state *rootState) *cobra.Command {
	_ = state
	command := &cobra.Command{
		Use:   "schema",
		Short: "Print the public management schema without starting a daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				encoder := json.NewEncoder(cmd.OutOrStdout())
				if err := encoder.Encode(map[string]any{
					"ok": false, "reason_code": "parametro_invalido",
					"detail": map[string]string{"message": "schema no acepta argumentos"},
				}); err != nil {
					return err
				}
				os.Exit(2)
			}
			encoder := json.NewEncoder(cmd.OutOrStdout())
			encoder.SetIndent("", "  ")
			return encoder.Encode(map[string]any{
				"ok": true,
				"data": map[string]any{
					"schema":           "mi-lsp-public-surface/v0",
					"version_command":  "text",
					"version_flag":     "text",
					"version_formats_differ": true,
					"provider_format":  "mi-mcp-provider/v1",
					"daemon":           false,
					"index":            false,
					"commands":         []string{"version", "--version", "schema", "provider-manifest", "--help"},
				},
			})
		},
	}
	command.SilenceUsage = true
	return command
}
