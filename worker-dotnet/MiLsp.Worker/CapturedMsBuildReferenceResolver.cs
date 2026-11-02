using System.Security.Cryptography;
using System.Text;
using System.Xml.Linq;
using Microsoft.Build.Evaluation;
using Microsoft.Build.Execution;
using Microsoft.Build.Framework;

namespace MiLsp.Worker;

/// <summary>
/// Runs only the native ResolveReferences target against the already evaluated
/// ProjectInstance. The dependency list is narrowed to reference-producing SDK
/// targets; unknown targets/tasks and project-to-project references fail closed.
/// This is not a complete build-input receipt.
/// </summary>
internal sealed class CapturedMsBuildReferenceResolution : IDisposable
{
    private readonly DirectoryInfo _scratch;
    private bool _disposed;

    internal CapturedMsBuildReferenceResolution(ProjectInstance project, DirectoryInfo scratch,
        int evaluationId, string snapshotId, CapturedReferenceInput assetsInput,
        IReadOnlyList<string> targets, IReadOnlyList<string> tasks)
    {
        Project = project;
        _scratch = scratch;
        EvaluationId = evaluationId;
        SnapshotId = snapshotId;
        AssetsInput = assetsInput;
        Targets = targets;
        Tasks = tasks;
    }

    internal ProjectInstance Project { get; }
    internal int EvaluationId { get; }
    internal string SnapshotId { get; }
    internal CapturedReferenceInput AssetsInput { get; }
    internal IReadOnlyList<string> Targets { get; }
    internal IReadOnlyList<string> Tasks { get; }

    public void Dispose()
    {
        if (_disposed) return;
        _disposed = true;
        _scratch.Delete(recursive: true);
    }
}

internal sealed record CapturedReferenceInput(string Path, string Sha256, long Length);

internal static class CapturedMsBuildReferenceResolver
{
    private const int MaximumTargets = 4096;
    private const int MaximumReferences = 4096;
    private const long MaximumAssetsBytes = 128L * 1024 * 1024;
    private const string ControlledResolveReferencesDependsOn = "ResolveAssemblyReferences";
    private const string ControlledResolveAssemblyReferencesDependsOn =
        "GetFrameworkPaths;GetReferenceAssemblyPaths;ResolveSDKReferences;ExpandSDKReferences;ResolveTargetingPackAssets;ResolveLockFileReferences;_HandlePackageFileConflicts";

    private static readonly HashSet<string> AllowedTasks = new(StringComparer.OrdinalIgnoreCase)
    {
        "ResolveAssemblyReference", "GetReferenceAssemblyPaths", "ResolveSDKReference", "GetSDKReferenceFiles",
        "ResolvePackageAssets", "CheckForTargetInAssetsFile", "GetAssemblyIdentity",
        "CreateWindowsSdkKnownFrameworkReferences", "CheckForDuplicateFrameworkReferences",
        "ProcessFrameworkReferences", "ResolveAppHosts", "GetPackageDirectory",
        "ResolveFrameworkReferences", "ResolveTargetingPackAssets", "ResolvePackageFileConflicts",
        "NETSdkError", "Message", "Warning", "Error"
    };

