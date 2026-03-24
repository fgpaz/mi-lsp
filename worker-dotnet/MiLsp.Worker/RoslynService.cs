using Microsoft.Build.Locator;
using Microsoft.CodeAnalysis;
using Microsoft.CodeAnalysis.FindSymbols;
using Microsoft.CodeAnalysis.MSBuild;
using System.Collections.Concurrent;
using System.Diagnostics;

namespace MiLsp.Worker;

public sealed class RoslynService
{
    private readonly ConcurrentDictionary<string, MSBuildWorkspace> _workspaceCache = new(StringComparer.OrdinalIgnoreCase);
    private readonly object _locatorLock = new();
    private bool _msbuildRegistered;

    public async Task<WorkerResponse> HandleAsync(WorkerRequest request, CancellationToken cancellationToken)
    {
        var started = Stopwatch.StartNew();
        try
        {
            if (!string.IsNullOrWhiteSpace(request.ProtocolVersion) && !string.Equals(request.ProtocolVersion, WorkerProtocol.Version, StringComparison.Ordinal))
            {
                return new WorkerResponse(false, "roslyn", Error: $"Protocol version mismatch. client={request.ProtocolVersion} worker={WorkerProtocol.Version}", Stats: new WorkerStats(Ms: started.ElapsedMilliseconds));
            }

            return request.Method switch
            {
                "find_symbol" => await FindSymbolsAsync(request, cancellationToken),
                "find_refs" => await FindReferencesAsync(request, cancellationToken),
                "get_overview" => await GetOverviewAsync(request, cancellationToken),
                "get_context" => await GetContextAsync(request, cancellationToken),
                "get_deps" => await GetDependenciesAsync(request, cancellationToken),
                "status" => GetStatus(request, started.ElapsedMilliseconds),
                _ => new WorkerResponse(false, "roslyn", Error: $"Unknown method '{request.Method}'", Stats: new WorkerStats(Ms: started.ElapsedMilliseconds)),
            };
        }
        catch (Exception exception)
        {
            return new WorkerResponse(false, "roslyn", Error: exception.Message, Stats: new WorkerStats(Ms: started.ElapsedMilliseconds));
        }
    }

    private static WorkerResponse GetStatus(WorkerRequest request, long elapsedMs)
    {
        var items = new List<Dictionary<string, object?>>
        {
            new()
            {
                ["backend"] = "roslyn",
                ["pid"] = Environment.ProcessId,
                ["protocol_version"] = WorkerProtocol.Version,
                ["repo"] = request.RepoName,
                ["entrypoint_id"] = request.EntrypointId,
                ["entrypoint_path"] = request.EntrypointPath,
            }
        };
        return new WorkerResponse(true, "roslyn", items, Stats: new WorkerStats(Files: 1, Ms: elapsedMs));
    }

    private async Task<WorkerResponse> FindSymbolsAsync(WorkerRequest request, CancellationToken cancellationToken)
    {
        var query = request.Payload.GetString("symbol") ?? request.Payload.GetString("pattern") ?? string.Empty;
        var solution = await LoadSolutionAsync(request, cancellationToken);
        var items = new List<Dictionary<string, object?>>();

        foreach (var project in solution.Projects)
        {
            var compilation = await project.GetCompilationAsync(cancellationToken).ConfigureAwait(false);
            if (compilation is null)
            {
                continue;
            }

            foreach (var symbol in compilation.GetSymbolsWithName(name => name.Contains(query, StringComparison.OrdinalIgnoreCase), SymbolFilter.TypeAndMember))
            {
                if (!symbol.Locations.Any(location => location.IsInSource))
                {
                    continue;
                }
                items.Add(SymbolToItem(symbol, request.RepoName));
            }
        }

        return new WorkerResponse(true, "roslyn", items, Stats: new WorkerStats(Symbols: items.Count));
    }

