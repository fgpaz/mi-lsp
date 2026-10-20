# Roadmap de respaldo — mi-lsp

Respaldo local opcional, no canónico (`canonical=false`). La planificación operativa sigue en bots-backoffice (Mi Oficina) o Azure Boards; este archivo solo refleja el encargo documental vigente. No cambia active/readiness ni crea gates, fechas o compromisos nuevos.

| id | rs | horizon | status | title | depends_on |
| --- | --- | --- | --- | --- | --- |
| RM-WIKI-1-1 | RS-MILSP-NAVIGATION | short | doing | Reconciliar la migración documental con la semántica shipped sobre dac86cf | |

La tabla usa el formato leído desde v0.14.1: `id`, `rs`, `horizon`, `status`, con `title` y `depends_on` opcionales. Horizontes: `short`, `medium`, `long`; estados: `todo`, `doing`, `done`, `blocked`. Las dependencias, cuando existan, son IDs separados por comas. No se agregan tareas para horizontes sin trabajo encargado.

`nav wiki graph --mode roadmap --render mermaid` deriva una vista efímera por horizonte y estado; no se guarda Mermaid aquí. `roadmap_for` se deriva solo hacia RS existentes. Los avisos `roadmap_*` no bloquean readiness ni cierre; no equivalen a un PASS de esta lane.
