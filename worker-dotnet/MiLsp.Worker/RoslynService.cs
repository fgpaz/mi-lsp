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
    private static readonly object _locatorLock = new();
    private static bool _msbuildRegistered;

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

    private static readonly string[] GraphCapabilities = ["declarations", "contains", "references", "calls", "implements", "extends"];

    private async Task<WorkerResponse> GraphObserveAsync(WorkerRequest request, Stopwatch started, CancellationToken cancellationToken)
    {
        var repository = request.Payload.GetString("repository_identity")?.Normalize(NormalizationForm.FormC).Trim();
        var rawModule = request.Payload.GetString("project_or_module");
        if (string.IsNullOrWhiteSpace(repository) || string.IsNullOrWhiteSpace(rawModule))
        {
            return new WorkerResponse(false, "roslyn", Error: "Graph observation provenance is required", ErrorCode: "GPH_BACKEND_PROVENANCE_MISSING", Stats: new WorkerStats(Ms: started.ElapsedMilliseconds));
        }
        if (!TryNormalizeProjectModule(rawModule, out var module))
        {
            return ProjectNotFoundResponse(started.ElapsedMilliseconds);
        }

        string repoRoot;
        string expectedProject;
        try
        {
            repoRoot = Path.GetFullPath(request.RepoRoot ?? request.Workspace).TrimEnd(Path.DirectorySeparatorChar, Path.AltDirectorySeparatorChar);
            expectedProject = Path.GetFullPath(Path.Combine(repoRoot, module.Replace('/', Path.DirectorySeparatorChar)));
        }
        catch (Exception)
        {
            return ProjectNotFoundResponse(started.ElapsedMilliseconds);
        }

        if (!IsPathInsideRoot(expectedProject, repoRoot) || !File.Exists(expectedProject))
        {
            return ProjectNotFoundResponse(started.ElapsedMilliseconds);
        }

        var backendVersion = typeof(Microsoft.CodeAnalysis.Compilation).Assembly.GetName().Version?.ToString() ?? "unknown";
        const string extractorVersion = "mi-lsp-roslyn-g2-v1";
        var batch = new GraphObservationBatch
        {
            WorkspaceIdentity = repository,
            RepositoryIdentity = repository,
            BackendVersion = backendVersion,
            ExtractorVersion = extractorVersion,
            ProjectOrModule = module,
            SourceFingerprint = GraphObservationBuilder.PreLoadSourceFingerprint(repoRoot, module, expectedProject, backendVersion, extractorVersion, repository),
            ConfigFingerprint = GraphObservationBuilder.PreLoadConfigFingerprint(module, expectedProject, backendVersion, extractorVersion, repository)
        };
        batch.Capabilities.AddRange(GraphCapabilities.Select(capability => new GraphObservationCapability { Capability = capability }));

        Project? project = null;
        var partial = false;
        try
        {
            var solution = await LoadSolutionAsync(request, cancellationToken).ConfigureAwait(false);
            project = solution.Projects.SingleOrDefault(candidate => candidate.FilePath is not null &&
                string.Equals(Path.GetFullPath(candidate.FilePath), expectedProject, StringComparison.OrdinalIgnoreCase));
            if (project is null)
            {
                return ProjectNotFoundResponse(started.ElapsedMilliseconds);
            }

            Compilation? compilation = null;
            try
            {
                compilation = await project.GetCompilationAsync(cancellationToken).ConfigureAwait(false);
            }
            catch (Exception)
            {
                partial = true;
            }

            batch.SourceFingerprint = GraphObservationBuilder.SourceFingerprint(project, repoRoot, module, batch.BackendVersion, batch.ExtractorVersion, repository, () => partial = true);
            batch.ConfigFingerprint = GraphObservationBuilder.ConfigFingerprint(project, compilation, module, batch.BackendVersion, batch.ExtractorVersion, repository);

            if (compilation is not null && compilation.GetDiagnostics(cancellationToken).Any(diagnostic => diagnostic.Severity == DiagnosticSeverity.Error))
            {
                partial = true;
                AddCapabilityOmissions(batch, "compiler_errors", "fix_compile_errors");
            }

            var builder = new GraphObservationBuilder(batch, request, project);
            await builder.ExtractAsync(cancellationToken).ConfigureAwait(false);
            partial |= builder.Partial;
        }
        catch (OperationCanceledException)
        {
            partial = true;
            AddCapabilityOmissions(batch, "canceled", "retry");
        }
        catch (Exception)
        {
            partial = true;
            AddCapabilityOmissions(batch, "semantic_exception", "retry");
        }

        batch.Completeness = partial ? "partial" : "complete";
        foreach (var capability in GraphCapabilities)
        {
            var observed = capability == "declarations" ? batch.Nodes.Count : batch.Edges.Count(edge => edge.Relation == capability);
            var unresolved = batch.Unresolved.Count(item => item.Capability == capability);
            var omitted = batch.Omissions.Count(item => item.Capability == capability);
            batch.Coverage.Add(new GraphObservationCoverage { Capability = capability, Eligible = observed + unresolved + omitted, Observed = observed, Unresolved = unresolved, Omitted = omitted });
        }
        NormalizeObservationIds(batch);
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

    private static WorkerResponse ProjectNotFoundResponse(long elapsedMs) => new(false, "roslyn", Error: "Requested project was not found in the repository", ErrorCode: "GPH_BACKEND_PROJECT_NOT_FOUND", Stats: new WorkerStats(Ms: elapsedMs));

    private static bool TryNormalizeProjectModule(string? value, out string module)
    {
        module = (value ?? string.Empty).Normalize(NormalizationForm.FormC).Trim();
        if (module.Length == 0 || Path.IsPathRooted(module) || module.StartsWith("/", StringComparison.Ordinal) || module.EndsWith("/", StringComparison.Ordinal))
        {
            module = string.Empty;
            return false;
        }
        module = module.Replace('\\', '/');
        var segments = module.Split('/');
        if (segments.Any(segment => segment.Length == 0 || segment == "." || segment == "..") ||
            !module.EndsWith(".csproj", StringComparison.OrdinalIgnoreCase))
        {
            module = string.Empty;
            return false;
        }
        return true;
    }

    private static bool IsPathInsideRoot(string path, string root)
    {
        var relative = Path.GetRelativePath(root, path).Replace('\\', '/');
        return relative != "." && relative != ".." && !relative.StartsWith("../", StringComparison.Ordinal) && !Path.IsPathRooted(relative);
    }

    private static void AddCapabilityOmissions(GraphObservationBatch batch, string reason, string recovery)
    {
        foreach (var capability in GraphCapabilities)
        {
            if (batch.Omissions.Any(item => item.Capability == capability && item.ReasonCode == reason))
            {
                continue;
            }
            batch.Omissions.Add(new GraphObservationOmission
            {
                Ref = "omission:" + (batch.Omissions.Count + 1).ToString("D8", System.Globalization.CultureInfo.InvariantCulture),
                OwnerPath = batch.ProjectOrModule,
                SubjectKind = "project",
                Capability = capability,
                ReasonCode = reason,
                RecoveryHintCode = recovery
            });
        }
    }

    private static void NormalizeObservationIds(GraphObservationBatch batch)
    {
        var orderedEdges = batch.Edges
            .OrderBy(edge => edge.Relation, StringComparer.Ordinal)
            .ThenBy(edge => edge.FromRef, StringComparer.Ordinal)
            .ThenBy(edge => edge.ToRef, StringComparer.Ordinal)
            .ThenBy(edge => edge.OwnerPath, StringComparer.Ordinal)
            .ThenBy(edge => edge.SourceDigest, StringComparer.Ordinal)
            .ToList();
        var edgeRefMap = new Dictionary<string, string>(StringComparer.Ordinal);
        for (var index = 0; index < orderedEdges.Count; index++)
        {
            var canonicalRef = "edge:" + (index + 1).ToString("D8", System.Globalization.CultureInfo.InvariantCulture);
            edgeRefMap[orderedEdges[index].Ref] = canonicalRef;
            orderedEdges[index].Ref = canonicalRef;
        }
        batch.Edges = orderedEdges;
        foreach (var evidence in batch.Evidence)
        {
            if (evidence.EdgeRef is not null && edgeRefMap.TryGetValue(evidence.EdgeRef, out var canonicalEdgeRef))
            {
                evidence.EdgeRef = canonicalEdgeRef;
            }
        }

        var orderedEvidence = batch.Evidence
            .OrderBy(evidence => evidence.NodeRef ?? string.Empty, StringComparer.Ordinal)
            .ThenBy(evidence => evidence.EdgeRef ?? string.Empty, StringComparer.Ordinal)
            .ThenBy(evidence => evidence.SourceUri, StringComparer.Ordinal)
            .ThenBy(evidence => evidence.ClaimKind, StringComparer.Ordinal)
            .ThenBy(evidence => evidence.SourceDigest, StringComparer.Ordinal)
            .ThenBy(evidence => evidence.ObservedDigest, StringComparer.Ordinal)
            .ToList();
        for (var index = 0; index < orderedEvidence.Count; index++)
        {
            orderedEvidence[index].Ref = "evidence:" + (index + 1).ToString("D8", System.Globalization.CultureInfo.InvariantCulture);
        }
        batch.Evidence = orderedEvidence;

        var orderedUnresolved = batch.Unresolved
            .OrderBy(item => item.OwnerPath, StringComparer.Ordinal)
            .ThenBy(item => item.Capability, StringComparer.Ordinal)
            .ThenBy(item => item.ReasonCode, StringComparer.Ordinal)
            .ThenBy(item => item.SelectorDigest, StringComparer.Ordinal)
            .ToList();
        for (var index = 0; index < orderedUnresolved.Count; index++)
        {
            orderedUnresolved[index].Ref = "unresolved:" + (index + 1).ToString("D8", System.Globalization.CultureInfo.InvariantCulture);
        }
        batch.Unresolved = orderedUnresolved;

        var orderedOmissions = batch.Omissions
            .OrderBy(item => item.OwnerPath, StringComparer.Ordinal)
            .ThenBy(item => item.Capability, StringComparer.Ordinal)
            .ThenBy(item => item.ReasonCode, StringComparer.Ordinal)
            .ThenBy(item => item.SubjectKind, StringComparer.Ordinal)
            .ThenBy(item => item.RecoveryHintCode, StringComparer.Ordinal)
            .ToList();
        for (var index = 0; index < orderedOmissions.Count; index++)
        {
            orderedOmissions[index].Ref = "omission:" + (index + 1).ToString("D8", System.Globalization.CultureInfo.InvariantCulture);
        }
        batch.Omissions = orderedOmissions;
    }

    private sealed class GraphObservationBuilder
    {
        private readonly GraphObservationBatch _batch;
        private readonly WorkerRequest _request;
        private readonly Project _project;
        private readonly Dictionary<ISymbol, string> _refs = new(SymbolEqualityComparer.Default);
        private readonly Dictionary<string, string> _identityRefs = new(StringComparer.Ordinal);
        private readonly Dictionary<string, string> _refIdentities = new(StringComparer.Ordinal);
        private readonly Dictionary<string, string> _sourceDigests = new(StringComparer.OrdinalIgnoreCase);
        private readonly Dictionary<string, GraphObservationEdge> _edgesByKey = new(StringComparer.Ordinal);
        private readonly HashSet<string> _evidenceKeys = new(StringComparer.Ordinal);
        private readonly HashSet<string> _externalOmissionKeys = new(StringComparer.Ordinal);
        private int _edgeNumber;
        private int _evidenceNumber;
        public bool Partial { get; private set; }
        public GraphObservationBuilder(GraphObservationBatch batch, WorkerRequest request, Project project) { _batch = batch; _request = request; _project = project; }

        public async Task ExtractAsync(CancellationToken cancellationToken)
        {
            var documents = new List<(Document Document, string Path, string Digest, SyntaxNode Root, SemanticModel Model)>();
            foreach (var document in _project.Documents.OrderBy(item => item.FilePath, StringComparer.Ordinal))
            {
                cancellationToken.ThrowIfCancellationRequested();
                if (document.FilePath is null) continue;
                var path = RelativePath(document.FilePath);
                if (!IsSafeSourcePath(path)) { Partial = true; AddOmission(_batch.ProjectOrModule, "file", "linked_outside_root", "inspect_candidates"); continue; }
                if (!IsEligibleSourcePath(path)) continue;
                var text = await document.GetTextAsync(cancellationToken).ConfigureAwait(false);
                var root = await document.GetSyntaxRootAsync(cancellationToken).ConfigureAwait(false);
                var model = await document.GetSemanticModelAsync(cancellationToken).ConfigureAwait(false);
                if (root is null || model is null) { Partial = true; continue; }
                var sourceDigest = Sha256(File.ReadAllBytes(document.FilePath));
                _sourceDigests[path] = sourceDigest;
                documents.Add((document, path, sourceDigest, root, model));
                foreach (var node in root.DescendantNodesAndSelf())
                {
                    cancellationToken.ThrowIfCancellationRequested();
                    if (!IsDeclaration(node)) continue;
                    var symbol = model.GetDeclaredSymbol(node, cancellationToken);
                    if (symbol is null || !symbol.Locations.Any(location => location.IsInSource)) { Partial = true; continue; }
                    AddNode(symbol, path, sourceDigest, node.GetLocation());
                }
            }
            foreach (var item in documents)
            {
                cancellationToken.ThrowIfCancellationRequested();
                var root = item.Root; var model = item.Model; var path = item.Path; var sourceDigest = item.Digest;
                foreach (var invocation in root.DescendantNodes().OfType<InvocationExpressionSyntax>())
                {
                    cancellationToken.ThrowIfCancellationRequested();
                    AddReference(model, invocation, invocation.Expression, "calls", path, sourceDigest);
                }
                foreach (var creation in root.DescendantNodes().OfType<ObjectCreationExpressionSyntax>())
                {
                    cancellationToken.ThrowIfCancellationRequested();
                    AddReference(model, creation, creation, "calls", path, sourceDigest);
                }
                foreach (var name in root.DescendantNodes().OfType<IdentifierNameSyntax>())
                {
                    cancellationToken.ThrowIfCancellationRequested();
                    if (name.Ancestors().Any(ancestor => ancestor switch
                    {
                        UsingDirectiveSyntax usingDirective => usingDirective.Name.Span.Contains(name.Span),
                        BaseNamespaceDeclarationSyntax namespaceDeclaration => namespaceDeclaration.Name.Span.Contains(name.Span),
                        _ => false
                    })) continue;
                    AddReference(model, name, name, "references", path, sourceDigest);
                }
            }
            AddTypeRelations();
        }

        private static bool IsDeclaration(SyntaxNode node) => node is BaseNamespaceDeclarationSyntax or BaseTypeDeclarationSyntax or DelegateDeclarationSyntax or BaseMethodDeclarationSyntax or PropertyDeclarationSyntax or EventDeclarationSyntax or VariableDeclaratorSyntax;
        private void AddNode(ISymbol symbol, string path, string digest, Location location)
        {
            var kind = Kind(symbol);
            if (string.IsNullOrWhiteSpace(kind)) return;
            symbol = CanonicalSymbol(symbol);
            var identity = symbol.GetDocumentationCommentId() ?? symbol.ToDisplayString(SymbolDisplayFormat.CSharpErrorMessageFormat);
            var reference = "roslyn:" + Sha256(identity);
            if (_refIdentities.TryGetValue(reference, out var registeredIdentity) && !string.Equals(registeredIdentity, identity, StringComparison.Ordinal))
            {
                Partial = true;
                AddOmission(path, kind, "declaration_ref_collision", "inspect_candidates");
                return;
            }

            if (_identityRefs.TryGetValue(identity, out var existingRef))
            {
                _refs[symbol] = existingRef;
                var existing = _batch.Nodes.First(node => node.Ref == existingRef);
                if (string.Equals(existing.Key.OwnerPath, path, StringComparison.Ordinal))
                {
                    AddEvidence(existingRef, null, path, location, digest, "declaration", identity);
                }
                else
                {
                    AddOmission(path, kind, "additional_owner_evidence", "inspect_candidates");
                }
                return;
            }

            _refs[symbol] = reference;
            _identityRefs[identity] = reference;
            _refIdentities[reference] = identity;
            _batch.Nodes.Add(new GraphObservationNode { Ref = reference, DisplayName = symbol.Name, SourceDigest = digest, Key = new GraphNodeKey { RepositoryIdentity = _batch.RepositoryIdentity, ProjectOrModule = _batch.ProjectOrModule, OwnerPath = path, SymbolKind = kind, SemanticIdentity = identity } });
            AddEvidence(reference, null, path, location, digest, "declaration", identity);
            if (symbol.ContainingSymbol is { } containing && containing is not INamespaceSymbol { IsGlobalNamespace: true }) AddRelation(containing, symbol, "contains", location, path, digest);
        }
        private void AddTypeRelations()
        {
            foreach (var pair in _refs.ToArray())
            {
                if (pair.Key is not INamedTypeSymbol type) continue;
                var owner = pair.Key.Locations.FirstOrDefault(item => item.IsInSource);
                if (type.BaseType is { SpecialType: not SpecialType.System_Object } baseType) AddRelation(type, baseType, "extends", owner);
                var inherited = type.BaseType?.AllInterfaces.Cast<ISymbol>().ToHashSet(SymbolEqualityComparer.Default) ?? new HashSet<ISymbol>(SymbolEqualityComparer.Default);
                foreach (var iface in type.Interfaces.Where(item => !inherited.Contains(item))) AddRelation(type, iface, "implements", owner);
            }
        }
        private void AddReference(SemanticModel model, SyntaxNode ownerNode, SyntaxNode targetNode, string relation, string path, string digest)
        {
            var owner = model.GetEnclosingSymbol(ownerNode.SpanStart);
            if (targetNode.Ancestors().OfType<BaseTypeDeclarationSyntax>().Any(type => type.BaseList?.Types.Any(baseType => baseType.Span.Contains(targetNode.Span)) == true)) return;
            var info = model.GetSymbolInfo(targetNode);
            var target = info.Symbol;
            if (owner is null)
            {
                Partial = true;
                AddUnresolved(path, "file", relation, "source_endpoint_missing", targetNode.ToString(), digest, info.CandidateSymbols);
                return;
            }
            if (target is null)
            {
                if (info.CandidateSymbols.Length == 0)
                {
                    AddTypedOmission(path, SubjectKind(owner), relation, "unbound_target", "inspect_candidates");
                    return;
                }
                Partial = true;
                var reason = info.CandidateSymbols.Length > 1 ? "ambiguous_target" : "target_endpoint_missing";
                AddUnresolved(path, SubjectKind(owner), relation, reason, targetNode.ToString(), digest, info.CandidateSymbols);
                return;
            }
            target = CanonicalSymbol(target);
            if (target.Locations.Any(item => item.IsInSource && item.SourceSpan == targetNode.Span)) return;
            if (string.IsNullOrWhiteSpace(Kind(target)))
            {
                AddTypedOmission(path, SubjectKind(owner), relation, "unsupported_symbol_kind", "inspect_candidates");
                return;
            }
            AddRelation(owner, target, relation, ownerNode.GetLocation(), path, digest);
        }
        private void AddRelation(ISymbol from, ISymbol to, string relation, Location? location, string? ownerPath = null, string? digest = null)
        {
            from = CanonicalSymbol(from);
            to = CanonicalSymbol(to);
            var path = ownerPath ?? RelativePath(location?.SourceTree?.FilePath ?? "");
            var sourceDigest = digest ?? (_sourceDigests.TryGetValue(path, out var registeredDigest) ? registeredDigest : Sha256(FileText(location)));
            var sourceKind = SubjectKind(from);
            if (string.IsNullOrWhiteSpace(Kind(from)))
            {
                Partial = true;
                AddUnresolved(path, sourceKind, relation, "source_endpoint_missing", from.ToDisplayString(SymbolDisplayFormat.CSharpErrorMessageFormat), sourceDigest, []);
                return;
            }
            if (string.IsNullOrWhiteSpace(Kind(to)))
            {
                AddTypedOmission(path, sourceKind, relation, "unsupported_symbol_kind", "inspect_candidates");
                return;
            }
            if (!_refs.TryGetValue(from, out var fromRef))
            {
                Partial = true;
                AddUnresolved(path, sourceKind, relation, "source_endpoint_missing", from.ToDisplayString(SymbolDisplayFormat.CSharpErrorMessageFormat), sourceDigest, []);
                return;
            }
            if (!_refs.TryGetValue(to, out var toRef))
            {
                if (to.IsImplicitlyDeclared)
                {
                    AddTypedOmission(path, sourceKind, relation, "implicit_target", "inspect_candidates");
                    return;
                }
                if (!to.Locations.Any(item => item.IsInSource))
                {
                    AddExternalOmission(path, sourceKind, relation);
                    return;
                }
                var targetLocation = to.Locations.FirstOrDefault(item => item.IsInSource);
                var targetPath = targetLocation?.SourceTree?.FilePath is { } targetFile ? RelativePath(targetFile) : "";
                if (targetLocation is not null && IsSafeSourcePath(targetPath) && _sourceDigests.TryGetValue(targetPath, out var targetDigest))
                {
                    AddNode(to, targetPath, targetDigest, targetLocation);
                    if (_refs.TryGetValue(to, out toRef))
                    {
                        AddRelation(from, to, relation, location, ownerPath, digest);
                        return;
                    }
                }
                AddTypedOmission(path, sourceKind, relation, "source_target_not_declared", "inspect_candidates");
                return;
            }

            var key = fromRef + "|" + toRef + "|" + relation + "|symbol";
            if (!_edgesByKey.TryGetValue(key, out var edge))
            {
                edge = new GraphObservationEdge { Ref = "edge:" + (++_edgeNumber).ToString("D8", System.Globalization.CultureInfo.InvariantCulture), FromRef = fromRef, ToRef = toRef, Relation = relation, OwnerPath = path, SourceDigest = sourceDigest };
                _edgesByKey[key] = edge;
                _batch.Edges.Add(edge);
            }
            else if (!string.Equals(edge.OwnerPath, path, StringComparison.Ordinal) || !string.Equals(edge.SourceDigest, sourceDigest, StringComparison.OrdinalIgnoreCase))
            {
                AddTypedOmission(path, sourceKind, relation, "additional_owner_evidence", "inspect_candidates");
                return;
            }
            AddEvidence(null, edge.Ref, path, location, sourceDigest, relation, key);
        }
        private void AddExternalOmission(string ownerPath, string subjectKind, string capability)
        {
            var key = ownerPath + "|" + subjectKind + "|" + capability + "|external_target";
            if (!_externalOmissionKeys.Add(key)) return;
            AddTypedOmission(ownerPath, subjectKind, capability, "external_target", "inspect_candidates");
        }
        private void AddOmission(string ownerPath, string subjectKind, string reason, string recovery) => AddTypedOmission(ownerPath, subjectKind, "declarations", reason, recovery);
        private void AddTypedOmission(string ownerPath, string subjectKind, string capability, string reason, string recovery)
        {
            if (_batch.Omissions.Any(item => item.OwnerPath == ownerPath && item.SubjectKind == subjectKind && item.Capability == capability && item.ReasonCode == reason)) return;
            _batch.Omissions.Add(new GraphObservationOmission { Ref = "omission:" + (_batch.Omissions.Count + 1).ToString("D8", System.Globalization.CultureInfo.InvariantCulture), OwnerPath = ownerPath, SubjectKind = subjectKind, Capability = capability, ReasonCode = reason, RecoveryHintCode = recovery });
        }
        private void AddUnresolved(string ownerPath, string subjectKind, string capability, string reason, string selector, string sourceDigest, IEnumerable<ISymbol> candidates)
        {
            var candidateIds = candidates.Select(symbol => symbol.GetDocumentationCommentId() ?? symbol.ToDisplayString(SymbolDisplayFormat.CSharpErrorMessageFormat))
                .Select(value => (value ?? string.Empty).Normalize(NormalizationForm.FormC).Trim())
                .Where(value => value.Length > 0)
                .Select(value => value.Length <= 256 ? value : "sha256:" + Sha256(value))
                .Distinct(StringComparer.Ordinal).OrderBy(value => value, StringComparer.Ordinal).Take(8).ToList();
            var selectorDigest = Sha256(string.Join("|", new[] { _batch.RepositoryIdentity, _batch.ProjectOrModule, ownerPath, subjectKind, capability, reason, selector }));
            var key = ownerPath + "|" + subjectKind + "|" + capability + "|" + reason + "|" + selectorDigest;
            if (_batch.Unresolved.Any(item => item.OwnerPath + "|" + item.SubjectKind + "|" + item.Capability + "|" + item.ReasonCode + "|" + item.SelectorDigest == key)) return;
            _batch.Unresolved.Add(new GraphObservationUnresolved { Ref = "unresolved:" + (_batch.Unresolved.Count + 1).ToString("D8", System.Globalization.CultureInfo.InvariantCulture), OwnerPath = ownerPath, SubjectKind = subjectKind, Capability = capability, SelectorDigest = selectorDigest, ReasonCode = reason, Candidates = candidateIds, SourceDigest = sourceDigest, RecoveryHintCode = "inspect_candidates" });
        }

        private void AddEvidence(string? nodeRef, string? edgeRef, string path, Location? location, string sourceDigest, string claim, string observed)
        {
            var span = location?.GetLineSpan();
            var range = span is null ? null : new GraphObservationRange { StartLine = span.Value.StartLinePosition.Line + 1, StartColumn = span.Value.StartLinePosition.Character + 1, EndLine = span.Value.EndLinePosition.Line + 1, EndColumn = span.Value.EndLinePosition.Character + 1 };
            var rangeKey = range is null ? "-" : string.Join(":", range.StartLine, range.StartColumn, range.EndLine, range.EndColumn);
            var evidenceKey = (edgeRef ?? nodeRef ?? "") + "|" + path + "|" + rangeKey + "|" + claim;
            if (!_evidenceKeys.Add(evidenceKey)) return;
            var canonicalClaim = string.Join("|", new[] { nodeRef ?? edgeRef ?? "", claim, path, rangeKey, sourceDigest, observed });
            var observedDigest = Sha256(canonicalClaim);
            _batch.Evidence.Add(new GraphObservationEvidence { Ref = "evidence:" + (++_evidenceNumber).ToString("D8", System.Globalization.CultureInfo.InvariantCulture), NodeRef = nodeRef, EdgeRef = edgeRef, SourceUri = path, Range = range, ExtractorVersion = _batch.ExtractorVersion, SourceDigest = sourceDigest, ObservedDigest = observedDigest, ClaimKind = claim });
        }
        private bool IsSafeSourcePath(string path) => path != "." && path != ".." && !path.StartsWith("../", StringComparison.Ordinal) && !Path.IsPathRooted(path);
        private string RelativePath(string path) { var root = Path.GetFullPath(_request.RepoRoot ?? _request.Workspace); var full = Path.GetFullPath(path); return Path.GetRelativePath(root, full).Replace('\\', '/'); }
        private static ISymbol CanonicalSymbol(ISymbol symbol)
        {
            if (symbol is IMethodSymbol method)
            {
                if (method.AssociatedSymbol is not null) symbol = method.AssociatedSymbol;
                else if (method.ReducedFrom is not null) symbol = method.ReducedFrom;
            }
            return symbol.OriginalDefinition;
        }
        private static string Kind(ISymbol symbol) => CanonicalSymbol(symbol) switch { INamespaceSymbol => "namespace", INamedTypeSymbol => "type", IMethodSymbol => "method", IFieldSymbol => "field", IPropertySymbol => "property", IEventSymbol => "event", _ => "" };
        private static string SubjectKind(ISymbol symbol) => Kind(symbol) is { Length: > 0 } kind ? kind : "file";
        internal static string Sha256(byte[] value) => Convert.ToHexString(SHA256.HashData(value)).ToLowerInvariant();
        internal static string Sha256(string value) => Sha256(Encoding.UTF8.GetBytes(value));
        internal static string PreLoadSourceFingerprint(string root, string module, string projectPath, string backend, string extractor, string repository)
        {
            var projectBytes = File.Exists(projectPath) ? File.ReadAllBytes(projectPath) : [];
            var sources = Directory.EnumerateFiles(root, "*", SearchOption.AllDirectories)
                .Where(path => IsEligibleSourcePath(Path.GetRelativePath(root, path).Replace('\\', '/')))
                .Select(path => Path.GetRelativePath(root, path).Replace('\\', '/') + ":" + Sha256(File.ReadAllBytes(path)))
                .OrderBy(value => value, StringComparer.Ordinal);
            return Sha256(string.Join("|", new[] { repository, module, backend, extractor, Sha256(projectBytes), "sources" }.Concat(sources)));
        }
        internal static string PreLoadConfigFingerprint(string module, string projectPath, string backend, string extractor, string repository)
        {
            var projectBytes = File.Exists(projectPath) ? File.ReadAllBytes(projectPath) : [];
            return Sha256(string.Join("|", repository, module, backend, extractor, Sha256(projectBytes), "preload"));
        }
        internal static string SourceFingerprint(Project project, string root, string module, string backend, string extractor, string repository, Action outsideSource)
        {
            var values = new List<string>();
            var projectBytes = project.FilePath is not null && File.Exists(project.FilePath) ? File.ReadAllBytes(project.FilePath) : [];
            foreach (var document in project.Documents.Where(item => item.FilePath is not null).OrderBy(item => item.FilePath, StringComparer.Ordinal))
            {
                var fullPath = Path.GetFullPath(document.FilePath!);
                var relative = Path.GetRelativePath(root, fullPath).Replace('\\', '/');
                if (!IsSafeRelativePath(relative) || !IsEligibleSourcePath(relative))
                {
                    if (IsEligibleSourcePath(relative)) outsideSource();
                    continue;
                }
                values.Add(relative + ":" + Sha256(File.ReadAllBytes(fullPath)));
            }
            return Sha256(string.Join("|", new[] { repository, module, backend, extractor, Sha256(projectBytes), "sources" }.Concat(values)));
        }
        internal static string ConfigFingerprint(Project project, Compilation? compilation, string module, string backend, string extractor, string repository)
        {
            var projectBytes = project.FilePath is not null && File.Exists(project.FilePath) ? File.ReadAllBytes(project.FilePath) : [];
            var options = compilation?.Options?.ToString() ?? project.CompilationOptions?.ToString() ?? "";
            var parseOptions = project.ParseOptions?.ToString() ?? "";
            var references = compilation is null ? [] : compilation.References.Select(reference => GetAssemblyIdentity(compilation, reference)).Where(identity => identity.Length > 0).OrderBy(identity => identity, StringComparer.Ordinal).ToArray();
            return Sha256(string.Join("|", new[] { repository, module, backend, extractor, Sha256(projectBytes), parseOptions, options, "deterministic" }.Concat(references)));
        }
        private static string GetAssemblyIdentity(Compilation compilation, MetadataReference reference)
        {
            try
            {
                if (compilation.GetAssemblyOrModuleSymbol(reference) is not IAssemblySymbol assembly)
                {
                    return string.Empty;
                }
                var identity = assembly.Identity;
                var token = identity.PublicKeyToken.IsDefaultOrEmpty ? string.Empty : Convert.ToHexString(identity.PublicKeyToken.ToArray()).ToLowerInvariant();
                return string.Join("/", identity.Name, identity.Version, identity.CultureName ?? string.Empty, token);
            }
            catch (Exception)
            {
                return string.Empty;
            }
        }
        private static bool IsSafeRelativePath(string path) => path != "." && path != ".." && !path.StartsWith("../", StringComparison.Ordinal) && !Path.IsPathRooted(path);
        private static bool IsEligibleSourcePath(string path)
        {
            var normalized = path.Replace('\\', '/');
            var segments = normalized.Split('/', StringSplitOptions.RemoveEmptyEntries);
            if (segments.Any(segment => string.Equals(segment, "obj", StringComparison.OrdinalIgnoreCase) || string.Equals(segment, "bin", StringComparison.OrdinalIgnoreCase))) return false;
            return normalized.EndsWith(".cs", StringComparison.OrdinalIgnoreCase) || normalized.EndsWith(".csx", StringComparison.OrdinalIgnoreCase);
        }
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
