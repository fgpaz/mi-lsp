# 00. Gobierno documental

## Proposito

Este documento es la fuente de verdad humana de la gobernanza documental de `mi-lsp`.
Define el perfil efectivo del repo, el orden global -> especifico, las reglas de bloqueo y la cadena canonica de contexto, cierre y auditoria.

Si este documento no esta perfectamente claro, la implementacion spec-driven debe detenerse.

## Governance Source

```yaml
version: 1
profile: spec_backend
overlays:
  - spec_core
  - technical
numbering_recommended: true
owner_hints:
  - terms:
      - continuation
      - memory pointer
      - memory_pointer
      - handoff reentry
    prefer_doc_ids:
      - FL-QRY-01
      - RF-QRY-010
      - CT-NAV-ASK
      - CT-NAV-ROUTE
      - CT-NAV-INTENT
    prefer_layers:
      - "03"
      - "04"
      - "09"
  - terms:
      - workspace status
      - stale
      - preview full
      - docs code mode
    prefer_doc_ids:
      - RF-WKS-005
      - RF-QRY-011
      - CT-CLI-AXI-MODE
      - CT-NAV-INTENT
    prefer_layers:
      - "04"
      - "09"
hierarchy:
  - id: governance
    label: Gobierno documental
    layer: "00"
    family: functional
    pack_stage: governance
    paths:
      - .docs/wiki/00_gobierno_documental.md
  - id: scope
    label: Alcance funcional
    layer: "01"
    family: functional
    pack_stage: scope
    paths:
      - .docs/wiki/01_*.md
  - id: architecture
    label: Arquitectura
    layer: "02"
    family: functional
    pack_stage: architecture
    paths:
      - .docs/wiki/02_*.md
  - id: flow
    label: Flujos
    layer: "03"
    family: functional
    pack_stage: flow
    paths:
      - .docs/wiki/03_FL.md
      - .docs/wiki/03_FL/*.md
  - id: requirements
    label: Requerimientos
    layer: "04"
    family: functional
    pack_stage: requirements
    paths:
      - .docs/wiki/04_RF.md
      - .docs/wiki/04_RF/*.md
  - id: data
    label: Modelo de datos
    layer: "05"
    family: functional
    pack_stage: data
    paths:
      - .docs/wiki/05_*.md
  - id: tests
    label: Pruebas
    layer: "06"
    family: functional
    pack_stage: tests
    paths:
      - .docs/wiki/06_*.md
      - .docs/wiki/06_pruebas/*.md
  - id: technical_baseline
    label: Baseline tecnica
    layer: "07"
    family: technical
    pack_stage: technical_baseline
    paths:
      - .docs/wiki/07_*.md
      - .docs/wiki/07_tech/*.md
  - id: physical_data
    label: Modelo fisico
    layer: "08"
    family: technical
    pack_stage: physical_data
    paths:
      - .docs/wiki/08_*.md
      - .docs/wiki/08_db/*.md
  - id: contracts
    label: Contratos tecnicos
    layer: "09"
    family: technical
    pack_stage: contracts
    paths:
      - .docs/wiki/09_*.md
      - .docs/wiki/09_contratos/*.md
context_chain:
  - governance
  - scope
  - architecture
  - flow
  - requirements
  - technical_baseline
  - contracts
closure_chain:
  - governance
  - flow
  - requirements
  - technical_baseline
  - contracts
  - tests
audit_chain:
  - governance
  - flow
  - requirements
  - technical_baseline
  - physical_data
  - contracts
  - tests
blocking_rules:
  - missing_human_governance_doc
  - missing_governance_yaml
  - invalid_governance_schema
  - projection_out_of_sync
  - workspace_index_stale
projection:
  output: .docs/wiki/_mi-lsp/read-model.toml
  format: toml
  auto_sync: true
  versioned: true
```

## Autoridad canonica

- `.docs/wiki/` es la fuente de verdad documental del repo.
- `00_gobierno_documental.md` manda sobre el `read-model.toml`; el archivo TOML es una proyeccion ejecutable y versionada de este documento.
- `README.md`, skills, prompts y notas externas pueden resumir la gobernanza, pero no redefinirla.
- Si el YAML embebido y el `read-model.toml` divergen, la gobernanza queda invalida hasta reparar o resincronizar.

## Perfil efectivo

- Perfil visible: `spec_backend`
- Base interna compilada: `ordered_wiki`
- Overlays efectivos: `spec_core`, `technical`
- Numeracion: recomendada porque aporta orden, pero no es requisito de validez

## Reglas de bloqueo

- Toda tarea arranca por gate de gobernanza.
- Si `00` falta, el bloque YAML esta incompleto, el `read-model.toml` esta fuera de sync o el indice quedo stale respecto de estos artefactos, el repo entra en `blocked mode`.
- En `blocked mode` solo se permite diagnostico y reparacion de gobernanza.
- `mi-lsp nav governance --workspace <alias> --format toon` es la superficie de diagnostico primaria.

## Orden global -> especifico

1. `00_gobierno_documental.md`
2. `01_alcance_funcional.md`
3. `02_arquitectura.md`
4. `03_FL.md` y `03_FL/`
5. `04_RF.md` y `04_RF/`
6. `05_modelo_datos.md`
7. `06_matriz_pruebas_RF.md` y `06_pruebas/`
8. `07_baseline_tecnica.md` y `07_tech/`
9. `08_modelo_fisico_datos.md` y `08_db/`
10. `09_contratos_tecnicos.md` y `09_contratos/`

## Cadenas canonicas

- Contexto: `00 -> 01 -> 02 -> 03 -> 04 -> 07 -> 09`
- Cierre: `00 -> 03 -> 04 -> 07 -> 09 -> 06`
- Auditoria: `00 -> 03 -> 04 -> 07 -> 08 -> 09 -> 06`

## Politica de sincronizacion

- Cambios en `00` deben proyectarse a `.docs/wiki/_mi-lsp/read-model.toml` y quedar versionados en el mismo repo.
- Cambios en `00` o en `read-model.toml` obligan a reindexar el workspace antes de continuar con consultas docs-first.
- `owner_hints` es opcional y sirve para declarar ownership documental de capabilities nuevas sin hardcodearlo por repo en el binario; la proyeccion los materializa en `read-model.toml`.
- `workspace status` debe exponer perfil, sync de gobernanza y estado bloqueado.
- `nav ask` y `nav pack` no deben continuar si la gobernanza no es valida.

## Superficies operativas obligatorias

- `mi-lsp` ayuda a diagnosticar y a proyectar la gobernanza.
- `ps-asistente-wiki` debe recomendar obligatoriamente `crear-gobierno-documental` cuando `00` falte, este incompleto o haya drift.
- `ps-trazabilidad` y `ps-auditar-trazabilidad` deben verificar que `00` este completo y que la proyeccion este sincronizada.
- `ps-crear-agentsclaudemd` debe codificar este gate en `AGENTS.md` y `CLAUDE.md`.

## Criterios de calidad

- El documento humano debe seguir siendo legible para personas y LLMs.
- El YAML debe ser estricto, chico y validable.
- Las decisiones de gobernanza deben vivir en `00`, no repartidas entre policy, skills y contratos tecnicos.
- Si una decision no esta en `00`, no forma parte de la gobernanza vigente del repo.
