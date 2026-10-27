---
wiki_contract: WIKI-CONTRACT-v1
id: TP-WIKI-002
kind: test
status: active
title: "Verificar idioma y avisos no bloqueantes"
links:
  verifies: [RF-WGRAPH-002]
  code:
    - path: internal/service/a14_wiki_red_test.go
    - path: internal/service/a14_wiki_consumers_test.go
    - path: internal/service/a14_safety_test.go
    - path: internal/docgraph/a14_wiki_doctor_test.go
    - path: internal/model/a14_wiki_language_test.go
---

## Alcance
Pilotos es/en y variaciones controladas de governance, raíces, retiros y diagramas, con CLI candidato y fixtures aislados.

## Casos
- Criterio 1 — disposición: manual; evidencia: validate-contract y matrix es/en positivos, más caso con referencia en idioma opuesto que no cubre el criterio.
- Criterio 2 — disposición: manual; evidencia: JSON de ambas operaciones con language ausente e inválido; advertencias tipadas y sin cobertura silenciosa.
- Criterio 3 — disposición: manual; evidencia: ARCH/FL sin diagramas y link inexistente, JSON con avisos de C4, secuencia y destino; hashes de fuentes sin mutación.
- Criterio 4 — disposición: automated; evidencia: `internal/service/a14_wiki_consumers_test.go`, `internal/docgraph/a14_wiki_doctor_test.go`, `internal/model/a14_wiki_language_test.go`; `TestA14AppValidateContractEffectiveRootAndScope`, `TestA14AppUnknownSetupSchemaFailsClosed`, `TestA14DoctorEffectiveGovernanceRootAndLayers` y `TestA14LanguageFrontmatterBeforeFence`.
- Criterio 5 — disposición: automated; evidencia: `internal/service/a14_wiki_red_test.go`, `internal/service/a14_safety_test.go`; `TestA14RetirementsEntries`, `TestA14RetirementsExplicitNoneAndNegativeControls`, `TestA14LegacyRetirementsYAMLAndEntrySemantics` y controles previos `TestCompileWikiContractRetirementsR2`.
