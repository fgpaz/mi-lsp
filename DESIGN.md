# Sistema visual de mi-lsp

## Propósito

Consola operativa local, workspace-first, para comprender el estado del daemon y ejecutar acciones explícitas. Esta guía describe la interfaz existente; no autoriza un rediseño.

## Principios

- Contexto del workspace siempre visible.
- Jerarquía de resumen, evidencia y detalle.
- Estados distinguibles por mensaje además del color.
- Acciones limitadas al contexto seleccionado.
- Accesibilidad por teclado, foco visible y regiones de estado anunciadas.

## Tokens

La única autoridad de valores es el bloque :root de `internal/daemon/admin_ui.html`: familias `--neo-*`, radios `--radius-*`, tipografía `--font-sans`/`--font-mono`, movimiento `--motion-fast` y foco `--focus-ring`. No se copian hexadecimales, tamaños ni fuentes a este archivo.

## Componentes y patrones

- `panel`, `topbar` y `meta-grid`: identidad y estado del daemon.
- `kpi-grid`: resumen de actividad.
- `toolbar`, `field`, `segmented` y `workspace-tabs`: filtro y selección.
- `panel-main`, `panel-side` y `drawer`: evidencia por contexto.
- `log-lines`, badges y toast: observación y feedback.
- `interactive` y skip-link: interacción/foco y acceso al contenido.

## Intención → patrón → componente

Orientarse → resumen estable → topbar y KPI. Acotar → selección explícita → filtros y tabs. Diagnosticar → detalle progresivo → paneles y logs. Actuar → acción contextual con resultado → botones y toast.

## Contenido y límites

Mantener literales de controles existentes, errores accionables y ausencia de secretos. PRODUCT.md procede del alcance; UJ/UXM describen el recorrido y los estados. No se ejecuta revisión visual durante BUILD.
