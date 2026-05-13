// Status bar widget that polls `agnostic-ai sync --check --json` and
// shows the current drift count. Click to open the agnostic-ai Tools
// menu (where Sync/Sync — check live).

package com.agnosticai.intellij.statusbar

import com.agnosticai.intellij.AgnosticAi
import com.agnosticai.intellij.settings.AgnosticAiSettings
import com.intellij.openapi.actionSystem.ActionManager
import com.intellij.openapi.actionSystem.ActionPlaces
import com.intellij.openapi.application.ApplicationManager
import com.intellij.openapi.project.Project
import com.intellij.openapi.util.Disposer
import com.intellij.openapi.wm.StatusBar
import com.intellij.openapi.wm.StatusBarWidget
import com.intellij.openapi.wm.StatusBarWidgetFactory
import com.intellij.util.Alarm
import java.awt.event.MouseEvent
import java.util.concurrent.TimeUnit

class DriftStatusBarFactory : StatusBarWidgetFactory {
    override fun getId(): String = "com.agnosticai.intellij.statusbar.DriftStatusBarFactory"
    override fun getDisplayName(): String = "agnostic-ai drift"
    override fun isAvailable(project: Project): Boolean =
        AgnosticAi.projectRoot(project)?.let { java.nio.file.Files.exists(it.resolve("agnostic.config.yaml")) } == true
    override fun createWidget(project: Project): StatusBarWidget = DriftWidget(project)
    override fun disposeWidget(widget: StatusBarWidget) = Disposer.dispose(widget)
}

private class DriftWidget(private val project: Project) :
    StatusBarWidget,
    StatusBarWidget.TextPresentation {
    private val alarm = Alarm(Alarm.ThreadToUse.POOLED_THREAD, this)
    private var statusBar: StatusBar? = null
    @Volatile private var text: String = "agnostic-ai: …"
    @Volatile private var tooltip: String = "agnostic-ai drift count"

    override fun ID(): String = "com.agnosticai.intellij.statusbar.DriftWidget"
    override fun getPresentation(): StatusBarWidget.WidgetPresentation = this
    override fun getText(): String = text
    override fun getAlignment(): Float = 0f
    override fun getTooltipText(): String = tooltip
    override fun getClickConsumer(): com.intellij.util.Consumer<MouseEvent>? = com.intellij.util.Consumer {
        val action = ActionManager.getInstance().getAction("com.agnosticai.intellij.actions.SyncCheckAction")
        if (action != null) {
            ActionManager.getInstance().tryToExecute(
                action,
                null,
                null,
                ActionPlaces.STATUS_BAR_PLACE,
                true,
            )
        }
    }

    override fun install(statusBar: StatusBar) {
        this.statusBar = statusBar
        scheduleRefresh(0)
    }

    override fun dispose() {
        alarm.cancelAllRequests()
    }

    private fun scheduleRefresh(delayMillis: Int) {
        if (alarm.isDisposed) return
        alarm.addRequest({ refresh() }, delayMillis)
    }

    private fun refresh() {
        val root = AgnosticAi.projectRoot(project) ?: return
        val res = AgnosticAi.exec(listOf("sync", "--check", "--json"), root, timeoutSeconds = 15)
        val (newText, newTooltip) = when {
            !res.binaryFound -> "agnostic-ai: not found" to "Install agnostic-ai or set Settings ▸ Tools ▸ agnostic-ai ▸ Binary path"
            res.exitCode == 0 -> "agnostic-ai: in sync" to "All target files are up to date"
            else -> {
                val drift = parseWritesCount(res.stdout)
                if (drift >= 0) "agnostic-ai: $drift drifted" to "Run Tools ▸ agnostic-ai ▸ Sync to reconcile"
                else "agnostic-ai: drift" to res.stderr.lineSequence().firstOrNull().orEmpty()
            }
        }
        text = newText
        tooltip = newTooltip
        ApplicationManager.getApplication().invokeLater { statusBar?.updateWidget(ID()) }
        val secs = AgnosticAiSettings.instance.state.driftPollSeconds.coerceAtLeast(5)
        scheduleRefresh(TimeUnit.SECONDS.toMillis(secs.toLong()).toInt())
    }
}

/**
 * Counts entries in the JSON `writes` array without pulling in a full
 * JSON parser. The schema is stable
 * (`{"version":"1","command":"sync --check","writes":[...],"skipped":[...],"errors":[...]}`),
 * so a tiny matcher is enough and keeps the plugin dependency-free.
 */
internal fun parseWritesCount(json: String): Int {
    val key = "\"writes\""
    val keyIdx = json.indexOf(key)
    if (keyIdx < 0) return -1
    val openBracket = json.indexOf('[', startIndex = keyIdx + key.length)
    if (openBracket < 0) return -1
    var depth = 0
    var count = 0
    var sawObject = false
    var i = openBracket
    while (i < json.length) {
        when (json[i]) {
            '[', '{' -> {
                depth++
                if (json[i] == '{' && depth == 2) sawObject = true
            }
            ']' -> {
                depth--
                if (depth == 0) {
                    return count
                }
            }
            '}' -> {
                depth--
                if (depth == 1 && sawObject) {
                    count++
                    sawObject = false
                }
            }
        }
        i++
    }
    return -1
}
