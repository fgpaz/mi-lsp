using System.Security.Cryptography;
using System.Text;
using Microsoft.Build.FileSystem;

namespace MiLsp.Worker;

/// <summary>
/// Thread-safe, bounded snapshot adapter for the public MSBuild filesystem seam.
/// Captures calls routed through this adapter only; property functions, SDK
/// resolvers, tasks, and other direct I/O are explicitly outside its claim.
/// </summary>
internal sealed class CapturedEvaluationFileSystem : MSBuildFileSystemBase
{
    private const int MaximumEntries = 4096;
    private const int MaximumFileBytes = 16 * 1024 * 1024;
    private const long MaximumReadBytes = 256L * 1024 * 1024;
    private const long MaximumEnumerationPathChars = 8L * 1024 * 1024;
    private readonly object _gate = new();
    private readonly Dictionary<string, PathSnapshot> _paths = new(PathComparer);
    private readonly Dictionary<string, EnumerationSnapshot> _enumerations = new(StringComparer.Ordinal);
    private readonly List<CapturedFileSystemCall> _calls = [];
    private long _readBytes;
    private long _enumerationPathChars;
    private bool _sealed;

    internal IReadOnlyList<CapturedFileSystemInput> Inputs
    {
        get
        {
            lock (_gate)
                return _paths.Values.Where(value => value.Bytes is not null)
                    .OrderBy(value => value.Path, PathComparer)
                    .Select(value => new CapturedFileSystemInput(value.Path,
                        Convert.ToHexString(SHA256.HashData(value.Bytes!)), value.Bytes!.LongLength,
                        value.ReadMethods.ToArray()))
                    .ToArray();
        }
    }

    internal IReadOnlyList<CapturedFileSystemObservation> Observations
    {
        get
        {
            lock (_gate)
            {
                var paths = _paths.Values.OrderBy(value => value.Path, PathComparer)
                    .Select(value => new CapturedFileSystemObservation(value.Path,
                        value.Present ? "present" : "absent", value.Attributes?.ToString(), value.LastWriteUtc,
                        value.Bytes is null ? null : Convert.ToHexString(SHA256.HashData(value.Bytes)),
                        value.Methods.ToArray()))
                    .ToList();
                paths.AddRange(_enumerations.Values.OrderBy(value => value.Key, StringComparer.Ordinal)
                    .Select(value => new CapturedFileSystemObservation(value.Directory, "enumerated",
                        value.Key, null, DigestPaths(value.Paths), Array.Empty<string>())));
                return paths;
            }
        }
    }

    internal IReadOnlyList<CapturedFileSystemCall> Calls
    {
        get { lock (_gate) return _calls.ToArray(); }
    }

    internal bool IsSealed { get { lock (_gate) return _sealed; } }

    internal string Coverage => "partial: calls through MSBuildFileSystemBase only; evaluator filesystem/environment bypasses are unobserved";

    public override TextReader ReadFile(string path)
    {
        var bytes = ReadBytes(path, nameof(ReadFile));
        return new StreamReader(new MemoryStream(bytes, writable: false), Encoding.UTF8, detectEncodingFromByteOrderMarks: true);
    }

    public override Stream GetFileStream(string path, FileMode mode, FileAccess access, FileShare share)
    {
        if (mode != FileMode.Open || access != FileAccess.Read)
            throw Unsupported(path, nameof(GetFileStream), "only FileMode.Open/FileAccess.Read is supported");
        return new MemoryStream(ReadBytes(path, nameof(GetFileStream)), writable: false);
    }

    public override string ReadFileAllText(string path)
    {
        var bytes = ReadBytes(path, nameof(ReadFileAllText));
        using var reader = new StreamReader(new MemoryStream(bytes, writable: false), Encoding.UTF8, detectEncodingFromByteOrderMarks: true);
        return reader.ReadToEnd();
    }

    public override byte[] ReadFileAllBytes(string path) => ReadBytes(path, nameof(ReadFileAllBytes));

    public override IEnumerable<string> EnumerateFiles(string path, string searchPattern = "*", SearchOption searchOption = SearchOption.TopDirectoryOnly)
        => Enumerate(path, searchPattern, searchOption, EntryKind.Files);

    public override IEnumerable<string> EnumerateDirectories(string path, string searchPattern = "*", SearchOption searchOption = SearchOption.TopDirectoryOnly)
        => Enumerate(path, searchPattern, searchOption, EntryKind.Directories);

    public override IEnumerable<string> EnumerateFileSystemEntries(string path, string searchPattern = "*", SearchOption searchOption = SearchOption.TopDirectoryOnly)
        => Enumerate(path, searchPattern, searchOption, EntryKind.All);

