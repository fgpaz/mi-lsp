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
	Kind         string          `json:"kind"`
	Command      string          `json:"command"`
	FixedArgs    []string        `json:"fixed_args"`
	InputMapping map[string]any  `json:"input_mapping"`
	Output       string          `json:"output"`
	Socket       *providerSocket `json:"socket,omitempty"`
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

type providerModelAlias struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
}

type providerOperation struct {
	ModelAlias   *providerModelAlias `json:"model_alias,omitempty"`
	Name         string              `json:"name"`
	Description  string              `json:"description"`
	Effect       string              `json:"effect"`
	Contract     string              `json:"contract"`
	InputSchema  map[string]any      `json:"input_schema"`
	OutputSchema map[string]any      `json:"output_schema"`
	Invocation   *providerInvocation `json:"invocation,omitempty"`
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
		"wiki_graph": "wiki-graph/v1", "wiki_validate_contract": "wiki-contract/v1",
		"version": "mi-lsp-version/v0", "schema": "mi-lsp-schema/v0", "--help": "mi-lsp-help/v0", "--version": "mi-lsp-version-flag/v0",
	}
	manifest := providerManifest{
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
			Socket: &providerSocket{Transport: "unix", PathEnv: "XDG_RUNTIME_DIR", Filename: cliSocketName, Mode: "0600", Framing: "jsonl", MaxRequestBytes: cliSocketMaxRequest, Fallback: "connection_failure_only", RetryAfterResponse: false},
		},
		Operations: []providerOperation{
			{Name: "q", Description: "Run a typed mi-lsp query pipeline", Effect: "read", Contract: "q-v1", InputSchema: qInputSchema(), OutputSchema: responseSchema("q-v1"), Invocation: qInvocation()},
			{Name: "intent", Description: "Resolve a navigation intent", Effect: "read", Contract: contracts["intent"], InputSchema: objectSchema([]string{"question"}, map[string]any{
				"question": stringSchema(1), "workspace": stringSchema(0), "top": integerSchema(1, 0), "offset": integerSchema(0, 0), "repo": stringSchema(0),
			}), OutputSchema: responseSchema(contracts["intent"])},
			{Name: "multi-read", Description: "Read the requested source ranges", Effect: "read", Contract: contracts["multi-read"], InputSchema: objectSchema([]string{"ranges"}, map[string]any{
				"ranges": map[string]any{"type": "array", "minItems": 1, "items": stringSchema(1)}, "workspace": stringSchema(0),
			}), OutputSchema: responseSchema(contracts["multi-read"])},
			{Name: "wiki_graph", Description: "Query the unified wiki and code graph", Effect: "read", Contract: contracts["wiki_graph"], InputSchema: wikiGraphInputSchema(), OutputSchema: responseSchema(contracts["wiki_graph"]), Invocation: wikiGraphInvocation()},
			{Name: "wiki_validate_contract", Description: "Validate WIKI-CONTRACT-v1 and its language-aware checks", Effect: "read", Contract: contracts["wiki_validate_contract"], InputSchema: wikiValidateContractInputSchema(), OutputSchema: responseSchema(contracts["wiki_validate_contract"]), Invocation: wikiValidateContractInvocation()},
			{Name: "doctor", Description: "Inspect local runtime health", Effect: "read", Contract: "mi-lsp-doctor/v0", InputSchema: objectSchema([]string{}, map[string]any{}), OutputSchema: responseSchema("mi-lsp-doctor/v0"), Invocation: maintenanceInvocation("doctor", map[string]any{})},
			{Name: "repair", Description: "Plan or explicitly apply local repairs", Effect: "write", Contract: "mi-lsp-repair/v1", InputSchema: repairInputSchema(), OutputSchema: repairOutputSchema(), Invocation: maintenanceInvocation("repair", map[string]any{
				"workspace": optionMapping("--workspace"), "apply": optionMapping("--apply"), "reindex": optionMapping("--reindex"), "worker": optionMapping("--worker"), "worker_mode": optionMapping("--worker-mode"),
			})},
			managementOperation("version", contracts["version"]), managementOperation("schema", contracts["schema"]),
			managementOperation("--help", contracts["--help"]), managementOperation("--version", contracts["--version"]),
		},
	}
	// One source for both the executable aliases and their CLI/catalog description.
	for i := range manifest.Operations {
		op := &manifest.Operations[i]
		switch op.Name {
		case "intent":
			op.ModelAlias = &providerModelAlias{Namespace: "lsp", Name: "intent"}
			op.Invocation = navigationInvocation([]string{"nav", "intent"}, map[string]any{"question": argumentMapping(0), "top": optionMapping("--top"), "offset": optionMapping("--offset"), "repo": optionMapping("--repo")})
		case "multi-read":
			op.ModelAlias = &providerModelAlias{Namespace: "lsp", Name: "read"}
			op.Invocation = navigationInvocation([]string{"nav", "multi-read"}, map[string]any{"ranges": map[string]any{"source": "argument", "position": 0, "repeat": true}})
		}
		if op.ModelAlias != nil {
			addNavigationBudget(op)
		}
	}
	manifest.Operations = append(manifest.Operations,
		modelNavigationOperation("search", "lsp", "search", "Search text; bounded matches, optional content", []string{"nav", "search"}, "pattern", map[string]any{"include_content": map[string]any{"type": "boolean"}}, map[string]any{"include_content": optionMapping("--include-content")}),
		modelNavigationOperation("refs", "lsp", "refs", "Find symbol references; optional file/line anchor", []string{"nav", "refs"}, "symbol", map[string]any{"file": stringSchema(0), "line": integerSchema(1, 0), "column": integerSchema(1, 0)}, map[string]any{"file": optionMapping("--file"), "line": optionMapping("--line"), "column": optionMapping("--column")}),
	)
	findSymbol := modelNavigationOperation("find_symbol", "lsp", "find_symbol", "Find a symbol with bounded intent, definition, semantic references, and source ranges; cold/catalog results are approximate. Qualified or Unicode Go names may use approximate intent rather than exact-name lookup", []string{"nav", "find_symbol"}, "query", map[string]any{"repo": stringSchema(0)}, map[string]any{"repo": optionMapping("--repo")})
	findSymbol.Contract = "find_symbol/v0"
	findSymbol.OutputSchema = nativeVerbResponseSchema(findSymbol.Contract, map[string]any{
		"query": stringSchema(0), "approximate": map[string]any{"type": "boolean"}, "source": stringSchema(0), "semantic_status": stringSchema(0),
		"definitions": map[string]any{"type": "array", "items": map[string]any{"type": "object"}}, "usages": map[string]any{"type": "array", "items": map[string]any{"type": "object"}},
		"usage_candidates": map[string]any{"type": "array", "items": map[string]any{"type": "object"}}, "ranges": map[string]any{"type": "array", "items": map[string]any{"type": "object"}},
		"ambiguity": map[string]any{"type": "boolean"},
	})
	manifest.Operations = append(manifest.Operations, findSymbol)
	rename := modelNavigationOperation("rename_symbol", "lsp", "rename_symbol", "Apply a versioned gopls rename after live prepareRename confirmation. Bare symbol names never auto-select because workspace/symbol completeness is unproven; select a prepareRename-validated opaque candidate.symbol and retry using the same symbol input field. Candidate lists are bounded and candidates_complete=false; file/line/column remain optional all-or-none selectors. Compilation remains unchecked. change_signature is unavailable without a semantic backend plan", []string{"nav", "rename_symbol"}, "symbol", map[string]any{
		"new_name": stringSchema(1), "file": stringSchema(0), "line": integerSchema(1, 0), "column": integerSchema(1, 0),
		"repo": stringSchema(0), "entrypoint": stringSchema(0), "solution": stringSchema(0), "project_path": stringSchema(0),
	}, map[string]any{
		"new_name": optionMapping("--new-name"), "file": optionMapping("--file"), "line": optionMapping("--line"), "column": optionMapping("--column"),
		"repo": optionMapping("--repo"), "entrypoint": optionMapping("--entrypoint"), "solution": optionMapping("--solution"), "project_path": optionMapping("--project"),
	})
	rename.Effect = "write"
	rename.Contract = "rename_symbol/v0"
	rename.OutputSchema = nativeVerbResponseSchema(rename.Contract, map[string]any{
		"status": stringSchema(0), "symbol": stringSchema(0), "new_name": stringSchema(0), "files_touched": map[string]any{"type": "array", "items": stringSchema(0)},
		"diagnostics_status": stringSchema(0), "compilation_state": map[string]any{"type": "string", "enum": []string{"not_checked"}}, "application": stringSchema(0),
		"diagnostics": map[string]any{"type": "object", "properties": map[string]any{
			"status":            map[string]any{"type": "string", "enum": []string{"checked", "partial", "not_checked"}},
			"compilation_state": map[string]any{"type": "string", "enum": []string{"not_checked"}}, "files_requested": integerSchema(0, 0), "files_checked": integerSchema(0, 0),
			"errors_observed": integerSchema(0, 0), "error_count_complete": map[string]any{"type": "boolean"}, "details_complete": map[string]any{"type": "boolean"},
			"diagnostics":       map[string]any{"type": "array", "items": map[string]any{"type": "object"}},
			"not_checked_files": map[string]any{"type": "array", "items": stringSchema(0)}, "truncated": map[string]any{"type": "boolean"}, "next_hint": stringSchema(0),
		}}, "mutation_state": stringSchema(0), "partial_mutation": map[string]any{"type": "boolean"},
		"candidates_complete": map[string]any{"type": "boolean"}, "uniqueness": map[string]any{"type": "string", "enum": []string{"not_proven"}},
		"candidates": map[string]any{"type": "array", "items": map[string]any{"type": "object", "properties": map[string]any{
			"name": stringSchema(0), "symbol": map[string]any{"type": "string", "maxLength": 4096, "description": "Versioned opaque mi-lsp-symbol:v1 selector; untrusted and valid only after workspace, repo, path, snapshot, and live prepareRename checks"},
			"file": stringSchema(0), "line": integerSchema(1, 0), "column": integerSchema(1, 0), "kind": integerSchema(0, 0), "container_name": stringSchema(0),
			"repo": stringSchema(0), "repo_id": stringSchema(0), "root": stringSchema(0), "default_entrypoint": stringSchema(0), "match_reason": stringSchema(0),
		}}},
		"recovery_artifacts": map[string]any{"type": "array", "items": map[string]any{"type": "object", "properties": map[string]any{
			"file": stringSchema(0), "path": stringSchema(0), "kind": stringSchema(1), "state": stringSchema(1), "detail": stringSchema(0),
		}}},
	})
	rename.InputSchema["required"] = []string{"symbol", "new_name"}
	rename.InputSchema["properties"].(map[string]any)["symbol"] = map[string]any{"type": "string", "minLength": 1, "maxLength": 4096, "description": "Bare Go identifier requests a bounded candidate list and never auto-renames; or pass a returned opaque candidate.symbol selector in this same field"}
	rename.InputSchema["allOf"] = []any{map[string]any{
		"if": map[string]any{"anyOf": []any{
			map[string]any{"required": []string{"file"}},
			map[string]any{"required": []string{"line"}},
			map[string]any{"required": []string{"column"}},
		}},
		"then": map[string]any{"required": []string{"file", "line", "column"}},
	}}
	manifest.Operations = append(manifest.Operations, rename)
	getErrors := modelNavigationOperation("get_errors", "lsp", "get_errors", "Semantic diagnostics for up to 64 canonical explicit paths (repeat --file or positional), or changed source paths; partial/not_checked is not clean and diagnostics do not prove compilation", []string{"nav", "get_errors"}, "", map[string]any{
		"files": map[string]any{"type": "array", "maxItems": 256, "items": stringSchema(1)}, "base": stringSchema(1), "max_files": integerSchema(1, 64),
	}, map[string]any{
		"files": map[string]any{"source": "option", "flag": "--file", "repeat": true}, "base": optionMapping("--base"), "max_files": optionMapping("--max-files"),
	})
	getErrors.Contract = "get_errors/v0"
	getErrors.OutputSchema = nativeVerbResponseSchema(getErrors.Contract, map[string]any{
		"status":            map[string]any{"type": "string", "enum": []string{"checked", "partial", "not_checked"}},
		"compilation_state": map[string]any{"type": "string", "enum": []string{"not_checked"}}, "files_requested": integerSchema(0, 0), "files_checked": integerSchema(0, 0),
		"errors_observed": integerSchema(0, 0), "error_count_complete": map[string]any{"type": "boolean"}, "details_complete": map[string]any{"type": "boolean"},
		"diagnostics":       map[string]any{"type": "array", "items": map[string]any{"type": "object"}},
		"not_checked_files": map[string]any{"type": "array", "items": stringSchema(0)}, "files_touched": map[string]any{"type": "array", "items": stringSchema(0)},
		"truncated": map[string]any{"type": "boolean"}, "next_hint": stringSchema(0),
	})
	manifest.Operations = append(manifest.Operations, getErrors)
	diagnostics := modelNavigationOperation("diagnostics", "lsp", "diagnostics", "Errors/warnings after editing a file or changed files; partial is not clean", []string{"nav", "diagnostics"}, "", map[string]any{"file": stringSchema(1), "changed": map[string]any{"type": "boolean"}, "base": stringSchema(1), "max_files": integerSchema(1, 64), "offset": integerSchema(0, 0)}, map[string]any{"file": argumentMapping(0), "changed": optionMapping("--changed"), "base": optionMapping("--base"), "max_files": optionMapping("--max-files"), "offset": optionMapping("--offset")})
	diagnostics.Contract = "diagnostics/v1"
	diagnostics.OutputSchema = responseSchema(diagnostics.Contract)
	manifest.Operations = append(manifest.Operations, diagnostics)
	find := modelNavigationOperation("find-files", "fs", "find", "Find filenames/globs respecting ignore rules; no index", []string{"find-files"}, "pattern", map[string]any{"top": integerSchema(1, 1000), "budget_bytes": integerSchema(512, 1048576), "literal": map[string]any{"type": "boolean"}, "refresh": map[string]any{"type": "boolean"}}, map[string]any{"top": optionMapping("--top"), "budget_bytes": optionMapping("--budget-bytes"), "literal": optionMapping("--literal"), "refresh": optionMapping("--refresh")})
	find.Contract = "find-files/v1"
	find.OutputSchema = responseSchema(find.Contract)
	manifest.Operations = append(manifest.Operations, find)
	for _, op := range manifest.Operations {
		manifest.Contracts[op.Name] = op.Contract
	}
	return manifest
}

