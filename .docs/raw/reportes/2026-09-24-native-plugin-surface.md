# Verificación 2026-09-24 — superficie nativa de plugins

Rama `feat/native-plugin-surface`, base `3a52775`. No hubo push, tag ni release. No se editaron `C:\repos\mios\mis-plugins-cc` ni `C:\repos\mios\mi-pi`. El daemon ya en marcha no se reinició ni se detuvo.

No hubo rotura de costura en la primera ola (`go test`, `go build`, `go vet`). No hubo segunda pasada ni trabajo de producto nuevo, salvo completar la celda de RSS que `TECH-MCP-ADAPTER` dejaba pendiente de esta ola.

## Rutas cambiadas

Modificadas, ya versionadas:

- `.docs/wiki/01_alcance_funcional.md`
- `.docs/wiki/02_arquitectura.md`
- `.docs/wiki/04_RF/RF-QRY-001.md`
- `.docs/wiki/07_baseline_tecnica.md`
- `.docs/wiki/09_contratos/CT-CLI-DAEMON-ADMIN.md`
- `.docs/wiki/09_contratos_tecnicos.md`
- `.docs/wiki/ae/AE-RELEASE-DISTRIBUTION.md`
- `CHANGELOG.md`
- `README.md`
- `cmd/mi-lsp/main.go`
- `internal/cli/nav.go`
- `internal/cli/root.go`
- `internal/cli/workspace.go`
- `internal/model/types.go`
- `internal/output/truncator.go`
- `internal/output/truncator_test.go`
- `skills/mi-lsp/SKILL.md`
- `skills/mi-lsp/agents/openai.yaml`
- `skills/mi-lsp/references/compound-commands.md`
- `skills/mi-lsp/references/quickstart.md`
- `skills/mi-lsp/references/recipes.md`
- `skills/mi-lsp/references/runtime-drift.md`

Nuevas, parte del cambio:

- `.claude-plugin/marketplace.json`
- `.docs/wiki/07_tech/TECH-MCP-ADAPTER.md`
- `.docs/raw/reportes/2026-09-24-native-plugin-surface.md` (este informe; la ruta está en `.gitignore` y entra con `git add -f`)
- `integrations/claude-code/` (plugin, hooks, `.mcp.json`, pruebas Node)
- `integrations/pi/mcp.json`, `integrations/pi/INSTALL.md`
- `integrations/codex/config.toml.snippet`, `integrations/codex/INSTALL.md`
- `integrations/cursor/mcp.json`, `integrations/cursor/INSTALL.md`
- `integrations/grok/` (plugin validado, `.mcp.json`, snippet, `INSTALL.md`)
- `internal/cli/mcp.go`, `internal/cli/mcp_test.go`
- `internal/cli/reason.go`, `internal/cli/reason_test.go`
- `internal/cli/suggest.go`, `internal/cli/suggest_test.go`
- `internal/cli/workspace_which.go`, `internal/cli/workspace_which_test.go`
- `internal/mcp/server.go`, `internal/mcp/tools.go`, `internal/mcp/child_windows.go`, `internal/mcp/child_other.go`

En disco y fuera del commit:

- `docs/policies/mi-lsp-host-doors.md`, `docs/policies/mi-lsp-host-troubleshooting.md`, `docs/policies/mi-lsp-project-snippets.md` y `docs/skills/claude-code-lsp.md`: existen, pero `/docs/policies/` y `/docs/skills/` están en `.gitignore`.
- `.grok/workflows/native-plugin-surface.rhai`: workflow de la sesión, no es superficie de producto.
- `.pi-subagents/`: excluido.
- `bin/mi-lsp.exe`: binario de esta ola. `bin/` y `*.exe` no se versionan.

## Pruebas

Una sola ola. El objetivo `-race` del Makefile no se ejecutó. La ola de Windows fue `go test ./...` sin `-race`.

