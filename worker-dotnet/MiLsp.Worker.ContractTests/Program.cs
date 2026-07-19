using System.Text.Json;
using MiLsp.Worker;

static void Require(bool condition, string message) { if (!condition) throw new InvalidOperationException(message); }
static WorkerRequest Request(string root, string project, string? repository = "example.invalid/repo") => new(
    "mi-lsp-v1.1", "graph_observe", root, "fixture", "roslyn", "repo", "repo", root, "fixture", project, "project", new Dictionary<string, JsonElement>
    {
        ["repository_identity"] = JsonSerializer.SerializeToElement(repository),
        ["project_or_module"] = JsonSerializer.SerializeToElement("Fixture")
    });

var root = Path.Combine(Path.GetTempPath(), "milsp-g2-" + Guid.NewGuid().ToString("N"));
Directory.CreateDirectory(root);
try
{
    var project = Path.Combine(root, "Fixture.csproj");
    await File.WriteAllTextAsync(project, "<Project Sdk=\"Microsoft.NET.Sdk\"><PropertyGroup><TargetFramework>net10.0</TargetFramework><EnableDefaultCompileItems>true</EnableDefaultCompileItems></PropertyGroup></Project>");
    await File.WriteAllTextAsync(Path.Combine(root, "Fixture.cs"), "namespace Fixture; public interface IContract { void Run(); } public class Base { public virtual void BaseRun() { } } public class Derived : Base, IContract { public int Value { get; set; } public event Action? Changed; public void Run() { BaseRun(); } } public class Consumer { public void Use(Derived d) { d.Run(); var x = new Derived(); x.Value = 1; } }");
    var service = new RoslynService();
    var response1 = await service.HandleAsync(Request(root, project), CancellationToken.None);
    var response2 = await service.HandleAsync(Request(root, project), CancellationToken.None);
    Require(response1.Ok && response1.Observation is not null, "observation response missing");
    Require(response1.Observation!.Nodes.Any(node => node.Key.SymbolKind == "namespace"), "namespace declaration missing");
    Require(response1.Observation.Nodes.Any(node => node.Key.SymbolKind == "type" && node.DisplayName == "Derived"), "type declaration missing");
    Require(response1.Observation.Nodes.Any(node => node.Key.SymbolKind == "method" && node.DisplayName == "Run"), "method declaration missing");
    Require(response1.Observation.Nodes.Any(node => node.Key.SymbolKind == "property" && node.DisplayName == "Value"), "property declaration missing");
    Require(response1.Observation.Nodes.Any(node => node.Key.SymbolKind == "event" && node.DisplayName == "Changed"), "event declaration missing");
    Require(response1.Observation.Edges.Any(edge => edge.Relation == "implements"), "implements edge missing");
    Require(response1.Observation.Edges.Any(edge => edge.Relation == "extends"), "extends edge missing");
    Require(response1.Observation.Edges.Any(edge => edge.Relation == "calls"), "calls edge missing");
    Require(response1.Observation.Edges.Any(edge => edge.Relation == "references"), "references edge missing");
    Require(response1.Observation.Nodes.All(node => node.SourceDigest.Length == 64 && node.SourceDigest.All(Uri.IsHexDigit)), "node digest invalid");
    Require(response1.Observation.Evidence.All(item => !Path.IsPathRooted(item.SourceUri) && item.Range is null || item.Range!.StartLine > 0), "evidence path/range invalid");
    Require(response1.Observation.Coverage.Count == 6 && response1.Observation.Capabilities.Count == 6, "coverage matrix invalid");
    var projection = JsonSerializer.Serialize(new { response1.Observation.SchemaVersion, response1.Observation.RepositoryIdentity, response1.Observation.ProjectOrModule, response1.Observation.Capabilities, response1.Observation.Coverage, response1.Observation.Nodes, response1.Observation.Edges, response1.Observation.Evidence, response1.Observation.Unresolved, response1.Observation.Omissions });
    var projection2 = JsonSerializer.Serialize(new { response2.Observation!.SchemaVersion, response2.Observation.RepositoryIdentity, response2.Observation.ProjectOrModule, response2.Observation.Capabilities, response2.Observation.Coverage, response2.Observation.Nodes, response2.Observation.Edges, response2.Observation.Evidence, response2.Observation.Unresolved, response2.Observation.Omissions });
    Require(projection == projection2, "semantic projection is not deterministic");
    var missing = await service.HandleAsync(Request(root, project, null), CancellationToken.None);
    Require(!missing.Ok && missing.ErrorCode == "GPH_BACKEND_PROVENANCE_MISSING", "missing provenance gate failed");
    using var canceled = new CancellationTokenSource(); canceled.Cancel();
    var partial = await service.HandleAsync(Request(root, project), canceled.Token);
    Require(partial.Observation is not null && partial.Observation.Completeness == "partial", "canceled extraction not partial");
    Console.WriteLine("PASS graph observation contract");
}
finally { try { Directory.Delete(root, true); } catch { } }
