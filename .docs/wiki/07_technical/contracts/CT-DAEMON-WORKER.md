---
wiki_contract: WIKI-CONTRACT-v1
id: CT-DAEMON-WORKER
kind: contract
status: active
title: CT-DAEMON-WORKER
links:
  supports:
  - RF-GPH-008
  - RF-QRY-002
  - RF-QRY-026
---

```yaml
harness_protocol: SDD-HARNESS-v1
id: CT-DAEMON-WORKER
kind: worker-contract
audience: dual
imports: [.docs/wiki/07_technical/TECH-DAEMON-GOBERNANZA.md, .docs/wiki/07_technical/TECH-WORKER-HOSTS.md]
exports: [CT-DAEMON-WORKER]
agent_must_read: [.docs/wiki/07_technical/contracts/CT-DAEMON-WORKER.md, internal/worker/lsp_client.go, internal/daemon/client.go]
agent_may_edit: [.docs/wiki/07_technical/contracts/CT-DAEMON-WORKER.md]
agent_must_not_edit: [internal/worker/, internal/daemon/, .docs/wiki/_mi-lsp/read-model.toml]
verify: ["Leer .docs/wiki/07_technical/contracts/CT-DAEMON-WORKER.md y .docs/wiki/07_technical/TECH-DAEMON-GOBERNANZA.md; cotejar las afirmaciones con la fuente publicada internal/worker/lsp_client.go y sus límites. Registrar únicamente comandos y resultados efectivamente ejecutados; esta especificación no afirma una ejecución."]
stop_if: [Se reenvíen errores después de conectar, se presente un backend parcial como limpio o se cambie lifecycle sin evidencia/oráculo.]
evidence: [.docs/wiki/07_technical/contracts/CT-DAEMON-WORKER.md, .docs/wiki/07_technical/TECH-DAEMON-GOBERNANZA.md, internal/worker/lsp_client.go]
```

## Consumidor

CLI, agentes y consumidores locales de las operaciones descritas; no se amplía la autoridad del consumidor.

### Frontera y responsable
- Boundary: daemon/core -> worker Roslyn, runtime `tsserver`, `pyright-langserver`, `gopls`, `rust-analyzer` o Kotlin experimental `kotlin-language-server`
- Owner logico: Semantic backends
- Scope: framing, envelopes, lifecycle, payloads derivados y bootstrap de runtimes

## Operaciones

Se conservan requests, envelopes, campos, límites y semántica detallados a continuación.

Volver a [07_technical/contracts/CT-TECHNICAL-BASELINE.md](CT-TECHNICAL-BASELINE.md).

### Resumen
Define el contrato interno entre core/daemon y los backends semánticos, con foco en versionado, framing, selección de runtime y reglas de degradacion.

### Familias de contrato
#### Framing canónico

Roslyn worker:

- transporte por `stdio`
- mensajes JSON con length-prefix
- request envelope actual:
  - `protocol_version`
  - `method`
  - `workspace`
  - `backend_type`
  - `payload`
- response envelope actual:
  - `ok`
  - `backend`
  - `items`
  - `warnings`
  - `error`
  - `stats`

TS semantic bridge:

- transporte via `node <tsserver.js>` y framing `Content-Length` propio de `tsserver`; no hay handshake separado de `mi-lsp`.
- En Windows se resuelve el ejecutable Node y el realpath de `tsserver.js`; los paths se pasan como argv separados. La consulta de raíz global usa `node <npm-cli.js> root -g`, nunca ejecuta un shim `.cmd` ni construye una cadena de shell. La raíz es la última línea no vacía de stdout; stderr es solo diagnóstico, nunca parte del path.
- El lector de respuestas ignora líneas vacías previas al primer header, consume exactamente los bytes de `Content-Length` y recorta espacios del cuerpo antes de decodificar JSON; tolera el LF sobrante de CRLF en Windows sin cambiar el framing de salida. Longitud inválida o JSON malformado son errores explícitos. La causa precede al argv sanitizado en el detalle de fallos.
- El fallo de spawn o protocolo conserva etapa, argv real con segmentos de perfil redacted, causa y stderr acotado/sanitizado; no incluye contenido de source ni secretos.

Pyright y Go LSP bridge:

- transporte por `stdio` hacia `pyright-langserver` o `gopls`
- framing LSP JSON-RPC 2.0 con `Content-Length`
- lifecycle y documentos abiertos gestionados por el cliente LSP generico

### Backends soportados

- `roslyn`
- `tsserver`
- `pyright`
- `gopls`