    internal static CapturedMsBuildReferenceResolution Resolve(
        CapturedMsBuildEvaluation evaluation, ProjectCollection collection, CancellationToken cancellationToken)
    {
        cancellationToken.ThrowIfCancellationRequested();
        var instance = evaluation.Instance;
        var evaluationId = evaluation.EvaluationId;
        if (!evaluation.FileSystem.IsSealed || evaluation.Project.LastEvaluationId != evaluationId ||
            instance.EvaluationId != evaluationId || !PathEquals(instance.FullPath, evaluation.Project.FullPath))
            throw new InvalidOperationException("Reference resolution requires the sealed original MSBuild ProjectInstance");

        EnsureSupportedProject(instance);
        _ = CapturedMsBuildToolchain.RequireHost(instance.GetPropertyValue("MSBuildToolsPath"),
            instance.GetPropertyValue("MSBuildSDKsPath"));
        var assetsPath = instance.GetPropertyValue("ProjectAssetsFile");
        if (string.IsNullOrWhiteSpace(assetsPath))
            throw new InvalidOperationException("Controlled native reference resolution requires the evaluated ProjectAssetsFile");
        var assetsInput = CaptureAssetsFile(assetsPath);
        var scratch = Directory.CreateTempSubdirectory("mi-lsp-msbuild-reference-");
        try
        {
            cancellationToken.ThrowIfCancellationRequested();
            var scratchRoot = Path.GetFullPath(scratch.FullName) + Path.DirectorySeparatorChar;
            var obj = Path.Combine(scratchRoot, "obj") + Path.DirectorySeparatorChar;
            var bin = Path.Combine(scratchRoot, "bin") + Path.DirectorySeparatorChar;
            var extensions = Path.Combine(scratchRoot, "extensions") + Path.DirectorySeparatorChar;
            Directory.CreateDirectory(obj);
            Directory.CreateDirectory(bin);
            Directory.CreateDirectory(extensions);

            // These scoped target-time overrides route SDK caches and any standard
            // intermediate/output paths into this operation-owned scratch. The
            // restored ProjectAssetsFile itself remains the original read-only input.
            instance.SetProperty("ResolveReferencesDependsOn", ControlledResolveReferencesDependsOn);
            instance.SetProperty("ResolveAssemblyReferencesDependsOn", ControlledResolveAssemblyReferencesDependsOn);
            instance.SetProperty("BuildProjectReferences", "false");
            instance.SetProperty("ProjectAssetsFile", assetsInput.Path);
            instance.SetProperty("BaseIntermediateOutputPath", obj);
            instance.SetProperty("IntermediateOutputPath", Path.Combine(obj, SafeSegment(instance.GetPropertyValue("TargetFramework"))) + Path.DirectorySeparatorChar);
            instance.SetProperty("MSBuildProjectExtensionsPath", extensions);
            instance.SetProperty("ProjectAssetsCacheFile", Path.Combine(obj, "project.assets.cache"));
            instance.SetProperty("ResolveAssemblyReferencesStateFile", Path.Combine(obj, "ResolveAssemblyReferences.cache"));
            instance.SetProperty("DesignTimeResolveAssemblyReferencesStateFile", Path.Combine(obj, "DesignTimeResolveAssemblyReferences.cache"));
            instance.SetProperty("AssemblyInformationCacheOutputPath", Path.Combine(obj, "AssemblyInformationCache.cache"));
            instance.SetProperty("GetSDKReferenceFilesCacheFolder", Path.Combine(obj, "sdk-reference-cache"));
            instance.SetProperty("DisablePackageAssetsCache", "true");
            instance.SetProperty("DisableRarCache", "true");
            instance.SetProperty("OutputPath", bin);
            instance.SetProperty("OutDir", bin);

            var targetNames = AuditTargetClosure(evaluation, instance, new[] { "ResolveReferences" });
            var taskNames = AuditTaskBindings(evaluation, instance, targetNames);
            if (evaluation.Project.LastEvaluationId != evaluationId || instance.EvaluationId != evaluationId)
                throw new InvalidOperationException("Reference-resolution planning changed the controlled evaluation identity");

            cancellationToken.ThrowIfCancellationRequested();
            using var manager = new BuildManager("mi-lsp-captured-references-" + Guid.NewGuid().ToString("N"));
            var parameters = new BuildParameters(collection)
            {
                MaxNodeCount = 1,
                EnableNodeReuse = false,
                ShutdownInProcNodeOnBuildFinish = true,
                Loggers = Array.Empty<ILogger>()
            };
            using var cancel = cancellationToken.Register(manager.CancelAllSubmissions);
            var request = new BuildRequestData(instance, new[] { "ResolveReferences" }, null,
                BuildRequestDataFlags.ProvideProjectStateAfterBuild);
            var result = manager.Build(parameters, request);
            cancellationToken.ThrowIfCancellationRequested();
            var assetsAfter = CaptureAssetsFile(assetsInput.Path);
            if (assetsAfter.Length != assetsInput.Length || !string.Equals(assetsAfter.Sha256, assetsInput.Sha256, StringComparison.Ordinal))
                throw new IOException("Native reference resolution changed the read-only ProjectAssetsFile");
            if (result.OverallResult != BuildResultCode.Success || result.ProjectStateAfterBuild is null ||
                !result.ResultsByTarget.TryGetValue("ResolveReferences", out var targetResult) ||
                targetResult.ResultCode != TargetResultCode.Success)
                throw new InvalidOperationException("Native ResolveReferences target did not produce a successful controlled ProjectStateAfterBuild");

            var resolved = result.ProjectStateAfterBuild;
            if (resolved.EvaluationId != evaluationId || !PathEquals(resolved.FullPath, instance.FullPath) ||
                evaluation.Project.LastEvaluationId != evaluationId)
                throw new InvalidOperationException("Native ResolveReferences result is not bound to the original project evaluation");
            var references = resolved.GetItems("ReferencePath").ToArray();
            if (references.Length == 0 || references.Length > MaximumReferences)
                throw new InvalidOperationException("Native ResolveReferences returned an empty or over-bound ReferencePath set");
            foreach (var reference in references)
            {
                var path = reference.GetMetadataValue("FullPath");
                if (string.IsNullOrWhiteSpace(path)) path = reference.EvaluatedInclude;
                if (string.IsNullOrWhiteSpace(path) || !Path.IsPathFullyQualified(path))
                    throw new InvalidOperationException("Native ResolveReferences returned a reference without a full path");
            }

            var snapshotId = ComputeSnapshotId(evaluation, assetsInput, resolved, targetNames, taskNames);
            return new CapturedMsBuildReferenceResolution(resolved, scratch, evaluationId, snapshotId,
                assetsInput, targetNames.AsReadOnly(), taskNames.AsReadOnly());
        }
        catch
        {
            scratch.Delete(recursive: true);
            throw;
        }
    }

