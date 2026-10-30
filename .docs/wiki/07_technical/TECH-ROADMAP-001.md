---
wiki_contract: WIKI-CONTRACT-v1
id: TECH-ROADMAP-001
kind: technical
status: active
title: "Proyección local no canónica del roadmap"
links:
  supports: [RF-ROADMAP-001]
  code:
    - path: internal/model/wiki_roadmap.go
      role: behavior
    - path: internal/service/wiki_roadmap.go
      role: behavior
    - path: internal/service/wiki_graph.go
      role: behavior
---

```yaml
harness_protocol: SDD-HARNESS-v1
id: TECH-ROADMAP-001
kind: technical
audience: dual
imports: [.docs/wiki/07_technical/TECH-GRAPH-NATIVE.md, .docs/wiki/07_technical/contracts/CT-ROADMAP-001.md]
exports: [TECH-ROADMAP-001]
agent_must_read: [.docs/wiki/07_technical/TECH-ROADMAP-001.md, internal/service/wiki_roadmap.go]
agent_may_edit: [.docs/wiki/07_technical/TECH-ROADMAP-001.md]
agent_must_not_edit: [internal/service/wiki_roadmap.go, .docs/roadmap/roadmap.md, .docs/wiki/_mi-lsp/read-model.toml]
verify: ["Leer .docs/wiki/07_technical/TECH-ROADMAP-001.md y .docs/wiki/07_technical/TECH-GRAPH-NATIVE.md; cotejar las afirmaciones con la fuente publicada internal/service/wiki_roadmap.go y sus límites. Registrar únicamente comandos y resultados efectivamente ejecutados; esta especificación no afirma una ejecución."]
stop_if: [La propuesta promueva roadmap a canon, readiness o autoridad de estado.]
evidence: [.docs/wiki/07_technical/TECH-ROADMAP-001.md, .docs/wiki/07_technical/TECH-GRAPH-NATIVE.md, internal/service/wiki_roadmap.go]
```

## Resumen
El servicio lee `.docs/roadmap/roadmap.md` fuera de la wiki y deriva nodos, enlaces `roadmap_for` hacia RS existentes y dependencias. El documento no es canónico (`canonical=false`) y no participa en readiness ni gates. Un RS ausente se avisa como `roadmap_rs_missing` solo si la generación del grafo está current.

La tabla del respaldo usa `id`, `rs`, `horizon`, `status`, `title` y `depends_on`; los dos últimos son opcionales. Horizontes `short|medium|long`; estados `todo|doing|done|blocked`. Los IDs de dependencia se separan con comas. Mermaid agrupa por horizonte y estado solo para la respuesta; no se escribe al disco.

## Mapa de componentes
`internal/model/wiki_roadmap.go` interpreta la tabla. `internal/service/wiki_roadmap.go` la carga desde el workspace. `internal/service/wiki_graph.go` proyecta entradas, avisos y el Mermaid efímero por horizonte. El enlace `supports` hacia RF-ROADMAP-001 está declarado en este header; la prosa de TECH-GRAPH-NATIVE no lo sustituye.
