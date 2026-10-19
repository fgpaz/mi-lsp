# H1 — precedencia de configuración local de mi-lsp

## Resultado

Se ejecutó una sola matriz FINAL_VERIFY con `mi-lsp skills index --with-embeddings` sobre el CLI instalado y fixtures aisladas. La selección del endpoint quedó observada en runtime: **flags > variables de entorno > config TOML bajo XDG**. Sin config, variables ni flags de embeddings, el CLI informó que no había provider/clave y omitió embeddings.

| Caso | Fuentes en competencia | Evidencia observada | Exit |
|---|---|---|---:|
| ninguno | Ninguna | `embeddings_unavailable: no configured provider or API key` | 0 |
| XDG | Config XDG; además config señuelo inferior en HOME | Seleccionó el marcador XDG; HOME no prevaleció | 0 |
| entorno | Env y config XDG con marcadores distintos | Seleccionó el marcador de entorno | 0 |
| flags | Flags, env y config XDG con marcadores distintos | Seleccionó el marcador de flags | 0 |

Los tres marcadores de endpoint se hicieron deliberadamente inválidos para que `http.NewRequestWithContext` fallara al parsear la URL antes de `http.Client.Do`. Así no se contactó red ni modelo; los mensajes de error distinguen el origen sin imprimir credenciales. Las claves eran fixtures ficticias en el entorno hijo aislado. Cada caso usó HOME, XDG, skills root y catálogo separados; no leyó la config real del usuario ni heredó sus variables de entorno. Los archivos y salidas quedaron fuera del repositorio.

## Alcance y límites

- El comportamiento real comprobado es la precedencia del endpoint y el camino sin provider. Se pasaron marcadores de modelo distintos por entorno y flag, pero el fallo local ocurre antes de enviar el payload; por seguridad, **no se afirma precedencia runtime del valor de modelo**.
- Esto no es evidencia de embeddings exitosos, compatibilidad de proveedor, ni uso de API.
- Se consumió `f50f079d05c4b91e97836c7b50c6a80100c6a279:results/mimcp39-ready15-11bis-h1-20261019.md`: acreditaba doctor/XDG aislado y dejaba pendiente la precedencia runtime. No se repitió el doctor.
- Source, instalado y probado se mantienen separados: el source del worktree era `586d21102457776ebcfb2a01171225f4a0d91743`; el CLI probado informó versión `0.13.2`, revisión `3dedab283f8b584d7a8f4f7239d7eac8cf560f39` y SHA-256 `1117ffb96e79a3997d4612a15b76bd166f4d8968ee6393af6b86b9db7ee869a7`. No se hizo build ni instalación, y no se atribuye el binario al source del worktree.
- La consulta mi-lsp para el source actual dio `read_error` (archivos no publicados en el índice) y el q-v1 mostró contenido anterior; tras ese fallo tipado, se usó lectura acotada del source local únicamente para confirmar que el error de URL ocurre antes de la llamada HTTP. No se modificó source.
- No se ejecutaron unit tests, Jev, modelo ni navegación/provider remoto.

## Paths

- Añadido: `results/herr-lsp-h1-xdg-20261019.md`.
- Sin cambios: source y resto de archivos del repositorio.

REPORTE-FINAL: results/herr-lsp-h1-xdg-20261019.md
LISTO
