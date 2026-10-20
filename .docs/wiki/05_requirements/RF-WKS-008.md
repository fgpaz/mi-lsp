---
wiki_contract: WIKI-CONTRACT-v1
id: RF-WKS-008
kind: requirement
status: active
title: RF-WKS-008 - Declarar raíces de wiki externas con `[[RF-WKS-008|canon]]`
links:
  implements:
  - FL-BOOT-01
  code:
  - path: internal/cli/workspace.go
    role: behavior
  - path: internal/docgraph/docgraph.go
    role: behavior
  - path: internal/model/types.go
    role: behavior
  - path: internal/workspace/canon.go
    role: behavior
  - path: internal/workspace/canon_link.go
    role: behavior
---

## Propósito

RF-WKS-008 - Declarar raíces de wiki externas con `[[RF-WKS-008|canon]]`

## Comportamiento ordenado

### Resultado requerido

- **Marcador estable**: RF-WKS-008.behavior
- **Estado del comportamiento**: implemented
- **actor**: Usuario|Skill|Agente|CLI
- **origin**: FL-BOOT-01
- **Intención**: declarar raíces de wiki externas portables sin paths absolutos
- **command**: mi-lsp workspace link <alias> --role producto|ecosistema|gobierno_local
- **project_file**: .mi-lsp/project.toml
- **workspace_root**: directorio que contiene .mi-lsp/project.toml
- **canon_table**:
  - **syntax**: [[RF-WKS-008|canon]]
  - **fields**:
    - **id**: null
    - **root**: null
    - **role**: null
    - **mode**: null
  - **resolve_relative_to**: workspace_root
  - **never_resolve_from**:
    - .mi-lsp/
    - cwd
  - **example_root**: ../wiki-repo/Ingenieria
- **role**: producto | ecosistema | gobierno_local
- **mode**: omitted | read-only
- **id**: unique case-insensitive
- **canon_policy**:
  - **table**: [canon_policy]
  - **field**: escape_max
  - **omitted**: 1
  - **zero**: no parent escape
- **rejected_roots**:
  - absolute POSIX /
  - home ~/
  - UNC \\server\share
  - **Windows drive C**: null
- **symlink_policy**: fail-closed if resolved path contains symlink or junction
- **no_canon**:
  - **wiki_root**: .docs/wiki
  - **governance_doc**: valor literal de compatibilidad sin canon, conservado en RF-WIKI-007; no es un enlace a gobierno activo.
  - **resolved_from**: default
  - **error**: false
- **governance**:
  - **source_doc**: relative in-workspace OR inside declared [[RF-WKS-008|canon]] root
  - **unique_00_gobierno**: una fuente de gobierno única fuera del default basta; se conserva el source_doc real y la compatibilidad con el nombre legacy, sin duplicarla.
  - **projection_output**: stays in the code workspace; never a read-only foreign canon root
- **index_docs_only**:
  - **walks**: declared [[RF-WKS-008|canon]] roots as ../relative markdown
  - **invalid_root**: skip with warning; do not write that root
- **registry**:
  - **command**: mi-lsp workspace link <alias> --role
  - **stores**: CanonLinks
- **out_of_scope**:
  - writable foreign canon
  - protocol bump
  - envelope type change
- **Comprobación prevista**:
  - go test ./internal/workspace -count=1 -run 'TestResolveCanons|TestCanonLinks|TestValidateCanon'
  - go test ./internal/docgraph -count=1 -run 'TestCollectCanon|GovernanceSourceDoc|Projection'
- **Condiciones de rechazo**:
  - canon_root_absolute_accepted=true
  - projection_writes_foreign_canon=true
- **Evidencia referenciada**:
  - internal/workspace/canon.go
  - internal/workspace/canon_link.go
  - internal/docgraph/docgraph.go
  - internal/cli/workspace.go

## Superficies

- mi-lsp workspace link <alias> --role producto|ecosistema|gobierno_local
- mi-lsp workspace link <alias> --role

## Entradas/salidas tipadas

Las entradas, salidas, enums y límites forman parte del contrato de comportamiento ordenado de este documento; los campos conservan sus nombres de protocolo.

## Errores

Se aplican las condiciones de rechazo y los warnings descritos en el comportamiento ordenado y sus variantes. No se añade ningún fallback silencioso.

## Criterios de aceptación (numerados)

1. root se resuelve desde la raíz del workspace, no desde cwd ni .mi-lsp/
2. ids de [[RF-WKS-008|canon]] son únicos
3. mode omitido equivale a read-only
4. index --docs-only no escribe un root de canon inválido

### Aceptación

- **Marcador estable**: RF-WKS-008.acceptance
- **Casos positivos**:
  - TC-WKS-038
  - TC-WKS-039
  - TC-WKS-040
- **Casos negativos**:
  - TC-WKS-041
  - TC-WKS-042
- **Invariantes**:
  - root se resuelve desde la raíz del workspace, no desde cwd ni .mi-lsp/
  - ids de [[RF-WKS-008|canon]] son únicos
  - mode omitido equivale a read-only
  - index --docs-only no escribe un root de canon inválido
- **Comprobación prevista**:
  - mi-lsp nav wiki trace RF-WKS-008 --workspace mi-lsp --format toon
- **Evidencia referenciada**:
  - .docs/wiki/06_tests/TP-WKS.md
