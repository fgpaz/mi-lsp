using System.Reflection;
using System.Runtime.InteropServices;
using System.Runtime.Loader;
using System.Security.Cryptography;

namespace MiLsp.Worker;

/// <summary>Loads non-framework managed dependencies only from bounded captured byte images.</summary>
internal sealed class CapturedManagedImageLoadContext : AssemblyLoadContext, IDisposable
{
    private const long MaximumImageBytes = 64L * 1024 * 1024;
    private const long MaximumTotalImageBytes = 512L * 1024 * 1024;
    private const int MaximumImages = 256;
    private const int MaximumBaselineAssemblies = 512;
    private const int MaximumRuntimeAssemblyNames = 1024;
    private const int MaximumTrackedEvents = 512;
    private const int MaximumTrackedNames = 512;
    private const int MaximumReportEntries = 64;
    private const int MaximumIdentityCharacters = 512;
    private const int MaximumRuntimeManifestCharacters = 128 * 1024;
    private readonly object _gate = new();
    private readonly string _componentPath;
    private readonly string _applicationRoot;
    private readonly string _runtimeRoot;
    private readonly AssemblyDependencyResolver _resolver;
    private readonly HashSet<string> _runtimeAssemblyNames;
    private readonly Assembly? _bootstrapAssembly;
    private readonly Dictionary<string, int> _ownedLoadEvents = new(StringComparer.Ordinal);
    private readonly Dictionary<string, int> _capturedImageIdentities = new(StringComparer.Ordinal);
    private readonly Dictionary<string, int> _foreignLoadEvents = new(StringComparer.Ordinal);
    private readonly List<string> _unknownBaseline = [];
    private readonly List<string> _untrackedLoads = [];
    private readonly List<ManagedImageRecord> _images = [];
    private readonly AssemblyLoadEventHandler _assemblyLoadHandler;
    private string? _hostToolsPath;
    private long _reservedBytes;
    private int _reservedImages;
    private int _baselineAssembliesChecked;
    private int _trackedLoadEvents;
    private int _foreignLoadEventCount;
    private bool _baselineTrusted = true;
    private bool _bookkeepingOverflow;
    private bool _sealed;
    private bool _captureFailed;

    internal CapturedManagedImageLoadContext(string componentPath, Assembly? bootstrapAssembly)
        : base("mi-lsp-controlled-managed-images", isCollectible: true)
    {
        _componentPath = Path.GetFullPath(componentPath);
        _applicationRoot = Path.GetDirectoryName(_componentPath)
            ?? throw new IOException("Worker component has no directory");
        _runtimeRoot = Path.TrimEndingDirectorySeparator(Path.GetFullPath(RuntimeEnvironment.GetRuntimeDirectory()));
        _resolver = new AssemblyDependencyResolver(_componentPath);
        _bootstrapAssembly = bootstrapAssembly;
        _runtimeAssemblyNames = ReadRuntimeAssemblyNames(_runtimeRoot, out var runtimeNamesOverflow);
        _bookkeepingOverflow = runtimeNamesOverflow;
        if (runtimeNamesOverflow) _baselineTrusted = false;
        _assemblyLoadHandler = OnAssemblyLoad;
        AppDomain.CurrentDomain.AssemblyLoad += _assemblyLoadHandler;
        // Subscribe before this fresh snapshot: pre-subscription loads appear in
        // the snapshot; subsequent loads are observed by the event handler.
        AuditBaselineAssemblies(AppDomain.CurrentDomain.GetAssemblies());
    }

    internal bool BaselineTrusted
    {
        get
        {
            lock (_gate) return _baselineTrusted && !_bookkeepingOverflow && _foreignLoadEventCount == 0;
        }
    }

    internal Assembly LoadWorkerImage()
    {
        var captured = CaptureImage(_componentPath, "worker_component");
        using var stream = new MemoryStream(captured.Bytes, writable: false);
        var assembly = LoadFromStream(stream);
        RegisterLoadedImage(assembly, captured);
        return assembly;
    }