| Comando | Resultado |
|---|---|
| `go test ./...` | exit 0 |
| `go build -o bin/mi-lsp.exe ./cmd/mi-lsp` | exit 0; 26893824 bytes; 2026-09-24 18:10:01; go1.24.4 windows/arm64 |
| `go vet ./...` | exit 0 |
| `node --test integrations/claude-code/tests/*.test.mjs` | 14 pass, 0 fail; Node v24.14.0 |
| `gofmt -l` | no reescribió nada: no hubo pasada de arreglo |

`go test ./...` dejó en ok: `internal/cli` 3.205s, `internal/daemon` 9.781s, `internal/docgraph` 1.199s, `internal/docidentity` 0.035s, `internal/embed` 0.083s, `internal/indexer` 63.566s, `internal/language` 0.081s, `internal/livecontext` 5.539s, `internal/milx` 4.399s, `internal/model` 0.112s, `internal/nav` 1.424s, `internal/output` 0.133s, `internal/processutil` 0.569s, `internal/reentry` 0.632s, `internal/rerank` 0.401s, `internal/service` 152.227s, `internal/skills` 1.422s, `internal/store` 24.718s, `internal/telemetry` 0.567s, `internal/wikichunk` 0.073s, `internal/wikisource` 0.034s, `internal/worker` 9.062s, `internal/workspace` 21.796s. Sin tests: `cmd/mi-lsp`, `internal/mcp` (la cobertura MCP está en `internal/cli`) y dos paquetes de corpus del victory-lab. No hubo línea `FAIL`.

`gofmt -l` informativo sobre `cmd/mi-lsp`, `internal/cli`, `internal/mcp`, `internal/model` e `internal/output` listó muchos archivos que esta ola no tocó, y también `cmd/mi-lsp/main.go`, `internal/cli/root.go` e `internal/model/types.go`. Los Go nuevos (`internal/mcp/*`, `mcp`, `reason`, `suggest`, `workspace_which` y `truncator`) no salieron en `gofmt -l`. No se reformateó el árbol.

El binario medido se identificó como `v0.8.3-0.20260910032715-3a52775a83d9+dirty`, `protocol=mi-lsp-v1.1`, `rid=win-arm64`, `modified=true`.

## Latencia

Comando en ambas olas: `nav overview --workspace mi-lsp --format toon`. El alias `mi-lsp` respondió; no hizo falta `--workspace .`. Método: nearest-rank `ceil(p*n)`, rango 1-based, solo sobre las 20 llamadas cronometradas. La sonda y los 3 calentamientos quedan fuera del percentil.

Antes, carril de baseline, no re-medido aquí. Binario `C:\repos\mios\mi-lsp\mi-lsp.exe` contra el daemon ya caliente. Tres calentamientos, 20 llamadas, todas exit 0, más una sonda extra no cronometrada en el percentil. p50 4677.136 ms. p95 6102.285 ms.

Después, esta ola, `C:\repos\mios\mi-lsp\bin\mi-lsp.exe`. Mismo daemon, no reiniciado: pid 1424, versión `v0.8.2`, arranque 2026-09-18T12:16:01-03:00, ejecutable `%USERPROFILE%\bin\mi-lsp.exe`. `daemon status` avisó ejecutable viejo (hash del daemon `41e1ffbf391a`, hash de este CLI `1a7d59deef08`) y sugirió `mi-lsp daemon restart`. No se reinició.

Sonda: 363.855 ms, exit 0. Calentamientos: 330.484, 387.387, 318.883 ms, todos exit 0. Veinte llamadas, todas exit 0, stdout 3380 bytes, `backend: catalog`, 28 ítems del corpus victory-lab:

234.495, 385.836, 373.564, 217.348, 226.016, 213.652, 251.970, 247.563, 226.510, 238.428, 199.928, 224.676, 253.618, 223.983, 232.484, 212.527, 264.142, 259.926, 240.056, 214.104.

