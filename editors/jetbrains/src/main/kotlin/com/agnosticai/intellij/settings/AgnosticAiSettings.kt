package com.agnosticai.intellij.settings

import com.intellij.openapi.application.ApplicationManager
import com.intellij.openapi.components.PersistentStateComponent
import com.intellij.openapi.components.Service
import com.intellij.openapi.components.State
import com.intellij.openapi.components.Storage
import com.intellij.util.xmlb.XmlSerializerUtil

@Service(Service.Level.APP)
@State(name = "AgnosticAiSettings", storages = [Storage("agnostic-ai.xml")])
class AgnosticAiSettings : PersistentStateComponent<AgnosticAiSettings.State> {
    data class State(
        var binaryPath: String = "",
        var driftPollSeconds: Int = 30,
        // Toggles the editor banner that appears above spec files. The
        // legacy "lineMarker" name persists in serialized state.
        var lineMarkerEnabled: Boolean = true,
    )

    private var state = State()

    override fun getState(): State = state
    override fun loadState(state: State) {
        XmlSerializerUtil.copyBean(state, this.state)
    }

    companion object {
        val instance: AgnosticAiSettings
            get() = ApplicationManager.getApplication().getService(AgnosticAiSettings::class.java)
    }
}
