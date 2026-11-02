using System.Security.Cryptography;
using System.Text;
using System.Xml;
using Microsoft.Build.Construction;
using Microsoft.Build.Definition;
using Microsoft.Build.Evaluation;
using Microsoft.Build.Evaluation.Context;
using Microsoft.Build.Execution;

namespace MiLsp.Worker;

/// <summary>
/// Owns one MSBuild evaluation rooted in captured XML and an injected public
/// filesystem context. Filesystem coverage remains partial by construction.
/// </summary>
internal sealed class CapturedMsBuildEvaluation : IDisposable
{
    private const int MaximumXmlBytes = 16 * 1024 * 1024;
    private const int MaximumImportDiscoveryRounds = 4;
    private const int MaximumCapturedInputs = 4096;
    private const long MaximumCapturedXmlBytes = 64L * 1024 * 1024;
    private readonly ProjectCollection _collection;
    private readonly EvaluationContext _evaluationContext;

    private CapturedMsBuildEvaluation(ProjectCollection collection, EvaluationContext evaluationContext,
        Project project, ProjectInstance instance, IReadOnlyList<CapturedXmlInput> xmlInputs,
        IReadOnlyList<string> unownedImports, string completeness, string snapshotId,
        CapturedEvaluationFileSystem fileSystem)
    {
        _collection = collection;
        _evaluationContext = evaluationContext;
        Project = project;
        Instance = instance;
        XmlInputs = xmlInputs;
        UnownedImports = unownedImports;
        Completeness = completeness;
        SnapshotId = snapshotId;
        FileSystem = fileSystem;
        EvaluationId = project.LastEvaluationId;
    }

    internal Project Project { get; }
    internal ProjectInstance Instance { get; }
    internal int EvaluationId { get; }
    internal string SnapshotId { get; }
    internal IReadOnlyList<CapturedXmlInput> XmlInputs { get; }
    internal IReadOnlyList<string> UnownedImports { get; }
    internal string Completeness { get; }
    internal CapturedEvaluationFileSystem FileSystem { get; }
    internal bool XmlImportsCaptured => UnownedImports.Count == 0 && Completeness == "xml-imports-captured-incomplete-other-inputs";
    // Evaluator SDK/host assembly reads are not a complete byte-level receipt.
    internal bool CompleteControlInputs => false;

    internal async Task<CapturedRoslynCompilation> CreateRoslynCompilationAsync(CancellationToken cancellationToken)
    {
        if (!FileSystem.IsSealed || Project.LastEvaluationId != EvaluationId || Instance.EvaluationId != EvaluationId)
            throw new InvalidOperationException("Controlled evaluation snapshot is not stable for Roslyn compilation");
        CapturedMsBuildToolchain.RequireHost(
            Project.GetPropertyValue("MSBuildToolsPath"),
            Project.GetPropertyValue("MSBuildSDKsPath"));
        var referenceSnapshotId = CapturedRoslynCompilation.EvaluatedReferenceSnapshotId(this, Instance);
        var compilation = await CapturedRoslynCompilation.CreateAsync(this, Instance, referenceSnapshotId, cancellationToken).ConfigureAwait(false);
        if (compilation.EvaluationId != EvaluationId || compilation.EvaluationSnapshotId != SnapshotId ||
            compilation.ReferenceResolutionSnapshotId != referenceSnapshotId || Project.LastEvaluationId != EvaluationId)
        {
            compilation.Dispose();
            throw new InvalidOperationException("Roslyn compilation is not bound to the controlled MSBuild evaluation snapshot");
        }
        return compilation;
    }

