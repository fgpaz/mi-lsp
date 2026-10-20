---
wiki_contract: WIKI-CONTRACT-v1
id: RF-QRY-002
kind: requirement
status: active
title: RF-QRY-002 - Resolver routing con fallback de daemon y backend
links:
  implements:
  - FL-QRY-01
  code:
  - path: internal/cli/nav.go
    role: behavior
  - path: internal/cli/root.go
    role: behavior
  - path: internal/daemon/lifecycle.go
    role: behavior
  - path: internal/daemon/server.go
    role: behavior
  - path: internal/service/autoindex.go
    role: behavior
  - path: internal/service/coach.go
    role: behavior
  - path: internal/service/context.go
    role: behavior
  - path: internal/service/search.go
    role: behavior
  - path: internal/service/semantic.go
    role: behavior
  - path: internal/service/semantic_errors.go
    role: behavior
  - path: internal/service/semantic_kotlin.go
    role: behavior
  - path: internal/service/semantic_refs.go
    role: behavior
  - path: internal/telemetry/access_diagnostics.go
    role: behavior
  - path: internal/telemetry/access_events.go
    role: behavior
  - path: internal/worker/gopls.go
    role: behavior
  - path: internal/worker/rust_analyzer.go
    role: behavior
  - path: internal/worker/tsserver.go
    role: behavior
---

## Propósito

RF-QRY-002 - Resolver routing con fallback de daemon y backend

### 1. Hoja de ejecución
- **Campo**: ID; **Valor**: RF-QRY-002
- **Campo**: Título; **Valor**: Resolver routing con fallback de daemon y backend
- **Campo**: Actores; **Valor**: Usuario, Skill, Agente, CLI, Daemon/Core
- **Campo**: Prioridad; **Valor**: alta
- **Campo**: Severidad; **Valor**: alta
- **Campo**: FL origen; **Valor**: FL-QRY-01

## Comportamiento ordenado

### 2. Precondiciones detalladas
- **Condición**: Workspace resoluble; **Tipo**: funcional; **Estado requerido**: obligatorio
- **Condición**: Operación `nav` soportada; **Tipo**: funcional; **Estado requerido**: obligatorio
- **Condición**: Backend candidato identificable; **Tipo**: técnica; **Estado requerido**: obligatorio

### 3. Secuencia principal
1. La CLI resuelve workspace y comando.
2. La CLI decide en forma centralizada si la operación debe usar daemon o ejecutarse directo.
3. Para lecturas baratas de catalogo/texto (`nav.find`, `nav.search`, `nav.intent`, `nav.symbols`, `nav.outline`, `nav.overview`, `nav.multi-read`), la CLI ejecuta directo sin depender del daemon.
4. Para queries semánticas o compuestas, la CLI intenta enviar la request al daemon global si esta disponible.
5. Si el daemon responde, enruta al backend adecuado y devuelve el envelope.
6. Si el daemon no responde o no aplica, la CLI ejecuta fallback directo y emite `hint: "daemon_unavailable; served from local text index"` en el envelope.
7. Si el backend primario no esta disponible, el core usa un backend degradado, registra warnings y marca el envelope con `degraded=true`, `reason` tipificado y `fallback_used` (catálogo versionado en [[RF-QRY-001]], contrato `primitives-v2`).

### Kotlin experimental: excepción al fallback general

- **Marcador estable**: RF-QRY-002.kotlin-runtime-v0
- **Estado del comportamiento**: experimental_unverified
- **Evidencia referenciada**:
  - internal/service/semantic_kotlin.go
  - internal/service/context.go
  - internal/service/semantic_refs.go
  - internal/cli/nav.go
- **Comprobación prevista**:
  - .docs/wiki/06_tests/TP-QRY.md::TP-QRY.kotlin-runtime-v0
- **routing**:
  - **kt**: kotlin-language-server
  - **kts**: excluded
  - **module**: explicit_declared_build_gradle_entrypoint
  - **position**: explicit_file_line_and_1_based_UTF16_column
  - **unavailable**: ok_false_reason_unavailable_no_text_substitution
  - **unsupported**: ok_false_reason_unsupported_operation
  - **valid_LSP_answer**: ok_true_partial_true_reason_explicit_incomplete
  - **empty_answer**: partial_not_proof_of_completeness
  - **symbol_name_anchoring**: forbidden
  - **compiler_calls_and_currentgraph**: unsupported_in_K1

Esta excepción prevalece sobre el fallback general de `nav.refs` y el enriquecimiento slice-first: Kotlin no agrega callers de catálogo ni prueba un vacío con búsqueda textual. El caller puede pedir búsqueda textual **separada**, nunca presentada como semántica. `nav refs --column` y `nav context --column` reciben UTF-16 explícito; no se inventa posición por nombre. Definition/document_symbols son interfaz worker, no cambian `nav.find`/`nav.symbols`. Falta de classpath/JDK observado mantiene K1 parcial incluso con initialize exitoso. Config y autorización Gradle en [[TECH-KOTLIN-BACKEND]].

