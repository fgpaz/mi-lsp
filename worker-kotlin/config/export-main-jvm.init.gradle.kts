import org.gradle.api.GradleException
import org.gradle.api.artifacts.ResolvedArtifact
import org.gradle.api.artifacts.component.ModuleComponentIdentifier
import org.gradle.api.artifacts.component.ProjectComponentIdentifier
import org.gradle.api.file.FileCollection
import org.gradle.api.file.SourceDirectorySet
import org.gradle.api.plugins.JavaPluginExtension
import org.gradle.api.tasks.SourceSetContainer
import org.gradle.jvm.toolchain.JavaToolchainService
import java.nio.file.Files
import java.nio.file.Path
import java.nio.file.Paths
import java.nio.file.StandardCopyOption
import java.security.MessageDigest

// Loaded only by export-main-jvm.sh. Absent opt-in registers nothing and does not resolve.
val miLspExportOptIn = System.getenv("MI_LSP_KOTLIN_EXPORT_MAIN_JVM") == "1" &&
    !System.getenv("MI_LSP_KOTLIN_EXPORT_MODULE").isNullOrBlank()
if (miLspExportOptIn) {
    val miLspWant = Paths.get(System.getenv("MI_LSP_KOTLIN_EXPORT_MODULE")).toRealPath()
    gradle.projectsLoaded {
        rootProject.allprojects {
            val targetProject = this
            tasks.register("miLspExportMainJvm") {
                onlyIf { targetProject.projectDir.toPath().toRealPath() == miLspWant }
                doLast { miLspCaptureMainJvm(targetProject, miLspWant) }
            }
        }
    }
}

data class MiLspHashed(val path: String, val sha256: String)
data class MiLspArtifactEvidence(val path: String, val sha256: String, val coordinate: String)
data class MiLspGap(val kind: String, val detail: String)

