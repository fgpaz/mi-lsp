# 00. Gobierno documental

## Proposito

Este documento define la autoridad, el alcance y las reglas de mantenimiento de la documentacion canonica de `mi-lsp`.
Su objetivo es evitar drift entre codigo, skill, CLI y decisiones funcionales/tecnicas.

## Autoridad canonica

- `.docs/wiki/` es la fuente de verdad documental del repo.
- La wiki del repo versionada en git tiene prioridad sobre cualquier GitHub Wiki, nota externa o memoria local.
- `README.md` es la puerta de entrada publica; no redefine comportamiento ni ownership del producto.
- La documentacion consumible por `nav ask` debe vivir o derivarse de `.docs/wiki/`.

## Estructura y ownership

### Capas funcionales

- `01_alcance_funcional.md`: alcance y propuesta de valor.
- `02_arquitectura.md`: arquitectura canonica del producto.
- `03_FL.md` y `03_FL/`: flujos funcionales.
- `04_RF.md` y `04_RF/`: requerimientos atomicos.
- `05_modelo_datos.md`: modelo semantico y entidades.
- `06_matriz_pruebas_RF.md` y `06_pruebas/`: cobertura y casos.

### Capas tecnicas

- `07_baseline_tecnica.md`: baseline tecnica y decisiones runtime.
- `08_modelo_fisico_datos.md`: persistencia y stores fisicos.
- `09_contratos_tecnicos.md`: contratos tecnicos y compatibilidad.
- `07_tech/`, `08_db/`, `09_contratos/`: detalle tecnico expandido.

### Perfil de lectura

- `.docs/wiki/_mi-lsp/read-model.toml` forma parte del canon del repo.
- Ese archivo influye comportamiento real de `nav ask`, por lo que no se trata como nota local.

## Politica de versionado

- Todo `.docs/wiki/**` se versiona en git.
- El snapshot actual de la wiki se considera canon valido aunque despues requiera curacion editorial.
- `.docs/tmp/` queda reservado para borradores, notas locales o material no canonico.
- Ningun documento canonico debe depender de una carpeta ignorada por git para mantenerse coherente.

## Reglas de sincronizacion

- Si cambia comportamiento visible del producto, revisar `01`, `02`, `03_FL*`, `04_RF*` y `06*` segun corresponda.
- Si cambia runtime, supervision, bootstrap, daemon, worker o routing, revisar `07*`.
- Si cambia persistencia, schema o telemetria, revisar `08*`.
- Si cambia CLI, flags, envelopes o protocolos, revisar `09*`.
- Ninguna decision de comportamiento debe vivir solo en `README.md`, skills o notas locales.

## Superficies publicas

- `README.md` debe apuntar a `.docs/wiki/` como canon del repo.
- Si existe GitHub Wiki externa, se la considera legado o espejo no autoritativo mientras no tenga sync explicito.
- Las skills del repo pueden resumir y operacionalizar el canon, pero no sustituirlo.

## Criterios de calidad

- Los documentos raiz deben seguir siendo cortos y navegables.
- Los documentos detalle deben expandir, no redefinir, las decisiones de los documentos raiz.
- La wiki debe permanecer grep-friendly, con nombres estables y ownership claro por capa.
- Si una decision no esta en `.docs/wiki/`, no se considera parte del canon del producto.
