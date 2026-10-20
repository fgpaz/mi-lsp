---
wiki_contract: WIKI-CONTRACT-v1
id: RF-QRY-022
kind: requirement
status: active
title: RF-QRY-022 — Exponer IDs q-v1, primitivas semánticas y memoria de sesión
links:
  implements:
  - FL-QRY-01
  code:
  - path: internal/service/app.go
    role: behavior
---

## Propósito

RF-QRY-022 — Exponer IDs q-v1, primitivas semánticas y memoria de sesión

## Comportamiento ordenado

### Resultado requerido

Exponer identidad estable por workspace, primitivas componibles y memoria acotada de sesión como parte del contrato [[CT-Q-V1]]. La revisión de contenido nunca se confunde con identidad ni se incorpora al `id` canónico.

### Primitivas y proyección

La API q reutiliza `sym`, `edges`, `read`, `text` y `docs`, más las fuentes `id`, `diff` y `changed`. Las transformaciones `where`, `limit`, `uniq`, `sort` y la salida `fields`, `count` componen sus resultados. Cada ítem conserva `origin`; las relaciones señalan `edge.dir`, `edge.from` y `edge.depth`; texto y documentación incluyen referencia al elemento de origen cuando corresponde.

- Identidad: `ws:<workspace_id>/<kind>/<path>#<symbol>`; revisión separada `rev:<sha256>`, conforme al escape, colisiones, compatibilidad legacy y resolución de stale/missing definidos por CT-Q-V1.
- La búsqueda/resolución de un identificador se limita al workspace embebido y seleccionado; un desajuste falla con `stage_failed` y nunca habilita acceso cruzado implícito.
- `budget` limita la proyección estimada en tokens; `fields` selecciona campos de salida y conserva los estados de resolución. La truncación ocurre entre ítems y ofrece cursor ligado al snapshot.
- `edges` admite `refs|callers|callees|impl`, profundidad 1–3, máximo 50 resultados por fuente y 200 por etapa. Un grafo ausente se expresa como resultado degradado tipado, no como falso vacío semántico.

### Memoria de sesión

Cada solicitud lleva `session_id` explícito. [RETIRADO 2026-10-15] MCP genera una identidad estable por proceso cuando el caller no la aporta. La memoria en proceso es LRU, máximo 200 sesiones, expira tras 30 minutos idle y conserva solo IDs/revisiones, nunca texto ni consultas. `read` puede omitir texto ya entregado para el mismo ID/revisión (`seen:true`); `fresh` lo vuelve a incluir. `session_mark` es monotónico. `changed since=<mark>` compara el snapshot entregado con la resolución actual y devuelve revisiones cambiadas, sin registrar ni indexar.

La caché/dedupe es opt-in. Sin `dedupe=true`, repetir una consulta conserva sus resultados y orden. Cancelación no publica texto ni resultados parciales en memoria como si fueran una lectura completa.

## Superficies

CLI `mi-lsp`, con operaciones y selectores detallados en el comportamiento del requisito.

## Entradas/salidas tipadas

Las entradas, salidas, enums y límites forman parte del contrato de comportamiento ordenado de este documento; los campos conservan sus nombres de protocolo.

## Errores

### Errores y solo lectura

Todos los errores de timeout, cancelación, cursor, snapshot, etapa y presupuesto usan exclusivamente los códigos congelados por [[CT-Q-V1]]. Cancelación vence al timeout, y timeout vence a fallo de etapa cuando concurren. Si hay salida anterior preservable se devuelve `ok:true`, `partial:true`, razón de la primera falla y etapas completadas; en otro caso `ok:false`.

Las primitivas no mutan índice, workspace registry ni archivos; no usan red. Una consulta directa conserva el mismo envelope y contrato que la ejecución por daemon.

## Criterios de aceptación (numerados)

1. `sym` genera ID con workspace, kind y path estable, revision completa separada, normalización LF y escape percent-encoding determinista.
2. Reindexar sin cambiar un símbolo conserva su ID; cambiar contenido conserva la identidad, actualiza `rev` y marca `stale:true`; una identidad ausente produce `missing:true`.
3. `read` soporta ID/rango, evita repetir texto en la misma sesión y `fresh` fuerza su inclusión.
4. `changed since` re-resuelve el snapshot; sesión limitada/idle expira sin crecimiento ilimitado.
5. Presupuesto y fan-out se aplican por pipeline/etapa; cancelación y deadline son cooperativos end-to-end.

### Aceptación

- `sym` genera ID con workspace, kind y path estable, revision completa separada, normalización LF y escape percent-encoding determinista.
- Reindexar sin cambiar un símbolo conserva su ID; cambiar contenido conserva la identidad, actualiza `rev` y marca `stale:true`; una identidad ausente produce `missing:true`.
- `read` soporta ID/rango, evita repetir texto en la misma sesión y `fresh` fuerza su inclusión.
- `changed since` re-resuelve el snapshot; sesión limitada/idle expira sin crecimiento ilimitado.
- Presupuesto y fan-out se aplican por pipeline/etapa; cancelación y deadline son cooperativos end-to-end.
- Casos: TP-QRY-194 a TP-QRY-196.

- **Marcador estable**: rf-qry-022-q-v1-primitives
- **Estado del comportamiento**: implemented
- **Comprobación prevista**:
  - go test ./internal/query ./internal/service
  - mi-lsp nav wiki validate-source --workspace <alias> --ids RF-QRY-022 --format toon
- **Evidencia referenciada**:
  - .docs/wiki/07_technical/contracts/CT-Q-V1.md
  - .docs/wiki/06_tests/TP-QRY.md
  - internal/query/parser_test.go
  - internal/service/primitives_q_test.go
- **Aceptación**:
  - **identity_and_revision_are_separate**: true
  - **query_is_workspace_scoped**: true
  - **projection_is_bounded_and_cursor_uses_snapshot**: true
  - **session_memory_is_opt_in_and_text_free**: true
  - **operations_are_read_only**: true