    private static void EnsureSupportedProject(ProjectInstance instance)
    {
        // Initial targets run even when only ResolveReferences was requested.
        // They are not part of the restricted target audit below.
        if (instance.InitialTargets.Any())
            throw new InvalidOperationException("Initial MSBuild targets are outside the controlled reference resolver subset");
        var framework = instance.GetPropertyValue("TargetFramework");
        if (string.IsNullOrWhiteSpace(framework) || framework.Contains(';'))
            throw new InvalidOperationException("Controlled native reference resolution requires one evaluated TargetFramework");
        foreach (var itemType in new[] { "ProjectReference", "COMReference", "NativeReference", "ReferencePath" })
        {
            if (instance.GetItems(itemType).Any())
                throw new InvalidOperationException("Controlled native reference resolution does not certify project, COM, native, or preexisting ReferencePath items");
        }
        if (instance.GetItems("SDKReference").Any())
            throw new InvalidOperationException("Legacy SDKReference items are outside the controlled resolver subset");
    }

    private static List<string> AuditTargetClosure(CapturedMsBuildEvaluation evaluation,
        ProjectInstance instance, IEnumerable<string> roots)
    {
        var targets = instance.Targets;
        var queue = new Queue<string>(roots);
        var selected = new HashSet<string>(StringComparer.OrdinalIgnoreCase);
        while (queue.Count > 0)
        {
            var name = queue.Dequeue();
            if (!selected.Add(name)) continue;
            if (selected.Count > MaximumTargets || !targets.TryGetValue(name, out var target))
                throw new InvalidOperationException("Native reference target closure is missing or exceeds its bound");
            // OnError targets can run outside DependsOn/Before/After closure.
            if (target.OnErrorChildren.Any())
                throw new InvalidOperationException("OnError target execution is outside the controlled reference resolver subset");
            var sourceFile = target.Location.File;
            if (string.IsNullOrWhiteSpace(sourceFile) || !IsTrustedTargetPath(instance, sourceFile))
                throw new InvalidOperationException("Native reference target closure includes a project/package-defined target outside the audited SDK roots");
            foreach (var dependency in SplitTargetList(instance.ExpandString(target.DependsOnTargets)))
                queue.Enqueue(dependency);

            // Include target hooks in both directions. Unknown hooks are not
            // silently skipped just because the requested target is narrow.
            foreach (var candidate in targets.Values)
            {
                var before = SplitTargetList(instance.ExpandString(candidate.BeforeTargets));
                var after = SplitTargetList(instance.ExpandString(candidate.AfterTargets));
                if (before.Contains(name, StringComparer.OrdinalIgnoreCase) || after.Contains(name, StringComparer.OrdinalIgnoreCase))
                    queue.Enqueue(candidate.Name);
            }
        }
        return selected.OrderBy(value => value, StringComparer.OrdinalIgnoreCase).ToList();
    }

