# Contrato de medición antes/después (v1)

Estado: baseline autorizado por `ALLOW_SPOT` del encargo; after reservado a FINAL_VERIFY del padre. Sin código de producto, builds, suites, commit, push ni compra de créditos. Este contrato se fija antes de las dos primeras invocaciones.

## Unidades y dependencias

| Entregable | Prerrequisito real | Recursos / ejecución |
|---|---|---|
| Baseline MiniMax | receta y fixture fijados | proceso Pi propio; listo, concurrente con LongCat |
| Baseline LongCat | receta y fixture fijados | proceso Pi propio; listo, concurrente con MiniMax |
| After MiniMax | producto integrado + FINAL_VERIFY del padre | mismo fixture, prompt, límites y alias MiniMax |
| After LongCat | producto integrado + FINAL_VERIFY del padre | mismo fixture, prompt, límites y alias LongCat |
| Comparación | ambos resultados terminales de cada par | offline, conserva intentos fallidos |

Concurrencia de la campaña: dos procesos Pi de medición, uno por modelo. No se asume capacidad garantizada del proveedor; un 429 se registra como fallo, no se convierte en resultado del otro modelo. No hay reintentos de campaña ni repetir hasta mejorar. Los tres microobjetivos comparten una conversación por modelo: editar depende de localizar; detectar precede reparar. No se inventa atribución independiente de tokens por microobjetivo.

## Piloto y oráculos

Fuente real del repo: `benchmarks/victory-lab/v1/corpus/go/greeter.go`, commit `4494e7cdf9934ae293d0231454f7d401f25d1d92`. Copia exacta congelada en `benchmarks/harness-measurement/seed/greeter.go`. Se añaden caller y distractor locales generados por `run.py`; no se modifica el corpus original ni el producto.

1. **Localizar:** encontrar la implementación Go de `Greet`; oráculo de salida final contiene `src/greeter.go`.
2. **Editar:** cambiar `Hello, ` por `Hola, ` conservando firma y concatenación de `name`; oráculo estático con firma exacta y expresión esperada (no equivale a aceptación general del programa).
3. **Error semántico antes del test:** caller inicial `Greet(42)` no satisface `name string`. Oráculo exige resultado real de herramienta con `42` + `cannot use`/`IncompatibleAssign` y caller final `Greet("world")`. Se cuenta 1 error sembrado. Se registra aparte que una edición terminó antes de la primera petición de diagnostics; no vale una llamada hermana concurrente a la edición.

El runner no ejecuta tests ni builds. Los oráculos son lectura estática y observación de herramientas del piloto autorizado, no QA del producto. Cualquier cumplimiento parcial queda visible. No hay juez LLM ni PASS inferido del exit code.

## Ruta y variables fijadas

- MiniMax: `opencode-go/minimax-m3.1-flash-free`.
- LongCat: `opencode-go/longcat-2.5-preview-free`.
- Host real: CLI Pi 1.3.0, `pi -p --no-session --model opencode-go/<modelo> --mode json`.
- Prompt literal: `benchmarks/harness-measurement/prompt.md`; misma extensión opt-in `guard.ts`, mismas herramientas, `thinking=low`, máximo 12 turnos, 2048 tokens de salida por petición y 240 segundos por corrida. Umbral de consumo observado acumulado 100000 tokens: corta al terminar la respuesta que lo alcanza; no es un límite duro de tokens de entrada del proveedor.
- `before_provider_request` fija `max_tokens`/`max_completion_tokens`; el journal registra peticiones con ese cap. Ningún header, credential o cuerpo HTTP se registra.
- `.pi/settings.json` del fixture deshabilita retry y compaction. No se cambia configuración global ni gateway. Reintentos internos de transporte no observables: unknown, no se los cuenta como llamadas confirmadas.
- Versiones del modelo: se fija el alias solicitado, y se registra provider/model/api observados. La revisión inmutable del proveedor es **unknown**: el alias free no acredita el peso exacto servido. Si cambia Pi, prompt, guard, runner, fixture, alias, límites, backend o entorno, el par se marca no comparable o se declara la diferencia; jamás se afirma causalidad.
- Única variable intencional: binario mi-lsp baseline publicado frente a producto integrado. Ambos comandos nuevos se intentan en ambas fases; baseline puede devolver comando inexistente, y se usan los mismos fallbacks `rg`/`gopls`. Esto mide ese flujo explícito, no la calidad de todas las superficies ni el nuevo provider MCP.
- Baseline congelado: mi-lsp 0.14.5, SHA256 `9c59d5f36c4512af5b99ed48732ec831097299095386d3eecdea56df1ebefd14`, copia local en `results/harness-measurement/bin/mi-lsp-baseline` (no publicar binario).

## Seguridad de la ejecución

Pi solo expone `read`, `edit`, `write`, `bench_command`. Sin MCP, carga de skills/contexto, extensiones descubiertas ni shell libre. `guard.ts` es infraestructura opt-in de esta medición, no un hook de producto instalado.

- Lecturas del modelo limitadas a archivos Go bajo `src/`, sin symlinks.
- Escrituras del modelo solo `src/greeter.go` y `src/handler.go` dentro de su fixture.
- `bench_command` acepta cuatro cadenas exactas, ejecutadas sin shell. Sus procesos van en bwrap con filesystem host readonly, red deshabilitada, HOME/cachés propios y única raíz writable en el fixture. No ejecuta comandos recibidos arbitrariamente del modelo.
- Autenticación: resolución nativa de Pi existente; no claves en argv ni lectura/volcado de auth. Cuerpos de error del proveedor se omiten del JSONL guardado; quedan categoría y status sanitizados. No se archiva stdout/stderr crudo del proveedor.
- Las carpetas de resultados rechazan sobrescritura. After reinicia solamente el fixture desechable propio; los eventos, métricas y código final de baseline se conservan.

