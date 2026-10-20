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
- **resolved_from**:
  - canon.<id>
  - default
  - registry.link
- **no_canon**:
  - **wiki_root**: .docs/wiki
  - **governance_doc**: literal legacy de compatibilidad devuelto por el caso sin canon; véase la nota de compatibilidad siguiente.
  - **resolved_from**: default
  - **error**: false
- **declared_canon_governance**:
  - **source**: source_doc existente y validado del read-model aplicable
  - **missing_source**: governance_doc vacío y warning explícito; nunca un nombre inventado
  - **profile_paths**: relativos al canon propietario; precedencia local sin cambios
  - **downstream_contract**: [[CT-NAV-WIKI]] conserva el ancla gobernada en route/pack aunque no esté indexada
  - **combined_oracle**: TestDeclaredCanonUnindexedAnchorSurvivesRankedSupport
- **paths**: portable relative with /; never absolute
- **role_flag**: filtra [[RF-WKS-008|canon]] o CanonLinks por role; si no hay match, error
- **precedence**:
  - project [[RF-WKS-008|canon]]
  - registry CanonLinks
  - default .docs/wiki
- **out_of_scope**:
  - fan-out --all-workspaces
  - protocol bump
  - envelope type change
- **Comprobación prevista**:
  - go test ./internal/cli -count=1 -run TestNavWikiRootCommandExists
  - go test ./internal/service -count=1 -run 'TestWikiRootResolvesCanonRelativeToWorkspaceRoot|TestWikiRootNoCanonDefaults|TestWikiRootRoleMissingMatchErrors'
- **Condiciones de rechazo**:
  - wiki_root_is_absolute=true
  - missing_canon_is_hard_error=true
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

En el baseline de esta migración, `internal/service/wiki_root.go` conserva `defaultGovernanceDocPath` con el valor histórico `.docs/wiki/00_gobierno_documental.md`. Ese literal de protocolo no es un enlace a una fuente activa ni se cambia por renombrar la wiki. La fuente activa del repo es `00_governance.md`; para canon declarado se conserva el source_doc real, existente y validado, nunca un basename sintetizado. Esta migración documental no afirma haber cambiado el default del binario.

## Criterios de aceptación (numerados)

1. nav wiki-root y nav wiki root son el mismo operation nav.wiki-root
2. sin [[RF-WKS-008|canon]] no hay error; default es .docs/wiki
3. la salida nunca incluye paths absolutos
4. consumers resuelven wiki_root con este comando, no con un path fijo

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
  - sin [[RF-WKS-008|canon]] no hay error; default es .docs/wiki
  - la salida nunca incluye paths absolutos
  - consumers resuelven wiki_root con este comando, no con un path fijo
- **Comprobación prevista**:
  - mi-lsp nav wiki trace RF-WIKI-007 --workspace mi-lsp --format toon
- **Evidencia referenciada**:
  - .docs/wiki/06_tests/TP-WIKI.md