    public override FileAttributes GetAttributes(string path)
    {
        lock (_gate)
        {
            EnsureOpen();
            var snapshot = GetPathSnapshot(path, nameof(GetAttributes));
            if (!snapshot.Present) throw new FileNotFoundException("Captured path is absent", snapshot.Path);
            return snapshot.Attributes!.Value;
        }
    }

    public override DateTime GetLastWriteTimeUtc(string path)
    {
        lock (_gate)
        {
            EnsureOpen();
            var snapshot = GetPathSnapshot(path, nameof(GetLastWriteTimeUtc));
            return snapshot.Present ? snapshot.LastWriteUtc : DateTime.FromFileTimeUtc(0);
        }
    }

    public override bool DirectoryExists(string path)
    {
        lock (_gate)
        {
            EnsureOpen();
            var snapshot = GetPathSnapshot(path, nameof(DirectoryExists));
            return snapshot.Present && (snapshot.Attributes!.Value & FileAttributes.Directory) != 0;
        }
    }

    public override bool FileExists(string path)
    {
        lock (_gate)
        {
            EnsureOpen();
            var snapshot = GetPathSnapshot(path, nameof(FileExists));
            return snapshot.Present && (snapshot.Attributes!.Value & FileAttributes.Directory) == 0;
        }
    }

    public override bool FileOrDirectoryExists(string path)
    {
        lock (_gate)
        {
            EnsureOpen();
            return GetPathSnapshot(path, nameof(FileOrDirectoryExists)).Present;
        }
    }

    internal void SealAndValidate()
    {
        lock (_gate)
        {
            EnsureOpen();
            foreach (var snapshot in _paths.Values)
            {
                var current = ReadPathState(snapshot.Path);
                if (current.Present != snapshot.Present || current.Attributes != snapshot.Attributes ||
                    current.LastWriteUtc != snapshot.LastWriteUtc || current.Length != snapshot.Length)
                    throw new IOException("Captured MSBuild path state changed during the controlled evaluation");
                if (snapshot.Bytes is not null)
                {
                    var currentBytes = ReadBoundedFile(snapshot.Path, snapshot.Bytes.LongLength);
                    if (!currentBytes.AsSpan().SequenceEqual(snapshot.Bytes))
                        throw new IOException("Captured MSBuild file bytes changed during the controlled evaluation");
                }
            }
            foreach (var enumeration in _enumerations.Values)
            {
                var current = EnumerateUncached(enumeration.Directory, enumeration.Pattern, enumeration.SearchOption, enumeration.Kind);
                if (!current.SequenceEqual(enumeration.Paths, PathComparer))
                    throw new IOException("Captured MSBuild directory enumeration changed during the controlled evaluation");
            }
            _sealed = true;
        }
    }

    private byte[] ReadBytes(string path, string method)
    {
        lock (_gate)
        {
            EnsureOpen();
            var snapshot = GetPathSnapshot(path, method);
            if (!snapshot.Present) throw new FileNotFoundException("Captured MSBuild file is absent", snapshot.Path);
            if ((snapshot.Attributes!.Value & FileAttributes.Directory) != 0) throw new IOException("Captured MSBuild read target is a directory");
            if (snapshot.Bytes is null)
            {
                if (snapshot.Length > MaximumFileBytes || _readBytes + snapshot.Length > MaximumReadBytes)
                    throw new IOException("Captured MSBuild file read exceeded its byte budget");
                var bytes = ReadBoundedFile(snapshot.Path, Math.Min(MaximumFileBytes, MaximumReadBytes - _readBytes));
                var after = ReadPathState(snapshot.Path);
                if (!after.Present || after.Attributes != snapshot.Attributes || after.LastWriteUtc != snapshot.LastWriteUtc ||
                    after.Length != snapshot.Length || bytes.LongLength != snapshot.Length)
                    throw new IOException("Captured MSBuild file changed while its bytes were read");
                snapshot.Bytes = bytes;
                snapshot.ReadMethods.Add(method);
                _readBytes += bytes.LongLength;
            }
            else if (!snapshot.ReadMethods.Contains(method, StringComparer.Ordinal))
            {
                snapshot.ReadMethods.Add(method);
            }
            return snapshot.Bytes.ToArray();
        }
    }

    private PathSnapshot GetPathSnapshot(string path, string method)
    {
        var fullPath = Path.GetFullPath(path);
        if (_paths.TryGetValue(fullPath, out var cached))
        {
            cached.Methods.Add(method);
            _calls.Add(new CapturedFileSystemCall(method, fullPath, cached.Present ? "present" : "absent"));
            BoundObservations();
            return cached;
        }
        PathState state;
        try
        {
            EnsureUnlinkedPath(fullPath);
            state = ReadPathState(fullPath);
        }
        catch (Exception exception) when (exception is IOException or UnauthorizedAccessException or ArgumentException)
        {
            _calls.Add(new CapturedFileSystemCall(method, fullPath, "error:" + exception.GetType().Name));
            throw;
        }
        var snapshot = new PathSnapshot(fullPath, state.Present, state.Attributes, state.LastWriteUtc, state.Length);
        snapshot.Methods.Add(method);
        _paths.Add(fullPath, snapshot);
        _calls.Add(new CapturedFileSystemCall(method, fullPath, state.Present ? "present" : "absent"));
        BoundObservations();
        return snapshot;
    }

