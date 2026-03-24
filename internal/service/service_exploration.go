package service

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
)

const (
	serviceCatalogLimit = 500
	serviceSearchLimit  = 200
)

var (
	endpointPattern  = regexp.MustCompile(`Map(Get|Post|Put|Delete|Patch)\s*\(\s*"([^"]+)"`)
	consumerPattern  = regexp.MustCompile(`IConsumer<\s*([A-Za-z0-9_\.]+)\s*>`)
	publisherPattern = regexp.MustCompile(`Publish(?:Async)?<\s*([A-Za-z0-9_\.]+)\s*>`)
)

func (a *App) serviceSummary(ctx context.Context, request model.CommandRequest) (model.Envelope, error) {
	registration, project, err := a.resolveWorkspaceWithProject(request.Context.Workspace)
	if err != nil {
		return model.Envelope{}, err
	}

	rawPath, _ := request.Payload["path"].(string)
	if strings.TrimSpace(rawPath) == "" {
		return model.Envelope{}, fmt.Errorf("service path is required")
	}
	includeArchetype, _ := request.Payload["include_archetype"].(bool)

	relativePath, err := makeRelative(registration.Root, rawPath)
	if err != nil {
		return model.Envelope{}, err
	}
	if relativePath == ".." || strings.HasPrefix(relativePath, "../") {
		return model.Envelope{}, fmt.Errorf("service path must be inside the workspace root")
	}
	if relativePath == "." {
		relativePath = ""
	}
	absolutePath := registration.Root
	if relativePath != "" {
		absolutePath = filepath.Join(registration.Root, filepath.FromSlash(relativePath))
	}

	warnings := []string{}
	sources := []string{}
	summary := model.ServiceSurfaceSummary{
		Service:          serviceNameFromPath(relativePath, registration.Root),
		Path:             normalizedServicePath(relativePath),
		Sources:          []string{},
		Symbols:          map[string]int{},
		HTTPEndpoints:    []map[string]any{},
		EventConsumers:   []map[string]any{},
		EventPublishers:  []map[string]any{},
		Entities:         []map[string]any{},
		Infrastructure:   map[string]any{},
		ArchetypeMatches: []string{},
		NextQueries:      []string{},
	}

	db, err := store.Open(registration.Root)
	if err == nil {
		defer db.Close()
		catalogSymbols, catalogErr := store.OverviewByPrefix(ctx, db, normalizedPrefix(relativePath), serviceCatalogLimit)
		if catalogErr != nil {
			warnings = append(warnings, fmt.Sprintf("catalog lookup failed: %v", catalogErr))
		} else {
			if len(catalogSymbols) > 0 {
				sources = appendUnique(sources, "catalog")
			} else {
				warnings = append(warnings, "catalog has no symbols under service path; run mi-lsp index --workspace <alias> for richer service summaries")
			}
			fillServiceSummaryFromCatalog(&summary, catalogSymbols, includeArchetype)
		}
	} else {
		warnings = append(warnings, fmt.Sprintf("catalog unavailable: %v", err))
	}

	endpointMatches, endpointErr := searchPatternScoped(ctx, registration.Root, absolutePath, project, `Map(Get|Post|Put|Delete|Patch)\s*\(`, true, serviceSearchLimit)
	if endpointErr != nil {
		warnings = append(warnings, fmt.Sprintf("endpoint search failed: %v", endpointErr))
	}
	consumerMatches, consumerErr := searchPatternScoped(ctx, registration.Root, absolutePath, project, `IConsumer<`, true, serviceSearchLimit)
	if consumerErr != nil {
		warnings = append(warnings, fmt.Sprintf("consumer search failed: %v", consumerErr))
	}
	publisherMatches, publisherErr := searchPatternScoped(ctx, registration.Root, absolutePath, project, `Publish(?:Async)?<|IPublishEndpoint`, true, serviceSearchLimit)
	if publisherErr != nil {
		warnings = append(warnings, fmt.Sprintf("publisher search failed: %v", publisherErr))
	}
	infrastructureMatches, infraErr := searchPatternScoped(ctx, registration.Root, absolutePath, project, `AddSetupEventBus|UseNpgsql|UseSqlServer|UseInMemoryDatabase|AddStackExchangeRedis`, true, serviceSearchLimit)
	if infraErr != nil {
		warnings = append(warnings, fmt.Sprintf("infrastructure search failed: %v", infraErr))
	}

	if len(endpointMatches) > 0 || len(consumerMatches) > 0 || len(publisherMatches) > 0 || len(infrastructureMatches) > 0 {
		sources = appendUnique(sources, "text")
	}

	summary.HTTPEndpoints = parseEndpointMatches(endpointMatches)
	summary.EventConsumers = parseConsumerMatches(consumerMatches, includeArchetype, &summary.ArchetypeMatches)
	summary.EventPublishers = parsePublisherMatches(publisherMatches)
	summary.Infrastructure = detectInfrastructure(infrastructureMatches)
	summary.Profile = detectServiceProfile(summary, registration)
	summary.Sources = sources
	summary.NextQueries = buildNextQueries(registration.Name, summary.Path, summary)

	if len(summary.HTTPEndpoints) == 0 && len(summary.EventConsumers) == 0 && len(summary.EventPublishers) == 0 && len(summary.Entities) == 0 && len(summary.Symbols) == 0 {
		warnings = append(warnings, "no service evidence found under path; verify the service path or index state")
	}

	backend := strings.Join(sources, "+")
	if backend == "" {
		backend = "text"
	}

	return model.Envelope{
		Ok:        true,
		Workspace: registration.Name,
		Backend:   backend,
		Items:     []model.ServiceSurfaceSummary{summary},
		Warnings:  warnings,
		Stats: model.Stats{
			Symbols: totalSymbolCount(summary.Symbols),
			Files:   serviceFileCount(summary),
		},
	}, nil
}

