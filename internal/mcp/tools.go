package mcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// ClientName is the product-neutral --client-name sent on every nav child.
const ClientName = "mi-lsp-mcp"

// ErrUnknownTool is returned by BuildArgv when the tool name is not one of
// the nav_* tools exposed by this server.
var ErrUnknownTool = errors.New("unknown tool")

type toolSpec struct {
	name        string
	description string
	schema      jsonSchema
}

type jsonSchema struct {
	Type       string                `json:"type"`
	Properties map[string]propSchema `json:"properties,omitempty"`
	Required   []string              `json:"required,omitempty"`
}

type propSchema struct {
	Type        string      `json:"type"`
	Description string      `json:"description,omitempty"`
	Enum        []string    `json:"enum,omitempty"`
	Items       *propSchema `json:"items,omitempty"`
}

// ToolInfo is one tools/list entry.
type ToolInfo struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

func ToolList() ([]ToolInfo, error) {
	out := make([]ToolInfo, len(toolCatalog))
	for i, spec := range toolCatalog {
		raw, err := json.Marshal(spec.schema)
		if err != nil {
			return nil, err
		}
		out[i] = ToolInfo{Name: spec.name, Description: spec.description, InputSchema: raw}
	}
	return out, nil
}

func pString(desc string) propSchema { return propSchema{Type: "string", Description: desc} }

func pInt(desc string) propSchema { return propSchema{Type: "integer", Description: desc} }

func pBool(desc string) propSchema { return propSchema{Type: "boolean", Description: desc} }

func pEnum(desc string, values ...string) propSchema {
	return propSchema{Type: "string", Description: desc, Enum: values}
}

func pArray(desc string) propSchema {
	return propSchema{Type: "array", Description: desc, Items: &propSchema{Type: "string"}}
}

func props(extra map[string]propSchema) map[string]propSchema {
	out := map[string]propSchema{
		"workspace": pString("Optional workspace alias or path; auto-detected from cwd when omitted."),
	}
	for key, value := range extra {
		out[key] = value
	}
	return out
}

func obj(required []string, properties map[string]propSchema) jsonSchema {
	return jsonSchema{Type: "object", Required: required, Properties: properties}
}

