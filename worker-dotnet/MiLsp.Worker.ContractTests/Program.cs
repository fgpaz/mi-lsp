using System.Diagnostics;
using System.Security.Cryptography;
using System.Text;
using System.Text.Json;
using MiLsp.Worker;

static void Require(bool condition, string message)
{
    if (!condition) throw new InvalidOperationException(message);
}

static WorkerRequest Request(string root, string moduleProject, string? entrypointProject = null, string? repository = "example.invalid/repo")
{
    return new WorkerRequest(
        "mi-lsp-v1.1", "graph_observe", root, "fixture", "roslyn", "repo", "repo", root,
        "fixture", entrypointProject ?? moduleProject, "project", new Dictionary<string, JsonElement>
        {
            ["repository_identity"] = JsonSerializer.SerializeToElement(repository),
            ["project_or_module"] = JsonSerializer.SerializeToElement(Path.GetRelativePath(root, moduleProject).Replace('\\', '/'))
        });
}

static WorkerRequest StatusRequest(string root, string moduleProject)
{
    return new WorkerRequest(
        "mi-lsp-v1.1", "status", root, "fixture", "roslyn", "repo", "repo", root,
        "fixture", moduleProject, "project", new Dictionary<string, JsonElement>());
}

static GraphObservationBatch RequireObservation(WorkerResponse response)
{
    Require(response.Ok && response.Observation is not null, "observation response missing");
    return response.Observation ?? throw new InvalidOperationException("observation response missing");
}

static async Task<GraphObservationBatch> ObserveWithService(RoslynService service, string root, string project, CancellationToken cancellationToken = default)
{
    var response = await service.HandleAsync(Request(root, project), cancellationToken);
    return RequireObservation(response);
}

static async Task AssertStatusReloadConcurrency(RoslynService service, string root, string project)
{
    var warm = await service.HandleAsync(Request(root, project), CancellationToken.None);
    Require(warm.Ok && warm.Observation is not null, "status stress warmup failed");

    var operations = Enumerable.Range(0, 12).Select(async index =>
    {
        if (index % 3 == 0)
        {
            var status = await service.HandleAsync(StatusRequest(root, project), CancellationToken.None);
            Require(status.Ok && status.Items?.Count == 1, "status request failed during workspace reload");
        }
        else
        {
            var observation = await service.HandleAsync(Request(root, project), CancellationToken.None);
            Require(observation.Ok && observation.Observation is not null, "graph request failed during status stress");
        }
    });
    await Task.WhenAll(operations);
}

static async Task<GraphObservationBatch> Observe(string root, string project, CancellationToken cancellationToken = default)
{
    var response = await new RoslynService().HandleAsync(Request(root, project), cancellationToken);
    return RequireObservation(response);
}

static async Task AssertNestedProjectUsesRepoRelativeModule()
{
    var root = Path.Combine(Path.GetTempPath(), "milsp-g2-nested-" + Guid.NewGuid().ToString("N"));
    var projectRoot = Path.Combine(root, "MiLsp.Worker");
    Directory.CreateDirectory(projectRoot);
    var project = Path.Combine(projectRoot, "MiLsp.Worker.csproj");
    File.WriteAllText(project, "<Project Sdk=\"Microsoft.NET.Sdk\"><PropertyGroup><TargetFramework>net10.0</TargetFramework></PropertyGroup></Project>");
    File.WriteAllText(Path.Combine(projectRoot, "Nested.cs"), "namespace Nested; public class Marker { public void Run() { } }");
    File.WriteAllText(Path.Combine(root, "MiLsp.Worker.sln"), "not opened by graph_observe");
    try
    {
        var observation = await Observe(root, project);
        Require(observation.ProjectOrModule == "MiLsp.Worker/MiLsp.Worker.csproj", "nested project module was not repo-relative");
        Require(observation.Nodes.Any(node => node.Key.ProjectOrModule == observation.ProjectOrModule), "nested project was not opened through RepoRoot + project_or_module");
    }
    finally
    {
        try { Directory.Delete(root, recursive: true); } catch { }
    }
}

static void AssertCoverage(GraphObservationBatch observation)
{
    Require(observation.Capabilities.Count == 6 && observation.Coverage.Count == 6, "coverage matrix invalid");
    foreach (var coverage in observation.Coverage)
    {
        Require(coverage.Eligible == coverage.Observed + coverage.Unresolved + coverage.Omitted, $"coverage not coherent for {coverage.Capability}");
    }
}

static void AssertSafePaths(GraphObservationBatch observation)
{
    foreach (var evidence in observation.Evidence)
    {
        Require(!Path.IsPathRooted(evidence.SourceUri) && evidence.SourceUri != "." && evidence.SourceUri != ".." && !evidence.SourceUri.StartsWith("../", StringComparison.Ordinal), "invalid evidence path");
        Require(evidence.Range is null || evidence.Range.StartLine > 0, "invalid evidence range");
    }
    foreach (var omission in observation.Omissions)
    {
        Require(!Path.IsPathRooted(omission.OwnerPath) && omission.OwnerPath != "." && omission.OwnerPath != ".." && !omission.OwnerPath.StartsWith("../", StringComparison.Ordinal), "invalid omission owner");
    }
}

