using System.Text.Json;
using System.Security.Cryptography;
using Microsoft.CodeAnalysis;
using Microsoft.CodeAnalysis.CSharp.Syntax;

namespace MiLsp.Worker;

// Distinguishes an absent legacy scope from an explicitly empty delta.
internal static class GraphDeltaObservation
{
    internal static bool TryReadPaths(Dictionary<string, JsonElement>? payload, string root, out string[]? paths)
    {
        paths = null;
        if (payload is null || !payload.TryGetValue("affected_paths", out var value)) return true;
        if (value.ValueKind != JsonValueKind.Array) return false;

        var normalized = new SortedSet<string>(StringComparer.Ordinal);
        try
        {
            var fullRoot = Path.GetFullPath(root);
            if (fullRoot.Length == 0 || !Directory.Exists(fullRoot) || HasReparsePoint(fullRoot)) return false;
            foreach (var item in value.EnumerateArray())
            {
                if (item.ValueKind != JsonValueKind.String) return false;
                var input = item.GetString();
                if (string.IsNullOrEmpty(input) || Path.IsPathRooted(input) || input.Contains('\\')) return false;
                var segments = input.Split('/');
                if (segments.Any(segment => segment.Length == 0 || segment == "." || segment == "..")) return false;
                var full = Path.GetFullPath(Path.Combine(fullRoot, input));
                var relative = Path.GetRelativePath(fullRoot, full).Replace('\\', '/');
                if (relative == "." || relative == ".." || relative.StartsWith("../", StringComparison.Ordinal) || Path.IsPathRooted(relative)) return false;
                if (HasReparsePointInExistingPath(fullRoot, full)) return false;
                normalized.Add(relative);
            }
        }
        catch (Exception e) when (e is ArgumentException or NotSupportedException or IOException or UnauthorizedAccessException)
        {
            return false;
        }
        paths = normalized.ToArray();
        return true;
    }

    internal static bool HasReparsePointInExistingPath(string root, string path)
    {
        var relative = Path.GetRelativePath(root, path);
        var current = root;
        if (HasReparsePoint(current)) return true;
        foreach (var segment in relative.Split(Path.DirectorySeparatorChar, Path.AltDirectorySeparatorChar))
        {
            if (segment is "" or ".") continue;
            current = Path.Combine(current, segment);
            try
            {
                if ((File.GetAttributes(current) & FileAttributes.ReparsePoint) != 0) return true;
            }
            catch (FileNotFoundException) { break; }
            catch (DirectoryNotFoundException) { break; }
        }
        return false;
    }

    // Inspect every component without Exists, which hides access and IO errors.
    internal static bool PathIsFile(string root, string relative)
    {
        var current = root;
        foreach (var segment in relative.Split('/'))
        {
            current = Path.Combine(current, segment);
            FileAttributes attributes;
            try { attributes = File.GetAttributes(current); }
            catch (FileNotFoundException) { return false; }
            catch (DirectoryNotFoundException) { return false; }
            if ((attributes & FileAttributes.ReparsePoint) != 0)
                throw new IOException("Ambiguous symbolic link in delta path");
            if (current != Path.Combine(root, relative) && (attributes & FileAttributes.Directory) == 0)
                throw new IOException("Non-directory delta path component");
            if (current == Path.Combine(root, relative) && (attributes & FileAttributes.Directory) != 0)
                throw new IOException("Delta path is a directory");
        }
        return true;
    }

    internal static string FileDigest(string path) => Convert.ToHexString(SHA256.HashData(File.ReadAllBytes(path)));

