---
wiki_contract: WIKI-CONTRACT-v1
id: TECH-WINDOWS-HOST
kind: technical
status: draft
title: Host Windows sin ventana y protocolo execsafe-host/v1
---

Esta especificación describe el contrato del productor, no certifica conformidad ni admisión de un consumidor. [[RF-HOST-001]] define requisitos, [[FL-HOST-001]] el flujo y [[TP-HOST-001]] los oráculos. El consumidor B comparte framing, no necesariamente política de ciclo de vida; su conexión y admisión requieren evidencia independiente.

## Invocación, control y streams

`mi-lsp-host.exe [--timeout-ms N] --control-file ABS_CONTROL -- ABS_EXE [args...]`. La opción timeout sólo precede `--control-file`, es decimal 1..86400000 ms, default 300000. Control debe ser archivo regular existente y vacío, de ruta absoluta e identidad estable bajo control privado y exclusivo del caller. `ABS_CONTROL.result` debe no existir: se crea con exclusividad antes de arrancar el payload. El ejecutable debe ser ruta absoluta con extensión `.exe`; se transmiten argumentos separados, sin shell ni búsqueda PATH. Se preservan las reglas de quoting Windows del runtime, incluidos espacios y argumentos vacíos.

El host duplica e incluye sólo stdin, stdout y stderr en la lista explícita de herencia. Son bytes sin framing ni diagnósticos añadidos; control, resultado y job no se heredan. Los streams pueden contener cualquier byte. El caller escribe `INT` o `TERM` opcionalmente seguidos de LF en el control estable; prefijos incompletos quedan pendientes, una secuencia inválida produce infraestructura. No registrar argv, entorno, datos de payload ni secretos. Directorio privado y sole-writer son precondiciones del caller, no autenticación por el protocolo; no se modifican ACL ni UAC.

## Resultado y consumidor

El sibling contiene exactamente dos líneas terminadas en LF: cabecera ASCII `execsafe-host/v1 KIND DWORD` y JSON compacto con `protocol`, `kind`, `exit_code`, y opcionalmente `root_exit_code`, `signal`, `error` y `cleanup_errors`. `kind` distingue `root_exit`, `cancelled` e `infra_error`; los códigos son DWORD sin pérdida. `root_exit_code` se preserva cuando el productor lo observó. Un root con 125, 130 o 143 sigue siendo `root_exit`, distinto de infraestructura 125 o cancelación 130/143.

Host status 0 significa sólo que la escritura completa y el cierre del resultado tuvieron éxito. Cualquier status host no cero, archivo ausente, incompleto, truncado, malformado o cabecera/JSON inconsistente invalida el transporte. Un consumidor Node debe esperar el evento `close` del proceso y el cierre de los streams, no sólo `exit`; después valida ambos registros, protocolo, tipo y DWORD, y propaga el resultado tipado. Nunca interpreta el cero del transporte como éxito del payload ni trunca DWORD a ocho bits; adaptar a una API de status limitado requiere mantener el valor original por separado. Los errores de cleanup impiden false success.

## Job y límites

Se crea JobObject privado y se establece `JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE` antes del root. Root se crea con `CREATE_SUSPENDED | CREATE_NO_WINDOW` y lista selectiva de handles, se asigna al job por handle y sólo entonces se reanuda. No hay breakaway solicitado ni PID reopen/taskkill. Si falla asignación, la limpieza del root suspendido usa su handle propio; si ya está asignado, errores, timeout y cancelación terminan y drenan el job por handle. La limpieza tiene espera acotada y sus errores se reportan, no se ocultan.

En salida normal, root exit por sí solo no basta: esperar `activeProcesses == 0`, cerrar recursos propios y luego escribir el resultado. Un hard kill del host cierra el último handle del job y Windows termina el árbol asociado. Esto no preserva daemons descendientes como la política NormalDaemon del candidato B. No garantiza neutralizar una GUI creada deliberadamente por el payload, exfiltración ni evasión con privilegios. No se mata por PID ni se actúa sobre procesos ajenos.

## Versionado y distribución

`mi-lsp-host.exe --version --format json` devuelve `tool`, `version`, `revision`, `abi`, `arch`, `job_policy`; el runtime usa `internal/model.BuildVersion` y `BuildRevision` y release también enlaza `BuildTime`. La fuente VCS limpia y su revisión verificable son obligatorias, no inferidas del nombre. Recursos PE se generan desde fresh clone mediante go-winres v0.3.3 antes de compilar, con versión numérica de release; el JSON fuente y el Makefile usan `0.0.0.0` como default local, pero el hook oficial lo reemplaza por `major.minor.patch.0`. Los `.syso` no se commitean. `BuildTime` se enlaza desde la fecha de GoReleaser; no aparece en el JSON de versión del host y no sustituye la revisión VCS.

La distribución incluye `mi-lsp-host_VERSION_win-x64.exe` y `mi-lsp-host_VERSION_win-arm64.exe` en la misma release oficial: 6 archives CLI/worker, 2 host y 1 checksum = 9 assets. Ambos host requieren PE GUI subsystem 2, arquitectura, versión, VCS y SHA verificados; la CLI principal sigue CUI. El checksum debe incluir ambos hosts. Falta de asset o evidencia bloquea, sin fallback CLI/CUI, PATH o ejecutable adyacente. El tag público, revisión de fuente, hashes y readback deben verificarse para la release concreta; la configuración fuente no prueba su existencia. La UI actual del daemon y sus raíces no cambian.
