---
wiki_contract: WIKI-CONTRACT-v1
id: TECH-DOCUMENTARY-GOVERNANCE
kind: technical
status: active
title: Autoridad documental, proyección y reglas transversales
links:
  supports:
  - RF-IDX-003
  - RF-WKS-005
  - RF-WKS-008
  code:
  - path: internal/docgraph/wiki_contract_projection.go
    role: implementation
  - path: internal/docgraph/wiki_contract_governance.go
    role: implementation
---

```yaml
harness_protocol: SDD-HARNESS-v1
id: TECH-DOCUMENTARY-GOVERNANCE
kind: technical-governance
audience: dual
imports: [.docs/wiki/07_technical/contracts/CT-GOVERNANCE-AE-KERNEL-V2.md, .docs/wiki/07_technical/TECH-GOVERNANCE-PROFILES.md]
exports: [TECH-DOCUMENTARY-GOVERNANCE]
agent_must_read: [.docs/wiki/07_technical/TECH-DOCUMENTARY-GOVERNANCE.md, .docs/wiki/00_governance.md]
agent_may_edit: [.docs/wiki/07_technical/TECH-DOCUMENTARY-GOVERNANCE.md]
agent_must_not_edit: [.docs/wiki/00_governance.md, .docs/wiki/_mi-lsp/read-model.toml, .docs/ae/repo-policy.yaml]
verify: ["Leer .docs/wiki/07_technical/TECH-DOCUMENTARY-GOVERNANCE.md y .docs/wiki/07_technical/contracts/CT-GOVERNANCE-AE-KERNEL-V2.md; cotejar las afirmaciones con la fuente publicada internal/docgraph/wiki_contract_governance.go y sus límites. Registrar únicamente comandos y resultados efectivamente ejecutados; esta especificación no afirma una ejecución."]
stop_if: [La interpretación contradiga el canon externo o requiera mutar governance, read-model o policy fuera de esta ficha.]
evidence: [.docs/wiki/07_technical/TECH-DOCUMENTARY-GOVERNANCE.md, .docs/wiki/07_technical/contracts/CT-GOVERNANCE-AE-KERNEL-V2.md, internal/docgraph/wiki_contract_governance.go]
```

## Resumen

`.docs/wiki/` es autoridad documental; governance machine-YAML declara traits y la proyección versionada deriva de traits + kind. README, skills y policy no redefinen el canon. Los nombres y las capas son los de WIKI-CONTRACT-v1; no hay índices normativos en disco.

## Mapa de componentes

- `internal/docgraph/wiki_contract_governance.go`: lee traits y excepciones.
- `internal/docgraph/wiki_contract_projection.go`: proyecta superficies por kind.
- `.docs/wiki/_mi-lsp/read-model.toml`: lectura/edición derivada, no autoridad independiente.
- `internal/service/contract_validate.go`: diagnóstico advisory del contrato v1, no prueba de implementación.
- `<kernel_home>/canon`: autoridad AE externa; `.docs/ae/repo-policy.yaml` conserva slots repo-locales.

### Autoridad y límites

El kernel v2 no se copia ni redefine. Su anclaje repo-local es CT-GOVERNANCE-AE-KERNEL-V2. `valid`, `missing` y `mismatch` son estados operativos: los dos últimos bloquean el trabajo normal conforme al contrato del kernel. No se admiten traversal, rutas absolutas, escapes, symlinks ni archivos no regulares en la resolución protegida. Policy no contiene secretos ni convierte fixtures en política productiva.

`.docs/wiki/ae/` es exclusivamente compatibility history; no participa en las cadenas activas ni concede autoridad de publicación. La distribución de binarios requiere provenance del ejecutable, worker y paquete bajo la política y el canon AE externos vigentes, no instrucciones copiadas desde ae/.

### Lectura y sincronización

El orden de lectura es gobierno, alcance, resultado, arquitectura/ADR, flujo, requisito y detalle técnico aplicable. Cierre incorpora TP; auditoría agrega modelo semántico y DB cuando stores_data aplica. Las relaciones inversas se derivan: FL fulfills RS, RF implements FL, TP verifies RF, TECH/CT supports RF y DB materializes data-model. No se copian enlaces inversos.

Cambios de governance se proyectan al TOML y ambos se versionan juntos. El índice queda stale al cambiar fuentes y se refresca en la fase autorizada posterior; nunca se indexa ni valida durante BUILD. En v1 el drift o output no escribible de la proyección es advisory, no un gate legacy inventado. `workspace status` publica estado y `nav governance` diagnostica. Se conserva por separado la seguridad del canon externo y los rechazos reales de navegación.

`ps-asistente-wiki` dirige a gobierno cuando falta o está incompleto; trazabilidad/auditoría comprueban la fuente y su proyección; las proyecciones AGENTS/CLAUDE proceden de policy. No se mantiene el triple SDD como metadata de autoría v1. Los validadores Harness/Source siguen documentados como superficies de compatibilidad de la herramienta, no como contratos de estos documentos.

### Ownership documental

- Continuation, memory pointer, memory_pointer y handoff reentry: FL-QRY-01, RF-QRY-010, CT-NAV-ASK, CT-NAV-ROUTE, CT-NAV-INTENT.
- Workspace status, stale, preview/full y docs/code mode: RF-WKS-005, RF-QRY-011, CT-CLI-AXI-MODE, CT-NAV-INTENT.
- AE, instalación y distribución: canon AE externo y CT-GOVERNANCE-AE-KERNEL-V2; las fichas ae/ son evidencia histórica, nunca owners activos.

Estos hints preservan conocimiento humano; no se agregan campos fuera del modelo inmutable a governance ni a su proyección derivada.

### Reglas transversales conservadas

