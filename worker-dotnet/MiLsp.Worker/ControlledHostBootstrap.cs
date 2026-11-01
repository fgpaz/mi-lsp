using System.Buffers.Binary;
using System.Collections;
using System.Reflection;
using System.Runtime.Loader;
using System.Security.Cryptography;
using System.Text.Json;
using System.Text.Json.Nodes;

namespace MiLsp.Worker;

/// <summary>Fresh-process entry point. It never registers MSBuild in the default load context.</summary>
internal static class ControlledHostBootstrap
{
    internal const string Argument = "--controlled-host";
    private const int MaximumRequestBytes = 16 * 1024 * 1024;
    private const int MaximumResponseBytes = 64 * 1024 * 1024;
    private static readonly JsonSerializerOptions JsonOptions = new(JsonSerializerDefaults.Web);
    internal static bool IsControlledHostProcess => Environment.GetCommandLineArgs()
        .Any(argument => string.Equals(argument, Argument, StringComparison.Ordinal));

    internal static async Task<int> RunAsync()
    {
        var output = Console.OpenStandardOutput();
        Console.SetOut(TextWriter.Null);
        byte[] response;
        try
        {
            var requestBytes = await ReadFrameAsync(Console.OpenStandardInput(), MaximumRequestBytes, CancellationToken.None).ConfigureAwait(false);
            if (requestBytes is null) throw new IOException("Controlled host request frame is missing");
            response = await ExecuteAsync(requestBytes).ConfigureAwait(false);
        }
        catch
        {
            response = FailureResponse("Controlled host bootstrap or managed-image capture failed");
        }

        try
        {
            await WriteFrameAsync(output, response, MaximumResponseBytes, CancellationToken.None).ConfigureAwait(false);
            await output.FlushAsync().ConfigureAwait(false);
        }
        catch { return 2; }
        return 0;
    }

    private static async Task<byte[]> ExecuteAsync(byte[] requestBytes)
    {
        var entryPath = Assembly.GetEntryAssembly()?.Location;
        if (string.IsNullOrWhiteSpace(entryPath) || !File.Exists(entryPath))
            return FailureResponse("Controlled host requires a file-backed worker entry assembly");

        var bootstrapAssembly = Assembly.GetEntryAssembly();
        using var loader = new CapturedManagedImageLoadContext(entryPath, bootstrapAssembly);
        try
        {
            if (!loader.BaselineTrusted)
            {
                var baselineReceipt = loader.SealAndGetReceipt();
                return AttachReceipt(FailureResponse("Controlled host baseline contains untrusted or overflowed assemblies"), baselineReceipt, requestBytes);
            }
            var workerAssembly = loader.LoadWorkerImage();
            var locatorAssembly = loader.LoadFromAssemblyName(new AssemblyName("Microsoft.Build.Locator"));
            var hostToolsPath = QueryHostToolsPath(locatorAssembly);
            loader.PinHostToolsPath(hostToolsPath);
            PublishCapturedHost(workerAssembly, hostToolsPath);

            var requestType = workerAssembly.GetType("MiLsp.Worker.WorkerRequest", throwOnError: true)!;
            var request = JsonSerializer.Deserialize(requestBytes, requestType, JsonOptions)
                ?? throw new JsonException("Controlled host request is invalid");
            ValidateOptInRequest(request, requestType);

            var serviceType = workerAssembly.GetType("MiLsp.Worker.RoslynService", throwOnError: true)!;
            var service = Activator.CreateInstance(serviceType)
                ?? throw new InvalidOperationException("Controlled Roslyn service could not be created");
            var handle = serviceType.GetMethods(BindingFlags.Instance | BindingFlags.NonPublic)
                .SingleOrDefault(method => method.Name == "HandleControlledHostAsync" && method.GetParameters().Length == 2 &&
                    method.GetParameters()[0].ParameterType == requestType && method.GetParameters()[1].ParameterType == typeof(CancellationToken))
                ?? throw new MissingMethodException("Direct controlled Roslyn producer entry is unavailable");
            var pending = handle.Invoke(service, [request, CancellationToken.None]) as Task
                ?? throw new InvalidOperationException("Controlled Roslyn request handler returned no task");
            await pending.ConfigureAwait(false);
            var result = pending.GetType().GetProperty("Result")?.GetValue(pending)
                ?? throw new InvalidOperationException("Controlled Roslyn request handler returned no response");
            var responseType = result.GetType();
            var isOk = responseType.GetProperty("Ok")?.GetValue(result) as bool? ?? true;
            var observation = responseType.GetProperty("Observation")?.GetValue(result);
            var responseBytes = isOk || observation is not null
                ? FailureResponse("Controlled host refused an unexpected stageable observation")
                : JsonSerializer.SerializeToUtf8Bytes(result, responseType, JsonOptions);

            var receipt = loader.SealAndGetReceipt();
            return AttachReceipt(responseBytes, receipt, requestBytes);
        }
        catch
        {
            var receipt = loader.SealAndGetReceipt();
            return AttachReceipt(FailureResponse("Controlled host capture or evaluation failed"), receipt, requestBytes);
        }
    }

