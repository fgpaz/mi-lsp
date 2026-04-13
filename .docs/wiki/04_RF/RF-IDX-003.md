# RF-IDX-003 - Proyectar y sincronizar el read-model desde 00_gobierno_documental.md

## 1. Execution Sheet

| Campo | Valor |
|---|---|
| ID | RF-IDX-003 |
| Titulo | Proyectar y sincronizar el read-model desde 00_gobierno_documental.md |
| Actores | Maintainer de wiki, Skill, CLI/Core |
| Prioridad | alta |
| Severidad | alta |
| FL origen | FL-IDX-01 |

## 2. Detailed Preconditions

| Condicion | Tipo | Estado requerido |
|---|---|---|
| `00_gobierno_documental.md` presente | funcional | obligatorio |
| Bloque YAML de gobernanza valido | funcional | obligatorio |
| Ruta de proyeccion canonica resoluble | tecnica | obligatorio |

## 3. Process Steps (Happy Path)

1. El core lee `.docs/wiki/00_gobierno_documental.md`.
2. Extrae el bloque YAML de gobernanza y valida schema comun estricto.
3. Compila el perfil visible (`ordered_wiki`, `spec_backend`, `spec_full`, `custom`) a `base + overlays`.
4. Proyecta `.docs/wiki/_mi-lsp/read-model.toml` como contrato ejecutable versionado.
5. Si el projection drift existe, auto-sincroniza el archivo de proyeccion.
6. Si cambia `00` o cambia la proyeccion, el workspace debe reindexarse antes de continuar con superficies docs-first.

## 4. Typed Errors

| Codigo | Causa | Trigger | Respuesta esperada |
|---|---|---|---|
| `IDX_GOV_DOC_MISSING` | falta `00` | doc inexistente | bloquear y explicar reparacion |
| `IDX_GOV_YAML_INVALID` | YAML invalido o incompleto | parse/validacion falla | bloquear y listar issues |
| `IDX_GOV_PROJECTION_WRITE_FAILED` | no se pudo escribir la proyeccion | permisos/ruta | bloquear con error accionable |

## 5. Special Cases and Variants

- La proyeccion siempre apunta a `.docs/wiki/_mi-lsp/read-model.toml`.
- `00` manda; `read-model.toml` nunca redefine la autoridad humana.
- El auto-sync no habilita continuar si el indice repo-local quedo stale respecto de `00` o del `read-model`.
- El perfil `custom` solo es valido si extiende un perfil canónico y respeta el schema.

## 6. Data Model Impact

- `GovernanceSource`
- `DocsGovernanceProfile`
- `DocsReadProfile`
- `QueryEnvelope`