    /// <summary>
    /// Bounded discovery/evaluation rounds. Discovery-round import paths are
    /// candidates only; final consumed XML identity is checked on the controlled
    /// evaluation's ImportedProject objects by ReferenceEquals.
    /// </summary>
    internal static CapturedMsBuildEvaluation Evaluate(
        string projectPath,
        IDictionary<string, string>? globalProperties = null)
    {
        var root = CaptureXml(projectPath);
        ValidateStaticProjectXml(root);
        var captured = new Dictionary<string, CapturedXmlInput>(PathComparer) { [root.FullPath] = root };
        var candidateImports = new HashSet<string>(PathComparer);
        var lastUnowned = Array.Empty<string>();
        long capturedBytes = root.Length;

        for (var round = 0; round < MaximumImportDiscoveryRounds; round++)
        {
            var properties = globalProperties is null
                ? null
                : new Dictionary<string, string>(globalProperties, StringComparer.OrdinalIgnoreCase);
            var collection = new ProjectCollection(properties);
            CapturedEvaluationFileSystem? fileSystem = null;
            EvaluationContext? evaluationContext = null;
            try
            {
                var elements = new Dictionary<string, ProjectRootElement>(PathComparer);
                foreach (var input in captured.Values)
                    elements.Add(input.FullPath, CreateRootElement(input, collection));
                if (!elements.TryGetValue(root.FullPath, out var rootElement))
                    throw new InvalidOperationException("Captured root XML was not loaded into the evaluation collection");

                fileSystem = new CapturedEvaluationFileSystem();
                evaluationContext = EvaluationContext.Create(EvaluationContext.SharingPolicy.Shared, fileSystem);
                var options = new ProjectOptions
                {
                    ProjectCollection = collection,
                    GlobalProperties = properties,
                    EvaluationContext = evaluationContext
                };
                var project = Project.FromProjectRootElement(rootElement, options);
                var discovered = project.Imports
                    .Select(import => import.ImportedProject)
                    .Where(imported => !string.IsNullOrWhiteSpace(imported.FullPath))
                    .Take(MaximumCapturedInputs + 1)
                    .ToArray();
                if (discovered.Length != 0)
                    throw new InvalidOperationException("Controlled evaluation rejects imported projects before target execution");
                if (discovered.Length > MaximumCapturedInputs)
                    throw new IOException("MSBuild XML import lookup exceeded its capture bound");
                var nextCandidates = discovered.Select(imported => Path.GetFullPath(imported.FullPath!))
                    .Where(path => !captured.ContainsKey(path)).Distinct(PathComparer).ToArray();
                var identityMismatches = discovered
                    .Where(imported => !elements.Values.Any(element => ReferenceEquals(element, imported)))
                    .Select(imported => imported.FullPath ?? "<import-without-path>")
                    .Distinct(PathComparer).ToArray();

                if (nextCandidates.Length > 0)
                {
                    candidateImports.UnionWith(nextCandidates);
                    lastUnowned = nextCandidates;
                    collection.Dispose();
                    foreach (var candidate in nextCandidates)
                    {
                        if (!File.Exists(candidate)) continue;
                        try
                        {
                            var input = CaptureXml(candidate);
                            if (captured.Count >= MaximumCapturedInputs || capturedBytes + input.Length > MaximumCapturedXmlBytes)
                                throw new IOException("MSBuild XML captured inputs exceeded their total bound");
                            captured.Add(input.FullPath, input);
                            capturedBytes += input.Length;
                        }
                        catch (IOException) { }
                        catch (UnauthorizedAccessException) { }
                        catch (XmlException) { }
                    }
                    continue;
                }

                var evaluationIdBeforeInstance = project.LastEvaluationId;
                var instance = project.CreateProjectInstance();
                var evaluationMatches = evaluationIdBeforeInstance == project.LastEvaluationId;
                if (!evaluationMatches)
                    throw new IOException("Controlled project reevaluated while creating its ProjectInstance");

                fileSystem.SealAndValidate();
                var unowned = identityMismatches
                    .Concat(lastUnowned.Where(path => !captured.ContainsKey(Path.GetFullPath(path))))
                    .Distinct(PathComparer).ToArray();
                var status = unowned.Length == 0
                    ? "xml-imports-captured-incomplete-other-inputs"
                    : "incomplete-import-or-snapshot-binding";
                var evaluationId = project.LastEvaluationId;
                var snapshotId = ComputeSnapshotId(evaluationId, root.FullPath, properties,
                    captured.Values, fileSystem.Inputs, fileSystem.Observations, fileSystem.Calls);
                return new CapturedMsBuildEvaluation(collection, evaluationContext, project, instance,
                    captured.Values.OrderBy(input => input.FullPath, PathComparer).ToArray(), unowned,
                    status, snapshotId, fileSystem);
            }
            catch
            {
                collection.Dispose();
                throw;
            }
        }

        throw new IOException($"MSBuild XML import discovery did not converge within {MaximumImportDiscoveryRounds} rounds; candidates={candidateImports.Count}");
    }

    public void Dispose() => _collection.Dispose();

    private static ProjectRootElement CreateRootElement(CapturedXmlInput input, ProjectCollection collection)
    {
        using var stream = new MemoryStream(input.Bytes, writable: false);
        using var reader = XmlReader.Create(stream, new XmlReaderSettings
        {
            DtdProcessing = DtdProcessing.Prohibit,
            XmlResolver = null,
            CloseInput = false
        });
        var root = ProjectRootElement.Create(reader, collection, preserveFormatting: false);
        root.FullPath = input.FullPath;
        return root;
    }

