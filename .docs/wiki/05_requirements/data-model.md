---
wiki_contract: WIKI-CONTRACT-v1
id: data-model
kind: data_model
status: active
title: 5. Modelo de datos
---

## Entidades

Las entidades, estados e identidades se detallan a continuación.

### Propósito

`mi-lsp` modela estado operativo local, no un dominio de negocio tradicional.
La base vigente distingue workspaces `single` de workspaces `container`, persiste ownership por repo/entrypoint y sostiene un grafo documental repo-local. La evolucion graph-native agrega identidad durable, generations inmutables, adjacency con evidencia y extensiones derivativas, sin reemplazar la autoridad wiki ni requerir servicios externos.

### Entidades canónicas

- **Entidad**: WorkspaceRegistration; **Tipo**: Operativa; **Owner**: Core runtime; **Persistencia**: `~/.mi-lsp/registry.toml`; **Descripción**: Alias, root, languages, `kind`, `CanonLinks` (`&#91;&#91;canon_link&#93;&#93;` alias+role vía `workspace link`) y compatibilidad legacy
- **Entidad**: ProjectConfig; **Tipo**: Operativa; **Owner**: Workspace owner; **Persistencia**: `<repo>/.mi-lsp/project.toml`; **Descripción**: Nombre local, ignores, `repos`, `entrypoints`, defaults, `&#91;&#91;canon&#93;&#93;` (`id`, `root`, `role`, `mode`), `[canon_policy]` (`escape_max`), `[embeddings]` (`provider`, `base_url`, `model`, `dim`, `api_key_env`, `profile`, `batch_size`, `timeout_ms`, `encoding_format`, `user_agent`) y `[recall.rerank_extension]` (`enabled`, `command`, `args`, `timeout_ms`, `candidate_count`, `top_n`, `max_snippet_chars`); alias semántico de `ProjectFile` en código Go
- **Entidad**: WorkspaceRepo; **Tipo**: Operativa derivada; **Owner**: Core runtime; **Persistencia**: `<repo>/.mi-lsp/project.toml`; **Descripción**: Repo hijo reconocido dentro de un workspace `container`
- **Entidad**: WorkspaceEntrypoint; **Tipo**: Operativa derivada; **Owner**: Core runtime; **Persistencia**: `<repo>/.mi-lsp/project.toml`; **Descripción**: `.sln` o `.csproj` semanticamente enrutable
- **Entidad**: SymbolRecord; **Tipo**: Derivada; **Owner**: Indexer; **Persistencia**: `<repo>/.mi-lsp/index.db`; **Descripción**: Declaracion liviana con `repo_id` y `repo`
- **Entidad**: FileRecord; **Tipo**: Derivada; **Owner**: Indexer; **Persistencia**: `<repo>/.mi-lsp/index.db`; **Descripción**: Metadata de archivo indexado con ownership por repo
- **Entidad**: DocRecord; **Tipo**: Derivada; **Owner**: Doc indexer; **Persistencia**: `<repo>/.mi-lsp/index.db`; **Descripción**: Documento indexado con `path`, `doc_id`, `layer`, `family` y texto de ranking
- **Entidad**: DocEdge; **Tipo**: Derivada; **Owner**: Doc indexer; **Persistencia**: `<repo>/.mi-lsp/index.db`; **Descripción**: Relacion explícita documento -> documento por doc ID, link markdown, wikilink, embed u hierarchy
- **Entidad**: DocMention; **Tipo**: Derivada; **Owner**: Doc indexer; **Persistencia**: `<repo>/.mi-lsp/index.db`; **Descripción**: Mención explícita desde docs hacia paths, símbolos, comandos o IDs documentales; conserva `source_block` opcional y las referencias `TP-*` pueden resolver un documento exacto
- **Entidad**: DocSourceBlock; **Tipo**: Derivada; **Owner**: Doc indexer; **Persistencia**: `<repo>/.mi-lsp/index.db`; **Descripción**: Bloque `toon` normativo de un artefacto `SDD-WIKI-SOURCE-v1`, con `block_id`, `doc_id`, líneas y hash
- **Entidad**: DocSourceRecord; **Tipo**: Derivada; **Owner**: Doc indexer; **Persistencia**: `<repo>/.mi-lsp/index.db`; **Descripción**: Record referenciable dentro de un bloque fuente, con `record_id`, `record_type`, líneas y hash
- **Entidad**: WikiChunkEmbedding; **Tipo**: Derivada; **Owner**: Doc indexer; **Persistencia**: `<repo>/.mi-lsp/index.db`; **Descripción**: Chunk wiki enriquecido con metadata, hash de contenido/prefix, BLOB float32, `embedding_model`, `embedding_dim`, heading, snippet y rango de líneas
- **Entidad**: GovernanceSource; **Tipo**: Operativa local; **Owner**: Maintainer de wiki; **Persistencia**: `<repo>/.docs/wiki/00_governance.md`; **Descripción**: Bloque YAML fuente que define perfil, jerarquia, cadenas y reglas de bloqueo
- **Entidad**: GovernanceStatus; **Tipo**: Derivada; **Owner**: CLI/Core; **Persistencia**: Respuesta en memoria; **Descripción**: Estado efectivo de gobernanza: perfil, sync, bloqueos, overlays y pasos de reparación
- **Entidad**: DocsReadProfile; **Tipo**: Operativa local; **Owner**: Maintainer de wiki; **Persistencia**: `<repo>/.docs/wiki/_mi-lsp/read-model.toml`; **Descripción**: Perfil opcional que clasifica familias, paths y fallback documental
- **Entidad**: DocsOwnerHint; **Tipo**: Operativa local; **Owner**: Maintainer de wiki; **Persistencia**: `<repo>/.docs/wiki/00_governance.md` -> `read-model.toml`; **Descripción**: Hint opcional repo-especifico para ownership documental de capabilities nuevas
- **Entidad**: DocsGovernanceProfile; **Tipo**: Operativa derivada; **Owner**: CLI/Core; **Persistencia**: `<repo>/.docs/wiki/_mi-lsp/read-model.toml`; **Descripción**: Proyección ejecutable de la gobernanza humana: perfil efectivo, base, overlays y cadenas
- **Entidad**: WorkspaceMeta; **Tipo**: Derivada; **Owner**: Indexer; **Persistencia**: `<repo>/.mi-lsp/index.db`; **Descripción**: Totales, defaults, schema y punteros activos del índice
- **Entidad**: GraphGeneration; **Tipo**: Derivada inmutable; **Owner**: Graph Kernel; **Persistencia**: `<repo>/.mi-lsp/index.db`; **Descripción**: Snapshot sellado por source/config/backend digests, con estado staged/active/retired/invalid
- **Entidad**: GraphNode; **Tipo**: Derivada; **Owner**: Graph Kernel; **Persistencia**: `<repo>/.mi-lsp/index.db`; **Descripción**: Nodo tipado con `node_id` local, `NodeKey` BLOB(32), identity fields, owner, generation, provenance y cross-RID
- **Entidad**: GraphEdge; **Tipo**: Derivada; **Owner**: Graph Kernel; **Persistencia**: `<repo>/.mi-lsp/index.db`; **Descripción**: Relacion dirigida y tipada entre NodeKeys existentes en la misma generation
- **Entidad**: GraphEvidence; **Tipo**: Derivada; **Owner**: Backend/Graph Kernel; **Persistencia**: `<repo>/.mi-lsp/index.db`; **Descripción**: Observacion versionada con source URI/range/digest, backend, claim, status, generation y cross-RID
- **Entidad**: GraphUnresolved; **Tipo**: Derivada; **Owner**: Backend/Validator; **Persistencia**: `<repo>/.mi-lsp/index.db`; **Descripción**: Identidad, edge o target no publicable con reason code, candidatos bounded y recovery hint; provenance documental nullable (`source_document`, `source_block`, `target_kind`, `target_value`), acotada y expuesta de forma sanitizada
- **Entidad**: GraphMigration; **Tipo**: Operativa derivada; **Owner**: SQLite Publisher; **Persistencia**: `<repo>/.mi-lsp/index.db`; **Descripción**: Ventana de schema, preflight, checksums, dual-read/write y rollback metadata
- **Entidad**: GraphAnalysis; **Tipo**: Derivada descartable; **Owner**: Core/MILX Host; **Persistencia**: `<repo>/.mi-lsp/index.db` o cache local; **Descripción**: Resultado de algoritmo/pack keyeado por generation, extension y parametros; nunca autoridad primaria
- **Entidad**: GlobalGraphSnapshot; **Tipo**: Derivada descartable; **Owner**: Core/Daemon opcional; **Persistencia**: Estado global local; **Descripción**: Vista sellada de member workspaces/generations y cross-edges; no escribe stores miembros
- **Entidad**: MILXManifest; **Tipo**: Operativa local; **Owner**: Extension owner/Core; **Persistencia**: Archivo de extension validado; **Descripción**: ID/version/digest, protocol, schemas, capabilities y resource hints
- **Entidad**: MILXExecution; **Tipo**: Historica local acotada; **Owner**: MILX Host; **Persistencia**: Telemetria/cache local; **Descripción**: request, generation, status, budgets, output digest y cleanup; sin payload arbitrario
- **Entidad**: ContextPack; **Tipo**: Derivada; **Owner**: Context Optimizer; **Persistencia**: Respuesta/cache; **Descripción**: Selección bounded de autoridad/evidencia con costo de tokens, coverage, omissions y digest
- **Entidad**: DaemonState; **Tipo**: Operativa; **Owner**: Runtime supervision; **Persistencia**: `~/.mi-lsp/daemon/state.json`; **Descripción**: PID, endpoint, admin URL y version/protocolo
- **Entidad**: DaemonRun; **Tipo**: Historica local; **Owner**: Runtime supervision; **Persistencia**: `~/.mi-lsp/daemon/daemon.db`; **Descripción**: Una corrida del daemon global
- **Entidad**: RuntimeSnapshot; **Tipo**: Derivada; **Owner**: Runtime supervision; **Persistencia**: `~/.mi-lsp/daemon/daemon.db`; **Descripción**: Estado de un runtime por `(workspace_root, backend, entrypoint)`
- **Entidad**: AccessEvent; **Tipo**: Historica local; **Owner**: Runtime supervision; **Persistencia**: `~/.mi-lsp/daemon/daemon.db`; **Descripción**: Acceso ejecutado con cliente, sesion, repo y entrypoint
- **Entidad**: QueryEnvelope; **Tipo**: Derivada; **Owner**: CLI/Core; **Persistencia**: Respuesta en memoria; **Descripción**: Envelope estable que ve el usuario o skill; mapea a `Envelope` en `internal/model/types.go`
- **Entidad**: AskResult; **Tipo**: Derivada; **Owner**: CLI/Core; **Persistencia**: Respuesta en memoria; **Descripción**: Resultado de `nav ask` con `summary`, `primary_doc`, evidencias, `why` y `next_queries`
- **Entidad**: RecallResult; **Tipo**: Derivada; **Owner**: CLI/Core; **Persistencia**: Respuesta en memoria; **Descripción**: Resultado de `nav recall` con `query`, `intent`, `archivo`, `heading`, `score`, `snippet`, rango de líneas y `why`
- **Entidad**: PackResult; **Tipo**: Derivada; **Owner**: CLI/Core; **Persistencia**: Respuesta en memoria; **Descripción**: Resultado de `nav pack` con familia, modo, documento primario, reading pack y siguientes pasos
- **Entidad**: PackDoc; **Tipo**: Derivada; **Owner**: CLI/Core; **Persistencia**: Respuesta en memoria; **Descripción**: Documento seleccionado dentro del reading pack con stage, targets y slice opcional
- **Entidad**: PackTarget; **Tipo**: Derivada; **Owner**: CLI/Core; **Persistencia**: Respuesta en memoria; **Descripción**: Heading/linea sugerida para orientar lectura compacta del documento
- **Entidad**: ServiceSurfaceSummary; **Tipo**: Derivada; **Owner**: Core/service exploration; **Persistencia**: Respuesta en memoria; **Descripción**: Resumen evidence-first de un path de servicio
- **Entidad**: MultiReadItem; **Tipo**: Derivada; **Owner**: CLI/Core; **Persistencia**: Respuesta en memoria; **Descripción**: Contenido de un rango de archivo leido en batch
- **Entidad**: BatchResult; **Tipo**: Derivada; **Owner**: CLI/Core; **Persistencia**: Respuesta en memoria; **Descripción**: Resultado de una operación individual dentro de un nav batch
- **Entidad**: SymbolNeighborhood; **Tipo**: Derivada; **Owner**: Core/service; **Persistencia**: Respuesta en memoria; **Descripción**: Vecindario de un símbolo: definicion, callers, implementors, tests
- **Entidad**: WorkspaceMapEntry; **Tipo**: Derivada; **Owner**: Core/service; **Persistencia**: Respuesta en memoria; **Descripción**: Mapa de repos, servicios, endpoints, consumers, publishers y dependencias
- **Entidad**: DiffContextResult; **Tipo**: Derivada; **Owner**: Core/service; **Persistencia**: Respuesta en memoria; **Descripción**: Símbolos cambiados en un diff git con analisis de impacto

