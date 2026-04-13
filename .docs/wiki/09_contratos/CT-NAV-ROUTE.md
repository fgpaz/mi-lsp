# CT-NAV-ROUTE

## Invocacion

```
mi-lsp nav route <task> [--workspace <alias>] [--full] [--include-code-discovery] [--format toon|compact|yaml]
```

## Semantica

Resuelve el documento canonico de anclaje y un mini reading pack previo para una tarea spec-driven.
Retorna `RouteResult` con `canonical lane` (autoritativa) y `discovery` opcional (advisory-only).

## Envelope de respuesta

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
          "path": ".docs/wiki/07_baseline_tecnica.md",
          "title": "07. Baseline tecnica",
          "doc_id": "",
          "layer": "07",
          "family": "technical",
          "why": "fts5=match,family=technical"
        },
        "preview_pack": [
          {"path": ".docs/wiki/09_contratos_tecnicos.md", "title": "09. Contratos tecnicos", "layer": "09", "stage": "contracts", "why": "canonical_preview"}
        ],
        "family": "technical",
        "authoritative": true
      },
      "discovery": null,
      "why": ["read_model=project", "tier2=indexed_docs", "family=technical"]
    }
  ],
  "warnings": [],
  "stats": {"files": 2},
  "truncated": false
}
```

## Flags

| Flag | Tipo | Default | Descripcion |
|---|---|---|---|
| `--include-code-discovery` | bool | false | Incluye discovery de codigo (solo en modo full) |
| `--full` | bool | false | Expande canonical lane y activa discovery advisory |

## Routing daemon

`nav route` es directo (no pasa por daemon). Similar a `nav pack`.

## AXI

`nav route` es AXI-default preview-first. En modo preview: `canonical.preview_pack` puede estar truncado, `discovery` puede ser omitida. Con `--full`: canonical lane expandida y discovery incluida.

## Errores

| Codigo | Descripcion |
|---|---|
| `QRY_ROUTE_TASK_REQUIRED` | task/question vacio |
| `QRY_ROUTE_WORKSPACE_NOT_FOUND` | workspace no resolucionable |
| `QRY_ROUTE_GOVERNANCE_BLOCKED` | governance bloqueada |

## Diferencia con nav ask y nav pack

- `nav route`: devuelve solo el anchor + mini preview, minimo tokens
- `nav ask`: llama al route core internamente, luego agrega code evidence y summary
- `nav pack`: llama al route core internamente, luego construye reading pack completo

## Estado

implemented

## RF asociado

RF-QRY-014, RF-QRY-015
