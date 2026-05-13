// Tools menu actions for agnostic-ai.
//
// Each action runs the binary in the project root via a background
// task and reports success or failure as an IDE notification. Stderr
// excerpts are inlined so the user does not have to dig through a log.

package com.agnosticai.intellij.actions

import com.agnosticai.intellij.AgnosticAi
import com.intellij.notification.NotificationGroupManager
import com.intellij.notification.NotificationType
import com.intellij.openapi.actionSystem.ActionUpdateThread
import com.intellij.openapi.actionSystem.AnAction
import com.intellij.openapi.actionSystem.AnActionEvent
import com.intellij.openapi.progress.ProgressIndicator
import com.intellij.openapi.progress.Task
import com.intellij.openapi.project.Project
import com.intellij.openapi.application.ApplicationManager

abstract class AgnosticAiAction(
    private val args: List<String>,
    private val title: String,
) : AnAction() {
    override fun getActionUpdateThread(): ActionUpdateThread = ActionUpdateThread.BGT

    override fun update(e: AnActionEvent) {
        e.presentation.isEnabled = e.project != null
    }

    override fun actionPerformed(e: AnActionEvent) {
        val project = e.project ?: return
        runBinary(project, args, title)
    }
}

class SyncAction : AgnosticAiAction(listOf("sync"), "Sync")
class SyncCheckAction : AgnosticAiAction(listOf("sync", "--check"), "Sync — check")
class DoctorFixAction : AgnosticAiAction(listOf("doctor", "--fix"), "Doctor — auto-fix")
class StatusAction : AgnosticAiAction(listOf("status"), "Status")

internal fun runBinary(project: Project, args: List<String>, title: String) {
    val root = AgnosticAi.projectRoot(project) ?: run {
        notify(project, "agnostic-ai: open a project folder first.", NotificationType.WARNING)
        return
    }
    object : Task.Backgroundable(project, "agnostic-ai $title", true) {
        override fun run(indicator: ProgressIndicator) {
            indicator.text = "agnostic-ai ${args.joinToString(" ")}"
            val result = AgnosticAi.exec(args, root, timeoutSeconds = 60)
            ApplicationManager.getApplication().invokeLater {
                if (!result.binaryFound) {
                    notify(
                        project,
                        "agnostic-ai not found on PATH. Set Settings ▸ Tools ▸ agnostic-ai ▸ Binary path or install it from the project README.",
                        NotificationType.ERROR,
                    )
                    return@invokeLater
                }
                val type = if (result.exitCode == 0) NotificationType.INFORMATION else NotificationType.WARNING
                val excerpt = (result.stdout.ifBlank { result.stderr })
                    .lines()
                    .filter { it.isNotBlank() }
                    .takeLast(8)
                    .joinToString("\n")
                val tail = if (excerpt.isNotEmpty()) "\n\n$excerpt" else ""
                val msg = if (result.exitCode == 0)
                    "agnostic-ai $title finished.$tail"
                else
                    "agnostic-ai $title exited with code ${result.exitCode}.$tail"
                notify(project, msg, type)
            }
        }
    }.queue()
}

internal fun notify(project: Project, message: String, type: NotificationType) {
    NotificationGroupManager.getInstance()
        .getNotificationGroup("agnostic-ai")
        .createNotification(message, type)
        .notify(project)
}
