package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/output"
	"github.com/spf13/cobra"
)

const mcpProtocolVersion = "2024-11-05"

type mcpMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type mcpResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *mcpRPCError    `json:"error,omitempty"`
}

type mcpRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type mcpTextContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type mcpToolResult struct {
	Content           []mcpTextContent `json:"content"`
	StructuredContent model.Envelope   `json:"structuredContent"`
	IsError           bool             `json:"isError"`
}

type mcpTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

func newMCPCommand(state *rootState) *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "Serve native Model Context Protocol tools over stdio",
		Long:  "Run the native MCP server using line-delimited JSON-RPC on stdin and stdout.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return serveMCP(cmd.Context(), cmd.InOrStdin(), cmd.OutOrStdout(), state, cmd)
		},
		Hidden: false,
	}
}

func serveMCP(ctx context.Context, input io.Reader, outputWriter io.Writer, state *rootState, cmd *cobra.Command) error {
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 64*1024), 8*1024*1024)
	encoder := json.NewEncoder(outputWriter)
	for scanner.Scan() {
		line := scanner.Bytes()
		var message mcpMessage
		if err := json.Unmarshal(line, &message); err != nil {
			if err := encoder.Encode(mcpResponse{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &mcpRPCError{Code: -32700, Message: "Parse error"}}); err != nil {
				return err
			}
			continue
		}
		if message.JSONRPC != "2.0" || strings.TrimSpace(message.Method) == "" {
			id := message.ID
			if len(id) == 0 {
				id = json.RawMessage("null")
			}
			if err := encoder.Encode(mcpResponse{JSONRPC: "2.0", ID: id, Error: &mcpRPCError{Code: -32600, Message: "Invalid Request"}}); err != nil {
				return err
			}
			continue
		}
		if message.Method == "notifications/initialized" || message.Method == "notifications/cancelled" {
			continue
		}
		if len(message.ID) == 0 {
			continue
		}
		response := handleMCPMessage(ctx, message, state, cmd)
		if err := encoder.Encode(response); err != nil {
			return err
		}
	}
	return scanner.Err()
}

func handleMCPMessage(ctx context.Context, message mcpMessage, state *rootState, cmd *cobra.Command) mcpResponse {
	response := mcpResponse{JSONRPC: "2.0", ID: message.ID}
	if message.JSONRPC != "2.0" || strings.TrimSpace(message.Method) == "" {
		response.Error = &mcpRPCError{Code: -32600, Message: "Invalid Request: jsonrpc must be 2.0"}
		return response
	}
	switch message.Method {
	case "initialize":
		var params struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(message.Params, &params)
		version := mcpProtocolVersion
		if params.ProtocolVersion == mcpProtocolVersion {
			version = params.ProtocolVersion
		}
		response.Result = map[string]any{
			"protocolVersion": version,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]string{"name": "mi-lsp", "version": rootVersionString(buildRootVersionInfo(state.repoRoot))},
		}
	case "ping":
		response.Result = map[string]any{}
	case "tools/list":
		response.Result = map[string]any{"tools": mcpTools()}
	case "tools/call":
		response.Result = callMCPTool(ctx, message.Params, state, cmd)
	default:
		response.Error = &mcpRPCError{Code: -32601, Message: "Method not found: " + message.Method}
	}
	return response
}

