---
wiki_contract: WIKI-CONTRACT-v1
id: UXM-DAEMON-CONSOLE
kind: ux_module
status: active
title: Consola local de gobernanza del daemon
links:
  maps: [RF-DAE-002, RF-DAE-004, RF-DAE-005]
---

## Pantallas y roles

Operador local: consola única workspace-first en admin_url loopback. Incluye metadatos del daemon, KPI, filtros, pestañas por workspace, panel principal/secundario, drawer y logs. No es un panel público ni otro servicio.

## Estados, mensaje y acción

- Sin selección: elegir un workspace para enfocar contexto y acciones.
- Estado disponible: mostrar métricas y runtimes; Refresh relee, Copy CLI conserva el contexto.
- Warm solicitado: reflejar el resultado de la operación; no asumir que un runtime existe antes de la respuesta.
- Logs: Open logs muestra líneas numeradas y evita crecimiento ilimitado.
- Error o acceso fallido: aviso visible con acción de refresco o diagnóstico; no presentar cero actividad como éxito.
- Watcher y límites: mostrar roots, directorios y pendientes conforme al contrato del daemon.

## Copy existente

Los controles de la implementación se llaman Search, Backend, Client, Window, Panel, Refresh, Warm workspace, Open logs y Copy CLI. El contenido documental está en español; los literales de interfaz se conservan.

## Decisiones y preguntas abiertas

Los tokens y componentes se toman de internal/daemon/admin_ui.html, descritos por DESIGN.md; no se duplican valores. No se rediseña ni se añaden acciones en esta migración. La revisión visual y de comportamiento queda a cargo del owner de FINAL_VERIFY.