Ordenadas, rango 10 = p50 232.484 ms. Rango 19 = p95 373.564 ms. Mínimo 199.928 ms. Máximo 385.836 ms.

La caída frente al baseline no se atribuye a un cambio del daemon: el proceso es el mismo y no se reinició. El cliente medido tampoco es el mismo binario. El baseline no guardó el tamaño de la respuesta, así que no se afirma que el payload sea el mismo.

## Working set MCP

Se lanzó `bin\mi-lsp.exe mcp`. Por stdin, JSON-RPC 2.0 delimitado por líneas: `initialize` (id 1, protocolVersion `2024-11-05`), `notifications/initialized` y `tools/list` (id 2). No hubo `tools/call`. stderr vacío. El proceso salió con código 0 al cerrar stdin.

Respuesta de `initialize`: `serverInfo.name=mi-lsp`, `version=mi-lsp-v1.1`, `protocolVersion=2024-11-05`. `tools/list` devolvió 13 herramientas: `nav_intent`, `nav_route`, `nav_pack`, `nav_wiki`, `nav_search`, `nav_find`, `nav_refs`, `nav_related`, `nav_flow_slice`, `nav_change_pack`, `nav_affected`, `nav_multi_read`, `nav_overview`.

Tras esas respuestas, `WorkingSet64` = 14340096 bytes (13,676 MiB). `PeakWorkingSet64` = 14569472 bytes (13,895 MiB). `PrivateMemorySize64` = 16367616 bytes (15,609 MiB). El objetivo de 20 MB no se supera en esta muestra: 14340096 está por debajo de 20000000 y de 20971520. La celda de `TECH-MCP-ADAPTER` queda en 14340096 bytes (13,676 MiB). La construcción de `service.App` en la raíz sigue ocurriendo antes de `mcp`; en esta muestra el working set igual quedó bajo el objetivo. No se midió el pico de un `tools/call` (tope de 4 MB de stdout y 4 MB de stderr en el hijo).

## Integraciones

| Host | CLI en esta máquina | Qué se ejecutó | Sesión viva |
|---|---|---|---|
| Claude Code | sí, `%USERPROFILE%\.local\bin\claude.exe` | no hay subcomando `plugin validate`. No se instaló el marketplace ni se arrancó sesión. Pruebas Node de hooks: 14 pass | no |
| Grok | sí, `%USERPROFILE%\.grok\bin\grok.exe` | `grok plugin validate integrations/grok`, exit 0: manifiesto válido, nombre `mi-lsp`, versión 0.1.0, componente MCP. No se instaló el plugin | no |
| Pi | sí, `pi.ps1` de npm | el help no ofrece un subcomando MCP. No se arrancó sesión ni se editó `mi-pi` | no; solo docs |
| Codex | sí, `codex.exe` | no se ejecutó `codex mcp add` (escribiría la config de usuario). No se arrancó sesión | no; solo docs |
| Cursor | sí, Cursor 3.21.13, launcher de IDE | no se abrió ventana ni se fusionó `mcp.json` | no; solo docs |

## Riesgos abiertos

