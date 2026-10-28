# Daemon local y compartido

El daemon predeterminado usa el endpoint del perfil local y no invoca `botsd`. `MI_LSP_ELASTIC=1` en cliente y servidor habilita explícitamente el pool compartido por nodo; requiere `botsd` en PATH, identidad configurada y fuente de coordinadores accesible. Reiniciar el daemon por su ciclo normal al cambiar el modo. Si faltan requisitos, el error es accionable; quitar la variable permite usar el daemon local.

## Exclusión y dueño no alcanzable

El fallo del health check no prueba muerte del dueño publicado en `state.json`. El arranque debe fallar cerrado ante PID vivo o indeterminado, estado ilegible o reserva ocupada, antes de fork/listener/apertura de `daemon.db`. En Windows, `AccessDenied` al consultar el proceso no habilita reemplazo. El diagnóstico conserva PID/endpoint sin exponer admin token ni contenido del estado.

El lock breve de bootstrap no basta para constructores concurrentes con endpoints distintos. Una reserva separada se adquiere antes de DB, se retiene hasta su cierre y se libera en todos los fallos de construcción. No se reclama por edad una reserva con dueño vivo/indeterminado. Una reserva basada en PID/nonce protegida por guard OS breve no equivale a un handle OS retenido durante toda la vida; no confundir ambas garantías.

La mitigación evita nuevos arranques cooperantes; no drena ni mata duplicados legacy. Todos los lanzadores deben consumir el binario actualizado y el supervisor realiza la recarga autorizada. No promete corregir con esto el bucle de CPU, idle de servidor ni WAL; cada uno exige evidencia separada. No cambia ACL, UAC, credenciales ni perfil global.

## Contratos de fuente pendientes — OPEN

La fuente independiente de consulta `17e01edc12d6bdcf6d9330d1bfe46456e1b311f5` implementa `mi-lsp daemon lease --format json`; no está publicada y v0.16.2 no contiene este comando. Produce un envelope `backend: daemon-lease` con un item plano: `schema: mi-lsp-daemon-lease/v1`, `state`, `owner_pid`, `instance_id`, `liveness`, `endpoint`. En la CLI los campos están presentes aun con valores cero/vacíos; el struct interno omite opcionales vacíos. `instance_id` es SHA-256 del nonce, identidad opaca cooperativa, no credencial, autenticación ni autoridad de stop; PID y nonce tampoco son un lock OS de vida completa ni prueba fuerte frente a PID reutilizado.

Lee solo `instance.lock` y `state.json` mediante doble lectura acotada, con límite de 64 KiB por archivo y comprobación de archivo regular/identidad. No abre DB ni IPC, no autoarranca ni autoactualiza, no crea runtime, no toma locks mutantes ni expone token o estado crudo. `absent` requiere ambas ausencias confirmadas; `legacy_owner` es estado legacy sin lock actual; `active` indica metadata coherente y PID observado vivo; `stale` indica PID observado muerto. `invalid` significa metadata malformada, `inconsistent` lecturas cambiantes o PID discordante y `indeterminate` imposibilidad de determinar. Liveness es `alive`, `dead` o `indeterminate`; AccessDenied no equivale a muerto ni libre. La consulta es una observación acotada, no autorización para limpiar ni detener.

WAL: la fuente independiente usa lectura de las tres columnas del checkpoint, mantenimiento por minuto, umbral de admisión de 16 MiB y límite de evento de 1 MiB. Un checkpoint y su espera busy son acotados; se preserva `busy_timeout` de la conexión y se descarta la conexión cuyo estado sea desconocido. El presupuesto de drenaje es de 2 s; rechazo, fallas, busy, deadline y contadores deben ser veraces y visibles en health sin datos sensibles. El umbral no es un máximo físico exacto: lectores fijados y asignaciones SQLite pueden mantener o aumentar el archivo. Cableado health/runtime y prueba integrada permanecen OPEN.

El servicio debe sobrevivir al fin/cancelación del comando con ownership independiente del Job Windows. DETACHED o BREAKAWAY por sí solos no prueban eso. No relajar el kill-tree ni los 30 asserts B22. La actualización conserva el fence conjunto y cancelación/join del scheduler A15; el cierre autónomo mantiene DB y lease hasta el drenaje efectivo.

Doctor informa `status: skip` y motivo `unavailable` cuando falta un componente opcional. Conserva los fallos de comunicación del daemon solicitado y los checks requeridos. La disponibilidad de workers/backends es independiente del pool compartido. Un resultado sano no agrega un warning de resumen.