static void AssertNonzeroFingerprints(GraphObservationBatch observation)
{
    Require(observation.SourceFingerprint.Length == 64 && observation.SourceFingerprint != new string('0', 64), "source fingerprint is zero");
    Require(observation.ConfigFingerprint.Length == 64 && observation.ConfigFingerprint != new string('0', 64), "config fingerprint is zero");
}

static void EmitObservationsIfRequested(GraphObservationBatch complete, GraphObservationBatch compilerError, GraphObservationBatch canceled)
{
    var output = Environment.GetEnvironmentVariable("MILSP_G2_EMIT_DIR");
    if (string.IsNullOrWhiteSpace(output) || !Path.IsPathFullyQualified(output)) return;
    Directory.CreateDirectory(output);
    var options = new JsonSerializerOptions(JsonSerializerDefaults.Web);
    foreach (var item in new[]
    {
        (Name: "complete.json", Observation: complete),
        (Name: "compiler-error.json", Observation: compilerError),
        (Name: "canceled.json", Observation: canceled)
    })
    {
        File.WriteAllText(Path.Combine(output, item.Name), JsonSerializer.Serialize(item.Observation, options));
    }
}

static void AssertGraphInvariants(GraphObservationBatch observation)
{
    var nodeRefs = observation.Nodes.Select(node => node.Ref).ToHashSet(StringComparer.Ordinal);
    Require(observation.Nodes.Select(node => node.Ref).Distinct(StringComparer.Ordinal).Count() == observation.Nodes.Count, "node refs collide");
    Require(observation.Nodes.All(node => node.Ref.StartsWith("roslyn:", StringComparison.Ordinal) && node.Ref[7..].Length == 64 && node.Ref[7..].All(Uri.IsHexDigit)), "node ref is not full SHA-256");
    // tedi-agent-mcp regression: empty Name / bare DocCommentId "T:" must never become nodes
    Require(observation.Nodes.All(node => !string.IsNullOrWhiteSpace(node.DisplayName) && node.DisplayName.Length <= 256 && node.DisplayName.All(ch => !char.IsControl(ch))), "node display_name is empty or unbounded");
    Require(observation.Nodes.All(node => !string.IsNullOrWhiteSpace(node.Key.SemanticIdentity) && node.Key.SemanticIdentity is not ("T:" or "M:" or "F:" or "P:" or "E:" or "N:")), "node semantic_identity is bare/incomplete");
    foreach (var edge in observation.Edges)
    {
        Require(nodeRefs.Contains(edge.FromRef) && nodeRefs.Contains(edge.ToRef), "dangling edge endpoint");
        var evidence = observation.Evidence.Where(item => item.EdgeRef == edge.Ref).ToList();
        Require(evidence.Count > 0, "edge evidence missing");
        Require(evidence.All(item => item.SourceUri == edge.OwnerPath && item.SourceDigest == edge.SourceDigest), "edge evidence provenance mismatch");
    }
    var registeredKinds = new HashSet<string>(StringComparer.Ordinal) { "workspace", "repository", "project", "package", "file", "namespace", "type", "method", "function", "field", "property", "event", "route", "test", "document" };
    Require(observation.Unresolved.All(item => registeredKinds.Contains(item.SubjectKind)), "unregistered unresolved subject kind");
    Require(observation.Unresolved.SelectMany(item => item.Candidates).All(candidate => candidate.Length <= 256), "candidate exceeds bound");
    Require(observation.Evidence.All(evidence => evidence.ObservedDigest.Length == 64 && evidence.ObservedDigest != evidence.SourceDigest), "observed digest is only source digest");
}

static string Sha256Text(string value)
    => Convert.ToHexString(SHA256.HashData(Encoding.UTF8.GetBytes(value))).ToLowerInvariant();