fun miLspCaptureMainJvm(project: org.gradle.api.Project, want: Path) {
    if (!project.gradle.startParameter.isOffline || project.gradle.startParameter.isBuildCacheEnabled) {
        throw GradleException("explicit_incomplete: capture requires --offline --no-build-cache; no bootstrap")
    }
    val blockers = mutableListOf<String>()
    val gaps = mutableListOf<MiLspGap>()
    fun block(kind: String, detail: String) {
        blockers += kind
        gaps += MiLspGap(kind, detail.take(240))
    }
    val root = project.projectDir.toPath().toRealPath()
    if (root != want) block("module_mismatch", root.toString())
    if (project.gradle.includedBuilds.isNotEmpty()) block("included_build_unrepresentable", "included builds")
    val settingsDir = project.gradle.settings.settingsDir.toPath().toRealPath()
    if (settingsDir != root) block("parent_settings_unrepresentable", settingsDir.toString())
    if (!project.pluginManager.hasPlugin("org.jetbrains.kotlin.jvm")) block("kotlin_jvm_plugin_missing", "org.jetbrains.kotlin.jvm")
    if (project.pluginManager.hasPlugin("com.android.application") || project.pluginManager.hasPlugin("com.android.library") || project.pluginManager.hasPlugin("org.jetbrains.kotlin.multiplatform")) {
        block("agp_or_multiplatform_unrepresentable", "outside main-jvm")
    }
    val javaExt = project.extensions.findByType(JavaPluginExtension::class.java) ?: run {
        block("java_extension_missing", "JavaPluginExtension")
        null
    }
    val toolchains = project.extensions.findByType(JavaToolchainService::class.java)
    var jdkHome = ""
    var jdkRelease = ""
    if (javaExt != null && toolchains != null && blockers.isEmpty()) {
        try {
            val launcher = toolchains.launcherFor(javaExt.toolchain).get()
            if (launcher.metadata.languageVersion.asInt() != 11) block("jdk_pin_mismatch", launcher.metadata.languageVersion.toString())
            val home = launcher.metadata.installationPath.asFile.toPath().toRealPath()
            jdkHome = home.toString()
            val release = home.resolve("release")
            if (!Files.isRegularFile(release)) block("jdk_release_missing", home.toString()) else jdkRelease = miLspSha(release, 2L shl 20)
        } catch (failure: Exception) {
            block("jdk_toolchain_unresolved_offline", failure.javaClass.simpleName)
        }
    } else if (toolchains == null) block("java_toolchain_missing", "JavaToolchainService")
    if (project.gradle.gradleVersion != "8.10") block("gradle_pin_mismatch", project.gradle.gradleVersion)
    val jvmTarget = miLspJvmTarget(project) ?: run { block("jvm_target_unresolved", "compileKotlin"); "" }
    if (jvmTarget.isNotEmpty() && !jvmTarget.matches(Regex("[0-9]+(\\.[0-9]+)?"))) block("jvm_target_unrepresentable", jvmTarget)
    val sources = mutableListOf<MiLspHashed>()
    if (blockers.isEmpty()) {
        val main = project.extensions.getByType(SourceSetContainer::class.java).getByName("main")
        val model = linkedSetOf<Path>()
        val kotlinExt = main.extensions.findByName("kotlin")
        val kotlinFiles = when (kotlinExt) {
            is SourceDirectorySet -> kotlinExt.files
            null -> emptySet()
            else -> {
                val nested = kotlinExt.javaClass.methods.firstOrNull { it.name == "getKotlin" && it.parameterCount == 0 }?.invoke(kotlinExt)
                when (nested) {
                    is SourceDirectorySet -> nested.files
                    is FileCollection -> nested.files
                    else -> emptySet()
                }
            }
        }
        if (kotlinFiles.isEmpty()) block("kotlin_source_set_missing", "sourceSets.main.kotlin")
        (kotlinFiles + main.java.files).forEach { file ->
            val path = file.toPath().toRealPath()
            when {
                path.toString().endsWith(".kts") -> block("scripts_unrepresentable", path.toString())
                path.toString().endsWith(".kt") || path.toString().endsWith(".java") -> model += path
                else -> block("source_unrepresentable", path.toString())
            }
        }
        val diskRoot = root.resolve("src/main")
        if (!Files.isDirectory(diskRoot)) block("src_main_missing", diskRoot.toString()) else {
            val disk = Files.walk(diskRoot).use { stream ->
                stream.filter { Files.isRegularFile(it) }.map { it.toRealPath() }.toList()
                    .filter { it.toString().endsWith(".kt") || it.toString().endsWith(".java") || it.toString().endsWith(".kts") }
            }
            if (disk.any { it.toString().endsWith(".kts") }) block("scripts_unrepresentable", "src/main")
            if (disk.map { it.toString() }.toSet() != model.map { it.toString() }.toSet()) block("source_inventory_mismatch", "src/main")
        }
        if (model.isEmpty() || model.none { it.toString().endsWith(".kt") }) block("no_kotlin_main_sources", "src/main")
        if (model.size > 512) block("source_bound", model.size.toString())
        model.sortedBy { it.toString() }.forEach { path ->
            if (Files.size(path) > 2L shl 20) block("source_bound", path.toString()) else sources += MiLspHashed(path.toString(), miLspSha(path, 2L shl 20))
        }
    }
    val classpath = mutableListOf<MiLspHashed>()
    val artifacts = mutableListOf<MiLspArtifactEvidence>()
    if (blockers.isEmpty()) {
        try {
            val conf = project.configurations.getByName("compileClasspath")
            val resolved = conf.resolvedConfiguration
            if (resolved.hasError()) block("classpath_resolution_error", "compileClasspath") else {
                var total = 0L
                resolved.resolvedArtifacts.sortedBy { it.file.absolutePath }.forEach { artifact ->
                    val file = artifact.file.toPath().toRealPath()
                    if (!file.toString().endsWith(".jar")) block("non_jar_classpath", file.toString()) else {
                        val size = Files.size(file)
                        total += size
                        if (size > 256L shl 20 || total > 1L shl 30) block("classpath_bound", file.toString()) else {
                            val digest = miLspSha(file, 256L shl 20)
                            val coordinate = miLspCoordinate(artifact)
                            classpath += MiLspHashed(file.toString(), digest)
                            artifacts += MiLspArtifactEvidence(file.toString(), digest, coordinate)
                        }
                    }
                }
            }
        } catch (failure: Exception) {
            block("classpath_resolution_error", failure.javaClass.simpleName)
        }
        if (classpath.isEmpty()) block("classpath_empty", "compileClasspath")
        if (classpath.size > 256) block("classpath_bound", classpath.size.toString())
        if (artifacts.none { !miLspStdlib(it.coordinate) }) block("stdlib_only", "kotlin-stdlib")
        if (artifacts.none { it.coordinate == "module:org.jetbrains.kotlin:kotlin-stdlib:2.1.0" || it.coordinate.startsWith("module:org.jetbrains.kotlin:kotlin-stdlib-jdk") && it.coordinate.endsWith(":2.1.0") }) {
            block("compiler_pin_mismatch", "kotlin-stdlib 2.1.0 absent")
        }
    }
    val buildInputs = mutableListOf<MiLspHashed>()
    listOf("build.gradle", "build.gradle.kts", "settings.gradle", "settings.gradle.kts", "gradle.properties", "gradle/libs.versions.toml", "gradle/wrapper/gradle-wrapper.properties").forEach { rel ->
        val path = root.resolve(rel)
        if (Files.isRegularFile(path)) {
            if (Files.size(path) > 2L shl 20) block("build_input_bound", rel) else buildInputs += MiLspHashed(path.toRealPath().toString(), miLspSha(path, 2L shl 20))
        }
    }
    if (buildInputs.none { it.path.endsWith("build.gradle") || it.path.endsWith("build.gradle.kts") }) block("build_file_missing", "build.gradle")
    if (buildInputs.none { it.path.endsWith("settings.gradle") || it.path.endsWith("settings.gradle.kts") }) block("settings_file_missing", "settings.gradle")
    if (buildInputs.size > 128) block("build_input_bound", buildInputs.size.toString())
    val exporter = Paths.get(System.getenv("MI_LSP_KOTLIN_EXPORTER_PATH") ?: "").toAbsolutePath()
    val init = Paths.get(System.getenv("MI_LSP_KOTLIN_INIT_PATH") ?: "").toAbsolutePath()
    if (!Files.isRegularFile(exporter) || exporter.fileName.toString() != "export-main-jvm.sh") block("exporter_path_invalid", exporter.toString())
    if (!Files.isRegularFile(init) || init.fileName.toString() != "export-main-jvm.init.gradle.kts") block("init_script_path_invalid", init.toString())
    val exporterSha = if (Files.isRegularFile(exporter)) miLspSha(exporter, 2L shl 20) else ""
    val initSha = if (Files.isRegularFile(init)) miLspSha(init, 2L shl 20) else ""
    val oracles = listOf("oracle_identity_fixture", "oracle_relation_precision_recall", "oracle_determinism_30_reruns", "oracle_incremental_vs_clean", "oracle_cross_rid", "oracle_conformance_final_verify")
    val certified = blockers.isEmpty() && sources.isNotEmpty() && classpath.isNotEmpty() && jdkHome.isNotEmpty() && jdkRelease.isNotEmpty() && jvmTarget.isNotEmpty()
    val miDir = root.resolve(".mi-lsp")
    Files.createDirectories(miDir)
    val configPath = miDir.resolve("kotlin-main-v1.json")
    val receiptPath = miDir.resolve("kotlin-main-v1.authority.json")
    if (!certified) {
        Files.deleteIfExists(configPath)
        miLspWrite(receiptPath, miLspReceipt(false, blockers.distinct(), gaps.distinctBy { it.kind }, root, project.name, jdkHome, jdkRelease, jvmTarget, "", exporter, exporterSha, init, initSha, sources, classpath, buildInputs, artifacts, oracles))
        throw GradleException("explicit_incomplete: " + blockers.distinct().joinToString(","))
    }
    val config = miLspConfig(root, project.name, jdkHome, jvmTarget, sources, classpath, buildInputs)
    miLspWrite(configPath, config)
    val configSha = miLspSha(configPath, 2L shl 20)
    miLspWrite(receiptPath, miLspReceipt(true, emptyList(), emptyList(), root, project.name, jdkHome, jdkRelease, jvmTarget, configSha, exporter, exporterSha, init, initSha, sources, classpath, buildInputs, artifacts, oracles))
}