func callMCPTool(ctx context.Context, rawParams json.RawMessage, state *rootState, cmd *cobra.Command) mcpToolResult {
	var params struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}
	if err := json.Unmarshal(rawParams, &params); err != nil {
		return mcpFailure("Invalid tools/call parameters")
	}
	operation, payload, ok := mcpOperation(params.Name, params.Arguments)
	if !ok {
		return mcpFailure("Unknown MCP tool: " + params.Name)
	}
	workspace := stringArgument(params.Arguments, "workspace")
	if workspace == "" {
		workspace = state.workspace
	}
	payload = clonePayload(payload)
	delete(payload, "workspace")
	options := state.queryOptions(cmd, operation, payload)
	options.Workspace = workspace
	options.Format = "json"
	options.Verbose = false
	request := model.CommandRequest{
		ProtocolVersion: model.ProtocolVersion,
		Operation:       operation,
		Context:         options,
		Payload:         payload,
	}
	refreshWarning := ""
	if operation == "nav.multi-read" {
		if _, refreshErr := state.refreshMultiReadPaths(ctx, request); refreshErr != nil {
			refreshWarning = "query index refresh deferred: " + refreshErr.Error()
		}
	}
	requestContext, cancel := context.WithTimeout(ctx, timeoutForRequest(request))
	defer cancel()
	envelope, err := state.app.Execute(requestContext, request)
	if err != nil {
		envelope = buildCLIErrorEnvelope(request, "direct", err)
	}
	if refreshWarning != "" {
		envelope.Warnings = append(envelope.Warnings, refreshWarning)
	}
	envelope = output.ApplyEnvelopeLimits(envelope, options)
	envelope.Profile = model.OutputProfileHuman
	format := "agent"
	if flagChanged(cmd, "format") {
		format = state.format
	}
	if state.verbose && format == "agent" {
		format = "compact"
	}
	compact, renderErr := output.Render(envelope, format, options.Compress)
	if renderErr != nil {
		return mcpFailure("Failed to render tool result")
	}
	return mcpToolResult{
		Content:           []mcpTextContent{{Type: "text", Text: string(compact)}},
		StructuredContent: envelope,
		IsError:           !envelope.Ok,
	}
}

func mcpFailure(message string) mcpToolResult {
	envelope := model.Envelope{
		Ok:      false,
		Backend: "mcp",
		Items:   []map[string]any{},
		Error:   &model.EnvelopeError{Kind: "validation", Code: "invalid_tool_call", Message: message, Stage: "selector_validation"},
	}
	return mcpToolResult{Content: []mcpTextContent{{Type: "text", Text: message}}, StructuredContent: envelope, IsError: true}
}

func mcpOperation(name string, args map[string]any) (string, map[string]any, bool) {
	if args == nil {
		args = map[string]any{}
	}
	payload := clonePayload(args)
	switch name {
	case "nav_intent":
		payload["question"] = args["question"]
		return "nav.intent", payload, true
	case "nav_route":
		if value, ok := args["includeCodeDiscovery"]; ok {
			payload["include_code_discovery"] = value
			delete(payload, "includeCodeDiscovery")
		}
		return "nav.route", payload, true
	case "nav_pack":
		return "nav.pack", payload, true
	case "nav_wiki":
		op := stringArgument(args, "op")
		operations := map[string]string{"search": "nav.wiki.search", "route": "nav.wiki.route", "pack": "nav.wiki.pack", "trace": "nav.wiki.trace", "inventory": "nav.wiki.inventory", "map": "nav.wiki.map", "root": "nav.wiki-root"}
		operation, ok := operations[op]
		if !ok {
			return "", nil, false
		}
		for source, target := range map[string]string{"allWorkspaces": "all_workspaces", "includeContent": "include_content", "withLayerCounts": "with_layer_counts"} {
			if value, exists := payload[source]; exists {
				payload[target] = value
				delete(payload, source)
			}
		}
		return operation, payload, true
	case "nav_search":
		for source, target := range map[string]string{"allWorkspaces": "all_workspaces", "includeContent": "include_content", "contextLines": "context_lines", "contextMode": "context_mode"} {
			if value, exists := payload[source]; exists {
				payload[target] = value
				delete(payload, source)
			}
		}
		return "nav.search", payload, true
	case "nav_find":
		if value, ok := payload["allWorkspaces"]; ok {
			payload["all_workspaces"] = value
			delete(payload, "allWorkspaces")
		}
		return "nav.find", payload, true
	case "nav_refs":
		return "nav.refs", payload, true
	case "nav_related":
		return "nav.related", payload, true
	case "nav_flow_slice":
		return "nav.flow-slice", payload, true
	case "nav_change_pack":
		return "nav.change-pack", payload, true
	case "nav_affected":
		for source, target := range map[string]string{"changedRef": "changed_ref", "fromGitDiff": "from_git_diff", "includeDocs": "include_docs", "includeTests": "include_tests"} {
			if value, exists := payload[source]; exists {
				payload[target] = value
				delete(payload, source)
			}
		}
		return "nav.affected", payload, true
	case "nav_multi_read":
		ranges, ok := args["ranges"].([]any)
		if !ok {
			if typed, typedOK := args["ranges"].([]string); typedOK {
				ranges = make([]any, len(typed))
				for i := range typed {
					ranges[i] = typed[i]
				}
			} else {
				return "", nil, false
			}
		}
		payload = map[string]any{"args": ranges}
		return "nav.multi-read", payload, true
	case "nav_overview":
		if value, ok := payload["workspaceMap"]; ok {
			payload["workspace_map"] = value
			delete(payload, "workspaceMap")
		}
		return "nav.overview", payload, true
	default:
		return "", nil, false
	}
}

