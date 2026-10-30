---
wiki_contract: WIKI-CONTRACT-v1
id: TECH-REPAIR-ACTIONS
kind: technical
status: draft
title: Acciones locales de reparación
links:
  code:
    - path: internal/repair/actions/actions.go
      role: behavior
    - path: internal/repair/actions/inspect.go
      role: behavior
---

## Resumen

Inspección y aplicación acotada de acciones de reparación, con cautela ante WAL, registro inválido y worker.

## Mapa de componentes

`internal/repair/actions/` separa inspección y aplicación; el motor decide cuándo invocarlas.

# Acciones locales de reparación

```yaml
harness_protocol: SDD-HARNESS-v1
id: TECH-REPAIR-ACTIONS
kind: technical-operations
audience: dual
imports: [.docs/wiki/07_technical/TECH-REPAIR.md]
exports: [TECH-REPAIR-ACTIONS]
agent_must_read: [.docs/wiki/07_technical/TECH-REPAIR-ACTIONS.md, internal/repair/actions]
agent_may_edit: [.docs/wiki/07_technical/TECH-REPAIR-ACTIONS.md]
agent_must_not_edit: [internal/repair/actions, internal/store/, .mi-lsp/]
verify: ["Leer .docs/wiki/07_technical/TECH-REPAIR-ACTIONS.md y .docs/wiki/07_technical/TECH-REPAIR.md; cotejar las afirmaciones con la fuente publicada internal/repair/actions/actions.go y sus límites. Registrar únicamente comandos y resultados efectivamente ejecutados; esta especificación no afirma una ejecución."]
stop_if: [Se intenta sobrescribir configuración ilegible, reclamar locks por edad o atribuir recuperación universal de corrupción.]
evidence: [.docs/wiki/07_technical/TECH-REPAIR-ACTIONS.md, .docs/wiki/07_technical/TECH-REPAIR.md, internal/repair/actions/actions.go]
```

El motor de reparación invoca `actions.Inspect` para planificar y `actions.Apply` para intentar cambios autorizados. La inspección no crea registro, directorios ni índice. El índice se consulta en modo SQLite inmutable; si hay WAL con contenido, se informa que no se puede verificar en vez de usar una lectura potencialmente obsoleta o crear archivos auxiliares. El worker opcional sólo se inspecciona cuando el motor lo solicita; se verifica mediante el protocolo, sin shell.

La configuración pública ausente puede crearse como registro vacío bajo el bloqueo del registro. Un registro ilegible o inválido no se sobrescribe. Para un workspace se requiere alias registrado o ruta explícita existente: el registro de una ruta usa detección local y modifica el registro bajo bloqueo, conservando otros alias y sin reemplazar `project.toml`. Si el nombre detectado ya corresponde a otro workspace se requiere resolución manual.

La indexación usa el servicio de jobs existente en modo síncrono y offline para Go; después comprueba el catálogo publicado. Un job activo no se considera éxito. La instancia de reparación resuelve el workspace sin GC y omite la normalización que podría registrar o limpiar entradas ajenas. Se rechaza anticipadamente un índice ilegible y se pide revisión humana. La recuperación de corrupción propia del servicio sigue activa: una corrupción concurrente posterior a la comprobación podría mover el índice elegido y podar cuarentenas de ese workspace; no se promete recuperación universal ni se modifican fuentes o configuración. El worker se instala mediante `InstallWorkerMode`; el artefacto instalado debe superar el handshake antes de registrar el recibo.

Los errores entregados por estas acciones son mensajes sanitizados, sin salida de procesos ni contenido de configuración. Los tiempos de espera, antirrecurrencia, protección por recibos de remoción y decisión de cuándo llamar a cada acción corresponden al motor de reparación.