    private IEnumerable<string> Enumerate(string path, string pattern, SearchOption option, EntryKind kind)
    {
        lock (_gate)
        {
            EnsureOpen();
            if (option is not (SearchOption.TopDirectoryOnly or SearchOption.AllDirectories))
                throw Unsupported(path, "Enumerate", "unsupported SearchOption");
            var directory = Path.GetFullPath(path);
            var key = $"{(int)kind}\0{(int)option}\0{directory}\0{pattern}";
            if (_enumerations.TryGetValue(key, out var cached))
            {
                _calls.Add(new CapturedFileSystemCall("Enumerate", directory, "cached"));
                BoundObservations();
                return cached.Paths.ToArray();
            }
            var dirSnapshot = GetPathSnapshot(directory, "EnumerateDirectory");
            if (!dirSnapshot.Present || (dirSnapshot.Attributes!.Value & FileAttributes.Directory) == 0)
                throw new DirectoryNotFoundException("Captured MSBuild enumeration directory is unavailable");
            List<string> result;
            try { result = EnumerateUncached(directory, pattern, option, kind); }
            catch (Exception exception) when (exception is IOException or UnauthorizedAccessException or ArgumentException)
            {
                _calls.Add(new CapturedFileSystemCall("Enumerate", directory, "error:" + exception.GetType().Name));
                throw;
            }
            _enumerationPathChars += result.Sum(value => (long)value.Length);
            if (result.Count > MaximumEntries || _enumerationPathChars > MaximumEnumerationPathChars)
                throw new IOException("Captured MSBuild directory enumeration exceeded its bound");
            _enumerations.Add(key, new EnumerationSnapshot(key, directory, pattern, option, kind, result));
            _calls.Add(new CapturedFileSystemCall("Enumerate", directory, "enumerated"));
            BoundObservations();
            return result.ToArray();
        }
    }

    private List<string> EnumerateUncached(string directory, string pattern, SearchOption option, EntryKind kind)
    {
        var pending = new Stack<string>();
        pending.Push(directory);
        var entries = new List<string>();
        long pathChars = 0;
        var visitedDirectories = 0;
        void AddEntry(string entry)
        {
            EnsureUnlinkedPath(entry);
            pathChars += entry.Length;
            if (entries.Count >= MaximumEntries || pathChars > MaximumEnumerationPathChars)
                throw new IOException("Captured MSBuild directory enumeration exceeded its bound");
            entries.Add(entry);
        }
        while (pending.Count > 0)
        {
            var current = pending.Pop();
            if (++visitedDirectories > MaximumEntries) throw new IOException("Captured MSBuild directory traversal exceeded its bound");
            EnsureUnlinkedPath(current);
            var currentState = ReadPathState(current);
            if (!currentState.Present || (currentState.Attributes!.Value & FileAttributes.Directory) == 0)
                throw new DirectoryNotFoundException("Captured MSBuild recursive directory is unavailable");
            if (!_paths.ContainsKey(current))
            {
                var snapshot = new PathSnapshot(current, currentState.Present, currentState.Attributes, currentState.LastWriteUtc, currentState.Length);
                snapshot.Methods.Add("EnumerateDirectory");
                _paths.Add(current, snapshot);
                if (_paths.Count > MaximumEntries) throw new IOException("Captured MSBuild path observation exceeded its bound");
            }
            if (option == SearchOption.AllDirectories)
            {
                foreach (var dir in Directory.EnumerateDirectories(current, "*", SearchOption.TopDirectoryOnly))
                {
                    EnsureUnlinkedPath(dir);
                    if (option == SearchOption.AllDirectories)
                    {
                        if (pending.Count >= MaximumEntries) throw new IOException("Captured MSBuild directory traversal exceeded its bound");
                        pending.Push(dir);
                    }
                }
            }
            if (kind is EntryKind.Files or EntryKind.All)
                foreach (var file in Directory.EnumerateFiles(current, pattern, SearchOption.TopDirectoryOnly)) AddEntry(file);
            if (kind is EntryKind.Directories or EntryKind.All)
                foreach (var child in Directory.EnumerateDirectories(current, pattern, SearchOption.TopDirectoryOnly)) AddEntry(child);
        }
        entries.Sort(PathComparer);
        return entries;
    }