    internal void PinHostToolsPath(string path)
    {
        var full = Path.TrimEndingDirectorySeparator(Path.GetFullPath(path));
        EnsureUnlinkedPath(full);
        var sdksPath = Path.Combine(full, "Sdks");
        if (!Directory.Exists(full) || !Directory.Exists(sdksPath))
            throw new IOException("Locator-selected host toolchain layout is unavailable");
        EnsureUnlinkedPath(sdksPath);
        lock (_gate)
        {
            EnsureOpen();
            if (_hostToolsPath is not null && !SamePath(_hostToolsPath, full))
                throw new InvalidOperationException("Controlled loader host identity is already pinned");
            _hostToolsPath = full;
        }
    }

    protected override Assembly? Load(AssemblyName assemblyName)
    {
        try
        {
            lock (_gate) EnsureOpen();
            var name = assemblyName.Name ?? throw new FileLoadException("Managed dependency has no simple name");
            if (_runtimeAssemblyNames.Contains(name)) return null;

            var hostToolsPath = Volatile.Read(ref _hostToolsPath);
            if (IsHostEngineAssembly(name) && hostToolsPath is null)
                throw new FileLoadException("MSBuild image requested before the locator-selected host was pinned");

            var path = ResolveManagedImage(assemblyName, hostToolsPath);
            var source = IsUnderHostRoot(path, hostToolsPath) ? "msbuild_host" : "dependency_resolver";
            var captured = CaptureImage(path, source);
            using var stream = new MemoryStream(captured.Bytes, writable: false);
            var assembly = LoadFromStream(stream);
            RegisterLoadedImage(assembly, captured, assemblyName);
            return assembly;
        }
        catch
        {
            lock (_gate) _captureFailed = true;
            throw;
        }
    }

    protected override IntPtr LoadUnmanagedDll(string unmanagedDllName)
    {
        lock (_gate) _captureFailed = true;
        throw new DllNotFoundException("Controlled image loader rejects unmanaged dependencies");
    }

    internal ManagedImageCaptureReceipt SealAndGetReceipt()
    {
        lock (_gate)
        {
            _sealed = true;
            CompareLoadEventsToCapturedImages();
            var images = _images.OrderBy(image => image.AssemblyIdentity, StringComparer.Ordinal)
                .ThenBy(image => image.Sha256, StringComparer.Ordinal).ToArray();
            var unmediated = FormatCounts(_foreignLoadEvents, "foreign-load");
            var registryMaterial = string.Join("\n", images.Select(image => string.Join("|", image.AssemblyIdentity, image.Sha256, image.Length, image.Source, image.PathAlias)));
            var registryId = Convert.ToHexString(SHA256.HashData(System.Text.Encoding.UTF8.GetBytes("managed-image-registry-v1\0" + registryMaterial))).ToLowerInvariant();
            var coreHostImagesCaptured = images.Any(image => image.AssemblyIdentity.StartsWith("Microsoft.Build,", StringComparison.OrdinalIgnoreCase)) &&
                images.Any(image => image.AssemblyIdentity.StartsWith("Microsoft.Build.Framework,", StringComparison.OrdinalIgnoreCase));
            var complete = !_captureFailed && !_bookkeepingOverflow && _baselineTrusted &&
                _hostToolsPath is not null && coreHostImagesCaptured && _foreignLoadEventCount == 0 && _untrackedLoads.Count == 0;
            AppDomain.CurrentDomain.AssemblyLoad -= _assemblyLoadHandler;
            return new ManagedImageCaptureReceipt(
                complete ? "managed_images_captured" : "explicit_incomplete",
                complete,
                registryId,
                coreHostImagesCaptured,
                _baselineTrusted,
                _baselineAssembliesChecked,
                _trackedLoadEvents,
                _foreignLoadEventCount,
                _bookkeepingOverflow,
                images,
                unmediated,
                _untrackedLoads.ToArray(),
                _unknownBaseline.ToArray(),
                "Trust only TPA assemblies loaded in Default from RuntimeEnvironment.GetRuntimeDirectory(), plus this exact entry bootstrap assembly. The bootstrap image is not MSBuild/SDK byte evidence; all other baseline and foreign-context loads make this receipt incomplete.");
        }
    }