    private static List<string> AuditTaskBindings(CapturedMsBuildEvaluation evaluation,
        ProjectInstance instance, IReadOnlyCollection<string> targets)
    {
        var taskNames = new HashSet<string>(StringComparer.OrdinalIgnoreCase);
        foreach (var targetName in targets)
        {
            var target = instance.Targets[targetName];
            foreach (var child in target.Tasks)
            {
                if (child is not ProjectTaskInstance task || !AllowedTasks.Contains(task.Name))
                    throw new InvalidOperationException("Native reference target closure contains an unaudited MSBuild task");
                taskNames.Add(task.Name);
            }
        }
        VerifyTaskAssemblies(evaluation, instance, taskNames);
        return taskNames.OrderBy(value => value, StringComparer.OrdinalIgnoreCase).ToList();
    }

    private static void VerifyTaskAssemblies(CapturedMsBuildEvaluation evaluation,
        ProjectInstance instance, IEnumerable<string> taskNames)
    {
        var expected = new HashSet<string>(taskNames.Select(ShortName), StringComparer.OrdinalIgnoreCase);
        foreach (var intrinsic in new[] { "Message", "Warning", "Error" }) expected.Remove(intrinsic);
        var bindings = new Dictionary<string, HashSet<string>>(StringComparer.OrdinalIgnoreCase);
        foreach (var xml in evaluation.XmlInputs)
        {
            using var stream = new MemoryStream(xml.Bytes, writable: false);
            var document = XDocument.Load(stream, LoadOptions.None);
            foreach (var element in document.Descendants().Where(value => value.Name.LocalName == "UsingTask"))
            {
                var taskName = (string?)element.Attribute("TaskName");
                if (string.IsNullOrWhiteSpace(taskName) || !expected.Contains(ShortName(taskName))) continue;
                if (element.Attribute("TaskFactory") is not null || element.Elements().Any(value => value.Name.LocalName == "Task"))
                    throw new InvalidOperationException("Inline/custom task factories are outside the audited SDK resolver subset");
                var assemblyFile = (string?)element.Attribute("AssemblyFile");
                if (string.IsNullOrWhiteSpace(assemblyFile))
                    throw new InvalidOperationException("Native reference task binding has no auditable SDK AssemblyFile");
                var expanded = instance.ExpandString(assemblyFile);
                if (expanded.Contains('$') || expanded.Contains('@'))
                    throw new InvalidOperationException("Native reference task AssemblyFile did not resolve to a concrete SDK path");
                var path = Path.GetFullPath(expanded);
                if (!IsTrustedTaskPath(instance, path) || !File.Exists(path))
                    throw new InvalidOperationException("Native reference task assembly is outside the active MSBuild/.NET SDK roots");
                EnsureNoReparsePoint(path);
                var key = ShortName(taskName);
                if (!bindings.TryGetValue(key, out var paths)) bindings[key] = paths = new HashSet<string>(PathComparer);
                paths.Add(path);
            }
        }
        foreach (var name in expected)
            if (!bindings.TryGetValue(name, out var paths) || paths.Count != 1)
                throw new InvalidOperationException("Native reference task binding is missing or ambiguous in captured SDK XML");
    }

    private static CapturedReferenceInput CaptureAssetsFile(string path)
    {
        var fullPath = Path.GetFullPath(path);
        EnsureNoReparsePoint(fullPath);
        var before = new FileInfo(fullPath);
        before.Refresh();
        if (!before.Exists || before.Length > MaximumAssetsBytes)
            throw new IOException("Evaluated ProjectAssetsFile is missing or exceeds the reference resolver bound");
        var length = before.Length;
        var modified = before.LastWriteTimeUtc;
        byte[] bytes;
        using (var stream = new FileStream(fullPath, FileMode.Open, FileAccess.Read, FileShare.Read, 64 * 1024, FileOptions.SequentialScan))
        using (var output = new MemoryStream((int)Math.Min(stream.Length, MaximumAssetsBytes)))
        {
            var buffer = new byte[64 * 1024];
            while (true)
            {
                var allowance = MaximumAssetsBytes + 1 - output.Length;
                if (allowance <= 0) throw new IOException("Evaluated ProjectAssetsFile exceeds its byte bound");
                var count = stream.Read(buffer, 0, (int)Math.Min(buffer.Length, allowance));
                if (count == 0) break;
                output.Write(buffer, 0, count);
            }
            bytes = output.ToArray();
        }
        EnsureNoReparsePoint(fullPath);
        var after = new FileInfo(fullPath);
        after.Refresh();
        if (!after.Exists || after.Length != length || after.LastWriteTimeUtc != modified || bytes.LongLength != length)
            throw new IOException("Evaluated ProjectAssetsFile changed during reference resolution capture");
        return new CapturedReferenceInput(fullPath, Convert.ToHexString(SHA256.HashData(bytes)), length);
    }