    private static void ValidateStaticProjectXml(CapturedXmlInput input)
    {
        using var stream = new MemoryStream(input.Bytes, writable: false);
        using var reader = XmlReader.Create(stream, new XmlReaderSettings
        {
            DtdProcessing = DtdProcessing.Prohibit,
            XmlResolver = null,
            CloseInput = false
        });
        var document = System.Xml.Linq.XDocument.Load(reader, System.Xml.Linq.LoadOptions.None);
        var project = document.Root;
        if (project is null || project.Name.LocalName != "Project" || project.Attribute("Sdk") is not null)
            throw new InvalidOperationException("Controlled evaluation requires a static, SDK-free Project root");
        var xmlNamespace = project.Name.NamespaceName;
        if ((xmlNamespace != "" && xmlNamespace != "http://schemas.microsoft.com/developer/msbuild/2003") ||
            project.DescendantsAndSelf().Any(element => element.Name.NamespaceName != xmlNamespace ||
                element.Attributes().Any(attribute => !attribute.IsNamespaceDeclaration && attribute.Name.NamespaceName != "")))
            throw new InvalidOperationException("Controlled evaluation rejects unknown XML namespaces");
        var forbidden = new HashSet<string>(StringComparer.OrdinalIgnoreCase)
        {
            "Import", "Sdk", "Target", "UsingTask", "Choose", "ItemDefinitionGroup"
        };
        if (project.Descendants().Any(element => forbidden.Contains(element.Name.LocalName)) ||
            project.DescendantsAndSelf().SelectMany(element => element.Attributes()).Any(attribute => attribute.Name.LocalName == "Condition"))
            throw new InvalidOperationException("Controlled evaluation rejects imports, targets, tasks and conditional project structure");
        // Inspect XML-decoded values, not a UTF-8 rendering of the raw bytes:
        // UTF-16 and character references must not hide MSBuild expressions.
        var values = project.DescendantNodes().OfType<System.Xml.Linq.XText>().Select(node => node.Value)
            .Concat(project.DescendantsAndSelf().SelectMany(element => element.Attributes()).Select(attribute => attribute.Value));
        if (values.Any(value => value.IndexOfAny(new[] { '$', '@', '%' }) >= 0))
            throw new InvalidOperationException("Controlled evaluation rejects decoded MSBuild expressions and escapes");
        var directory = Path.GetDirectoryName(input.FullPath)
            ?? throw new InvalidOperationException("Controlled project has no directory");
        var itemCount = 0;
        foreach (var child in project.Elements())
        {
            if (child.Name.LocalName == "PropertyGroup") continue;
            if (child.Name.LocalName != "ItemGroup")
                throw new InvalidOperationException("Controlled evaluation contains an unsupported project element");
            foreach (var item in child.Elements())
            {
                if (++itemCount > 4096 || item.Name.LocalName is not
                    ("Compile" or "ReferencePath" or "AdditionalFiles" or "AnalyzerConfigFiles" or "EmbeddedResource"))
                    throw new InvalidOperationException("Controlled evaluation contains unsupported or excessive compiler items");
                var include = item.Attribute("Include")?.Value;
                if (string.IsNullOrWhiteSpace(include) || include.IndexOfAny(new[] { '*', '?', ';' }) >= 0 ||
                    item.Attributes().Any(attribute => attribute.Name != "Include"))
                    throw new InvalidOperationException("Controlled evaluation requires one literal Include per compiler item");
                // MSBuild normalizes backslashes on Unix too. Check the same
                // coordinates before native evaluation can enumerate anything.
                var full = Path.GetFullPath(include.Replace('\\', '/'), directory);
                var relative = Path.GetRelativePath(directory, full);
                if (relative is "." or ".." || relative.StartsWith(".." + Path.DirectorySeparatorChar, StringComparison.Ordinal) ||
                    Path.IsPathFullyQualified(relative) || GraphDeltaObservation.HasReparsePointInExistingPath(directory, full))
                    throw new InvalidOperationException("Controlled evaluation requires unlinked project-local compiler inputs");
            }
        }
    }