static void AssertEdgeEvidenceStableUnderReordering()
{
    const string sourceDigest = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa";

    static GraphObservationBatch CreateBatch(bool reverseDiscovery)
    {
        var calls = new GraphObservationEdge
        {
            Ref = reverseDiscovery ? "edge:00000002" : "edge:00000001",
            FromRef = "roslyn:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
            ToRef = "roslyn:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
            Relation = "calls",
            OwnerPath = "Fixture.cs",
            SourceDigest = sourceDigest
        };
        var references = new GraphObservationEdge
        {
            Ref = reverseDiscovery ? "edge:00000001" : "edge:00000002",
            FromRef = "roslyn:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
            ToRef = "roslyn:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
            Relation = "references",
            OwnerPath = "Fixture.cs",
            SourceDigest = sourceDigest
        };
        var edges = reverseDiscovery ? new[] { references, calls } : new[] { calls, references };
        var evidence = edges.Select((edge, index) =>
        {
            var line = edge.Relation == "calls" ? 1 : 2;
            var rangeKey = $"{line}:1:{line}:10";
            var observed = string.Join("|", edge.FromRef, edge.ToRef, edge.Relation, edge.Scope);
            return new GraphObservationEvidence
            {
                Ref = $"evidence:{index + 1:D8}",
                EdgeRef = edge.Ref,
                SourceUri = edge.OwnerPath,
                Range = new GraphObservationRange { StartLine = line, StartColumn = 1, EndLine = line, EndColumn = 10 },
                ExtractorVersion = "test",
                SourceDigest = sourceDigest,
                ObservedDigest = Sha256Text(string.Join("|", edge.Ref, edge.Relation, edge.OwnerPath, rangeKey, sourceDigest, observed)),
                ClaimKind = edge.Relation
            };
        }).ToList();
        return new GraphObservationBatch { Edges = edges.ToList(), Evidence = evidence };
    }

    var first = CreateBatch(false);
    var second = CreateBatch(true);
    Require(first.Evidence.Select(item => item.ObservedDigest).OrderBy(item => item, StringComparer.Ordinal).SequenceEqual(second.Evidence.Select(item => item.ObservedDigest).OrderBy(item => item, StringComparer.Ordinal)) == false, "test fixture did not vary provisional edge evidence digests");
    RoslynService.NormalizeObservationIdsForContract(first);
    RoslynService.NormalizeObservationIdsForContract(second);
    var firstProjection = JsonSerializer.Serialize(new { first.Edges, first.Evidence });
    var secondProjection = JsonSerializer.Serialize(new { second.Edges, second.Evidence });
    Require(firstProjection == secondProjection, "canonical edge evidence changed with discovery order");
    Require(first.Evidence.Where(item => item.EdgeRef is not null).All(item => item.ObservedDigest.Length == 64 && item.ObservedDigest != item.SourceDigest), "canonical edge observed digest is invalid");
}

static string SourceDigest(string path)
    => Convert.ToHexString(SHA256.HashData(File.ReadAllBytes(path))).ToLowerInvariant();

static string CreateFixture(string name, string? source = null, string? project = null)
{
    var root = Path.Combine(Path.GetTempPath(), "milsp-g2-" + name + "-" + Guid.NewGuid().ToString("N"));
    Directory.CreateDirectory(root);
    File.WriteAllText(Path.Combine(root, "Fixture.csproj"), project ?? "<Project Sdk=\"Microsoft.NET.Sdk\"><PropertyGroup><TargetFramework>net10.0</TargetFramework><EnableDefaultCompileItems>true</EnableDefaultCompileItems></PropertyGroup></Project>");
    File.WriteAllText(Path.Combine(root, "Fixture.cs"), source ?? "namespace Fixture; public delegate void ChangedHandler(); public interface IContract { void Run(); } public class Base { public virtual void BaseRun() { } } public class Derived : Base, IContract { public int Value { get; set; } public event ChangedHandler? Changed; public void Run() { BaseRun(); } } public class Consumer { public void Use() { new Derived().Run(); new Derived().Run(); } }");
    return root;
}

static void CopyTree(string source, string destination)
{
    Directory.CreateDirectory(destination);
    foreach (var directory in Directory.GetDirectories(source, "*", SearchOption.AllDirectories))
    {
        Directory.CreateDirectory(Path.Combine(destination, Path.GetRelativePath(source, directory)));
    }
    foreach (var file in Directory.GetFiles(source, "*", SearchOption.AllDirectories))
    {
        var target = Path.Combine(destination, Path.GetRelativePath(source, file));
        Directory.CreateDirectory(Path.GetDirectoryName(target)!);
        File.Copy(file, target);
    }
}

static void BuildFixtureProject(string projectPath)
{
    using var process = Process.Start(new ProcessStartInfo
    {
        FileName = "dotnet",
        WorkingDirectory = Path.GetDirectoryName(projectPath)!,
        UseShellExecute = false,
        CreateNoWindow = true,
        ArgumentList = { "build", projectPath, "--configuration", "Release", "--nologo" }
    });
    Require(process is not null && process.WaitForExit(120_000) && process.ExitCode == 0, "external fixture build failed");
}