func maintenanceInvocation(command string, fields map[string]any) *providerInvocation {
	return &providerInvocation{Kind: "cli", Command: "mi-lsp", FixedArgs: []string{"--format", "json", "--client-name", "mi-mcp", "--no-auto-register", command}, InputMapping: map[string]any{"fields": fields}, Output: "json"}
}

func repairInputSchema() map[string]any {
	return objectSchema([]string{}, map[string]any{
		"workspace":   stringSchema(1),
		"apply":       map[string]any{"type": "boolean", "default": false},
		"reindex":     map[string]any{"type": "boolean", "default": false},
		"worker":      map[string]any{"type": "boolean", "default": false},
		"worker_mode": map[string]any{"type": "string", "enum": []string{"auto", "framework-dependent", "self-contained"}, "default": "auto"},
	})
}

func repairOutputSchema() map[string]any {
	stringField := map[string]any{"type": "string"}
	timestamp := map[string]any{"type": []string{"string", "null"}, "format": "date-time"}
	return map[string]any{"type": "object", "required": []string{"schema", "ok", "mode", "runtime", "actions", "reasonCodes", "changed"}, "properties": map[string]any{
		"schema": map[string]any{"const": "mi-lsp-repair/v1"}, "ok": map[string]any{"type": "boolean"}, "changed": map[string]any{"type": "boolean"},
		"mode":    map[string]any{"type": "string", "enum": []string{"plan", "apply"}},
		"runtime": map[string]any{"type": "object", "required": []string{"executablePath", "managedRoot", "managedExecutablePath", "receiptPath"}, "properties": map[string]any{"executablePath": stringField, "managedRoot": stringField, "managedExecutablePath": stringField, "receiptPath": stringField}},
		"actions": map[string]any{"type": "array", "items": map[string]any{"type": "object", "required": []string{"id", "status", "reasonCode", "detail", "retryable", "nextAction", "lastAttemptAt", "retryAfter"}, "properties": map[string]any{
			"id":         map[string]any{"type": "string", "enum": []string{"runtime", "config", "workspace", "index", "worker"}},
			"status":     map[string]any{"type": "string", "enum": []string{"pass", "skip", "planned", "repaired", "blocked", "failed"}},
			"reasonCode": stringField, "detail": stringField, "retryable": map[string]any{"type": "boolean"}, "nextAction": stringField, "lastAttemptAt": timestamp, "retryAfter": timestamp,
		}}},
		"reasonCodes": map[string]any{"type": "array", "items": stringField},
	}}
}

