---
wiki_contract: WIKI-CONTRACT-v1
id: TECH-RUNTIME-UPDATE
kind: technical
status: active
title: Runtime-owned updater y contrato update-status/v1
links:
  supports:
  - RF-UPD-001
  code:
  - path: internal/selfupdate/engine.go
    role: behavior
  - path: internal/selfupdate/state.go
    role: contract
  - path: internal/selfupdate/source.go
    role: behavior
  - path: internal/selfupdate/policy.go
    role: behavior
  - path: internal/selfupdate/schedule.go
    role: behavior
  - path: internal/selfupdate/version.go
    role: behavior
  - path: internal/selfupdate/lock_unix.go
    role: behavior
  - path: internal/selfupdate/lock_windows.go
    role: behavior
  - path: internal/selfupdate/transaction.go
    role: behavior
  - path: internal/selfupdate/transaction_linux.go
    role: behavior
  - path: internal/selfupdate/transaction_windows.go
    role: behavior
  - path: internal/selfupdate/transaction_other.go
    role: behavior
  - path: internal/cli/update.go
    role: behavior
  - path: internal/cli/root.go
    role: behavior
  - path: internal/daemon/update.go
    role: behavior
  - path: internal/daemon/server.go
    role: behavior
  - path: internal/daemon/update_controller.go
    role: behavior
  - path: internal/daemon/update_listener.go
    role: behavior
  - path: internal/daemon/update_retention.go
    role: behavior
  - path: internal/daemon/admin.go
    role: behavior
  - path: internal/daemon/file_watcher.go
    role: behavior
  - path: internal/daemon/lifecycle.go
    role: behavior
  - path: internal/indexer/job.go
    role: behavior
  - path: internal/service/primitives_session.go
    role: behavior
---

```yaml
harness_protocol: SDD-HARNESS-v1
id: TECH-RUNTIME-UPDATE
kind: technical-contract
audience: dual
imports: [.docs/wiki/07_technical.md]
exports: [TECH-RUNTIME-UPDATE]
agent_must_read: [.docs/wiki/07_technical/TECH-RUNTIME-UPDATE.md, internal/selfupdate/engine.go, internal/selfupdate/transaction.go]
agent_may_edit: [.docs/wiki/07_technical/TECH-RUNTIME-UPDATE.md]
agent_must_not_edit: [internal/selfupdate/, internal/daemon/, .docs/wiki/_mi-lsp/read-model.toml]
verify: ["Leer .docs/wiki/07_technical/TECH-RUNTIME-UPDATE.md y .docs/wiki/07_technical.md; cotejar las afirmaciones con la fuente publicada internal/selfupdate/engine.go y sus límites. Registrar únicamente comandos y resultados efectivamente ejecutados; esta especificación no afirma una ejecución."]
stop_if: [La propuesta permita origen externo configurable, limpie backup antes de readback o afirme rollback/actualización sin evidencia.]
evidence: [.docs/wiki/07_technical/TECH-RUNTIME-UPDATE.md, .docs/wiki/07_technical.md, internal/selfupdate/engine.go]
```

## Resumen

RF-UPD-001 define un updater único en el runtime; los consumidores consultan estado y notifican. Este documento no acredita las pruebas o el cierre pendientes en TP-UPD-001.

## Mapa de componentes

`internal/selfupdate/` contiene estado, política, fuente y promoción; `internal/cli/` expone update; `internal/daemon/` coordina el ciclo del daemon y sus consumidores.

## Decisión y autoridad

RF-UPD-001 define un único updater en el runtime. Los consumidores consultan el estado y notifican; no implementan otra descarga. Este documento es contrato de BUILD: no acredita pruebas o cierre aún pendientes en TP-UPD-001.

## Estado y concurrencia

El home contiene ~/.mi-lsp/update: lock OS global, cache.json de metadata y state/<hash>.json por ejecutable canónico. Windows normaliza case y se resuelven symlinks para identidad. Una ruta no hereda el recibo de otra. El cache común no puede impedir aplicar metadata fresca en una segunda instalación. Escritura JSON atómica; lectura acotada.

La consulta --check sólo lee estado existente y version en disco. Cache vencida conserva su fecha histórica. Ausencia, datos inválidos o contradicción de un updated con readback no se convierten en up_to_date ficticio.

## Política

