---
wiki_contract: WIKI-CONTRACT-v1
id: RF-HARNESS-FIND-001
kind: requirement
status: draft
title: "Buscar archivos por nombre con presupuesto explícito"
links:
  implements: [FL-ALLHARNESS-001]
  code:
    - path: internal/cli/find_files.go
      role: behavior
    - path: internal/service/find_files.go
      role: behavior
    - path: internal/model/find_files.go
      role: behavior
---

## Propósito
`mi-lsp find-files <nombre|glob>` busca nombres sin leer contenido, iniciar workers, registrar workspaces ni exigir índice semántico. El integrador de la superficie enlaza este RF desde el FL común; no se inventan IDs RS/FL aquí.

## Comportamiento ordenado
La operación aplica el ranking, glob, enumeración nativa, límites y validaciones descritos en los criterios de aceptación, sin convertir una lista incompleta en una promesa exhaustiva.

## Superficies
El límite por defecto es el directorio actual; `--workspace` selecciona un directorio o alias registrado. La superficie devuelve únicamente nombres de archivos, no contenido.

## Entradas/salidas tipadas
La entrada es `<nombre|glob>` con `--literal`, `--top` y `--budget-bytes` opcionales. `*`, `?` y clases `[]` activan glob; con `/`, glob de ruta relativa; `**` consume cero o más directorios. La salida es JSON `find-files/v1` estable, sin contenido ni rutas absolutas, con `items`, `matched`, `truncated`, `budget`, `listing` y pistas condicionales.

## Errores
Los códigos tipados son `invalid_budget`, `invalid_pattern`, `invalid_workspace`, `cancelled`, `listing_timeout`, `file_listing_unavailable`, `file_listing_failed`, `unsupported_format` y `find_files_failed`. Un formato no JSON explícito se rechaza.

## Criterios de aceptación
1. Texto simple: búsqueda sin distinción de mayúsculas por basename y ruta relativa; orden exacto de basename, prefijo de basename, segmento/prefijo de segmento y subcadena restante. Empates: menos separadores, menos bytes y orden lexical de ruta. Los rangos JSON son 0, 1, 2 y 3.
2. `*`, `?` y clases `[]` activan glob de basename; con `/`, glob de ruta relativa. `**` como segmento consume cero o más directorios. `*.go` busca extensión. `--literal` desactiva glob. No hay expresiones regulares de usuario.
3. Git enumera archivos versionados y no versionados no ignorados, con semántica nativa de `.gitignore` anidados/negados, exclusiones locales y globales. Un archivo ya versionado sigue visible aunque después se ignore. Sin worktree Git, rg/fd existentes conservan sus reglas nativas; sin enumerador exacto se informa `file_listing_unavailable`, nunca un walker aproximado.
4. Solo se devuelven archivos regulares relativos al límite. Se excluyen `.git`, rutas padre/absolutas y todos los symlinks, incluidos ancestros. Espacios, Unicode y nombres con `-` se conservan; `--` separa opciones. Ningún patrón de usuario llega a argv de subprocesos ni a una shell.
5. `--top` (20 por defecto, 1..1000) y `--budget-bytes` (4096 por defecto, 512..1048576) limitan la respuesta JSON completa, incluida la nueva línea. `truncated`, `truncation_reason`, `next_hint` y `budget` explican omisiones. Los límites globales explícitos de ítems/caracteres/tokens solo pueden reducir el presupuesto.
6. La lista de nombres puede reutilizarse entre procesos durante cinco segundos, con backend, edad y antigüedad máxima explícitos. `--refresh` fuerza nueva enumeración; no se promete detectar instantáneamente nuevos archivos ni cambios de ignores dentro del TTL. Los candidatos devueltos se revalidan contra symlinks/archivos borrados.
7. JSON `find-files/v1` estable, sin contenido ni rutas absolutas, con `items`, `matched`, `truncated`, `budget`, `listing` y pistas condicionales. `matched` cuenta coincidencias regulares comprobadas del snapshot; no se calcula un total global adicional. Error tipado `{error:{code,message}}`, sin stderr privado.

## Límites
La operación tiene un plazo de diez segundos y una lista nativa máxima de 32 MiB; falla explícitamente antes que entregar resultados incompletos como si fueran exhaustivos. No se añaden servicios ni dependencias Go. La caché es best-effort y vive fuera del repositorio. La búsqueda de extensiones no necesita fd: Git sirve como alternativa portable.
