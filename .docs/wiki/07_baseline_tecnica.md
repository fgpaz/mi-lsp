# 07. Baseline tecnica

## Proposito y alcance

Este documento resume la base tecnica operativa de `mi-lsp` en su fase `v1.3 hardening`.
Su foco es el runtime local, los componentes ejecutables, las fronteras tecnicas y las decisiones que deben sobrevivir a futuros refactors.
El detalle operativo y de subsistemas vive en `07_tech/`.

## Inventario de componentes

| Componente | Tipo | Owner logico | Responsabilidad |
|---|---|---|---|
| `mi-lsp` CLI | Binario Go | Core runtime | Punto de entrada, flags, routing y salida |
| Shortcut `init` | Surface CLI | Core runtime | Detectar, registrar e indexar rapido el workspace actual |
| Daemon global | Proceso Go opcional | Runtime supervision | Compartir warm state entre terminales/agentes |
| Runtime pool | Subsistema daemon | Runtime supervision | Mantener un runtime vivo por `(workspace, backend)` |
| Worker Roslyn | Proceso .NET hijo | C# semantic backend | Semantica profunda C# |
| Docgraph/read-model | Subsistema Go | Query/runtime | Rankear wiki, clasificar preguntas y conectar docs con codigo |
| TS catalog/indexer | Subsistema Go | Discovery backend | Discovery estructural TS/JS/Next repo-local |
| Service exploration profile | Subsistema Go | Query/runtime | Agregar evidencia observable por path de servicio usando catalogo + texto |
| TS semantic backend | Runtime opcional | TS semantic backend | Semantica TS/JS via `tsserver` cuando exista |
| Python indexer | Subsistema Go | Discovery backend | Indexacion Python via tree-sitter pure Go (`gotreesitter`) |
| Pyright semantic backend | Runtime opcional | Python semantic backend | Semantica Python via `pyright-langserver` |
| Governance UI | HTTP loopback local | Runtime supervision | Estado, accesos, memoria y diagnostico |
| File watcher (fsnotify) | Subsistema daemon | Pre-fetch | Re-indexa archivos modificados en background |
| Agent acceleration CLI | Subsistema Go | Compound commands | `multi-read`, `batch`, `related`, `workspace-map`, `search --include-content`, `nav ask` |
| Store repo-local | SQLite | Workspace owner | Catalogo de codigo, indice documental y metadata del repo |
| Store global daemon | SQLite + state file | Runtime supervision | Estado global del daemon y telemetria local |
| Cross-platform detach | Modulos Go | Runtime supervision | `server_windows.go` para named pipes, `server_unix.go` para unix sockets |
| Git integration | Subsistema Go | Query/indexing | Soporte incremental git-aware para re-indexing de archivos modificados |

## Dependencias externas

- `github.com/fsnotify/fsnotify` v1.9.0+ (file watcher para pre-fetch daemon)
- `ripgrep` opcional (busqueda de texto; fallback Go nativo si no existe)
- SDK/runtime de cada backend (Roslyn/.NET, Node/tsserver, Pyright)
- `.docs/wiki/_mi-lsp/read-model.toml` opcional como perfil declarativo por proyecto

## Mapa runtime e integracion

```mermaid
flowchart LR
    C[CLI mi-lsp] -->|named pipe / unix socket| D[Daemon global por usuario]
    C -->|fallback directo| G[Core Go]
    C --> I[init]
    I --> G
    D --> G
    D --> UI[Governance UI loopback]
    G --> IDX[.mi-lsp/index.db repo-local]
    G --> DOC[doc_records/doc_edges/doc_mentions]
    G --> CAT[TS catalog + ripgrep]
    G --> ASK[ask pipeline]
    G --> SV[Service exploration profile]
    D --> RP[Runtime pool por workspace/backend]
    RP --> RW[Roslyn worker]
    RP --> TW[tsserver opcional]
    RP --> PW[Pyright opcional]
    D --> GD[~/.mi-lsp/daemon state + db]
```

## Decisiones e invariantes