### Operaciones minimas actuales

Roslyn:

- `find_symbol`
- `find_refs`
- `get_overview`
- `get_context`
- `get_deps`
- `status`

TS semantic bridge:

- `get_context`
- `find_refs`

Pyright y gopls LSP bridge:

- `get_context` (via `textDocument/hover`)
- `find_refs` (via `textDocument/references`)

### Verbos nativos de símbolos y diagnósticos

Los verbos root y sus aliases `nav` reutilizan el contrato CLI-daemon existente; no son métodos LSP nuevos ni una puerta de transporte adicional.

- Root: `mi-lsp find_symbol <query> [--repo <id>]`, `mi-lsp rename_symbol <symbol> --new-name <identifier>`, `mi-lsp get_errors [file ...]`.
- Aliases: `mi-lsp nav find_symbol`, `mi-lsp nav rename_symbol` y `mi-lsp nav get_errors`; el provider manifest publica las mismas operaciones y sus schemas.
- `find_symbol` requiere `query`; `repo` es opcional y `--workspace` permanece en el contexto global. RPC `operation=find_symbol`, respuesta `find_symbol/v0`.
- `rename_symbol` requiere `symbol` y `new_name`; el símbolo puede ser un identificador Go o el selector opaco devuelto en `candidate.symbol`. La selección explícita alternativa requiere `file`, `line` y `column` juntos (línea 1-based, columna UTF-16 1-based); `repo`, `entrypoint`, `solution` y `project_path` seleccionan scope. RPC `operation=rename_symbol`, respuesta `rename_symbol/v0`; solo `gopls` entrega el plan semántico.
- `get_errors` admite paths posicionales o `--file` repetible; sin ellos usa archivos fuente cambiados. `--base` solo se admite con ese default y `--max-files` está entre 1 y 64. RPC `operation=get_errors`, respuesta `get_errors/v0`.
- Request: `protocol_version=mi-lsp-v1.1`, `operation`, `context` y `payload`. La operación se enruta igual desde root y `nav`; el daemon valida la versión antes de ejecutar.
- El rename usa `workspace/symbol` y `textDocument/prepareRename` para proponer/revalidar selección, y `textDocument/references` + `textDocument/rename` para planear. El core acepta solo `documentChanges` versionados, con fuentes/snapshots y referencias verificadas; paths y URIs no locales, `.git` y escapes por symlink fallan cerrado. Límite de aplicación: 32 archivos y 5000 edits; recuperación informa cualquier mutación parcial.
- `get_errors` y los diagnósticos posteriores al rename distinguen `checked`, `partial` y `not_checked`; `compilation_state` siempre es `not_checked`. Diagnósticos no certifican compilación.

### Ciclo CLI-daemon y deadlines

- **Marcador estable**: CT-DAEMON-WORKER.request-lifecycle
- **Comprobación prevista**:
  - go test ./internal/daemon/... ./internal/service/...
- **Evidencia referenciada**:
  - .docs/wiki/07_technical/contracts/CT-DAEMON-WORKER.md
  - internal/daemon/client.go
- **routing**:
  - **no_daemon**:
    - **flag**: --no-daemon
    - **Comportamiento**: no_connect_and_no_start
  - **no_auto_daemon**:
    - **flag**: --no-auto-daemon
    - **Comportamiento**: skip_auto_start_only
    - **existing_daemon**: connect_allowed
  - **execute**:
    - **method**: ExecuteWithDialTimeout
    - **short_timeout_scope**: dial_only
    - **after_dial**: original_context
    - **operations**:
      - write
      - read
      - response_processing
      - cancellation

`--no-daemon` fuerza la ejecución local y no conecta ni inicia el daemon. `--no-auto-daemon` solo omite el auto-start; si la operación usa daemon, aun puede conectar a una instancia existente. `ExecuteWithDialTimeout` deriva un contexto breve para establecer la conexion y lo libera al terminar el dial; write, read, procesamiento de respuesta y cancelacion vuelven a obedecer el contexto original de la request.

### Lock de arranque y frontera de observacion graph-native

- **Marcador estable**: CT-DAEMON-WORKER.start-lock-and-graph-observation
- **Evidencia referenciada**:
  - .docs/wiki/07_technical/contracts/CT-DAEMON-WORKER.md
  - internal/daemon/state_store.go
  - internal/daemon/start_lock.go
  - internal/daemon/start_lock_windows.go
  - internal/daemon/start_lock_unix.go
  - internal/daemon/process_liveness_windows.go
  - internal/service/graph_observer.go
  - internal/model/graph_observation.go
  - worker-dotnet/MiLsp.Worker/RoslynService.cs