    // Expand only declarations in selected documents, never a prebuilt full graph.
    internal static async Task<(string[] Paths, Dictionary<string, string> Digests)> ExpandPartialClosureAsync(
        Project project, string root, string[] applied, CancellationToken cancellationToken,
        CapturedRoslynCompilation? captured = null)
    {
        if (captured is not null && !ReferenceEquals(project, captured.Project))
            throw new IOException("Controlled closure is not using the captured project");
        var capturedDigests = captured?.Inputs.Where(input => input.Kind == "source")
            .ToDictionary(input => Path.GetRelativePath(root, input.FullPath).Replace('\\', '/'),
                input => input.Sha256, StringComparer.Ordinal);
        var documents = new Dictionary<string, Document>(StringComparer.Ordinal);
        foreach (var document in project.Documents)
        {
            if (document.FilePath is null) continue;
            var full = Path.GetFullPath(document.FilePath);
            var relative = Path.GetRelativePath(root, full).Replace('\\', '/');
            if (relative == ".." || relative.StartsWith("../", StringComparison.Ordinal) || Path.IsPathRooted(relative)) continue;
            if (!documents.TryAdd(relative, document))
                throw new IOException("Ambiguous project document path");
        }
        var pending = new Queue<string>(applied);
        var selected = new SortedSet<string>(StringComparer.Ordinal);
        var digests = new Dictionary<string, string>(StringComparer.Ordinal);
        while (pending.Count != 0)
        {
            cancellationToken.ThrowIfCancellationRequested();
            var path = pending.Dequeue();
            if (!selected.Add(path)) continue;
            if (!documents.TryGetValue(path, out var document) || !PathIsFile(root, path))
                throw new IOException("Partial declaration document unavailable");
            var full = Path.Combine(root, path);
            string digest;
            var text = await document.GetTextAsync(cancellationToken).ConfigureAwait(false);
            if (capturedDigests is null)
            {
                digest = FileDigest(full);
                if (!string.Equals(text.ToString(), File.ReadAllText(full), StringComparison.Ordinal))
                    throw new IOException("Project document differs from disk");
            }
            else if (!capturedDigests.TryGetValue(path, out digest!) ||
                !string.Equals(Convert.ToHexString(text.GetChecksum().ToArray()), digest, StringComparison.OrdinalIgnoreCase))
                throw new IOException("Controlled closure source differs from captured compiler bytes");
            var syntax = await document.GetSyntaxRootAsync(cancellationToken).ConfigureAwait(false);
            if (syntax is null) throw new IOException("Partial declaration syntax unavailable");
            var model = captured is null
                ? await document.GetSemanticModelAsync(cancellationToken).ConfigureAwait(false)
                : captured.Compilation.GetSemanticModel(syntax.SyntaxTree);
            if (model is null) throw new IOException("Partial declaration semantics unavailable");
            digests.Add(path, digest);
            foreach (var node in syntax.DescendantNodesAndSelf())
            {
                if (node is not (BaseTypeDeclarationSyntax or DelegateDeclarationSyntax or BaseMethodDeclarationSyntax or PropertyDeclarationSyntax or EventDeclarationSyntax or VariableDeclaratorSyntax)) continue;
                var symbol = model.GetDeclaredSymbol(node, cancellationToken);
                if (symbol is null) throw new IOException("Declaration symbol unavailable");
                var symbols = new List<ISymbol> { symbol };
                if (symbol is IMethodSymbol method)
                {
                    if (method.PartialDefinitionPart is not null) symbols.Add(method.PartialDefinitionPart);
                    if (method.PartialImplementationPart is not null) symbols.Add(method.PartialImplementationPart);
                }
                foreach (var part in symbols)
                foreach (var reference in part.DeclaringSyntaxReferences)
                {
                    var file = reference.SyntaxTree.FilePath;
                    if (string.IsNullOrEmpty(file)) throw new IOException("Declaration source unavailable");
                    var owner = Path.GetRelativePath(root, Path.GetFullPath(file)).Replace('\\', '/');
                    if (!documents.TryGetValue(owner, out var ownerDocument))
                        throw new IOException("Declaration outside project or repository");
                    var ownerTree = await ownerDocument.GetSyntaxTreeAsync(cancellationToken).ConfigureAwait(false);
                    if (ownerTree != reference.SyntaxTree)
                        throw new IOException("Ambiguous declaration source");
                    if (!selected.Contains(owner)) pending.Enqueue(owner);
                }
            }
        }
        return (selected.ToArray(), digests);
    }

    private static bool HasReparsePoint(string path)
    {
        try { return (File.GetAttributes(path) & FileAttributes.ReparsePoint) != 0; }
        catch (FileNotFoundException) { return false; }
        catch (DirectoryNotFoundException) { return false; }
    }
}