### Datos tocados por RF

- **RF**: RF-WKS-001; **Entidades principales**: WorkspaceRegistration, ProjectConfig, WorkspaceRepo, WorkspaceEntrypoint
- **RF**: RF-WKS-002; **Entidades principales**: WorkspaceRegistration, ProjectConfig, SymbolRecord, FileRecord
- **RF**: RF-WKS-003; **Entidades principales**: WorkspaceRegistration, ProjectConfig, QueryEnvelope
- **RF**: RF-WKS-005; **Entidades principales**: GovernanceSource, GovernanceStatus, WorkspaceRegistration, QueryEnvelope
- **RF**: RF-WKS-008; **Entidades principales**: ProjectConfig, WorkspaceRegistration, GovernanceSource
- **RF**: RF-IDX-001; **Entidades principales**: SymbolRecord, FileRecord, DocRecord, DocEdge, DocMention, DocSourceBlock, DocSourceRecord, WorkspaceMeta
- **RF**: RF-IDX-002; **Entidades principales**: SymbolRecord, FileRecord, DocRecord, DocEdge, DocMention, DocSourceBlock, DocSourceRecord, WorkspaceMeta
- **RF**: RF-IDX-003; **Entidades principales**: GovernanceSource, DocsGovernanceProfile, DocsReadProfile, WorkspaceMeta
- **RF**: RF-GPH-001; **Entidades principales**: GraphNode, GraphUnresolved
- **RF**: RF-GPH-002; **Entidades principales**: GraphGeneration, GraphMigration, WorkspaceMeta
- **RF**: RF-GPH-003; **Entidades principales**: GraphNode, GraphEdge, GraphEvidence, GraphUnresolved
- **RF**: RF-GPH-004; **Entidades principales**: GraphGeneration, GraphNode, GraphEdge, GraphEvidence, GraphUnresolved
- **RF**: RF-GPH-005; **Entidades principales**: GraphGeneration, GraphNode, GraphEdge, GraphEvidence, QueryEnvelope
- **RF**: RF-GPH-006; **Entidades principales**: GraphGeneration, GraphEdge, GraphEvidence, GraphUnresolved, DiffContextResult
- **RF**: RF-GPH-007; **Entidades principales**: DocRecord, DocEdge, DocMention, GraphNode, GraphEdge, ContextPack
- **RF**: RF-GPH-008; **Entidades principales**: GraphNode, GraphEdge, GraphEvidence, GraphUnresolved
- **RF**: RF-GPH-009; **Entidades principales**: GlobalGraphSnapshot, GraphGeneration, GraphEdge
- **RF**: RF-GPH-010; **Entidades principales**: MILXManifest, MILXExecution, GraphAnalysis
- **RF**: RF-GPH-011; **Entidades principales**: ContextPack, GraphAnalysis, MILXManifest
- **RF**: RF-QRY-001; **Entidades principales**: QueryEnvelope
- **RF**: RF-QRY-002; **Entidades principales**: QueryEnvelope, AccessEvent, WorkspaceEntrypoint
- **RF**: RF-QRY-003; **Entidades principales**: QueryEnvelope, ServiceSurfaceSummary, SymbolRecord, FileRecord
- **RF**: RF-QRY-004; **Entidades principales**: MultiReadItem, QueryEnvelope
- **RF**: RF-QRY-005; **Entidades principales**: BatchResult, QueryEnvelope
- **RF**: RF-QRY-006; **Entidades principales**: SymbolNeighborhood, QueryEnvelope, SymbolRecord
- **RF**: RF-QRY-007; **Entidades principales**: WorkspaceMapEntry, QueryEnvelope, SymbolRecord
- **RF**: RF-QRY-008; **Entidades principales**: DiffContextResult, QueryEnvelope, SymbolRecord
- **RF**: RF-QRY-009; **Entidades principales**: QueryEnvelope, SymbolRecord
- **RF**: RF-QRY-010; **Entidades principales**: AskResult, DocRecord, DocEdge, DocMention, DocsReadProfile, QueryEnvelope
- **RF**: RF-QRY-011; **Entidades principales**: SymbolRecord, DocRecord, DocsOwnerHint, QueryEnvelope
- **RF**: RF-QRY-012; **Entidades principales**: PackResult, PackDoc, PackTarget, DocRecord, DocEdge, DocsReadProfile, QueryEnvelope
- **RF**: RF-QRY-013; **Entidades principales**: GovernanceStatus, DocsReadProfile, QueryEnvelope
- **RF**: RF-QRY-016; **Entidades principales**: WikiSearchResult, HarnessValidationResult, WikiSourceValidationResult, DocRecord, DocSourceBlock, DocSourceRecord, QueryEnvelope
- **RF**: RF-WIKI-007; **Entidades principales**: QueryEnvelope, ProjectConfig, WorkspaceRegistration
- **RF**: RF-CS-001; **Entidades principales**: QueryEnvelope, RuntimeSnapshot, WorkspaceEntrypoint
- **RF**: RF-DAE-001; **Entidades principales**: DaemonState
- **RF**: RF-DAE-002; **Entidades principales**: RuntimeSnapshot, AccessEvent, DaemonState
- **RF**: RF-DAE-003; **Entidades principales**: DaemonState
- **RF**: RF-DAE-004; **Entidades principales**: SymbolRecord, FileRecord

