---
doc_id: TECH-KOTLIN-BACKEND
title: Runtime Kotlin opcional y frontera compiler-backed
layer: TECH
status: experimental
---

# TECH-KOTLIN-BACKEND

```yaml
harness_protocol: SDD-HARNESS-v1
id: TECH-KOTLIN-BACKEND
kind: support-doc
audience: llm-first
imports:
  - '[[00_gobierno_documental]]'
  - '[[CT-DAEMON-WORKER]]'
  - '[[TECH-GRAPH-NATIVE]]'
exports:
  - TECH-KOTLIN-BACKEND
agent_must_read:
  - .docs/wiki/07_tech/TECH-KOTLIN-BACKEND.md
  - .docs/wiki/09_contratos/CT-DAEMON-WORKER.md
agent_may_edit:
  - .docs/wiki/07_tech/TECH-KOTLIN-BACKEND.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav wiki validate-harness --workspace <alias> --paths .docs/wiki/07_tech/TECH-KOTLIN-BACKEND.md --format toon
stop_if:
  - harness_verdict=BLOCKED
evidence:
  - .docs/wiki/07_tech/TECH-KOTLIN-BACKEND.md
  - .docs/wiki/06_pruebas/TP-QRY.md
```

K1 implementa el puente stdio hacia **fwcd/kotlin-language-server 1.3.13**, no un compilador embebido ni un grafo construido desde texto. Es una autoría sin verificación: runtime, JDK y fixture no están acreditados. Kotlin permanece `explicit_incomplete`; no se modifica el orden stable de [[RF-GPH-008]].

```toon
harness_protocol: SDD-HARNESS-v1
source_protocol: SDD-WIKI-SOURCE-v1
doc_id: TECH-KOTLIN-BACKEND
block_id: TECH-KOTLIN-BACKEND.runtime-v0
kind: backend-contract
source_of_truth: this
status: experimental_unverified
owner: Semantic-backends
imports:
  - .docs/wiki/07_baseline_tecnica.md
  - .docs/wiki/09_contratos/CT-DAEMON-WORKER.md
  - .docs/wiki/04_RF/RF-QRY-002.md
  - .docs/wiki/04_RF/RF-GPH-008.md
  - .docs/wiki/07_tech/TECH-GRAPH-NATIVE.md
exports:
  - kotlin_runtime_v0
  - kotlin_extension_transport_seam
evidence:
  - internal/worker/kotlin.go
  - internal/worker/lsp_client.go
  - internal/worker/lsp_transport.go
  - internal/worker/lsp_protocol.go
  - internal/worker/lsp_queries.go
  - internal/service/semantic_kotlin.go
  - internal/service/context.go
  - internal/daemon/lifecycle.go
verify:
  - .docs/wiki/06_pruebas/TP-QRY.md::TP-QRY.kotlin-runtime-v0
runtime:
  backend: kotlin-language-server
  version: 1.3.13
  upstream_commit: f532bcc9c6578f5a35009a8b5f281f93d4d131ba
  compiler_dependency: 2.1.0
  platform_v0: unix_owned_process_group
  windows: unavailable_until_owned_Job_Object
  discovery: bounded_local_metadata_only
  installation: operator_managed_local_distribution
  download_bootstrap: forbidden
  automatic_warm: none
queries:
  get_context: textDocument/hover
  find_refs: textDocument/references
  find_definition: textDocument/definition
  document_symbols: textDocument/documentSymbol
  capability_gate: actual_initialize_capabilities
  symbols_authority: PSI_structural_extracted
  semantic_coverage: always_partial_in_K1
  calls: unsupported_no_resolved_call_export
  currentgraph: gated_Graph_v1_registry_no_Kotlin_admission
  textual_semantic_fallback: forbidden
```

## Configuración y autorización

