---
name: mi-lsp
description: Contexto de código/wiki, definiciones, referencias y errores mediante mi-mcp.
---

# mi-lsp

Usá mi-lsp para entender un repo o el impacto de un cambio. Para texto literal o una ruta conocida alcanza grep/Read; no hace falta un recorrido semántico.

Pasá siempre `workspace` (alias registrado). Cuatro tools de mi-mcp:
- `get_context({question, workspace})`: contexto de código/wiki.
- `find_symbol({query, workspace})`: definición.
- `nav_refs({symbol, workspace})`: usos; cada uso incluye su `caller`. No uses `nav_callers`: vacío no prueba ausencia de callers.
- `get_errors({files, workspace})`: errores; cero no prueba compilación.

Leé `ok`, `partial`, `omissions` y `warnings`. Seguí las continuaciones ejecutables devueltas; no inventes comandos. Si falla el backend, informá el motivo y usá lectura/búsqueda acotada.

Code Mode (`execute` devuelve lo que retorna el cuerpo):

```js
return await call({cli: "lsp", comando: "nav_refs",
  args: {symbol: "Parse", workspace: "<alias>"}});
```

Para otra operación, descubrí su esquema con `search` antes de llamar. El esquema publicado manda; `call` con `get_context` usa `file: "ruta:línea"`, no `question`.
