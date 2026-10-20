---
wiki_contract: WIKI-CONTRACT-v1
id: TP-HARNESS-FIND-001
kind: test
status: draft
title: "Comprobar búsqueda de archivos desde la CLI real"
links:
  verifies: [RF-HARNESS-FIND-001]
  code:
    - path: internal/service/find_files_test.go
      role: test
    - path: internal/cli/find_files_test.go
      role: test
---

## Estado
Casos y fixtures escritos, no ejecutados durante BUILDING. El padre ejecuta la única FINAL_VERIFY tras integrar el constructor. Esta receta no acredita PASS ni evidencia de rendimiento medida.

## Receta de comportamiento real
Usar un repositorio Git temporal, fuera del checkout del producto, sin depender de fd. Crear `.gitignore` con `ignored/`, `*.tmp`, `!keep.tmp` y `*.log`; en `src/.gitignore`, `!kept.log`, `local*.go` y `!localkeep.go`. Crear archivos correspondientes, un `tracked.tmp` añadido con `git add -f -- tracked.tmp`, nombres con espacios, `café.go` y `-draft.go`.

- CA 1: crear `config`, `src/config`, `config-more`, `configdir/other` y `myconfig`; consultar `config`. Verificar ese orden y motivos/rangos. Dos ejecuciones sin cambios conservan el orden, no necesariamente la edad de caché.
- CA 2: consultar `'*.go'`, `'src/**/config?.go'`, `'**/*.go'` y `--literal 'a[b].go'`. Verificar basename, profundidad cero/múltiple y Unicode en `?`; `'[broken'` devuelve `invalid_pattern`.
- CA 3: no aparecen archivos ignorados; sí `keep.tmp`, `src/kept.log`, `src/localkeep.go` y `tracked.tmp`. Backend `git` confirma independencia de fd. En carpeta sin `.git`, repetir con rg/fd si están disponibles, conservando sus exclusiones nativas.
- CA 4: `find-files -- '-draft.go'` y nombre con espacios/Unicode conservan exactamente la ruta relativa. Symlink a archivo y a directorio externo no aparece. Reemplazar un ancestro por un symlink entre consultas tampoco permite salir del límite. En Windows, omitir solo la parte que exija privilegio de symlink no disponible, y registrar esa limitación.
- CA 5: consultar `'*' --top 1 --budget-bytes 512` y medir bytes de stdout, incluida nueva línea. Nunca supera 512; `items` puede quedar vacío. Motivo `top` o `budget_bytes`, con pista explícita y JSON completo. Presupuestos inválidos fallan antes de enumerar.
- CA 6: calentar caché, crear otro archivo y consultar inmediatamente sin refresh: si `listing.cached=true`, puede faltar el nuevo nombre. Con `--refresh` aparece y `cached=false`. Escribir un snapshot vencido en el fixture y comprobar que se ignora, sin sleeps.
- CA 7: parsear JSON, verificar ausencia de contenido/rutas absolutas y códigos estables. `--format json` funciona; otro formato explícito produce `unsupported_format`. Una consulta sin coincidencias devuelve `items:[]`, `matched:0`, `truncated:false`.

## Portabilidad y coste
En la verificación del padre, ejecutar el mismo binario/receta en Linux y Windows/ARM64 según runners disponibles, sin afirmar soporte probado por compilación cruzada solamente. Medir por separado primera consulta y consultas dentro del TTL sobre el mismo fixture grande; registrar tiempo, backend, caché, bytes y cantidad devuelta. Los fixtures se generan en los tests; no contienen datos personales ni servicios externos.
