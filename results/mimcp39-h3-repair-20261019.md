# H3 — saneamiento de informes y ejemplos de configuración

## Resultado

Retiré datos locales concretos de los informes ready15 rastreados y de los documentos/ejemplos de configuración alcanzados por la auditoría. Se conservaron el criterio, SHA fuente, semántica y referencias de evidencia: las cachés privadas ahora se describen sin exponer su ubicación; el endpoint de ejemplo usa el dominio reservado `.invalid`; las rutas Windows conservan su forma con marcadores genéricos.

El cambio de saneamiento está en `01e8dfd211ab6698e6c10cdae4e61ab3dcd9b686` (rama propia, basado en `origin/main` `bc11831`). Se publicará en `main` junto con este informe por push normal.

## Alcance auditado y hallazgos

- Los tres informes ready15 rastreados: H3 y H5 contenían rutas absolutas a caché privada; 6H7 tenía una ruta absoluta en `REPORTE-FINAL`. Se reemplazaron por descripciones sin ruta y por la ruta relativa del repo.
- `docs/micro-proxy-bifrost-embeddings.project.toml.example` contenía una IP de red concreta; se cambió a `bifrost.example.invalid` sin alterar el resto del ejemplo.
- `.docs/wiki/ae/AE-PROJECTION-POLICY.md` y `.docs/wiki/ae/AE-RELEASE-DISTRIBUTION.md` contenían nombres de usuario y de repositorio locales en rutas de ejemplo; ahora usan `<user>` y `<mirror-repo>`.
- `docs/configuracion.md` usa ubicaciones genéricas XDG/`~`; `.docs/wiki/09_contratos_tecnicos.md` sólo contiene loopback `127.0.0.1`. No requirieron cambios.
- Brecha explícita: quedan registros históricos fuera de este alcance acotado (p. ej. auditorías, planificación y planes archivados) que pueden conservar datos locales antiguos. No se reescribió historia ni se afirma que todo el repo esté libre de datos personales.

## Herramientas y límites

- `mi-decidir consultar` y `milsp.intent` se intentaron por el gateway aprobado. La llamada MCP expiró tipadamente (`MCP request timed out after 60000ms`); no llegó una decisión ni resultado de búsqueda. Continué con el reemplazo mínimo y la búsqueda local acotada. Referencias persona aplicables: `persona:simplicidad-primero-camino-mas-sencillo-y-comodo-20261016` y `persona:xp-push-directo-a-main-20261014`.
- `botsd msg sesiones` no identificó un Task para el pane de esta sesión. No adopté ni envié mensajes y no usé el Task del coordinador. El informe durable queda como entrega.
- Sin cambios de código, configuración real, secretos, instalación, reindexación, modelo/API o pruebas unitarias.

## Paths tocados

- `.docs/wiki/ae/AE-PROJECTION-POLICY.md`
- `.docs/wiki/ae/AE-RELEASE-DISTRIBUTION.md`
- `docs/micro-proxy-bifrost-embeddings.project.toml.example`
- `results/mimcp39-ready15-11bis-h3-20261019.md`
- `results/mimcp39-ready15-11bis-h5-20261019.md`
- `results/mimcp39-ready15-6h7-20261019.md`
- `results/mimcp39-h3-repair-20261019.md`

REPORTE-FINAL: results/mimcp39-h3-repair-20261019.md
LISTO