1. `MI_LSP_KOTLIN_ENABLE=1` habilita **la ejecución posterior** del LS: initialize puede ejecutar wrapper/Gradle, scripts de build y acceso a red. No es readonly ni una autorización genérica para proyectos externos.
2. `MI_LSP_KOTLIN_LANGUAGE_SERVER_PATH` debe ser una ruta absoluta a un launcher local versionado. No se busca en PATH ni se instala o descarga nada.
3. El sidecar `<launcher>.mi-lsp.json` declara `server_version=1.3.13`, `compiler_version=2.1.0`, `upstream_commit=f532bcc9c6578f5a35009a8b5f281f93d4d131ba` y `launcher_sha256` (hexadecimal SHA-256 del launcher). Cada lectura está limitada a 2 MiB. Es una declaración del empaquetador local: no certifica las JAR transitivas ni el JDK. Initialize debe devolver además la versión exacta del manifest; stock `1.3.13` o fork explícito `1.3.13+mi-lsp.obs2`. Ausente o distinto rechaza el runtime. El fork no se presenta como stock.
4. Se requiere `--entrypoint <id|path>` declarado en topología, que seleccione `build.gradle` o `build.gradle.kts` de un módulo explícito con `src/main`. No se elige automáticamente entre módulos ni por el nombre del símbolo. `.kt` se reconoce; `.kts` no es fuente semántica. Los archivos de build/settings/wrapper solo son metadata durante discovery.
5. `--file`, `--line` y `--column` (columna **1-based UTF-16**) son explícitos para refs; context recibe archivo/línea y requiere `--column`. Se rechaza una columna fuera de línea o dentro de un surrogate pair. El archivo real debe estar bajo `src/main` del módulo, sin escape por symlink. Esta restricción de entrada **no acredita** que upstream haya aislado el source-set compilado: K2 deberá probarlo.

Formas de uso posteriores a la instalación/autorización local, **no ejecutadas en K1**:

```text
mi-lsp nav refs Nombre --file src/main/kotlin/Ejemplo.kt --line 8 --column 5 --entrypoint build.gradle.kts
mi-lsp nav context src/main/kotlin/Ejemplo.kt 8 --column 5 --entrypoint build.gradle.kts
```

`--backend kotlin-language-server` se acepta como selector explícito. Definition y symbols son métodos del worker (`find_definition`, `document_symbols`), no comandos CLI nuevos ni reemplazos silenciosos de `nav.find`/`nav.symbols`. Estos últimos conservan el catálogo existente.

## Estados y límites de precisión

Missing, disabled, manifest incompatible, entrypoint inválido, timeout y crash devuelven `ok=false`, `reason=unavailable`, sin regex/text fallback. Unsupported devuelve `unsupported_operation`. Respuestas LSP válidas se mantienen `ok=true`, `partial=true`, `reason=explicit_incomplete`; incluso una lista vacía **no prueba** completitud. No se agregan callers de catálogo a refs Kotlin.

El initialize original no expone un certificado de classpath completo y puede usar backup resolver. Por eso K1 no distingue éxito real de resolución parcial: conserva todas las consultas como parciales hasta que K2 aporte observación estricta. JDK home/target, Java 25, Gradle/AGP, Android, multiplatform, scripts, variantes, configuración y JARs efectivos **no están validados**. La toolchain 11 declarada upstream no prueba compatibilidad con Java 25.

## Lifecycle

