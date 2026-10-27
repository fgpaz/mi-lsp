---
wiki_contract: WIKI-CONTRACT-v1
id: UJ-RUNTIME-UPDATE
kind: journey
status: active
title: Mantener la CLI portable sin interrumpir trabajo vivo
links:
  experiences: [RS-MILSP-RUNTIME]
  stages: [FL-UPD-001]
---

## Actor y objetivo

El desarrollador usa la CLI o un asistente consumidor del mismo runtime. Quiere recibir una versión verificada sin bloquear su consulta ni perder una sesión existente. El actor no implica permisos adicionales; no se requieren admin ni cambios del sistema.

## Recorrido y estados

1. El desarrollador ejecuta una operación normal. El runtime agenda la comprobación fuera del camino de respuesta; los consumidores no descargan otra copia por su cuenta.
2. Si no hay actualización, la persona continúa sin aviso. Red no disponible, lock ocupado y trabajo vivo difieren la operación sin ruido ni falsa confirmación.
3. Tras promoción y readback correctos, un consumidor puede avisar una sola vez por last_check: de previous a installed. installed es el disco; el daemon puede conservar la imagen anterior mientras exista trabajo.
4. Para inspeccionar, la persona usa update --check --format json. Un estado failed/deferred legible conserva exit0; ausencia/ilegibilidad no inventa éxito. La comprobación no escribe ni contacta red.
5. La persona puede optar por MI_LSP_AUTO_UPDATE=0 o un pin explícito; user-bin no es pin por ubicación. update permite aplicación explícita verificada.
6. El trabajo existente termina antes del reciclado idle. Una sesión MCP stdio nueva usa el ejecutable nuevo; no se mata una sesión ajena para forzar adopción.

## Límites de evidencia

Es un recorrido CLI/notificación, no una pantalla nueva ni una afirmación de legibilidad visual. La fábrica de procesos y las pruebas runtime sustentan comportamiento técnico; la fuente pública y los readbacks de release permanecen separados en TP-UPD-001. Las versiones antiguas publicadas sin updater requieren primera adopción manual, no capacidad retroactiva.
