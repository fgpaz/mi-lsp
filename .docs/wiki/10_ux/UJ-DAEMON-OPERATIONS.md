---
wiki_contract: WIKI-CONTRACT-v1
id: UJ-DAEMON-OPERATIONS
kind: journey
status: active
title: Diagnosticar y calentar un workspace desde la consola local
links:
  experiences: [RS-MILSP-RUNTIME]
  stages: [FL-DAE-01]
---

## Objetivo

Hacer visible el daemon actual, seleccionar un workspace y diagnosticar su runtime sin otra instancia ni una GUI remota.

## Recorrido

1. El operador abre admin_url con workspace y panel opcionales.
2. La consola enfoca el workspace solicitado y muestra overview, runtimes o logs.
3. Los filtros acotan backend, cliente y ventana de eventos.
4. Warm workspace solicita el calentamiento del workspace enfocado; Refresh vuelve a leer estado.
5. Open logs muestra la cola y Copy CLI ofrece la orden del contexto seleccionado.

## Estados y límites

La respuesta real determina listo, vacío o error; un acceso fallido no significa actividad cero. Las acciones y el mensaje deben conservar el contexto del workspace. No se exponen tokens del daemon ni contenido sensible. Se describe la UI existente, no una pantalla nueva.

```yaml
harness_protocol: SDD-HARNESS-v1
id: UJ-DAEMON-OPERATIONS
kind: journey
audience: human
imports:
  - .docs/wiki/00_governance.md
  - .docs/wiki/02_outcomes/RS-MILSP-RUNTIME.md
  - .docs/wiki/04_flows/FL-DAE-01.md
exports:
  - UJ-DAEMON-OPERATIONS
agent_must_read:
  - .docs/wiki/10_ux/UJ-DAEMON-OPERATIONS.md
agent_may_edit:
  - .docs/wiki/10_ux/UJ-DAEMON-OPERATIONS.md
agent_must_not_edit:
  - .docs/wiki/05_requirements/RF-HOST-001.md
  - .docs/wiki/07_technical/TECH-DAEMON-GOBERNANZA.md
verify:
  - "Lectura/cotejo: comparar los pasos y estados workspace-first con RF-DAE-002, FL-DAE-01, TP-DAE, DESIGN.md e internal/daemon/admin_ui.html; conservar la regla de que error no significa actividad cero y no afirmar revisión visual o ejecución no observada."
stop_if:
  - harness_verdict=BLOCKED
  - observed_ui_state_is_not_supported_by_document
evidence:
  - .docs/wiki/05_requirements/RF-DAE-002.md
  - .docs/wiki/04_flows/FL-DAE-01.md
  - .docs/wiki/06_tests/TP-DAE.md
  - DESIGN.md
  - internal/daemon/admin_ui.html
  - internal/daemon/admin_ui.go
```
