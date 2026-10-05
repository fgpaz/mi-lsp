package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

const minProviderVersion = "0.10.0"

type providerManifest struct {
	Format             string              `json:"format"`
	Namespace          string              `json:"namespace"`
	ProviderVersion    string              `json:"provider_version"`
	MinProviderVersion string              `json:"min_provider_version"`
	Contracts          map[string]string   `json:"contracts"`
	Auth               string              `json:"auth"`
	Invocation         providerInvocation  `json:"invocation"`
	Operations         []providerOperation `json:"operations"`
}

type providerInvocation struct {
	Kind      string   `json:"kind"`
	Command   string   `json:"command"`
	FixedArgs []string `json:"fixed_args"`
}

type providerOperation struct {
	Name        string         `json:"name"`
	Effect      string         `json:"effect"`
	Contract    string         `json:"contract"`
	InputSchema map[string]any `json:"input_schema"`
	Output      map[string]any `json:"output"`
}

func newProviderManifestCommand(state *rootState) *cobra.Command {
	format := "json"
	command := &cobra.Command{
		Use:   "provider-manifest",
		Short: "Print the mi-mcp provider manifest",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if format != "json" {
				return fmt.Errorf("unsupported provider manifest format %q; only json is supported", format)
			}
			version := strings.Fields(buildRootVersionInfo(state.repoRoot).Version)
			providerVersion := "unknown"
			if len(version) > 0 {
				providerVersion = strings.TrimPrefix(version[0], "v")
			}
			encoder := json.NewEncoder(cmd.OutOrStdout())
			encoder.SetIndent("", "  ")
			return encoder.Encode(newProviderManifest(providerVersion))
		},
	}
	command.Flags().StringVar(&format, "format", "json", "Manifest output format (json)")
	return command
}

func newProviderManifest(version string) providerManifest {
	return providerManifest{
		Format:             "mi-mcp-provider/v1",
		Namespace:          "milsp",
		ProviderVersion:    version,
		MinProviderVersion: minProviderVersion,
		Contracts:          map[string]string{"q": "q-v1"},
		Auth:               "none",
		Invocation: providerInvocation{
			Kind:      "cli",
			Command:   "mi-lsp",
			FixedArgs: []string{"--format", "json", "--client-name", "mi-mcp", "--no-auto-register"},
		},
		Operations: []providerOperation{{
			Name:     "q",
			Effect:   "read",
			Contract: "q-v1",
			InputSchema: map[string]any{
				"type":                 "object",
				"required":             []string{"pipeline"},
				"additionalProperties": false,
				"properties": map[string]any{
					"pipeline":   map[string]any{"type": "string", "minLength": 1},
					"workspace":  map[string]any{"type": "string"},
					"budget":     map[string]any{"type": "integer", "minimum": 1, "maximum": 12000},
					"max_bytes":  map[string]any{"type": "integer", "minimum": 1024},
					"timeout_ms": map[string]any{"type": "integer", "minimum": 1, "maximum": 30000},
					"session_id": map[string]any{"type": "string"},
					"page":       map[string]any{"type": "string"},
					"fresh":      map[string]any{"type": "boolean"},
					"dedupe":     map[string]any{"type": "boolean"},
				},
			},
			Output: map[string]any{
				"contract_version": "q-v1",
				"ref":              "mi-lsp .docs/wiki/09_contratos/CT-Q-V1.md",
			},
		}},
	}
}