fun miLspJvmTarget(project: org.gradle.api.Project): String? {
    val task = project.tasks.findByName("compileKotlin") ?: return null
    val options = task.javaClass.methods.firstOrNull { it.name == "getCompilerOptions" && it.parameterCount == 0 }?.invoke(task) ?: return null
    val property = options.javaClass.methods.firstOrNull { it.name == "getJvmTarget" && it.parameterCount == 0 }?.invoke(options) ?: return null
    val raw = property.javaClass.methods.firstOrNull { it.name == "getOrNull" && it.parameterCount == 0 }?.invoke(property)?.toString() ?: return null
    return raw.removePrefix("JVM_").removePrefix("jvm_")
}

fun miLspCoordinate(artifact: ResolvedArtifact): String {
    val id = artifact.id.componentIdentifier
    return when (id) {
        is ModuleComponentIdentifier -> "module:${id.group}:${id.module}:${id.version}"
        is ProjectComponentIdentifier -> "project:${id.projectPath}"
        else -> "file:${artifact.file.toPath().toRealPath()}"
    }
}

fun miLspStdlib(coordinate: String): Boolean {
    val parts = coordinate.split(":")
    return parts.size >= 3 && parts[0] == "module" && parts[2].startsWith("kotlin-stdlib")
}

fun miLspSha(path: Path, limit: Long): String {
    val size = Files.size(path)
    if (size > limit) throw GradleException("explicit_incomplete: input bound")
    val digest = MessageDigest.getInstance("SHA-256")
    Files.newInputStream(path).use { input ->
        val buf = ByteArray(1 shl 16)
        var n = input.read(buf)
        while (n >= 0) {
            digest.update(buf, 0, n)
            n = input.read(buf)
        }
    }
    return digest.digest().joinToString("") { "%02x".format(it.toInt() and 255) }
}