func qInvocation() *providerInvocation {
	return &providerInvocation{Kind: "cli", Command: "mi-lsp", FixedArgs: []string{"--format", "json", "--client-name", "mi-mcp", "--no-auto-register", "q"}, InputMapping: map[string]any{"fields": map[string]any{
		"pipeline": argumentMapping(0), "workspace": optionMapping("--workspace"), "budget": optionMapping("--budget"), "max_bytes": optionMapping("--max-bytes"), "timeout_ms": optionMapping("--timeout-ms"), "session_id": optionMapping("--session-id"), "page": optionMapping("--page"), "fresh": optionMapping("--fresh"), "dedupe": optionMapping("--dedupe"),
	}}, Output: "json"}
}

func navigationInvocation(argv []string, fields map[string]any) *providerInvocation {
	fields["workspace"] = optionMapping("--workspace")
	return &providerInvocation{Kind: "cli", Command: "mi-lsp", FixedArgs: append([]string{"--format", "json", "--client-name", "mi-mcp", "--no-auto-register", "--token-budget", "2000", "--max-items", "20"}, argv...), InputMapping: map[string]any{"fields": fields}, Output: "json"}
}

func addNavigationBudget(op *providerOperation) {
	properties := op.InputSchema["properties"].(map[string]any)
	fields := op.Invocation.InputMapping["fields"].(map[string]any)
	for key, flag := range map[string]string{"max_items": "--max-items", "token_budget": "--token-budget"} {
		properties[key] = integerSchema(1, 0)
		fields[key] = optionMapping(flag)
	}
}

