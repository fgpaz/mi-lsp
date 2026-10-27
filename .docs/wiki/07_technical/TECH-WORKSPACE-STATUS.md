# Estado del workspace

`workspace status` devuelve `entrypoint_count` para la topología completa y `stats.files`/`stats.symbols` para el catálogo consultado, no para el tamaño de la lista presentada. El índice puede estar ausente o desactualizado: consultar `index_ready` y las advertencias por separado.

La salida normal incluye `entrypoints`. `--no-entrypoints` omite esa lista, pero conserva `entrypoint_count`; equivale a `no_entrypoints: true` en el payload de `workspace.status`. Es útil para clientes que solo necesitan los conteos. La opción no modifica el índice.

`truncated: true` indica un límite de salida, no que el índice sea inválido. Puede deberse al presupuesto de caracteres o al límite de elementos del sobre; inspeccionar `omissions` y `next_hint`. En particular, el limitador actual no recorta individualmente la lista anidada: si un único elemento supera el presupuesto, lo conserva y marca el exceso. Un límite de renderizado explícito puede cortar el texto, incluso JSON; para lectura estructurada elegir un presupuesto suficiente. No tratar `truncated` como prueba de corrupción ni asumir que siempre significa que se recortaron entrypoints.