fun miLspJson(value: String): String = buildString {
    append('"')
    value.forEach { ch ->
        when (ch) {
            '\\' -> append("\\\\")
            '"' -> append("\\\"")
            '\n' -> append("\\n")
            '\r' -> append("\\r")
            '\t' -> append("\\t")
            else -> append(ch)
        }
    }
    append('"')
}

fun miLspInputs(items: List<MiLspHashed>): String = items.joinToString(",\n") { "    {\"path\": ${miLspJson(it.path)}, \"sha256\": ${miLspJson(it.sha256)}}" }

fun miLspConfig(root: Path, name: String, jdk: String, target: String, sources: List<MiLspHashed>, classpath: List<MiLspHashed>, buildInputs: List<MiLspHashed>): String = """
{
  "schema_version": 1,
  "resolution": "resolved",
  "module_root": ${miLspJson(root.toString())},
  "module_name": ${miLspJson(name)},
  "source_set": "main-jvm",
  "jdk_home": ${miLspJson(jdk)},
  "jvm_target": ${miLspJson(target)},
  "gradle_version": "8.10",
  "compiler_version": "2.1.0",
  "server_version": "1.3.13+mi-lsp.obs2",
  "sources": [
${miLspInputs(sources)}
  ],
  "classpath": [
${miLspInputs(classpath)}
  ],
  "build_inputs": [
${miLspInputs(buildInputs)}
  ]
}
""".trimStart()

fun miLspPlain(value: String) {
    if (value.contains('\n') || value.contains('\r') || value.contains('\t')) throw GradleException("explicit_incomplete: delimiter in evidence")
}

fun miLspEvidence(certified: Boolean, root: Path, name: String, jdk: String, jdkRelease: String, target: String, exporterSha: String, initSha: String, sources: List<MiLspHashed>, classpath: List<MiLspHashed>, buildInputs: List<MiLspHashed>, artifacts: List<MiLspArtifactEvidence>, oracles: List<String>): String {
    if (!certified) return ""
    listOf(name, jdk, jdkRelease, target, exporterSha, initSha, root.toString()).forEach(::miLspPlain)
    (sources + classpath + buildInputs).forEach { miLspPlain(it.path); miLspPlain(it.sha256) }
    artifacts.forEach { miLspPlain(it.path); miLspPlain(it.sha256); miLspPlain(it.coordinate) }
    val lines = mutableListOf(
        "authority=gradle-compileClasspath-resolvedArtifacts/v1",
        "capture=explicit-opt-in-offline",
        "resolver=configurations.compileClasspath.resolvedArtifacts",
        "gradle=8.10",
        "compiler=2.1.0",
        "server=1.3.13+mi-lsp.obs2",
        "jdk_home=$jdk",
        "jdk_release=$jdkRelease",
        "jvm_target=$target",
        "module_name=$name",
        "module_root=${root.toString()}",
        "exporter_sha256=$exporterSha",
        "init_script_sha256=$initSha",
        "promotion_oracles=${oracles.joinToString(",")}"
    )
    sources.forEach { lines += "source\t${it.path}\t${it.sha256}" }
    classpath.forEach { lines += "classpath\t${it.path}\t${it.sha256}" }
    artifacts.forEach { lines += "artifact\t${it.path}\t${it.sha256}\t${it.coordinate}" }
    buildInputs.forEach { lines += "build_input\t${it.path}\t${it.sha256}" }
    val digest = MessageDigest.getInstance("SHA-256").digest(lines.joinToString("\n").toByteArray(Charsets.UTF_8))
    return digest.joinToString("") { "%02x".format(it.toInt() and 255) }
}

