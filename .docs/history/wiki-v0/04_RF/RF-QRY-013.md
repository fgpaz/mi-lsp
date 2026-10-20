# RF-QRY-013 - Diagnosticar gobernanza y perfil efectivo con nav governance

```yaml
harness_protocol: SDD-HARNESS-v1
id: "RF-QRY-013"
kind: "support-doc"
audience: "llm-first"
imports:
  - '[[00_gobierno_documental]]'
  - '[[RF-QRY-013]]'
exports:
  - 'RF-QRY-013'
agent_must_read:
  - .docs/wiki/00_gobierno_documental.md
  - .docs/wiki/04_RF/RF-QRY-013.md
agent_may_edit:
  - .docs/wiki/04_RF/RF-QRY-013.md
agent_must_not_edit:
  - .docs/wiki/_mi-lsp/read-model.toml
verify:
  - mi-lsp nav governance --workspace mi-lsp --format toon
  - mi-lsp nav wiki validate-harness --workspace mi-lsp --format toon
stop_if:
  - governance_blocked=true
  - harness_verdict=BLOCKED
evidence:
  - .docs/wiki/04_RF/RF-QRY-013.md
```

## 1. Execution Sheet

| Campo | Valor |
|---|---|
| ID | RF-QRY-013 |
| Titulo | Diagnosticar gobernanza y perfil efectivo con nav governance |
| Actores | Usuario, Skill, Agente, CLI/Core |
| Prioridad | alta |
| Severidad | alta |
| FL origen | FL-QRY-01 |

## 2. Detailed Preconditions

| Condicion | Tipo | Estado requerido |
|---|---|---|
| Workspace resoluble | funcional | obligatorio |
| `.docs/wiki/00_gobierno_documental.md` accesible | funcional | obligatorio |

## 3. Process Steps (Happy Path)

1. La CLI recibe `mi-lsp nav governance`.
2. El core inspecciona `00_gobierno_documental.md`, extrae el bloque YAML y valida el schema.
3. Resuelve el perfil efectivo y compila `base + overlays`.
4. Verifica sincronizacion con `.docs/wiki/_mi-lsp/read-model.toml`.
5. Informa si el indice del workspace quedo stale respecto de las fuentes de gobernanza e incluye timestamps comparados (`index.db`, `00`, `read-model`) para explicar la causa.
6. Devuelve pasos accionables de reparacion o siguientes pasos si la gobernanza es valida.

## 4. Typed Errors

| Codigo | Causa | Trigger | Respuesta esperada |
|---|---|---|---|
| `QRY_GOV_WORKSPACE_NOT_FOUND` | workspace invalido | alias/path no resoluble | abortar con error explicito |
| `QRY_GOV_INVALID_SCHEMA` | YAML invalido | parse/validacion falla | devolver estado bloqueado e issues |
| `QRY_GOV_INDEX_STALE` | indice stale | `00` o `read-model` mas nuevos que `index.db` | devolver warning bloqueante con timestamps y pedir `mi-lsp index --workspace <alias>` |

## 5. Special Cases and Variants

- `nav governance` es la superficie primaria de diagnostico cuando el repo esta bloqueado.
- La respuesta debe servir tanto a personas como a skills.
- El comando puede auto-sincronizar la proyeccion, pero no debe ocultar que hace falta reindex si corresponde.
- El bloque `index_sync_details` es aditivo y debe explicar `reason`, `index_path` y cada path comparado sin requerir que el agente lea el filesystem por separado.

## 6. Data Model Impact

- `GovernanceStatus`
- `DocsReadProfile`
- `QueryEnvelope`