var toolCatalog = []toolSpec{
	{
		name: "nav_intent",
		description: "Resolve an open-ended goal in hybrid docs|code mode (BM25 over wiki + symbol metadata). " +
			"This is the DEFAULT first move for an exploratory question such as 'how does X work', 'what governs Y', " +
			"or 'where do we handle Z' when you do not yet know the exact file. Prefer this over starting a Read/Grep/Glob loop; " +
			"follow the returned continuation.next verbatim instead of guessing the next raw search.",
		schema: obj([]string{"question"}, props(map[string]propSchema{
			"question": pString("The open-ended question or goal, in natural language."),
			"top":      pInt("Maximum number of results (default 10)."),
			"offset":   pInt("Skip first N results, for pagination."),
			"repo":     pString("Repo child selector for container workspaces."),
		})),
	},
	{
		name: "nav_route",
		description: "Resolve the single canonical governing wiki document for a literal task/topic in one call. " +
			"Prefer this over grepping the wiki tree by hand, or over nav_intent, whenever the request is explicitly " +
			"'which doc governs/owns X' rather than an open exploratory question.",
		schema: obj([]string{"task"}, props(map[string]propSchema{
			"task":                 pString("The literal task or topic to route to a governing document."),
			"includeCodeDiscovery": pBool("Also include code-based discovery hints (implies full mode)."),
		})),
	},
	{
		name: "nav_pack",
		description: "Build a multi-document reading pack for a task across the governed wiki in one call, " +
			"optionally anchored to a known --doc/--fl/--rf id. Prefer this over reading several wiki docs one by one " +
			"with sequential Read calls when the task needs more than one governing document.",
		schema: obj([]string{"task"}, props(map[string]propSchema{
			"task": pString("The task the reading pack should cover."),
			"doc":  pString("Document path anchor to harden pack selection."),
			"fl":   pString("Flow anchor (FL-*) to harden pack selection."),
			"rf":   pString("Requirement anchor (RF-*) to harden pack selection."),
		})),
	},
	{
		name: "nav_wiki",
		description: "Literal wiki operations: search by layer (RS/RF/FL/TP/CT/TECH/DB), route, build a pack, trace a spec doc to code, " +
			"or list inventory/map/root. Use when the request is explicitly about the wiki as a document set " +
			"(not an open goal — use nav_intent for that, and nav_route/nav_pack directly for a single task).",
		schema: obj([]string{"op"}, props(map[string]propSchema{
			"op":              pEnum("Which wiki subcommand to run.", "search", "route", "pack", "trace", "inventory", "map", "root"),
			"query":           pString("Query/task/doc-id, required for search, route, pack and trace (unless op=trace with all=true)."),
			"layer":           pString("search only: comma-separated layers, e.g. RS,RF,FL,TP,CT,TECH,DB."),
			"includeContent":  pBool("search only: include markdown content for each candidate."),
			"top":             pInt("search only: maximum number of wiki docs to return."),
			"allWorkspaces":   pBool("search/route/pack/trace/inventory: operate across all registered workspaces."),
			"doc":             pString("pack only: document path anchor."),
			"fl":              pString("pack only: flow anchor."),
			"rf":              pString("pack only: requirement anchor."),
			"all":             pBool("trace only: trace all RFs instead of a single doc id."),
			"summary":         pBool("trace only: summary table format (with all=true)."),
			"withLayerCounts": pBool("inventory only: include doc counts per layer."),
			"role":            pString("root only: filter by role (producto, ecosistema, gobierno_local)."),
		})),
	},
	{
		name: "nav_search",
		description: "Exact literal full-text search for a known string/token/error code across the indexed workspace, with symbol-aware context. " +
			"Prefer this over a manual Bash `rg`/`grep`/`Select-String` call or a Grep tool loop: same file:line results, " +
			"but through the shared index and with optional surrounding code content in one call.",
		schema: obj([]string{"pattern"}, props(map[string]propSchema{
			"pattern":        pString("Literal text (or regex if regex=true) to search for."),
			"includeContent": pBool("Include code content around each match."),
			"regex":          pBool("Interpret pattern as a regular expression."),
			"contextLines":   pInt("Context lines for line-based fallback (default 20)."),
			"contextMode":    pEnum("Content mode (default hybrid).", "hybrid", "symbol", "lines"),
			"allWorkspaces":  pBool("Search across all registered workspaces."),
		})),
	},
	{
		name: "nav_find",
		description: "Find symbols (functions/classes/types) by name across the workspace catalog in one call. " +
			"Prefer this over Glob+Grep when the need is 'where is symbol X defined' and you do not yet need callers/usages (use nav_related once you do).",
		schema: obj([]string{"pattern"}, props(map[string]propSchema{
			"pattern":       pString("Symbol name or fragment to look up."),
			"exact":         pBool("Require exact symbol name match."),
			"kind":          pString("Optional symbol kind filter."),
			"allWorkspaces": pBool("Search across all registered workspaces."),
			"offset":        pInt("Skip first N results, for pagination."),
		})),
	},
	{
		name: "nav_refs",
		description: "Find semantic references (usages) of a known symbol in one call. " +
			"Prefer this over grepping the symbol name across the repo by hand — it resolves through the language backend (roslyn/tsserver/pyright/gopls) rather than plain text matching.",
		schema: obj([]string{"symbol"}, props(map[string]propSchema{
			"symbol":     pString("Symbol name to find references for."),
			"file":       pString("Anchor file for backends that resolve by position."),
			"line":       pInt("Anchor line for backends that resolve by position."),
			"entrypoint": pString("Semantic entrypoint ID or path."),
			"project":    pString("Explicit project path override."),
			"solution":   pString("Explicit solution path override."),
		})),
	},
	{
		name: "nav_related",
		description: "One call for a named symbol's full neighborhood: definition + callers + implementors + tests. " +
			"Prefer this over a manual chain of nav_find -> nav_refs -> more Reads whenever the need centers on one named symbol " +
			"and you care who calls/implements/tests it, not just where it is defined.",
		schema: obj([]string{"symbol"}, props(map[string]propSchema{
			"symbol":     pString("Symbol name to expand."),
			"depth":      pString("Comma-separated neighborhoods: definition,callers,implementors,tests (default: all)."),
			"entrypoint": pString("Semantic entrypoint ID or path."),
			"project":    pString("Explicit project path override."),
			"solution":   pString("Explicit solution path override."),
		})),
	},
	{
		name: "nav_flow_slice",
		description: "One-shot packet for a behavior that crosses several files: the path between endpoints, its neighborhood, " +
			"ranked read_first anchors, and a batched continuation. Prefer this over a sequential Read loop across modules " +
			"when you are following a cross-file flow/process rather than a single symbol (use nav_related for a single symbol).",
		schema: obj(nil, props(map[string]propSchema{
			"from":     pString("Flow start selector."),
			"to":       pString("Flow end selector."),
			"selector": pString("Primary symbol when path endpoints are unknown."),
			"limit":    pInt("Maximum ranked anchors (default 8)."),
		})),
	},
	{
		name: "nav_change_pack",
		description: "One-shot packet for a diff/change's blast radius: changed paths/symbols, ranked affected surfaces, hub risk, and must-read wiki docs. " +
			"Prefer this over `git diff` followed by several manual grep/Read calls when you need to understand what a change touches " +
			"(use nav_affected for the narrower git-aware impact heuristic alone).",
		schema: obj(nil, props(map[string]propSchema{
			"ref":   pString("Git ref used as the change base."),
			"paths": pArray("Explicit changed paths (used when there is no ref, or to narrow it)."),
			"limit": pInt("Maximum ranked items (default 12)."),
		})),
	},
	{
		name: "nav_affected",
		description: "Narrower git-aware impact selector than nav_change_pack: conservative heuristic impact with confidence scores for a set of changed paths. " +
			"Prefer this when you specifically want the affected-files heuristic (optionally with suggested tests/docs), not the full change packet.",
		schema: obj(nil, props(map[string]propSchema{
			"paths":        pArray("Explicit changed paths."),
			"changedRef":   pString("Git ref used as the diff base (default HEAD)."),
			"fromGitDiff":  pBool("Read changed paths from git diff in addition to explicit paths."),
			"mode":         pEnum("Graph impact mode (default direct).", "direct", "transitive"),
			"includeTests": pBool("Suggest focused test commands for affected paths."),
			"includeDocs":  pBool("Suggest canonical docs likely affected by changed paths."),
			"limit":        pInt("Maximum impact items."),
		})),
	},
	{
		name: "nav_multi_read",
		description: "Batch-read several known file:start-end spans in one call. Prefer this over N sequential Read calls " +
			"once you already know the exact files and ranges — typically the read_first anchors from a prior nav_related or nav_flow_slice result.",
		schema: obj([]string{"ranges"}, props(map[string]propSchema{
			"ranges": pArray("file:start-end spans, e.g. 'src/foo.ts:10-42'."),
		})),
	},
	{
		name: "nav_overview",
		description: "High-level map of a directory prefix (workspaceMap=false, default) or of the whole workspace's services/endpoints/events/deps (workspaceMap=true). " +
			"Prefer this over browsing a directory tree by hand or reading many top-level files just to get oriented in unfamiliar code.",
		schema: obj(nil, props(map[string]propSchema{
			"dir":          pString("Directory prefix to summarize; omit for the workspace root."),
			"workspaceMap": pBool("true runs the whole-workspace services/endpoints/events map instead of a directory overview."),
			"offset":       pInt("directory overview only: skip first N results, for pagination."),
		})),
	},
}

