package service

import (
	"context"
	"database/sql"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/fgpaz/mi-lsp/internal/model"
	"github.com/fgpaz/mi-lsp/internal/store"
)

var (
	consumerEventRegex  = regexp.MustCompile(`IConsumer\s*<\s*([A-Za-z_][A-Za-z0-9_]*)\s*>`)
	publisherEventRegex = regexp.MustCompile(`Publish(?:Async)?\s*<\s*([A-Za-z_][A-Za-z0-9_]*)\s*>`)
)

type workspaceMapEntry struct {
	Mode         string              `json:"mode,omitempty"`
	NextSteps    []string            `json:"next_steps,omitempty"`
	Repos        []repoMapEntry      `json:"repos"`
	Services     []serviceMapEntry   `json:"services"`
	Dependencies []serviceDependency `json:"dependencies,omitempty"`
	Stats        workspaceMapStats   `json:"stats"`
}

type repoMapEntry struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Root      string   `json:"root"`
	Languages []string `json:"languages,omitempty"`
}

type serviceMapEntry struct {
	Path           string `json:"path"`
	Name           string `json:"name"`
	Profile        string `json:"profile,omitempty"`
	SymbolCount    int    `json:"symbol_count"`
	EndpointCount  int    `json:"endpoint_count"`
	ConsumerCount  int    `json:"consumer_count"`
	PublisherCount int    `json:"publisher_count"`
	EntityCount    int    `json:"entity_count"`
}

type serviceDependency struct {
	From     string `json:"from"`
	To       string `json:"to"`
	Kind     string `json:"kind"`     // "event", "type"
	Evidence string `json:"evidence"` // event type name or shared type
}

type workspaceMapStats struct {
	RepoCount       int `json:"repo_count"`
	ServiceCount    int `json:"service_count"`
	DependencyCount int `json:"dependency_count"`
	TotalSymbols    int `json:"total_symbols"`
	TotalEndpoints  int `json:"total_endpoints"`
}

func (a *App) workspaceMap(ctx context.Context, request model.CommandRequest) (model.Envelope, error) {
	started := time.Now()
	registration, project, err := a.resolveWorkspaceWithProject(request.Context.Workspace)
	if err != nil {
		return model.Envelope{}, err
	}

	warnings := []string{}

	// Build repo entries
	repos := make([]repoMapEntry, 0, len(project.Repos))
	for _, repo := range project.Repos {
		repos = append(repos, repoMapEntry{
			ID:        repo.ID,
			Name:      repo.Name,
			Root:      repo.Root,
			Languages: repo.Languages,
		})
	}

	// Open catalog DB
	db, err := store.Open(registration.Root)
	if err != nil {
		warnings = append(warnings, "catalog unavailable: "+err.Error())
		return model.Envelope{
			Ok:        true,
			Workspace: registration.Name,
			Backend:   "registry",
			Items:     []workspaceMapEntry{{Repos: repos, Stats: workspaceMapStats{RepoCount: len(repos)}}},
			Warnings:  warnings,
			Stats:     model.Stats{Ms: time.Since(started).Milliseconds()},
		}, nil
	}
	defer db.Close()

	// Discover services by scanning for entrypoints
	services, serviceEvents, svcWarnings := discoverServices(ctx, db, registration, project)
	warnings = append(warnings, svcWarnings...)

	// Detect inter-service dependencies
	deps := detectDependencies(services, serviceEvents)

	// Build stats
	stats := workspaceMapStats{
		RepoCount:       len(repos),
		ServiceCount:    len(services),
		DependencyCount: len(deps),
	}
	for _, svc := range services {
		stats.TotalSymbols += svc.SymbolCount
		stats.TotalEndpoints += svc.EndpointCount
	}

	mapEntry := workspaceMapEntry{
		Repos:        repos,
		Services:     services,
		Dependencies: deps,
		Stats:        stats,
	}
	previewExpanded := false
	if isAXIMode(request.Context) {
		mapEntry.Mode = "full"
		mapEntry.NextSteps = buildWorkspaceAXINextSteps(registration.Name)
		if isAXIPreview(request.Context) {
			mapEntry.Mode = "preview"
			trimmedServices := trimSlice(mapEntry.Services, 5)
			trimmedDependencies := trimSlice(mapEntry.Dependencies, 5)
			previewExpanded = len(trimmedServices) < len(mapEntry.Services) || len(trimmedDependencies) < len(mapEntry.Dependencies)
			mapEntry.Services = trimmedServices
			mapEntry.Dependencies = trimmedDependencies
		}
	}

	env := model.Envelope{
		Ok:        true,
		Workspace: registration.Name,
		Backend:   "catalog+text",
		Items:     []workspaceMapEntry{mapEntry},
		Warnings:  warnings,
		Stats:     model.Stats{Symbols: stats.TotalSymbols, Files: stats.ServiceCount, Ms: time.Since(started).Milliseconds()},
	}
	if previewExpanded {
		return applyAXIPreviewHints(env, request.Context, axiPreviewSummaryHint), nil
	}
	return env, nil
}

type serviceEventData struct {
	ServicePath string
	Consumers   []map[string]any // IConsumer<EventType> hits
	Publishers  []map[string]any // Publish*/IPublishEndpoint hits
}

