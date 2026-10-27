---
wiki_contract: WIKI-CONTRACT-v1
id: RF-WIKI-007
kind: requirement
status: active
title: RF-WIKI-007 - Resolver la raíz de wiki con `nav wiki-root`
links:
  implements:
  - FL-WIKI-01
  code:
  - path: internal/cli/nav.go
    role: behavior
  - path: internal/service/wiki_root.go
    role: behavior
---

## Propósito

RF-WIKI-007 - Resolver la raíz de wiki con `nav wiki-root`

## Comportamiento ordenado

### Resultado requerido

- **Marcador estable**: RF-WIKI-007.behavior
- **Estado del comportamiento**: implemented
- **actor**: Usuario|Skill|Agente|CLI
- **origin**: FL-WIKI-01
- **command**: mi-lsp nav wiki-root --workspace <alias> [--role producto|ecosistema|gobierno_local] [--format compact|json|text|toon|yaml]
- **alias**: mi-lsp nav wiki root
- **operation**: nav.wiki-root
- **backend**: wiki-root
- **protocol**: mi-lsp-v1.1
- **Intención**: publicar la raíz de wiki portable para que skills y agentes no hardcodeen .docs/wiki
- **envelope_items**:
  - wiki_root
  - role
  - workspace
  - governance_doc
  - resolved_from
  - id
- **setup_item_fields** (solo con contrato Setup): `state` (`resolved`|`pending`) y `reason_code` cuando queda pendiente.
- **resolved_from**:
  - canon.<id>
  - default
  - registry.link
  - setup.azure_wiki
  - setup.repo_folder
- **sin_setup_ni_canon**:
  - **wiki_root**: .docs/wiki
  - **governance_doc**: literal legacy de compatibilidad devuelto cuando no hay Setup ni canon; véase la nota de compatibilidad siguiente.
  - **resolved_from**: default
  - **error**: false
- **declared_canon_governance**:
  - **source**: source_doc existente y validado del read-model aplicable
  - **missing_source**: governance_doc vacío y warning explícito; nunca un nombre inventado
  - **profile_paths**: relativos al canon propietario; precedencia local sin cambios
  - **downstream_contract**: [[CT-NAV-WIKI]] conserva el ancla gobernada en route/pack aunque no esté indexada
  - **combined_oracle**: TestDeclaredCanonUnindexedAnchorSurvivesRankedSupport
- **paths**: canons/default conservan rutas relativas portables con `/`; la fuente local de Setup devuelve una ruta relativa al workspace cuando se puede representar y, si está en otro volumen, la ruta absoluta configurada.
- **role_flag**: sin contrato Setup filtra [[RF-WKS-008|canon]] o CanonLinks por role; si no hay coincidencia, error. Con fuente Setup, devuelve esa única raíz y no inventa un role.
- **precedencia**:
  - `wiki.setup_source` compartido de Setup v1/v2 y `paths.wiki_checkout` / `paths.workspace_root` locales
  - sin contrato Setup: [[RF-WKS-008|canon]] del proyecto
  - sin contrato Setup ni canons del proyecto: CanonLinks del registro
  - default legacy `.docs/wiki` solo cuando no existe contrato Setup
- **clasificación_setup**: `.sandinas/project.yaml` legible sin clave `schema` no configura Setup, aunque contenga `wiki.canon_roots`; se aplica el camino publicado sin Setup y `governance.source_doc` validado. Una clave `schema` presente desconocida, vacía, nula o no textual produce `setup_schema_unsupported`; YAML ilegible produce `setup_contract_unreadable`.
- **fuente_setup_ausente**: item con `state=pending`, `reason_code` tipado y sin `wiki_root`; nunca volver a `.docs/wiki`, registrar otro workspace ni declarar PASS de validación.
- **fuera_de_alcance**:
  - fan-out --all-workspaces
  - cambio de versión del protocolo (los dos campos de estado son aditivos)
  - mutaciones del registro o del workspace
- **Comprobación prevista**:
  - go test ./internal/cli -count=1 -run TestNavWikiRootCommandExists
  - go test ./internal/service -count=1 -run 'TestWikiRootResolvesCanonRelativeToWorkspaceRoot|TestWikiRootNoCanonDefaults|TestWikiRootRoleMissingMatchErrors'
- **Condiciones de rechazo**:
  - contrato Setup malformado/no compatible o fuente seleccionada no disponible: devolver `state=pending`, `reason_code` tipado y sin raíz; no sustituir por `.docs/wiki`
  - nunca auto-registrar un checkout de wiki ni leer fuera de la raíz Setup/canon declarada
  - un canon del proyecto sin match por role sigue siendo error tipado; nunca afirmar PASS
- **Evidencia referenciada**:
  - internal/service/wiki_root.go
  - internal/service/wiki_root.go
  - internal/cli/nav.go
  - .docs/wiki/06_tests/TP-WIKI.md

## Superficies

- mi-lsp nav wiki-root --workspace <alias> [--role producto|ecosistema|gobierno_local] [--format compact|json|text|toon|yaml]
- mi-lsp nav wiki root
- nav.wiki-root

## Entradas/salidas tipadas

Las entradas, salidas, enums y límites forman parte del contrato de comportamiento ordenado de este documento; los campos conservan sus nombres de protocolo.

## Errores

Se aplican las condiciones de rechazo y los warnings descritos en el comportamiento ordenado y sus variantes. No se añade ningún fallback silencioso.

### Compatibilidad del default runtime

Sin contrato Setup, `internal/service/wiki_root.go` conserva el default histórico `.docs/wiki` y `defaultGovernanceDocPath` (`.docs/wiki/00_gobierno_documental.md`). Ese literal de protocolo no es un enlace a una fuente activa ni cambia por renombrar la wiki. La fuente activa del repo es `00_governance.md`; para canon declarado se conserva el `source_doc` real, existente y validado, nunca un basename sintetizado. Cuando hay Setup v1/v2, su fuente local resuelta toma precedencia; una fuente pendiente no hereda el default legacy.

## Criterios de aceptación (numerados)

1. nav wiki-root y nav wiki root son el mismo operation nav.wiki-root
2. sin Setup config se conservan la precedencia canon/registry y el default legacy `.docs/wiki`
3. con Setup v1/v2 resuelve `repo_folder` o `azure_wiki` desde los paths locales declarados; `wiki_root` identifica exactamente esa fuente
4. una fuente Setup ausente/inválida produce `state=pending` y `reason_code`, sin sustituir `.docs/wiki`
5. `index --docs-only` incluye Markdown de la raíz Setup; las validaciones de wiki acotan su corpus a esa raíz, con los mismos guards de path/symlink y sin registrar otro workspace
6. consumers resuelven wiki_root con este comando, no con un path fijo

### Aceptación

- **Marcador estable**: RF-WIKI-007.acceptance
- **Casos positivos**:
  - TC-WIKI-034
  - TC-WIKI-035
  - TC-WIKI-036
- **Casos negativos**:
  - TC-WIKI-037
- **Invariantes**:
  - nav wiki-root y nav wiki root son el mismo operation nav.wiki-root
  - sin contrato Setup ni [[RF-WKS-008|canon]], no hay error; el default legacy es `.docs/wiki`
  - las raíces Setup pueden ser absolutas solo si están en otro volumen; las demás conservan rutas relativas
  - consumers resuelven wiki_root con este comando, no con un path fijo
- **Comprobación prevista**:
  - mi-lsp nav wiki trace RF-WIKI-007 --workspace mi-lsp --format toon
- **Evidencia referenciada**:
  - .docs/wiki/06_tests/TP-WIKI.md
