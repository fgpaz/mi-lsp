# Extensión fuente del compilador Kotlin — raw v2 experimental

Esto **no es** una capacidad de fwcd stock ni un runtime instalado. Es una extensión auditable de fwcd `1.3.13`, commit `f532bcc9c6578f5a35009a8b5f281f93d4d131ba`, compiler `2.1.0`. El fork se identifica como `1.3.13+mi-lsp.obs2`, extensión `mi-lsp-compiler-observations/v2`. Autoría sin build ni ejecución; no hay PASS.

## Superficies

- `src/main/kotlin/org/javacs/kt/MiLspCompilerObservations.kt`: servicio JSON-RPC y observación real mediante `Compiler.compileKtFiles` → `BindingContext` + `ModuleDescriptor`.
- `upstream.patch`: flag del launcher que selecciona endpoint compiler-only, más parámetros efectivos de módulo/JDK del compilador. No reemplaza el compilador ni agrega framework.
- `upstream-inputs.json`: SHA-256 de fuentes upstream consultadas; la versión y el ref se verifican en el build explícito futuro.
- `LICENSE.upstream.txt`: MIT de fwcd conservada íntegramente.
- `licenses/upstream-licenseReport.html`: inventario original de transitivas; **no** licencia MIT universal ni certificación de las JAR aún no construidas.
- `build.sh`: build manual offline sobre checkout local fijado, salida nueva propia, sin instalación global, bootstrap ni integración implícita en release.

## Build futuro, no ejecutado durante BUILD

Requiere checkout local completo en el pin, Git, Bash, tar, Python 3, **Gradle 8.10 preinstalado**, JDK/toolchain **11** y dependencias exactas ya cacheadas. El wrapper upstream también declara 8.10; el script no lo ejecuta para evitar downloads de distribución. No descarga SDK/binarios ni código en runtime. Si falta una dependencia offline, el build falla y deja la salida propia para el owner; no borra ni modifica el checkout original.

```text
bash worker-kotlin/build.sh /ruta/checkout-fwcd-f532bcc9 /ruta/absoluta/salida-nueva
```

Internamente: `gradle --offline --no-daemon -Pversion=1.3.13+mi-lsp.obs2 :server:installDist -x test`. Genera `runtime/bin/kotlin-language-server`, su sidecar `.mi-lsp.json`, provenance, licencia original e inventario original. El sidecar fija version/compiler/ref, hash del launcher, hashes de patch/source y cada archivo distribuido. El handshake debe devolver la versión **del fork**, no la stock. No ejecuta tests por ser comando de construcción; la única FINAL_VERIFY posterior pertenece al goal-root.

Dependencias directas upstream fijadas: Kotlin compiler/stdlib/reflect/scripting/sam-with-receiver `2.1.0`; LSP4J + JSON-RPC `0.21.2`; Exposed core/dao/jdbc `0.37.3`; Guava `33.3.0-jre`; Fernflower `1.0`; H2 `1.4.200`; ktfmt `b5d31d1`; JCommander `1.78`; SQLite JDBC `3.41.2.1`. Build plugins tcdeps `1.2`, license `0.8.42`, detekt `1.22.0`. Las dependencias de tests upstream no se incorporan por esta autoría como verificaciones nuevas. El inventario transitivo original se conserva, pero el empaquetador debe cotejar las JAR reales y sus licencias en FINAL_VERIFY; no se atribuye una única licencia a todas ellas.

## Configuración efectiva, no ejecutada aquí

`config/export-main-jvm.sh` más `config/export-main-jvm.init.gradle.kts` capturan el modelo Gradle main-jvm. No los llama el runtime, `build.sh` ni el arranque del language server. Opt-in: `MI_LSP_KOTLIN_EXPORT_MAIN_JVM=1 bash worker-kotlin/config/export-main-jvm.sh /modulo/absoluto`. Requiere Gradle 8.10 y JDK 11 ya instalados. Usa `gradle --offline --no-daemon --no-build-cache`; no usa wrapper ni descarga. Escribe solo `<modulo>/.mi-lsp/kotlin-main-v1.json` y `kotlin-main-v1.authority.json` cuando el modelo es representable. Esos archivos no se versionan. Un JSON manual con `resolved`, un classpath backup, solo stdlib o cero diagnostics no certifican. Los hashes de exporter e init están pineados en el loader y en el compiler.

## Protocolo exacto

El mismo launcher recibe `--miLspCompilerOnly` y selecciona `MiLspCompilerServer`, sin construir los servicios stock de DB/index/Gradle. Initialize exige `initializationOptions.miLspCompilerOnly=true`; no hace database setup, classpath resolver Gradle, carga de workspace ni indexación upstream. Anuncia `capabilities.experimental.miLspCompilerObservationsV2=true`; ningún provider PSI se usa como hecho semántico. Las consultas K1 normales conservan el modo stock y sus límites parciales.

Método: `kotlin/miLsp/compilerObservations/v2`.

Params: `module_root` absoluto real, `configuration_path` exactamente `<module>/.mi-lsp/kotlin-main-v1.json`, `configuration_sha256` hexadecimal lowercase SHA-256 del JSON completo. No envía source text ni secretos.

El descriptor local contiene:

```text
schema_version: 1
resolution: resolved
module_root: /ruta/real/modulo
module_name: nombre-configurado-unico
source_set: main-jvm
jdk_home: /ruta/real/jdk-en-uso
jvm_target: 11
gradle_version: 8.10
compiler_version: 2.1.0
server_version: 1.3.13+mi-lsp.obs2
sources: [{path: /ruta/real/modulo/src/main/kotlin/A.kt, sha256: <sha256-real>}]
classpath: [{path: /ruta/jar-resuelto.jar, sha256: <sha256-real>}]
build_inputs: [{path: /ruta/real/modulo/build.gradle.kts, sha256: <sha256-real>}]
```

Es una descripción del formato, **no fixture ni configuración utilizable**. El archivo operacional lo escribe el exporter, no esta autoría. El recibo debe coincidir con fuentes, JARs resueltos, build inputs y el `release` del JDK. `resolved` sin ese recibo se rechaza. Nunca se acepta backup/fallback/stdlib-only. El compilador usa exclusivamente los JARs, fuentes, nombre de módulo, JDK real de su JVM y target válido del descriptor. No se mezclan módulos ni FQN iguales de módulos distintos. Se exige inventario exacto de `.kt`/`.java` bajo `src/main` y hashes antes/después. CRLF, scripts, AGP/Android, multiplatform, variantes, tests y frontend K2 universal están fuera de v1; compiler **2.1.0** no implica que esta API interna sea el frontend K2.

Límites: 512 fuentes de 2 MiB; 256 JARs de hasta 256 MiB, total JARs 1 GiB; 128 inputs de build de 2 MiB; JSON de config 2 MiB; transporte K1 respuesta 8 MiB. El JDK debe ser el que ejecuta la JVM; no se admite una ruta JDK decorativa. El descriptor efectivo y distribución se incluyen en la llave del runtime; no hay caché de observaciones. Cambios, cancel/deadline, error, crash o generación tardía invalidan toda la respuesta, sin partialbatch ready.

## Respuesta y semántica

`schema_version=2` es **protocolo de exportación raw**, no cambio del schema Graph v1. Versiones/ref, module root/descriptor, configuración efectiva, fingerprints SHA-256, `resolution=BindingContext+ModuleDescriptor`, `completeness=partial`, errores de diagnóstico contados sin logs, capabilities, coverage, declarations, references, omissions y limitations.

Declarations provienen exclusivamente de `DECLARATION_TO_DESCRIPTOR`; referencias de `REFERENCE_TARGET`, con descriptor resuelto de origen y target. Exporta descriptor class, firma, módulo compilador/observación, dispatch/extension receiver. Identidad `descriptor-renderer-fq/v1|...` usa descriptores originales y chain de contención del compilador, tipos FQ, sin paths/ranges/PSI-text ni nombres de parámetros. No es un NodeKey ni un nuevo formato persistido. Locales/anónimos/constructores especiales/kinds no representables se omiten. Offsets UTF-16 y owner digest son evidencia, nunca identidad. Homónimos/sobrecargas no se unen por display name. Solo referencias con ambos descriptores originales presentes en declaraciones locales inequívocas se exportan; targets externos/Java se omiten.

`calls=unsupported`: no se extraen resolved calls, no se convierte una referencia en llamada. Java puede aportar resolución, pero sus declarations no se exportan. Coverage raw v2 enumera elegibles examinados/observados/irresueltos/omitidos por declarations y references; no acredita precision/recall. Todas las respuestas son parciales; ni lista vacía, descriptor `resolved` ni initialize exitoso certifican completitud.

## Adaptador y límite Graph

Go usa `NewKotlinClient`/`RequestExtension` existentes, el mismo lector, proceso, deadline y cancelación. `Call(graph_observe)` devuelve `WorkerResponse.items[0].compiler_observation` derivado y `partial=true`; no SQL/generations desde la extensión. `internal/indexer/extractor_kotlin.go` valida raw v2 y entrega batch canónico sin sello usando NewNodeKey-v1. No publica ni promueve partial/experimental.

**Admisión reversible:** coord-milsp autorizó exactamente `kotlin-language-server` / `kotlin` / `kotlin/binding-context`; ver `ADR-KOTLIN-GRAPH-ADMISSION-V1`. El mapping admite type/function/method/property y referencias compiler locales, no calls. Identidad original+module+owner, sin offsets/root/config JSONSHA ni nombres de parámetros. Rangos UTF-16 se convierten a byte columns con digest. Aunque el recibo certifique la configuración, `KotlinPromotionAllowed` deja el raw partial/experimental: faltan los seis oráculos de RF-GPH-008. ValidateCanonical→rebase→seal→ReadyForStaging no publica. La admisión de enums no es promoción estable.

Matriz runtime: Unix experimental, grupo hijo supervisado; Windows unavailable hasta Job Objects; no full-platform PASS ni Java 25 certificado. Dependencias propuestas, no instaladas. Owner de fixture/build/license audit y juicio del límite: coord-milsp / Semantic-backends + Graph-Kernel, vencimiento de decisión 2026-10-17. Contrato wiki: `TECH-KOTLIN-BACKEND`, `CT-DAEMON-WORKER`, `RF-GPH-008`, `TECH-GRAPH-NATIVE`, `TP-GPH.kotlin-compiler-export-v1` y `TP-QRY.kotlin-runtime-v0`.
