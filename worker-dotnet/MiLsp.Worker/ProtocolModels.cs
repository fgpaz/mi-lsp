using System.Text.Json;
using System.Text.Json.Serialization;

namespace MiLsp.Worker;

public sealed record WorkerRequest(
    [property: JsonPropertyName("protocol_version")] string? ProtocolVersion,
    [property: JsonPropertyName("method")] string Method,
    [property: JsonPropertyName("workspace")] string Workspace,
    [property: JsonPropertyName("workspace_name")] string? WorkspaceName,
    [property: JsonPropertyName("backend_type")] string? BackendType,
    [property: JsonPropertyName("repo_id")] string? RepoId,
    [property: JsonPropertyName("repo_name")] string? RepoName,
    [property: JsonPropertyName("repo_root")] string? RepoRoot,
    [property: JsonPropertyName("entrypoint_id")] string? EntrypointId,
    [property: JsonPropertyName("entrypoint_path")] string? EntrypointPath,
    [property: JsonPropertyName("entrypoint_type")] string? EntrypointType,
    [property: JsonPropertyName("payload")] Dictionary<string, JsonElement>? Payload
);

public sealed record WorkerStats(
    [property: JsonPropertyName("symbols")] int Symbols = 0,
    [property: JsonPropertyName("files")] int Files = 0,
    [property: JsonPropertyName("ms")] long Ms = 0
);

public sealed record WorkerResponse(
    [property: JsonPropertyName("ok")] bool Ok,
    [property: JsonPropertyName("backend")] string? Backend = null,
    [property: JsonPropertyName("items")] List<Dictionary<string, object?>>? Items = null,
    [property: JsonPropertyName("warnings")] List<string>? Warnings = null,
    [property: JsonPropertyName("error")] string? Error = null,
    [property: JsonPropertyName("stats")] WorkerStats? Stats = null
);

internal static class WorkerProtocol
{
    public const string Version = "mi-lsp-v1.1";
}

internal static class PayloadHelpers
{
    public static string? GetString(this Dictionary<string, JsonElement>? payload, string key)
    {
        if (payload is null || !payload.TryGetValue(key, out var value))
        {
            return null;
        }

        return value.ValueKind switch
        {
            JsonValueKind.String => value.GetString(),
            JsonValueKind.Number => value.ToString(),
            JsonValueKind.True => "true",
            JsonValueKind.False => "false",
            _ => null,
        };
    }

    public static int GetInt(this Dictionary<string, JsonElement>? payload, string key, int defaultValue = 0)
    {
        if (payload is null || !payload.TryGetValue(key, out var value))
        {
            return defaultValue;
        }

        if (value.ValueKind == JsonValueKind.Number && value.TryGetInt32(out var number))
        {
            return number;
        }

        return int.TryParse(value.ToString(), out var parsed) ? parsed : defaultValue;
    }
}
