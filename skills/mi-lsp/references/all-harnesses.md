# Despliegue de la superficie común

## Fuente y orden

1. Integrar archivos de diagnostics/find y los seams de surface en la misma revisión mi-lsp. Consumir el seam genérico de C165/H3; surface no entrega cambios de loader ni aliases en mi-mcp.
2. El padre hace una FINAL_VERIFY con los binarios de esas revisiones; esta lane no ejecuta builds, tests, releases ni instalaciones.
3. Regenerar el snapshot con ese binario: `MI_LSP_BINARY=/ruta/mi-lsp sh scripts/provider-snapshot.sh`. Nunca editar un mapping en mi-mcp para agregar una operación de mi-lsp.
4. Publicar por `scripts/release.sh <version> --upload` siguiendo el contrato de release del repo. Instalar artefacto correspondiente: linux-x64/linux-arm64 o win-x64/win-arm64; no compilar en el host Windows como requisito.
5. Consumir el pin publicado por C165/H3 que implemente `model_alias`; recargar el proceso gateway de cada harness. La rama fuente origin/exec/c165-v187-h3-longcat y el pin inspeccionado fe0849f6b0f7b78e3dfcbb7ff8f29de92c01b18c no acreditan todavía ese soporte ni publicación en main. No prometer lsp.* desde CLI129. Al publicar mi-lsp, entregar manifiesto y pin propios al consumidor jefe-harness-web, dueño del enganche tras-edit.

## Misma configuración en cada harness y usuario

El registro publicado ya contiene `{"id":"lsp","command":"mi-lsp"}`. Conservarlo sin `manifest_file` ni allowlist `ops` que congele la lista antigua: el runtime lee `mi-lsp provider-manifest` del PATH del proceso gateway. `MI_MCP_HERRAMIENTAS_REGISTRY` puede seleccionar un registro por usuario; no poner rutas personales en este repo público.

Perfil del gateway: `MI_MCP_CODEMODE_PERFIL_FILE` selecciona el archivo de usuario y `MI_MCP_CODEMODE_PERFIL` la entrada activa. Si restringe namespaces debe admitir **milsp** (namespace canónico), no basta agregar lsp/fs. Si restringe server_tools debe admitir los targets canónicos nuevos según `herramientas.describe`: `lsp/diagnostics`, `lsp/find-files`, `lsp/search`, `lsp/refs` además de los existentes. El perfil restringe, no otorga permisos; el PEP sellado también debe permitir las lecturas. No reemplazar el perfil entero del usuario con uno genérico ni quitar restricciones para resolver un forbidden.

| Harness | Cómo habilitar operaciones |
|---|---|
| pi | Actualizar gateway mediante su paquete/plugin ya publicado y recargarlo; ese proceso debe ver el nuevo mi-lsp en PATH y registro/perfil del usuario. Cargar la skill compartida mi-lsp. |
| Claude Code | Actualizar/recargar plugin mi-mcp ya publicado; conservar hooks existentes, no agregar PostToolUse. El marketplace mi-lsp existente es opcional, no otro router. |
| Codex | Conservar el servidor mi-mcp ya configurado; reiniciar/reconectar ese proceso con los binarios nuevos y misma skill compartida. No inventar un servidor mi-lsp MCP. |
| PiG | Reusar el gateway execute de su integración publicada y la skill compartida. Si no expone execute, la misma skill usa CLI directa; eso no acredita el criterio execute en pi/Claude. |

Skill canónica: `skills/mi-lsp/SKILL.md` y sus referencias. El instalador publicado `scripts/install/install-agent.sh` / `.ps1` compone instalación CLI + `npx skills add fgpaz/mi-lsp --skill mi-lsp -g -a codex -a claude-code -y`; no InstallAll. El contenido compartido instalado en `~/.agents/skills/mi-lsp` es la misma fuente para los hosts que ya consumen skills compartidas (pi/PiG); no mantener cuatro variantes. Para otro perfil, usar su selector de directorio de skills publicado y apuntarlo a ese contenido, no copiar instrucciones divergentes. La carga efectiva en cada perfil se acredita en FINAL_VERIFY, no por presencia en disco.

No ejecutar estos pasos desde BUILD ni tocar caches/perfiles de usuarios allí. El padre aplica el procedimiento por host y usuario objetivo, en paralelo cuando no comparten escrituras. Una sesión viva no se mata para actualizar; recarga plugin o siguiente sesión.

