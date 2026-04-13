# RF-QRY-013 - Diagnosticar gobernanza y perfil efectivo con nav governance

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
5. Informa si el indice del workspace quedo stale respecto de las fuentes de gobernanza.
6. Devuelve pasos accionables de reparacion o siguientes pasos si la gobernanza es valida.

## 4. Typed Errors

| Codigo | Causa | Trigger | Respuesta esperada |
|---|---|---|---|
| `QRY_GOV_WORKSPACE_NOT_FOUND` | workspace invalido | alias/path no resoluble | abortar con error explicito |
| `QRY_GOV_INVALID_SCHEMA` | YAML invalido | parse/validacion falla | devolver estado bloqueado e issues |
| `QRY_GOV_INDEX_STALE` | indice stale | `00` o `read-model` mas nuevos que `index.db` | devolver warning bloqueante y pedir reindex |

## 5. Special Cases and Variants

- `nav governance` es la superficie primaria de diagnostico cuando el repo esta bloqueado.
- La respuesta debe servir tanto a personas como a skills.
- El comando puede auto-sincronizar la proyeccion, pero no debe ocultar que hace falta reindex si corresponde.

## 6. Data Model Impact

- `GovernanceStatus`
- `DocsReadProfile`
- `QueryEnvelope`
