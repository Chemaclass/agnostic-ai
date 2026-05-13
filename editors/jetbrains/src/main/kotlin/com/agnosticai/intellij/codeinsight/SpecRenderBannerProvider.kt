// Editor banner above each spec file. The JetBrains analogue of VS
// Code's CodeLens: same surface (one button per configured target),
// different IDE idiom (top-of-editor banner vs inline link).
//
// Banner only shows for files that live under one of the configured
// `<base>/{agents,skills,rules,hooks,mcps}/` directories. Suppressed
// for everything else, including the user's source code.

package com.agnosticai.intellij.codeinsight

import com.agnosticai.intellij.AgnosticAi
import com.agnosticai.intellij.settings.AgnosticAiSettings
import com.intellij.notification.NotificationGroupManager
import com.intellij.notification.NotificationType
import com.intellij.openapi.application.ApplicationManager
import com.intellij.openapi.fileEditor.FileEditor
import com.intellij.openapi.progress.ProgressIndicator
import com.intellij.openapi.progress.Task
import com.intellij.openapi.project.Project
import com.intellij.openapi.vfs.VirtualFile
import com.intellij.ui.EditorNotificationPanel
import com.intellij.ui.EditorNotificationProvider
import java.nio.file.Path
import java.util.function.Function
import javax.swing.JComponent

class SpecRenderBannerProvider : EditorNotificationProvider {
    override fun collectNotificationData(
        project: Project,
        file: VirtualFile,
    ): Function<in FileEditor, out JComponent?>? {
        if (!AgnosticAiSettings.instance.state.lineMarkerEnabled) return null
        val root = AgnosticAi.projectRoot(project) ?: return null
        if (!AgnosticAi.isInsideSpecSources(root, file)) return null
        val targets = AgnosticAi.configuredTargets(root)
        if (targets.isEmpty()) return null
        return Function { _: FileEditor -> buildPanel(project, root, file, targets) }
    }

    private fun buildPanel(
        project: Project,
        root: Path,
        file: VirtualFile,
        targets: List<String>,
    ): JComponent {
        val panel = EditorNotificationPanel(EditorNotificationPanel.Status.Info)
        panel.text = "agnostic-ai: render this spec"
        for (target in targets) {
            panel.createActionLabel(target) { renderToTarget(project, root, file.path, target) }
        }
        return panel
    }

    private fun renderToTarget(project: Project, root: Path, specPath: String, target: String) {
        object : Task.Backgroundable(project, "agnostic-ai render → $target", true) {
            override fun run(indicator: ProgressIndicator) {
                val rel = root.relativize(Path.of(specPath))
                val res = AgnosticAi.exec(listOf("render", rel.toString(), "--target", target), root)
                ApplicationManager.getApplication().invokeLater {
                    if (!res.binaryFound) {
                        notify(project, "agnostic-ai not found on PATH.", NotificationType.ERROR)
                        return@invokeLater
                    }
                    if (res.exitCode != 0) {
                        notify(project, "render failed: ${res.stderr.ifBlank { "exit ${res.exitCode}" }}", NotificationType.WARNING)
                        return@invokeLater
                    }
                    val excerpt = res.stdout.lineSequence().take(40).joinToString("\n")
                    notify(project, "$target →\n$excerpt", NotificationType.INFORMATION)
                }
            }
        }.queue()
    }

    private fun notify(project: Project, message: String, type: NotificationType) {
        NotificationGroupManager.getInstance()
            .getNotificationGroup("agnostic-ai")
            .createNotification(message, type)
            .notify(project)
    }
}
