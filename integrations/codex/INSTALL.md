# Instalación — Codex

Codex usa el CLI `mi-lsp` directo desde el shell. No hay servidor MCP: la puerta `mi-lsp mcp` fue retirada (decisión 2026-10-15).

Para que la telemetría local identifique a Codex, agregá el bloque de `integrations/codex/config.toml.snippet` en `~/.codex/config.toml`, o en `.codex/config.toml` del proyecto si el repo está marcado como confiable:

```toml
[shell_environment_policy]
set = { MI_LSP_CLIENT_NAME = "codex" }
```

## Binario

`mi-lsp` tiene que estar en el `PATH`. En Windows, si resuelve a un shim `.cmd` o `.bat`, usá la ruta del `.exe` real. `MI_LSP_BIN` solo aplica a los hooks de Claude Code que invocan `nav suggest`.

No hace falta red ni una clave.