static (string Root, string Project) CreateExternalAliasFixture()
{
    var root = Path.Combine(Path.GetTempPath(), "milsp-g2-external-alias-" + Guid.NewGuid().ToString("N"));
    var libraryRoot = Path.Combine(root, "Library");
    var consumerRoot = Path.Combine(root, "Consumer");
    Directory.CreateDirectory(libraryRoot);
    Directory.CreateDirectory(consumerRoot);

    var libraryProject = Path.Combine(libraryRoot, "Library.csproj");
    File.WriteAllText(libraryProject, "<Project Sdk=\"Microsoft.NET.Sdk\"><PropertyGroup><TargetFramework>net10.0</TargetFramework><AssemblyName>Shared.External</AssemblyName></PropertyGroup></Project>");
    File.WriteAllText(Path.Combine(libraryRoot, "Shared.Type.cs"), "namespace Shared { public static class Type { public static void Run() { } } }");
    BuildFixtureProject(libraryProject);

    var externalAssembly = Path.Combine(libraryRoot, "bin", "Release", "net10.0", "Shared.External.dll");
    Require(File.Exists(externalAssembly), "external fixture assembly was not produced");
    var consumerProject = Path.Combine(consumerRoot, "Consumer.csproj");
    File.WriteAllText(consumerProject, $"<Project Sdk=\"Microsoft.NET.Sdk\"><PropertyGroup><TargetFramework>net10.0</TargetFramework><EnableDefaultCompileItems>true</EnableDefaultCompileItems></PropertyGroup><ItemGroup><Reference Include=\"Shared.External\"><HintPath>{externalAssembly.Replace("\\", "/")}</HintPath><Aliases>lib</Aliases></Reference></ItemGroup></Project>");
    File.WriteAllText(Path.Combine(consumerRoot, "Consumer.cs"), "extern alias lib; namespace Shared { public static class Type { public static void Run() { } } } namespace Fixture { public class Local { public void Use() { Shared.Type.Run(); lib::Shared.Type.Run(); } } }");
    return (root, consumerProject);
}