    private static void ValidateOptInRequest(object request, Type requestType)
    {
        if (!string.Equals(requestType.GetProperty("Method")?.GetValue(request) as string, "graph_observe", StringComparison.Ordinal))
            throw new InvalidOperationException("Controlled host accepts graph_observe only");
        if (requestType.GetProperty("Payload")?.GetValue(request) is not Dictionary<string, JsonElement> payload ||
            !payload.TryGetValue("control_inputs", out var control) || control.ValueKind != JsonValueKind.True)
            throw new InvalidOperationException("Controlled host requires the explicit control_inputs opt-in");
    }

    private static string QueryHostToolsPath(Assembly locatorAssembly)
    {
        var locatorType = locatorAssembly.GetType("Microsoft.Build.Locator.MSBuildLocator", throwOnError: true)!;
        var query = locatorType.GetMethod("QueryVisualStudioInstances", BindingFlags.Public | BindingFlags.Static, Type.EmptyTypes)
            ?? throw new MissingMethodException("Public MSBuildLocator host discovery API is unavailable");
        if (query.Invoke(null, null) is not IEnumerable candidates)
            throw new InvalidOperationException("MSBuildLocator returned no host candidates");
        var candidatesWithVersion = new List<(Version Version, string Path)>();
        foreach (var candidate in candidates)
        {
            if (candidate is null) continue;
            var type = candidate.GetType();
            var path = type.GetProperty("MSBuildPath")?.GetValue(candidate) as string;
            var version = type.GetProperty("Version")?.GetValue(candidate) as Version;
            if (!string.IsNullOrWhiteSpace(path) && version is not null)
                candidatesWithVersion.Add((version, path));
        }
        return candidatesWithVersion.OrderByDescending(candidate => candidate.Version).Select(candidate => candidate.Path).FirstOrDefault()
            ?? throw new FileNotFoundException("MSBuildLocator found no usable host toolchain");
    }

    private static void PublishCapturedHost(Assembly workerAssembly, string hostToolsPath)
    {
        var toolchainType = workerAssembly.GetType("MiLsp.Worker.CapturedMsBuildToolchain", throwOnError: true)!;
        var prepare = toolchainType.GetMethod("PrepareHost", BindingFlags.NonPublic | BindingFlags.Static)
            ?? throw new MissingMethodException("Controlled host preparation API is unavailable");
        var publish = toolchainType.GetMethod("PublishRegisteredHost", BindingFlags.NonPublic | BindingFlags.Static)
            ?? throw new MissingMethodException("Controlled host publication API is unavailable");
        var prepared = prepare.Invoke(null, [hostToolsPath])
            ?? throw new InvalidOperationException("Controlled host preparation failed");
        publish.Invoke(null, [prepared]);
    }

    private static byte[] AttachReceipt(byte[] responseBytes, ManagedImageCaptureReceipt receipt, byte[] requestBytes)
    {
        JsonObject response;
        try { response = JsonNode.Parse(responseBytes) as JsonObject ?? new JsonObject(); }
        catch { response = new JsonObject(); }
        if (response["ok"]?.GetValue<bool>() != false || response["observation"] is not null)
        {
            response = JsonNode.Parse(FailureResponse("Controlled host did not produce an error-only receipt")) as JsonObject ?? new JsonObject();
        }

        var items = response["items"] as JsonArray;
        if (items is null)
        {
            items = [];
            response["items"] = items;
        }
        if (items.Count == 0) items.Add(new JsonObject());
        var acknowledgement = items[0] as JsonObject;
        if (acknowledgement is null)
        {
            acknowledgement = new JsonObject();
            items[0] = acknowledgement;
        }
        var control = acknowledgement["control_inputs"] as JsonObject;
        if (control is null)
        {
            control = new JsonObject { ["schema_version"] = 1, ["complete"] = false, ["status"] = "explicit_incomplete" };
            acknowledgement["control_inputs"] = control;
        }
        control["managed_image_capture"] = ReceiptNode(receipt, control, requestBytes);
        control["complete"] = false;
        if (string.IsNullOrWhiteSpace(control["status"]?.GetValue<string>())) control["status"] = "explicit_incomplete";
        response["observation"] = null;
        response["ok"] = false;
        return JsonSerializer.SerializeToUtf8Bytes(response, JsonOptions);
    }

