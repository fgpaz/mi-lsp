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

static GraphObservationBatch RequireObservation(WorkerResponse response)
{
    Require(response.Ok && response.Observation is not null, "observation response missing");
    return response.Observation ?? throw new InvalidOperationException("observation response missing");
}

static async Task<GraphObservationBatch> Observe(string root, string project, CancellationToken cancellationToken = default)
{
    var response = await new RoslynService().HandleAsync(Request(root, project), cancellationToken);
    return RequireObservation(response);
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

var roots = new List<string>();
GraphObservationBatch? emittedCompilerError = null;
GraphObservationBatch? emittedCanceled = null;
try
{
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
    Require(observation1.Completeness == "complete", "main fixture is not complete");
    Require(observation1.Omissions.Any(item => item.ReasonCode == "implicit_target"), "implicit constructor omission missing");
    AssertSafePaths(observation1);
    AssertCoverage(observation1);
    AssertNonzeroFingerprints(observation1);
    AssertGraphInvariants(observation1);
    var repeatedCallEdge = observation1.Edges.Where(edge => edge.Relation == "calls").OrderByDescending(edge => observation1.Evidence.Count(evidence => evidence.EdgeRef == edge.Ref)).First();
    Require(observation1.Evidence.Count(evidence => evidence.EdgeRef == repeatedCallEdge.Ref) >= 2, "repeated call evidence was dropped");

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
    var beforeSource = await Observe(mutationRoot, mutationProject);
    File.AppendAllText(Path.Combine(mutationRoot, "Fixture.cs"), "\npublic class AddedAfterFingerprint { }");
    var afterSource = await Observe(mutationRoot, mutationProject);
    Require(beforeSource.SourceFingerprint != afterSource.SourceFingerprint, "source byte mutation did not change source fingerprint");
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
