using System.Collections.Immutable;
using System.Security.Cryptography;
using System.Text;
using Microsoft.CodeAnalysis;
using Microsoft.CodeAnalysis.CSharp;
using Microsoft.CodeAnalysis.Text;

namespace MiLsp.Worker;

/// <summary>
/// A Roslyn snapshot constructed from one product-owned evaluated instance, not
/// from a second opaque MSBuildWorkspace evaluation. This is a compilation-input
/// snapshot, NOT a complete control-input/read-set receipt.
/// </summary>
internal sealed class CapturedRoslynCompilation : IDisposable
{
    private const int MaximumInputs = 4096;
    private const long MaximumSourceBytes = 64L * 1024 * 1024;
    private const long MaximumReferenceBytes = 256L * 1024 * 1024;
    private readonly AdhocWorkspace _workspace;

    private CapturedRoslynCompilation(AdhocWorkspace workspace, Project project,
        Compilation compilation, int evaluationId, string evaluationSnapshotId, string referenceResolutionSnapshotId,
        string snapshotId, IReadOnlyList<CapturedCompilationInput> inputs,
        IReadOnlyDictionary<string, string> compilerProperties, bool sourceAndReferenceBytesCaptured)
    {
        _workspace = workspace;
        Project = project;
        Compilation = compilation;
        EvaluationId = evaluationId;
        EvaluationSnapshotId = evaluationSnapshotId;
        ReferenceResolutionSnapshotId = referenceResolutionSnapshotId;
        SnapshotId = snapshotId;
        Inputs = inputs;
        CompilerProperties = compilerProperties;
        SourceAndReferenceBytesCaptured = sourceAndReferenceBytesCaptured;
    }

    internal Project Project { get; }
    internal Compilation Compilation { get; }
    internal int EvaluationId { get; }
    internal string EvaluationSnapshotId { get; }
    internal string ReferenceResolutionSnapshotId { get; }
    internal string SnapshotId { get; }
    internal IReadOnlyList<CapturedCompilationInput> Inputs { get; }
    internal IReadOnlyDictionary<string, string> CompilerProperties { get; }
    internal bool SourceAndReferenceBytesCaptured { get; }
    // MSBuild host DLL/environment read sets remain incomplete even when the
    // selected compiler inputs below are byte-bound.
    internal bool CompleteControlInputs => false;

