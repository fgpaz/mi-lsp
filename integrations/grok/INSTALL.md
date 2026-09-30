# Instalación — Grok

Grok tiene plugins (`grok plugin`). Este directorio es un plugin: manifiesto en `.grok-plugin/plugin.json` y servidor MCP en `.mcp.json` (`command`: `mi-lsp`, `args`: `["mcp"]`). No incluye los hooks de Claude Code. En Grok, la salida de un `UserPromptSubmit` que deja seguir el turno no se inyecta como contexto, así que el aviso de `nav suggest` vive solo en `integrations/claude-code`.

## Plugin

Desde la raíz del repo, o con la ruta absoluta:

```bash
grok plugin validate integrations/grok
grok plugin install integrations/grok --trust
```

`--trust` activa el MCP del plugin. Sin eso, Grok lo lista pero no lo conecta. El plugin no se instala solo por estar en el árbol.

## Snippet, si no querés el plugin

Pegá `integrations/grok/config.toml.snippet` en `~/.grok/config.toml` (todas las sesiones) o en `.grok/config.toml` del proyecto:

```toml
[mcp_servers.mi-lsp]
command = "mi-lsp"
args = ["mcp"]
enabled = true
```

O con la CLI, que escribe la config de usuario:

```bash
grok mcp add --scope user mi-lsp -- mi-lsp mcp
```

## Binario

`command` es el proceso que Grok lanza. En Windows, si `mi-lsp` del `PATH` es un shim `.cmd` o `.bat`, usá la ruta del `.exe` real. `MI_LSP_BIN` no sustituye ese `command`. Reservalo para los hooks de Claude Code, y que apunte al `.exe`, no al `.cmd`, porque esos hooks hacen `spawn` con argv y sin shell.

No hay un smoke en vivo de `mi-lsp mcp` en esta nota: el subcomando puede seguir sin implementar en el CLI. `grok plugin validate` solo revisa el manifiesto.
