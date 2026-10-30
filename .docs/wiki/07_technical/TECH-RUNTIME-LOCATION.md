---
wiki_contract: WIKI-CONTRACT-v1
id: TECH-RUNTIME-LOCATION
kind: technical
status: draft
title: Ubicación del runtime y recibos
links:
  code:
    - path: internal/runtimeinfo/location.go
      role: behavior
    - path: internal/runtimeinfo/receipts.go
      role: behavior
---

## Resumen

Ubicación recomendada y recibos del runtime, sin atribuir causa a una ausencia.

## Mapa de componentes

`internal/runtimeinfo/` calcula rutas y gestiona recibos; repair los consume.

# Ubicación del runtime y recibos

```yaml
harness_protocol: SDD-HARNESS-v1
id: TECH-RUNTIME-LOCATION
kind: technical-operations
audience: dual
imports: [.docs/wiki/07_technical/TECH-REPAIR.md]
exports: [TECH-RUNTIME-LOCATION]
agent_must_read: [.docs/wiki/07_technical/TECH-RUNTIME-LOCATION.md, internal/runtimeinfo/location.go]
agent_may_edit: [.docs/wiki/07_technical/TECH-RUNTIME-LOCATION.md]
agent_must_not_edit: [internal/runtime/, ~/.mi-lsp/repair/installations.json]
verify: ["Leer .docs/wiki/07_technical/TECH-RUNTIME-LOCATION.md y .docs/wiki/07_technical/TECH-REPAIR.md; cotejar las afirmaciones con la fuente publicada internal/runtimeinfo/location.go y sus límites. Registrar únicamente comandos y resultados efectivamente ejecutados; esta especificación no afirma una ejecución."]
stop_if: [Una modificación cree archivos durante consultas de ubicación o trate recibos inválidos como autorizados para reemplazo.]
evidence: [.docs/wiki/07_technical/TECH-RUNTIME-LOCATION.md, .docs/wiki/07_technical/TECH-REPAIR.md, internal/runtimeinfo/location.go]
```

`version --format json` informa en `items[0].runtime` la ruta real del ejecutable, la raíz gestionada recomendada, la ruta del ejecutable gestionado y la ruta de recibos. No registra ni instala nada. `capabilities` incluye `repair-v1` en la integración que incorpora repair; no implica que releases anteriores la tengan.

La raíz gestionada es `LOCALAPPDATA/mi-lsp-runtime` en Windows (fallback `AppData/Local` bajo el directorio de usuario) y `XDG_DATA_HOME/mi-lsp-runtime` en Unix (fallback `.local/share` bajo el directorio de usuario). Es una recomendación, no prueba de instalación. Se resuelven enlaces del ejecutable cuando es posible; si no se puede determinar una ruta base absoluta, el campo correspondiente queda vacío.

Los recibos `~/.mi-lsp/repair/installations.json` usan `mi-lsp-installations/v1`, con componente `runtime` o `worker`, path absoluto y fecha RFC3339 UTC. Solo se registran artefactos existentes. Sin recibo previo, la ausencia no prueba remoción; un recibo válido cuyo path ya no existe sí permite informar remoción, sin atribuirla a una causa. Los recibos inválidos generan error y no se reemplazan. Los escritores usan bloqueo de archivo del sistema operativo no bloqueante y reemplazo JSON mediante archivo temporal en el mismo directorio. Consultar ubicación o recibos no crea archivos.