    internal static async Task<CapturedRoslynCompilation> CreateAsync(
        CapturedMsBuildEvaluation evaluation, Microsoft.Build.Execution.ProjectInstance resolvedInstance,
        string referenceResolutionSnapshotId, CancellationToken cancellationToken)
    {
        cancellationToken.ThrowIfCancellationRequested();
        var evaluationId = evaluation.EvaluationId;
        if (evaluation.Project.LastEvaluationId != evaluationId || resolvedInstance.EvaluationId != evaluationId ||
            !string.Equals(Path.GetFullPath(resolvedInstance.FullPath), Path.GetFullPath(evaluation.Instance.FullPath),
                OperatingSystem.IsWindows() ? StringComparison.OrdinalIgnoreCase : StringComparison.Ordinal))
            throw new InvalidOperationException("Controlled MSBuild evaluation changed before compilation capture");

        // Deep-copy the native ResolveReferences state while preserving the
        // original evaluation identity; source selection remains from this state.
        var instance = resolvedInstance;
        var directory = Path.GetDirectoryName(instance.FullPath)
            ?? throw new InvalidOperationException("Controlled project has no directory");
        var host = CapturedMsBuildToolchain.RequireHost(
            instance.GetPropertyValue("MSBuildToolsPath"), instance.GetPropertyValue("MSBuildSDKsPath"));
        var compileItems = instance.GetItems("Compile").ToArray();
        var referenceItems = instance.GetItems("ReferencePath").ToArray();
        var additionalItems = instance.GetItems("AdditionalFiles").ToArray();
        var analyzerConfigItems = instance.GetItems("AnalyzerConfigFiles").ToArray();
        var resourceItems = instance.GetItems("EmbeddedResource").ToArray();
        if (instance.GetItems("Analyzer").Any())
            throw new InvalidOperationException("Controlled Roslyn producer does not execute analyzers or source generators");
        if (instance.GetItems("ProjectReference").Any() || instance.GetItems("COMReference").Any() ||
            instance.GetItems("NativeReference").Any())
            throw new InvalidOperationException("Controlled Roslyn producer rejects project, COM and native references");
        if (compileItems.Length == 0 || compileItems.Length > MaximumInputs || referenceItems.Length > MaximumInputs)
            throw new InvalidOperationException("Controlled compilation source/reference selection is incomplete or exceeds its bound");
        if (referenceItems.Length == 0)
            throw new InvalidOperationException("Controlled evaluated instance has no resolved ReferencePath; resolution must come from the same controlled context");

        var languageText = instance.GetPropertyValue("LangVersion");
        if (!LanguageVersionFacts.TryParse(string.IsNullOrEmpty(languageText) ? "default" : languageText, out var languageVersion))
            throw new InvalidOperationException("Controlled compiler language version is unsupported");
        var symbols = instance.GetPropertyValue("DefineConstants")
            .Split(new[] { ';', ',' }, StringSplitOptions.RemoveEmptyEntries | StringSplitOptions.TrimEntries);
        if (symbols.Any(symbol => !SyntaxFacts.IsValidIdentifier(symbol)))
            throw new InvalidOperationException("Controlled compiler defines are invalid");
        var parseOptions = new CSharpParseOptions(languageVersion, DocumentationMode.Parse,
            SourceCodeKind.Regular, symbols);
        var nullableText = instance.GetPropertyValue("Nullable");
        var nullable = nullableText.ToLowerInvariant() switch
        {
            "" or "disable" => NullableContextOptions.Disable,
            "enable" => NullableContextOptions.Enable,
            "warnings" => NullableContextOptions.Warnings,
            "annotations" => NullableContextOptions.Annotations,
            _ => throw new InvalidOperationException("Controlled nullable compiler option is unsupported")
        };
        var outputKind = instance.GetPropertyValue("OutputType").ToLowerInvariant() switch
        {
            "" or "library" => OutputKind.DynamicallyLinkedLibrary,
            "exe" => OutputKind.ConsoleApplication,
            "winexe" => OutputKind.WindowsApplication,
            _ => throw new InvalidOperationException("Controlled compiler output kind is unsupported")
        };
        var platformText = instance.GetPropertyValue("PlatformTarget");
        if (string.IsNullOrWhiteSpace(platformText)) platformText = "AnyCpu";
        if (!Enum.TryParse<Platform>(platformText, ignoreCase: true, out var platform))
            throw new InvalidOperationException("Controlled compiler platform is unsupported");
        var optimize = string.Equals(instance.GetPropertyValue("Optimize"), "true", StringComparison.OrdinalIgnoreCase);
        var warningLevelText = instance.GetPropertyValue("WarningLevel");
        var warningLevel = string.IsNullOrWhiteSpace(warningLevelText) ? 4 :
            int.TryParse(warningLevelText, System.Globalization.NumberStyles.None, System.Globalization.CultureInfo.InvariantCulture, out var parsedWarningLevel) ? parsedWarningLevel :
            throw new InvalidOperationException("Controlled compiler warning level is invalid");
        if (warningLevel is < 0 or > 999) throw new InvalidOperationException("Controlled compiler warning level is outside its bound");
        var diagnosticOptions = new Dictionary<string, ReportDiagnostic>(StringComparer.OrdinalIgnoreCase);
        AddDiagnosticOptions(diagnosticOptions, instance.GetPropertyValue("NoWarn"), ReportDiagnostic.Suppress);
        AddDiagnosticOptions(diagnosticOptions, instance.GetPropertyValue("WarningsAsErrors"), ReportDiagnostic.Error);
        AddDiagnosticOptions(diagnosticOptions, instance.GetPropertyValue("WarningsNotAsErrors"), ReportDiagnostic.Warn);
        var generalDiagnostic = string.Equals(instance.GetPropertyValue("TreatWarningsAsErrors"), "true", StringComparison.OrdinalIgnoreCase)
            ? ReportDiagnostic.Error : ReportDiagnostic.Default;
        var options = new CSharpCompilationOptions(outputKind)
            .WithNullableContextOptions(nullable)
            .WithAllowUnsafe(string.Equals(instance.GetPropertyValue("AllowUnsafeBlocks"), "true", StringComparison.OrdinalIgnoreCase))
            .WithOverflowChecks(string.Equals(instance.GetPropertyValue("CheckForOverflowUnderflow"), "true", StringComparison.OrdinalIgnoreCase))
            .WithPlatform(platform)
            .WithOptimizationLevel(optimize ? OptimizationLevel.Release : OptimizationLevel.Debug)
            .WithWarningLevel(warningLevel)
            .WithGeneralDiagnosticOption(generalDiagnostic)
            .WithSpecificDiagnosticOptions(diagnosticOptions.ToImmutableDictionary(StringComparer.OrdinalIgnoreCase))
            .WithMainTypeName(NullIfWhiteSpace(instance.GetPropertyValue("StartupObject")))
            .WithDeterministic(ParseOptionalBool(instance.GetPropertyValue("Deterministic"), false))
            .WithConcurrentBuild(ParseOptionalBool(instance.GetPropertyValue("ConcurrentBuild"), true));
        var compilerProperties = ReadCompilerProperties(instance);
        var sourceAndReferenceBytesCaptured = true;

        var workspace = new AdhocWorkspace();
        try
        {
            var name = instance.GetPropertyValue("AssemblyName");
            if (string.IsNullOrWhiteSpace(name)) name = Path.GetFileNameWithoutExtension(instance.FullPath);
            var project = workspace.AddProject(ProjectInfo.Create(ProjectId.CreateNewId(),
                VersionStamp.Create(), name, name, LanguageNames.CSharp, filePath: instance.FullPath,
                compilationOptions: options, parseOptions: parseOptions));
            var inputs = new List<CapturedCompilationInput>();
            long sourceBytes = 0;
            long referenceBytes = 0;
            foreach (var item in referenceItems)
            {
                cancellationToken.ThrowIfCancellationRequested();
                var path = ResolvePath(item.GetMetadataValue("FullPath"), item.EvaluatedInclude, directory);
                EnsureAllowedInputPath(path, directory, host.ToolsPath);
                var capture = Capture(path, "reference", MaximumReferenceBytes - referenceBytes, "ReferencePath", ReadItemMetadata(item));
                referenceBytes += capture.Bytes.Length;
                inputs.Add(capture.Record);
                var aliases = item.GetMetadataValue("Aliases")
                    .Split(',', StringSplitOptions.RemoveEmptyEntries | StringSplitOptions.TrimEntries);
                var properties = MetadataReferenceProperties.Assembly
                    .WithEmbedInteropTypes(string.Equals(item.GetMetadataValue("EmbedInteropTypes"), "true", StringComparison.OrdinalIgnoreCase));
                if (aliases.Length != 0) properties = properties.WithAliases(aliases);
                project = project.AddMetadataReference(MetadataReference.CreateFromImage(
                    ImmutableArray.CreateRange(capture.Bytes), properties, filePath: path));
            }
            foreach (var item in compileItems)
            {
                cancellationToken.ThrowIfCancellationRequested();
                var path = ResolvePath(item.GetMetadataValue("FullPath"), item.EvaluatedInclude, directory);
                EnsureAllowedInputPath(path, directory);
                var capture = Capture(path, "source", MaximumSourceBytes - sourceBytes, "Compile", ReadItemMetadata(item));
                sourceBytes += capture.Bytes.Length;
                inputs.Add(capture.Record);
                using var stream = new MemoryStream(capture.Bytes, writable: false);
                var text = SourceText.From(stream, encoding: null, checksumAlgorithm: SourceHashAlgorithm.Sha256);
                project = project.AddDocument(Path.GetFileName(path), text, filePath: path).Project;
            }
            CaptureAuxiliaryItems(additionalItems, "AdditionalFiles", "additional");
            CaptureAuxiliaryItems(analyzerConfigItems, "AnalyzerConfigFiles", "analyzer_config");
            CaptureAuxiliaryItems(resourceItems, "EmbeddedResource", "resource");

            void CaptureAuxiliaryItems(IEnumerable<Microsoft.Build.Execution.ProjectItemInstance> items, string itemType, string kind)
            {
                foreach (var item in items)
                {
                    cancellationToken.ThrowIfCancellationRequested();
                    var path = ResolvePath(item.GetMetadataValue("FullPath"), item.EvaluatedInclude, directory);
                    EnsureAllowedInputPath(path, directory);
                    var capture = Capture(path, kind, MaximumSourceBytes - sourceBytes, itemType, ReadItemMetadata(item));
                    sourceBytes += capture.Bytes.Length;
                    inputs.Add(capture.Record);
                }
            }
            // Analyzer execution, generated outputs and host SDK DLL read sets
            // remain unsupported; this is never a positive control receipt.
            if (!workspace.TryApplyChanges(project.Solution))
                throw new InvalidOperationException("Controlled Roslyn snapshot could not be applied");
            project = workspace.CurrentSolution.GetProject(project.Id)
                ?? throw new InvalidOperationException("Controlled Roslyn project snapshot is missing");
            var compilation = await project.GetCompilationAsync(cancellationToken)
                ?? throw new InvalidOperationException("Controlled Roslyn compilation is unavailable");
            if (evaluation.Project.LastEvaluationId != evaluationId)
                throw new InvalidOperationException("Controlled MSBuild evaluation changed during compilation capture");
            return new CapturedRoslynCompilation(workspace, project, compilation, evaluationId,
                evaluation.SnapshotId, referenceResolutionSnapshotId,
                ComputeSnapshotId(evaluation.SnapshotId, referenceResolutionSnapshotId, inputs, parseOptions, options, compilerProperties),
                inputs.AsReadOnly(), compilerProperties, sourceAndReferenceBytesCaptured);
        }
        catch
        {
            workspace.Dispose();
            throw;
        }
    }