var roots = new List<string>();
GraphObservationBatch? emittedCompilerError = null;
GraphObservationBatch? emittedCanceled = null;
try
{
    AssertEdgeEvidenceStableUnderReordering();
    await AssertNestedProjectUsesRepoRelativeModule();

    var root = CreateFixture("main");
    roots.Add(root);
    var project = Path.Combine(root, "Fixture.csproj");
    var observation1 = await Observe(root, project);
    var observation2 = await Observe(root, project);
    Require(observation1.Nodes.Any(node => node.Key.SymbolKind == "namespace"), "namespace declaration missing");
    Require(observation1.Nodes.Any(node => node.Key.SymbolKind == "type" && node.DisplayName == "Derived"), "type declaration missing");
    Require(observation1.Nodes.Any(node => node.Key.SymbolKind == "method" && node.DisplayName == "Run"), "method declaration missing");
    Require(observation1.Nodes.Any(node => node.Key.SymbolKind == "property" && node.DisplayName == "Value"), "property declaration missing");
    Require(observation1.Nodes.Any(node => node.Key.SymbolKind == "event" && node.DisplayName == "Changed"), "event declaration missing");
    Require(observation1.Edges.Any(edge => edge.Relation == "implements"), "implements edge missing");
    Require(observation1.Edges.Any(edge => edge.Relation == "extends"), "extends edge missing");
    Require(observation1.Edges.Any(edge => edge.Relation == "calls"), "calls edge missing");
    Require(observation1.Edges.Any(edge => edge.Relation == "references"), "references edge missing");
    Require(observation1.Nodes.All(node => node.SourceDigest.Length == 64 && node.SourceDigest.All(Uri.IsHexDigit)), "node digest invalid");
    var nodeRefsByIdentity = observation1.Nodes.OrderBy(node => node.Key.SemanticIdentity, StringComparer.Ordinal).Select(node => node.Ref).ToList();
    var nodeRefsByRef = observation1.Nodes.OrderBy(node => node.Ref, StringComparer.Ordinal).Select(node => node.Ref).ToList();
    Require(nodeRefsByIdentity.Count > 1 && !nodeRefsByIdentity.SequenceEqual(nodeRefsByRef), "main fixture does not exercise divergent identity/hash ordering");
    Require(observation1.Nodes.Select(node => node.Ref).SequenceEqual(nodeRefsByRef), "worker node array is not in model canonical Ref order");
    Require(observation1.Completeness == "complete", "main fixture is not complete");
    Require(observation1.Omissions.Any(item => item.ReasonCode == "implicit_target"), "implicit constructor omission missing");
    AssertSafePaths(observation1);
    AssertCoverage(observation1);
    AssertNonzeroFingerprints(observation1);
    AssertGraphInvariants(observation1);
    var repeatedCallEdge = observation1.Edges.Where(edge => edge.Relation == "calls").OrderByDescending(edge => observation1.Evidence.Count(evidence => evidence.EdgeRef == edge.Ref)).First();
    Require(observation1.Evidence.Count(evidence => evidence.EdgeRef == repeatedCallEdge.Ref) >= 2, "repeated call evidence was dropped");

    var repositoryRoot = Directory.GetCurrentDirectory();
    var repositoryProject = Path.Combine(repositoryRoot, "worker-dotnet", "MiLsp.Worker", "MiLsp.Worker.csproj");
    Require(File.Exists(repositoryProject), "real worker project fixture is missing");
    var repositoryObservation = await Observe(repositoryRoot, repositoryProject);
    Require(repositoryObservation.Completeness == "complete", "real worker project observation is not complete");
    Require(repositoryObservation.Unresolved.Count == 0, "real worker project observation has unresolved records");
    Require(repositoryObservation.Evidence.Count > 0, "real worker project graph provenance is missing");
    AssertGraphInvariants(repositoryObservation);
    Require(repositoryObservation.Nodes.Count >= 100 && repositoryObservation.Edges.Count >= 300, "real worker project graph fixture is unexpectedly small");

    var localRoot = CreateFixture("local-functions", "namespace Fixture; public class LocalSource { public void Use() { void LocalFunction() { } LocalFunction(); System.Action action = () => LocalFunction(); action(); } }");
    roots.Add(localRoot);
    var localObservation = await Observe(localRoot, Path.Combine(localRoot, "Fixture.csproj"));
    Require(localObservation.Completeness == "complete", "local function fixture became partial");
    Require(localObservation.Omissions.Any(item => item.ReasonCode == "unsupported_symbol_kind"), "local function omission is not typed");
    Require(!localObservation.Unresolved.Any(item => item.ReasonCode == "source_endpoint_missing"), "local function produced source endpoint unresolved record");

    var topLevelRoot = CreateFixture("top-level-control", "using System; Console.WriteLine(UnknownTopLevel());");
    roots.Add(topLevelRoot);
    var topLevelObservation = await Observe(topLevelRoot, Path.Combine(topLevelRoot, "Fixture.csproj"));
    Require(topLevelObservation.Completeness == "partial", "top-level compiler failure was accepted as complete");
    Require(topLevelObservation.Omissions.Any(item => item.ReasonCode == "compiler_errors"), "top-level compiler failure lost typed omission");
    Require(topLevelObservation.Unresolved.Any(item => item.ReasonCode == "source_endpoint_missing"), "top-level missing declaration was not preserved as unresolved");
    AssertGraphInvariants(topLevelObservation);

    // Incomplete/error type symbols (empty Name, DocCommentId "T:") must be omitted, not sealed as nodes.
    var incompleteTypeRoot = CreateFixture("incomplete-type", "namespace Fixture; public class Broken : MissingBase { public void Run() { } }");
    roots.Add(incompleteTypeRoot);
    var incompleteTypeObservation = await Observe(incompleteTypeRoot, Path.Combine(incompleteTypeRoot, "Fixture.csproj"));
    Require(incompleteTypeObservation.Completeness == "partial", "incomplete base type fixture was accepted as complete");
    Require(incompleteTypeObservation.Nodes.All(node => !string.IsNullOrWhiteSpace(node.DisplayName)), "empty display_name node escaped producer filter");
    Require(incompleteTypeObservation.Nodes.All(node => node.Key.SemanticIdentity is not ("T:" or "M:")), "bare semantic_identity node escaped producer filter");
    AssertGraphInvariants(incompleteTypeObservation);

    var multiFileRoot = CreateFixture("multi-file", "namespace Fixture; public partial class Split { public void First() { } }");
    roots.Add(multiFileRoot);
    File.WriteAllText(Path.Combine(multiFileRoot, "Fixture.Partial.cs"), "namespace Fixture; public partial class Split { public void Second() { } }");
    var multiFileResponse = await Observe(multiFileRoot, Path.Combine(multiFileRoot, "Fixture.csproj"));
    Require(multiFileResponse.Completeness == "complete", "multi-file declaration owners made observation partial");
    Require(multiFileResponse.Omissions.Any(item => item.ReasonCode == "additional_owner_evidence"), "additional declaration owner omission missing");
    Require(!multiFileResponse.Omissions.Any(item => item.ReasonCode == "declaration_owner_conflict"), "legacy declaration owner conflict emitted");
    AssertGraphInvariants(multiFileResponse);

    var accessorRoot = CreateFixture("accessor", "namespace Fixture; public class Accessor { private int _value; public int Value { get { return _value; } set { _value = value; } } public int Read() => Value; }");
    roots.Add(accessorRoot);
    var accessorResponse = await Observe(accessorRoot, Path.Combine(accessorRoot, "Fixture.csproj"));
    Require(accessorResponse.Completeness == "complete", "property/accessor fixture is not complete");
    AssertGraphInvariants(accessorResponse);

    var unsupportedRoot = CreateFixture("unsupported", "namespace Fixture; public class Unsupported { public void Use() { var local = 1; local.ToString(); } }");
    roots.Add(unsupportedRoot);
    var unsupportedResponse = await Observe(unsupportedRoot, Path.Combine(unsupportedRoot, "Fixture.csproj"));
    var unsupported = unsupportedResponse.Omissions.Where(item => item.ReasonCode == "unsupported_symbol_kind").ToList();
    Require(unsupported.Count > 0 && unsupported.Count <= 2, "unsupported symbol omissions are not bounded");
    Require(unsupported.All(item => item.SubjectKind != "symbol"), "unsupported omission uses invalid subject kind");
    AssertGraphInvariants(unsupportedResponse);

    var projection1 = JsonSerializer.Serialize(new { observation1.SchemaVersion, observation1.RepositoryIdentity, observation1.ProjectOrModule, observation1.SourceFingerprint, observation1.ConfigFingerprint, observation1.Capabilities, observation1.Coverage, observation1.Nodes, observation1.Edges, observation1.Evidence, observation1.Unresolved, observation1.Omissions });
    var projection2 = JsonSerializer.Serialize(new { observation2.SchemaVersion, observation2.RepositoryIdentity, observation2.ProjectOrModule, observation2.SourceFingerprint, observation2.ConfigFingerprint, observation2.Capabilities, observation2.Coverage, observation2.Nodes, observation2.Edges, observation2.Evidence, observation2.Unresolved, observation2.Omissions });
    Require(projection1 == projection2, "semantic projection is not deterministic");

    var otherProject = Path.Combine(root, "Other.csproj");
    File.WriteAllText(otherProject, File.ReadAllText(project));
    var mismatch = await new RoslynService().HandleAsync(Request(root, project, otherProject), CancellationToken.None);
    Require(!mismatch.Ok && mismatch.ErrorCode == "GPH_BACKEND_PROJECT_NOT_FOUND" && mismatch.Observation is null, "exact project mismatch did not reject");

    if (OperatingSystem.IsWindows())
    {
        using var lockedProject = new FileStream(project, FileMode.Open, FileAccess.Read, FileShare.None);
        var unavailable = await new RoslynService().HandleAsync(Request(root, project), CancellationToken.None);
        Require(!unavailable.Ok && unavailable.ErrorCode == "GPH_BACKEND_UNAVAILABLE" && unavailable.Observation is null, "locked project did not return sanitized unavailability");
        var unavailableError = unavailable.Error ?? string.Empty;
        Require(!unavailableError.Contains(root, StringComparison.OrdinalIgnoreCase) && !unavailableError.Contains(project, StringComparison.OrdinalIgnoreCase), "unavailability response exposed a local path");
    }

    using (var canceledSource = new CancellationTokenSource())
    {
        canceledSource.Cancel();
        var canceledResponse = await new RoslynService().HandleAsync(Request(root, project), canceledSource.Token);
        Require(canceledResponse.Ok && canceledResponse.Observation is not null && canceledResponse.Observation.Completeness == "partial", "canceled extraction not partial");
        var canceled = canceledResponse.Observation!;
        emittedCanceled = canceled;
        AssertNonzeroFingerprints(canceled);
        AssertCoverage(canceled);
        AssertSafePaths(canceled);
        Require(canceled.Omissions.Count(omission => omission.ReasonCode == "canceled") == 6, "canceled omissions are not one per capability");
        Require(canceled.Omissions.Where(omission => omission.ReasonCode == "canceled").All(omission => omission.OwnerPath == "Fixture.csproj"), "canceled omission owner is not the project module");
    }

    var errorRoot = CreateFixture("compiler-error", "namespace Fixture; public class Broken { public void M( { } }");
    roots.Add(errorRoot);
    var errorProject = Path.Combine(errorRoot, "Fixture.csproj");
    var errorResponse = await new RoslynService().HandleAsync(Request(errorRoot, errorProject), CancellationToken.None);
    Require(errorResponse.Ok && errorResponse.Observation is not null && errorResponse.Observation.Completeness == "partial", "compiler error batch is not partial");
    emittedCompilerError = errorResponse.Observation;
    Require(errorResponse.Error is null && (errorResponse.Warnings is null || errorResponse.Warnings.All(warning => !warning.Contains("CS", StringComparison.OrdinalIgnoreCase))), "raw compiler diagnostics emitted");
    Require(errorResponse.Observation!.Omissions.Count(omission => omission.ReasonCode == "compiler_errors") == 6, "compiler error omissions are not deduplicated per capability");
    AssertCoverage(errorResponse.Observation);
    AssertSafePaths(errorResponse.Observation);

    var outsideRoot = CreateFixture("linked", "namespace Fixture; public class Local { }");
    roots.Add(outsideRoot);
    var linkedSource = Path.Combine(Path.GetDirectoryName(outsideRoot)!, "linked-outside-" + Guid.NewGuid().ToString("N") + ".cs");
    File.WriteAllText(linkedSource, "namespace Fixture; public class LinkedOutside { }");
    var linkedProject = Path.Combine(outsideRoot, "Fixture.csproj");
    File.WriteAllText(linkedProject, $"<Project Sdk=\"Microsoft.NET.Sdk\"><PropertyGroup><TargetFramework>net10.0</TargetFramework><EnableDefaultCompileItems>false</EnableDefaultCompileItems></PropertyGroup><ItemGroup><Compile Include=\"{linkedSource.Replace("\\", "/")}\" /></ItemGroup></Project>");
    var linkedResponse = await new RoslynService().HandleAsync(Request(outsideRoot, linkedProject), CancellationToken.None);
    Require(linkedResponse.Ok && linkedResponse.Observation is not null && linkedResponse.Observation.Completeness == "partial", "linked outside source did not produce partial batch");
    Require(linkedResponse.Observation!.Omissions.Any(omission => omission.ReasonCode == "linked_outside_root" && omission.OwnerPath == "Fixture.csproj"), "linked outside omission missing or unsafe");
    AssertSafePaths(linkedResponse.Observation);
    File.Delete(linkedSource);

    var externalRoot = CreateFixture("external", "using System; namespace Fixture; public class ExternalConsumer { public void Use() { Console.WriteLine(\"x\"); } }");
    roots.Add(externalRoot);
    var externalResponse = await Observe(externalRoot, Path.Combine(externalRoot, "Fixture.csproj"));
    if (externalResponse.Completeness != "complete") Console.WriteLine(JsonSerializer.Serialize(new { externalResponse.Completeness, externalResponse.Omissions, externalResponse.Unresolved }));
    Require(externalResponse.Completeness == "complete", "external metadata scope made observation partial");
    var externalOmissions = externalResponse.Omissions.Where(omission => omission.ReasonCode == "external_target").ToList();
    Require(externalOmissions.Count > 0 && externalOmissions.Count <= 2, "external metadata omissions are not bounded and deduplicated");
    Require(externalOmissions.All(omission => omission.Capability is "calls" or "references"), "external omission capability is not typed");
    AssertCoverage(externalResponse);

    var externalAliasFixture = CreateExternalAliasFixture();
    roots.Add(externalAliasFixture.Root);
    var externalAliasResponse = await Observe(externalAliasFixture.Root, externalAliasFixture.Project);
    Require(externalAliasResponse.Completeness == "complete", "extern alias collision fixture is not complete");
    Require(externalAliasResponse.Unresolved.Count == 0, "extern alias collision fixture has unresolved records");
    Require(externalAliasResponse.Evidence.Count > 0, "extern alias collision fixture graph provenance is missing");
    var localRun = externalAliasResponse.Nodes.Single(node => node.Key.SemanticIdentity == "M:Shared.Type.Run");
    var localUse = externalAliasResponse.Nodes.Single(node => node.Key.SemanticIdentity == "M:Fixture.Local.Use");
    var localCallEdges = externalAliasResponse.Edges.Where(edge => edge.Relation == "calls" && edge.FromRef == localUse.Ref && edge.ToRef == localRun.Ref).ToList();
    Require(localCallEdges.Count == 1 && externalAliasResponse.Evidence.Count(evidence => evidence.EdgeRef == localCallEdges[0].Ref) == 1, "external alias produced a false local call edge");
    Require(externalAliasResponse.Omissions.Any(omission => omission.Capability == "calls" && omission.ReasonCode == "external_target"), "extern alias collision was not omitted as external_target");
    AssertGraphInvariants(externalAliasResponse);

    var ambiguousRoot = CreateFixture("ambiguous", "namespace Fixture; public class Ambiguous { public void Pick(int value) { } public void Pick(string value) { } public void Use() { Pick(default); } }");
    roots.Add(ambiguousRoot);
    var ambiguousResponse = await Observe(ambiguousRoot, Path.Combine(ambiguousRoot, "Fixture.csproj"));
    Require(ambiguousResponse.Completeness == "partial", "ambiguous overload did not make observation partial");
    var ambiguous = ambiguousResponse.Unresolved.Where(item => item.ReasonCode == "ambiguous_target").ToList();
    Require(ambiguous.Count > 0 && ambiguous.All(item => item.Candidates.Count is > 0 and <= 8), "ambiguous unresolved candidates missing or unbounded");
    Require(ambiguous.All(item => item.Candidates.SequenceEqual(item.Candidates.OrderBy(candidate => candidate, StringComparer.Ordinal))), "ambiguous candidates are not sorted");
    AssertGraphInvariants(ambiguousResponse);

    var lambdaRoot = CreateFixture("lambda", "using System; Action action = () => Console.WriteLine(\"x\");");
    roots.Add(lambdaRoot);
    var lambdaResponse = await Observe(lambdaRoot, Path.Combine(lambdaRoot, "Fixture.csproj"));
    Require(lambdaResponse.Completeness == "partial", "source-owner-missing lambda did not make observation partial");
    Require(lambdaResponse.Unresolved.Any(item => item.ReasonCode == "source_endpoint_missing"), "source endpoint unresolved record missing");
    AssertGraphInvariants(lambdaResponse);

    var copyRoot = Path.Combine(Path.GetTempPath(), "milsp-g2-copy-" + Guid.NewGuid().ToString("N"));
    roots.Add(copyRoot);
    CopyTree(root, copyRoot);
    var firstCopyObservation = await Observe(root, project);
    var secondCopyObservation = await Observe(copyRoot, Path.Combine(copyRoot, "Fixture.csproj"));
    Require(firstCopyObservation.SourceFingerprint == secondCopyObservation.SourceFingerprint, "relocated source fingerprint changed");
    Require(firstCopyObservation.ConfigFingerprint == secondCopyObservation.ConfigFingerprint, "relocated config fingerprint changed");

    var mutationRoot = CreateFixture("mutation");
    roots.Add(mutationRoot);
    var mutationProject = Path.Combine(mutationRoot, "Fixture.csproj");
    var mutationSourcePath = Path.Combine(mutationRoot, "Fixture.cs");
    var mutationService = new RoslynService();
    var beforeSource = await ObserveWithService(mutationService, mutationRoot, mutationProject);
    await AssertStatusReloadConcurrency(mutationService, mutationRoot, mutationProject);
    File.AppendAllText(mutationSourcePath, "\npublic class AddedAfterFingerprint { }");
    var afterSource = await ObserveWithService(mutationService, mutationRoot, mutationProject);
    Require(beforeSource.SourceFingerprint != afterSource.SourceFingerprint, "source byte mutation did not change source fingerprint");
    var addedNode = afterSource.Nodes.SingleOrDefault(node => node.DisplayName == "AddedAfterFingerprint");
    Require(addedNode is not null, "same RoslynService instance returned stale semantic declarations");
    var currentSourceDigest = SourceDigest(mutationSourcePath);
    Require(addedNode!.SourceDigest == currentSourceDigest, "mutated declaration has stale source provenance");
    var addedEvidence = afterSource.Evidence.Where(evidence => evidence.NodeRef == addedNode.Ref).ToList();
    Require(addedEvidence.Count > 0 && addedEvidence.All(evidence => evidence.SourceDigest == currentSourceDigest && evidence.ObservedDigest != evidence.SourceDigest), "mutated declaration provenance is incoherent");
    AssertGraphInvariants(afterSource);
    var beforeConfig = afterSource.ConfigFingerprint;
    File.WriteAllText(mutationProject, File.ReadAllText(mutationProject).Replace("</PropertyGroup>", "<LangVersion>preview</LangVersion></PropertyGroup>", StringComparison.Ordinal));
    var afterConfig = await Observe(mutationRoot, mutationProject);
    Require(beforeConfig != afterConfig.ConfigFingerprint, "project/compiler option mutation did not change config fingerprint");

    if (OperatingSystem.IsWindows())
    {
        var lockRoot = CreateFixture("source-lock");
        roots.Add(lockRoot);
        var lockProject = Path.Combine(lockRoot, "Fixture.csproj");
        var warmedService = new RoslynService();
        var warmed = await warmedService.HandleAsync(Request(lockRoot, lockProject), CancellationToken.None);
        Require(warmed.Ok && warmed.Observation is not null, "preload lock observation failed");
        var lockedSourcePath = Path.Combine(lockRoot, "Fixture.cs");
        using (new FileStream(lockedSourcePath, FileMode.Open, FileAccess.Read, FileShare.None))
        {
            var locked = await warmedService.HandleAsync(Request(lockRoot, lockProject), CancellationToken.None);
            Require(!locked.Ok && locked.ErrorCode == "GPH_BACKEND_UNAVAILABLE" && locked.Observation is null, "source lock did not return sanitized backend unavailable");
            Require(!locked.Error!.Contains(lockRoot, StringComparison.OrdinalIgnoreCase) && !locked.Error.Contains(lockProject, StringComparison.OrdinalIgnoreCase) && !locked.Error.Contains(lockedSourcePath, StringComparison.OrdinalIgnoreCase), "source lock response leaked a path");
        }
    }

    var missing = await new RoslynService().HandleAsync(Request(root, project, project, null), CancellationToken.None);
    Require(!missing.Ok && missing.ErrorCode == "GPH_BACKEND_PROVENANCE_MISSING", "missing provenance gate failed");
    Require(emittedCompilerError is not null && emittedCanceled is not null, "emission observations missing");
    var compilerError = emittedCompilerError ?? throw new InvalidOperationException("emission observations missing");
    var emittedCanceledBatch = emittedCanceled ?? throw new InvalidOperationException("emission observations missing");
    EmitObservationsIfRequested(observation1, compilerError, emittedCanceledBatch);
    Console.WriteLine("PASS graph observation provenance contract");
}
finally
{
    foreach (var root in roots)
    {
        try { Directory.Delete(root, true); } catch { }
    }
}