// BuildArgv maps a tool call onto a mi-lsp argv vector. Values stay individual
// elements; this function never joins a shell command string.
func BuildArgv(name string, args map[string]any) ([]string, error) {
	if args == nil {
		args = map[string]any{}
	}
	switch name {
	case "nav_intent":
		return buildIntent(args)
	case "nav_route":
		return buildRoute(args)
	case "nav_pack":
		return buildPack(args)
	case "nav_wiki":
		return buildWiki(args)
	case "nav_search":
		return buildSearch(args)
	case "nav_find":
		return buildFind(args)
	case "nav_refs":
		return buildRefs(args)
	case "nav_related":
		return buildRelated(args)
	case "nav_flow_slice":
		return buildFlowSlice(args)
	case "nav_change_pack":
		return buildChangePack(args)
	case "nav_affected":
		return buildAffected(args)
	case "nav_multi_read":
		return buildMultiRead(args)
	case "nav_overview":
		return buildOverview(args)
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnknownTool, name)
	}
}

func buildIntent(args map[string]any) ([]string, error) {
	pos, err := onePos(args, "question")
	if err != nil {
		return nil, err
	}
	flags, err := flagsThenWorkspace(args,
		flagSpec{key: "top", kind: kindInt},
		flagSpec{key: "offset", kind: kindInt},
		flagSpec{key: "repo", kind: kindString},
	)
	if err != nil {
		return nil, err
	}
	return assemble([]string{"nav", "intent"}, pos, flags), nil
}