### 5. Casos especiales y variantes
- Daemon caido: no es error fatal si el modo directo puede responder.
- `nav.find`, `nav.search`, `nav.intent`, `nav.symbols`, `nav.outline`, `nav.overview` y `nav.multi-read` no deben bloquearse esperando daemon; su contrato es repo-local directo.
- Workspace `container`: `find/search/intent/overview` operan globalmente; `find/search/intent` pueden acotar con `--repo` sin pasar a routing semántico, mientras `refs/context/deps` pueden requerir `--repo` o `--entrypoint`.
- El `backend` usado siempre se informa; si hay ambigüedad controlada, el backend canónico es `router`.
- Para archivos `.py`/`.pyi`, el routing resuelve a `pyright` si esta disponible; si no, degrada a `catalog`/`text` con warning explicito. El valor `--backend pyright` fuerza el uso de Pyright sin fallback automatico.
- Para `nav context` sobre archivos no semánticos, el routing salta directo a `backend=text` y conserva el mismo envelope.
- Para `nav context` sobre `ts/js`, el core prioriza `tsserver` pero debe conservar `slice_text` y degradar a `catalog` o `text` cuando `tsserver` no exista.
- Para archivos `.go`, el extractor AST nativo conserva catálogo navegable sin `gopls`; `nav context` puede enriquecer la respuesta con `gopls` y degrada a `catalog` o `text` con warning cuando no esta disponible.
- Para archivos `.rs`, `nav context` y `nav refs` usan `rust-analyzer` cuando está disponible; el catálogo lexical de Rust y las superficies `nav symbols`, `nav intent`, `nav search` y `nav multi-read` siguen disponibles sin él.
- Para `nav context` sobre C#/TS/Python/Go/Rust, el core es slice-first: si Roslyn/tsserver/Pyright/gopls/rust-analyzer fallan al arrancar por permisos o proceso bloqueado, devuelve `ok=true`, conserva `slice_text`, agrega warning tipado `backend_runtime/<code>` y registra `requested_backend`, `result_backend`, `backend_fallback_taken` y `runtime_error_code` en telemetria sanitizada.
- Para `nav search`, si `rg` existe pero falla por permisos o arranque de proceso, el runtime debe degradar a busqueda Go nativa, emitir warning tipado `backend_runtime/<code>` y no exponer argv, payload ni contenido de archivos en `decision_json`.
- `nav.search` puede procesar archivos en paralelo; la selección acotada conserva los primeros resultados por orden lexicográfico de ruta y línea, con salida determinista sin ordenar todo el conjunto de coincidencias.
- `nav.find` puede refrescar los archivos candidatos antes de consultar: hasta 32 archivos, 4 MiB por archivo y 16 MiB agregados, con presupuesto de 250 ms. Si publica una actualización del catálogo, repite la consulta; si el refresh falla o vence, sirve el snapshot publicado y añade una advertencia accionable. Este refresh no ejecuta Roslyn ni deja escrituras en segundo plano.
- `nav.find` decide si puede declarar cero coincidencias usando una generación publicada (`active_catalog_generation_id`) o metadata completa de `ReplaceCatalog` (`indexed_at` + `total_files`, ambos escritos atómicamente, incluso `total_files=0`). Un esquema sin esos campos o metadata parcial no basta. La ausencia de filas no demuestra completitud; un root transitorio puede responder si su catálogo se publicó con una de esas señales.
- `nav.find` sin catálogo publicado, con base ilegible o con esquema roto responde desde texto (`ok=true`, `backend=text`, `degraded=true`, `reason=index_not_ready|index_schema_broken`) y lanza un único reindex completo en segundo plano; ver [[RF-IDX-001]]. Nunca responde un vacío sin marcarlo como degradado.
- `nav.refs` elige el backend por el lenguaje del símbolo y no por el primer lenguaje del workspace: definición en el catálogo, si no el primer hit de texto, si no los lenguajes registrados. Roslyn se usa solo para C#. Cuando no hay `--file`, inyecta el ancla de la definición para `gopls`, `tsserver` y `rust-analyzer`, que resuelven por posición. `--file` explícito se enruta por la extensión del archivo; una extensión sin backend semántico cae a `backend=text` con `reason=language_unsupported`.
- `nav.refs` nunca devuelve un falso vacío: cualquier error del backend (`reason=lsp_unavailable` si falta el binario o runtime, `lsp_error` en el resto) o un resultado semántico vacío vuelve a consultar por texto con límites de palabra. Si el texto encuentra coincidencias tras un vacío semántico, la razón es `semantic_empty_text_hits`; si tampoco encuentra, el vacío es válido con `reason=no_matches`. Cada item lleva `origin` (`semantic` o `text`) y, cuando el catálogo conoce el símbolo contenedor, `caller {name, kind, line}`. El backend semántico de `nav.refs` tiene un plazo (`MI_LSP_REFS_TIMEOUT`, por defecto 8 s): si lo agota, responde el texto con `reason=lsp_error` y ese backend queda 2 minutos en enfriamiento antes de volver a intentarse. Excepción acotada para `tsserver`: una primera espera de hasta 500 ms entrega resultados de texto tipados mientras el proceso sigue calentándose; la llamada semántica continúa desacoplada del contexto de la petición y tiene un deadline propio máximo de 8 s. El fallback de texto dispone de un presupuesto de 1,2 s; la expiración de la primera espera no activa el enfriamiento para permitir que una llamada posterior use la semántica ya disponible. La salida permanece `degraded=true`, `fallback_used=text` y `items[].origin=text`; nunca presenta el fallback como resultado semántico. El objetivo de menos de 2 s se valida en un checkout y host reproducibles, no se presume por configuración. La cobertura de fallback textual tipado, supervivencia del warm-up a la cancelación y retry semántico está en [[TP-QRY]] (TC-QRY-192). El protocolo de `tsserver` es asimétrico: solicitudes JSON terminadas en salto de línea por `stdin` y respuestas con `Content-Length` por `stdout`. `bench_ts_cold_refs.py` mide por separado la primera latencia y la calidad semántica de una consulta posterior en la misma App de un daemon aislado; solo acepta esta última si `backend=tsserver`, `degraded` ausente o `false` según `primitives-v2` ([[RF-QRY-001]]), todos los items son semánticos y están presentes las referencias esperadas. Una primera respuesta de texto tipada puede cumplir solo `first_latency_under_2s`; no prueba warm-up ni calidad. La aceptación global exige `warmup_success` y todas las referencias esperadas reales en la respuesta semántica posterior; ver [[TP-QRY]] (TC-QRY-193). El texto prioriza los archivos de código del lenguaje del símbolo, después otros lenguajes de código y solo sin código cae a archivos no código (`language_unsupported`).
- `nav.refs --context N` (0 a 5; el argumento `context` de la herramienta MCP `nav_refs` fue retirado con la puerta MCP el 2026-10-15) agrega N líneas de contexto antes y después de cada referencia; un valor fuera de rango se rechaza antes de ejecutar.
- Localización de `gopls` (`internal/worker/gopls.go`), en este orden: `MI_LSP_GOPLS_PATH`, `PATH`, `GOBIN`, `GOPATH/bin`, `~/go/bin` y `bin/` o `.bin/` del workspace. Localización de Rust (`internal/worker/rust_analyzer.go`): `MI_LSP_RUST_ANALYZER_PATH`, `PATH`, `~/.cargo/bin/rust-analyzer` y `bin/` o `.bin/` del workspace. Si el binario no existe, `nav.refs` degrada con `reason=lsp_unavailable`.
- El warm del daemon es por lenguaje del workspace e incluye `gopls` para Go y `rust-analyzer` para Rust; ya no calienta Roslyn por defecto en repos sin C# (`backendsForWorkspace`).
- `nav.search` sigue siendo una búsqueda textual directa y puede devolver coincidencias de archivos aunque el workspace no esté registrado o no tenga índice semántico. No depende de la evidencia de completitud que requiere `nav.find`.
- Con `--allow-cross-workspace`, un path existente pero todavía no registrado sólo recibe `workspace_resolution_failed` (`Kind=workspace`, `Stage=workspace_resolution`, `ReasonCode=invalid_workspace`) y el comando dinámico `workspace add <path>` cuando tampoco tiene catálogo publicado listo. Un catálogo transitorio publicado con pointer activo o par atómico `indexed_at` + `total_files` se puede consultar antes del registro. Sin ese flag, la negativa de mismatch conserva su error terminal `explicit_incomplete` y su comportamiento fail-closed.
- JSON explícito mantiene el mismo diagnóstico y los mismos datos estructurados que la llamada directa. [RETIRADO 2026-10-15: la vista compacta MCP de `nav_refs` ya no existe.] Las advertencias stale existentes se mantienen cuando se responde con un snapshot publicado.
- `nav.route` y `nav.intent` pueden refrescar hasta cinco paths candidatos Markdown con presupuesto de 500 ms. Tras una publicación confirmada recargan el catálogo y vuelven a puntuar una única vez; si no se confirma por error o deadline, conservan los resultados en memoria y añaden una advertencia. El refresh no reconstruye el grafo ni deja escrituras de fondo.
- El refresh lee y extrae contenido bajo bloqueo de escritura cancelable; una solicitud que esperaba el lock usa el contenido observado al obtenerlo. Un dialecto de hash desconocido no se interpreta como stale ni elimina una fila del índice legacy.
- Para un path consultado cuya fila de catálogo existe pero el archivo ya no existe, el refresh publica un tombstone sólo para esa fila/path; también trata así un `ENOENT` ocurrido durante la lectura. `nav.find` repite la query tras publicar y no devuelve los símbolos stale. La publicación inválida el estado graph-native; no sintetiza evidencia de grafo.
- Para `nav search`, si la busqueda agota timeout durante el scan después de materializar resultados parciales seguros, el runtime debe conservar esos resultados, responder `ok=true`, emitir warning humano de timeout, registrar `hint_code=search_timeout`, `coach.trigger=search_timeout` y `failure_stage=none`, y proponer un `next_hint` de narrowing sin repetir ciegamente la misma consulta.
- Para `nav search --include-content`, si un match viene del índice pero el archivo ya no existe en disco, el runtime debe conservar el item y emitir stale-index warning accionable hacia reindexado, no fallar toda la busqueda.
- Para queries symbol-like en `nav search` literal, el envelope puede emitir `coach.trigger=symbol_query_detected` con acciones hacia `nav find --exact` y `nav related`; esta guia complementa, no reemplaza, el resultado textual.