MI_LSP_CHANNEL=pinned o ~/.mi-lsp/update/pin.json con schema mi-lsp-update-pin/v1 y pinned=true explicitan pin. MI_LSP_AUTO_UPDATE=0 deshabilita; channel y auto exponen la decisión. Las rutas de mis-entornos %USERPROFILE%/bin y ~/.local/bin NO implican pin. No hay escritura del pin desde una consulta.

## Fuente y promoción

Metadata: https://api.github.com/repos/fgpaz/mi-lsp/releases/latest. Assets: https://github.com/fgpaz/mi-lsp/releases/download/ con redirecciones HTTPS acotadas a infraestructura de assets de GitHub. No credenciales ni origen configurable externo. RID win/linux/darwin y x64/arm64; ZIP Windows, tar.gz Unix. Checksums: mi-lsp_<version>_checksums.txt, nombre exacto y SHA-256 obligatorio. Se preserva el basename oficial del archivo para validar su wrapper. Los seis paquetes públicos 0.15.6 tienen un directorio envolvente igual al basename sin extensión; no se asume CLI en raíz.

Se rechazan traversal, symlinks, entradas especiales/duplicadas, CLI ambigua y tamaños excesivos. Límites: archivo comprimido 512 MiB, entrada 256 MiB y suma expandida 2 GiB; el paquete público 0.15.6 win-arm64 contiene 708896784 bytes expandidos por sus seis payloads de workers. Metadata tiene plazo de 30 s; descargar el asset admite 3 minutos, preservando un timeout explícito del cliente de prueba. Se promueve sólo la CLI. Stage adyacente y backup de propiedad/hash verificables. Windows usa rename del ejecutable en uso; Unix reemplaza atómicamente. Readback ejecuta version con MI_LSP_UPDATE_CHILD=1, límite y timeout. Un backup POSIX propio puede desvincularse sin cortar un proceso que retiene el inode; en Windows la eliminación ocupada difiere. Nunca se limpia .old durante readback ni se borra un backup ajeno o alterado.

## Contrato público

Diez campos exactos: schema=update-status/v1, tool=mi-lsp, installed, latest, last_check, last_result, code, previous, channel, auto.

last_result: up_to_date, updated, deferred, failed, disabled, pinned. code es null salvo failed/deferred; previous sólo existe en updated. Allí installed==latest tras readback y el aviso es “de previous a installed”. Hooks deduplican por last_check.

Códigos: UPDATE_NETWORK_UNAVAILABLE, UPDATE_NO_RELEASE, UPDATE_CHECKSUM_MISMATCH, UPDATE_DOWNGRADE_REFUSED, UPDATE_BELOW_FLOOR, UPDATE_LOCKED, UPDATE_IN_USE_DEFERRED, UPDATE_READBACK_FAILED_ROLLED_BACK, UPDATE_QUARANTINED, UPDATE_PERMISSION_DENIED. Red, lock y ocupación no generan aviso a la persona. Nunca se afirma restauración si falló rollback ni antivirus sin señal concreta. Error técnico no representable no inventa otro código público.

## Daemon y consumidores

El daemon viejo sigue ejecutando su imagen hasta reposo comprobado. La barrera debe cubrir Accept, respuesta completa, HTTP, background y contextos/sesiones; desconocido significa diferir. Cleanup/shutdown del estado viejo precede al spawn nuevo. La admisión de jobs in-process se cierra bajo el mismo mutex que registra su lanzamiento; Stop de watchers cancela timers pendientes y espera callbacks admitidos. HTTP queda pausado pero reversible hasta el compromiso; cerrar el servidor durante preparación no es rollback. Retención y reaper se cancelan y se unen antes de cerrar telemetry. Un daemon antiguo o de otra instalación no puede suprimir para siempre el autorun de esta CLI.

El hijo usa processutil de A14: oculto/desconectado, sin heredar streams y con marcador antirrecursión. MCP stdio adopta el nuevo ejecutable al abrir una nueva sesión. El primer bootstrap de versiones publicadas sin updater es explícitamente manual; no se les atribuye retroactivamente esta capacidad.

## Evidencia y límites

TP-UPD-001 conserva disposiciones por criterio. BUILDING no es certificación; no se ejecutó macOS por haberlo compilado, no se reinicia daemon con contextos ajenos y no se hace una publicación puente adicional sin autoridad.