- Existe un unico daemon por usuario/host; no un daemon por workspace.
- El daemon debe ser compartible entre Claude Code, Codex y subagentes del mismo usuario.
- El daemon nunca es requisito funcional: toda consulta debe poder hacer fallback directo.
- Las lecturas baratas de catalogo/texto (`nav.find`, `nav.search`, `nav.symbols`, `nav.outline`, `nav.overview`, `nav.multi-read`) deben ejecutarse directas y no depender del health del daemon.
- Todo subprocesso no interactivo debe usar la politica comun de proceso; en Windows eso implica `HideWindow + CREATE_NO_WINDOW`, y los procesos background del daemon agregan `DETACHED_PROCESS`.
- El `tool_root` del worker se resuelve contra el ejecutable/distribucion activa o, en desarrollo, contra el repo `mi-lsp`; nunca contra el `cwd` arbitrario del workspace consultado.
- La unidad de warm state es un runtime por `(workspace_root, backend_type)`.
- El estado semantico persistente del workspace vive repo-local; el estado global solo guarda registro, estado del daemon y telemetria local.
- El estado documental persistente tambien vive repo-local: `doc_records`, `doc_edges` y `doc_mentions`.
- `nav ask` es docs-first: primero rankea docs canonicos, luego deriva evidencia de codigo desde menciones y fallback textual.
- La UI de gobernanza es unica, local a loopback y debe abrirse enfocando workspace, sin duplicar instancias.
- C# profundo se resuelve con Roslyn; TS/JS discovery sigue existiendo aunque no haya backend semantico.
- Python se indexa con tree-sitter pure Go (`gotreesitter`); semantica profunda opcional via `pyright-langserver` cuando exista.
- `nav context` es slice-first: el core arma un bloque legible por lineas y luego superpone enriquecimiento semantico o de catalogo cuando exista.
- `nav service` usa evidencia observable, no score fuerte de completitud.

## Busqueda: cadena de fallback

La busqueda textual implementa una cadena de fallback robusta:

1. `rg` binario: si existe y es accesible, usa `ripgrep` nativo
2. Go native: fallback a `searchPatternGo` nativo que respeta `.milspignore` y filtra binarios
3. `MI_LSP_RG` env var: permite override de la ruta de `rg`

La cadena garantiza que la busqueda siempre funciona sin dependencias externas obligatorias.
Si `rg` devuelve exit code `1` por ausencia de matches, el core lo normaliza a `items=[]` en vez de exponerlo como error.
La exploracion de servicios y el fallback de `nav ask` reutilizan la misma cadena.

## Config y valores por defecto

El struct `internal/service/config.go` centraliza todos los valores hardcodeados:

- `DefaultConfig()` inicializa fallbacks sensatos
- Incluye rutas de workers, timeouts, limites de memoria, ignoreslists por defecto
- Permite override via flags CLI y variables de entorno
- El `read-model` por defecto se embebe en `docgraph.DefaultProfile()` y puede ser sobreescrito por el proyecto
- Separacion clara entre valores de compilacion vs runtime

## Telemetria universal

- Todas las operaciones (con y sin daemon) registran `access_events` en `~/.mi-lsp/daemon/daemon.db`.
- CLI directo usa `daemon_run_id = NULL`; el daemon usa su `run_id`.
- WAL mode habilitado para manejar escrituras concurrentes daemon + CLI.
- Auto-purge de eventos > 30 dias (configurable via `MI_LSP_RETENTION_DAYS`) en startup de CLI y daemon.
- `access_events` separa identidad analitica y diagnostica: `workspace_root` es la clave canonica de agrupacion; `workspace_alias` y `workspace_input` preservan display y forensics.
- `access_events.seq` ordena eventos dentro de un `session_id`; vale `0` cuando la llamada no trae sesion y arranca en `1` para la primera operacion de una sesion trazable.
- `access_events` tambien preserva metadata minima del llamado para analitica local: `route` (`direct`, `daemon`, `direct_fallback`), `format`, presupuestos (`token_budget`, `max_items`, `max_chars`) y `compress`.
- La taxonomia minima de errores tipados distingue al menos `sdk/*`, `worker_bootstrap/*` y `backend_runtime/*`.
- Export: `mi-lsp admin export` soporta raw (json/csv/compact), `--summary` y el preset explicito `--recent` para la ultima ventana de 24h.
- `admin export --summary` debe agregar sobre toda la ventana filtrada por defecto; `--limit` solo acota el summary si el usuario lo pide explicitamente.
- Governance UI y admin HTTP comparten la misma semantica de ventana via `window=recent|7d|30d|90d`.

