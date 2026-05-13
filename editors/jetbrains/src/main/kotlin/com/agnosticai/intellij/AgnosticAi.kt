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

    /**
     * Returns the first workspace folder that contains agnostic.config.yaml,
     * or the project base path as a fallback so a clean tree can still
     * launch the binary (which surfaces its own error).
     */
    fun projectRoot(project: Project): Path? {
        val base = project.basePath ?: return null
        val basePath = Path.of(base)
        if (Files.exists(basePath.resolve("agnostic.config.yaml"))) return basePath
        // Walk one level deep for a Gradle/Maven multi-module setup.
        val children = runCatching { Files.list(basePath).use { it.toList() } }.getOrNull() ?: emptyList()
        for (child in children) {
            if (Files.exists(child.resolve("agnostic.config.yaml"))) return child
        }
        return basePath
    }

    /** Reads the configured targets straight from agnostic.config.yaml without spawning a process. */
    fun configuredTargets(root: Path): List<String> {
        val cfg = root.resolve("agnostic.config.yaml")
        if (!Files.exists(cfg)) return emptyList()
        val text = Files.readString(cfg)
        val out = mutableListOf<String>()
        var inTargets = false
        for (raw in text.lineSequence()) {
            val line = raw.trimEnd()
            if (line.matches(Regex("""^targets:\s*$"""))) {
                inTargets = true; continue
            }
            if (!inTargets) continue
            val match = Regex("""^\s+-\s+(\S+)""").find(line)
            if (match != null) {
                out += match.groupValues[1]; continue
            }
            if (line.isNotEmpty() && !line.startsWith(" ") && !line.startsWith("\t")) inTargets = false
        }
        return out
    }

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