## Invariantes

Se preservan las invariantes semánticas y de ownership del modelo, independientes de las tablas físicas.

### Identidad graph-native

`NodeKey v1` es SHA-256 de la serializacion binaria versionada y length-prefixed de `{repository_identity, backend_type, language, project_or_module, owner_path, symbol_kind, semantic_identity}`. Strings usan UTF-8/NFC y paths repo-relative con `/`; root absoluto, timestamps y declaration ranges no participan. `node_id INTEGER` es solo surrogate SQLite. Missing fields, normalización no determinista o una colision hash con tupla distinta producen `GraphUnresolved` y bloquean la publicacion.

Los cross-RIDs de nodo/edge/evidence son representaciones versionadas derivadas de sus identidades canónicas, no del RID del binario ni del host. El mismo fixture/config/backend version debe producirlos byte-identical en los RIDs soportados.

### Estados operativos

#### Workspace

- `detected`: el root fue identificado como compatible
- `registered`: existe alias en `registry.toml`
- `indexed`: existe `.mi-lsp/index.db` valido
- `container`: el workspace agrupa muchos repos hijos y requiere routing semántico
- `docs_profiled`: existe `read-model.toml` propio o se usa el default embebido
- `governance_blocked`: la gobernanza esta inválida o el índice quedo stale respecto de `00`/`read-model`