- La inicialización LSP sale de la asignación del pool y ocurre dentro de `Call(ctx)`. La espera del lock, initialize, consultas, escrituras y backoffs respetan el contexto original. Kotlin refs aplica el presupuesto `MI_LSP_REFS_TIMEOUT`; context usa máximo 30 s, siempre limitado por el caller. No se desacopla un warmup Kotlin.
- Un lector por generación consume framing Content-Length; headers ≤8 KiB, cuerpo ≤8 MiB, cola de respuestas ≤16. Notificaciones no llenan la cola; IDs no esperados se descartan. Requests del servidor no soportadas reciben error JSON-RPC `-32601`, sin aceptar mutaciones del cliente.
- Cancelación envía `$/cancelRequest` con ID con hasta 100 ms para despacho, luego invalida/termina la generación. Fallo de escritura también corta el transporte cuando vence el contexto. No se mezclan respuestas tardías con la consulta siguiente.
- El launcher Kotlin recibe `GRADLE_OPTS` con `-Dorg.gradle.daemon=false` para evitar reutilizar un daemon Gradle compartido. Los scripts de build pueden crear procesos fuera del grupo: no se presume aislamiento frente a código arbitrario ni se mata por nombre/PID ajeno. La fixture posterior debe demostrar el cierre del árbol real.
- Cada proceso Unix tiene su grupo propio: cierre y crash terminan solo ese grupo hijo. No se corta ningún LS compartido ni sesión ajena. Kotlin Windows falla cerrado hasta implementar Job Objects. El cambio genérico conserva la política Windows no interactiva de los otros backends.
- Shutdown/didClose/exit tiene presupuesto conjunto de 1 s. Si una llamada retiene el lock, Close puede cerrar los pipes y el proceso sin esperar la lectura. `cmd.Wait` tiene dueño único. El siguiente caller puede crear una generación nueva; no hay retry ciego dentro de la consulta fallida. El daemon elimina el cliente Kotlin con error del pool.
- `didOpen` ocurre una vez; cambios completos válidos llevan `didChange` con versión creciente. Se compara contenido, no solo mtime/tamaño. Límite: 32 documentos de hasta 2 MiB; eviction y shutdown envían `didClose`.
- Pool Kotlin: backend + raíz absoluta del módulo + entrypoint real + fingerprint. Fingerprint incluye launcher/sidecar, JAVA_HOME/opciones JVM/Gradle y metadata local de build/settings/properties/wrapper/version catalog hasta la raíz del repo. No equivale a fingerprint del classpath resuelto; K2 debe extenderlo. No hay caché de respuestas semánticas K1.
- Roslyn conserva su framing length-prefix; no pasa por este parser. tsserver conserva su transporte propio.

## Interfaz exacta para K2

```text
NewKotlinClient(workspace model.WorkspaceRegistration, request model.WorkerRequest) (*KotlinClient, error)
KotlinRuntimeConfiguration(workspace model.WorkspaceRegistration, request model.WorkerRequest) (KotlinRuntimeConfig, error)
(*KotlinClient).Call(ctx context.Context, request model.WorkerRequest) (model.WorkerResponse, error)
(*KotlinClient).RequestExtension(ctx context.Context, method string, requiredCapability string, params any) (json.RawMessage, error)
```

`KotlinClient.Runtime` contiene `Launcher`, `ModuleRoot`, `Entrypoint`, `Fingerprint`, `Manifest`. `KotlinClient` embebe `*LSPClient` y cumple `RuntimeClient` (`Call`, `Close`, `PID`). `RequestExtension` solo admite namespace `kotlin/` y exige `initialize.capabilities.experimental[requiredCapability] == true`. **No declara un método compiler universal ni inventa una respuesta**. K2 debe fijar su propio método/capability versionados en el worker y adapter, validar descriptors/firma/receiver/módulo/config/resolution y provenance, y solo entonces construir `GraphObservationBatch`. Stock fwcd no anuncia ese contrato: falla cerrado.

K2 conserva `ReadyForStaging` estricto y ownership Semantic-backends → Graph-Kernel → Store. K2 entrega la extensión fuente y el adaptador raw descritos abajo, no admisión de Kotlin al registro Graph v1. PSI/nombres/ranges nunca generan NodeKey ni `RoleCompiler`. K1 no toca Store, generaciones, B/C ni observation adapters.

## Fuentes y licencia

Pin público: `https://github.com/fwcd/kotlin-language-server/tree/f532bcc9c6578f5a35009a8b5f281f93d4d131ba`.

- `KotlinLanguageServer.kt`: initialize anuncia providers, refresca classpath y devuelve VERSION; no anuncia call hierarchy.
- `GradleClassPathResolver.kt`: invoca wrapper/Gradle con init scripts para resolver dependencias.
- `Compiler.kt` y `references/FindReferences.kt`: BindingContext/descriptors y filtrado de referencias del compilador.
- `symbols/Symbols.kt`: PSI, no identidad compiler exportada.
- `gradle/libs.versions.toml`: dependencia compiler 2.1.0.

