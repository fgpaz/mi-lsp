---
wiki_contract: WIKI-CONTRACT-v1
id: RS-MILSP-RUNTIME
kind: outcome
status: active
title: Uso local portable con daemon opcional y diagnóstico explícito
---

## Resultado observable

La CLI funciona con selección de workspace confiable y sin exigir daemon. Los runtimes especializados y la consola loopback permiten operar y diagnosticar el sistema sin convertir estado local en configuración portable.

## Beneficiario

Desarrolladores, agentes y operadores locales del daemon.

## Criterios de éxito

- Init y el registro conservan la topología single/container y las raíces de canon.
- Los workers se instalan explícitamente; no se descargan durante una consulta.
- La CLI portable se mantiene mediante un único updater runtime, con estado verificable, pin explícito y respeto del trabajo vivo; ver [[FL-UPD-001]] y [[RF-UPD-001]].
- Fallback, carga de runtime, estado y errores siguen visibles y accionables.
- Probe no crea DB, WAL/SHM, registry ni telemetría.
- La consola muestra workspaces, runtimes, accesos y logs del daemon actual; el warm no crea otra instancia.
- La telemetría contiene agregados sanitizados, no patrones, contenido, tokens ni secretos.
- El host Windows separa transporte y resultado de ejecución y espera el árbol completo; la admisión del consumidor y la ausencia de ventanas requieren evidencia de [[RF-HOST-001]], [[FL-HOST-001]] y [[TP-HOST-001]].

## Fuera de alcance del RS

Setup remoto, dashboard público, nueva puerta MCP, migraciones automáticas de estado o almacenamiento de secretos en la wiki.

```yaml
harness_protocol: SDD-HARNESS-v1
id: RS-MILSP-RUNTIME
kind: outcome
audience: dual
imports:
  - .docs/wiki/00_governance.md
exports:
  - RS-MILSP-RUNTIME
agent_must_read:
  - .docs/wiki/02_outcomes/RS-MILSP-RUNTIME.md
agent_may_edit:
  - .docs/wiki/02_outcomes/RS-MILSP-RUNTIME.md
agent_must_not_edit:
  - .docs/wiki/05_requirements/RF-HOST-001.md
verify:
  - "Lectura/cotejo: contrastar topología/workspace y consola con RF-WKS-008, RF-DAE-002/003/004/005 y FL-DAE-01; updater con RF-UPD-001/FL-UPD-001/TP-UPD-001; Windows con RF-HOST-001/FL-HOST-001/TP-HOST-001. Preservar disposiciones pendientes y no inferir ejecución de planes o evidencia histórica."
stop_if:
  - harness_verdict=BLOCKED
  - outcome_status_or_acceptance_would_change
evidence:
  - .docs/wiki/05_requirements/RF-WKS-008.md
  - .docs/wiki/05_requirements/RF-DAE-002.md
  - .docs/wiki/05_requirements/RF-DAE-003.md
  - .docs/wiki/05_requirements/RF-DAE-004.md
  - .docs/wiki/05_requirements/RF-DAE-005.md
  - .docs/wiki/05_requirements/RF-UPD-001.md
  - .docs/wiki/05_requirements/RF-HOST-001.md
  - .docs/wiki/04_flows/FL-DAE-01.md
  - .docs/wiki/04_flows/FL-UPD-001.md
  - .docs/wiki/04_flows/FL-HOST-001.md
  - .docs/wiki/06_tests/TP-DAE.md
  - .docs/wiki/06_tests/TP-UPD-001.md
  - .docs/wiki/06_tests/TP-HOST-001.md
  - internal/daemon/admin_ui.html
  - internal/daemon/update.go
```
