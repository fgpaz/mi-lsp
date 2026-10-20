---
wiki_contract: WIKI-CONTRACT-v1
id: RF-DAE-005
kind: requirement
status: active
title: Estadísticas de telemetría local por cliente
links:
  implements:
  - FL-DAE-01
  code:
  - path: internal/cli/stats.go
    role: behavior
  - path: internal/daemon/export.go
    role: behavior
  - path: internal/daemon/state_store.go
    role: behavior
---

## Propósito

Estadísticas de telemetría local por cliente

## Comportamiento ordenado

- **Marcador estable**: rf-dae-005-telemetry-stats
- **Estado del comportamiento**: implemented
- **Propósito**: resumir telemetría local por cliente sin exponer contenido de consultas ni archivos
- **Entradas**:
  - **command**: mi-lsp stats
  - **flags**:
    - **by_client**: boolean, optional
    - **by_op**: boolean, optional
    - **days**: positive_integer, default=7
    - **format**: json|compact, default=json
- **Comportamiento**:
  - Si no se selecciona --by-client ni --by-op, mostrar el desglose por cliente.
  - Leer access_events local desde daemon.db; no iniciar navegación semántica ni enviar datos a red.
  - Agrupar por client_name o 'unknown' si falta.
  - Para cada grupo, informar calls, ok_pct, p50_ms, p90_ms, p50_bytes, p90_bytes y fallback_pct.
  - Considerar fallback los eventos con route=direct_fallback o routing_outcome que indique fallback.
  - La salida solo incluye agregados; no incluir patrones, texto, argv, errores crudos ni contenido de archivos.
  - by-op agrega un desglose análogo por operación; puede combinarse con --by-client.
- **Salidas**:
  - **fields**: [days, total_calls, by_client?, by_op?]
  - **group_fields**: [calls, ok_pct, p50_ms, p90_ms, p50_bytes, p90_bytes, fallback_pct]
- **Implementación**:
  - internal/cli/stats.go
  - internal/daemon/export.go
  - internal/daemon/state_store.go
- **Comprobaciones previstas**:
  - TP-DAE/TC-DAE-027
  - TP-DAE/TC-DAE-028
- **Límites**:
  - El error al abrir o consultar daemon.db se informa; no se presenta como cero actividad.
  - days <= 0 se rechaza sin consulta.
  - Los agregados no contienen valores de consulta ni paths.
- **Comprobación prevista**:
  - go build ./cmd/mi-lsp ./internal/cli ./internal/daemon
  - mi-lsp nav wiki validate-source --workspace <alias> --ids RF-DAE-005 --format toon
- **Evidencia referenciada**:
  - .docs/wiki/05_requirements/RF-DAE-005.md
  - .docs/wiki/06_tests/TP-DAE.md
  - internal/cli/stats.go
  - internal/daemon/export.go
  - internal/daemon/state_store.go

## Superficies

- mi-lsp stats

## Entradas/salidas tipadas

Las entradas, salidas, enums y límites forman parte del contrato de comportamiento ordenado de este documento; los campos conservan sus nombres de protocolo.

## Errores

Se aplican las condiciones de rechazo y los warnings descritos en el comportamiento ordenado y sus variantes. No se añade ningún fallback silencioso.

## Criterios de aceptación (numerados)

1. `mi-lsp stats --by-client --days 7 --format json` agrupa los eventos por `client_name` (o `unknown`) e informa `calls`, `ok_pct`, p50/p90 de latencia, p50/p90 de bytes y `fallback_pct`, sin incluir contenido de consultas ni archivos.
2. Cuando se solicita `--by-op`, la salida agrega el mismo conjunto de métricas por operación y puede combinarse con `--by-client`; sin esos flags, el desglose por cliente es el predeterminado.
3. Un valor `--days` menor o igual a cero se rechaza antes de consultar `daemon.db`.
4. Un error al abrir o consultar `daemon.db` se informa explícitamente y no se representa como cero actividad.
5. Los agregados no exponen patrones, texto, argv, errores crudos, paths ni contenido de archivos; los eventos de fallback se cuentan solo cuando `route=direct_fallback` o `routing_outcome` indica fallback.