## Superficies

CLI `mi-lsp`, con operaciones y selectores detallados en el comportamiento del requisito.

## Entradas/salidas tipadas

### 6. Impacto en el modelo de datos
- `QueryEnvelope`
- `AccessEvent`
- `WorkspaceEntrypoint`

## Errores

### 4. Errores tipados
- **Código**: `QRY_ROUTE_WORKSPACE_NOT_FOUND`; **Causa**: workspace inválido; **Trigger**: alias/path no resoluble; **Respuesta esperada**: abortar con `ok=false`
- **Código**: `QRY_BACKEND_UNAVAILABLE`; **Causa**: no hay backend ejecutable; **Trigger**: sin daemon y sin backend directo utilizable; **Respuesta esperada**: abortar con warning/error explícito
- **Código**: `QRY_PROTOCOL_MISMATCH`; **Causa**: handshake incompatible; **Trigger**: CLI y daemon/worker difieren en protocolo; **Respuesta esperada**: abortar con mensaje accionable
- **Código**: `QRY_ROUTING_AMBIGUOUS`; **Causa**: selector insuficiente en `container`; **Trigger**: símbolo o target coincide con varios repos; **Respuesta esperada**: devolver candidatos y `next_hint`
- **Código**: `process_spawn_access_denied`; **Causa**: runtime o herramienta externa no arranca por permisos; **Trigger**: `CreateProcess`, `Access is denied`, `permission denied`; **Respuesta esperada**: preservar fallback posible y registrar `backend_runtime`
- **Código**: `process_spawn_failed`; **Causa**: runtime o herramienta externa no arranca por error de proceso; **Trigger**: `fork/exec`, `failed to start`, imagen inválida; **Respuesta esperada**: preservar fallback posible y registrar `backend_runtime`
- **Código**: `lsp_unavailable` / `lsp_error`; **Causa**: el backend semántico de `nav.refs` falta o falla; **Trigger**: binario ausente (`executable file not found`, `node is required`) o error del backend; **Respuesta esperada**: no es error terminal: `ok=true`, fallback de texto por límites de palabra, `degraded=true`, `reason=lsp_unavailable` o `lsp_error`, `fallback_used=text`

## Criterios de aceptación (numerados)

1. La CLI resuelve workspace y comando.
2. La CLI decide en forma centralizada si la operación debe usar daemon o ejecutarse directo.
3. Para lecturas baratas de catalogo/texto (`nav.find`, `nav.search`, `nav.intent`, `nav.symbols`, `nav.outline`, `nav.overview`, `nav.multi-read`), la CLI ejecuta directo sin depender del daemon.
4. Para queries semánticas o compuestas, la CLI intenta enviar la request al daemon global si esta disponible.
5. Si el daemon responde, enruta al backend adecuado y devuelve el envelope.
6. Si el daemon no responde o no aplica, la CLI ejecuta fallback directo y emite `hint: "daemon_unavailable; served from local text index"` en el envelope.
7. Si el backend primario no esta disponible, el core usa un backend degradado, registra warnings y marca el envelope con `degraded=true`, `reason` tipificado y `fallback_used` (catálogo versionado en [[RF-QRY-001]], contrato `primitives-v2`).
