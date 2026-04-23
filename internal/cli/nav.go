package cli

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

func newNavCommand(state *rootState) *cobra.Command {
	command := &cobra.Command{
		Use:   "nav",
		Short: "Navigate indexed catalogs and semantic backends",
		Long: `Query the indexed symbol catalog and semantic backends.
Includes text search, symbol lookup, outline, references,
context retrieval, dependency analysis, and service exploration.`,
	}

	symbolsCommand := &cobra.Command{
		Use:   "symbols <file>",
		Short: "List symbols for a file from the catalog",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireArgs(args, 1, "file"); err != nil {
				return err
			}
			payload := map[string]any{"file": args[0]}
			offset, _ := cmd.Flags().GetInt("offset")
			if offset > 0 {
				payload["offset"] = offset
			}
			return state.executeOperation(cmd, "nav.symbols", payload, true)
		},
	}
	symbolsCommand.Flags().Int("offset", 0, "Skip first N results (for pagination)")

	var kind string
	var exact bool
	var allWorkspacesFind bool
	var findRepo string
	findCommand := &cobra.Command{
		Use:   "find <pattern>",
		Short: "Find symbols by name",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireArgs(args, 1, "pattern"); err != nil {
				return err
			}
			payload := map[string]any{"pattern": args[0], "kind": kind, "exact": exact}
			offset, _ := cmd.Flags().GetInt("offset")
			if findRepo != "" {
				payload["repo"] = findRepo
			}
			if offset > 0 {
				payload["offset"] = offset
			}
			if allWorkspacesFind {
				payload["all_workspaces"] = true
			}
			return state.executeOperation(cmd, "nav.find", payload, true)
		},
	}
	findCommand.Flags().StringVar(&kind, "kind", "", "Optional symbol kind filter")
	findCommand.Flags().BoolVar(&exact, "exact", false, "Require exact symbol name match")
	findCommand.Flags().Int("offset", 0, "Skip first N results (for pagination)")
	findCommand.Flags().BoolVar(&allWorkspacesFind, "all-workspaces", false, "Search across all registered workspaces")
	attachCatalogRepoFlag(findCommand, &findRepo)

	var refsFile string
	var refsLine int
	var refsRepo string
	var refsEntrypoint string
	var refsSolution string
	var refsProject string
	refsCommand := &cobra.Command{
		Use:   "refs <symbol>",
		Short: "Find references via the semantic backend",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireArgs(args, 1, "symbol"); err != nil {
				return err
			}
			payload := semanticPayload(refsRepo, refsEntrypoint, refsSolution, refsProject)
			payload["symbol"] = args[0]
			if refsFile != "" {
				payload["file"] = refsFile
			}
			if refsLine > 0 {
				payload["line"] = refsLine
			}
			return state.executeOperation(cmd, "nav.refs", payload, true)
		},
	}
	refsCommand.Flags().StringVar(&refsFile, "file", "", "Anchor file for backends that resolve references by position")
	refsCommand.Flags().IntVar(&refsLine, "line", 0, "Anchor line for backends that resolve references by position")
	attachSemanticSelectorFlags(refsCommand, &refsRepo, &refsEntrypoint, &refsSolution, &refsProject)

	overviewCommand := &cobra.Command{
		Use:   "overview [dir]",
		Short: "Summarize the catalog for a directory prefix",
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := "."
			if len(args) > 0 {
				dir = args[0]
			}
			payload := map[string]any{"dir": dir}
			offset, _ := cmd.Flags().GetInt("offset")
			if offset > 0 {
				payload["offset"] = offset
			}
			return state.executeOperation(cmd, "nav.overview", payload, true)
		},
	}
	overviewCommand.Flags().Int("offset", 0, "Skip first N results (for pagination)")

	outlineCommand := &cobra.Command{
		Use:   "outline <file>",
		Short: "Alias of nav symbols for hierarchical use",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireArgs(args, 1, "file"); err != nil {
				return err
			}
			return state.executeOperation(cmd, "nav.outline", map[string]any{"file": args[0]}, true)
		},
	}

	var allWorkspacesAsk bool
	var askRepo string
	askCommand := &cobra.Command{
		Use:   "ask <question>",
		Short: "Ask a docs-first question across wiki and code evidence",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireArgs(args, 1, "question"); err != nil {
				return err
			}
			question := strings.Join(args, " ")
			payload := map[string]any{"question": question}
			if askRepo != "" {
				payload["repo"] = askRepo
			}
			if allWorkspacesAsk {
				payload["all_workspaces"] = true
			}
			return state.executeOperation(cmd, "nav.ask", payload, true)
		},
	}
	askCommand.Flags().BoolVar(&allWorkspacesAsk, "all-workspaces", false, "Search docs across all registered workspaces")
	attachWikiCompatRepoFlag(askCommand, &askRepo)

	var packRF string
	var packFL string
	var packDoc string
	var packRepo string
	packCommand := &cobra.Command{
		Use:   "pack <task>",
		Short: "Build a canonical reading pack for a task across the wiki",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireArgs(args, 1, "task"); err != nil {
				return err
			}
			task := strings.Join(args, " ")
			payload := map[string]any{"task": task}
			if packRF != "" {
				payload["rf"] = packRF
			}
			if packFL != "" {
				payload["fl"] = packFL
			}
			if packDoc != "" {
				payload["doc"] = packDoc
			}
			if packRepo != "" {
				payload["repo"] = packRepo
			}
			return state.executeOperation(cmd, "nav.pack", payload, true)
		},
	}
	packCommand.Flags().StringVar(&packRF, "rf", "", "Requirement anchor to harden pack selection")
	packCommand.Flags().StringVar(&packFL, "fl", "", "Flow anchor to harden pack selection")
	packCommand.Flags().StringVar(&packDoc, "doc", "", "Document path anchor to harden pack selection")
	attachWikiCompatRepoFlag(packCommand, &packRepo)

	var routeIncludeCodeDiscovery bool
	var routeRepo string
	routeCommand := &cobra.Command{
		Use:   "route <task>",
		Short: "Resolve the canonical document route for a spec-driven task (RF-QRY-014)",
		Long: `Resolve the lowest-token canonical document route for a task.

Tier 1: reads the governance profile to produce a canonical anchor doc and
preview pack without touching the index. Tier 2 (when available) enriches
from the indexed docs.

Use --full to include discovery advisory and expanded route details.
Use --include-code-discovery to add code-based discovery hints.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireArgs(args, 1, "task"); err != nil {
				return err
			}
			task := strings.Join(args, " ")
			payload := map[string]any{"task": task}
			if routeIncludeCodeDiscovery {
				payload["include_code_discovery"] = true
			}
			if routeRepo != "" {
				payload["repo"] = routeRepo
			}
			return state.executeOperation(cmd, "nav.route", payload, true)
		},
	}
	routeCommand.Flags().BoolVar(&routeIncludeCodeDiscovery, "include-code-discovery", false, "Include code-based discovery hints (only in full mode)")
	attachWikiCompatRepoFlag(routeCommand, &routeRepo)

	governanceCommand := &cobra.Command{
		Use:   "governance",
		Short: "Explain the effective governance profile, sync status, and blockers",
		RunE: func(cmd *cobra.Command, args []string) error {
			return state.executeOperation(cmd, "nav.governance", map[string]any{}, true)
		},
	}

	var includeArchetype bool
	serviceCommand := &cobra.Command{
		Use:   "service <path>",
		Short: "Summarize the implementation surface of a service path",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireArgs(args, 1, "path"); err != nil {
				return err
			}
			return state.executeOperation(cmd, "nav.service", map[string]any{"path": args[0], "include_archetype": includeArchetype}, true)
		},
	}
	serviceCommand.Flags().BoolVar(&includeArchetype, "include-archetype", false, "Include known archetype placeholders in the summary")

	var useRegex bool
	var includeContent bool
	var contextLines int
	var contextMode string
	var allWorkspacesSearch bool
	var searchRepo string
	searchCommand := &cobra.Command{
		Use:   "search <text>",
		Short: "Full-text search with ripgrep",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireArgs(args, 1, "text"); err != nil {
				return err
			}
			pattern := strings.Join(args, " ")
			payload := map[string]any{"pattern": pattern, "regex": useRegex}
			if searchRepo != "" {
				payload["repo"] = searchRepo
			}
			if includeContent {
				payload["include_content"] = true
				payload["context_lines"] = contextLines
				payload["context_mode"] = contextMode
			}
			if allWorkspacesSearch {
				payload["all_workspaces"] = true
			}
			return state.executeOperation(cmd, "nav.search", payload, true)
		},
	}
	searchCommand.Flags().BoolVar(&useRegex, "regex", false, "Interpret the pattern as regex")
	searchCommand.Flags().BoolVar(&includeContent, "include-content", false, "Include code content around each match")
	searchCommand.Flags().IntVar(&contextLines, "context-lines", 20, "Number of context lines for line-based fallback")
	searchCommand.Flags().StringVar(&contextMode, "context-mode", "hybrid", "Content mode: hybrid, symbol, or lines")
	searchCommand.Flags().BoolVar(&allWorkspacesSearch, "all-workspaces", false, "Search across all registered workspaces")
	attachCatalogRepoFlag(searchCommand, &searchRepo)

	var contextRepo string
	var contextEntrypoint string
	var contextSolution string
	var contextProject string
	contextCommand := &cobra.Command{
		Use:   "context <file> <line>",
		Short: "Get semantic context for a source line",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireArgs(args, 2, "file and line"); err != nil {
				return err
			}
			lineNumber, err := strconv.Atoi(args[1])
			if err != nil {
				return err
			}
			payload := semanticPayload(contextRepo, contextEntrypoint, contextSolution, contextProject)
			payload["file"] = args[0]
			payload["line"] = lineNumber
			return state.executeOperation(cmd, "nav.context", payload, true)
		},
	}
	attachSemanticSelectorFlags(contextCommand, &contextRepo, &contextEntrypoint, &contextSolution, &contextProject)

	var depsRepo string
	var depsEntrypoint string
	var depsSolution string
	var depsProject string
	depsCommand := &cobra.Command{
		Use:   "deps [project]",
		Short: "Get semantic dependencies for a project or solution",
		RunE: func(cmd *cobra.Command, args []string) error {
			payload := semanticPayload(depsRepo, depsEntrypoint, depsSolution, depsProject)
			if len(args) > 0 {
				payload["project_hint"] = args[0]
			}
			return state.executeOperation(cmd, "nav.deps", payload, true)
		},
	}
	attachSemanticSelectorFlags(depsCommand, &depsRepo, &depsEntrypoint, &depsSolution, &depsProject)

	var multiReadStdin bool
	multiReadCommand := &cobra.Command{
		Use:   "multi-read [file:start-end ...]",
		Short: "Batch-read multiple file ranges in one call",
		Long: `Read multiple file ranges in a single invocation.
Each argument should be in format: file:startLine-endLine
Example: mi-lsp nav multi-read src/main.go:1-50 src/handler.go:100-200
Use --stdin to read JSON array of ranges from stdin.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			payload := map[string]any{}
			if multiReadStdin {
				payload["stdin"] = true
			}
			if len(args) > 0 {
				argSlice := make([]any, len(args))
				for i, arg := range args {
					argSlice[i] = arg
				}
				payload["args"] = argSlice
			}
			return state.executeOperation(cmd, "nav.multi-read", payload, true)
		},
	}
	multiReadCommand.Flags().BoolVar(&multiReadStdin, "stdin", false, "Read file ranges as JSON array from stdin")

	var batchSequential bool
	batchCommand := &cobra.Command{
		Use:   "batch",
		Short: "Execute multiple operations in one call via stdin JSON",
		Long: `Execute multiple nav operations in a single process invocation.
Read a JSON array of operations from stdin.
Each operation: {"id":"...", "op":"nav.search", "params":{...}}
Results are returned as an array of envelopes.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			data, err := io.ReadAll(os.Stdin)
			if err != nil {
				return fmt.Errorf("reading stdin: %w", err)
			}
			payload := map[string]any{
				"operations": string(data),
				"sequential": batchSequential,
			}
			return state.executeOperation(cmd, "nav.batch", payload, true)
		},
	}
	batchCommand.Flags().BoolVar(&batchSequential, "sequential", false, "Execute operations sequentially instead of in parallel")

	var relatedDepthFlag string
	var relatedRepo string
	var relatedEntrypoint string
	var relatedSolution string
	var relatedProject string
	relatedCommand := &cobra.Command{
		Use:   "related <symbol>",
		Short: "Show a symbol's neighborhood: definition, callers, implementors, tests",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireArgs(args, 1, "symbol"); err != nil {
				return err
			}
			payload := semanticPayload(relatedRepo, relatedEntrypoint, relatedSolution, relatedProject)
			payload["symbol"] = args[0]
			if relatedDepthFlag != "" {
				payload["depth"] = relatedDepthFlag
			}
			return state.executeOperation(cmd, "nav.related", payload, true)
		},
	}
	relatedCommand.Flags().StringVar(&relatedDepthFlag, "depth", "", "Comma-separated neighborhoods: definition,callers,implementors,tests (default: all)")
	attachSemanticSelectorFlags(relatedCommand, &relatedRepo, &relatedEntrypoint, &relatedSolution, &relatedProject)

	workspaceMapCommand := &cobra.Command{
		Use:   "workspace-map",
		Short: "High-level map of services, endpoints, events, and dependencies",
		RunE: func(cmd *cobra.Command, args []string) error {
			return state.executeOperation(cmd, "nav.workspace-map", map[string]any{}, true)
		},
	}

	var diffIncludeContent bool
	diffContextCommand := &cobra.Command{
		Use:   "diff-context [ref]",
		Short: "Semantic context of changed symbols in a git diff",
		Long: `Show changed symbols and their semantic context from a git diff.
Compares the working tree against a git ref (default: HEAD).
Use --include-content to embed symbol bodies in the output.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ref := ""
			if len(args) > 0 {
				ref = args[0]
			}
			payload := map[string]any{"ref": ref, "include_content": diffIncludeContent}
			return state.executeOperation(cmd, "nav.diff-context", payload, true)
		},
	}
	diffContextCommand.Flags().BoolVar(&diffIncludeContent, "include-content", false, "Include changed symbol bodies in output")

	var traceAll bool
	var traceSummary bool
	traceCommand := &cobra.Command{
		Use:   "trace [RF-ID]",
		Short: "Trace spec-to-code links for RF requirements",
		Long: `Analyze implementation status of functional requirements (RF).
Uses explicit markers from wiki frontmatter and heuristic inference.
Use --all for all RFs, --summary for tabular view.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			payload := map[string]any{}
			if len(args) > 0 {
				payload["rf"] = args[0]
			}
			if traceAll {
				payload["all"] = true
			}
			if traceSummary {
				payload["summary"] = true
			}
			if len(args) == 0 && !traceAll {
				return fmt.Errorf("rf ID required or use --all")
			}
			return state.executeOperation(cmd, "nav.trace", payload, true)
		},
	}
	traceCommand.Flags().BoolVar(&traceAll, "all", false, "Trace all RFs")
	traceCommand.Flags().BoolVar(&traceSummary, "summary", false, "Summary table format (with --all)")

	var intentTop int
	var intentRepo string
	intentCommand := &cobra.Command{
		Use:   "intent <question>",
		Short: "Resolve intent in docs-or-code mode",
		Long: `Perform intent-based navigation in hybrid docs|code mode.
Capability-like questions route to owner-aware canonical docs.
Symbol-like questions keep BM25 ranking over enriched symbol metadata
including names, signatures, documentation comments, and file paths.

Examples:
  mi-lsp nav intent "how do continuation and memory pointer work?"
  mi-lsp nav intent "where do we handle workspace routing fallback?" --repo backend
  mi-lsp nav intent "error handling daemon" --top 20`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireArgs(args, 1, "question"); err != nil {
				return err
			}
			offset, _ := cmd.Flags().GetInt("offset")
			question := strings.Join(args, " ")
			payload := map[string]any{
				"question": question,
				"top":      intentTop,
			}
			if offset > 0 {
				payload["offset"] = offset
			}
			if intentRepo != "" {
				payload["repo"] = intentRepo
			}
			return state.executeOperation(cmd, "nav.intent", payload, true)
		},
	}
	intentCommand.Flags().IntVar(&intentTop, "top", 10, "Maximum number of results")
	intentCommand.Flags().Int("offset", 0, "Skip first N results (for pagination)")
	attachCatalogRepoFlag(intentCommand, &intentRepo)

	wikiCommand := newNavWikiCommand(state)

	command.AddCommand(symbolsCommand, findCommand, refsCommand, overviewCommand, outlineCommand, askCommand, packCommand, routeCommand, wikiCommand, governanceCommand, serviceCommand, searchCommand, contextCommand, depsCommand, multiReadCommand, batchCommand, relatedCommand, workspaceMapCommand, diffContextCommand, traceCommand, intentCommand)
	return command
}

func attachCatalogRepoFlag(command *cobra.Command, repo *string) {
	command.Flags().StringVar(repo, "repo", "", "Repo child selector for container workspaces")
}

func attachWikiCompatRepoFlag(command *cobra.Command, repo *string) {
	command.Flags().StringVar(repo, "repo", "", "Compatibility only: ignored for wiki/docs routing; use nav wiki search|route|pack")
}

func attachSemanticSelectorFlags(command *cobra.Command, repo *string, entrypoint *string, solution *string, project *string) {
	command.Flags().StringVar(repo, "repo", "", "Repo child selector for container workspaces")
	command.Flags().StringVar(entrypoint, "entrypoint", "", "Semantic entrypoint ID or path")
	command.Flags().StringVar(solution, "solution", "", "Explicit solution path override")
	command.Flags().StringVar(project, "project", "", "Explicit project path override")
}

func semanticPayload(repo string, entrypoint string, solution string, project string) map[string]any {
	payload := map[string]any{}
	if repo != "" {
		payload["repo"] = repo
	}
	if entrypoint != "" {
		payload["entrypoint"] = entrypoint
	}
	if solution != "" {
		payload["solution"] = solution
	}
	if project != "" {
		payload["project_path"] = project
	}
	return payload
}

func newNavWikiCommand(state *rootState) *cobra.Command {
	command := &cobra.Command{
		Use:   "wiki",
		Short: "Explore governed wiki documentation",
		Long: `Explore the governed wiki as a first-class documentation surface.

Use search to find RF/FL/TP/CT/TECH/DB docs, route for the canonical anchor,
pack for a reading pack, and trace for RF implementation links.`,
	}

	var searchLayer string
	var searchTop int
	var searchOffset int
	var searchIncludeContent bool
	searchCommand := &cobra.Command{
		Use:   "search <query>",
		Short: "Search governed wiki docs by query and layer",
		Example: `  mi-lsp nav wiki search "workflow masterformularios" --workspace idp --layer RF,FL,CT,TP --format toon
  mi-lsp nav wiki search "RF IDX" --workspace mi-lsp --include-content --format toon`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireArgs(args, 1, "query"); err != nil {
				return err
			}
			payload := map[string]any{"query": strings.Join(args, " ")}
			if searchLayer != "" {
				payload["layer"] = searchLayer
			}
			if searchTop > 0 {
				payload["top"] = searchTop
			}
			if searchOffset > 0 {
				payload["offset"] = searchOffset
			}
			if searchIncludeContent {
				payload["include_content"] = true
			}
			return state.executeOperation(cmd, "nav.wiki.search", payload, true)
		},
	}
	searchCommand.Flags().StringVar(&searchLayer, "layer", "", "Comma-separated wiki layers: RF,FL,TP,CT,TECH,DB")
	searchCommand.Flags().IntVar(&searchTop, "top", 10, "Maximum number of wiki docs to return")
	searchCommand.Flags().IntVar(&searchOffset, "offset", 0, "Skip first N wiki docs")
	searchCommand.Flags().BoolVar(&searchIncludeContent, "include-content", false, "Include markdown content for each wiki candidate")

	routeCommand := &cobra.Command{
		Use:     "route <task>",
		Short:   "Resolve the canonical wiki route for a task",
		Example: `  mi-lsp nav wiki route "workflow con masterformularios" --workspace idp --format toon`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireArgs(args, 1, "task"); err != nil {
				return err
			}
			return state.executeOperation(cmd, "nav.route", map[string]any{"task": strings.Join(args, " ")}, true)
		},
	}

	var packRF string
	var packFL string
	var packDoc string
	packCommand := &cobra.Command{
		Use:   "pack <task>",
		Short: "Build a governed wiki reading pack for a task",
		Example: `  mi-lsp nav wiki pack "workflow con masterformularios" --workspace idp --format toon
  mi-lsp nav wiki pack "indexing docs" --workspace mi-lsp --rf RF-IDX-001 --format toon`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireArgs(args, 1, "task"); err != nil {
				return err
			}
			payload := map[string]any{"task": strings.Join(args, " ")}
			if packRF != "" {
				payload["rf"] = packRF
			}
			if packFL != "" {
				payload["fl"] = packFL
			}
			if packDoc != "" {
				payload["doc"] = packDoc
			}
			return state.executeOperation(cmd, "nav.pack", payload, true)
		},
	}
	packCommand.Flags().StringVar(&packRF, "rf", "", "Requirement anchor to harden pack selection")
	packCommand.Flags().StringVar(&packFL, "fl", "", "Flow anchor to harden pack selection")
	packCommand.Flags().StringVar(&packDoc, "doc", "", "Document path anchor to harden pack selection")

	var traceAll bool
	var traceSummary bool
	traceCommand := &cobra.Command{
		Use:   "trace <RF-ID|--all>",
		Short: "Trace wiki RF requirements to implementation links",
		RunE: func(cmd *cobra.Command, args []string) error {
			payload := map[string]any{}
			if len(args) > 0 {
				payload["rf"] = args[0]
			}
			if traceAll {
				payload["all"] = true
			}
			if traceSummary {
				payload["summary"] = true
			}
			if len(args) == 0 && !traceAll {
				return fmt.Errorf("rf ID required or use --all")
			}
			return state.executeOperation(cmd, "nav.trace", payload, true)
		},
	}
	traceCommand.Flags().BoolVar(&traceAll, "all", false, "Trace all RFs")
	traceCommand.Flags().BoolVar(&traceSummary, "summary", false, "Summary table format (with --all)")

	command.AddCommand(searchCommand, routeCommand, packCommand, traceCommand)
	return command
}