func buildRoute(args map[string]any) ([]string, error) {
	pos, err := onePos(args, "task")
	if err != nil {
		return nil, err
	}
	flags, err := workspaceThenFlags(args)
	if err != nil {
		return nil, err
	}
	include, err := boolArg(args, "includeCodeDiscovery")
	if err != nil {
		return nil, err
	}
	if include {
		flags = append(flags, kv{flag: "full", val: true}, kv{flag: "include-code-discovery", val: true})
	}
	return assemble([]string{"nav", "route"}, pos, flags), nil
}

func buildPack(args map[string]any) ([]string, error) {
	pos, err := onePos(args, "task")
	if err != nil {
		return nil, err
	}
	flags, err := flagsThenWorkspace(args,
		flagSpec{key: "doc", kind: kindString},
		flagSpec{key: "fl", kind: kindString},
		flagSpec{key: "rf", kind: kindString},
	)
	if err != nil {
		return nil, err
	}
	return assemble([]string{"nav", "pack"}, pos, flags), nil
}

func buildWiki(args map[string]any) ([]string, error) {
	op, err := wikiOp(args)
	if err != nil {
		return nil, err
	}
	switch op {
	case "search":
		pos, err := onePos(args, "query")
		if err != nil {
			return nil, err
		}
		flags, err := workspaceThenFlags(args,
			flagSpec{key: "layer", kind: kindString},
			flagSpec{key: "includeContent", flag: "include-content", kind: kindBool},
			flagSpec{key: "top", kind: kindInt},
			flagSpec{key: "allWorkspaces", flag: "all-workspaces", kind: kindBool},
		)
		if err != nil {
			return nil, err
		}
		return assemble([]string{"nav", "wiki", "search"}, pos, flags), nil
	case "route":
		pos, err := onePos(args, "query")
		if err != nil {
			return nil, err
		}
		flags, err := workspaceThenFlags(args,
			flagSpec{key: "allWorkspaces", flag: "all-workspaces", kind: kindBool},
		)
		if err != nil {
			return nil, err
		}
		return assemble([]string{"nav", "wiki", "route"}, pos, flags), nil
	case "pack":
		pos, err := onePos(args, "query")
		if err != nil {
			return nil, err
		}
		flags, err := workspaceThenFlags(args,
			flagSpec{key: "doc", kind: kindString},
			flagSpec{key: "fl", kind: kindString},
			flagSpec{key: "rf", kind: kindString},
			flagSpec{key: "allWorkspaces", flag: "all-workspaces", kind: kindBool},
		)
		if err != nil {
			return nil, err
		}
		return assemble([]string{"nav", "wiki", "pack"}, pos, flags), nil
	case "trace":
		all, err := boolArg(args, "all")
		if err != nil {
			return nil, err
		}
		var pos []string
		if !all {
			pos, err = onePos(args, "query")
			if err != nil {
				return nil, err
			}
		}
		flags, err := workspaceThenFlags(args,
			flagSpec{key: "all", kind: kindBool},
			flagSpec{key: "summary", kind: kindBool},
			flagSpec{key: "allWorkspaces", flag: "all-workspaces", kind: kindBool},
		)
		if err != nil {
			return nil, err
		}
		return assemble([]string{"nav", "wiki", "trace"}, pos, flags), nil
	case "inventory":
		flags, err := workspaceThenFlags(args,
			flagSpec{key: "withLayerCounts", flag: "with-layer-counts", kind: kindBool},
			flagSpec{key: "allWorkspaces", flag: "all-workspaces", kind: kindBool},
		)
		if err != nil {
			return nil, err
		}
		return assemble([]string{"nav", "wiki", "inventory"}, nil, flags), nil
	case "map":
		flags, err := workspaceThenFlags(args)
		if err != nil {
			return nil, err
		}
		return assemble([]string{"nav", "wiki", "map"}, nil, flags), nil
	case "root":
		flags, err := workspaceThenFlags(args, flagSpec{key: "role", kind: kindString})
		if err != nil {
			return nil, err
		}
		return assemble([]string{"nav", "wiki", "root"}, nil, flags), nil
	default:
		return nil, fmt.Errorf("unsupported nav_wiki op: %s", op)
	}
}