- **Comprobación prevista**:
  - go test ./internal/daemon/... ./internal/service/... ./internal/model/... ./internal/indexer/...
  - dotnet run --project worker-dotnet/MiLsp.Worker.ContractTests/MiLsp.Worker.ContractTests.csproj
- **Condiciones de rechazo**:
  - live_owner_lock_reclaimed=true
  - unknown_metadata_lock_reclaimed=true
  - ambiguous_windows_liveness_reclaimed=true
  - start_guard_removed=true
  - start_lock_operation_outside_guard=true
  - lock_owner_file_replaced=true
  - noncanonical_batch_accepted=true
  - batch_not_ready_for_staging=true
- **start_lock**:
  - **guard**:
    - **path**: start.guard
    - **persistence**: persistent_never_removed
    - **os_exclusive_lock**:
      - **windows**: LockFileEx
      - **unix**: flock
    - **serializes**:
      - create
      - inspect
      - reclaim
      - Close
  - **create**:
    - **flags**: O_CREATE|O_EXCL|O_RDWR
    - **mode**: 384
    - **metadata**:
      - **version**: 1
      - **fields**:
        - pid
        - nonce
      - **descriptor_close**: after_versioned_metadata_sync
  - **pid**:
    - **valid_range**: 1..math.MaxInt32
  - **owner**:
    - **live**: preserve
    - **dead**: reclaim
    - **metadata_unknown**: preserve
    - **Errores**: fail_closed
    - **windows_liveness**:
      - **ERROR_INVALID_PARAMETER**: nonexistent
      - **ACCESS_DENIED**: alive
      - **ambiguous_error**: alive
      - **exit_code_error**: alive
  - **legacy_empty**:
    - **reclaim_only_after**: 5m
  - **close**:
    - **under_guard**: true
    - **remove_only_when_pid_and_nonce_match**: true
    - **replacement_with_different_metadata**: preserve
- **graph_observation**:
  - **worker_output**: canonical_unsealed_batch
  - **core_sequence**:
    - ValidateCanonical
    - SealGraphObservationBatch
    - ReadyForStaging

`start.guard` es persistente y nunca se elimina. Su lock exclusivo del sistema operativo (`LockFileEx` en Windows, `flock` en Unix) cubre toda operación de crear, inspeccionar, recuperar y `Close`; por ello no se libera el guard entre recuperar `start.lock` y crear su reemplazo. `start.lock` conserva `O_CREATE|O_EXCL`, se escribe con metadata versionada `pid+nonce`, se sincroniza y solo entonces se cierra el descriptor.

Un PID válido está en `1..math.MaxInt32`. Un owner vivo, una metadata desconocida o cualquier error de liveness ambiguo se preservan; en Windows, `ACCESS_DENIED` y todo error distinto de `ERROR_INVALID_PARAMETER` se tratan como owner vivo, y únicamente `ERROR_INVALID_PARAMETER` prueba inexistencia. Un lock legacy vacío solo se recupera cuando tiene más de cinco minutos. `Close` actúa bajo `start.guard` y elimina únicamente si coinciden el PID y el nonce que adquirieron el lock; los errores son fail-closed.

### Bootstrap y selección de runtime

- Para Roslyn, el caller debe resolver el `tool_root` desde el ejecutable/distribucion actual o, en desarrollo, desde el repo `mi-lsp`.
- Orden canónico de candidatos Roslyn para queries: `bundle -> installed -> dev-local`, resuelto por presencia de archivos y sin probe de compatibilidad en el hot path.
- Compatibilidad mínima se valida mediante probe `status` y comparacion de `protocol_version`; ese probe vive en `worker status` y diagnóstico explicito.
- Si el primer candidato Roslyn falla por bootstrap/arranque, el caller puede reintentar una sola vez con el siguiente candidato determinista antes de devolver error accionable.
- `gopls` es opcional y se resuelve primero desde `PATH`, luego desde `bin/gopls(.exe)` o `.bin/gopls(.exe)` dentro del repo; su ausencia no inválida el catálogo AST nativo de Go.
- Los procesos hijo no interactivos del worker y del bridge semántico deben usar la politica comun de proceso; en Windows eso significa `HideWindow + CREATE_NO_WINDOW`, y los procesos detached agregan `DETACHED_PROCESS`.
- El runtime pool debe respetar presupuesto de memoria/proceso observable por daemon: working set, private bytes, handles y cantidad de runtimes activos son parte del contrato de `daemon status`/`perf-smoke`.
- El worker instalado por RID vive en `~/.mi-lsp/workers/<rid>/`.
- Un repo de desarrollo no debe tratar `bin/workers/<rid>` como bundle de distribucion canónico para consultas regulares.

