---
wiki_contract: WIKI-CONTRACT-v1
id: TECH-WORKSPACE-STATUS
kind: technical
status: draft
title: Estado del workspace
links:
  supports: [RF-WKS-005]
  code:
    - path: internal/service/workspace_ops.go
      role: behavior
    - path: internal/cli/workspace.go
      role: behavior
---

## Resumen

Vista de estado del workspace, conteos, readiness y límites de presentación.

## Mapa de componentes

CLI construye el payload; servicio resuelve y presenta estado y estadísticas.

# Estado del workspace

```yaml
harness_protocol: SDD-HARNESS-v1
id: TECH-WORKSPACE-STATUS
kind: technical-contract
audience: dual
imports: [.docs/wiki/07_technical/TECH-DAEMON-GOBERNANZA.md, .docs/wiki/07_technical.md]
exports: [TECH-WORKSPACE-STATUS]
agent_must_read: [.docs/wiki/07_technical/TECH-WORKSPACE-STATUS.md, internal/service/workspace_ops.go]
agent_may_edit: [.docs/wiki/07_technical/TECH-WORKSPACE-STATUS.md]
agent_must_not_edit: [internal/service/workspace_ops.go, .mi-lsp/index.db, .docs/wiki/_mi-lsp/read-model.toml]
verify: ["Leer .docs/wiki/07_technical/TECH-WORKSPACE-STATUS.md y .docs/wiki/07_technical/TECH-DAEMON-GOBERNANZA.md; cotejar las afirmaciones con la fuente publicada internal/service/workspace_ops.go y sus límites. Registrar únicamente comandos y resultados efectivamente ejecutados; esta especificación no afirma una ejecución."]
stop_if: [Una interpretación convierta truncation en corrupción o suponga que truncation siempre recorta entrypoints.]
evidence: [.docs/wiki/07_technical/TECH-WORKSPACE-STATUS.md, .docs/wiki/07_technical/TECH-DAEMON-GOBERNANZA.md, internal/service/workspace_ops.go]
```

`workspace status` devuelve `entrypoint_count` para la topología completa y `stats.files`/`stats.symbols` para el catálogo consultado, no para el tamaño de la lista presentada. El índice puede estar ausente o desactualizado: consultar `index_ready` y las advertencias por separado. Los conteos cero de `stats` pueden omitirse por el contrato JSON existente y equivalen a cero, no a un conteo desconocido.

La vista completa incluye `entrypoints`; la vista AXI preview puede omitir listas aunque no se use el flag. `--full` solicita la vista completa. `--no-entrypoints` omite esa lista, pero conserva `entrypoint_count`; equivale a `no_entrypoints: true` en el payload de `workspace.status`. Es útil para clientes que solo necesitan los conteos. La opción no modifica el índice.

`truncated: true` indica un límite de salida, no que el índice sea inválido. Puede deberse al presupuesto de caracteres o al límite de elementos del sobre; inspeccionar `omissions` y `next_hint`. En particular, el limitador actual no recorta individualmente la lista anidada: si un único elemento supera el presupuesto, lo conserva y marca el exceso. Un límite de renderizado explícito puede cortar el texto, incluso JSON; para lectura estructurada elegir un presupuesto suficiente. No tratar `truncated` como prueba de corrupción ni asumir que siempre significa que se recortaron entrypoints.