- El daemon pid 1424 sigue en `v0.8.2` y su ejecutable no es este CLI. Las consultas salieron bien, con aviso de ejecutable viejo. No se reinició.
- El p50/p95 de después (232.484 / 373.564 ms) no es comparable de forma estricta con el baseline (4677.136 / 6102.285 ms): otro binario cliente, mismo daemon, payload del baseline no archivado.
- El working set cumple el objetivo en `initialize` + `tools/list`. Un `tools/call` puede bufferar hasta 4 MB de stdout y 4 MB de stderr, y el padre espera el contexto del request sin un timeout más corto que el de la CLI (2–5 min). Eso no se midió.
- `NewRootCommand` sigue construyendo `service.App` antes de servir MCP. En esta muestra no llevó el working set por encima de 20 MB. `internal/mcp` no tiene tests propios; `DefaultExec` no se dispara en los tests del CLI (usan recorder e inspección de `NewCommand`).
- `docs/policies/` y `docs/skills/` describen puertas de host y quedan fuera de git por `.gitignore`. La cobertura versionada está en el wiki, README, CHANGELOG, `skills/mi-lsp` e `integrations/`.
- Los hooks de Claude Code asumen que `nav suggest` imprime una línea corta `mi-lsp nav …` (o la pista estática de `nav intent`) y aceptan argv `--event` / `--prompt` / `--tool` / `--consecutive`. Otro contrato queda en silencio o fail-open. Las tools que no son mi-lsp no reinician la racha Read/Grep/Glob. En Windows `MI_LSP_BIN` tiene que ser el `.exe` real.
- `nav suggest`: JSON `--args` inválido sale 1. Grep solo recibe `--regex` cuando el JSON marca el patrón como regex. Glob solo acepta un identificador, así que globs de path y nombres como `Foo.Bar` no sugieren. Offset sin limit sugiere un rango de una línea. No se afirma una sesión viva de host.
- `gofmt -l` no está limpio en varios archivos previos del paquete CLI, `main.go`, `root.go` y `types.go`. No se reformateó.
- La ola no usó `-race`.
- Este informe vive bajo `.docs/raw/`, ignorado por defecto. Entra en el commit solo por `git add -f` de este archivo.

## Resúmenes de carriles