#### Runtime

- `cold`: no existe runtime activo para el entrypoint pedido
- `active`: runtime vivo en el pool del daemon
- `evicted`: runtime removido por LRU o idle timeout
- `ambiguous`: no se pudo resolver repo/entrypoint de forma única

#### GraphGeneration

- `staged`: completa en escritura, aun invisible para readers
- `active`: snapshot único seleccionado por el puntero del workspace
- `retired`: snapshot validado anterior, retenido para rollback o reactivación canónica
- `invalid`: staging rechazado o incompleto; nunca consultable

#### Backend / claim

- `exact`: compiler/LSP resolvio identidad y relacion
- `extracted`: parser estructural versionado observo la forma
- `inferred`: regla derivativa explícita, nunca compiler fact
- `ambiguous` / `unresolved` / `unsupported`: no se publica claim positivo

### Invariantes

- `registry.toml` no contiene topología detallada del container.
- `project.toml` es la fuente local para `repo[]`, `entrypoint[]`, `default_repo` y `default_entrypoint`.
- `SymbolRecord`, `FileRecord` y `DocRecord` son reconstruibles y nunca persisten ASTs ni refs profundas.
- `DocMention` y `GraphUnresolved` son entidades derivadas, reconstruibles y read-only respecto de la wiki; exponen diagnóstico, no autoridad ni mutación del canon.
- `RuntimeSnapshot` y `AccessEvent` deben ser suficientes para explicar por que un acceso fue warm, cold o ambiguo.
- `QueryEnvelope` siempre incluye `backend`, `warnings`, `stats` y `truncated`; si hay ambigüedad, el `backend` canónico es `router`.
- `QueryEnvelope` puede agregar `mode` cuando la superficie publica distingue variantes estables (`nav.intent docs|code|mixed`).
- `QueryEnvelope` puede agregar `degraded`, `reason` (catálogo versionado `primitives-v2`: `index_not_ready`, `index_schema_broken`, `lsp_unavailable`, `lsp_error`, `semantic_empty_text_hits`, `language_unsupported`, `no_matches`; independiente de `error.reason_code`) y `fallback_used` (`catalog|text`); sus items pueden agregar `origin` (`semantic|catalog|text|wiki`). Son aditivos y no se persisten.
- `AskResult` nunca debe invertir prioridad: la wiki rankea primero y el código actua como evidencia o verificacion.
- `PackResult` debe preservar el orden canónico global -> especifico y no degradar silenciosamente a docs genericos cuando la wiki canónica existe pero el índice documental esta vacio.
- `ServiceSurfaceSummary` no persiste score de completitud ni conclusion final de auditoria.
- SQLite repo-local es la autoridad de adjacency; queries no materializan el grafo completo en RAM, no persisten closures transitivos y no migran schema.
- Readers fijan una sola `GraphGeneration`; observan el snapshot anterior o el nuevo completo, nunca una mezcla.
- Todo `GraphEdge` activo tiene endpoints, evidence, generation, provenance y cross-RID válidos; ambiguous/unresolved no crea dangling edge.
- Publicación, dual-read/write y rollback son transaccionales; crash/cancel conserva o restaura el puntero validado anterior.
- Wiki sigue siendo autoridad: graph/code puede verificar o detectar drift, no sobreescribir canon.
- MILX/packs son aislados y derivativos, sin graph/wiki write, MCP, red o secretos en v1.
- Toda claim de victoria conserva raw samples, ambos comparadores, 30 repeticiones y metricas unavailable explicitas.