    public void Dispose()
    {
        AppDomain.CurrentDomain.AssemblyLoad -= _assemblyLoadHandler;
        if (!_sealed) Unload();
    }

    private void AuditBaselineAssemblies(IEnumerable<Assembly> assemblies)
    {
        lock (_gate)
        {
            try
            {
                foreach (var assembly in assemblies)
                {
                    if (_baselineAssembliesChecked >= MaximumBaselineAssemblies)
                    {
                        _baselineTrusted = false;
                        _bookkeepingOverflow = true;
                        return;
                    }
                    _baselineAssembliesChecked++;
                    if (ReferenceEquals(assembly, _bootstrapAssembly) || IsTrustedDefaultRuntime(assembly)) continue;
                    _baselineTrusted = false;
                    AddBoundedSample(_unknownBaseline, "baseline:" + BoundedIdentity(assembly.FullName ?? assembly.GetName().Name));
                }
            }
            catch
            {
                _baselineTrusted = false;
                _bookkeepingOverflow = true;
            }
        }
    }

    private string ResolveManagedImage(AssemblyName requested, string? hostToolsPath)
    {
        var simpleName = requested.Name ?? throw new FileLoadException("Managed dependency has no simple name");
        if (IsHostEngineAssembly(simpleName))
        {
            var hostPath = FindHostImage(hostToolsPath!, simpleName);
            if (hostPath is not null) return hostPath;
            throw new FileNotFoundException("Requested MSBuild image is absent from the pinned host root");
        }

        var applicationPath = _resolver.ResolveAssemblyToPath(requested);
        if (!string.IsNullOrWhiteSpace(applicationPath)) return Path.GetFullPath(applicationPath);
        if (hostToolsPath is not null)
        {
            var hostPath = FindHostImage(hostToolsPath, simpleName);
            if (hostPath is not null) return hostPath;
        }
        throw new FileNotFoundException("Managed dependency has no captured application or pinned-host image");
    }

    private static string? FindHostImage(string hostToolsPath, string simpleName)
    {
        foreach (var directory in new[]
        {
            hostToolsPath,
            Path.Combine(hostToolsPath, "Roslyn", "bincore"),
            Path.Combine(hostToolsPath, "Sdks")
        })
        {
            var candidate = Path.Combine(directory, simpleName + ".dll");
            if (File.Exists(candidate)) return Path.GetFullPath(candidate);
        }
        return null;
    }

    private CapturedImage CaptureImage(string path, string source)
    {
        var full = Path.GetFullPath(path);
        EnsureUnlinkedPath(full);
        var before = new FileInfo(full);
        before.Refresh();
        if (!before.Exists || before.Length is <= 0 or > MaximumImageBytes)
            throw new IOException("Managed image is missing or exceeds its per-image byte bound");
        var expectedLength = before.Length;
        var modified = before.LastWriteTimeUtc;
        lock (_gate)
        {
            EnsureOpen();
            if (_images.Count + _reservedImages >= MaximumImages || _reservedBytes + expectedLength > MaximumTotalImageBytes)
                throw new IOException("Managed image capture exceeded its count or total byte bound");
            _reservedBytes += expectedLength;
            _reservedImages++;
        }

        try
        {
            byte[] bytes;
            using (var stream = new FileStream(full, FileMode.Open, FileAccess.Read, FileShare.Read, 64 * 1024, FileOptions.SequentialScan))
            using (var memory = new MemoryStream((int)expectedLength))
            {
                var buffer = new byte[64 * 1024];
                while (true)
                {
                    var allowance = expectedLength + 1 - memory.Length;
                    if (allowance <= 0) throw new IOException("Managed image grew beyond its captured byte bound");
                    var count = stream.Read(buffer, 0, (int)Math.Min(buffer.Length, allowance));
                    if (count == 0) break;
                    memory.Write(buffer, 0, count);
                    if (memory.Length > expectedLength) throw new IOException("Managed image grew beyond its captured byte bound");
                }
                bytes = memory.ToArray();
            }
            EnsureUnlinkedPath(full);
            var after = new FileInfo(full);
            after.Refresh();
            if (!after.Exists || after.Length != expectedLength || after.LastWriteTimeUtc != modified || bytes.LongLength != expectedLength)
                throw new IOException("Managed image changed during bounded capture");
            var pathAlias = IsUnderHostRoot(full, Volatile.Read(ref _hostToolsPath))
                ? "@msbuild-host/" + Path.GetRelativePath(_hostToolsPath!, full).Replace('\\', '/')
                : IsWithin(_applicationRoot, full)
                    ? "@worker-app/" + Path.GetRelativePath(_applicationRoot, full).Replace('\\', '/')
                    : "@resolved-dependency/" + Path.GetFileName(full);
            pathAlias = BoundedText(pathAlias);
            return new CapturedImage(bytes, pathAlias,
                Convert.ToHexString(SHA256.HashData(bytes)).ToLowerInvariant(), expectedLength, source);
        }
        catch
        {
            lock (_gate)
            {
                _reservedBytes -= expectedLength;
                _reservedImages--;
            }
            throw;
        }
    }