    internal void ValidateInputsUnchanged()
    {
        foreach (var input in Inputs)
        {
            // Reuse the capture reader with the ORIGINAL input length as its
            // ceiling: replacement/growth cannot allocate an unbounded file.
            var current = Capture(input.FullPath, input.Kind, input.Length, input.ItemType, input.Metadata).Record;
            if (current.Length != input.Length ||
                !string.Equals(current.Sha256, input.Sha256, StringComparison.Ordinal))
                throw new IOException("Controlled compiler input changed after compilation capture");
        }
    }

    public void Dispose() => _workspace.Dispose();

    internal static string EvaluatedReferenceSnapshotId(CapturedMsBuildEvaluation evaluation, Microsoft.Build.Execution.ProjectInstance instance)
    {
        var references = instance.GetItems("ReferencePath").OrderBy(item => item.EvaluatedInclude, StringComparer.Ordinal).ToArray();
        if (references.Length == 0 || references.Length > MaximumInputs)
            throw new InvalidOperationException("Controlled evaluation has no bounded evaluated ReferencePath set");
        var material = new StringBuilder().Append("evaluated-reference-items-v1\0")
            .Append(evaluation.SnapshotId).Append('\0').Append(evaluation.EvaluationId).Append('\0');
        foreach (var item in references)
            material.Append(item.EvaluatedInclude).Append(':').Append(item.GetMetadataValue("FullPath")).Append(':')
                .Append(item.GetMetadataValue("Aliases")).Append(':').Append(item.GetMetadataValue("EmbedInteropTypes")).Append('\0');
        return Convert.ToHexString(SHA256.HashData(Encoding.UTF8.GetBytes(material.ToString())));
    }