## Dueño de cada entidad

Workspace/registry posee aliases y configuración; indexer/store posee catálogo y snapshot documental; Graph Kernel posee GraphGeneration, NodeKey, edges y unresolved; daemon posee runtime y telemetría; embed/wikichunk posee chunks y embeddings derivados. El archivo canónico sigue siendo dueño del contenido documental.

### Relaciones y ownership

- Un `WorkspaceRegistration` referencia un workspace `single` o `container` y puede guardar `CanonLinks` hacia otro alias registrado.
- Un `ProjectConfig` puede contener muchos `WorkspaceRepo`, muchos `WorkspaceEntrypoint` y cero o más `&#91;&#91;canon&#93;&#93;` con `[canon_policy]`.
- Un `ProjectConfig` puede declarar `[embeddings]`; `api_key_env` nombra una variable de entorno, no guarda secretos.
- Un `ProjectConfig` puede declarar `[recall.rerank_extension]` como hook local externo; no guarda payloads, respuestas de proveedor ni secretos.
- Cada `FileRecord` y `SymbolRecord` pertenece a un `repo_id`.
- Cada `DocRecord` puede tener muchos `DocEdge`, `DocMention`, `DocSourceBlock` y `DocSourceRecord`.
- Una `DocMention` conserva `source_block` opcional para atribución; una referencia `TP-*` se resuelve solo contra un `DocRecord.doc_id` exacto y único. Si falta o hay más de un target, no deriva edge y produce `GraphUnresolved` tipado con `missing_doc_target` o `ambiguous_doc_target`.
- Un `WikiChunkEmbedding` pertenece a un chunk documental y se inválida cuando cambia metadata-prefix, texto enriquecido, content hash, modelo o dimension.
- Un `GovernanceSource` manda sobre el `DocsReadProfile`; la proyección ejecutable no redefine la autoridad humana.
- Un `DocsOwnerHint` vive en `GovernanceSource` y se proyecta al `DocsReadProfile`; no redefine la gobernanza, solo refina ranking documental repo-especifico.
- Un `DocsReadProfile` gobierna como se interpreta la wiki del repo, pero no reemplaza el corpus indexado.
- Un `RuntimeSnapshot` pertenece a una combinacion `daemon_run_id + runtime_key`, donde `runtime_key` incluye `workspace_root` y `entrypoint_id`.
- Un `AccessEvent` puede guardar `workspace` visible, identidad canónica del workspace, `repo` y `entrypoint_id` para explicar routing y ambiguedad.
- Un `AskResult` se deriva de `DocRecord/DocEdge/DocMention` y, de forma secundaria, de `SymbolRecord/FileRecord`.
- Un `PackResult` se deriva de `DocRecord/DocEdge` y del `DocsReadProfile`; las slices se materializan on-demand desde archivos del workspace.
- Un `ServiceSurfaceSummary` se deriva de `SymbolRecord`, `FileRecord` y evidencia textual scoped al path pedido.
- Un `GraphGeneration` contiene muchos `GraphNode`, `GraphEdge`, `GraphEvidence` y `GraphUnresolved`; solo una generation sellada puede ser activa por workspace/schema.
- `GraphEdge` referencia dos `GraphNode` de la misma generation y una o más `GraphEvidence`; unresolved nunca se materializa como endpoint fantasma.
- El contexto de `GraphUnresolved` (`source_document`, `source_block`, `target_kind`, `target_value`) es derivado, nullable y bounded; no participa en `UnresolvedKey` ni `CrossRID` y no convierte un unresolved en endpoint.
- Aunque no define identidad, el contexto de `GraphUnresolved` participa, con framing determinista y orden estable, en los digests de contenido y facts que alimentan los fingerprints de source/config/backend y el `generation_id`.
- `GraphMigration` gobierna compatibilidad entre schema legacy y graph-native; no modifica el significado de las entidades.
- `GraphAnalysis` y `ContextPack` se derivan de generation(s), authority/profile digest, pack/extension version y parameters digest; invalidar cualquiera inválida el cache.
- `GlobalGraphSnapshot` referencia member generations inmutables por `(workspace_identity, generation_id)` y resuelve cross-edges por cross-RID sin adquirir ownership de sus datos.
- `MILXExecution` consume un pack/snapshot read-only y puede producir `GraphAnalysis`; no crea o modifica `GraphNode`, `GraphEdge`, wiki ni generation activa.

