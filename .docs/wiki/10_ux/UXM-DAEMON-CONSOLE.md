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

```yaml
harness_protocol: SDD-HARNESS-v1
id: UXM-DAEMON-CONSOLE
kind: ux-module
audience: human
imports:
  - .docs/wiki/00_governance.md
  - .docs/wiki/05_requirements/RF-DAE-002.md
  - .docs/wiki/05_requirements/RF-DAE-004.md
  - .docs/wiki/05_requirements/RF-DAE-005.md
exports:
  - UXM-DAEMON-CONSOLE
agent_must_read:
  - .docs/wiki/10_ux/UXM-DAEMON-CONSOLE.md
agent_may_edit:
  - .docs/wiki/10_ux/UXM-DAEMON-CONSOLE.md
agent_must_not_edit:
  - .docs/wiki/05_requirements/
  - internal/daemon/admin_ui.html
verify:
  - "Lectura/cotejo: comparar pantallas, literales, estados y acciones con RF-DAE-002, FL-DAE-01, TP-DAE, DESIGN.md e internal/daemon/admin_ui.html; comprobar que los tokens siguen viniendo de la implementación y que la revisión visual continúa pendiente si no hubo observación."
stop_if:
  - harness_verdict=BLOCKED
  - visual_review_is_claimed_without_observation
  - journey_or_acceptance_would_change
evidence:
  - .docs/wiki/05_requirements/RF-DAE-002.md
  - .docs/wiki/04_flows/FL-DAE-01.md
  - .docs/wiki/06_tests/TP-DAE.md
  - DESIGN.md
  - internal/daemon/admin_ui.html
  - internal/daemon/admin_ui.go
```