    private async Task<WorkerResponse> FindReferencesAsync(WorkerRequest request, CancellationToken cancellationToken)
    {
        var symbolQuery = request.Payload.GetString("symbol") ?? throw new InvalidOperationException("symbol is required");
        var solution = await LoadSolutionAsync(request, cancellationToken);
        var declarations = await ResolveDeclarationsAsync(solution, symbolQuery, cancellationToken);
        var items = new List<Dictionary<string, object?>>();

        foreach (var declaration in declarations)
        {
            var references = await SymbolFinder.FindReferencesAsync(declaration, solution, cancellationToken).ConfigureAwait(false);
            foreach (var reference in references)
            {
                foreach (var location in reference.Locations)
                {
                    var linePosition = location.Location.GetLineSpan().StartLinePosition;
                    items.Add(new Dictionary<string, object?>
                    {
                        ["name"] = declaration.Name,
                        ["kind"] = declaration.Kind.ToString().ToLowerInvariant(),
                        ["file"] = location.Document.FilePath,
                        ["line"] = linePosition.Line + 1,
                        ["column"] = linePosition.Character + 1,
                        ["project"] = location.Document.Project.Name,
                        ["repo"] = request.RepoName,
                        ["entrypoint_id"] = request.EntrypointId,
                    });
                }
            }
        }

        return new WorkerResponse(true, "roslyn", items, Stats: new WorkerStats(Symbols: items.Count));
    }

    private async Task<WorkerResponse> GetOverviewAsync(WorkerRequest request, CancellationToken cancellationToken)
    {
        var solution = await LoadSolutionAsync(request, cancellationToken);
        var items = new List<Dictionary<string, object?>>();
        foreach (var project in solution.Projects)
        {
            items.Add(new Dictionary<string, object?>
            {
                ["name"] = project.Name,
                ["kind"] = "project",
                ["file"] = project.FilePath,
                ["line"] = 1,
                ["documents"] = project.DocumentIds.Count,
                ["repo"] = request.RepoName,
            });
        }
        return new WorkerResponse(true, "roslyn", items, Stats: new WorkerStats(Files: items.Count));
    }

    private async Task<WorkerResponse> GetContextAsync(WorkerRequest request, CancellationToken cancellationToken)
    {
        var solution = await LoadSolutionAsync(request, cancellationToken);
        var filePath = request.Payload.GetString("file") ?? throw new InvalidOperationException("file is required");
        var line = request.Payload.GetInt("line", 1);
        var normalizedPath = NormalizePath(request.Workspace, filePath);

        var document = solution.Projects.SelectMany(project => project.Documents)
            .FirstOrDefault(doc => string.Equals(doc.FilePath, normalizedPath, StringComparison.OrdinalIgnoreCase));
        if (document is null)
        {
            return new WorkerResponse(false, "roslyn", Error: $"Document '{filePath}' not found");
        }

        var text = await document.GetTextAsync(cancellationToken).ConfigureAwait(false);
        var syntaxRoot = await document.GetSyntaxRootAsync(cancellationToken).ConfigureAwait(false);
        var semanticModel = await document.GetSemanticModelAsync(cancellationToken).ConfigureAwait(false);
        if (syntaxRoot is null || semanticModel is null)
        {
            return new WorkerResponse(false, "roslyn", Error: "Unable to load semantic model for requested file");
        }

        var targetLine = Math.Clamp(line - 1, 0, Math.Max(0, text.Lines.Count - 1));
        var position = text.Lines[targetLine].Start;
        var token = syntaxRoot.FindToken(position);
        var node = token.Parent ?? syntaxRoot;
        var symbol = semanticModel.GetDeclaredSymbol(node, cancellationToken)
            ?? semanticModel.GetSymbolInfo(node, cancellationToken).Symbol
            ?? semanticModel.GetEnclosingSymbol(position, cancellationToken);

        var items = new List<Dictionary<string, object?>>();
        if (symbol is not null)
        {
            items.Add(SymbolToItem(symbol, request.RepoName));
        }

        return new WorkerResponse(true, "roslyn", items, Stats: new WorkerStats(Symbols: items.Count));
    }

