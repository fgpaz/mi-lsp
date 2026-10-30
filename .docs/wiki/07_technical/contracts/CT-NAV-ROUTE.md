---
wiki_contract: WIKI-CONTRACT-v1
id: CT-NAV-ROUTE
kind: contract
status: active
title: CT-NAV-ROUTE
links:
  supports:
  - RF-QRY-014
  - RF-QRY-015
---

```yaml
harness_protocol: SDD-HARNESS-v1
id: CT-NAV-ROUTE
kind: cli-contract
audience: dual
imports: [.docs/wiki/07_technical/TECH-DOC-ROUTER.md]
exports: [CT-NAV-ROUTE]
agent_must_read: [.docs/wiki/07_technical/contracts/CT-NAV-ROUTE.md, internal/service/route.go]
agent_may_edit: [.docs/wiki/07_technical/contracts/CT-NAV-ROUTE.md]
agent_must_not_edit: [internal/service/route.go, .docs/wiki/_mi-lsp/read-model.toml]
verify: ["Leer .docs/wiki/07_technical/contracts/CT-NAV-ROUTE.md y .docs/wiki/07_technical/TECH-DOC-ROUTER.md; cotejar las afirmaciones con la fuente publicada internal/service/route.go y sus límites. Registrar únicamente comandos y resultados efectivamente ejecutados; esta especificación no afirma una ejecución."]
stop_if: [README sustituya anchor canónico, discovery altere autoridad o se agregue routing/daemon al comando directo.]
evidence: [.docs/wiki/07_technical/contracts/CT-NAV-ROUTE.md, .docs/wiki/07_technical/TECH-DOC-ROUTER.md, internal/service/route.go]
```

## Consumidor

CLI, agentes y consumidores locales de las operaciones descritas; no se amplía la autoridad del consumidor.

## Operaciones

Se conservan requests, envelopes, campos, límites y semántica detallados a continuación.

### Invocacion

```
mi-lsp nav route <task> [--workspace <alias>] [--full] [--include-code-discovery] [--format toon|compact|yaml]
```

### Semántica

Resuelve el documento canónico de anclaje y un mini reading pack previo para una tarea spec-driven.
Retorna `RouteResult` con `canonical lane` (autoritativa) y `discovery` opcional (advisory-only).
El envelope puede agregar `continuation` y `memory_pointer` como guidance de bajo costo para la siguiente consulta.
La canonical lane usa el scorer owner-aware compartido: FTS + overlap lexico + `doc_id` + stem/path + `owner_hints` opcionales; `README` solo puede ganar si no existe un candidato canónico positivo.
Si la tarea incluye un RF explícito y ese ID vive dentro de un documento agregado, Tier 1 debe anclar el documento contenedor aunque el índice documental este vacio. La respuesta conserva el `doc_id` pedido en `anchor_doc.doc_id`.

### Envelope de respuesta

```json
{
  "ok": true,
  "backend": "route",
  "workspace": "alias",
  "items": [
    {
      "task": "understand daemon routing",
      "mode": "preview",
      "canonical": {
        "anchor_doc": {
          "path": ".docs/wiki/07_technical.md",
          "title": "07. Baseline tecnica",
          "doc_id": "",
          "layer": "07",
          "family": "technical",
          "stage": "anchor",
          "why": "fts5=match,family=technical"
        },
        "preview_pack": [
          {"path": ".docs/wiki/07_technical/contracts/CT-TECHNICAL-BASELINE.md", "title": "09. Contratos tecnicos", "layer": "09", "stage": "contracts", "why": "canonical_preview"}
        ],
        "family": "technical",
        "authoritative": true
      },
      "discovery": null,
      "why": ["read_model=project", "tier2=indexed_docs", "family=technical"]
    }
  ],
  "warnings": [],
  "continuation": {
    "reason": "expand_preview",
    "next": {
      "op": "nav.pack",
      "query": "understand daemon routing",
      "full": true
    }
  },
  "memory_pointer": {
    "doc_id": "TECH-DAEMON-GOBERNANZA",
    "why": "Documento tecnico cambiado recientemente y util para reentrar.",
    "reentry_op": "workspace.status",
    "handoff": "continuation-v1",
    "stale": false
  },
  "stats": {"files": 2},
  "truncated": false
}
```

### Flags

- **Flag**: `--include-code-discovery`; **Tipo**: bool; **Default**: false; **Descripción**: Incluye discovery de código (solo en modo full)
- **Flag**: `--full`; **Tipo**: bool; **Default**: false; **Descripción**: Expande canonical lane y activa discovery advisory

### Routing daemon

`nav route` es directo (no pasa por daemon). Similar a `nav pack`.

### AXI

`nav route` es AXI-default preview-first. En modo preview: `canonical.preview_pack` puede estar truncado, `discovery` puede ser omitida. Con `--full`: canonical lane expandida y discovery incluida.
En preview, `continuation` puede sugerir el salto directo a `nav.pack --full`; `memory_pointer` puede aparecer cuando existe snapshot repo-local util.

### Casos de RF agregados

- El router no exige que cada RF tenga un archivo propio.
- Cuando el read-model apunta a `.docs/wiki/05_requirements/*.md`, Tier 1 puede leer esos markdown y buscar el `RF-*` explícito en headings o tablas.
- El `title` del anchor puede tomarse de la celda siguiente al RF en una tabla markdown cuando exista; si no, usa el primer heading del documento.

### Diferencia con nav ask y nav pack

- `nav route`: devuelve solo el anchor + mini preview, mínimo tokens
- `nav ask`: llama al route core internamente, luego agrega code evidence y summary
- `nav pack`: llama al route core internamente, luego construye reading pack completo

### Estado

implemented

### RF asociado

RF-QRY-014, RF-QRY-015

## Errores

Se mantienen las razones, warnings, rechazos y fallbacks explícitos de cada operación.

### Errores

- **Código**: `QRY_ROUTE_TASK_REQUIRED`; **Descripción**: task/question vacio
- **Código**: `QRY_ROUTE_WORKSPACE_NOT_FOUND`; **Descripción**: workspace no resolucionable
- **Código**: `QRY_ROUTE_GOVERNANCE_BLOCKED`; **Descripción**: governance bloqueada

## Compatibilidad

La migración cambia el contrato documental, no el protocolo runtime ni sus versiones. Los esquemas legacy que la herramienta consume continúan como compatibilidad de entrada, no como formato de autoría de esta wiki.