## Workspace y backend TypeScript en runtime

En Code Mode, el consumidor debe propagar explícitamente el workspace real resuelto (alias o raíz física) mediante el campo `workspace` ya publicado. El cwd del subproceso provider puede ser temporal; el `last_workspace` global no identifica el proyecto del llamador. Esto no vuelve obligatorio el campo en el schema ni cambia los defaults de la CLI local directa; tampoco requiere auto-resolver, registro/indexación, cambios de scopes ni hooks. Scope ausente/inválido o diagnostics parcial/no disponible no significa cero errores. La invocación nativa de Pi devolvió TS2322 al recibir `workspace` explícito, según la evidencia runtime aportada por el padre.

Solo donde se use el backend TypeScript se necesita una distribución compatible con `node_modules/typescript/lib/tsserver.js` (`internal/worker/tsserver.go`, `findTsserverPath`/`CanUseTsserver`). La distribución oficial `typescript@7.0.2` es solo compilador (`bin/tsc`, sin servidor JS); la clásica `typescript@6.0.3` incluye `bin/tsc`, `bin/tsserver` y `lib/tsserver.js`, y permitió obtener TS2322 real en la ejecución del padre. Ejemplo compatible: `npm install -g typescript@6.0.3`. También sirve una distribución local compatible; TypeScript no es un toolchain global obligatorio para todos los hosts o proyectos.

## Smoke exacto de execute para el padre

El cuerpo de `execute` en mi-mcp es el cuerpo de una función async: lo que devuelve es el resultado. Ese sandbox no define `console`, así que `console.log(...)` corta la llamada con `script_error` «console is not defined» (observado en mi-mcp 0.3.139) y no imprime nada. El smoke entrega un único `return` estructurado:

```js
const diagnostics = await lsp.diagnostics({workspace: "project", file: "src/main.go", max_items: 10});
const files = await fs.find({workspace: "project", pattern: "*.go", top: 5, budget_bytes: 2048});
return {diagnostics, files, helpers: {read: typeof fs.read, glob: typeof fs.glob}}; // helpers previos deben coexistir
```

Un solo resultado → `return await lsp.diagnostics({...});`. Varios resultados independientes → un único `return` con objeto o array coherente, sin pérdida; si pueden ir en paralelo, `Promise.all` (o `Promise.allSettled` cuando un fallo no debe borrar el resto) y el resultado sigue siendo ese `return`.

Compatibilidad por backend y versión, no corte universal: el contrato de `return` está acreditado para el sandbox `execute` de mi-mcp en 0.3.139 (source `core/sandbox/child.go` y `core/sandbox/scriptapi.go` de fgpaz/mi-mcp; evidencia de runtime: `script_error` «console is not defined»). Para otra versión u otro backend, verificar el contrato vigente antes de asumirlo. Un cuerpo `execute` que imprime con `console.log` queda legacy para ese backend: etiquetarlo, no borrarlo, y no generalizarlo a otras superficies. La herramienta `codemode` del harness es otra superficie y sí expone `console.log`, `text()` y `return` de nivel superior según su propio contrato; `console.log` también es válido en Node. El smoke lo ejecuta el padre en su host: esta corrección documental no lo corre ni lo sustituye.

Cambiar workspace/path por fixture real del host. Diagnostics requiere exactamente file o changed:true; base solo con changed. Registrar respuestas/errores, versión, host, harness, backend, truncamiento y presupuesto. Ausencia de backend, forbidden o alias ausente es evidencia de fallo/degradación, nunca PASS. La medición antes/después minimax/longcat y el smoke manual execute de ambos hosts pertenecen al padre. Caso TS: backend2322 → corregir → 0 sin stale. El tras-edit automático lo acredita jefe-harness-web con su adaptador, no las llamadas manuales. H1Yoga binding39 es otro pendiente de C165, no evidencia ni bloqueo implícito de nuestro smoke.

Frontera honesta: H3 inspeccionado rechaza argumentos con guion inicial; usar CLI directa `mi-lsp find-files -- '-draft'` para esos nombres. Surface no implementa un delimitador ni modifica loader para resolverlo; la necesidad genérica queda con C165. Los patrones ordinarios conservan el mapping existente.
