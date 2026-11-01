using System.Buffers.Binary;
using System.Diagnostics;
using System.Reflection;
using System.Text.Json;

namespace MiLsp.Worker;

/// <summary>Runs the opt-in route in a clean process so the caller's default ALC cannot contaminate it.</summary>
internal static class ControlledHostClient
{
    private const int MaximumRequestBytes = 16 * 1024 * 1024;
    private const int MaximumResponseBytes = 64 * 1024 * 1024;
    private static readonly JsonSerializerOptions JsonOptions = new(JsonSerializerDefaults.Web);

    internal static async Task<WorkerResponse> ExecuteAsync(WorkerRequest request, CancellationToken cancellationToken)
    {
        if (ControlledHostBootstrap.IsControlledHostProcess)
            return Unavailable("Nested controlled-host process creation is forbidden");
        if (!string.Equals(request.Method, "graph_observe", StringComparison.Ordinal) ||
            request.Payload is null || !request.Payload.TryGetValue("control_inputs", out var control) || control.ValueKind != JsonValueKind.True)
            return Unavailable("Controlled host route requires the explicit graph_observe control_inputs opt-in");

        var requestBytes = JsonSerializer.SerializeToUtf8Bytes(request, JsonOptions);
        if (requestBytes.Length == 0 || requestBytes.Length > MaximumRequestBytes)
            return Unavailable("Controlled host request exceeds its frame bound");

        Process? process = null;
        try
        {
            var executable = Environment.ProcessPath;
            if (string.IsNullOrWhiteSpace(executable)) return Unavailable("Controlled host executable path is unavailable");
            var entryPath = Assembly.GetEntryAssembly()?.Location;
            var startInfo = new ProcessStartInfo
            {
                FileName = executable,
                UseShellExecute = false,
                RedirectStandardInput = true,
                RedirectStandardOutput = true,
                RedirectStandardError = true,
                CreateNoWindow = true
            };
            if (string.Equals(Path.GetFileNameWithoutExtension(executable), "dotnet", StringComparison.OrdinalIgnoreCase))
            {
                if (string.IsNullOrWhiteSpace(entryPath) || !File.Exists(entryPath))
                    return Unavailable("Controlled host worker entry assembly is unavailable");
                startInfo.ArgumentList.Add(entryPath);
            }
            startInfo.ArgumentList.Add(ControlledHostBootstrap.Argument);

            process = new Process { StartInfo = startInfo, EnableRaisingEvents = true };
            if (!process.Start()) return Unavailable("Controlled host process could not be started");
            var stderrDrain = DrainAsync(process.StandardError);
            await WriteFrameAsync(process.StandardInput.BaseStream, requestBytes, cancellationToken).ConfigureAwait(false);
            process.StandardInput.Close();
            var responseBytes = await ReadFrameAsync(process.StandardOutput.BaseStream, MaximumResponseBytes, cancellationToken).ConfigureAwait(false);
            await process.WaitForExitAsync(cancellationToken).ConfigureAwait(false);
            await stderrDrain.ConfigureAwait(false);
            if (process.ExitCode != 0) return Unavailable("Controlled host process exited without a usable response");
            var response = JsonSerializer.Deserialize<WorkerResponse>(responseBytes, JsonOptions);
            if (response is null || response.Ok || response.Observation is not null)
                return Unavailable("Controlled host returned an invalid or stageable response");
            return response;
        }
        catch (OperationCanceledException)
        {
            StopOwnedProcess(process);
            throw;
        }
        catch
        {
            StopOwnedProcess(process);
            return Unavailable("Controlled host capture or protocol failed");
        }
        finally
        {
            process?.Dispose();
        }
    }

    private static WorkerResponse Unavailable(string message)
        => new(false, "roslyn", Error: message, ErrorCode: "GPH_CONTROL_RECEIPT_UNAVAILABLE");

    private static void StopOwnedProcess(Process? process)
    {
        try
        {
            if (process is not null && !process.HasExited) process.Kill(entireProcessTree: true);
        }
        catch { }
    }

    private static async Task DrainAsync(StreamReader reader)
    {
        var buffer = new char[2048];
        while (await reader.ReadAsync(buffer.AsMemory()).ConfigureAwait(false) != 0) { }
    }

    private static async Task WriteFrameAsync(Stream stream, byte[] payload, CancellationToken cancellationToken)
    {
        var header = new byte[4];
        BinaryPrimitives.WriteUInt32BigEndian(header, (uint)payload.Length);
        await stream.WriteAsync(header, cancellationToken).ConfigureAwait(false);
        await stream.WriteAsync(payload, cancellationToken).ConfigureAwait(false);
        await stream.FlushAsync(cancellationToken).ConfigureAwait(false);
    }

    private static async Task<byte[]> ReadFrameAsync(Stream stream, int maximumBytes, CancellationToken cancellationToken)
    {
        var header = new byte[4];
        await ReadExactlyAsync(stream, header, cancellationToken).ConfigureAwait(false);
        var length = BinaryPrimitives.ReadUInt32BigEndian(header);
        if (length == 0 || length > maximumBytes) throw new IOException("Controlled host response exceeds its frame bound");
        var payload = new byte[(int)length];
        await ReadExactlyAsync(stream, payload, cancellationToken).ConfigureAwait(false);
        return payload;
    }

    private static async Task ReadExactlyAsync(Stream stream, Memory<byte> buffer, CancellationToken cancellationToken)
    {
        var read = 0;
        while (read < buffer.Length)
        {
            var count = await stream.ReadAsync(buffer[read..], cancellationToken).ConfigureAwait(false);
            if (count == 0) throw new EndOfStreamException("Controlled host response frame ended early");
            read += count;
        }
    }
}