- Toda respuesta debe incluir `backend`, `warnings`, `stats` y `truncated`.
- El daemon nunca es requisito para que la CLI funcione.
- El worker .NET nunca se descarga en forma implicita durante una consulta.
- El índice repo-local almacena catálogo liviano de código y un grafo documental reutilizable para `nav ask`.
- `nav ask` rankea docs primero y usa código como evidencia, no al reves.
- `nav ask` y `nav pack` deben bloquear si la gobernanza del repo no es valida.
- `nav service` devuelve evidencia estructurada y no un score fuerte de completitud.
- `client_name` y `session_id` son metadatos operativos de trazabilidad, no parte del payload funcional del usuario.
- Si el request omite `workspace`, la resolución debe priorizar el root registrado que contiene el `caller_cwd`; solo después puede caer a `last_workspace`, con warning visible.
- `nav.search` debe auto-curarse ante regex inválido y devolver warning/hint util antes que error duro cuando existe fallback literal seguro.
- `nav.ask` debe servir por camino directo por default; el daemon queda reservado para queries semánticas o compuestas que realmente se benefician del warm state.
- `workspace list` y `nav.workspace-map` deben separar explicitamente modo rapido vs expansion profunda.
- El auto-registro predeterminado se limita a roots dentro de `$HOME`; linked worktrees resuelven el alias del repositorio principal registrado sin crear una entrada propia. Si falta el registro principal, la resolución devuelve `explicit_incomplete` con `reason_code=invalid_workspace` y no indexa.
- `MI_LSP_AUTOREGISTER=force` es el único override de alcance; su uso debe quedar explícito en warnings. La recolección automática al listar/resolver y `workspace prune --stale` retienen roots vacíos o con errores ambiguos y solo eliminan aliases con roots definitivamente inexistentes.
- La generación de grafo es compiler-first y publica una `GraphGeneration` identificable; `NodeKey` no se deriva de posiciones inestables cuando existe identidad semantica.
- Query, explain e impact son de solo lectura y deben reportar la generación consultada.
- Una extension `MILX-v1` no puede escribir el grafo primario, heredar memoria del proceso principal ni convertir un timeout en fallo global.

### Mantenimiento documental

#### Idioma y diagramas (ampliación v1.1)

`language: es` en `00_governance.md` declara el idioma del cuerpo y las H2 contractuales, localizadas por stable key; admite `es|en` sin cambiar la identidad WIKI-CONTRACT-v1. Los criterios RF son numerados; los casos TP citan ese número e indican disposición y evidencia esperada. El read-model refleja `language`, los traits presentes y las superficies `agent_surface.kind`; conserva roots reales y familias de lectura, sin autorar profile/overlays/hierarchy legacy. La serialización de esta proyección v1.1 conserva reasons, `layer_exceptions` y `ui_roots` seguros desde governance. Inspección y autoSync usan el mismo serializador; `read_model_contract` identifica la revisión sin cambiar la versión de compatibilidad del runtime. El anclaje `ae_canon` se declara en governance y se preserva para las fuentes candidatas `kernel_v2`, sin inventar capas ni reemplazar el resolver de idioma shipped.

`03_architecture.md` contiene Mermaid C4 de contexto y contenedores en Contexto y Contenedores (C4). Cada `04_flows/FL-*.md` contiene su Mermaid de secuencia en Secuencia. Los diagramas se derivan del texto y no agregan comportamiento: si difieren, manda el texto. Los Mermaid derivados a pedido por `nav wiki graph` ya están shipped y no se guardan como canon. `diagram_missing` es warning no bloqueante; agregar los diagramas exigidos aquí no lo convierte en gate.

#### Roadmap fuera del canon

`.docs/roadmap/roadmap.md` es un respaldo no canónico y ultrafino del trabajo ya encargado, vinculado mediante identificadores RS escritos como datos simples. La tabla shipped desde v0.14.1 admite `id`, `rs`, `horizon`, `status` y opcionalmente `title`, `depends_on`. `rs` identifica un RS real; horizontes `short|medium|long`, estados `todo|doing|done|blocked`, dependencias por IDs separados por coma. No lleva contrato wiki ni links tipados autorados, no declara autoridad de negocio y no se agrega a las superficies canónicas del read-model. `canonical=false`, enlaces `roadmap_for` solo hacia RS existentes y Mermaid efímero por horizonte son proyecciones del servicio, no autoría del archivo. Los avisos `roadmap_*` son no bloqueantes y los horizontes sin trabajo respaldado quedan vacíos.

- Cada RF existe como archivo en `05_requirements/`.
- El modelo semántico vive en `05_requirements/data-model.md`; TP declara verifies desde `06_tests/`.
- Un cambio de ownership, secuencia o mitigaciones se refleja primero en el flujo de `04_flows/`.

La antigua tabla declaraba RF-QRY-019 implementado y verificado para preparación semántica portable, con FL-QRY-01 y TP-QRY-PREPARATION. Es historia de entrega, no una matriz normativa activa.

- Ningún RF se considera cerrado si no tiene al menos un caso positivo y uno negativo trazado a un `TP-*`.
- Los casos manuales siguen el TP propietario y sus criterios, aunque todavía no exista automatización completa.

La evidencia anterior de RF-QRY-019 declaraba TC-QRY-PREPARATION-01..17 con 17/17 PASS y TP11 Windows junction PASS; se conserva en TP-QRY-PREPARATION y en el snapshot histórico. No se reejecutó durante esta migración.

La navegación de capas se obtiene con `mi-lsp nav wiki index <kind>` y RF×TP con `mi-lsp nav wiki matrix`. No se sincroniza ninguna matriz o catálogo manual. Las etiquetas ready/planned/implemented de las antiguas tablas son historia de entrega, no estados nuevos del contrato v1 ni evidencia de esta migración.
