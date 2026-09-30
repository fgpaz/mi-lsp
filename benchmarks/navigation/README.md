# Benchmark de navegación de mi-lsp

Este benchmark reproduce las consultas observadas en `laneG-report.md` y añade
una muestra de arranque frío del servidor MCP nativo. Cada ejecución conserva
las muestras individuales y calcula mediana, p95 nearest-rank y máximo; no
selecciona la mejor muestra. El protocolo sigue el requisito de 30 repeticiones
de `TP-GPH` y la salida por muestra de Victory Lab.

Ejecutar desde la raíz del repositorio:

```sh
MI_LSP_CLIENT_NAME=codex-benchmark MI_LSP_SESSION_ID=navigation-benchmark \
  python3 benchmarks/navigation/run.py --binary /path/to/checkout-built/mi-lsp \
  --workspace mi-lsp-src --cwd "$PWD" --samples 30 --output /tmp/milsp-navigation.json
```

También se aceptan `--binary`, `--workspace`, `--cwd`, `--samples`, `--timeout`
y `--output`. El proceso hijo recibe `MI_LSP_CLIENT_NAME` y
`MI_LSP_SESSION_ID`; ambos deben estar definidos y no vacíos en el entorno.
El timeout es por proceso completo y al vencer se termina el grupo de procesos.

Se miden `nav find ExtractFileSymbols`, `nav search ErrGraphRepositoryInvalid`,
`nav multi-read internal/model/graph.go:35-52`, `nav route "mi-lsp navigation"`,
`nav intent "medir navegación mi-lsp"` y `nav affected
internal/model/graph.go`. Para cada consulta CLI se guardan 30 muestras
`warm` (cada una en subprocess nuevo) después de una muestra inicial `cold`.
“Cold” identifica la primera ejecución de esta campaña; no limpia la caché del
sistema operativo ni promete un estado de daemon vacío. También se repiten las
consultas find/search/multi-read/affected con selector por cwd, alias y ruta
absoluta para registrar la identidad de workspace observada.

La medición MCP inicia un proceso nuevo por muestra y cronometra desde antes del
spawn hasta la respuesta a `tools/call` `nav_search`, incluyendo `initialize`,
la notificación `initialized`, el intercambio JSON-RPC y el arranque del
proceso. Cada muestra tiene timeout firme. El caso usa el workspace indicado y
la búsqueda `ErrGraphRepositoryInvalid`.

Los agregados incluyen solo procesos CLI con exit code 0 y respuestas MCP que
completaron `initialize` y `tools/call`. Fallos y timeouts quedan en las
muestras crudas; si MCP no existe en ese binario, el artefacto declara
`status: unavailable` y deja las estadísticas sin muestras válidas.

El JSON de salida usa el contrato `milsp-navigation-benchmark/v1`, incluye
revisión Git, SHA-256 del binario, Python/OS/arquitectura, cwd, workspace,
variables de cliente (sin otros valores del entorno), argv, estado de salida,
bytes de stdout/stderr, workspace anunciado cuando se puede reconocer, muestras
crudas y agregados. Para JSON explícito se ejecuta un probe aparte de
`nav search ... --format json`: registra si stdout parsea como JSON y el tipo
del valor raíz. Las comparaciones before/after deben usar el mismo binario,
checkout, cwd, workspace, variables, cantidad de muestras, timeout y estado
operativo. El benchmark informa tiempos observados; no impone umbrales ni afirma
causalidad.