func buildSearch(args map[string]any) ([]string, error) {
	pos, err := onePos(args, "pattern")
	if err != nil {
		return nil, err
	}
	flags, err := flagsThenWorkspace(args,
		flagSpec{key: "includeContent", flag: "include-content", kind: kindBool},
		flagSpec{key: "regex", kind: kindBool},
		flagSpec{key: "contextLines", flag: "context-lines", kind: kindInt},
		flagSpec{key: "contextMode", flag: "context-mode", kind: kindString},
		flagSpec{key: "allWorkspaces", flag: "all-workspaces", kind: kindBool},
	)
	if err != nil {
		return nil, err
	}
	return assemble([]string{"nav", "search"}, pos, flags), nil
}

func buildFind(args map[string]any) ([]string, error) {
	pos, err := onePos(args, "pattern")
	if err != nil {
		return nil, err
	}
	flags, err := flagsThenWorkspace(args,
		flagSpec{key: "exact", kind: kindBool},
		flagSpec{key: "kind", kind: kindString},
		flagSpec{key: "allWorkspaces", flag: "all-workspaces", kind: kindBool},
		flagSpec{key: "offset", kind: kindInt},
	)
	if err != nil {
		return nil, err
	}
	return assemble([]string{"nav", "find"}, pos, flags), nil
}

func buildRefs(args map[string]any) ([]string, error) {
	pos, err := onePos(args, "symbol")
	if err != nil {
		return nil, err
	}
	flags, err := flagsThenWorkspace(args,
		flagSpec{key: "file", kind: kindString},
		flagSpec{key: "line", kind: kindInt},
		flagSpec{key: "entrypoint", kind: kindString},
		flagSpec{key: "project", kind: kindString},
		flagSpec{key: "solution", kind: kindString},
	)
	if err != nil {
		return nil, err
	}
	return assemble([]string{"nav", "refs"}, pos, flags), nil
}

func buildRelated(args map[string]any) ([]string, error) {
	pos, err := onePos(args, "symbol")
	if err != nil {
		return nil, err
	}
	flags, err := flagsThenWorkspace(args,
		flagSpec{key: "depth", kind: kindString},
		flagSpec{key: "entrypoint", kind: kindString},
		flagSpec{key: "project", kind: kindString},
		flagSpec{key: "solution", kind: kindString},
	)
	if err != nil {
		return nil, err
	}
	return assemble([]string{"nav", "related"}, pos, flags), nil
}

func buildFlowSlice(args map[string]any) ([]string, error) {
	flags, err := flagsThenWorkspace(args,
		flagSpec{key: "from", kind: kindString},
		flagSpec{key: "to", kind: kindString},
		flagSpec{key: "selector", kind: kindString},
		flagSpec{key: "limit", kind: kindInt},
	)
	if err != nil {
		return nil, err
	}
	return assemble([]string{"nav", "flow-slice"}, nil, flags), nil
}

func buildChangePack(args map[string]any) ([]string, error) {
	pos, err := onePos(args, "ref")
	if err != nil {
		return nil, err
	}
	flags, err := flagsThenWorkspace(args,
		flagSpec{key: "paths", flag: "path", kind: kindStrings},
		flagSpec{key: "limit", kind: kindInt},
	)
	if err != nil {
		return nil, err
	}
	return assemble([]string{"nav", "change-pack"}, pos, flags), nil
}

