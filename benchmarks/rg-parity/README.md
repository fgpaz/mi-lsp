# Benchmark de paridad mi-lsp vs ripgrep

Compara `mi-lsp` con `rg` en cuatro tipos de consulta (definition, refs, intent, literal) sobre repos Go, TypeScript y C#. Mide precisión, recall, bytes de salida, latencia, "falsos vacíos" (mi-lsp devuelve 0 items o error mientras rg encuentra resultados) y tasa de error.

Solo usa la biblioteca estándar de Python 3 y requiere `rg` en el `PATH`. Todas las consultas son de solo lectura: el benchmark nunca ejecuta `mi-lsp index`.

## Uso

```bash
python3 benchmarks/rg-parity/bench.py \
  --milsp <binario mi-lsp> --label <baseline|final> --out <directorio>
```

Opciones útiles: `--repos-root` (por defecto `~/repos/mios`, donde viven los repos), `--only <substring>` para correr solo los casos cuyo id lo contenga, `--cases` para usar otro archivo de casos.

Variable `BENCH_MILSP_EXTRA_ARGS`: argumentos extra para cada llamada a mi-lsp (por ejemplo `--no-daemon`, para no medir un daemon global de otra versión). La corrida final usó `MI_LSP_AUTOINDEX=0 BENCH_MILSP_EXTRA_ARGS=--no-daemon`, para que el autoindex no altere el estado de los repos durante la medición.

Salida en `<out>`:

- `<label>.json`: resultado crudo por caso (comandos, items normalizados como `archivo:línea`, bytes, latencias, error).
- `<label>.md`: tabla resumen por tipo de consulta y lenguaje.
- `compare.md`: se genera cuando existen `baseline.json` y `final.json` en `<out>`.

## Repos y casos

Los repos son workspaces registrados en mi-lsp y se consultan con `cwd` en la raíz del repo y `--workspace <alias>`:

| Alias | Lenguaje |
|---|---|
| `mi-lsp` | Go |
| `mi-gateway` | Go |
| `pi-subagents` | TypeScript |
| `tedi-memory` | C# |

`cases.json` define 44 casos curados (por repo: 4 definition, 4 refs, 1 literal, 2 intent). Cada repo declara los globs de código y la expresión regular de declaración que usa rg.

| Tipo | mi-lsp | rg | Oráculo |
|---|---|---|---|
| definition | `nav find <sym> --exact` | `rg -n -e <regex de declaración>` | resultado de rg (archivo:línea); `expected` en el caso es un control de cordura |
| refs | `nav refs <sym>` | `rg -n -w -F <sym>` | resultado de rg sobre archivos de código |
| literal | `nav search <texto>` | `rg -n -F <texto>` | resultado de rg sobre archivos de código |
| intent | `nav intent "<pregunta>"` | `rg -i -c` con las palabras de la pregunta, archivos ordenados por coincidencias | `expected_files` del caso |

## Métricas

- Se ejecuta con `MI_LSP_CLIENT_NAME=bench` y `MI_LSP_SESSION_ID=bench-<label>`, flags por defecto (sin `--full`), y se registra `truncated`.
- Puntuación con `--format json`; los bytes se miden además con `--format compact` y sin `--format`. Bytes de rg = stdout.
- Precisión = |mi ∩ oráculo| / |mi|, recall = |mi ∩ oráculo| / |oráculo|, comparando `archivo:línea`. Si mi-lsp devuelve items sin línea se compara por archivo y se marca `line_missing`.
- Precisión queda sin valor cuando mi-lsp devuelve 0 items. En los resúmenes se promedia solo sobre casos con valor; un error cuenta recall 0.
- intent: hit@5 = el archivo esperado aparece entre los primeros 5 archivos distintos (campos `file`, `file_path`, `path` o `doc_path`). Para rg, entre los 5 archivos con más coincidencias.
- Latencia: mediana de 3 corridas de pared, en ms. Timeout de 30 s por comando.
- Error: salida distinta de cero, `ok:false` o timeout; se registra el código (`error.code`, `timeout`, `invalid_json`, `exit_N`).
- Falso vacío: el oráculo de rg tiene resultados (o, en intent, hay archivo esperado) y mi-lsp devuelve 0 items o error.

## Latencia fría de refs TypeScript

`bench_ts_cold_refs.py` separa la latencia de la primera respuesta de la calidad semántica posterior. Para cada intento crea un `HOME` temporal, copia allí el registro existente (sin modificar el registro del usuario), inicia un daemon aislado desde la raíz del workspace y ejecuta dos consultas en la misma App persistente. No indexa ni registra workspaces. La primera respuesta puede ser un fallback textual degradado durante el warm-up: su latencia se informa, pero no cuenta como calidad semántica. La segunda consulta debe devolver `backend=tsserver`, items exclusivamente `origin=semantic` y todas las referencias esperadas explícitas; el campo `degraded` ausente o explícitamente `false` cuenta como no degradado según `primitives-v2` (`omitempty`), pero `null` no cuenta como `false`. El proceso y runtime TypeScript deben estar disponibles antes de ejecutar el benchmark:

```bash
python3 benchmarks/rg-parity/bench_ts_cold_refs.py \
  --milsp <binario-mi-lsp> --workspace pi-subagents \
  --workspace-root ~/repos/mios/pi-subagents \
  --symbol resolvePiLaunchToolPlan \
  --file src/runs/shared/child-tool-plan.ts \
  --expected-ref src/api/preflight.ts:403 \
  --expected-ref src/runs/shared/child-launch.ts:194 \
  --expected-ref src/runs/background/async-execution.ts:1157 \
  --expected-ref src/runs/background/async-execution.ts:1977 \
  --expected-ref src/runs/background/subagent-runner.ts:817 \
  --expected-ref src/runs/background/subagent-runner.ts:1177 \
  --npm-prefix /tmp/milsp-ts-runtime --runs 5
```

`--expected-ref` se puede repetir para ampliar el oráculo; los seis valores del ejemplo corresponden a call sites TypeScript reales del checkout curado y se comprueban todos, no solo uno. Si una referencia se mueve, actualiza explícitamente el oráculo. `--npm-prefix` es opcional cuando el runtime ya se resuelve por las rutas habituales. El JSON separa `first_latency_under_2s`, `warmup_observation`, `warmup_success` y `semantic_quality`; esta última incluye las ubicaciones esperadas presentes y ausentes. Una primera respuesta degradada de texto sí puede cumplir el umbral de latencia cuando el fallback está tipado (`fallback_used=text`, reason presente, origins solo `text`); nunca cuenta como warm-up semántico ni como calidad. La aceptación global requiere la primera latencia válida menor de 2000 ms, warm-up semántico posterior en la misma App y todas las referencias esperadas en resultados exclusivamente semánticos no degradados. Envelope inicial inválido/no tipado, warm-up fallido o referencias esperadas ausentes producen rechazo con causas explícitas. `degraded_flag_present` distingue la omisión contractual de `false` de un `degraded: null`, que se rechaza. El arranque/parada del daemon queda fuera de la latencia de consulta y se informa por separado. Este benchmark se prepara para FINAL_VERIFY y no se ejecuta durante BUILD.

## Notas

- Los resultados dependen del estado del índice de cada workspace al momento de correr. Si un workspace no tiene catálogo publicado, `nav find` devuelve `index_not_ready`; eso cuenta como falso vacío y como error.
- Los repos se leen en su estado actual; las posiciones `expected` de definition se curaron en un instante dado y el control `expected_in_rg_oracle` del resultado avisa si se desfasaron.
- Los `.md` solo usan alias de repo, sin rutas absolutas.