## Comandos exactos

Desde la raíz del worktree de medición, lanzar los dos comandos baseline en paralelo mediante tareas background del harness:

```sh
python3 benchmarks/harness-measurement/run.py baseline minimax --binary results/harness-measurement/bin/mi-lsp-baseline
python3 benchmarks/harness-measurement/run.py baseline longcat --binary results/harness-measurement/bin/mi-lsp-baseline
```

Solo el padre, en FINAL_VERIFY y con su binario integrado (no build fragmentado de lane):

```sh
python3 benchmarks/harness-measurement/run.py after minimax --binary "$INTEGRATED_MI_LSP" --integrated-sha "$INTEGRATED_SHA"
python3 benchmarks/harness-measurement/run.py after longcat --binary "$INTEGRATED_MI_LSP" --integrated-sha "$INTEGRATED_SHA"
python3 benchmarks/harness-measurement/report.py
```

`$INTEGRATED_MI_LSP` y `$INTEGRATED_SHA` son parámetros provistos por el padre, no una ruta o SHA ficticios. El script exige SHA no vacío, pero no acredita por sí solo que el padre haya congelado el producto. Los argv literales completos por intento quedan en `invocation.json`. No relanzar un intento fallido ni borrar su carpeta para maquillarlo.

Comandos disponibles para el modelo, idénticos en ambas fases:

```text
mi-lsp find-files '*greet*' --format json
rg --files src -g *greet*
mi-lsp nav diagnostics src/handler.go --format json --no-daemon
gopls check src/handler.go
```

El backend corre directo y aislado para no usar ni mutar estado de otro workspace. Es un benchmark de CLI en Pi; no acredita mi-mcp execute, Claude Code, Yoga o Windows (otras unidades del objetivo padre).

## Esquema de datos crudos

Carpeta por `phase/model`, schema `harness-measurement/v1`:

- `invocation.json`: phase, alias/version conocida o unknown, Pi version, repo HEAD, integrated SHA/null, hashes de binario/prompt/guard/runner/fixture, límites, argv, cwd y fecha UTC.
- `events.jsonl`: eventos nativos seleccionados sin duplicación: session, turn_start, message_end, tool_execution_start/end, agent_settled y retry. No sumar deltas acumulativos ni mensajes duplicados de agent_end. Errores del proveedor sanitizados.
- `guard.jsonl`: request (output_cap), http (status), limit (reason). Sin payloads ni headers.
- `metrics.json`: metadata anterior más status literal (`completed`, `provider_error`, `harness_error`, `task_incomplete`, `wall_timeout`, `token_threshold`), exit, segundos monotónicos, usage, peticiones observadas, respuestas, llamadas y errores de herramientas, archivos leídos explícitamente, errores de proveedor/harness y los tres oráculos booleanos.
- `final-src/`: copia de archivos resultantes del piloto para revisión del oráculo.

`usage.{input,output,cacheRead,cacheWrite,totalTokens}` contiene `{value:number|null,source,reason}`. Se suman únicamente los campos numéricos finales de respuestas assistant completadas reportados por Pi. Un intento sin respuesta/usage confiable lleva null + motivo, no cero tokens gastados. Si un intento mezcla respuestas exitosas y errores, los tokens solo cubren respuestas contabilizadas, nunca representan un total facturado completo. Los ceros de caché son contadores normalizados reportados por Pi, no prueba independiente de cache miss del proveedor. Sin conversiones desde bytes/caracteres ni ahorro ficticio.

`tool_calls` = eventos tool_execution_start; `tool_errors` = finales isError. `explicit_files_read` cuenta paths distintos de herramientas read; archivos transitivamente leídos por gopls/mi-lsp = unknown por falta de inventario de I/O, no cero. `request_payloads` cuenta el hook pre-provider, no intenta reconstruir llamadas HTTP invisibles. También se conservan respuestas assistant y statuses HTTP observados.

## Comparación

Tabla con ambas fases y todas las métricas, incluso fallos. Ratio por modelo y métrica = after/baseline; baseline=0 → undefined_zero_denominator; ausente/unknown → unknown con motivo; par no comparable → not_comparable. Un fallo conserva sus datos crudos pero no obtiene un ratio presentado como mejora. No mezclar MiniMax con LongCat, no sustituir uno por otro. N=1 conversación por modelo/fase, tres microobjetivos compartidos, sin inferencia causal ni superioridad estadística; la disponibilidad free, el caché y una revisión opaca del modelo son confusores.

## Contexto aplicable

Consulta `mi-decidir consultar` ejecutada: `persona:ejecutores-muse-primero-y-free-20261019`; paquete parcial, lexical fallback, adapter_absent, judged=false y already_decided=unknown. No autoriza por sí mismo: la ruta específica MiniMax/LongCat de este benchmark es el mandato explícito del usuario. Fuente operacional leída: reporte OpenCode Go del asistente del 20/10; sus éxitos previos no se reutilizan como baseline de estas tareas. Canon contextual consultado mediante mi-lsp: RF-GPH-011 (no extrapolar superioridad desde subset), gobierno y arquitectura del repo. No se ejecutan sus suites ni gates durante esta lane.