    private static JsonObject ReceiptNode(ManagedImageCaptureReceipt receipt, JsonObject control, byte[] requestBytes)
    {
        var images = new JsonArray();
        foreach (var image in receipt.Images)
        {
            images.Add(new JsonObject
            {
                ["requested_identity"] = image.RequestedIdentity,
                ["assembly_identity"] = image.AssemblyIdentity,
                ["sha256"] = image.Sha256,
                ["length"] = image.Length,
                ["source"] = image.Source,
                ["path_alias"] = image.PathAlias,
                ["loader"] = "AssemblyLoadContext.LoadFromStream"
            });
        }
        var unmediated = new JsonArray(receipt.UnmediatedLoads.Select(value => (JsonNode?)JsonValue.Create(value)).ToArray());
        var untracked = new JsonArray(receipt.UntrackedLoads.Select(value => (JsonNode?)JsonValue.Create(value)).ToArray());
        var binding = new JsonObject
        {
            ["request_sha256"] = Convert.ToHexString(SHA256.HashData(requestBytes)).ToLowerInvariant(),
            ["evaluation_id"] = control["evaluation_id"]?.DeepClone(),
            ["evaluation_snapshot_id"] = control["evaluation_snapshot_id"]?.DeepClone(),
            ["compiler_snapshot_id"] = control["compiler_snapshot_id"]?.DeepClone(),
            ["reference_snapshot_id"] = control["reference_snapshot_id"]?.DeepClone()
        };
        return new JsonObject
        {
            ["schema_version"] = 1,
            ["status"] = receipt.Status,
            ["managed_images_complete"] = receipt.Complete,
            ["registry_id"] = receipt.RegistryId,
            ["binding"] = binding,
            ["core_host_images_captured"] = receipt.CoreHostImagesCaptured,
            ["image_count"] = receipt.Images.Count,
            ["images"] = images,
            ["unmediated_loads"] = unmediated,
            ["untracked_loads"] = untracked,
            ["baseline_trusted"] = receipt.BaselineTrusted,
            ["baseline_assemblies_checked"] = receipt.BaselineAssembliesChecked,
            ["unknown_baseline_components"] = new JsonArray(receipt.UnknownBaselineComponents.Select(value => (JsonNode?)JsonValue.Create(value)).ToArray()),
            ["tracked_load_events"] = receipt.TrackedLoadEvents,
            ["foreign_load_event_count"] = receipt.ForeignLoadEventCount,
            ["bookkeeping_overflow"] = receipt.BookkeepingOverflow,
            ["runtime_bcl_assumption"] = receipt.RuntimeBclAssumption,
            ["control_inputs_complete"] = false
        };
    }


    private static byte[] FailureResponse(string message)
        => JsonSerializer.SerializeToUtf8Bytes(new WorkerResponse(false, "roslyn", Error: message,
            ErrorCode: "GPH_CONTROL_RECEIPT_UNAVAILABLE"), JsonOptions);

    private static async Task<byte[]?> ReadFrameAsync(Stream stream, int maximumBytes, CancellationToken cancellationToken)
    {
        var header = new byte[4];
        var first = await stream.ReadAsync(header.AsMemory(0, 1), cancellationToken).ConfigureAwait(false);
        if (first == 0) return null;
        await ReadExactlyAsync(stream, header.AsMemory(1), cancellationToken).ConfigureAwait(false);
        var length = BinaryPrimitives.ReadUInt32BigEndian(header);
        if (length == 0 || length > maximumBytes) throw new IOException("Controlled host frame exceeds its byte bound");
        var payload = new byte[(int)length];
        await ReadExactlyAsync(stream, payload, cancellationToken).ConfigureAwait(false);
        return payload;
    }

    private static async Task WriteFrameAsync(Stream stream, byte[] payload, int maximumBytes, CancellationToken cancellationToken)
    {
        if (payload.Length == 0 || payload.Length > maximumBytes) throw new IOException("Controlled host response exceeds its byte bound");
        var header = new byte[4];
        BinaryPrimitives.WriteUInt32BigEndian(header, (uint)payload.Length);
        await stream.WriteAsync(header, cancellationToken).ConfigureAwait(false);
        await stream.WriteAsync(payload, cancellationToken).ConfigureAwait(false);
    }

    private static async Task ReadExactlyAsync(Stream stream, Memory<byte> buffer, CancellationToken cancellationToken)
    {
        var read = 0;
        while (read < buffer.Length)
        {
            var count = await stream.ReadAsync(buffer[read..], cancellationToken).ConfigureAwait(false);
            if (count == 0) throw new EndOfStreamException("Controlled host frame ended early");
            read += count;
        }
    }
}