    private void RegisterLoadedImage(Assembly assembly, CapturedImage captured, AssemblyName? requested = null)
    {
        var actual = assembly.GetName();
        var actualIdentity = BoundedIdentity(actual.FullName ?? actual.Name);
        var requestedIdentity = BoundedIdentity(requested?.FullName ?? actual.FullName ?? actual.Name);
        if (requested is not null && !IdentityMatches(requested, actual))
            throw new FileLoadException("Captured managed image identity differs from the requested assembly");
        lock (_gate)
        {
            EnsureOpen();
            if (_images.Count >= MaximumImages)
            {
                _bookkeepingOverflow = true;
                throw new IOException("Managed image registry exceeded its entry bound");
            }
            _reservedImages--;
            AddBoundedCount(_capturedImageIdentities, actualIdentity);
            _images.Add(new ManagedImageRecord(
                requestedIdentity,
                actualIdentity,
                captured.Sha256,
                captured.Length,
                captured.Source,
                captured.PathAlias));
        }
    }

    private void OnAssemblyLoad(object? sender, AssemblyLoadEventArgs args)
    {
        try
        {
            var assembly = args.LoadedAssembly;
            var context = GetLoadContext(assembly);
            lock (_gate)
            {
                if (_sealed)
                {
                    _bookkeepingOverflow = true;
                    return;
                }
                if (ReferenceEquals(context, this))
                {
                    if (!TryRecordEvent(_ownedLoadEvents, assembly.FullName ?? assembly.GetName().Name))
                        _bookkeepingOverflow = true;
                    return;
                }
                if (ReferenceEquals(context, Default) &&
                    (ReferenceEquals(assembly, _bootstrapAssembly) || IsTrustedDefaultRuntime(assembly))) return;

                _foreignLoadEventCount = Math.Min(MaximumTrackedEvents + 1, _foreignLoadEventCount + 1);
                var contextKind = ReferenceEquals(context, Default) ? "default" : context is null ? "unknown-alc" : "foreign-alc";
                if (!TryRecordEvent(_foreignLoadEvents, contextKind + ":" + (assembly.FullName ?? assembly.GetName().Name)))
                    _bookkeepingOverflow = true;
            }
        }
        catch
        {
            lock (_gate) _bookkeepingOverflow = true;
        }
    }

    private void CompareLoadEventsToCapturedImages()
    {
        foreach (var identity in _ownedLoadEvents.Keys.Concat(_capturedImageIdentities.Keys).Distinct(StringComparer.Ordinal))
        {
            var eventCount = _ownedLoadEvents.GetValueOrDefault(identity);
            var imageCount = _capturedImageIdentities.GetValueOrDefault(identity);
            if (eventCount == imageCount) continue;
            AddBoundedSample(_untrackedLoads, "identity-count-mismatch:" + identity + ":events=" + eventCount + ":images=" + imageCount);
        }
        if (_trackedLoadEvents > MaximumTrackedEvents || _foreignLoadEventCount > MaximumTrackedEvents)
            _bookkeepingOverflow = true;
    }