fun miLspReceipt(certified: Boolean, blockers: List<String>, gaps: List<MiLspGap>, root: Path, name: String, jdk: String, jdkRelease: String, target: String, configSha: String, exporter: Path, exporterSha: String, init: Path, initSha: String, sources: List<MiLspHashed>, classpath: List<MiLspHashed>, buildInputs: List<MiLspHashed>, artifacts: List<MiLspArtifactEvidence>, oracles: List<String>): String {
    val scope = if (certified) "project-effective" else if (blockers.contains("stdlib_only")) "stdlib-only" else "unresolved"
    val evidence = miLspEvidence(certified, root, name, jdk, jdkRelease, target, exporterSha, initSha, sources, classpath, buildInputs, artifacts, oracles)
    val blockerJson = blockers.joinToString(",\n") { "    ${miLspJson(it)}" }
    val oracleJson = oracles.joinToString(",\n") { "    ${miLspJson(it)}" }
    val gapJson = gaps.joinToString(",\n") { "    {\"kind\": ${miLspJson(it.kind)}, \"detail\": ${miLspJson(it.detail)}}" }
    val artifactJson = artifacts.joinToString(",\n") { "    {\"path\": ${miLspJson(it.path)}, \"sha256\": ${miLspJson(it.sha256)}, \"coordinate\": ${miLspJson(it.coordinate)}}" }
    return """
{
  "receipt_version": 1,
  "authority": "gradle-compileClasspath-resolvedArtifacts/v1",
  "capture": "explicit-opt-in-offline",
  "certified": $certified,
  "certification_blockers": [
${blockerJson}
  ],
  "resolver": "configurations.compileClasspath.resolvedArtifacts",
  "backup_classpath": false,
  "classpath_scope": ${miLspJson(scope)},
  "calls": "unsupported",
  "graph_completeness": "partial",
  "promotion_oracles_missing": [
${oracleJson}
  ],
  "module_root": ${miLspJson(root.toString())},
  "module_name": ${miLspJson(name)},
  "source_set": "main-jvm",
  "jdk_home": ${miLspJson(jdk)},
  "jdk_release_sha256": ${miLspJson(jdkRelease)},
  "jvm_target": ${miLspJson(target)},
  "gradle_version": "8.10",
  "compiler_version": "2.1.0",
  "server_version": "1.3.13+mi-lsp.obs2",
  "server_version_binding": "contract_pin_not_runtime_proof",
  "configuration_sha256": ${miLspJson(configSha)},
  "exporter_path": ${miLspJson(exporter.toString())},
  "exporter_sha256": ${miLspJson(exporterSha)},
  "init_script_path": ${miLspJson(init.toString())},
  "init_script_sha256": ${miLspJson(initSha)},
  "evidence_digest": ${miLspJson(evidence)},
  "capture_command": ${miLspJson("gradle --offline --no-daemon --no-build-cache -p $root -I $init miLspExportMainJvm")},
  "sources": [
${miLspInputs(sources)}
  ],
  "classpath": [
${miLspInputs(classpath)}
  ],
  "build_inputs": [
${miLspInputs(buildInputs)}
  ],
  "artifacts": [
${artifactJson}
  ],
  "unrepresentable": [
${gapJson}
  ]
}
""".trimStart()
}

fun miLspWrite(path: Path, body: String) {
    val tmp = path.resolveSibling(path.fileName.toString() + ".tmp")
    Files.writeString(tmp, body)
    Files.move(tmp, path, StandardCopyOption.ATOMIC_MOVE, StandardCopyOption.REPLACE_EXISTING)
}
