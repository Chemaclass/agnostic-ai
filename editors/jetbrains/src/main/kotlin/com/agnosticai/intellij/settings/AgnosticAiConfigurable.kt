package com.agnosticai.intellij.settings

import com.intellij.openapi.options.Configurable
import com.intellij.ui.components.JBLabel
import com.intellij.ui.components.JBTextField
import com.intellij.util.ui.FormBuilder
import javax.swing.JCheckBox
import javax.swing.JComponent
import javax.swing.JPanel
import javax.swing.JSpinner
import javax.swing.SpinnerNumberModel

class AgnosticAiConfigurable : Configurable {
    private val binaryField = JBTextField()
    private val driftSpinner = JSpinner(SpinnerNumberModel(30, 5, 600, 5))
    private val lineMarkerCheck = JCheckBox("Show 'Render to <target>' banner above each spec")

    private var panel: JPanel? = null

    override fun getDisplayName(): String = "agnostic-ai"

    override fun createComponent(): JComponent {
        val s = AgnosticAiSettings.instance.state
        binaryField.text = s.binaryPath
        binaryField.emptyText.text = "agnostic-ai (resolved on PATH)"
        driftSpinner.value = s.driftPollSeconds.coerceIn(5, 600)
        lineMarkerCheck.isSelected = s.lineMarkerEnabled

        val form = FormBuilder.createFormBuilder()
            .addLabeledComponent(JBLabel("Binary path:"), binaryField, 1, false)
            .addLabeledComponent(JBLabel("Drift poll (seconds):"), driftSpinner, 1, false)
            .addComponent(lineMarkerCheck, 1)
            .addComponentFillVertically(JPanel(), 0)
            .panel
        panel = form
        return form
    }

    override fun isModified(): Boolean {
        val s = AgnosticAiSettings.instance.state
        return binaryField.text != s.binaryPath ||
            (driftSpinner.value as Int) != s.driftPollSeconds ||
            lineMarkerCheck.isSelected != s.lineMarkerEnabled
    }

    override fun apply() {
        val s = AgnosticAiSettings.instance.state
        s.binaryPath = binaryField.text.trim()
        s.driftPollSeconds = (driftSpinner.value as Int).coerceIn(5, 600)
        s.lineMarkerEnabled = lineMarkerCheck.isSelected
    }

    override fun reset() {
        val s = AgnosticAiSettings.instance.state
        binaryField.text = s.binaryPath
        driftSpinner.value = s.driftPollSeconds.coerceIn(5, 600)
        lineMarkerCheck.isSelected = s.lineMarkerEnabled
    }

    override fun disposeUIResources() {
        panel = null
    }
}
