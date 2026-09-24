# Instalación — Codex

Agregá el servidor MCP stdio `mi-lsp` en `~/.codex/config.toml`, o en `.codex/config.toml` del proyecto si el repo está marcado como confiable. El bloque es `[mcp_servers.mi-lsp]`.

```toml
[mcp_servers.mi-lsp]
command = "mi-lsp"
args = ["mcp"]
```

El mismo texto está en `integrations/codex/config.toml.snippet`.

## Binario

`command` es el ejecutable que Codex lanza. En Windows, si el nombre `mi-lsp` del `PATH` es un shim `.cmd` o `.bat`, reemplazalo por la ruta del `.exe` real, por ejemplo `C:\\tools\\mi-lsp.exe`. `MI_LSP_BIN` no cambia este `command`: tiene que apuntar al `.exe` solo cuando un hook de Claude Code invoca `nav suggest`.

También podés registrarlo con la CLI:

```bash
codex mcp add mi-lsp -- mi-lsp mcp
```

No hace falta red ni una clave. Si el binario no está, Codex no carga el servidor; el resto de la sesión sigue.
