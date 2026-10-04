// Process and project-root helpers shared by every entry point.
//
// All actions, the line marker, and the status bar widget shell out to
// the user's installed `agnostic-ai` binary; nothing in this plugin
// bundles the binary or reimplements its logic. Centralizing the
// helpers keeps each surface tiny and consistent.

package com.agnosticai.intellij

import com.agnosticai.intellij.settings.AgnosticAiSettings
import com.intellij.openapi.project.Project
import com.intellij.openapi.vfs.VirtualFile
import java.nio.file.Files
import java.nio.file.Path
import java.util.concurrent.TimeUnit

object AgnosticAi {
    /** Resolves the binary path from settings, defaulting to `agnostic-ai` on PATH. */
    fun binary(): String {
        val configured = AgnosticAiSettings.instance.state.binaryPath.trim()
        return if (configured.isEmpty()) "agnostic-ai" else configured
    }

    /** Config file names in lookup order: the CLI prefers agnostic-ai.yaml. */
    val CONFIG_FILE_NAMES = listOf("agnostic-ai.yaml", "agnostic.config.yaml")

    /** The per-developer file the CLI merges over the base config. */
    const val LOCAL_OVERRIDE_FILE_NAME = "agnostic-ai.local.yaml"

    /** Whether a file name is one of the config file names. */
    fun isConfigFileName(name: String): Boolean = name in CONFIG_FILE_NAMES

    /** The config file in dir, preferring agnostic-ai.yaml like the CLI. */
    fun configFile(dir: Path): Path? = CONFIG_FILE_NAMES.map { dir.resolve(it) }.firstOrNull { Files.exists(it) }

    /**
     * Returns the first workspace folder that contains a config file,
     * or the project base path as a fallback so a clean tree can still
     * launch the binary (which surfaces its own error).
     */
    fun projectRoot(project: Project): Path? = project.basePath?.let { projectRoot(Path.of(it)) }

    /** The folder under basePath that holds the config, children sorted by name. */
    fun projectRoot(basePath: Path): Path {
        if (configFile(basePath) != null) return basePath
        // Walk one level deep for a Gradle/Maven multi-module setup.
        val children = runCatching { Files.list(basePath).use { it.sorted().toList() } }.getOrNull() ?: emptyList()
        for (child in children) {
            if (configFile(child) != null) return child
        }
        return basePath
    }

    /**
     * Reads the configured targets straight from the config files without
     * spawning a process. A `targets:` list in the local override replaces
     * the base list, as the CLI's merge does.
     */
    fun configuredTargets(root: Path): List<String> {
        val cfg = configFile(root) ?: return emptyList()
        val local = root.resolve(LOCAL_OVERRIDE_FILE_NAME)
        val override = if (Files.exists(local)) runCatching { parseTargetList(Files.readString(local)) }.getOrNull() else null
        return override ?: runCatching { parseTargetList(Files.readString(cfg)) }.getOrNull() ?: emptyList()
    }

    /**
     * The top-level `targets:` list, in block or flow style. Null when the
     * file sets no targets or uses a shape this line reader cannot follow,
     * so a caller falls back instead of showing none.
     */
    fun parseTargetList(text: String): List<String>? {
        val all = text.removePrefix("\uFEFF").lines().map { it.replace(Regex("""(^|\s)#.*$"""), "").trimEnd() }
        // The CLI reads only the first YAML document. A marker before any
        // content starts that document instead of ending it.
        val marker = Regex("""^(---|\.\.\.)(\s|$)""")
        val docEnd = all.withIndex().indexOfFirst { (i, l) ->
            marker.containsMatchIn(l) && all.take(i).any { it.isNotEmpty() && !it.startsWith("%") && !marker.containsMatchIn(it) }
        }
        val lines = if (docEnd < 0) all else all.take(docEnd)
        val key = Regex("""^(targets|"targets"|'targets'):(\s|$)""")
        val start = lines.indexOfFirst { key.containsMatchIn(it) }
        if (start < 0) return null
        val value = lines[start].substringAfter(':').trim()
        if (value.isEmpty()) return blockItems(lines.drop(start + 1))
        if (value in setOf("null", "Null", "NULL", "~")) return emptyList()
        if (!value.startsWith("[")) return null
        val flow = (listOf(value) + lines.drop(start + 1)).joinToString(" ")
        val end = flow.indexOf(']')
        if (end < 0) return null
        return targetNames(flow.substring(1, end).split(",").map(::unquote).filter { it.isNotEmpty() })
    }

    // An alias, escape, or nested collection is not a plain target name, so
    // the whole list is unreadable here.
    private fun targetNames(items: List<String>): List<String>? =
        if (items.all { Regex("""^[A-Za-z0-9][A-Za-z0-9_.-]*$""").matches(it) }) items else null

    private fun blockItems(lines: List<String>): List<String>? {
        val items = mutableListOf<String>()
        for (line in lines) {
            if (line.isEmpty()) continue
            val match = Regex("""^\s*-\s+(\S+)$""").find(line)
            if (match != null) {
                items += unquote(match.groupValues[1]); continue
            }
            if (Regex("""^[^\s-]""").containsMatchIn(line)) break
            return null
        }
        return targetNames(items)
    }

    private fun unquote(value: String): String = value.trim().replace(Regex("""^(["'])(.*)\1$"""), "$2")

    /** Quick check used by the line marker to suppress non-spec files. */
    fun isInsideSpecSources(root: Path, file: VirtualFile): Boolean {
        val filePath = runCatching { Path.of(file.path) }.getOrNull() ?: return false
        val rel = runCatching { root.relativize(filePath) }.getOrNull() ?: return false
        val segs = rel.toString().replace('\\', '/').split('/')
        if (segs.size < 2) return false
        val kinds = setOf("agents", "skills", "rules", "hooks", "mcps")
        if (kinds.contains(segs[0])) return true
        if (segs.size >= 3 && segs[0].startsWith(".") && kinds.contains(segs[1])) return true
        return false
    }

    data class ExecResult(val exitCode: Int, val stdout: String, val stderr: String, val binaryFound: Boolean)

    /**
     * Runs the binary synchronously. Caller decides how to surface the
     * result; we never raise, only report. binaryFound=false means the
     * configured binary was not on PATH and the caller should pop the
     * "install agnostic-ai" hint.
     */
    fun exec(args: List<String>, cwd: Path, timeoutSeconds: Long = 30): ExecResult {
        val cmd = mutableListOf(binary())
        cmd.addAll(args)
        return try {
            val process = ProcessBuilder(cmd)
                .directory(cwd.toFile())
                .redirectErrorStream(false)
                .start()
            val finished = process.waitFor(timeoutSeconds, TimeUnit.SECONDS)
            if (!finished) {
                process.destroyForcibly()
                ExecResult(-2, "", "agnostic-ai timed out after ${timeoutSeconds}s", true)
            } else {
                ExecResult(
                    exitCode = process.exitValue(),
                    stdout = String(process.inputStream.readAllBytes()),
                    stderr = String(process.errorStream.readAllBytes()),
                    binaryFound = true,
                )
            }
        } catch (_: java.io.IOException) {
            ExecResult(-1, "", "agnostic-ai not found on PATH", false)
        }
    }
}