    private static string ComputeSnapshotId(string evaluationSnapshotId, string referenceResolutionSnapshotId, IEnumerable<CapturedCompilationInput> inputs,
        CSharpParseOptions parseOptions, CSharpCompilationOptions compilationOptions, IReadOnlyDictionary<string, string> compilerProperties)
    {
        var material = new StringBuilder().Append("captured-roslyn-compilation-v2\0")
            .Append(evaluationSnapshotId).Append('\0').Append(referenceResolutionSnapshotId).Append('\0')
            .Append(parseOptions.LanguageVersion).Append('\0')
            .Append(string.Join(",", parseOptions.PreprocessorSymbolNames)).Append('\0')
            .Append(compilationOptions.OutputKind).Append(':').Append(compilationOptions.NullableContextOptions).Append(':')
            .Append(compilationOptions.AllowUnsafe).Append(':').Append(compilationOptions.CheckOverflow).Append(':')
            .Append(compilationOptions.Platform).Append(':').Append(compilationOptions.OptimizationLevel).Append(':')
            .Append(compilationOptions.WarningLevel).Append(':').Append(compilationOptions.GeneralDiagnosticOption).Append(':')
            .Append(compilationOptions.MainTypeName).Append(':').Append(compilationOptions.Deterministic).Append(':')
            .Append(compilationOptions.ConcurrentBuild).Append('\0');
        foreach (var property in compilerProperties.OrderBy(item => item.Key, StringComparer.OrdinalIgnoreCase))
            material.Append("property:").Append(property.Key).Append(':').Append(property.Value).Append('\0');
        foreach (var input in inputs.OrderBy(item => item.FullPath, OperatingSystem.IsWindows() ? StringComparer.OrdinalIgnoreCase : StringComparer.Ordinal))
        {
            material.Append(input.ItemType).Append(':').Append(input.Kind).Append(':').Append(input.FullPath).Append(':').Append(input.Sha256).Append(':').Append(input.Length).Append('\0');
            foreach (var metadata in input.Metadata.OrderBy(item => item.Key, StringComparer.OrdinalIgnoreCase))
                material.Append("metadata:").Append(metadata.Key).Append(':').Append(metadata.Value).Append('\0');
        }
        return Convert.ToHexString(SHA256.HashData(Encoding.UTF8.GetBytes(material.ToString())));
    }

