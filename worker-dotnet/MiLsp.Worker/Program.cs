using System.Buffers.Binary;
using System.Text.Json;

namespace MiLsp.Worker;

internal static class Program
{
    private static readonly JsonSerializerOptions JsonOptions = new(JsonSerializerDefaults.Web)
    {
        WriteIndented = false
    };

    public static async Task<int> Main()
    {
        var service = new RoslynService();
        await using var input = Console.OpenStandardInput();
        await using var output = Console.OpenStandardOutput();

        while (true)
        {
            WorkerRequest? request;
            try
            {
                request = await ReadRequestAsync(input, CancellationToken.None);
            }
            catch (EndOfStreamException)
            {
                return 0;
            }

            if (request is null)
            {
                return 0;
            }

            var response = await service.HandleAsync(request, CancellationToken.None);
            await WriteResponseAsync(output, response, CancellationToken.None);
        }
    }

    private static async Task<WorkerRequest?> ReadRequestAsync(Stream input, CancellationToken cancellationToken)
    {
        var header = new byte[4];
        var bytesRead = await input.ReadAsync(header.AsMemory(0, 4), cancellationToken);
        if (bytesRead == 0)
        {
            throw new EndOfStreamException();
        }
        if (bytesRead < 4)
        {
            throw new InvalidDataException("Invalid frame header length");
        }

        var length = BinaryPrimitives.ReadUInt32BigEndian(header);
        var payload = new byte[length];
        var offset = 0;
        while (offset < payload.Length)
        {
            var read = await input.ReadAsync(payload.AsMemory(offset, payload.Length - offset), cancellationToken);
            if (read == 0)
            {
                throw new EndOfStreamException();
            }
            offset += read;
        }

        return JsonSerializer.Deserialize<WorkerRequest>(payload, JsonOptions);
    }

    private static async Task WriteResponseAsync(Stream output, WorkerResponse response, CancellationToken cancellationToken)
    {
        var payload = JsonSerializer.SerializeToUtf8Bytes(response, JsonOptions);
        var header = new byte[4];
        BinaryPrimitives.WriteUInt32BigEndian(header, (uint)payload.Length);
        await output.WriteAsync(header.AsMemory(0, 4), cancellationToken);
        await output.WriteAsync(payload.AsMemory(0, payload.Length), cancellationToken);
        await output.FlushAsync(cancellationToken);
    }
}