    private static string ComputeSnapshotId(CapturedMsBuildEvaluation evaluation,
        CapturedReferenceInput assets, ProjectInstance resolved, IEnumerable<string> targets, IEnumerable<string> tasks)
    {
        var material = new StringBuilder().Append("captured-msbuild-reference-resolution-v1\0")
            .Append(evaluation.SnapshotId).Append('\0').Append(evaluation.EvaluationId).Append('\0')
            .Append(assets.Path).Append(':').Append(assets.Sha256).Append(':').Append(assets.Length).Append('\0')
            .Append("target:ResolveReferences\0")
            .Append("override:ResolveReferencesDependsOn=ResolveAssemblyReferences\0")
            .Append("override:ResolveAssemblyReferencesDependsOn=").Append(ControlledResolveAssemblyReferencesDependsOn).Append('\0')
            .Append("output-policy:scratch;no-restore;no-project-references;no-com-or-native\0");
        foreach (var target in targets.OrderBy(value => value, StringComparer.OrdinalIgnoreCase)) material.Append("target:").Append(target).Append('\0');
        foreach (var task in tasks.OrderBy(value => value, StringComparer.OrdinalIgnoreCase)) material.Append("task:").Append(task).Append('\0');
        foreach (var item in resolved.GetItems("ReferencePath").OrderBy(value => value.EvaluatedInclude, PathComparer))
        {
            material.Append("reference:").Append(item.GetMetadataValue("FullPath")).Append(':')
                .Append(item.GetMetadataValue("Aliases")).Append(':')
                .Append(item.GetMetadataValue("EmbedInteropTypes")).Append(':')
                .Append(item.GetMetadataValue("ReferenceSourceTarget")).Append('\0');
        }
        return Convert.ToHexString(SHA256.HashData(Encoding.UTF8.GetBytes(material.ToString())));
    }

    private static IReadOnlyList<string> SplitTargetList(string value)
        => value.Split(new[] { ';', ',' }, StringSplitOptions.RemoveEmptyEntries | StringSplitOptions.TrimEntries);

    private static string ShortName(string name)
    {
        var index = name.LastIndexOf('.');
        return index < 0 ? name : name[(index + 1)..];
    }

    private static bool IsTrustedTargetPath(ProjectInstance instance, string path)
    {
        var host = CapturedMsBuildToolchain.RequireHost(instance.GetPropertyValue("MSBuildToolsPath"),
            instance.GetPropertyValue("MSBuildSDKsPath"));
        return host.Contains(path);
    }

    private static bool IsTrustedTaskPath(ProjectInstance instance, string path)
        => IsTrustedTargetPath(instance, path);

    private static bool IsWithin(string root, string path)
    {
        if (string.IsNullOrWhiteSpace(root)) return false;
        var fullRoot = Path.GetFullPath(root);
        var relative = Path.GetRelativePath(fullRoot, path);
        return relative != ".." && !relative.StartsWith(".." + Path.DirectorySeparatorChar, StringComparison.Ordinal) && !Path.IsPathFullyQualified(relative);
    }

    private static void EnsureNoReparsePoint(string path)
    {
        var root = Path.GetPathRoot(path) ?? throw new IOException("Reference input path has no root");
        var current = root;
        foreach (var part in path[root.Length..].Split(new[] { Path.DirectorySeparatorChar, Path.AltDirectorySeparatorChar }, StringSplitOptions.RemoveEmptyEntries))
        {
            current = Path.Combine(current, part);
            if ((File.GetAttributes(current) & FileAttributes.ReparsePoint) != 0)
                throw new IOException("Linked ProjectAssetsFile paths are outside the controlled resolver subset");
        }
    }

    private static string SafeSegment(string value)
    {
        if (string.IsNullOrWhiteSpace(value) || value.IndexOfAny(Path.GetInvalidFileNameChars()) >= 0 || value.Contains(Path.DirectorySeparatorChar) || value.Contains(Path.AltDirectorySeparatorChar))
            return "tfm";
        return value;
    }

    private static bool PathEquals(string left, string right)
        => PathComparer.Equals(Path.GetFullPath(left), Path.GetFullPath(right));

    private static StringComparer PathComparer => OperatingSystem.IsWindows() ? StringComparer.OrdinalIgnoreCase : StringComparer.Ordinal;
}