Licencia del proyecto upstream: MIT (`LICENSE.txt`); conservar su aviso al distribuir. Las transitivas requieren su inventario (`server/src/main/dist/licenseReport.html`); no se atribuye MIT universal ni se empaqueta el runtime en esta autoría. Sin servicio externo ni tier pago.

## K2 — extensión fuente compiler-backed y admisión cerrada

El código fuente está en `worker-kotlin/`: servicio Kotlin, patch mínimo sobre el pin, script de build **explícito** offline, hashes de fuentes consultadas, licencia MIT fwcd íntegra e inventario transitivo original. La línea preview documental `52964ee` alinea pin init/exporter en Go y Kotlin (**SOURCE-ONLY**); recheck1 histórico compiló fork y exportó recibo K08 en sandbox pero no observó `compilerObservations/v2` ni PASS de grafo. No se atribuye MIT universal ni admisión runtime. [[TP-GPH]] contiene el oráculo futuro, sin PASS.

```toon
doc_id: TECH-KOTLIN-BACKEND
block_id: TECH-KOTLIN-BACKEND.compiler-export-v1
kind: backend-contract
source_of_truth: this
status: experimental_unverified_explicit_incomplete
owner: Semantic-backends
imports: [RF-GPH-008, TECH-GRAPH-NATIVE, CT-DAEMON-WORKER, TP-GPH, TP-QRY]
evidence:
  - worker-kotlin/README.md
  - worker-kotlin/upstream.patch
  - worker-kotlin/src/main/kotlin/org/javacs/kt/MiLspCompilerObservations.kt
  - internal/worker/kotlin_observation.go
  - internal/indexer/extractor_kotlin.go
  - internal/service/graph_observer.go
verify:
  - .docs/wiki/06_pruebas/TP-GPH.md::TP-GPH.kotlin-compiler-export-v1
fork:
  server_version: 1.3.13+mi-lsp.obs2
  extension_version: mi-lsp-compiler-observations/v2
  upstream_commit: f532bcc9c6578f5a35009a8b5f281f93d4d131ba
  compiler_version: 2.1.0
  gradle_build: 8.10
  build_jdk_toolchain: 11
protocol:
  initialize_option: miLspCompilerOnly
  method: kotlin/miLsp/compilerObservations/v2
  capability: initialize.capabilities.experimental.miLspCompilerObservationsV2
  response_schema: raw_export_v2_not_GraphObservationBatch
  worker_envelope: items[0].compiler_observation
  stock_extension: unsupported
compiler:
  APIs: [Compiler.compileKtFiles, BindingContext.DECLARATION_TO_DESCRIPTOR, BindingContext.REFERENCE_TARGET, ModuleDescriptor]
  identity: compiler_descriptor_chain_signature_and_FQ_types_no_PSI_text_or_ranges
  receiver: dispatch_and_extension_from_CallableDescriptor
  calls: unsupported_no_resolved_call_export
  source_set: explicit_main_jvm_single_module
  configuration: absolute_module/.mi-lsp/kotlin-main-v1.json
  authority_receipt: absolute_module/.mi-lsp/kotlin-main-v1.authority.json
  authority: gradle-compileClasspath-resolvedArtifacts/v1
  exporter: worker-kotlin/config/export-main-jvm.sh
  exporter_sha256: 97fd0a079a12cb921c21404de8415fcdbc7915168fb7d1940836fbbc3579b1b3
  init_script_sha256: ebccdf78fdbf0d56668b924d88452e89215763b761cefdb08964facabf5ed3a5
  capture: explicit_opt_in_offline_no_bootstrap
  classpath: resolvedArtifacts_hashed_JARs_no_backup_or_stdlib_only
  response_completeness: partial_until_promotion_oracles
  promotion_gate: KotlinPromotionAllowed_false_this_revision
  java_declarations: not_exported
  scripts_AGP_multiplatform_variants_frontend_K2_universal: unsupported
admission:
  Graph_v1_enums: exact_additive_Kotlin_backend_resolution
  Kotlin_NodeKey: existing_tuple_descriptor_original_module_owner
  batch_staging: complete_stable_only_current_raw_partial_rejected
  decision: ADR-KOTLIN-GRAPH-ADMISSION-V1
  pipeline_omission: backend_partial
  resolution_owner: coord-milsp_and_Graph-Kernel
  decision_due: 2026-10-17
runtime_matrix: {unix: experimental_owned_group, windows: unavailable_JobObjects_missing}
```

