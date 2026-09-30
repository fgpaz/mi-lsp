# Fiabilidad para harness por defecto — 2026-09-24

Rama `feat/reliability-default`. No hubo push, tag, release ni reinicio del daemon.

## client_name de mi-pi

El alias `mi-pi` resuelve al clon `C:\Users\fgpaz\.pi\agent\git\github.com\fgpaz\mi-pi`. Cuando el selector fue la ruta `C:\repos\mios\mi-pi`, el daemon igual usó ese clon.

Esas filas no llegaron como `claude-code` ni `codex`. Llegaron así:

| client_name | éxitos contra el clon | fallas |
|---|---|---|
| root | 60 | 7 |
| builtin_child | 15 | 1 |
| pi-root | 10 | 0 |
| mi-pi-control-plane | 4 | 0 |

Las fallas de `mi-pi` en toda la tabla, no solo el mismatch, fueron `root` (nav.intent 6, nav.search 3, nav.find 1, nav.flow-slice 1), `builtin_child` (1) y `manual-cli` (1, nav.search). `manual-cli` sigue siendo el CLI humano y no entra en el rechazo.

La normalización cubre `root`, `builtin_child`, `mi-lsp-mcp`, el prefijo de familia (`pi-chief`, `grok-measure-leaf`, `claude-code`, `codex`, `grok`, `pi`, `cursor`) y el sufijo de rol (`-control-plane`).

## Humo

Daemon compartido, sin reinicio: pid 43908, `v0.9.0`, arranque 2026-09-24 21:12, ejecutable `C:\Users\fgpaz\bin\mi-lsp.exe`. El rechazo nuevo no está en ese proceso. El humo de rechazo usó `bin/mi-lsp.exe --no-daemon`.

CLI, cwd `C:\repos\mios\mi-pi`, `workspace status --workspace mi-pi`:

| client_name | resultado |
|---|---|
| claude-code, codex, grok, pi, cursor | exit 1, `cross-workspace refused` |
| root, builtin_child, pi-chief, grok-measure-leaf, mi-lsp-mcp | exit 1, `cross-workspace refused` |
| manual-cli | no se ejecutó en vivo: el binario viejo del daemon seguiría y el test de unidad lo deja como warning |

MCP contra el daemon 0.9.0, `initialize` + `tools/list` + `tools/call nav_overview` workspace `mi-lsp`:

| MI_LSP_CLIENT_NAME | resultado |
|---|---|
| vacío (puerta `mi-lsp-mcp`) | exit 0, 13 herramientas, call respondió, stderr vacío |
| claude-code, codex, grok, pi, cursor | igual, exit 0 |

## Verificación

`go test ./...` pasó salvo `internal/telemetry`, que esperaba `explicit_incomplete` fuera de la lista cerrada de códigos. Se agregó ese código a la lista y se reejecutó `go test` de telemetry, daemon, workspace y service: exit 0. No hubo segunda pasada del resto.

## Daemon

No se reinició. Hasta que el owner coordine el reinicio, una sesión que hable con el daemon 0.9.0 sigue con el warning de mismatch para `root`. El rechazo nuevo ya corre en el binario de esta rama con `--no-daemon` y en la puerta MCP solo para lo que el proceso nuevo decide antes de delegar; la resolución dentro del daemon viejo no cambia.
