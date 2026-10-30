---
wiki_contract: WIKI-CONTRACT-v1
id: TECH-LOCAL-DAEMON
kind: technical
status: draft
title: Daemon local y compartido
links:
  supports: [RF-DAE-001]
  code:
    - path: internal/daemon/server.go
      role: behavior
    - path: internal/daemon/admin.go
      role: behavior
---

## Resumen

Describe daemon local y pool opt-in, exclusión del dueño y diagnóstico; no acredita corrección de duplicados legacy.

## Mapa de componentes

`internal/daemon/` administra inicio y servicio; el pool compartido requiere sus dependencias explícitas.

# Daemon local y compartido

```yaml
harness_protocol: SDD-HARNESS-v1
id: TECH-LOCAL-DAEMON
kind: technical-operations
audience: dual
imports: [.docs/wiki/07_technical/TECH-DAEMON-GOBERNANZA.md, .docs/wiki/07_technical/contracts/CT-CLI-DAEMON-ADMIN.md]
exports: [TECH-LOCAL-DAEMON]
agent_must_read: [.docs/wiki/07_technical/TECH-LOCAL-DAEMON.md]
agent_may_edit: [.docs/wiki/07_technical/TECH-LOCAL-DAEMON.md]
agent_must_not_edit: [internal/daemon/, .mi-lsp/, .docs/wiki/_mi-lsp/read-model.toml]
verify: ["Leer .docs/wiki/07_technical/TECH-LOCAL-DAEMON.md y .docs/wiki/07_technical/TECH-DAEMON-GOBERNANZA.md; cotejar las afirmaciones con la fuente publicada internal/daemon/admin.go y sus límites. Registrar únicamente comandos y resultados efectivamente ejecutados; esta especificación no afirma una ejecución."]
stop_if: [El procedimiento requiera terminar procesos ajenos, alterar ACL/credenciales o reparar datos compartidos fuera de su contrato.]
evidence: [.docs/wiki/07_technical/TECH-LOCAL-DAEMON.md, .docs/wiki/07_technical/TECH-DAEMON-GOBERNANZA.md, internal/daemon/admin.go]
```

El daemon predeterminado usa el endpoint del perfil local y no invoca `botsd`. `MI_LSP_ELASTIC=1` en cliente y servidor habilita explícitamente el pool compartido por nodo; requiere `botsd` en PATH, identidad configurada y fuente de coordinadores accesible. Reiniciar el daemon por su ciclo normal al cambiar el modo. Si faltan requisitos, el error es accionable; quitar la variable permite usar el daemon local.

## Exclusión y dueño no alcanzable

El fallo del health check no prueba muerte del dueño publicado en `state.json`. El arranque debe fallar cerrado ante PID vivo o indeterminado, estado ilegible o reserva ocupada, antes de fork/listener/apertura de `daemon.db`. En Windows, `AccessDenied` al consultar el proceso no habilita reemplazo. El diagnóstico conserva PID/endpoint sin exponer admin token ni contenido del estado.

El lock breve de bootstrap no basta para constructores concurrentes con endpoints distintos. Una reserva separada se adquiere antes de DB, se retiene hasta su cierre y se libera en todos los fallos de construcción. No se reclama por edad una reserva con dueño vivo/indeterminado. Una reserva basada en PID/nonce protegida por guard OS breve no equivale a un handle OS retenido durante toda la vida; no confundir ambas garantías.

La mitigación evita nuevos arranques cooperantes; no drena ni mata duplicados legacy. Todos los lanzadores deben consumir el binario actualizado y el supervisor realiza la recarga autorizada. No promete corregir con esto el bucle de CPU, idle de servidor ni WAL; cada uno exige evidencia separada. No cambia ACL, UAC, credenciales ni perfil global.

Doctor informa `status: skip` y motivo `unavailable` cuando falta un componente opcional. Conserva los fallos de comunicación del daemon solicitado y los checks requeridos. La disponibilidad de workers/backends es independiente del pool compartido. Un resultado sano no agrega un warning de resumen.