    private async Task<WorkerResponse> GetDependenciesAsync(WorkerRequest request, CancellationToken cancellationToken)
    {
        var solution = await LoadSolutionAsync(request, cancellationToken);
        var projectHint = request.Payload.GetString("project_hint") ?? request.Payload.GetString("project");
        var entrypointProject = string.Equals(request.EntrypointType, "project", StringComparison.OrdinalIgnoreCase) ? NormalizePath(request.Workspace, request.EntrypointPath ?? string.Empty) : null;

        var project = solution.Projects.FirstOrDefault(candidate =>
            string.Equals(candidate.Name, projectHint, StringComparison.OrdinalIgnoreCase) ||
            string.Equals(candidate.FilePath, NormalizePath(request.Workspace, projectHint ?? string.Empty), StringComparison.OrdinalIgnoreCase) ||
            (!string.IsNullOrWhiteSpace(entrypointProject) && string.Equals(candidate.FilePath, entrypointProject, StringComparison.OrdinalIgnoreCase)))
            ?? solution.Projects.FirstOrDefault();

        if (project is null)
        {
            return new WorkerResponse(false, "roslyn", Error: "No project found for dependency inspection");
        }

        var items = new List<Dictionary<string, object?>>();
        foreach (var projectReference in project.ProjectReferences)
        {
            var referencedProject = solution.GetProject(projectReference.ProjectId);
            items.Add(new Dictionary<string, object?>
            {
                ["name"] = referencedProject?.Name ?? projectReference.ProjectId.ToString(),
                ["kind"] = "project_reference",
                ["file"] = referencedProject?.FilePath,
                ["line"] = 1,
                ["repo"] = request.RepoName,
            });
        }

        return new WorkerResponse(true, "roslyn", items, Stats: new WorkerStats(Files: items.Count));
    }

    private async Task<Solution> LoadSolutionAsync(WorkerRequest request, CancellationToken cancellationToken)
    {
        EnsureMsBuildRegistered();
        var cacheKey = ResolveCacheKey(request);
        var workspace = _workspaceCache.GetOrAdd(cacheKey, _ => MSBuildWorkspace.Create());
        if (workspace.CurrentSolution.ProjectIds.Count > 0)
        {
            return workspace.CurrentSolution;
        }

        var solutionPath = ResolveSolutionPath(request);
        if (solutionPath.EndsWith(".sln", StringComparison.OrdinalIgnoreCase))
        {
            return await workspace.OpenSolutionAsync(solutionPath, cancellationToken: cancellationToken).ConfigureAwait(false);
        }

        await workspace.OpenProjectAsync(solutionPath, cancellationToken: cancellationToken).ConfigureAwait(false);
        return workspace.CurrentSolution;
    }

    private static string ResolveCacheKey(WorkerRequest request)
    {
        if (!string.IsNullOrWhiteSpace(request.EntrypointPath))
        {
            return NormalizePath(request.Workspace, request.EntrypointPath);
        }
        var explicitSolution = request.Payload.GetString("solution");
        if (!string.IsNullOrWhiteSpace(explicitSolution))
        {
            return NormalizePath(request.Workspace, explicitSolution);
        }
        var explicitProject = request.Payload.GetString("project_path") ?? request.Payload.GetString("project");
        if (!string.IsNullOrWhiteSpace(explicitProject))
        {
            return NormalizePath(request.Workspace, explicitProject);
        }
        return Path.GetFullPath(request.Workspace);
    }

    private async Task<List<ISymbol>> ResolveDeclarationsAsync(Solution solution, string query, CancellationToken cancellationToken)
    {
        var items = new List<ISymbol>();
        foreach (var project in solution.Projects)
        {
            var compilation = await project.GetCompilationAsync(cancellationToken).ConfigureAwait(false);
            if (compilation is null)
            {
                continue;
            }

            items.AddRange(compilation.GetSymbolsWithName(
                name => string.Equals(name, query, StringComparison.OrdinalIgnoreCase) || name.Contains(query, StringComparison.OrdinalIgnoreCase),
                SymbolFilter.TypeAndMember));
        }
        return items
            .Where(symbol => symbol.Locations.Any(location => location.IsInSource))
            .GroupBy(symbol => symbol.ToDisplayString())
            .Select(group => group.First())
            .ToList();
    }

