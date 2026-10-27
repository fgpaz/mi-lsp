# Daemon local y compartido

El daemon predeterminado usa el endpoint del perfil local y no invoca `botsd`. `MI_LSP_ELASTIC=1` en cliente y servidor habilita explícitamente el pool compartido por nodo; requiere `botsd` en PATH, identidad configurada y fuente de coordinadores accesible. Reiniciar el daemon por su ciclo normal al cambiar el modo. Si faltan requisitos, el error es accionable; quitar la variable permite usar el daemon local.

Doctor informa `status: skip` y motivo `unavailable` cuando falta un componente opcional. Conserva los fallos de comunicación del daemon solicitado y los checks requeridos. La disponibilidad de workers/backends es independiente del pool compartido. Un resultado sano no agrega un warning de resumen.
