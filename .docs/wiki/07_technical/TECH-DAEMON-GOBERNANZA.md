---
wiki_contract: WIKI-CONTRACT-v1
id: TECH-DAEMON-GOBERNANZA
kind: technical
status: active
title: TECH-DAEMON-GOBERNANZA
links:
  code:
  - path: internal/cli/daemon.go
    role: implementation
  - path: internal/daemon/admin.go
    role: implementation
  - path: internal/daemon/export.go
    role: implementation
  - path: internal/daemon/log_tail.go
    role: implementation
  - path: internal/daemon/state_store.go
    role: implementation
---

## Resumen

Define el modelo canónico del daemon global, su governance UI workspace-first y la gestion compartida de runtimes calientes entre multiples clientes locales.

### Resumen
Define el modelo canónico del daemon global, su governance UI workspace-first y la gestion compartida de runtimes calientes entre multiples clientes locales.

## Mapa de componentes

Los componentes, paths exactos, responsabilidades y límites se mantienen en el detalle técnico siguiente.

Volver a [07_technical.md](../07_technical.md).

### Responsable y alcance
- Owner logico: Runtime supervision
- Scope: daemon global, runtime pool, health, governance UI, acceso concurrente local, warm seguro y logs locales
- Non-goals: auth remota, cluster multi-host, observabilidad externa, acciones destructivas desde la UI

### Runtime o subsistema

#### Topología canónica

- Un daemon por usuario/host.
- Un runtime vivo por `(workspace_root, backend_type, entrypoint_id)`.
- Aliases duplicados del mismo `workspace_root` pueden compartir runtime si backend y entrypoint coinciden.
- Worktrees distintos del mismo repositorio no comparten runtime porque su `workspace_root` físico es distinto aunque compartan `git common dir`.
- El auto-registro identifica un linked worktree comparando rutas absolutas, canónicas y normalizadas de `git dir` y `git common dir`. Por defecto resuelve el workspace principal si ya está registrado, sin registrar ni indexar la raíz física del linked worktree; si no existe esa entrada, responde `explicit_incomplete` (`reason_code=invalid_workspace`) sin indexar. `MI_LSP_AUTOREGISTER=force` registra el linked worktree como root físico separado y deja un warning explícito. Fuera del auto-registro, la resolución read-only con **path explícito** hacia un linked worktree no registrado fija `workspace_root` en la raíz física de ese path para pools y runtime, sin registrar ni indexar esa raíz; selector **omitido**, **alias** explícito o path explícito en modo **mutante** conservan la conducta vigente (alias del main registrado cuando corresponda, sin nuevo registro ni índice).
- Pools separados por backend:
  - `roslyn`
  - `tsserver`
  - `pyright`
  - `gopls`
  - `rust-analyzer`