func modelNavigationOperation(name, namespace, alias, description string, argv []string, positional string, properties, fields map[string]any) providerOperation {
	properties["workspace"] = stringSchema(0)
	required := []string{}
	if positional != "" {
		properties[positional] = stringSchema(1)
		fields[positional] = argumentMapping(0)
		required = append(required, positional)
	}
	op := providerOperation{Name: name, ModelAlias: &providerModelAlias{Namespace: namespace, Name: alias}, Description: description, Effect: "read", Contract: "mi-lsp-" + name + "/v0", InputSchema: objectSchema(required, properties), OutputSchema: responseSchema("mi-lsp-" + name + "/v0"), Invocation: navigationInvocation(argv, fields)}
	addNavigationBudget(&op)
	return op
}

func nativeVerbResponseSchema(contract string, itemProperties map[string]any) map[string]any {
	return map[string]any{
		"type": "object", "description": "Response governed by " + contract + "; item status and partial/detail fields distinguish checked results from incomplete results",
		"properties": map[string]any{
			"ok": map[string]any{"type": "boolean"}, "contract_version": map[string]any{"type": "string", "const": contract}, "partial": map[string]any{"type": "boolean"}, "degraded": map[string]any{"type": "boolean"},
			"reason": stringSchema(0), "workspace": stringSchema(0), "backend": stringSchema(0), "items": map[string]any{"type": "object", "properties": itemProperties},
			"truncated": map[string]any{"type": "boolean"}, "next_hint": stringSchema(0), "warnings": map[string]any{"type": "array", "items": stringSchema(0)},
			"diagnostics": map[string]any{"type": "object", "properties": map[string]any{
				"errors": integerSchema(0, 0), "warnings": integerSchema(0, 0), "information": integerSchema(0, 0), "hints": integerSchema(0, 0),
				"files_requested": integerSchema(0, 0), "files_checked": integerSchema(0, 0), "returned": integerSchema(0, 0), "backends": map[string]any{"type": "object"},
			}},
			"omissions": map[string]any{"type": "array", "items": map[string]any{"type": "object"}},
			"error":     map[string]any{"type": "object", "properties": map[string]any{"kind": stringSchema(0), "code": stringSchema(0), "message": stringSchema(0), "reason_code": stringSchema(0)}},
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

func wikiGraphInputSchema() map[string]any {
	return objectSchema([]string{"workspace", "mode"}, map[string]any{
		"workspace": stringSchema(1),
		"mode":      map[string]any{"type": "string", "enum": []string{"neighbors", "path", "explain", "affected", "change-pack", "matrix", "roadmap"}},
		"from":      stringSchema(0), "to": stringSchema(0), "ref": stringSchema(0),
		"changed_paths": map[string]any{"type": "array", "minItems": 1, "items": stringSchema(1)},
		"relations":     stringSchema(0),
		"direction":     map[string]any{"type": "string", "enum": []string{"in", "out", "both"}},
		"depth":         integerSchema(0, 0), "limit": integerSchema(1, 500), "cursor": stringSchema(0),
		"render": map[string]any{"type": "string", "enum": []string{"mermaid"}},
	})
}

func wikiGraphInvocation() *providerInvocation {
	return &providerInvocation{
		Kind: "cli", Command: "mi-lsp",
		FixedArgs: []string{"--format", "json", "--client-name", "mi-mcp", "--no-auto-register", "--no-daemon", "--allow-cross-workspace", "nav", "wiki", "graph"},
		InputMapping: map[string]any{"fields": map[string]any{
			"workspace": optionMapping("--workspace"), "mode": optionMapping("--mode"),
			"from": optionMapping("--from"), "to": optionMapping("--to"), "ref": optionMapping("--ref"),
			"changed_paths": map[string]any{"source": "argument", "position": 0, "repeat": true},
			"relations":     optionMapping("--relations"), "direction": optionMapping("--direction"),
			"depth": optionMapping("--depth"), "limit": optionMapping("--limit"),
			"cursor": optionMapping("--cursor"), "render": optionMapping("--render"),
		}},
		Output: "json",
	}
}

func wikiValidateContractInputSchema() map[string]any {
	return objectSchema([]string{"workspace"}, map[string]any{
		"workspace": stringSchema(1), "ids": stringSchema(0), "paths": stringSchema(0),
	})
}

func wikiValidateContractInvocation() *providerInvocation {
	return &providerInvocation{
		Kind: "cli", Command: "mi-lsp",
		FixedArgs: []string{"--format", "json", "--client-name", "mi-mcp", "--no-auto-register", "--no-daemon", "--allow-cross-workspace", "nav", "wiki", "validate-contract"},
		InputMapping: map[string]any{"fields": map[string]any{
			"workspace": optionMapping("--workspace"), "ids": optionMapping("--ids"), "paths": optionMapping("--paths"),
		}},
		Output: "json",
	}
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