func fillServiceSummaryFromCatalog(summary *model.ServiceSurfaceSummary, symbols []model.SymbolRecord, includeArchetype bool) {
	for _, symbol := range symbols {
		kind := strings.ToLower(strings.TrimSpace(symbol.Kind))
		if kind != "" {
			summary.Symbols[kind]++
		}
		if !isEntityFile(symbol.FilePath) {
			continue
		}
		if kind != "class" && kind != "record" {
			continue
		}
		if match, ok := archetypeMatchForName(symbol.Name, "entity"); ok {
			summary.ArchetypeMatches = appendUnique(summary.ArchetypeMatches, match)
			if !includeArchetype {
				continue
			}
		}
		summary.Entities = append(summary.Entities, map[string]any{
			"name": symbol.Name,
			"kind": symbol.Kind,
			"file": symbol.FilePath,
			"line": symbol.StartLine,
		})
	}
	sortMaps(summary.Entities)
}

func parseEndpointMatches(matches []map[string]any) []map[string]any {
	items := make([]map[string]any, 0, len(matches))
	for _, match := range matches {
		text, _ := match["text"].(string)
		parsed := endpointPattern.FindStringSubmatch(text)
		item := map[string]any{
			"file": match["file"],
			"line": match["line"],
			"text": strings.TrimSpace(text),
		}
		if len(parsed) == 3 {
			item["method"] = strings.ToUpper(parsed[1])
			item["route"] = parsed[2]
		}
		items = append(items, item)
	}
	sortMaps(items)
	return items
}

func parseConsumerMatches(matches []map[string]any, includeArchetype bool, archetypeMatches *[]string) []map[string]any {
	items := make([]map[string]any, 0, len(matches))
	for _, match := range matches {
		text, _ := match["text"].(string)
		parsed := consumerPattern.FindStringSubmatch(text)
		item := map[string]any{
			"file": match["file"],
			"line": match["line"],
			"text": strings.TrimSpace(text),
		}
		if len(parsed) == 2 {
			item["event"] = trimNamespace(parsed[1])
		}
		file, _ := match["file"].(string)
		consumerName := strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
		item["consumer"] = consumerName
		if matchValue, ok := archetypeMatchForName(consumerName, "consumer"); ok {
			*archetypeMatches = appendUnique(*archetypeMatches, matchValue)
			if !includeArchetype {
				continue
			}
		}
		items = append(items, item)
	}
	sortMaps(items)
	return items
}

func parsePublisherMatches(matches []map[string]any) []map[string]any {
	items := make([]map[string]any, 0, len(matches))
	for _, match := range matches {
		text, _ := match["text"].(string)
		parsed := publisherPattern.FindStringSubmatch(text)
		item := map[string]any{
			"file": match["file"],
			"line": match["line"],
			"text": strings.TrimSpace(text),
		}
		if len(parsed) == 2 {
			item["event"] = trimNamespace(parsed[1])
		}
		items = append(items, item)
	}
	sortMaps(items)
	return items
}