- El warm de un workspace es por lenguaje detectado (`roslyn` para C#, `tsserver`, `pyright`, `gopls` para Go y `rust-analyzer` para Rust) y ya no calienta Roslyn por defecto en repos sin C#.
- Politica por defecto:
  - `max_workers = 8` (era 3; constante única `defaultMaxWorkers`, override con env `MI_LSP_DAEMON_MAX_WORKERS` entero positivo; el flag `--max-workers` en 0 significa env u 8). Motivo: proyectos C#, Go, Python, TS, Rust y Kotlin concurrentes comparten un daemon y con 3 el LRU descartaba runtimes caros (Roslyn) en cada alternancia. El auto-start, `daemon restart` y el fallback del manager usan el mismo valor
  - `idle_timeout = 30m`
  - `watch_mode = lazy`
  - `max_watched_roots = 8`
  - `max_inflight = 48` (era 16; env `MI_LSP_DAEMON_MAX_INFLIGHT`)
  - eviction `LRU`

- **Marcador estable**: TECH-DAEMON-GOBERNANZA.topology-defaults-v2
- **description**: Daemon topology defaults updated for max concurrency
- **max_workers**: 8
- **max_workers_env**: MI_LSP_DAEMON_MAX_WORKERS
- **max_inflight**: 48
- **max_runtime_memory_mb**: 1024
- **soft_memory_threshold_mb**: 500
- **wal_checkpoint_interval_sec**: 1800
- **result_cache**:
  - **enabled**: true
  - **max_entries**: 256
  - **ttl_minutes**: 10
  - **cacheable_ops**:
    - nav.ask
    - nav.search
    - nav.pack
    - nav.governance
    - nav.route
  - **disable_env**: MI_LSP_DAEMON_RESULT_CACHE=0
  - **key_derivation**: sha256(workspace_root + generation(mtime index.db) + op + canonical_payload_json)

#### Hito 6 de §11bis: molde elástico común (pendiente)

Este hito adopta el contrato `CT-CLI-DAEMON-V1` y la biblioteca Go común que mantiene su responsable. El alcance aprobado es una instancia del daemon por nodo; procesos de arranque y procesos hijos sin ventanas, también en Windows; y ayudantes cuyo mínimo sea el número de coordinadores vivos del nodo (piso 1), con crecimiento bajo demanda hasta coordinadores × 3 o el límite de la máquina. El tratamiento exacto de límites queda en el contrato publicado: esta nota no fija valores por defecto de CPU/RAM ni redefine su política.

El molde compartido debe cubrir fila/cola, error `ocupado`, tope por llamador, reciclado y salud. Las CLI internas usan la identidad del nodo y no piden credenciales a Gabriel. Esta preparación documental no expone valores de secretos en conversaciones, evidencias ni logs ni modifica permisos. Esto es el criterio de aceptación del hito, no una descripción de comportamiento ya implementado.

La publicación ya está disponible en `fgpaz/bots-herramientas` (`6ec36b8e70b1ca1cc1b0831f58fcba71551886b5`, release `v0.1.10`). El módulo Go del subdirectorio resuelve el tag del repo raíz como `github.com/fgpaz/bots-herramientas/mold/daemon@v0.0.0-20261024002600-6ec36b8e70b1`, no como `@v0.1.10`. La API observada en esa fuente incluye `Start(Config)`, `Config.SetMinimum`, `CoordinatorSource`, `DefaultCoordinatorSource(node)`, `CoordinatorSizing(ctx,node,source,machineCeiling)`, `Server.Submit`, `Server.Stats`, `Call`, y `StartHidden`/`StartDetached` en Windows.

Estado de H6 en este checkout: **adaptación local implementada, sizing/idle dinámicos aún incompletos y runtime sin verificar**. `internal/daemon/elastic.go` consulta `botsd identidad`, toma el campo público `nodo`, obtiene el censo con `DefaultCoordinatorSource(node)` y calcula el sizing inicial con `CoordinatorSizing`. `NewServerWithOptions` inicia `mold/daemon` dentro del mismo proceso. El listener legacy sigue atendiendo el protocolo existente; cada request admitido se serializa una vez, pasa por `Server.Submit` y se despacha una sola vez a `handleRequestContext`. `client_name` y `session_id` se conservan en el payload y forman el caller para el límite común. El `Manager` por workspace/backend y su aislamiento/LRU se mantienen. La respuesta de dominio y el fallback spawn permanecen en sus rutas existentes; una cola común llena se mapea al envelope legacy `backpressure_busy`, incluyendo `retry_after_ms=100` y `reason_code=ocupado`.

El `system.status` publica el `Stats()` real bajo `elastic_pool`; `doctor` proyecta esos mismos siete campos en `daemon_pool` cuando hay conexión. No se infieren ayudantes comunes de `WorkerCapacity` de Roslyn.

La publicación aporta cola acotada (4), límite por caller (2), escalado tras 50 ms, retiro tras 90 s y reciclado cada 64 trabajos. No se copia esa policy. **Gap upstream de sizing**: `Start` copia Config y no actualiza min/max del `Server` vivo; `SetMinimum` no modifica el pool ya iniciado y no hay refresco de `CoordinatorSizing` al minuto/evento. El código integra únicamente el censo/sizing inicial, sin afirmar que el límite sea dinámico. La máxima se clamp a la máquina dentro del proveedor.

**Gap upstream de idle**: la implementación del proveedor tiene `idleMonitor` de una sola espera desde el arranque, no reinicia el reloj ante actividad. La integración configura `AlwaysOn` porque el monitor común puede cerrar el Submit en un momento incorrecto; por eso el apagado idle del molde no está activo aquí. Se requiere que el proveedor permita idle-reset seguro del servidor vivo o que el owner autorice un lifecycle común corregido; no se duplicó un segundo timer/pool local.

`Start` también publica su listener/lock común en el mismo proceso además del listener legacy que conserva compatibilidad. El nombre incluye la identidad Babel acreditada (`tesla-desktop`), pero el proveedor ubica lock/socket bajo el runtime/profile del usuario; el alcance entre usuarios distintos del mismo nodo no se comprobó. Además, el servidor compartido intercepta `op=shutdown` y cierra solo su listener/pool, sin notificar al daemon mi-lsp; un Submit legacy posterior no tiene una API pública para detectar/recrear el pool cerrado. Requiere que el proveedor conecte el cierre al lifecycle anfitrión o haga que la operación pase por el Handler, sin recrear un pool local. Windows/Linux real no se ejecutó. No declarar H6 cerrado hasta que el setter dinámico, idle/reset y lifecycle de shutdown estén disponibles y el padre verifique el comportamiento.

Fuente: `fgpaz/bots-herramientas` SHA `6ec36b8e70b1ca1cc1b0831f58fcba71551886b5`; módulo resuelto `github.com/fgpaz/bots-herramientas/mold/daemon@v0.0.0-20261024002600-6ec36b8e70b1`. Código mi-lsp: `internal/daemon/elastic.go`, `internal/daemon/server.go`, `internal/cli/doctor.go`, `internal/model/types.go`; control de arranque oculto legacy: `internal/daemon/server_windows.go` y `internal/processutil/command_windows.go`.

#### Acceso compartido y caching

- El daemon vive fuera de la terminal que lo lanza.
- Claude Code, Codex y subagentes deben poder conectarse al mismo daemon bajo el mismo usuario.
- No todo `nav` debe pasar por el daemon: las lecturas baratas de catalogo/texto se resuelven directo en la CLI y reservan el daemon para queries semánticas o compuestas.
- `nav.ask`, `nav.search`, `nav.pack` son daemon-elegibles cuando el daemon vive (sin auto-start forzado; fallback a directo intacto).
- Cuando el daemon atiende diagnósticos administrativos como `worker status`, debe delegar al contrato canónico del core y no reinterpretar el payload como una lista cruda de runtimes.
- Cada request debe incluir cuando esta disponible:
  - `client_name`
  - `session_id`
- `daemon start` debe:
  - chequear health primero
  - devolver metadatos de la instancia existente si ya corre
  - crear nueva instancia solo bajo lock global
- `EnsureDaemon` debe usar el mismo lock global y health recheck que `daemon start`; el estado persistido no basta si el pipe/socket no responde.
- El watcher no arranca recursivamente sobre todos los aliases por default: `lazy` activa por root canónico, `eager` es opt-in y `off` deshabilita watchers.
- Los watchers se deduplican por `workspace_root` canónico y se acotan por LRU con `max_watched_roots`; worktrees con roots distintos se observan por separado.
- Todo worker lanzado (Roslyn, cliente LSP generico, tsserver) queda ligado al daemon: en Linux se crea con `Pdeathsig=SIGKILL`, de modo que un daemon caido o terminado a la fuerza no deja language servers huerfanos. En otros SO no cambia: el cierre ocurre por shutdown ordenado o reaping por `idle_timeout`.
- El registry global decodificado se cachea por path en proceso (invalidado por mtime+size y por `SaveRegistry`); cada llamador recibe una copia del mapa, así una mutación local no corrompe el cache.
- Ranking documental: la normalización de texto es de una sola pasada (misma salida) y el contexto wiki-codigo vivo y la autoridad de gobernanza reutilizan el cache de `doc_records` por generación en lugar de recargarlos varias veces por consulta.
- Requests pesadas daemon-aware se acotan con `max_inflight`; saturacion devuelve `daemon/backpressure_busy`.
- Result cache LRU (256 entradas, TTL 10 min): cachea read-only ops (nav.ask/search/pack/governance/route) keyed por sha256(workspace_root + generación(mtime index.db) + op + payload canónico sin session_id). Sin error-caching. Disable con `MI_LSP_DAEMON_RESULT_CACHE=0`. Stats en `system.status.result_cache: {hits, misses, entries}`. Los logs HIT/MISS son diagnóstico opt-in con `MI_LSP_DAEMON_RESULT_CACHE_DEBUG`; no forman parte del hot path normal.
- Ranking owner-aware aplica `canonical_ceiling`: un owner canónico positivo no puede quedar debajo de ruido FTS de README genérico.
- Workspace hygiene conserva aliases con root vacío como casos ambiguos; la recolección automática y `workspace prune --stale` solo remueven aliases cuando la raíz está definitivamente inexistente. Permisos y otros errores de I/O se reportan de forma sanitizada y no se borran directorios, cachés ni worktrees.
- SQLite conserva un handle de escritura serializado (`MaxOpenConns=1`) y separa consultas en un pool read-only acotado (8 open/4 idle). Cada conexion de lectura recibe `_pragma=query_only(ON)` y `_pragma=foreign_keys(ON)` desde el DSN modernc antes de entrar al pool.
- Presupuesto SLO local: `daemon perf-smoke` valida callers paralelos contra working set, private bytes y handles; cualquier excedente debe devolver envelope `ok=false` tipado y dejar warning accionable, no un pass degradado.

#### Governance UI

- Expuesta solo en `127.0.0.1:<port>`.
- Instancia única global.
- `admin open --workspace <alias>` y `daemon open --workspace <alias>` deben abrir la misma UI enfocando el workspace por query params.
- Deep-link canónico:
  - `/?workspace=<alias>&panel=<overview|activity|logs|metrics>&window=<recent|7d|30d|90d>&backend=<type>`
- Vistas minimas:
  - estado del daemon
  - KPIs operativos
  - tabs por workspace
  - runtimes activos y memoria por backend
  - access events recientes por cliente/sesion con corte temporal explícito y drawer con `workspace_root`, `workspace_input`, `error_kind` y `error_code`
  - drawer de detalle
  - tail de logs locales
  - panel metrics: p50/p95 por operación, error rate y truncation rate con ventana configurable
- Acciones seguras soportadas:
  - `refresh`
  - `warm workspace`
  - `open logs`
  - `copy CLI command`

### Endpoints admin locales

- `GET /`
- `GET /api/status?window=<recent|7d|30d|90d>`
- `GET /api/workspaces?window=<recent|7d|30d|90d>`
- `GET /api/workspaces/{workspace}?window=<recent|7d|30d|90d>`
- `POST /api/workspaces/{workspace}/warm` (requiere admin token pre-compartido en header `X-Mi-Lsp-Token` + validación Host/Origin)
- `GET /api/accesses?window=<recent|7d|30d|90d>`
- `GET /api/logs?tail=<n>`
- `GET /api/metrics?window=<recent|7d|30d|90d>` — computa p50/p95, error rate y truncation rate por operacion/workspace/cliente desde `access_events`; mantiene compatibilidad legacy con `days=<n>`
- `mi-lsp admin export --summary` reutiliza la misma ventana y agrega breakdowns opcionales por route/client/hint/failure-stage; raw y summary soportan `--format toon` para revision agent-friendly, el summary sin `--limit` debe agregarse en streaming desde `daemon.db`, y el raw export prioriza CLI antes que UI para debugging operativo y es la primera superficie donde deben aparecer los campos causales nuevos.
- `GET /api/status` también expone `daemon_process` y `watchers`, equivalentes a `daemon status`, para validar presupuestos de agentes sin inspección externa.
- admin token es un valor pre-compartido guardado en `~/.mi-lsp/daemon/state.json` y requerido solo para endpoints mutantes; se comunica al usuario via `daemon status` o `admin open`.
- Host/Origin validation previene CSRF/DNS-rebinding local: endpoints mutantes rechazan requests cuyo header `Host` o `Origin` no apunten a `127.0.0.1:<puerto>` del daemon.

### Dependencias e interacciones

- CLI publica
- named pipe en Windows / unix socket en Linux
- `~/.mi-lsp/daemon/state.json`
- `~/.mi-lsp/daemon/daemon.db`
- `{repoRoot}/.mi-lsp/daemon.log`
- worker Roslyn
- `tsserver` opcional

### Fallos y notas operativas
- **Riesgo**: Doble daemon; **Sintoma**: dos `start` simultaneos; **Mitigacion canonica**: lock + health recheck
- **Riesgo**: Auto-start concurrente; **Sintoma**: N agentes arrancan N procesos; **Mitigacion canonica**: `EnsureDaemon` comparte lock y no confia en state sin health
- **Riesgo**: Explosion de handles por watchers; **Sintoma**: aliases duplicados o registry grande; **Mitigacion canonica**: `watch_mode=lazy`, dedupe por root canónico, cap LRU
- **Riesgo**: Saturacion de requests; **Sintoma**: latencia/memoria crecen sin límite; **Mitigacion canonica**: backpressure `daemon/backpressure_busy` y env `MI_LSP_DAEMON_MAX_INFLIGHT`
- **Riesgo**: Socket/pipe huerfano; **Sintoma**: connect falla pero state existe; **Mitigacion canonica**: purge + restart controlado
- **Riesgo**: RAM excesiva; **Sintoma**: demasiados runtimes vivos; **Mitigacion canonica**: `max_workers` + LRU eviction
- **Riesgo**: SLO de memoria/proceso excedido; **Sintoma**: `perf-smoke` supera working set/private bytes/handles; **Mitigacion canonica**: envelope `ok=false` tipado + ajustar `max_workers`, watchers o backpressure
- **Riesgo**: UI duplicada; **Sintoma**: cada cliente abre una vista separada; **Mitigacion canonica**: admin URL única + deep link por query
- **Riesgo**: Cliente antiguo; **Sintoma**: errores sutiles de protocolo; **Mitigacion canonica**: handshake con version explícita
- **Riesgo**: Warm fallido; **Sintoma**: runtime no queda disponible; **Mitigacion canonica**: warning visible + logs locales
- **Riesgo**: Tail ruidoso al apagar; **Sintoma**: líneas benignas de socket/pipe cerrado más bloque `Usage`; **Mitigacion canonica**: filtro de ruido benigno en `daemon logs` y `/api/logs` sin ocultar líneas no benignas
- **Riesgo**: `tsserver` frio roto; **Sintoma**: cada request vuelve a intentar bootstrap y falla; **Mitigacion canonica**: cooldown corto + degradacion sticky a catalog/text
- **Riesgo**: Worker/proceso bloqueado por permisos; **Sintoma**: `nav context` o `nav search` devuelve warning `backend_runtime/process_spawn_access_denied`; **Mitigacion canonica**: fallback slice/catalog/text o Go search + evidencia exportable via `admin export --format toon`

### Referencias de contexto
- [DB-STATE-Y-TELEMETRIA.md](db/DB-STATE-Y-TELEMETRIA.md)
- [CT-CLI-DAEMON-ADMIN.md](contracts/CT-CLI-DAEMON-ADMIN.md)
- [CT-DAEMON-WORKER.md](contracts/CT-DAEMON-WORKER.md)