## Resumen operativo

- `daemon start` debe ser idempotente y resolver si ya existe una instancia saludable.
- Queries semanticas y compuestas seleccionadas inician automaticamente el daemon si no esta corriendo (desactivar con `--no-auto-daemon`).
- `nav.find`, `nav.search`, `nav.intent`, `nav.symbols`, `nav.outline`, `nav.overview` y `nav.multi-read` no deben auto-iniciar ni enrutar por daemon en builds actuales; en workspaces `container`, `find/search/intent` pueden acotar con `--repo`.
- `init` registra, persiste proyecto e indexa por defecto sin requerir `workspace add` previo.
- `worker install` es explicito; no hay descargas silenciosas durante consultas.
- `worker install` copia un worker bundled por RID cuando la distribucion lo trae adjunto; si la CLI corre dentro del repo `mi-lsp` y no existe bundle adjunto, publica el worker desde `worker-dotnet/` con `dotnet publish`.
- Las queries Roslyn resuelven candidatos por presencia de archivos en orden `bundle -> installed -> dev-local` y no hacen probe de compatibilidad en el hot path.
- `worker status` debe exponer `tool_root`, `tool_root_kind`, `cli_path`, `protocol_version`, origen seleccionado (`bundle|installed|dev-local`) y compatibilidad de candidatos; el probe explicito vive ahi y no en las queries regulares.
- Si `worker status` se sirve a traves del daemon, la respuesta visible debe seguir siendo el mismo envelope canonico de `backend=worker`; el estado vivo del daemon solo entra via `active_workers`.
- Si el candidato Roslyn elegido falla por bootstrap/arranque, el caller reintenta una sola vez con el siguiente candidato determinista antes de devolver error accionable.
- Los cambios en `.docs/wiki`, `README*`, `docs/` o `read-model.toml` fuerzan full re-index del corpus documental; el incremental por git no intenta mezclar deltas parciales de docs.
- `workspace status` debe exponer si el proyecto usa `read-model` propio o el default embebido.
- `nav ask --all-workspaces` fan-out sobre workspaces registrados con un pool acotado de 4 workers y merge determinista por score.
- `nav.find`, `nav.symbols`, `nav.overview` y `nav.intent` aceptan `--offset` para paginacion cursor-like sobre queries SQL; `nav.search` queda fuera de ese contrato porque sigue siendo rg/text-backed.
- `nav service` debe funcionar sin Roslyn y seguir entregando evidencia util incluso cuando el catalogo es parcial.
- `nav context` sobre archivos no semanticos no debe depender de Roslyn, `tsserver` ni Pyright.

## Documentos detalle

- [TECH-DAEMON-GOBERNANZA.md](07_tech/TECH-DAEMON-GOBERNANZA.md)
- [TECH-TS-BACKEND.md](07_tech/TECH-TS-BACKEND.md)
- [TECH-DEPENDENCY-HARDENING.md](07_tech/TECH-DEPENDENCY-HARDENING.md)
- [TECH-PYTHON-BACKEND.md](07_tech/TECH-PYTHON-BACKEND.md)
- [TECH-SERVICE-EXPLORATION.md](07_tech/TECH-SERVICE-EXPLORATION.md)
- [TECH-WIKI-AWARE-SEARCH.md](07_tech/TECH-WIKI-AWARE-SEARCH.md)

## Change triggers

Actualizar `07` y/o `TECH-*` cuando cambie cualquiera de estos puntos:

- topologia CLI/daemon/runtime pool/UI
- lifecycle de runtimes o politica de eviction
- dependencia obligatoria del worker o estrategia de instalacion
- backend semantico TS/JS
- backend semantico Python (Pyright)
- estrategia de hardening de dependencias o bootstrap runtime
- perfiles de exploracion docs-first o evidence-first como `nav ask` y `nav service`
