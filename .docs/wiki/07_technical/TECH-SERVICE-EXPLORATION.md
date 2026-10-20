---
wiki_contract: WIKI-CONTRACT-v1
id: TECH-SERVICE-EXPLORATION
kind: technical
status: active
title: TECH-SERVICE-EXPLORATION
---

## Resumen

Define el perfil técnico de `nav service`: un agregador evidence-first que combina catálogo repo-local y busqueda textual scoped a un path para resumir la superficie observable de un servicio.

### Resumen
Define el perfil técnico de `nav service`: un agregador evidence-first que combina catálogo repo-local y busqueda textual scoped a un path para resumir la superficie observable de un servicio.

## Mapa de componentes

Los componentes, paths exactos, responsabilidades y límites se mantienen en el detalle técnico siguiente.

Volver a [07_technical.md](../07_technical.md).

### Responsable y alcance
- Owner logico: Core runtime / Query layer
- Scope: agregacion de evidencia, deteccion de placeholders, perfilado generico vs `.NET microservice` vs `go-package`
- Non-goals: score fuerte de completitud, auditoria funcional completa, dependencia de Roslyn

### Contrato runtime
- Input canónico: `nav service <path> --workspace <alias>`
- Fuentes permitidas en v1:
  - catálogo repo-local (`symbols`, `files`)
  - busqueda textual scoped al path
- Fuentes excluidas en v1:
  - Roslyn obligatorio
  - fanout automático multi-repo
  - analisis de completitud basado en pesos o porcentajes

### Familias de evidencia
- `symbols`: conteo por kind observado en catálogo
- `http_endpoints`: `.NET` `MapGet|MapPost|MapPut|MapDelete|MapPatch`; Go `net/http HandleFunc` y llamadas router-style `Get|Post|Put|Delete|Patch|HEAD|OPTIONS`
- `event_consumers`: ocurrencias `IConsumer<...>`
- `event_publishers`: ocurrencias `PublishAsync<...>` o `IPublishEndpoint`
- `entities`: clases/records bajo `Domain/Entities` o `Domain/Models`
- `infrastructure`: wiring como EventBus, Redis, Npgsql, SqlServer o InMemory; en Go, indicios acotados de router (`chi`, `gin`, `fiber`), `http.ListenAndServe`, `cobra.Command` y worker loops/goroutines
- `archetype_matches`: placeholders detectados y filtrados por default

### Límites de confiabilidad
- El output debe ser accionable pero no autoritativo sobre completitud.
- Cuando el catálogo es insuficiente, el comando degrada a texto y emite warning.
- Cuando el catálogo bajo el path es mayoritariamente Go, el comando evita patrones .NET y solo suma patrones Go language-aware; las coincidencias dentro de tests, fixtures y literales string/raw se filtran para reducir falsos positivos.
- Cuando se detectan placeholders conocidos, deben quedar trazados en `archetype_matches`.
- `--include-archetype` habilita reinsertar esa evidencia filtrada, no cambia el resto del contrato.

### Referencias de contexto
- [07_technical/contracts/CT-TECHNICAL-BASELINE.md](contracts/CT-TECHNICAL-BASELINE.md)
- [CT-CLI-DAEMON-ADMIN.md](contracts/CT-CLI-DAEMON-ADMIN.md)
- [FL-QRY-01](../04_flows/FL-QRY-01.md)
