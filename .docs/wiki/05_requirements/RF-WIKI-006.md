---
wiki_contract: WIKI-CONTRACT-v1
id: RF-WIKI-006
kind: requirement
status: active
title: RF-WIKI-006 - Publicar un mapa compacto de hubs de una wiki de conocimiento
links:
  implements:
  - FL-WIKI-01
  code:
  - path: internal/cli/nav.go
    role: behavior
  - path: internal/service/wiki_map.go
    role: behavior
---

## Propósito

RF-WIKI-006 - Publicar un mapa compacto de hubs de una wiki de conocimiento

## Comportamiento ordenado

### Resultado requerido

- **Marcador estable**: RF-WIKI-006.behavior
- **Estado del comportamiento**: implemented
- **actor**: Usuario|Skill|Agente|CLI
- **origin**: FL-WIKI-01
- **command**: mi-lsp nav wiki map --workspace <alias> [--max-items N] [--token-budget N] [--format compact|json|text|toon|yaml]
- **backend**: wiki.map
- **Intención**: mapa compacto de hubs para personalización diaria y Momento; nunca cuerpos completos
- **locks**:
  - D-TEDI-017
- **hubs**:
  - **id**: persona
  - **Título**: Persona
  - **paths**: wiki/00-09 markdown de primer nivel
  - **id**: proyectos
  - **Título**: Proyectos
  - **paths**: wiki/10-19
  - **id**: sistema
  - **Título**: Sistema
  - **paths**: wiki/20-30 de primer nivel o 30-dashboard
  - **id**: materia
  - **Título**: Materia
  - **paths**: bibliotecas/**
- **excluded**:
  - wiki/31-workers
  - wiki/32-contratos
  - yaml
  - old|archive|deprecated|historico|legacy
  - .docs/wiki SDD del producto
- **configuration**:
  - **source**: .docs/wiki/_mi-lsp/read-model.toml
  - **defaults**:
    - **enabled**: true
    - **roots**:
      - wiki/
      - bibliotecas/
    - **hubs**: D-TEDI-017
  - **roots**: wiki/ y bibliotecas/ permanecen; roots configuradas se agregan de forma ordenada y deduplicada
  - **custom_hubs**: [[CT-NAV-WIKI|wiki_map.hub]] en orden declarado; primer patrón coincidente gana y no mezcla hubs default
  - **disabled**: enabled=false desactiva mapa y alta automática de raíces de conocimiento
- **sources**:
  - **preferred**: query SQLite acotada a path,title y raíces configuradas
  - **fallback**: walk bounded de raíces configuradas si el índice falla o no contiene hubs
  - **safety**: ignore matcher, cancelación, symlink/reparse skip y warnings sanitizados
- **envelope**:
  - **items**: hubs[{id,title,total_docs,docs[{path,title}]}]
  - **stats**:
    - **files**: total_returned
    - **total_docs**: null
    - **total_returned**: null
    - **truncation_reason**: null
    - **ms**: null
    - **tokens_estimate**: null
  - **truncated**: true cuando total_returned < total_docs
  - **next_hint**: aumentar --max-items o --token-budget según el límite efectivo
- **budgets**:
  - **max_items**: asignación round-robin equitativa que conserva orden interno y de hubs
  - **token_budget**: búsqueda binaria del mayor conteo equitativo que cabe
- **out_of_scope**:
  - fan-out --all-workspaces
  - grafo de vecinos (lo publica el Graph Kernel, no este comando)
  - memoria propia de Tedi v1
  - Gastos y Notas vivos
- **Comprobación prevista**:
  - go test ./internal/service -count=1 -run 'TestClassifyWikiMapHubDefaultsRemainLocked|TestGroupWikiMapDocsUsesDefaultOrCustomDeclarationOrder|TestTrimWikiMapHubs|TestWalkWikiMapDocs'
  - go test ./internal/cli -count=1 -run 'TestNavWikiMapUsesDirectExecution'
- **Condiciones de rechazo**:
  - map_emits_full_markdown_bodies=true
  - map_classifies_sdd_dot_docs_wiki_as_hub=true
- **Evidencia referenciada**:
  - internal/service/wiki_map.go
  - internal/service/wiki_map.go
  - internal/cli/nav.go
  - .docs/wiki/06_tests/TP-WIKI.md

## Superficies

- mi-lsp nav wiki map --workspace <alias> [--max-items N] [--token-budget N] [--format compact|json|text|toon|yaml]

## Entradas/salidas tipadas

Las entradas, salidas, enums y límites forman parte del contrato de comportamiento ordenado de este documento; los campos conservan sus nombres de protocolo.

## Errores

Se aplican las condiciones de rechazo y los warnings descritos en el comportamiento ordenado y sus variantes. No se añade ningún fallback silencioso.

## Criterios de aceptación (numerados)

1. el mapa es catálogo, no lectura de ficha
2. sin configuración, el orden de hubs es persona, proyectos, sistema, materia
3. con hubs custom, solo se emiten los declarados y en ese orden
4. ningún hub no vacío desaparece mientras el budget alcance una ronda
5. truncation, totales y siguiente acción nunca se ocultan
6. nav.wiki.map siempre ejecuta directo
7. Tedi no inventa un grafo en JavaScript a partir de este envelope

### Aceptación

- **Marcador estable**: RF-WIKI-006.acceptance
- **Casos positivos**:
  - TC-WIKI-029
  - TC-WIKI-030
  - TC-WIKI-031
  - TC-WIKI-032
  - TC-WIKI-034
  - TC-WIKI-036
  - TC-WIKI-037
  - TC-WIKI-039
  - TC-WIKI-040
- **Casos negativos**:
  - TC-WIKI-033
  - TC-WIKI-035
  - TC-WIKI-038
- **Invariantes**:
  - el mapa es catálogo, no lectura de ficha
  - sin configuración, el orden de hubs es persona, proyectos, sistema, materia
  - con hubs custom, solo se emiten los declarados y en ese orden
  - ningún hub no vacío desaparece mientras el budget alcance una ronda
  - truncation, totales y siguiente acción nunca se ocultan
  - nav.wiki.map siempre ejecuta directo
  - Tedi no inventa un grafo en JavaScript a partir de este envelope
- **Comprobación prevista**:
  - mi-lsp nav wiki trace RF-WIKI-006 --workspace mi-lsp --format toon
- **Evidencia referenciada**:
  - .docs/wiki/06_tests/TP-WIKI.md