func clonePayload(source map[string]any) map[string]any {
	copy := make(map[string]any, len(source))
	for key, value := range source {
		copy[key] = value
	}
	return copy
}

func stringArgument(args map[string]any, key string) string {
	value, _ := args[key].(string)
	return strings.TrimSpace(value)
}

func mcpTools() []mcpTool {
	return []mcpTool{
		tool("nav_intent", "Resolve an open-ended goal in hybrid docs|code mode (BM25 over wiki + symbol metadata). This is the DEFAULT first move for an exploratory question such as 'how does X work', 'what governs Y', or 'where do we handle Z' when you do not yet know the exact file. Prefer this over starting a Read/Grep/Glob loop; follow the returned continuation.next verbatim instead of guessing the next raw search.", []string{"question"}, field("question", "string", "The open-ended question or goal, in natural language."), field("top", "integer", "Maximum number of results (default 10)."), field("offset", "integer", "Skip first N results, for pagination."), field("repo", "string", "Repo child selector for container workspaces."), field("workspace", "string", "Optional workspace alias or path; auto-detected from cwd when omitted.")),
		tool("nav_route", "Resolve the single canonical governing wiki document for a literal task/topic in one call. Prefer this over grepping the wiki tree by hand, or over nav_intent, whenever the request is explicitly 'which doc governs/owns X' rather than an open exploratory question.", []string{"task"}, field("task", "string", "The literal task or topic to route to a governing document."), field("includeCodeDiscovery", "boolean", "Also include code-based discovery hints (implies full mode)."), field("workspace", "string", "Optional workspace alias or path; auto-detected from cwd when omitted.")),
		tool("nav_pack", "Build a multi-document reading pack for a task across the governed wiki in one call, optionally anchored to a known --doc/--fl/--rf id. Prefer this over reading several wiki docs one by one with sequential Read calls when the task needs more than one governing document.", []string{"task"}, field("task", "string", "The task the reading pack should cover."), field("doc", "string", "Document path anchor to harden pack selection."), field("fl", "string", "Flow anchor (FL-*) to harden pack selection."), field("rf", "string", "Requirement anchor (RF-*) to harden pack selection."), field("workspace", "string", "Optional workspace alias or path; auto-detected from cwd when omitted.")),
		tool("nav_wiki", "Literal wiki operations: search by layer (RS/RF/FL/TP/CT/TECH/DB), route, build a pack, trace a spec doc to code, or list inventory/map/root. Use when the request is explicitly about the wiki as a document set (not an open goal — use nav_intent for that, and nav_route/nav_pack directly for a single task).", []string{"op"}, fieldEnum("op", []string{"search", "route", "pack", "trace", "inventory", "map", "root"}, "Which wiki operation to run."), field("query", "string", "Query/task/doc-id, required for search, route, pack and trace (unless op=trace with all=true)."), field("layer", "string", "search only: comma-separated layers, e.g. RS,RF,FL,TP,CT,TECH,DB."), field("top", "integer", "search only: maximum number of wiki docs to return."), field("includeContent", "boolean", "search only: include markdown content for each candidate."), field("all", "boolean", "trace only: trace all RFs instead of a single doc id."), field("summary", "boolean", "trace only: summary table format (with all=true)."), field("doc", "string", "pack only: document path anchor."), field("fl", "string", "pack only: flow anchor."), field("rf", "string", "pack only: requirement anchor."), field("role", "string", "root only: filter by role (producto, ecosistema, gobierno_local)."), field("withLayerCounts", "boolean", "inventory only: include doc counts per layer."), field("allWorkspaces", "boolean", "search/route/pack/trace/inventory: operate across all registered workspaces."), field("workspace", "string", "Optional workspace alias or path; auto-detected from cwd when omitted.")),
		tool("nav_search", "Exact literal full-text search for a known string/token/error code across the indexed workspace, with symbol-aware context. Prefer this over a manual Bash `rg`/`grep`/`Select-String` call or a Grep tool loop: same file:line results, but through the shared index and with optional surrounding code content in one call.", []string{"pattern"}, field("pattern", "string", "Literal text (or regex if regex=true) to search for."), field("regex", "boolean", "Interpret pattern as a regular expression."), field("includeContent", "boolean", "Include code content around each match."), field("contextLines", "integer", "Context lines for line-based fallback (default 20)."), fieldEnum("contextMode", []string{"hybrid", "symbol", "lines"}, "Content mode (default hybrid)."), field("allWorkspaces", "boolean", "Search across all registered workspaces."), field("workspace", "string", "Optional workspace alias or path; auto-detected from cwd when omitted.")),
		tool("nav_find", "Find symbols (functions/classes/types) by name across the workspace catalog in one call. Prefer this over Glob+Grep when the need is 'where is symbol X defined' and you do not yet need callers/usages (use nav_related once you do).", []string{"pattern"}, field("pattern", "string", "Symbol name or fragment to look up."), field("kind", "string", "Optional symbol kind filter."), field("exact", "boolean", "Require exact symbol name match."), field("offset", "integer", "Skip first N results, for pagination."), field("allWorkspaces", "boolean", "Search across all registered workspaces."), field("workspace", "string", "Optional workspace alias or path; auto-detected from cwd when omitted.")),
		tool("nav_refs", "Find semantic references (usages) of a known symbol in one call. Prefer this over grepping the symbol name across the repo by hand — it resolves through the language backend (roslyn/tsserver/pyright/gopls) rather than plain text matching.", []string{"symbol"}, field("symbol", "string", "Symbol name to find references for."), field("file", "string", "Anchor file for backends that resolve by position."), field("line", "integer", "Anchor line for backends that resolve by position."), field("entrypoint", "string", "Semantic entrypoint ID or path."), field("solution", "string", "Explicit solution path override."), field("project", "string", "Explicit project path override."), field("workspace", "string", "Optional workspace alias or path; auto-detected from cwd when omitted.")),
		tool("nav_related", "One call for a named symbol's full neighborhood: definition + callers + implementors + tests. Prefer this over a manual chain of nav_find -> nav_refs -> more Reads whenever the need centers on one named symbol and you care who calls/implements/tests it, not just where it is defined.", []string{"symbol"}, field("symbol", "string", "Symbol name to expand."), field("depth", "string", "Comma-separated neighborhoods: definition,callers,implementors,tests (default: all)."), field("entrypoint", "string", "Semantic entrypoint ID or path."), field("solution", "string", "Explicit solution path override."), field("project", "string", "Explicit project path override."), field("workspace", "string", "Optional workspace alias or path; auto-detected from cwd when omitted.")),
		tool("nav_flow_slice", "One-shot packet for a behavior that crosses several files: the path between endpoints, its neighborhood, ranked read_first anchors, and a batched continuation. Prefer this over a sequential Read loop across modules when you are following a cross-file flow/process rather than a single symbol (use nav_related for a single symbol).", nil, field("from", "string", "Flow start selector."), field("to", "string", "Flow end selector."), field("selector", "string", "Primary symbol when path endpoints are unknown."), field("limit", "integer", "Maximum ranked anchors (default 8)."), field("workspace", "string", "Optional workspace alias or path; auto-detected from cwd when omitted.")),
		tool("nav_change_pack", "One-shot packet for a diff/change's blast radius: changed paths/symbols, ranked affected surfaces, hub risk, and must-read wiki docs. Prefer this over `git diff` followed by several manual grep/Read calls when you need to understand what a change touches (use nav_affected for the narrower git-aware impact heuristic alone).", nil, field("ref", "string", "Git ref used as the change base."), fieldArray("paths", "Explicit changed paths (used when there is no ref, or to narrow it)."), field("limit", "integer", "Maximum ranked items (default 12)."), field("workspace", "string", "Optional workspace alias or path; auto-detected from cwd when omitted.")),
		tool("nav_affected", "Narrower git-aware impact selector than nav_change_pack: conservative heuristic impact with confidence scores for a set of changed paths. Prefer this when you specifically want the affected-files heuristic (optionally with suggested tests/docs), not the full change packet.", nil, fieldArray("paths", "Explicit changed paths."), field("fromGitDiff", "boolean", "Read changed paths from git diff in addition to explicit paths."), field("changedRef", "string", "Git ref used as the diff base (default HEAD)."), fieldEnum("mode", []string{"direct", "transitive"}, "Graph impact mode (default direct)."), field("includeTests", "boolean", "Suggest focused test commands for affected paths."), field("includeDocs", "boolean", "Suggest canonical docs likely affected by changed paths."), field("limit", "integer", "Maximum impact items."), field("workspace", "string", "Optional workspace alias or path; auto-detected from cwd when omitted.")),
		tool("nav_multi_read", "Batch-read several known file:start-end spans in one call. Prefer this over N sequential Read calls once you already know the exact files and ranges — typically the read_first anchors from a prior nav_related or nav_flow_slice result.", []string{"ranges"}, fieldArray("ranges", "file:start-end spans, e.g. 'src/foo.ts:10-42'."), field("workspace", "string", "Optional workspace alias or path; auto-detected from cwd when omitted.")),
		tool("nav_overview", "High-level map of a directory prefix (workspaceMap=false, default) or of the whole workspace's services/endpoints/events/deps map (workspaceMap=true). Prefer this over browsing a directory tree by hand or reading many top-level files just to get oriented in an unfamiliar codebase.", nil, field("dir", "string", "Directory prefix to summarize; omit for the workspace root."), field("workspaceMap", "boolean", "true runs the whole-workspace services/endpoints/events map instead of a directory overview."), field("offset", "integer", "Directory overview only: skip first N results."), field("workspace", "string", "Optional workspace alias or path; auto-detected from cwd when omitted.")),
	}
}

func tool(name, description string, required []string, fields ...map[string]any) mcpTool {
	properties := make(map[string]any, len(fields))
	for _, property := range fields {
		for key, value := range property {
			properties[key] = value
		}
	}
	schema := map[string]any{"type": "object", "properties": properties}
	if len(required) != 0 {
		schema["required"] = required
	}
	return mcpTool{Name: name, Description: description, InputSchema: schema}
}

func field(name, kind, description string) map[string]any {
	return map[string]any{name: map[string]any{"type": kind, "description": description}}
}

func fieldEnum(name string, values []string, description string) map[string]any {
	return map[string]any{name: map[string]any{"type": "string", "description": description, "enum": values}}
}

func fieldArray(name, description string) map[string]any {
	return map[string]any{name: map[string]any{"type": "array", "description": description, "items": map[string]any{"type": "string"}}}
}