    private static string NormalizePath(string workspaceRoot, string path)
    {
        if (string.IsNullOrWhiteSpace(path))
        {
            return workspaceRoot;
        }
        return Path.IsPathRooted(path) ? Path.GetFullPath(path) : Path.GetFullPath(Path.Combine(workspaceRoot, path));
    }

    private static string ResolveSolutionPath(WorkerRequest request)
    {
        var workspaceRoot = Path.GetFullPath(request.Workspace);
        if (!string.IsNullOrWhiteSpace(request.EntrypointPath))
        {
            return NormalizePath(workspaceRoot, request.EntrypointPath);
        }

        var explicitSolution = request.Payload.GetString("solution");
        if (!string.IsNullOrWhiteSpace(explicitSolution))
        {
            return NormalizePath(workspaceRoot, explicitSolution);
        }

        var explicitProject = request.Payload.GetString("project_path");
        if (!string.IsNullOrWhiteSpace(explicitProject))
        {
            return NormalizePath(workspaceRoot, explicitProject);
        }

        var legacyProject = request.Payload.GetString("project");
        if (!string.IsNullOrWhiteSpace(legacyProject) && legacyProject.EndsWith(".csproj", StringComparison.OrdinalIgnoreCase))
        {
            return NormalizePath(workspaceRoot, legacyProject);
        }

        var searchRoot = !string.IsNullOrWhiteSpace(request.RepoRoot) ? Path.GetFullPath(request.RepoRoot) : workspaceRoot;
        var solutions = Directory.EnumerateFiles(searchRoot, "*.sln", SearchOption.AllDirectories).ToList();
        if (solutions.Count > 0)
        {
            return solutions[0];
        }

        var projects = Directory.EnumerateFiles(searchRoot, "*.csproj", SearchOption.AllDirectories).ToList();
        if (projects.Count == 0)
        {
            throw new FileNotFoundException($"No .sln or .csproj found under '{searchRoot}'");
        }
        return projects[0];
    }

    private void EnsureMsBuildRegistered()
    {
        if (_msbuildRegistered)
        {
            return;
        }

        lock (_locatorLock)
        {
            if (_msbuildRegistered)
            {
                return;
            }

            var instance = MSBuildLocator.QueryVisualStudioInstances().OrderByDescending(candidate => candidate.Version).FirstOrDefault();
            if (instance is null)
            {
                throw new InvalidOperationException("No MSBuild instance found for Roslyn worker");
            }

            MSBuildLocator.RegisterInstance(instance);
            _msbuildRegistered = true;
        }
    }

    private static Dictionary<string, object?> SymbolToItem(ISymbol symbol, string? repoName)
    {
        var sourceLocation = symbol.Locations.FirstOrDefault(location => location.IsInSource);
        var lineSpan = sourceLocation?.GetLineSpan();
        return new Dictionary<string, object?>
        {
            ["name"] = symbol.Name,
            ["kind"] = symbol.Kind.ToString().ToLowerInvariant(),
            ["file"] = sourceLocation?.SourceTree?.FilePath,
            ["line"] = lineSpan?.StartLinePosition.Line is int line ? line + 1 : 1,
            ["scope"] = symbol.DeclaredAccessibility.ToString().ToLowerInvariant(),
            ["signature"] = symbol.ToDisplayString(SymbolDisplayFormat.MinimallyQualifiedFormat),
            ["qualified_name"] = symbol.ToDisplayString(SymbolDisplayFormat.CSharpErrorMessageFormat),
            ["repo"] = repoName,
        };
    }
}