    private static CapturedXmlInput CaptureXml(string path)
    {
        var fullPath = Path.GetFullPath(path);
        EnsureNoReparsePoint(fullPath);
        var before = new FileInfo(fullPath);
        before.Refresh();
        if (!before.Exists) throw new FileNotFoundException("MSBuild XML input does not exist", fullPath);
        if (before.Length > MaximumXmlBytes) throw new IOException("MSBuild XML input exceeds the capture size limit");
        var lengthBefore = before.Length;
        var modifiedBefore = before.LastWriteTimeUtc;
        byte[] bytes;
        using (var input = new FileStream(fullPath, FileMode.Open, FileAccess.Read, FileShare.Read, 64 * 1024, FileOptions.SequentialScan))
        using (var buffer = new MemoryStream())
        {
            var chunk = new byte[64 * 1024];
            while (true)
            {
                var available = MaximumXmlBytes + 1L - buffer.Length;
                if (available <= 0) throw new IOException("MSBuild XML input exceeds the capture size limit");
                var count = input.Read(chunk, 0, (int)Math.Min(chunk.Length, available));
                if (count == 0) break;
                buffer.Write(chunk, 0, count);
                if (buffer.Length > MaximumXmlBytes) throw new IOException("MSBuild XML input exceeds the capture size limit");
            }
            bytes = buffer.ToArray();
        }
        EnsureNoReparsePoint(fullPath);
        var after = new FileInfo(fullPath);
        after.Refresh();
        if (!after.Exists || after.Length != lengthBefore || after.LastWriteTimeUtc != modifiedBefore || bytes.LongLength != lengthBefore)
            throw new IOException("MSBuild XML input changed while being captured");
        return new CapturedXmlInput(fullPath, bytes, Convert.ToHexString(SHA256.HashData(bytes)), lengthBefore, modifiedBefore);
    }

    private static string ComputeSnapshotId(int evaluationId, string rootPath,
        IDictionary<string, string>? globalProperties, IEnumerable<CapturedXmlInput> xmlInputs,
        IEnumerable<CapturedFileSystemInput> fileInputs, IEnumerable<CapturedFileSystemObservation> observations,
        IEnumerable<CapturedFileSystemCall> calls)
    {
        var material = new StringBuilder().Append("msbuild-evaluation-snapshot-v1\0")
            .Append(evaluationId).Append('\0').Append(rootPath).Append('\0');
        if (globalProperties is not null)
            foreach (var property in globalProperties.OrderBy(item => item.Key, StringComparer.OrdinalIgnoreCase))
                material.Append("property:").Append(property.Key).Append(':')
                    .Append(Convert.ToHexString(SHA256.HashData(Encoding.UTF8.GetBytes(property.Value)))).Append('\0');
        foreach (var input in xmlInputs.OrderBy(item => item.FullPath, PathComparer))
            material.Append("xml:").Append(input.FullPath).Append(':').Append(input.Sha256).Append('\0');
        foreach (var input in fileInputs.OrderBy(item => item.FullPath, PathComparer))
            material.Append("read:").Append(input.FullPath).Append(':').Append(input.Sha256).Append(':').Append(input.Length).Append('\0');
        foreach (var observation in observations.OrderBy(item => item.Path, PathComparer).ThenBy(item => item.State, StringComparer.Ordinal))
            material.Append("state:").Append(observation.Path).Append(':').Append(observation.State).Append(':')
                .Append(observation.Metadata).Append(':').Append(observation.LastWriteUtc?.Ticks).Append(':')
                .Append(observation.Digest).Append(':').Append(string.Join(",", observation.Methods)).Append('\0');
        foreach (var call in calls.OrderBy(item => item.Path, PathComparer).ThenBy(item => item.Method, StringComparer.Ordinal).ThenBy(item => item.State, StringComparer.Ordinal))
            material.Append("call:").Append(call.Path).Append(':').Append(call.Method).Append(':').Append(call.State).Append('\0');
        return Convert.ToHexString(SHA256.HashData(Encoding.UTF8.GetBytes(material.ToString())));
    }

    private static void EnsureNoReparsePoint(string fullPath)
    {
        var current = Path.GetPathRoot(fullPath) ?? throw new IOException("MSBuild XML path has no root");
        foreach (var segment in fullPath[current.Length..].Split(new[] { Path.DirectorySeparatorChar, Path.AltDirectorySeparatorChar }, StringSplitOptions.RemoveEmptyEntries))
        {
            current = Path.Combine(current, segment);
            var attributes = File.GetAttributes(current);
            if ((attributes & FileAttributes.ReparsePoint) != 0)
                throw new IOException("Linked MSBuild XML paths are not captured");
        }
    }

    private static StringComparer PathComparer => OperatingSystem.IsWindows() ? StringComparer.OrdinalIgnoreCase : StringComparer.Ordinal;
}

internal sealed record CapturedXmlInput(string FullPath, byte[] Bytes, string Sha256, long Length, DateTime LastWriteTimeUtc);