    private PathState ReadPathState(string path)
    {
        EnsureUnlinkedPath(path);
        try
        {
            var attributes = File.GetAttributes(path);
            var writeTime = File.GetLastWriteTimeUtc(path);
            var length = (attributes & FileAttributes.Directory) == 0 ? new FileInfo(path).Length : 0;
            return new PathState(true, attributes, writeTime, length);
        }
        catch (FileNotFoundException) { return PathState.Absent; }
        catch (DirectoryNotFoundException) { return PathState.Absent; }
    }

    private static byte[] ReadBoundedFile(string path, long maxBytes)
    {
        if (maxBytes < 0 || maxBytes > int.MaxValue) throw new IOException("Captured MSBuild file byte bound is invalid");
        using var stream = new FileStream(path, FileMode.Open, FileAccess.Read, FileShare.Read, 64 * 1024, FileOptions.SequentialScan);
        using var output = new MemoryStream((int)Math.Min(stream.Length, maxBytes));
        var buffer = new byte[64 * 1024];
        while (true)
        {
            var allowance = maxBytes + 1 - output.Length;
            if (allowance <= 0) throw new IOException("Captured MSBuild file exceeds its read bound");
            var count = stream.Read(buffer, 0, (int)Math.Min(buffer.Length, allowance));
            if (count == 0) break;
            output.Write(buffer, 0, count);
            if (output.Length > maxBytes) throw new IOException("Captured MSBuild file exceeds its read bound");
        }
        return output.ToArray();
    }

    private static void EnsureUnlinkedPath(string fullPath)
    {
        var current = Path.GetPathRoot(fullPath) ?? throw new IOException("Captured MSBuild path has no root");
        foreach (var segment in fullPath[current.Length..].Split(new[] { Path.DirectorySeparatorChar, Path.AltDirectorySeparatorChar }, StringSplitOptions.RemoveEmptyEntries))
        {
            current = Path.Combine(current, segment);
            FileAttributes attributes;
            try { attributes = File.GetAttributes(current); }
            catch (FileNotFoundException) { return; }
            catch (DirectoryNotFoundException) { return; }
            if ((attributes & FileAttributes.ReparsePoint) != 0)
                throw new IOException("Linked/reparse MSBuild paths are outside the captured filesystem subset");
        }
    }

    private IOException Unsupported(string path, string method, string reason)
    {
        lock (_gate)
        {
            _calls.Add(new CapturedFileSystemCall(method, path, "incomplete:" + reason));
            BoundObservations();
        }
        return new IOException("Captured MSBuild filesystem operation is unsupported: " + reason);
    }

    private void BoundObservations()
    {
        if (_paths.Count > MaximumEntries || _enumerations.Count > MaximumEntries || _calls.Count > MaximumEntries * 4)
            throw new IOException("Captured MSBuild filesystem observation exceeded its operation bound");
    }

    private void EnsureOpen()
    {
        if (_sealed) throw new IOException("Captured MSBuild filesystem snapshot is sealed");
    }

    private static string DigestPaths(IEnumerable<string> paths)
        => Convert.ToHexString(SHA256.HashData(Encoding.UTF8.GetBytes(string.Join("\0", paths))));

    private static StringComparer PathComparer => OperatingSystem.IsWindows() ? StringComparer.OrdinalIgnoreCase : StringComparer.Ordinal;
    private enum EntryKind { Files, Directories, All }
    private sealed class PathSnapshot(string path, bool present, FileAttributes? attributes, DateTime lastWriteUtc, long length)
    {
        internal string Path { get; } = path;
        internal bool Present { get; } = present;
        internal FileAttributes? Attributes { get; } = attributes;
        internal DateTime LastWriteUtc { get; } = lastWriteUtc;
        internal long Length { get; } = length;
        internal byte[]? Bytes { get; set; }
        internal List<string> Methods { get; } = [];
        internal List<string> ReadMethods { get; } = [];
    }
    private sealed record EnumerationSnapshot(string Key, string Directory, string Pattern, SearchOption SearchOption, EntryKind Kind, List<string> Paths);
    private sealed record PathState(bool Present, FileAttributes? Attributes, DateTime LastWriteUtc, long Length)
    {
        internal static PathState Absent { get; } = new(false, null, DateTime.FromFileTimeUtc(0), 0);
    }
}

internal sealed record CapturedFileSystemCall(string Method, string Path, string State);
internal sealed record CapturedFileSystemInput(string FullPath, string Sha256, long Length, IReadOnlyList<string> ReadMethods);
internal sealed record CapturedFileSystemObservation(string Path, string State, string? Metadata, DateTime? LastWriteUtc, string? Digest, IReadOnlyList<string> Methods);
