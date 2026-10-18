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
	Binary             string              `json:"binary"`
	ProviderVersion    string              `json:"provider_version"`
	Description        string              `json:"description"`
	Tags               []string            `json:"tags"`
	MinProviderVersion string              `json:"min_provider_version"`
	Contracts          map[string]string   `json:"contracts"`
	Auth               string              `json:"auth"`
	Invocation         providerInvocation  `json:"invocation"`
	Operations         []providerOperation `json:"operations"`
}

type providerInvocation struct {
	Kind         string         `json:"kind"`
	Command      string         `json:"command"`
	FixedArgs    []string       `json:"fixed_args"`
	InputMapping map[string]any `json:"input_mapping"`
	Output       string         `json:"output"`
	Socket       providerSocket `json:"socket,omitempty"`
}

type providerSocket struct {
	Transport          string `json:"transport"`
	PathEnv            string `json:"path_env"`
	Filename           string `json:"filename"`
	Mode               string `json:"mode"`
	Framing            string `json:"framing"`
	MaxRequestBytes    int    `json:"max_request_bytes"`
	Fallback           string `json:"fallback"`
	RetryAfterResponse bool   `json:"retry_after_response"`
}

type providerOperation struct {
	Name         string         `json:"name"`
	Description  string         `json:"description"`
	Effect       string         `json:"effect"`
	Contract     string         `json:"contract"`
	InputSchema  map[string]any `json:"input_schema"`
	OutputSchema map[string]any `json:"output_schema"`
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
	contracts := map[string]string{
		"q": "q-v1", "intent": "mi-lsp-nav-intent/v0", "multi-read": "mi-lsp-nav-multi-read/v0",
		"version": "mi-lsp-version/v0", "schema": "mi-lsp-schema/v0", "--help": "mi-lsp-help/v0", "--version": "mi-lsp-version-flag/v0",
	}
	return providerManifest{
		Format: "mi-mcp-provider/v2", Namespace: "milsp", Binary: "mi-lsp", ProviderVersion: version,
		Description: "Semantic code navigation and query operations", Tags: []string{"code", "navigation", "semantic search", "workspace"},
		MinProviderVersion: minProviderVersion, Contracts: contracts, Auth: "none",
		Invocation: providerInvocation{
			Kind: "cli", Command: "mi-lsp", FixedArgs: []string{"--format", "json", "--client-name", "mi-mcp", "--no-auto-register"},
			InputMapping: map[string]any{
				"operation": map[string]any{"source": "subcommand"},
				"operations": map[string]any{
					"q":          map[string]any{"pipeline": argumentMapping(0), "workspace": optionMapping("--workspace"), "budget": optionMapping("--budget"), "max_bytes": optionMapping("--max-bytes"), "timeout_ms": optionMapping("--timeout-ms"), "session_id": optionMapping("--session-id"), "page": optionMapping("--page"), "fresh": optionMapping("--fresh"), "dedupe": optionMapping("--dedupe")},
					"intent":     map[string]any{"question": argumentMapping(0), "workspace": optionMapping("--workspace"), "top": optionMapping("--top"), "offset": optionMapping("--offset"), "repo": optionMapping("--repo")},
					"multi-read": map[string]any{"ranges": map[string]any{"source": "argument", "position": 0, "multiple": true}, "workspace": optionMapping("--workspace")},
				},
			}, Output: "json",
			Socket: providerSocket{Transport: "unix", PathEnv: "XDG_RUNTIME_DIR", Filename: cliSocketName, Mode: "0600", Framing: "jsonl", MaxRequestBytes: cliSocketMaxRequest, Fallback: "connection_failure_only", RetryAfterResponse: false},
		},
		Operations: []providerOperation{
			{Name: "q", Description: "Run a typed mi-lsp query pipeline", Effect: "read", Contract: "q-v1", InputSchema: qInputSchema(), OutputSchema: responseSchema("q-v1")},
			{Name: "intent", Description: "Resolve a navigation intent", Effect: "read", Contract: contracts["intent"], InputSchema: objectSchema([]string{"question"}, map[string]any{
				"question": stringSchema(1), "workspace": stringSchema(0), "top": integerSchema(1, 0), "offset": integerSchema(0, 0), "repo": stringSchema(0),
			}), OutputSchema: responseSchema(contracts["intent"])},
			{Name: "multi-read", Description: "Read the requested source ranges", Effect: "read", Contract: contracts["multi-read"], InputSchema: objectSchema([]string{"ranges"}, map[string]any{
				"ranges": map[string]any{"type": "array", "minItems": 1, "items": stringSchema(1)}, "workspace": stringSchema(0),
			}), OutputSchema: responseSchema(contracts["multi-read"])},
			managementOperation("version", contracts["version"]), managementOperation("schema", contracts["schema"]),
			managementOperation("--help", contracts["--help"]), managementOperation("--version", contracts["--version"]),
		},
	}
}

func argumentMapping(position int) map[string]any {
	return map[string]any{"source": "argument", "position": position}
}

func optionMapping(flag string) map[string]any {
	return map[string]any{"source": "option", "flag": flag}
}

func qInputSchema() map[string]any {
	return objectSchema([]string{"pipeline"}, map[string]any{
		"pipeline": stringSchema(1), "workspace": stringSchema(0), "budget": integerSchema(1, 12000),
		"max_bytes": integerSchema(1024, 0), "timeout_ms": integerSchema(1, 30000), "session_id": stringSchema(0),
		"page": stringSchema(0), "fresh": map[string]any{"type": "boolean"}, "dedupe": map[string]any{"type": "boolean"},
	})
}

func objectSchema(required []string, properties map[string]any) map[string]any {
	return map[string]any{"type": "object", "required": required, "additionalProperties": false, "properties": properties}
}

func stringSchema(minLength int) map[string]any {
	return map[string]any{"type": "string", "minLength": minLength}
}

func integerSchema(minimum, maximum int) map[string]any {
	schema := map[string]any{"type": "integer", "minimum": minimum}
	if maximum > 0 {
		schema["maximum"] = maximum
	}
	return schema
}

func responseSchema(contract string) map[string]any {
	return map[string]any{"type": "object", "description": "Response governed by " + contract}
}

func managementOperation(name, contract string) providerOperation {
	return providerOperation{Name: name, Description: "Read mi-lsp provider metadata", Effect: "read", Contract: contract,
		InputSchema: objectSchema([]string{}, map[string]any{}), OutputSchema: responseSchema(contract)}
}