func detectInfrastructure(matches []map[string]any) map[string]any {
	infrastructure := map[string]any{}
	for _, match := range matches {
		text, _ := match["text"].(string)
		switch {
		case strings.Contains(text, "AddSetupEventBus"):
			infrastructure["event_bus"] = true
		case strings.Contains(text, "UseNpgsql"):
			infrastructure["database"] = "postgres"
		case strings.Contains(text, "UseSqlServer"):
			infrastructure["database"] = "sqlserver"
		case strings.Contains(text, "UseInMemoryDatabase"):
			infrastructure["database"] = "inmemory"
		case strings.Contains(text, "AddStackExchangeRedis"):
			infrastructure["redis"] = true
		}
	}
	return infrastructure
}

func detectServiceProfile(summary model.ServiceSurfaceSummary, registration model.WorkspaceRegistration) string {
	hasCSharp := false
	for _, language := range registration.Languages {
		if strings.EqualFold(language, "csharp") {
			hasCSharp = true
			break
		}
	}
	if hasCSharp && (len(summary.HTTPEndpoints) > 0 || len(summary.EventConsumers) > 0 || len(summary.Entities) > 0) {
		return "dotnet-microservice"
	}
	return "generic"
}

func buildNextQueries(workspaceName string, servicePath string, summary model.ServiceSurfaceSummary) []string {
	queries := []string{
		fmt.Sprintf("mi-lsp nav overview %s --workspace %s --format compact", servicePath, workspaceName),
		fmt.Sprintf("mi-lsp nav search \"Map(Get|Post|Put|Delete|Patch)\" --workspace %s --format compact", workspaceName),
	}
	if len(summary.HTTPEndpoints) > 0 {
		file, _ := summary.HTTPEndpoints[0]["file"].(string)
		line, _ := summary.HTTPEndpoints[0]["line"].(int)
		if file != "" && line > 0 {
			queries = append([]string{fmt.Sprintf("mi-lsp nav context %s %d --workspace %s --format compact", file, line, workspaceName)}, queries...)
		}
	}
	return queries
}

func serviceFileCount(summary model.ServiceSurfaceSummary) int {
	files := map[string]struct{}{}
	collectFiles := func(items []map[string]any) {
		for _, item := range items {
			if file, ok := item["file"].(string); ok && file != "" {
				files[file] = struct{}{}
			}
		}
	}
	collectFiles(summary.HTTPEndpoints)
	collectFiles(summary.EventConsumers)
	collectFiles(summary.EventPublishers)
	collectFiles(summary.Entities)
	return len(files)
}

func totalSymbolCount(kinds map[string]int) int {
	total := 0
	for _, count := range kinds {
		total += count
	}
	return total
}

func serviceNameFromPath(relativePath string, root string) string {
	trimmed := strings.Trim(filepath.ToSlash(relativePath), "/")
	if trimmed == "" {
		return filepath.Base(root)
	}
	return filepath.Base(trimmed)
}

func normalizedServicePath(relativePath string) string {
	trimmed := strings.Trim(filepath.ToSlash(relativePath), "/")
	if trimmed == "" {
		return "."
	}
	return trimmed
}

func normalizedPrefix(relativePath string) string {
	trimmed := strings.Trim(filepath.ToSlash(relativePath), "/")
	if trimmed == "" {
		return ""
	}
	return trimmed + "/"
}

func isEntityFile(filePath string) bool {
	normalized := filepath.ToSlash(filePath)
	return strings.Contains(normalized, "/Domain/Entities/") || strings.Contains(normalized, "/Domain/Models/")
}

func archetypeMatchForName(name string, category string) (string, bool) {
	normalized := strings.ToLower(strings.TrimSpace(name))
	archetypes := map[string]struct{}{
		"usuario":                    {},
		"editarusuariocommand":       {},
		"getusuarioquery":            {},
		"usuarioactualizadoconsumer": {},
	}
	if _, ok := archetypes[normalized]; ok {
		return fmt.Sprintf("%s:%s", category, name), true
	}
	return "", false
}

func appendUnique(items []string, value string) []string {
	for _, existing := range items {
		if existing == value {
			return items
		}
	}
	return append(items, value)
}

func sortMaps(items []map[string]any) {
	sort.Slice(items, func(i, j int) bool {
		leftFile, _ := items[i]["file"].(string)
		rightFile, _ := items[j]["file"].(string)
		if leftFile != rightFile {
			return leftFile < rightFile
		}
		leftLine, _ := items[i]["line"].(int)
		rightLine, _ := items[j]["line"].(int)
		return leftLine < rightLine
	})
}

func trimNamespace(value string) string {
	parts := strings.Split(value, ".")
	if len(parts) == 0 {
		return value
	}
	return parts[len(parts)-1]
}