```yaml
harness_protocol: SDD-HARNESS-v1
id: data-model
kind: data_model
audience: dual
imports:
- .docs/wiki/05_requirements/RF-WKS-001.md
- .docs/wiki/05_requirements/RF-WKS-002.md
- .docs/wiki/05_requirements/RF-WKS-003.md
- .docs/wiki/05_requirements/RF-WKS-005.md
- .docs/wiki/05_requirements/RF-WKS-008.md
- .docs/wiki/05_requirements/RF-IDX-001.md
exports:
- data-model
- 5. Modelo de datos
agent_must_read:
- .docs/wiki/05_requirements/data-model.md
- .docs/wiki/05_requirements/RF-WKS-001.md
- .docs/wiki/05_requirements/RF-WKS-002.md
- .docs/wiki/05_requirements/RF-WKS-003.md
- .docs/wiki/05_requirements/RF-WKS-005.md
- .docs/wiki/05_requirements/RF-WKS-008.md
- .docs/wiki/05_requirements/RF-IDX-001.md
agent_may_edit:
- .docs/wiki/05_requirements/data-model.md
agent_must_not_edit:
- .docs/wiki/00_governance.md
- AGENTS.md
verify:
- Validar el contrato y sus referencias con `mi-lsp nav wiki validate-harness --workspace
  <alias>`; contrastar los casos de .docs/wiki/05_requirements/data-model.md sin inferir
  que fueron ejecutados.
stop_if:
- Detenerse si la propuesta requiere alterar criterios, decisiones o comportamiento
  más allá de data-model.
evidence:
- .docs/wiki/05_requirements/data-model.md
```
