# Instalación — Claude Code

Este directorio es un plugin de Claude Code. El servidor MCP nativo arranca con el comando `mi-lsp` y los argumentos `["mcp"]` (`.mcp.json`). Los hooks `UserPromptSubmit` y `PostToolUse` son solo advisory y fallan abiertos: un timeout, un binario ausente, JSON inválido o una excepción terminan con código 0 y `{"continue": true}`. No bloquean el turno ni la tool que ya corrió.

## Binario

Los hooks resuelven el ejecutable así:

1. Si `MI_LSP_BIN` está definido, se usa ese valor.
2. Si no, se usa `mi-lsp` en el `PATH`.

`MI_LSP_BIN` tiene que apuntar al `.exe` real, por ejemplo `C:\tools\mi-lsp.exe`. No lo apuntes a un shim `.cmd` o `.bat`. El hook hace `spawn` con un arreglo de argumentos y `shell: false`, y Windows no puede ejecutar un `.cmd` de esa forma. Si el binario no existe, el hook sigue sin sugerencia.

Definí `MI_LSP_BIN` en el entorno que lanza Claude Code. No hace falta repetirlo dentro de `.mcp.json`: ahí el host ya ejecuta `mi-lsp` con `mcp`.

## Qué hacen los hooks

Ambos llaman `mi-lsp nav suggest` por argv, nunca por un shell.

- `UserPromptSubmit` inyecta como máximo una línea corta, y solo cuando `nav suggest` imprime un comando `mi-lsp nav …` o la pista estática `mi-lsp nav intent "<goal>"`. Si no hay nada usable, no agrega contexto.
- `PostToolUse` cuenta llamadas consecutivas a `Read`, `Grep` y `Glob` (otras tools no reinician la racha). A la tercera emite un único aviso que contiene `mi-lsp nav suggest` y después no lo repite. Una tool cuyo nombre incluye `mi-lsp` (también `milsp` / `mi_lsp`) o un `Bash`/`PowerShell` que invoca el CLI `mi-lsp` reinicia el contador.

Contrato que se le pasa al CLI:

```text
mi-lsp nav suggest --event user_prompt [--prompt TEXT]
mi-lsp nav suggest --event post_tool --tool NAME --consecutive N
```

La salida útil es una sola línea `mi-lsp nav …`, o JSON con `command`, `hint` o `suggested_command`. Cualquier otro resultado se ignora.

## Marketplace

El catálogo del repo está en `.claude-plugin/marketplace.json` y apunta a `./integrations/claude-code`. Desde Claude Code, agregá este repositorio como marketplace local e instalá el plugin `mi-lsp`.

## Pruebas

```bash
node --test integrations/claude-code/tests/*.test.mjs
```

No usan red. El binario se simula con `tests/fixtures/fake-mi-lsp.mjs`.
