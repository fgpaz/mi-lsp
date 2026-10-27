# Ubicación del runtime y recibos

`version --format json` informa en `items[0].runtime` la ruta real del ejecutable, la raíz gestionada recomendada, la ruta del ejecutable gestionado y la ruta de recibos. No registra ni instala nada. `capabilities` incluye `repair-v1` en la integración que incorpora repair; no implica que releases anteriores la tengan.

La raíz gestionada es `LOCALAPPDATA/mi-lsp-runtime` en Windows (fallback `AppData/Local` bajo el directorio de usuario) y `XDG_DATA_HOME/mi-lsp-runtime` en Unix (fallback `.local/share` bajo el directorio de usuario). Es una recomendación, no prueba de instalación. Se resuelven enlaces del ejecutable cuando es posible; si no se puede determinar una ruta base absoluta, el campo correspondiente queda vacío.

Los recibos `~/.mi-lsp/repair/installations.json` usan `mi-lsp-installations/v1`, con componente `runtime` o `worker`, path absoluto y fecha RFC3339 UTC. Solo se registran artefactos existentes. Sin recibo previo, la ausencia no prueba remoción; un recibo válido cuyo path ya no existe sí permite informar remoción, sin atribuirla a una causa. Los recibos inválidos generan error y no se reemplazan. Los escritores usan bloqueo de archivo del sistema operativo no bloqueante y reemplazo JSON mediante archivo temporal en el mismo directorio. Consultar ubicación o recibos no crea archivos.
