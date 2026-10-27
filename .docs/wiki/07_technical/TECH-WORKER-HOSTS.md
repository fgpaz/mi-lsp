# Hosts de workers semánticos

`mi-lsp worker install --mode auto` prefiere una DLL Roslyn framework-dependent compatible, ejecutada por `dotnet`; conserva el fallback self-contained. `--mode framework-dependent` exige bundle FDD y runtime compatibles, o fuentes con SDK y runtime; falla en lugar de degradar. `--mode self-contained` selecciona la distribución existente o la publica desde fuentes. `--rid` modifica el RID destino. La publicación FDD usa `--self-contained false -p:UseAppHost=false`.

Un bundle FDD con DLL y runtimeconfig que declara `framework`/`frameworks` puede usarse sin SDK; un probe del protocolo valida el runtime real antes de seleccionarlo. Un runtimeconfig self-contained con `includedFrameworks` no se considera FDD. La instalación FDD explícita desde un bundle SC necesita otro bundle FDD o fuentes/SDK. No se garantiza ejecutar localmente un RID de arquitectura incompatible.

`worker status --format json` añade `selected.mode`, `selected.host`, `selected.capabilities`; los mismos campos aparecen en candidatos disponibles. Una DLL FDD usa `host: dotnet`; un apphost FDD se identifica por separado y SC usa `apphost`. Status y lanzamiento priorizan candidatos FDD compatibles sobre SC; si coexisten bundle FDD e instalación SC, consultar `installed` para distinguirlos. La selección no acredita que un daemon ya vivo se haya reiniciado: usar su ciclo normal al cambiar workers.

TypeScript ya busca `node_modules/typescript/lib/tsserver.js` en workspace y ancestros antes del TypeScript global npm, y lo ejecuta con `node`. No instala paquetes automáticamente.

Para verificar sin modificar settings globales, usar un proceso con HOME/USERPROFILE y directorios de configuración aislados, agregar dotnet al PATH de ese proceso, instalar FDD, consultar status y ejecutar semántica sobre un workspace descartable. Los campos nuevos permiten detectar capacidad; no se asignó una versión de release en este frente local.