func discoverServices(ctx context.Context, db *sql.DB, registration model.WorkspaceRegistration, project model.ProjectFile) ([]serviceMapEntry, []serviceEventData, []string) {
	warnings := []string{}
	services := make([]serviceMapEntry, 0)
	serviceEvents := make([]serviceEventData, 0)

	// Use entrypoints as service discovery — each .csproj or .sln is a potential service
	for _, ep := range project.Entrypoints {
		if ctx.Err() != nil {
			break
		}

		// Get the directory of the entrypoint
		epDir := filepath.Dir(ep.Path)
		if epDir == "." {
			epDir = ""
		}
		// Normalize to forward slashes for consistency
		epDir = filepath.ToSlash(epDir)

		// Get symbol count for this prefix
		prefix := normalizedPrefix(epDir)
		symbols, err := store.OverviewByPrefix(ctx, db, prefix, 500, 0)
		if err != nil || len(symbols) == 0 {
			continue
		}

		// Count by kind
		endpointCount := 0
		consumerCount := 0
		publisherCount := 0
		entityCount := 0
		for _, sym := range symbols {
			switch {
			case strings.Contains(sym.Kind, "endpoint") || strings.Contains(sym.Name, "Endpoint"):
				endpointCount++
			case strings.Contains(sym.Kind, "consumer") || strings.HasSuffix(sym.Name, "Consumer"):
				consumerCount++
			case strings.Contains(sym.Kind, "publisher"):
				publisherCount++
			case sym.Kind == "class" && (strings.Contains(sym.Name, "Entity") || strings.Contains(sym.Scope, "Entities") || strings.Contains(sym.Scope, "Domain")):
				entityCount++
			}
		}

		// Detect HTTP endpoints via text search
		absolutePath := registration.Root
		if epDir != "" {
			absolutePath = filepath.Join(registration.Root, filepath.FromSlash(epDir))
		}
		httpHits, _ := searchPatternScoped(ctx, registration.Root, absolutePath, project, `Map(Get|Post|Put|Delete|Patch)\s*\(`, true, 100)
		if len(httpHits) > endpointCount {
			endpointCount = len(httpHits)
		}

		// Detect event consumers
		consumerHits, _ := searchPatternScoped(ctx, registration.Root, absolutePath, project, `IConsumer<`, true, 100)
		if len(consumerHits) > consumerCount {
			consumerCount = len(consumerHits)
		}

		// Detect event publishers
		publisherHits, _ := searchPatternScoped(ctx, registration.Root, absolutePath, project, `Publish(?:Async)?<|IPublishEndpoint`, true, 100)
		if len(publisherHits) > publisherCount {
			publisherCount = len(publisherHits)
		}

		name := filepath.Base(epDir)
		if name == "" || name == "." {
			name = filepath.Base(ep.Path)
		}

		services = append(services, serviceMapEntry{
			Path:           epDir,
			Name:           name,
			SymbolCount:    len(symbols),
			EndpointCount:  endpointCount,
			ConsumerCount:  consumerCount,
			PublisherCount: publisherCount,
			EntityCount:    entityCount,
		})

		// Store event data for dependency detection
		serviceEvents = append(serviceEvents, serviceEventData{
			ServicePath: epDir,
			Consumers:   consumerHits,
			Publishers:  publisherHits,
		})
	}

	return services, serviceEvents, warnings
}

func detectDependencies(services []serviceMapEntry, serviceEvents []serviceEventData) []serviceDependency {
	if len(serviceEvents) == 0 {
		return nil
	}

	// Build a map of event types published by each service
	publishersByEvent := make(map[string]string) // eventType -> servicePath
	for _, svcEvent := range serviceEvents {
		for _, hit := range svcEvent.Publishers {
			text, ok := hit["text"].(string)
			if !ok {
				continue
			}
			eventType := extractEventType(text)
			if eventType != "" {
				publishersByEvent[eventType] = svcEvent.ServicePath
			}
		}
	}

	// Build dependencies by matching published events with consumers
	deps := make([]serviceDependency, 0)
	depsSet := make(map[string]bool) // dedup: "from->to:eventType"

	for _, svcEvent := range serviceEvents {
		for _, hit := range svcEvent.Consumers {
			text, ok := hit["text"].(string)
			if !ok {
				continue
			}
			eventType := extractEventType(text)
			if eventType == "" {
				continue
			}

			// Find which service publishes this event
			publisherPath, found := publishersByEvent[eventType]
			if !found || publisherPath == svcEvent.ServicePath {
				// No publisher found or it's the same service
				continue
			}

			// Find service names from paths
			publisherName := findServiceName(services, publisherPath)
			consumerName := findServiceName(services, svcEvent.ServicePath)

			if publisherName == "" || consumerName == "" {
				continue
			}

			// Dedup
			key := publisherName + "->" + consumerName + ":" + eventType
			if !depsSet[key] {
				deps = append(deps, serviceDependency{
					From:     publisherName,
					To:       consumerName,
					Kind:     "event",
					Evidence: eventType,
				})
				depsSet[key] = true
			}
		}
	}

	return deps
}

// extractEventType extracts the event type name from IConsumer<EventType> or Publish<EventType> patterns.
// Examples: "IConsumer<OrderCreatedEvent>" -> "OrderCreatedEvent"
//
//	"await publishEndpoint.Publish<OrderCreatedEvent>" -> "OrderCreatedEvent"
func extractEventType(text string) string {
	// Match IConsumer<EventType>
	matches := consumerEventRegex.FindStringSubmatch(text)
	if len(matches) > 1 {
		return matches[1]
	}

	// Match Publish<EventType> or PublishAsync<EventType>
	matches = publisherEventRegex.FindStringSubmatch(text)
	if len(matches) > 1 {
		return matches[1]
	}

	return ""
}

// findServiceName finds the service name given its path.
func findServiceName(services []serviceMapEntry, path string) string {
	for _, svc := range services {
		if svc.Path == path {
			return svc.Name
		}
	}
	return ""
}