    private static string ResolvePath(string fullPath, string include, string directory)
    {
        var path = string.IsNullOrWhiteSpace(fullPath) ? include : fullPath;
        if (string.IsNullOrWhiteSpace(path)) throw new IOException("Controlled compilation input has no path");
        return Path.GetFullPath(path, directory);
    }

    private static (byte[] Bytes, CapturedCompilationInput Record) Capture(
        string path, string kind, long remaining, string itemType, IReadOnlyDictionary<string, string> metadata)
    {
        if (remaining < 0) throw new IOException("Controlled compilation input budget exceeded");
        EnsureUnlinked(path);
        var before = new FileInfo(path);
        before.Refresh();
        if (!before.Exists || before.Length > remaining) throw new IOException("Controlled compilation input is missing or exceeds its bound");
        var length = before.Length;
        var modified = before.LastWriteTimeUtc;
        using var file = new FileStream(path, FileMode.Open, FileAccess.Read, FileShare.Read);
        using var buffer = new MemoryStream();
        var chunk = new byte[64 * 1024];
        while (true)
        {
            var allowance = remaining + 1 - buffer.Length;
            if (allowance <= 0) throw new IOException("Controlled compilation input budget exceeded");
            var count = file.Read(chunk, 0, (int)Math.Min(chunk.Length, allowance));
            if (count == 0) break;
            buffer.Write(chunk, 0, count);
            if (buffer.Length > remaining) throw new IOException("Controlled compilation input budget exceeded");
        }
        EnsureUnlinked(path);
        var after = new FileInfo(path);
        after.Refresh();
        if (!after.Exists || after.Length != length || after.LastWriteTimeUtc != modified || buffer.Length != length)
            throw new IOException("Controlled compilation input changed during capture");
        var bytes = buffer.ToArray();
        return (bytes, new CapturedCompilationInput(path, kind, Convert.ToHexString(SHA256.HashData(bytes)), length, itemType, metadata));
    }