### Referencias de contexto
- [TECH-TS-BACKEND.md](../TECH-TS-BACKEND.md)
- [TECH-DEPENDENCY-HARDENING.md](../TECH-DEPENDENCY-HARDENING.md)
- [CT-CLI-DAEMON-ADMIN.md](CT-CLI-DAEMON-ADMIN.md)

### Kotlin experimental y transporte LSP

[[TECH-KOTLIN-BACKEND]] fija el contrato v0 del backend opcional `kotlin-language-server`: pin fwcd 1.3.13/compiler 2.1.0, opt-in que autoriza initialize/Gradle, módulo explícito, manifest local y handshake de versión. No descarga, instala ni calienta automáticamente. Kotlin Windows permanece unavailable hasta tener Job Objects; Unix es dueño de su grupo hijo.

`get_context`, `find_refs`, `find_definition` y `document_symbols` usan los providers **realmente anunciados** en initialize. Definition/symbols son interfaz worker, no nuevos comandos CLI. Las respuestas Kotlin K1 son parciales sin certificado de classpath/JDK; symbols PSI son estructura extracted, no RoleCompiler ni NodeKey. Calls siguen unsupported: K2 no exporta resolved calls. Raw v2 agrega inventario compiler y mapping Kotlin aditivo; el batch actual partial/experimental no es stageable. Calls unsupported permanece visible fuera del batch; ver [[ADR-KOTLIN-GRAPH-ADMISSION-V1]].

- **Marcador estable**: CT-DAEMON-WORKER.kotlin-lsp-v0
- **Estado del comportamiento**: experimental_unverified
- **Evidencia referenciada**:
  - internal/worker/lsp_transport.go
  - internal/worker/lsp_process_unix.go
  - internal/worker/lsp_client.go
  - internal/daemon/lifecycle.go
- **Comprobación prevista**:
  - .docs/wiki/06_tests/TP-QRY.md::TP-QRY.kotlin-runtime-v0
- **framing**: JSON_RPC_2_Content_Length
- **bounds**:
  - **headers_bytes**: 8192
  - **body_bytes**: 8388608
  - **response_queue**: 16
- **context**: original_request_initialize_query_write_read_and_backoff
- **cancel**: cancelRequest_by_ID_then_invalidate_process_generation
- **shutdown_budget**: 1s
- **documents**: didOpen_once_didChange_increasing_versions_didClose_on_eviction_and_close
- **position_encoding**: utf-16
- **pool_key**: backend_absolute_module_root_explicit_entrypoint_config_fingerprint
- **restart**: subsequent_caller_only_no_blind_retry
- **unsupported**: no_textual_semantic_fallback
- **roslyn_framing**: unchanged_length_prefix

La asignación del pool ya no ejecuta initialize LSP bajo el mutex del manager: lo hace `Call(ctx)`. El lector pertenece a una generación del proceso y no retiene el mutex de Close. Gopls/Pyright/Rust conservan el bridge y las inferencias legacy de ancla, convirtiendo sus byte offsets a UTF-16; Kotlin exige posiciones explícitas. Requests del servidor no soportadas se rechazan, nunca autorizan mutaciones.

#### Fork Kotlin compiler-only v1

- **Marcador estable**: CT-DAEMON-WORKER.kotlin-compiler-export-v1
- **Estado del comportamiento**: experimental_unverified
- **owner**: Semantic-backends
- **Evidencia referenciada**:
  - worker-kotlin/upstream.patch
  - worker-kotlin/build.sh
  - internal/worker/kotlin_observation.go
  - internal/indexer/extractor_kotlin.go
- **Comprobación prevista**:
  - .docs/wiki/06_tests/TP-GPH.md::TP-GPH.kotlin-compiler-export-v1
- **framing**: existing_LSP_JSON_RPC_Content_Length
- **initialize_option**: miLspCompilerOnly_true
- **serverInfo_version**: 1.3.13+mi-lsp.obs2
- **extension_version**: mi-lsp-compiler-observations/v2
- **method**: kotlin/miLsp/compilerObservations/v2
- **capability**: experimental.miLspCompilerObservationsV2_true
- **params**:
  - module_root
  - configuration_path
  - configuration_sha256