[{"files":["C:\\repos\\mios\\mi-lsp\\internal\\cli\\mcp.go","C:\\repos\\mios\\mi-lsp\\internal\\cli\\mcp_test.go","C:\\repos\\mios\\mi-lsp\\internal\\mcp\\server.go","C:\\repos\\mios\\mi-lsp\\internal\\mcp\\tools.go","C:\\repos\\mios\\mi-lsp\\internal\\mcp\\child_windows.go","C:\\repos\\mios\\mi-lsp\\internal\\mcp\\child_other.go"],"ok":true,"p50_ms":"unmeasured","p95_ms":"unmeasured","risks":"Idle 20 MB RSS is not proven without a measurement. The loop is only a codec (no index, daemon, or session), but the process is the full mi-lsp binary: root construction allocates service.App before mcp runs (root.go is outside this lane) and store/sqlite stay linked. A tool call can transiently buffer up to 4 MB of stdout and 4 MB of stderr. The parent waits on the request context with no shorter timeout, so one hung child blocks the loop until that child exits (the CLI's own nav timeouts are 2–5 min). Tests use a recorder and NewCommand inspection; DefaultExec is not spawned. Latency was not measured.","summary":"Replaced the mi-lsp mcp stub with a stdio JSON-RPC 2.0 server (initialize, notifications/initialized, tools/list, tools/call, ping) that exposes the 13 nav_* tools and execs this binary, or MI_LSP_BIN, with a []string argv. No shell and no nav reimplementation."},{"files":["C:\\repos\\mios\\mi-lsp\\internal\\cli\\reason.go","C:\\repos\\mios\\mi-lsp\\internal\\cli\\reason_test.go","C:\\repos\\mios\\mi-lsp\\internal\\output\\truncator.go","C:\\repos\\mios\\mi-lsp\\internal\\output\\truncator_test.go"],"ok":true,"p50_ms":"","p95_ms":"","risks":"Tests were not run (go test/build forbidden). Marker N is len(rendered)-maxChars in bytes. Over-budget text with no continuation is a hard prefix and has no marker. Detail is the sanitized diagnostic, not a fixed canonical sentence, unless the message is empty.","summary":"Failure envelopes and process-failure stderr now emit one of the four fallback reason codes plus a bounded detail, and CapRendered keeps text or JSON continuation.next when --max-chars cuts the render."},{"files":["C:\\repos\\mios\\mi-lsp\\internal\\cli\\workspace.go","C:\\repos\\mios\\mi-lsp\\internal\\cli\\workspace_which.go","C:\\repos\\mios\\mi-lsp\\internal\\cli\\workspace_which_test.go"],"ok":true,"p50_ms":"","p95_ms":"","risks":"No existing Go decode of a raw Windows workspace path used strconv.Unquote or fmt JSON assembly; daemon workspace_input is the selector already received. Slash-normalized match keys stay internal and are not the stored root or which output. go test was not run.","summary":"Added registry-only workspace which and a regression that keeps C:\\repos\\mios\\mis-plugins-cc unchanged in JSON output."},{"files":["C:\\repos\\mios\\mi-lsp\\internal\\cli\\suggest.go","C:\\repos\\mios\\mi-lsp\\internal\\cli\\suggest_test.go","C:\\repos\\mios\\mi-lsp\\internal\\cli\\nav.go"],"ok":true,"p50_ms":"","p95_ms":"","risks":"Not executed here (go test was out of lane scope). Invalid --args JSON exits 1. Grep gets --regex only when the JSON marks the pattern as a regex. Glob accepts only a single identifier token, so path globs and qualified names such as Foo.Bar yield no suggestion. Offset without limit suggests a one-line file:N-N range. Literal JSON control escapes in --args strings are kept as backslash sequences so Windows paths survive.","summary":"Added mi-lsp nav suggest as a pure local mapping from Read, Grep, and Glob JSON args to one success-envelope item (nav multi-read, nav search, or nav find), with empty items and exit 0 when there is no equivalent. Windows path backslashes, including backslash-r, are preserved in argv."},{"files":[".claude-plugin/marketplace.json","integrations/claude-code/.claude-plugin/plugin.json","integrations/claude-code/.mcp.json","integrations/claude-code/package.json","integrations/claude-code/INSTALL.md","integrations/claude-code/hooks/hooks.json","integrations/claude-code/hooks/user-prompt-submit.mjs","integrations/claude-code/hooks/post-tool-use.mjs","integrations/claude-code/lib/exec.mjs","integrations/claude-code/lib/hook-main.mjs","integrations/claude-code/lib/parse-suggest.mjs","integrations/claude-code/lib/state.mjs","integrations/claude-code/lib/suggest-call.mjs","integrations/claude-code/tests/hooks.test.mjs","integrations/claude-code/tests/fixtures/fake-mi-lsp.mjs","integrations/pi/mcp.json","integrations/pi/INSTALL.md","integrations/codex/config.toml.snippet","integrations/codex/INSTALL.md","integrations/cursor/mcp.json","integrations/cursor/INSTALL.md","integrations/grok/.grok-plugin/plugin.json","integrations/grok/.mcp.json","integrations/grok/config.toml.snippet","integrations/grok/INSTALL.md"],"ok":true,"p50_ms":"not measured","p95_ms":"not measured","risks":"Hooks assume nav suggest prints one short mi-lsp nav line (or the static mi-lsp nav intent \"<goal>\") and accept argv --event/--prompt/--tool/--consecutive; any other CLI contract stays silent or falls open. mi-lsp mcp may still be unimplemented, so host configs only register the command. Non-mi-lsp tools do not reset the Read/Grep/Glob streak. On Windows MI_LSP_BIN must be the real .exe because spawn uses shell false. Grok plugin validate confirmed the manifest and MCP entry only; no live MCP session. Grok discards allowing UserPromptSubmit stdout, so advisory hooks stay on Claude Code.","summary":"Claude Code plugin at integrations/claude-code (MCP mi-lsp mcp, fail-open UserPromptSubmit and PostToolUse hooks that spawn mi-lsp nav suggest by argv, marketplace at .claude-plugin/marketplace.json) plus Pi, Codex, and Cursor MCP registration docs and a validated Grok plugin with an MCP snippet. No Jev."},{"files":["C:\\repos\\mios\\mi-lsp\\README.md","C:\\repos\\mios\\mi-lsp\\CHANGELOG.md","C:\\repos\\mios\\mi-lsp\\docs\\policies\\mi-lsp-host-doors.md","C:\\repos\\mios\\mi-lsp\\docs\\policies\\mi-lsp-host-troubleshooting.md","C:\\repos\\mios\\mi-lsp\\docs\\policies\\mi-lsp-project-snippets.md","C:\\repos\\mios\\mi-lsp\\docs\\skills\\claude-code-lsp.md","C:\\repos\\mios\\mi-lsp\\skills\\mi-lsp\\SKILL.md","C:\\repos\\mios\\mi-lsp\\skills\\mi-lsp\\agents\\openai.yaml","C:\\repos\\mios\\mi-lsp\\skills\\mi-lsp\\references\\quickstart.md","C:\\repos\\mios\\mi-lsp\\skills\\mi-lsp\\references\\compound-commands.md","C:\\repos\\mios\\mi-lsp\\skills\\mi-lsp\\references\\recipes.md","C:\\repos\\mios\\mi-lsp\\skills\\mi-lsp\\references\\runtime-drift.md","C:\\repos\\mios\\mi-lsp\\.docs\\wiki\\01_alcance_funcional.md","C:\\repos\\mios\\mi-lsp\\.docs\\wiki\\04_RF\\RF-QRY-001.md","C:\\repos\\mios\\mi-lsp\\.docs\\wiki\\07_baseline_tecnica.md","C:\\repos\\mios\\mi-lsp\\.docs\\wiki\\07_tech\\TECH-MCP-ADAPTER.md","C:\\repos\\mios\\mi-lsp\\.docs\\wiki\\09_contratos\\CT-CLI-DAEMON-ADMIN.md","C:\\repos\\mios\\mi-lsp\\.docs\\wiki\\09_contratos_tecnicos.md","C:\\repos\\mios\\mi-lsp\\.docs\\wiki\\ae\\AE-RELEASE-DISTRIBUTION.md"],"ok":true,"p50_ms":"n/a","p95_ms":"n/a","risks":"Docs describe mi-lsp mcp, workspace which, and nav suggest, but internal/cli/mcp.go still returns not implemented and which/suggest are not wired in internal/cli. No new RF id was minted; coverage extends TECH-MCP-ADAPTER, CT-CLI-DAEMON-ADMIN, 01, and RF-QRY-001. The 20 MB RSS cell in TECH-MCP-ADAPTER is still pending. scripts/release/build-dist.ps1 already go-builds mi-lsp.exe for win-x64 and win-arm64, so scripts/install and scripts/release were not edited.","summary":"Documented the optional mi-lsp mcp door, global --max-chars, the four reason codes with a separate canonical detail, workspace which, nav suggest, and MI_LSP_BIN as a real-executable override, with the CLI as authority and integrations/ host doors optional. README install and the Unreleased changelog state that Windows releases ship mi-lsp.exe and a .cmd shim is not the product. build-dist.ps1 already produces that exe for win-x64 and win-arm64."},{"files":[],"ok":true,"p50_ms":"4677.136","p95_ms":"6102.285","risks":"Daemon was already warm on %USERPROFILE%\\bin\\mi-lsp.exe (v0.8.2). Status reported that executable stale versus the measured CLI hash. It was not restarted or stopped. Latency is the local CLI talking to that existing daemon, not a daemon built from .\\mi-lsp.exe. One extra untimed probe ran before the three warmup calls.","summary":"Warm nav overview on C:\\repos\\mios\\mi-lsp\\mi-lsp.exe against an already-running daemon (pid 1424, started 2026-09-18, not stopped). Command: mi-lsp nav overview --workspace mi-lsp --format toon. Three warmup calls then 20 timed calls, all exit 0. Nearest-rank p50 4677.136 ms, p95 6102.285 ms."}]