    private bool TryRecordEvent(Dictionary<string, int> counts, string? key)
    {
        if (_trackedLoadEvents >= MaximumTrackedEvents)
        {
            _bookkeepingOverflow = true;
            _trackedLoadEvents = MaximumTrackedEvents + 1;
            return false;
        }
        _trackedLoadEvents++;
        var bounded = BoundedIdentity(key);
        if (_bookkeepingOverflow || (counts.Count >= MaximumTrackedNames && !counts.ContainsKey(bounded))) return false;
        counts[bounded] = Math.Min(MaximumTrackedEvents + 1, counts.GetValueOrDefault(bounded) + 1);
        return true;
    }

    private void AddBoundedCount(Dictionary<string, int> counts, string key)
    {
        if (counts.Count >= MaximumImages && !counts.ContainsKey(key))
        {
            _bookkeepingOverflow = true;
            return;
        }
        counts[key] = Math.Min(MaximumImages + 1, counts.GetValueOrDefault(key) + 1);
    }

    private List<string> FormatCounts(Dictionary<string, int> counts, string prefix)
    {
        var result = new List<string>(Math.Min(counts.Count, MaximumReportEntries));
        foreach (var pair in counts.OrderBy(item => item.Key, StringComparer.Ordinal).Take(MaximumReportEntries))
            result.Add(BoundedText(prefix + ":" + pair.Key + ":count=" + pair.Value));
        if (counts.Count > MaximumReportEntries) _bookkeepingOverflow = true;
        return result;
    }

    private bool IsTrustedDefaultRuntime(Assembly assembly)
    {
        if (!ReferenceEquals(GetLoadContext(assembly), Default)) return false;
        try
        {
            var name = assembly.GetName().Name;
            var location = assembly.Location;
            return !string.IsNullOrEmpty(name) && name.Length <= MaximumIdentityCharacters &&
                _runtimeAssemblyNames.Contains(name) && !string.IsNullOrEmpty(location) && IsWithin(_runtimeRoot, location);
        }
        catch { return false; }
    }

    private HashSet<string> ReadRuntimeAssemblyNames(string runtimeRoot, out bool overflow)
    {
        var result = new HashSet<string>(StringComparer.OrdinalIgnoreCase);
        overflow = false;
        var paths = AppContext.GetData("TRUSTED_PLATFORM_ASSEMBLIES") as string;
        if (string.IsNullOrEmpty(paths)) return result;
        if (paths.Length > MaximumRuntimeManifestCharacters)
        {
            overflow = true;
            return result;
        }

        var start = 0;
        while (start < paths.Length)
        {
            var separator = paths.IndexOf(Path.PathSeparator, start);
            var end = separator < 0 ? paths.Length : separator;
            var length = end - start;
            if (length > MaximumIdentityCharacters * 8)
            {
                overflow = true;
                return result;
            }
            var path = paths.Substring(start, length);
            try
            {
                if (IsWithin(runtimeRoot, path))
                {
                    var name = Path.GetFileNameWithoutExtension(path);
                    if (name.Length > MaximumIdentityCharacters ||
                        (result.Count >= MaximumRuntimeAssemblyNames && !result.Contains(name)))
                    {
                        overflow = true;
                        return result;
                    }
                    if (name.Length > 0) result.Add(name);
                }
            }
            catch
            {
                overflow = true;
                return result;
            }
            if (separator < 0) break;
            start = separator + 1;
        }
        return result;
    }

    private string BoundedIdentity(string? identity)
    {
        if (string.IsNullOrEmpty(identity)) return "unknown";
        if (identity.Length <= MaximumIdentityCharacters) return identity;
        _bookkeepingOverflow = true;
        return identity[..MaximumIdentityCharacters];
    }