- **configuration_path**: selected_absolute_module/.mi-lsp/kotlin-main-v1.json
- **authority_receipt**: selected_absolute_module/.mi-lsp/kotlin-main-v1.authority.json
- **authority**: gradle-compileClasspath-resolvedArtifacts/v1
- **exporter**: MI_LSP_KOTLIN_EXPORT_MAIN_JVM=1_bash_worker-kotlin/config/export-main-jvm.sh
- **capture**: explicit_offline_no_daemon_no_bootstrap_not_run_in_BUILD
- **invalidation**: source_build_JAR_JDK_config_exporter_digest
- **promotion**: KotlinPromotionAllowed_false_until_six_RF-GPH-008_oracles
- **result_schema**: raw_export_v2_not_Graph_schema
- **worker_method**: graph_observe
- **envelope**: items[0].compiler_observation
- **partial**: always_true
- **calls**: unsupported
- **stock_server**: fails_closed_no_extension
- **pool**: existing_owner_with_compiler_only_mode_and_resolved_configuration_fingerprint
- **cancel_deadline_crash**: discard_entire_export_and_invalidate_generation
- **worker_SQL_generation_publication**: forbidden
- **adapter_gate**: canonical_unsealed_subset_then_strict_ReadyForStaging
- **distribution**: explicit_build_only_manifest_launcher_source_patch_and_file_hashes_licenses

El flag `--miLspCompilerOnly` del mismo launcher selecciona `MiLspCompilerServer` en su JVM, sin construir los servicios stock DB/Gradle/index. Initialize exige `miLspCompilerOnly=true` y anuncia únicamente esa capability; no hace setup DB, resolver Gradle ni indexación stock. La extensión crea un compilador aislado en el **mismo JVM** por request, usa classpath/source/module/JDK/target explícitos y exporta los slices BindingContext reales, sin backup. Conserva lifecycle, ownership y reader K1; no abre otro subprocess ni cachea resultados. Las consultas K1 no compiler-only conservan sus límites previos. Stock no finge exponer el método; Windows sigue unavailable. Build/install/version/licencias y la resolución externa del descriptor son dependencias propuestas, no acreditadas por este documento.

El adapter raw v2 rechaza provenance incompleta, descriptor sin recibo del exporter pineado, backup classpath y stdlib-only. Conserva partial/experimental y produce batch canónico sin sello. El core valida→rebasa→sella→Ready; devuelve backend_partial con `satisfy_rf_gph_008_promotion_oracles` sin publicar. Calls unsupported queda visible en status. Una respuesta vacía, cancelada, de otro módulo o con cero diagnostics no se convierte en batch listo.

## Errores

Se mantienen las razones, warnings, rechazos y fallbacks explícitos de cada operación.

### Payload, error y compatibilidad

- Los payloads son siempre derivados; no enviar ASTs ni blobs completos por defecto.
- Toda respuesta debe poder incluir:
  - `warnings`
  - `stats`
  - `backend`
- Los errores del worker deben mapearse a mensajes accionables para el CLI.
- Si `tsserver` falla o no existe, el caller debe poder degradar a catalog/text con warning.
- Si `pyright` falla o no existe, el caller debe poder degradar a catalog/text con warning.
- Si `gopls` falla o no existe, el caller debe conservar el catálogo AST nativo de Go y degradar a catalog/text con warning.
- Si Roslyn falla en `get_context` y el archivo existe, el caller debe preservar `slice_text` y degradar a `catalog` o `text` con warning accionable.
- Si Roslyn falla en operaciones sin fallback util, como `find_refs` o `get_deps`, el caller debe devolver error tipado y accionable.
- Si el SLO de memoria/proceso se excede durante `perf-smoke`, el envelope debe ser `ok=false` con `error.kind=daemon` o `backend_runtime`, `error.code` estable y warning accionable; no debe esconderse como warning benigno.

## Compatibilidad

La migración cambia el contrato documental, no el protocolo runtime ni sus versiones. Los esquemas legacy que la herramienta consume continúan como compatibilidad de entrada, no como formato de autoría de esta wiki.

### Versioning y deprecacion

- El protocolo CLI-daemon esta versionado por `protocol_version`.
- El protocolo daemon-worker también transporta `protocol_version` por request para cortar incompatibilidades temprano, aun sin handshake separado de sesion.
- Agregar campos es compatible si el receptor ignora desconocidos.
- Cambiar framing o envelopes base requiere actualizar la documentacion de contrato y el parser correspondiente.