func buildAffected(args map[string]any) ([]string, error) {
	pos, err := listPos(args, "paths")
	if err != nil {
		return nil, err
	}
	flags, err := flagsThenWorkspace(args,
		flagSpec{key: "changedRef", flag: "changed-ref", kind: kindString},
		flagSpec{key: "fromGitDiff", flag: "from-git-diff", kind: kindBool},
		flagSpec{key: "mode", kind: kindString},
		flagSpec{key: "includeTests", flag: "include-tests", kind: kindBool},
		flagSpec{key: "includeDocs", flag: "include-docs", kind: kindBool},
		flagSpec{key: "limit", kind: kindInt},
	)
	if err != nil {
		return nil, err
	}
	return assemble([]string{"nav", "affected"}, pos, flags), nil
}

func buildMultiRead(args map[string]any) ([]string, error) {
	pos, err := listPos(args, "ranges")
	if err != nil {
		return nil, err
	}
	flags, err := flagsThenWorkspace(args)
	if err != nil {
		return nil, err
	}
	return assemble([]string{"nav", "multi-read"}, pos, flags), nil
}

func buildOverview(args map[string]any) ([]string, error) {
	asMap, err := boolArg(args, "workspaceMap")
	if err != nil {
		return nil, err
	}
	if asMap {
		flags, err := flagsThenWorkspace(args)
		if err != nil {
			return nil, err
		}
		return assemble([]string{"nav", "workspace-map"}, nil, flags), nil
	}
	pos, err := onePos(args, "dir")
	if err != nil {
		return nil, err
	}
	flags, err := flagsThenWorkspace(args, flagSpec{key: "offset", kind: kindInt})
	if err != nil {
		return nil, err
	}
	return assemble([]string{"nav", "overview"}, pos, flags), nil
}

func wikiOp(args map[string]any) (string, error) {
	raw, ok := args["op"]
	if !ok || raw == nil {
		return "", errors.New("unsupported nav_wiki op: <missing>")
	}
	op, ok := raw.(string)
	if !ok || op == "" || len(op) > 64 || strings.ContainsAny(op, "\r\n") {
		return "", errors.New("unsupported nav_wiki op")
	}
	switch op {
	case "search", "route", "pack", "trace", "inventory", "map", "root":
		return op, nil
	default:
		return "", fmt.Errorf("unsupported nav_wiki op: %s", op)
	}
}

type flagKind int

const (
	kindString flagKind = iota
	kindBool
	kindInt
	kindStrings
)

type flagSpec struct {
	key  string
	flag string
	kind flagKind
}

type kv struct {
	flag string
	val  any
}

func baseFlags() []kv {
	return []kv{
		{flag: "format", val: "toon"},
		{flag: "client-name", val: ClientName},
	}
}

func flagsThenWorkspace(args map[string]any, specs ...flagSpec) ([]kv, error) {
	flags, err := applyFlags(baseFlags(), args, specs)
	if err != nil {
		return nil, err
	}
	return addWorkspace(flags, args)
}

func workspaceThenFlags(args map[string]any, specs ...flagSpec) ([]kv, error) {
	flags, err := addWorkspace(baseFlags(), args)
	if err != nil {
		return nil, err
	}
	return applyFlags(flags, args, specs)
}

func applyFlags(flags []kv, args map[string]any, specs []flagSpec) ([]kv, error) {
	var err error
	for _, spec := range specs {
		name := spec.flag
		if name == "" {
			name = spec.key
		}
		switch spec.kind {
		case kindString:
			flags, err = addString(flags, args, spec.key, name)
		case kindBool:
			flags, err = addBool(flags, args, spec.key, name)
		case kindInt:
			flags, err = addInt(flags, args, spec.key, name)
		case kindStrings:
			flags, err = addStrings(flags, args, spec.key, name)
		default:
			err = errors.New("unknown flag kind")
		}
		if err != nil {
			return nil, err
		}
	}
	return flags, nil
}

func addWorkspace(flags []kv, args map[string]any) ([]kv, error) {
	return addString(flags, args, "workspace", "workspace")
}

