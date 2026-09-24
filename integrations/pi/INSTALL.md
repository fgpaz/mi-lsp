# Instalación — Pi

Registro del servidor MCP stdio nativo. No modifica el repo `mi-pi` y no porta hooks de Jev. El comando es `mi-lsp` y el argumento es `mcp`.

## Registro

Pi no trae un bloque MCP propio en este repo. Registrá el servidor en el puente MCP que ya uses (por ejemplo la config stdio de la extensión MCP de Pi) con este proceso:

| Campo | Valor |
| --- | --- |
| nombre | `mi-lsp` |
| transporte | stdio |
| command | `mi-lsp` |
| args | `["mcp"]` |

El mismo objeto está en `integrations/pi/mcp.json`, con la forma `mcpServers` que aceptan varios puentes:

```json
{
  "mcpServers": {
    "mi-lsp": {
      "command": "mi-lsp",
      "args": ["mcp"]
    }
  }
}
```

Ejemplo para un puente que pide el transporte aparte:

```json
{
  "name": "mi-lsp",
  "transport": {
    "type": "stdio",
    "command": "mi-lsp",
    "args": ["mcp"]
  }
}
```

## Binario

En Windows, si `mi-lsp` en el `PATH` es un shim `.cmd`, apuntá `command` al `.exe` real. `MI_LSP_BIN` no lo reescribe este registro: el host lanza el comando de arriba tal cual.

Si el binario no está o `mcp` todavía no responde, el turno de Pi no depende de este archivo. No hay hook que bloquee la sesión.
