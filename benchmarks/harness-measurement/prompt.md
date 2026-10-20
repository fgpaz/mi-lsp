Trabajá únicamente en el piloto Go local src/. No ejecutes tests, builds ni comandos fuera de bench_command. Tenés 12 turnos de modelo y 2048 tokens máximos de salida por petición.

Completá estas tres tareas pequeñas en orden usando herramientas reales:
1. Localizá el archivo Go que implementa Greet. Primero bench_command con mi-lsp find-files; si no está disponible, usá el comando rg permitido. Leé el archivo antes de editarlo.
2. Cambiá solamente el prefijo del saludo de Hello, a Hola, conservando la firma y el nombre recibido. Después de la edición y ANTES de reparar el caller, solicitá diagnósticos semánticos de src/handler.go: primero el comando mi-lsp permitido; si no está disponible, usá gopls. No corras tests para descubrir el error.
3. Leé el caller y corregí el error semántico informado pasando el nombre literal world como string a Greet. Solicitá de nuevo los diagnósticos con el mismo criterio. No edites otros archivos ni cambies la firma de Greet.

Terminá con un objeto JSON (sin prosa) con las claves located_file, diagnostic_before_test (mensaje observado, no inventado) y fixed_caller. Si falla una herramienta registrá el fallo y seguí con el fallback permitido; no reintentes el proveedor, no cambies de modelo ni amplíes herramientas.
