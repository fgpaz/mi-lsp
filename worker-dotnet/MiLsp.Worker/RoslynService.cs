using Microsoft.Build.Locator;
using Microsoft.CodeAnalysis;
using Microsoft.CodeAnalysis.FindSymbols;
using Microsoft.CodeAnalysis.MSBuild;
using Microsoft.CodeAnalysis.CSharp.Syntax;
using System.Collections.Concurrent;
using System.Diagnostics;
using System.Security.Cryptography;
using System.Text;

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
                "graph_observe" => await GraphObserveAsync(request, started, cancellationToken),
                "status" => GetStatus(request, started.ElapsedMilliseconds),
                _ => new WorkerResponse(false, "roslyn", Error: $"Unknown method '{request.Method}'", Stats: new WorkerStats(Ms: started.ElapsedMilliseconds)),
            };
        }
        catch (Exception exception)
        {
            var errorCode = "";

            // Check if this is a solution config error
            if (exception.Message.StartsWith("solution_config_error", StringComparison.OrdinalIgnoreCase))
            {
                errorCode = "solution_config_error";
            }
            else if (exception.Message.Contains("already exists") && exception.Message.Contains("solution folder"))
            {
                // Roslyn error about duplicate project names
                errorCode = "solution_config_error";
            }

            return new WorkerResponse(false, "roslyn", Error: exception.Message, ErrorCode: errorCode, Stats: new WorkerStats(Ms: started.ElapsedMilliseconds));
        }
    }

    private WorkerResponse GetStatus(WorkerRequest request, long elapsedMs)
    {
        var cachedWorkspaces = _workspaceCache.Values.ToList();
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
                ["workspace_cache_count"] = _workspaceCache.Count,
                ["project_count"] = cachedWorkspaces.Sum(workspace => workspace.CurrentSolution.ProjectIds.Count),
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

    private async Task<WorkerResponse> GraphObserveAsync(WorkerRequest request, Stopwatch started, CancellationToken cancellationToken)
    {
        var repository = request.Payload.GetString("repository_identity");
        var module = request.Payload.GetString("project_or_module");
        if (string.IsNullOrWhiteSpace(repository) || string.IsNullOrWhiteSpace(module))
        {
            return new WorkerResponse(false, "roslyn", Error: "Graph observation provenance is required", ErrorCode: "GPH_BACKEND_PROVENANCE_MISSING", Stats: new WorkerStats(Ms: started.ElapsedMilliseconds));
        }

        var batch = new GraphObservationBatch
        {
            WorkspaceIdentity = repository,
            RepositoryIdentity = repository,
            BackendVersion = typeof(Microsoft.CodeAnalysis.Compilation).Assembly.GetName().Version?.ToString() ?? "unknown",
            ExtractorVersion = "mi-lsp-roslyn-g2-v1",
            ProjectOrModule = module,
            SourceFingerprint = new string('0', 64),
            ConfigFingerprint = new string('0', 64)
        };
        var capabilities = new[] { "declarations", "contains", "references", "calls", "implements", "extends" };
        batch.Capabilities.AddRange(capabilities.Select(capability => new GraphObservationCapability { Capability = capability }));
        var partial = false;
        try
        {
            var solution = await LoadSolutionAsync(request, cancellationToken).ConfigureAwait(false);
            var project = solution.Projects.FirstOrDefault(candidate =>
                string.Equals(candidate.Name, module, StringComparison.Ordinal) ||
                string.Equals(candidate.FilePath, NormalizePath(request.Workspace, module), StringComparison.OrdinalIgnoreCase))
                ?? solution.Projects.FirstOrDefault();
            if (project is null)
                throw new InvalidOperationException("No Roslyn project selected");

            var builder = new GraphObservationBuilder(batch, request, project);
            await builder.ExtractAsync(cancellationToken).ConfigureAwait(false);
            partial = builder.Partial;
        }
        catch (OperationCanceledException)
        {
            partial = true;
            batch.Omissions.Add(new GraphObservationOmission { Ref = "O1", OwnerPath = ".", Capability = "declarations", ReasonCode = "canceled", RecoveryHintCode = "retry" });
        }
        catch (Exception)
        {
            partial = true;
            batch.Omissions.Add(new GraphObservationOmission { Ref = "O1", OwnerPath = ".", Capability = "declarations", ReasonCode = "compiler_partial", RecoveryHintCode = "retry" });
        }

        batch.Completeness = partial ? "partial" : "complete";
        if (batch.Evidence.Count > 0)
        {
            batch.SourceFingerprint = GraphObservationBuilder.Sha256(string.Join("|", batch.Evidence.Select(item => item.SourceUri + ":" + item.SourceDigest).Distinct(StringComparer.Ordinal).OrderBy(item => item, StringComparer.Ordinal)));
            batch.ConfigFingerprint = GraphObservationBuilder.Sha256(module + "|" + request.Payload.GetString("project_path"));
        }
        foreach (var capability in capabilities)
        {
            var observed = capability == "declarations" ? batch.Nodes.Count : batch.Edges.Count(edge => edge.Relation == capability);
            var unresolved = batch.Unresolved.Count(item => item.Capability == capability);
            var omitted = batch.Omissions.Count(item => item.Capability == capability);
            batch.Coverage.Add(new GraphObservationCoverage { Capability = capability, Eligible = observed + unresolved + omitted, Observed = observed, Unresolved = unresolved, Omitted = omitted });
        }
        batch.Capabilities = batch.Capabilities.OrderBy(item => item.Capability, StringComparer.Ordinal).ToList();
        batch.Coverage = batch.Coverage.OrderBy(item => item.Capability, StringComparer.Ordinal).ToList();
        batch.Nodes = batch.Nodes.OrderBy(item => item.Key.SemanticIdentity, StringComparer.Ordinal).ToList();
        batch.Edges = batch.Edges.OrderBy(item => item.Relation, StringComparer.Ordinal).ThenBy(item => item.FromRef, StringComparer.Ordinal).ThenBy(item => item.ToRef, StringComparer.Ordinal).ToList();
        batch.Evidence = batch.Evidence.OrderBy(item => item.Ref, StringComparer.Ordinal).ToList();
        batch.Unresolved = batch.Unresolved.OrderBy(item => item.Ref, StringComparer.Ordinal).ToList();
        batch.Omissions = batch.Omissions.OrderBy(item => item.Ref, StringComparer.Ordinal).ToList();
        batch.ResourceStats = new GraphObservationResourceStats { ElapsedMilliseconds = started.ElapsedMilliseconds, RssBytes = Environment.WorkingSet };
        return new WorkerResponse(true, "roslyn", Warnings: batch.Completeness == "partial" ? new List<string> { "partial compiler observation" } : null, Stats: new WorkerStats(Symbols: batch.Nodes.Count, Files: batch.Evidence.Select(item => item.SourceUri).Distinct(StringComparer.Ordinal).Count(), Ms: started.ElapsedMilliseconds), Observation: batch);
    }

    private sealed class GraphObservationBuilder
    {
        private readonly GraphObservationBatch _batch;
        private readonly WorkerRequest _request;
        private readonly Project _project;
        private readonly Dictionary<ISymbol, string> _refs = new(SymbolEqualityComparer.Default);
        private readonly Dictionary<string, string> _paths = new(StringComparer.OrdinalIgnoreCase);
        private readonly HashSet<string> _edgeKeys = new(StringComparer.Ordinal);
        private int _edgeNumber;
        private int _evidenceNumber;
        public bool Partial { get; private set; }
        public GraphObservationBuilder(GraphObservationBatch batch, WorkerRequest request, Project project) { _batch = batch; _request = request; _project = project; }

        public async Task ExtractAsync(CancellationToken cancellationToken)
        {
            foreach (var document in _project.Documents.OrderBy(item => item.FilePath, StringComparer.OrdinalIgnoreCase))
            {
                cancellationToken.ThrowIfCancellationRequested();
                if (document.FilePath is null) continue;
                var text = await document.GetTextAsync(cancellationToken).ConfigureAwait(false);
                var root = await document.GetSyntaxRootAsync(cancellationToken).ConfigureAwait(false);
                var model = await document.GetSemanticModelAsync(cancellationToken).ConfigureAwait(false);
                if (root is null || model is null) { Partial = true; continue; }
                var path = RelativePath(document.FilePath);
                var sourceDigest = Sha256(text.ToString());
                _paths[document.FilePath] = path;
                foreach (var node in root.DescendantNodesAndSelf())
                {
                    cancellationToken.ThrowIfCancellationRequested();
                    if (!IsDeclaration(node)) continue;
                    var symbol = model.GetDeclaredSymbol(node, cancellationToken);
                    if (symbol is null || !symbol.Locations.Any(location => location.IsInSource)) { Partial = true; continue; }
                    AddNode(symbol, path, sourceDigest, node.GetLocation());
                }
                foreach (var invocation in root.DescendantNodes().OfType<InvocationExpressionSyntax>()) AddReference(model, invocation, invocation.Expression, "calls", path, sourceDigest);
                foreach (var creation in root.DescendantNodes().OfType<ObjectCreationExpressionSyntax>()) AddReference(model, creation, creation.Type, "calls", path, sourceDigest);
                foreach (var name in root.DescendantNodes().OfType<IdentifierNameSyntax>()) AddReference(model, name, name, "references", path, sourceDigest);
            }
            AddTypeRelations();
        }

        private static bool IsDeclaration(SyntaxNode node) => node is BaseNamespaceDeclarationSyntax or BaseTypeDeclarationSyntax or BaseMethodDeclarationSyntax or PropertyDeclarationSyntax or EventDeclarationSyntax or EventFieldDeclarationSyntax or VariableDeclaratorSyntax;
        private void AddNode(ISymbol symbol, string path, string digest, Location location)
        {
            var kind = Kind(symbol);
            if (kind is null) return;
            if (_refs.ContainsKey(symbol)) return;
            var identity = symbol.GetDocumentationCommentId() ?? symbol.ToDisplayString(SymbolDisplayFormat.CSharpErrorMessageFormat);
            var reference = "roslyn:" + Sha256(identity).Substring(0, 32);
            _refs[symbol] = reference;
            _batch.Nodes.Add(new GraphObservationNode { Ref = reference, DisplayName = symbol.Name, SourceDigest = digest, Key = new GraphNodeKey { RepositoryIdentity = _batch.RepositoryIdentity, ProjectOrModule = _batch.ProjectOrModule, OwnerPath = path, SymbolKind = kind, SemanticIdentity = identity } });
            AddEvidence(reference, null, path, location, digest, "declaration", digest);
            if (symbol.ContainingSymbol is not null && _refs.ContainsKey(symbol.ContainingSymbol)) AddRelation(symbol.ContainingSymbol, symbol, "contains", location, path, digest);
        }
        private void AddTypeRelations()
        {
            foreach (var pair in _refs.ToArray())
            {
                if (pair.Key is not INamedTypeSymbol type) continue;
                var owner = pair.Key.Locations.FirstOrDefault(item => item.IsInSource);
                if (type.BaseType is { SpecialType: not SpecialType.System_Object } baseType) AddRelation(type, baseType, "extends", owner);
                var inherited = type.BaseType?.AllInterfaces.Cast<ISymbol>().ToHashSet(SymbolEqualityComparer.Default) ?? new HashSet<ISymbol>(SymbolEqualityComparer.Default);
                foreach (var iface in type.Interfaces.Where(item => item.Locations.Any(location => location.IsInSource) && !inherited.Contains(item))) AddRelation(type, iface, "implements", owner);
            }
        }
        private void AddReference(SemanticModel model, SyntaxNode ownerNode, SyntaxNode targetNode, string relation, string path, string digest)
        {
            var owner = model.GetEnclosingSymbol(ownerNode.SpanStart);
            var target = model.GetSymbolInfo(targetNode).Symbol;
            if (owner is null || target is null) { Partial = true; return; }
            if (target.Locations.Any(item => item.IsInSource && item.SourceSpan == targetNode.Span)) return;
            var ownerSource = owner.Locations.FirstOrDefault(item => item.IsInSource);
            AddRelation(owner, target, relation, ownerSource, path, digest);
        }
        private void AddRelation(ISymbol from, ISymbol to, string relation, Location? location, string? ownerPath = null, string? digest = null)
        {
            if (!_refs.TryGetValue(from, out var fromRef) || !_refs.TryGetValue(to, out var toRef)) return;
            var path = ownerPath ?? RelativePath(location?.SourceTree?.FilePath ?? "");
            var sourceDigest = digest ?? Sha256(FileText(location));
            var key = fromRef + "|" + toRef + "|" + relation + "|symbol";
            if (!_edgeKeys.Add(key)) return;
            var edgeRef = "edge:" + (++_edgeNumber).ToString("D8", System.Globalization.CultureInfo.InvariantCulture);
            _batch.Edges.Add(new GraphObservationEdge { Ref = edgeRef, FromRef = fromRef, ToRef = toRef, Relation = relation, OwnerPath = path, SourceDigest = sourceDigest });
            AddEvidence(null, edgeRef, path, location, sourceDigest, relation, sourceDigest);
        }
        private void AddEvidence(string? nodeRef, string? edgeRef, string path, Location? location, string sourceDigest, string claim, string observed)
        {
            var span = location?.GetLineSpan();
            var range = span is null ? null : new GraphObservationRange { StartLine = span.Value.StartLinePosition.Line + 1, StartColumn = span.Value.StartLinePosition.Character + 1, EndLine = span.Value.EndLinePosition.Line + 1, EndColumn = span.Value.EndLinePosition.Character + 1 };
            _batch.Evidence.Add(new GraphObservationEvidence { Ref = "evidence:" + (++_evidenceNumber).ToString("D8", System.Globalization.CultureInfo.InvariantCulture), NodeRef = nodeRef, EdgeRef = edgeRef, SourceUri = path, Range = range, ExtractorVersion = _batch.ExtractorVersion, SourceDigest = sourceDigest, ObservedDigest = observed, ClaimKind = claim });
        }
        private string RelativePath(string path) { var root = Path.GetFullPath(_request.RepoRoot ?? _request.Workspace); var full = Path.GetFullPath(path); return Path.GetRelativePath(root, full).Replace('\\', '/'); }
        private static string Kind(ISymbol symbol) => symbol switch { INamespaceSymbol => "namespace", INamedTypeSymbol => "type", IMethodSymbol => "method", IFieldSymbol => "field", IPropertySymbol => "property", IEventSymbol => "event", _ => "" };
        internal static string Sha256(string value) => Convert.ToHexString(SHA256.HashData(Encoding.UTF8.GetBytes(value))).ToLowerInvariant();
        private static string FileText(Location? location) => location?.SourceTree?.ToString() ?? "";
    }

    private async Task<Solution> LoadSolutionAsync(WorkerRequest request, CancellationToken cancellationToken)
    {
        EnsureMsBuildRegistered();
        var solutionPath = ResolveSolutionPath(request);

        // Check for duplicate project names BEFORE any caching logic
        if (solutionPath.EndsWith(".sln", StringComparison.OrdinalIgnoreCase))
        {
            var duplicates = DetectDuplicateProjectNames(solutionPath);
            if (duplicates.Count > 0)
            {
                throw new InvalidOperationException($"solution_config_error: duplicate project names in solution: {string.Join(", ", duplicates)}");
            }
        }

        var cacheKey = ResolveCacheKey(request);
        var workspace = _workspaceCache.GetOrAdd(cacheKey, _ => MSBuildWorkspace.Create());
        if (workspace.CurrentSolution.ProjectIds.Count > 0)
        {
            return workspace.CurrentSolution;
        }

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

    private static List<string> DetectDuplicateProjectNames(string slnPath)
    {
        var solutionDir = Path.GetDirectoryName(slnPath) ?? string.Empty;
        var projectsByFolder = new Dictionary<string, List<string>>(StringComparer.OrdinalIgnoreCase);

        try
        {
            var lines = File.ReadAllLines(slnPath);
            foreach (var line in lines)
            {
                // Match lines like: Project("{GUID}") = "ProjectName", "relative/path/ProjectName.csproj", "{GUID}"
                if (line.StartsWith("Project(", StringComparison.OrdinalIgnoreCase))
                {
                    var parts = line.Split(new[] { '"' }, StringSplitOptions.None);
                    if (parts.Length >= 4)
                    {
                        var projectName = parts[3];
                        // Root folder for projects that are not in nested folders
                        var folderKey = "Root";

                        if (!projectsByFolder.ContainsKey(folderKey))
                        {
                            projectsByFolder[folderKey] = new List<string>();
                        }
                        projectsByFolder[folderKey].Add(projectName);
                    }
                }
            }
        }
        catch (Exception)
        {
            // If we can't read the file, don't block on duplicate detection
            return new List<string>();
        }

        // Find duplicates within each folder
        var duplicates = new List<string>();
        foreach (var folder in projectsByFolder.Values)
        {
            var grouped = folder.GroupBy(name => name, StringComparer.OrdinalIgnoreCase).ToList();
            var dupes = grouped.Where(g => g.Count() > 1).Select(g => g.Key).ToList();
            duplicates.AddRange(dupes);
        }

        return duplicates.Distinct(StringComparer.OrdinalIgnoreCase).ToList();
    }
}