func addString(flags []kv, args map[string]any, key, flag string) ([]kv, error) {
	value, ok := args[key]
	if !ok || value == nil {
		return flags, nil
	}
	text, ok := value.(string)
	if !ok {
		return nil, fmt.Errorf("%s must be a string", key)
	}
	if text == "" {
		return flags, nil
	}
	return append(flags, kv{flag: flag, val: text}), nil
}

func addBool(flags []kv, args map[string]any, key, flag string) ([]kv, error) {
	on, err := boolArg(args, key)
	if err != nil || !on {
		return flags, err
	}
	return append(flags, kv{flag: flag, val: true}), nil
}

func addInt(flags []kv, args map[string]any, key, flag string) ([]kv, error) {
	value, ok := args[key]
	if !ok || value == nil {
		return flags, nil
	}
	text, err := formatInt(value)
	if err != nil {
		return nil, fmt.Errorf("%s %w", key, err)
	}
	return append(flags, kv{flag: flag, val: text}), nil
}

func addStrings(flags []kv, args map[string]any, key, flag string) ([]kv, error) {
	value, ok := args[key]
	if !ok || value == nil {
		return flags, nil
	}
	items, err := stringList(value, key)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return flags, nil
	}
	return append(flags, kv{flag: flag, val: items}), nil
}

func boolArg(args map[string]any, key string) (bool, error) {
	value, ok := args[key]
	if !ok || value == nil {
		return false, nil
	}
	parsed, ok := value.(bool)
	if !ok {
		return false, fmt.Errorf("%s must be a boolean", key)
	}
	return parsed, nil
}

func onePos(args map[string]any, key string) ([]string, error) {
	value, ok := args[key]
	if !ok || value == nil {
		return nil, nil
	}
	text, ok := value.(string)
	if !ok {
		return nil, fmt.Errorf("%s must be a string", key)
	}
	if text == "" {
		return nil, nil
	}
	return []string{text}, nil
}

func listPos(args map[string]any, key string) ([]string, error) {
	value, ok := args[key]
	if !ok || value == nil {
		return nil, nil
	}
	return stringList(value, key)
}

func stringList(value any, key string) ([]string, error) {
	switch items := value.(type) {
	case []string:
		out := make([]string, 0, len(items))
		for _, item := range items {
			if item != "" {
				out = append(out, item)
			}
		}
		return out, nil
	case []any:
		out := make([]string, 0, len(items))
		for _, item := range items {
			text, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("%s must be an array of strings", key)
			}
			if text != "" {
				out = append(out, text)
			}
		}
		return out, nil
	default:
		return nil, fmt.Errorf("%s must be an array of strings", key)
	}
}

func formatInt(value any) (string, error) {
	switch n := value.(type) {
	case json.Number:
		if parsed, err := n.Int64(); err == nil {
			return strconv.FormatInt(parsed, 10), nil
		}
		parsed, err := n.Float64()
		if err != nil || !wholeInt(parsed) {
			return "", errors.New("must be an integer")
		}
		return strconv.FormatInt(int64(parsed), 10), nil
	case float64:
		if !wholeInt(n) {
			return "", errors.New("must be an integer")
		}
		return strconv.FormatInt(int64(n), 10), nil
	case int:
		return strconv.Itoa(n), nil
	case int64:
		return strconv.FormatInt(n, 10), nil
	default:
		return "", errors.New("must be an integer")
	}
}

func wholeInt(n float64) bool {
	return !math.IsNaN(n) && !math.IsInf(n, 0) && n == math.Trunc(n) && n <= math.MaxInt64 && n >= math.MinInt64
}

// assemble appends subcommands, positionals, and flags as separate argv elements.
func assemble(sub []string, positional []string, flags []kv) []string {
	argv := make([]string, 0, len(sub)+len(positional)+len(flags)*2)
	argv = append(argv, sub...)
	for _, arg := range positional {
		if arg != "" {
			argv = append(argv, arg)
		}
	}
	for _, flag := range flags {
		switch value := flag.val.(type) {
		case bool:
			if value {
				argv = append(argv, "--"+flag.flag)
			}
		case []string:
			for _, item := range value {
				if item == "" {
					continue
				}
				argv = append(argv, "--"+flag.flag, item)
			}
		case string:
			if value == "" {
				continue
			}
			argv = append(argv, "--"+flag.flag, value)
		}
	}
	return argv
}