Build futuro: `bash worker-kotlin/build.sh /ruta/checkout-fwcd-f532bcc9 /ruta/absoluta/salida-nueva`. Dependencias preinstaladas/cacheadas: Bash/Git/tar/Python, Gradle 8.10 y JDK 11. El script no hace download ni install global; produce launcher, sidecar, hashes de distribución/patch/source y licencias/provenance. El sidecar y handshake usan la versión del fork. Las dependencias directas y el inventario transitivo se detallan en `worker-kotlin/README.md`, `upstream-inputs.json` y `licenses/upstream-licenseReport.html`.

El descriptor JSON v1 sigue en schema 1: `resolution=resolved`, `source_set=main-jvm` y listas con path absoluto más SHA-256. Ya no es autoridad por sí solo. El recibo hermano lo emite solo el exporter explícito, fuera de BUILD, con `MI_LSP_KOTLIN_EXPORT_MAIN_JVM=1 bash worker-kotlin/config/export-main-jvm.sh /modulo/absoluto`. Exige Gradle 8.10 ya instalado, `--offline --no-daemon --no-build-cache`, sin wrapper ni descarga. Captura toolchain JDK 11, `compileKotlin.jvmTarget`, sources main, `compileClasspath.resolvedArtifacts` y build inputs. Rechaza AGP, multiplatform, scripts, classpath no JAR, stdlib-only y pin distinto. El recibo ata `evidence_digest` a esos inputs. Loader, compiler y adapter recomputan ese digest y los hashes del exporter pineado. Cambiar source, build, JAR, JDK, config o exporter invalida. Cero diagnostics no certifica. CRLF queda unsupported en el export v1.

El mismo launcher recibe `--miLspCompilerOnly` y selecciona el endpoint mínimo `MiLspCompilerServer` en su JVM, sin construir los servicios stock DB/Gradle/index. Su initialize exige el opt-in compiler-only y no hace DB setup, Gradle resolver ni indexación upstream. Observaciones sin caché, recompiladas íntegramente en el mismo JVM/proceso supervisado; no subprocess paralelo, SQL ni publicación desde la extensión. La llave incluye modo compiler-only/config resuelto/distribución. Cambios de inputs invalidan la respuesta; deadline/cancel/crash conserva el lifecycle K1 y descarta el resultado completo.

`Call(status)` informa metadata configurada no verificada, calls unsupported y graph_stageable=false; no inicia LS ni acredita conformance. Fuentes Java requieren el root convencional src/main/java; roots Java no estándar se rechazan explícitamente. El adapter v2 produce batch canónico sin sello usando NewNodeKey-v1 y el registro exacto de [[ADR-KOTLIN-GRAPH-ADMISSION-V1]]. El join exige identidad original + compiler module + owner local inequívoco. Targets externos/Java/no representables son omisiones, nunca nodos inventados. La cobertura de dominio sigue el scan real del compiler; si el recibo no certifica, no hay batch. Con recibo válido el batch sigue partial y suma `promotion_oracle_missing`, más `compiler_analysis_incomplete` si hay diagnostics. Calls no entra al batch. El core valida, rebasa, sella y exige ReadyForStaging. Publicar exige los seis oráculos de [[RF-GPH-008]]; esta revisión no los consume. Owner coord-milsp, 2026-10-17.
