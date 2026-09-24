# Instalación — Cursor

Fusioná `integrations/cursor/mcp.json` en la config MCP de Cursor: `~/.cursor/mcp.json` para el usuario, o `.cursor/mcp.json` en el proyecto.

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

El servidor es el CLI nativo: comando `mi-lsp`, argumento `mcp`, transporte stdio.

## Binario

En Windows, si `mi-lsp` en el `PATH` abre un `.cmd` y Cursor no lo ejecuta, poné en `command` la ruta del `.exe` real (`C:\\tools\\mi-lsp.exe`). `MI_LSP_BIN` no reescribe este JSON. Tiene que apuntar al `.exe` real cuando lo uses desde los hooks de Claude Code, que hacen `spawn` sin shell.

Si el binario falta, Cursor marca el servidor como caído y el chat sigue sin esa herramienta.