    private static IReadOnlyDictionary<string, string> ReadCompilerProperties(Microsoft.Build.Execution.ProjectInstance instance)
    {
        var names = new[]
        {
            "AssemblyName", "TargetFramework", "LangVersion", "DefineConstants", "Nullable", "OutputType",
            "AllowUnsafeBlocks", "CheckForOverflowUnderflow", "PlatformTarget", "Optimize", "WarningLevel",
            "TreatWarningsAsErrors", "WarningsAsErrors", "WarningsNotAsErrors", "NoWarn", "StartupObject",
            "Deterministic", "ConcurrentBuild", "Features", "GenerateDocumentationFile", "RootNamespace",
            "Prefer32Bit", "PublicSign", "DelaySign"
        };
        return names.ToDictionary(name => name, instance.GetPropertyValue, StringComparer.OrdinalIgnoreCase);
    }

    private static IReadOnlyDictionary<string, string> ReadItemMetadata(Microsoft.Build.Execution.ProjectItemInstance item)
    {
        var names = new[] { "Aliases", "EmbedInteropTypes", "Link", "Visible", "AutoGen", "DependentUpon", "Generator", "LogicalName", "Culture" };
        return names.ToDictionary(name => name, item.GetMetadataValue, StringComparer.OrdinalIgnoreCase);
    }

    private static bool ParseOptionalBool(string value, bool defaultValue)
        => string.IsNullOrWhiteSpace(value) ? defaultValue :
            bool.TryParse(value, out var parsed) ? parsed :
            throw new InvalidOperationException("Controlled compiler boolean option is invalid");

    private static string? NullIfWhiteSpace(string value) => string.IsNullOrWhiteSpace(value) ? null : value.Trim();

    private static void AddDiagnosticOptions(Dictionary<string, ReportDiagnostic> options, string value, ReportDiagnostic disposition)
    {
        foreach (var id in value.Split(new[] { ';', ',' }, StringSplitOptions.RemoveEmptyEntries | StringSplitOptions.TrimEntries))
        {
            if (id.Length is < 3 or > 32 || id.Any(character => !(char.IsAsciiLetterOrDigit(character) || character == '_')))
                throw new InvalidOperationException("Controlled compiler diagnostic option is invalid");
            options[id] = disposition;
        }
    }

    private static void EnsureAllowedInputPath(string path, string projectDirectory, string? hostRoot = null)
    {
        var full = Path.GetFullPath(path);
        var inProject = IsWithin(projectDirectory, full);
        var inHost = hostRoot is not null && IsWithin(hostRoot, full);
        if (!inProject && !inHost)
            throw new IOException("Controlled compiler input is outside the project and pinned SDK roots");
    }

    private static bool IsWithin(string root, string path)
    {
        var relative = Path.GetRelativePath(Path.GetFullPath(root), Path.GetFullPath(path));
        return relative != "." && relative != ".." && !relative.StartsWith(".." + Path.DirectorySeparatorChar, StringComparison.Ordinal) &&
            !Path.IsPathFullyQualified(relative);
    }

    private static void EnsureUnlinked(string fullPath)
    {
        var current = Path.GetPathRoot(fullPath) ?? throw new IOException("Controlled input path has no root");
        foreach (var segment in fullPath[current.Length..].Split(
            new[] { Path.DirectorySeparatorChar, Path.AltDirectorySeparatorChar }, StringSplitOptions.RemoveEmptyEntries))
        {
            current = Path.Combine(current, segment);
            if ((File.GetAttributes(current) & FileAttributes.ReparsePoint) != 0)
                throw new IOException("Linked controlled compilation inputs are unsupported");
        }
    }
}

internal sealed record CapturedCompilationInput(string FullPath, string Kind, string Sha256, long Length,
    string ItemType, IReadOnlyDictionary<string, string> Metadata);