    private string BoundedText(string value)
    {
        if (value.Length <= MaximumIdentityCharacters) return value;
        _bookkeepingOverflow = true;
        return value[..MaximumIdentityCharacters];
    }

    private void AddBoundedSample(List<string> destination, string value)
    {
        if (destination.Count >= MaximumReportEntries)
        {
            _bookkeepingOverflow = true;
            return;
        }
        destination.Add(BoundedText(value));
    }

    private static bool IdentityMatches(AssemblyName requested, AssemblyName actual)
    {
        if (!string.Equals(requested.Name, actual.Name, StringComparison.OrdinalIgnoreCase)) return false;
        if (requested.Version is not null && requested.Version != actual.Version) return false;
        if (!string.IsNullOrEmpty(requested.CultureName) && !string.Equals(requested.CultureName, actual.CultureName, StringComparison.OrdinalIgnoreCase)) return false;
        var requestedToken = requested.GetPublicKeyToken() ?? [];
        var actualToken = actual.GetPublicKeyToken() ?? [];
        return requestedToken.Length == 0 || requestedToken.AsSpan().SequenceEqual(actualToken);
    }

    private static bool IsHostEngineAssembly(string name)
        => !string.Equals(name, "Microsoft.Build.Locator", StringComparison.OrdinalIgnoreCase) &&
           (name.StartsWith("Microsoft.Build", StringComparison.OrdinalIgnoreCase) || name.StartsWith("MSBuild", StringComparison.OrdinalIgnoreCase));

    private bool IsUnderHostRoot(string path, string? hostToolsPath)
        => hostToolsPath is not null && IsWithin(hostToolsPath, path);

    private static bool IsWithin(string root, string path)
    {
        var relative = Path.GetRelativePath(Path.GetFullPath(root), Path.GetFullPath(path));
        return relative != "." && relative != ".." && !relative.StartsWith(".." + Path.DirectorySeparatorChar, StringComparison.Ordinal) && !Path.IsPathFullyQualified(relative);
    }

    private static bool SamePath(string left, string right)
    {
        var comparer = OperatingSystem.IsWindows() ? StringComparer.OrdinalIgnoreCase : StringComparer.Ordinal;
        return comparer.Equals(Path.TrimEndingDirectorySeparator(Path.GetFullPath(left)), Path.TrimEndingDirectorySeparator(Path.GetFullPath(right)));
    }

    private static void EnsureUnlinkedPath(string path)
    {
        var full = Path.GetFullPath(path);
        var root = Path.GetPathRoot(full) ?? throw new IOException("Managed image path has no root");
        var current = root;
        foreach (var segment in full[root.Length..].Split(new[] { Path.DirectorySeparatorChar, Path.AltDirectorySeparatorChar }, StringSplitOptions.RemoveEmptyEntries))
        {
            current = Path.Combine(current, segment);
            if ((File.GetAttributes(current) & FileAttributes.ReparsePoint) != 0)
                throw new IOException("Linked managed image paths are unsupported");
        }
    }

    private void EnsureOpen()
    {
        if (_sealed) throw new FileLoadException("Managed image registry is sealed");
    }

    private sealed record CapturedImage(byte[] Bytes, string PathAlias, string Sha256, long Length, string Source);
}

internal sealed record ManagedImageRecord(
    string RequestedIdentity,
    string AssemblyIdentity,
    string Sha256,
    long Length,
    string Source,
    string PathAlias);

internal sealed record ManagedImageCaptureReceipt(
    string Status,
    bool Complete,
    string RegistryId,
    bool CoreHostImagesCaptured,
    bool BaselineTrusted,
    int BaselineAssembliesChecked,
    int TrackedLoadEvents,
    int ForeignLoadEventCount,
    bool BookkeepingOverflow,
    IReadOnlyList<ManagedImageRecord> Images,
    IReadOnlyList<string> UnmediatedLoads,
    IReadOnlyList<string> UntrackedLoads,
    IReadOnlyList<string> UnknownBaselineComponents,
    string RuntimeBclAssumption);
