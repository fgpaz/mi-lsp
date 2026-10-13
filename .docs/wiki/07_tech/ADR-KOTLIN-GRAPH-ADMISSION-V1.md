# ADR-KOTLIN-GRAPH-ADMISSION-V1 — admisión Kotlin aditiva

```toon
doc_id: ADR-KOTLIN-GRAPH-ADMISSION-V1
block_id: ADR-KOTLIN-GRAPH-ADMISSION-V1.decision
kind: architecture-decision
source_of_truth: this
status: delegated_source_decision_unverified
owner: coord-milsp
date: 2026-10-13
imports: [02_arquitectura, TECH-GRAPH-NATIVE, RF-GPH-001, RF-GPH-008, DB-SYMBOL-EDGE-GRAPH, CT-DAEMON-WORKER]
bindings: [internal/model/graph.go, internal/model/graph_observation.go, internal/indexer/extractor_kotlin.go, internal/indexer/graph_pipeline.go, internal/worker/kotlin_observation.go, internal/worker/kotlin_config.go]
evidence: [worker-kotlin/src/main/kotlin/org/javacs/kt/MiLspCompilerObservations.kt, worker-kotlin/config/export-main-jvm.sh, testdata/kotlin-graph-main-v1/README.md]
verify: [.docs/wiki/06_pruebas/TP-GPH.md::TP-GPH.kotlin-compiler-export-v1]
decision_basis: parent_packet_milsp-kotlin-kernel-author-20261013
supersedes: K2_local_no_new_Graph_enums_for_exact_Kotlin_registration_only
backend: kotlin-language-server
language: kotlin
resolution: kotlin/binding-context
kinds: [type, function, method, property]
capabilities: [declarations, references]
calls: unsupported_outside_batch
conformance: explicit_incomplete
pending_owner: coord-milsp
pending_due: 2026-10-17
```

## Decisión y alcance

El parent coord-milsp asignó al writer responsabilidad de Graph-Kernel y Semantic-backends para esta extensión reversible, apoyada en RF-GPH-001 «otro registrado» y RF-GPH-008. Sustituye únicamente el guardrail local K2 y su proyección WIP. No es un ADR global aprobado por Gabriel, ni una aprobación de conformance ni una sesión Graph-Kernel inventada. La matriz habilita claims exact compiler experimentales, no promociona el producto.

Dominio: módulo Gradle explícito, main-jvm, declaraciones Kotlin nombradas representables y referencias compiler con ambos endpoints locales representables. Java puede resolver tipos pero no aporta nodos; typealias/locales/anónimos/constructores/accessors/sintetizados/externos se omiten. Calls no se extrae: worker, pipeline y wiki lo dicen; no se añade capability u omission calls al batch subset. No se afirma grafo íntegro del proyecto. Java usa el root convencional src/main/java (otros roots Java fallan explícitamente); parámetros de constructor no se exportan como propiedades. `status` solo informa metadata configurada/no verificada y calls unsupported, sin initialize ni claims de stageability.

NodeKey-v1 conserva tupla, framing, hash y cross-RID. ProjectOrModule es el path relativo del módulo + `kotlin-main-jvm`; semantic_identity versionada contiene renderer del descriptor original sin nombres de parámetros, compiler module y receivers. Owner proviene de la declaración, no del target FQN. Config/source fingerprints son provenance del batch, nunca JSONSHA absoluto en NodeKeys. Signatures raw conservan evidencia descriptiva, no reemplazan identidad normalizada.

## Completitud y límites materiales

Raw v2/fork obs2 enumera todos los KtDeclaration/KtReferenceExpression examinados y cuenta elegibles, observados, irresueltos y omitidos. Los descriptores originales se enlazan dentro del compiler; Go exige identidad+compiler module+owner inequívocos. Los endpoints no representables son omisiones, los targets realmente irresueltos se separan. Referencias repetidas se agregan por arista y conservan evidencia; su ocurrencia adicional queda como omission representacional.

La cobertura raw conserva denominadores reales del scan. La cobertura Graph suma, separadamente de esos denominadores source, obligaciones explícitas de autoridad de configuración y análisis incompleto. No oculta fallos como omisiones representacionales. Offset UTF-16 se convierte a línea/columna byte UTF-8 y observed digest del span; interior de surrogate/fuera de archivo se rechaza. Todos los nodos/aristas requieren evidencia.

**Autoridad de configuración:** el JSON que solo dice `resolved` ya no alcanza. El exporter opt-in `worker-kotlin/config/export-main-jvm.sh` (init Gradle 8.10, `--offline --no-daemon`, sin bootstrap ni wrapper) captura `compileClasspath.resolvedArtifacts`, toolchain JDK, jvmTarget, sources main y build inputs, y escribe el recibo `.mi-lsp/kotlin-main-v1.authority.json`. Ese recibo no se versiona ni se generó en BUILD. Loader, compiler y adapter exigen el exporter pineado, `backup_classpath=false`, alcance distinto de stdlib-only y digests vivos. Cero diagnostics no certifica. Cambiar source/build/JAR/JDK/config/exporter invalida el snapshot.

**Sigue sin ser stageable.** `KotlinPromotionAllowed` no lee env, permiso ni etiqueta y en esta revisión devuelve false. Faltan, antes de publicar, estos oráculos de RF-GPH-008: `oracle_identity_fixture`, `oracle_relation_precision_recall`, `oracle_determinism_30_reruns`, `oracle_incremental_vs_clean`, `oracle_cross_rid`, `oracle_conformance_final_verify`. El adapter mantiene partial/experimental y agrega `promotion_oracle_missing`. No hay autogrant ni productor stable.

## Gates y compatibilidad

ValidateCanonical → core rebase → seal → strict ReadyForStaging → staging/publicación existentes. Ready exige complete, todas las capabilities stable, coverage/digest/owner/fingerprints; no bypass. Store y graph_staging son dependencias sin modificación, sin DDL ni tablas paralelas, querybackfill o migración en query. C#/Go/docgraph conservan sus keys y contratos.

Los lectores previos rechazan honestamente el nuevo enum. La primera release que incluya la extensión debe fijar mínimo lector con SHA/versión reales; ambos pendientes, no se inventa release publicada. Rollback conserva prior generation y usa binario compatible con su enum, restaurando pointer por caminos existentes, sin reinterpretar/destruir datos.

## Verificación posterior

Siete fuentes manuales bajo `testdata/kotlin-graph-main-v1/`; no runner nuevo, fixtures ejecutadas ni unit tests. Parent decide unión y única FINAL_VERIFY con JDK11/Gradle8.10/compiler2.1.0/pin fwcd acreditados, audit de otra familia y recheck. Java25/caches distintas no sustituyen pins. Este ADR no consume esos gates ni declara PASS.
