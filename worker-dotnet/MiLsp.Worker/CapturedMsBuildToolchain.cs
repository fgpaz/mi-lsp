namespace MiLsp.Worker;

// Host-selected locator identity, never derived from a ProjectInstance property.
// This anchors the execution allowlist; it is NOT a complete SDK read-set receipt.
internal sealed class CapturedMsBuildToolchain
{
    private static CapturedMsBuildToolchain? _registered;
    internal string ToolsPath { get; }
    internal string SdksPath { get; }
    internal static bool HasRegisteredHost => Volatile.Read(ref _registered) is not null;

    private CapturedMsBuildToolchain(string toolsPath)
    {
        ToolsPath = Path.TrimEndingDirectorySeparator(Path.GetFullPath(toolsPath));
        SdksPath = Path.Combine(ToolsPath, "Sdks");
        // Do not impose controlled-resolver restrictions on legacy workspace
        // startup. Existence/link checks belong to RequireHost, before execution.
    }

    // Legacy callers publish only after locator registration; the isolated host publishes after pinning its custom loader.
    // This type deliberately has no Microsoft.Build type references, so preparing
    // the host identity does not load MSBuild before the locator registers it.
    internal static CapturedMsBuildToolchain PrepareHost(string locatorPath) => new(locatorPath);

    internal static Dictionary<string, string> EvaluationGlobalProperties()
    {
        var host = Volatile.Read(ref _registered)
            ?? throw new InvalidOperationException("No host-selected MSBuild toolchain identity is pinned");
        return new Dictionary<string, string>(StringComparer.OrdinalIgnoreCase)
        {
            ["MSBuildToolsPath"] = host.ToolsPath,
            ["MSBuildSDKsPath"] = host.SdksPath
        };
    }

    internal static void PublishRegisteredHost(CapturedMsBuildToolchain toolchain)
    {
        if (Interlocked.CompareExchange(ref _registered, toolchain, null) is not null)
            throw new InvalidOperationException("The MSBuild host toolchain was already pinned");
    }

    internal static CapturedMsBuildToolchain RequireHost(string projectToolsPath, string projectSdksPath)
    {
        var host = Volatile.Read(ref _registered)
            ?? throw new InvalidOperationException("No host-selected MSBuild toolchain identity is pinned");
        if (!SamePath(projectToolsPath, host.ToolsPath) || !SamePath(projectSdksPath, host.SdksPath))
            throw new InvalidOperationException("Project MSBuild toolchain roots diverge from the host-selected SDK");
        if (!Directory.Exists(host.ToolsPath) || !Directory.Exists(host.SdksPath))
            throw new InvalidOperationException("Host SDK layout is outside the controlled resolver subset");
        EnsureUnlinked(host.ToolsPath);
        EnsureUnlinked(host.SdksPath);
        return host;
    }

    internal bool Contains(string path)
    {
        var full = Path.GetFullPath(path);
        EnsureUnlinked(full);
        var relative = Path.GetRelativePath(ToolsPath, full);
        return relative != "." && relative != ".." &&
            !relative.StartsWith(".." + Path.DirectorySeparatorChar, StringComparison.Ordinal) &&
            !Path.IsPathFullyQualified(relative);
    }

    private static bool SamePath(string candidate, string expected)
    {
        if (string.IsNullOrWhiteSpace(candidate)) return false;
        var comparer = OperatingSystem.IsWindows() ? StringComparer.OrdinalIgnoreCase : StringComparer.Ordinal;
        return comparer.Equals(Path.TrimEndingDirectorySeparator(Path.GetFullPath(candidate)), expected);
    }

    private static void EnsureUnlinked(string path)
    {
        var root = Path.GetPathRoot(path) ?? throw new IOException("Host toolchain path has no root");
        var current = root;
        foreach (var part in path[root.Length..].Split(new[] { Path.DirectorySeparatorChar, Path.AltDirectorySeparatorChar }, StringSplitOptions.RemoveEmptyEntries))
        {
            current = Path.Combine(current, part);
            if ((File.GetAttributes(current) & FileAttributes.ReparsePoint) != 0)
                throw new IOException("Linked paths are outside the host toolchain execution allowlist");
        }
    }
}
