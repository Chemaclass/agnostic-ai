// Contributes the published config.schema.json to IntelliJ's JSON
// schema service so agnostic-ai.yaml, and the legacy agnostic.config.yaml,
// get validation, completion, and hover docs out of the box. Same schema
// URL the YAML Language Server hint in init-generated configs points at;
// keeping it in sync is the publishing pipeline's job, not this plugin's.

package com.agnosticai.intellij.schema

import com.agnosticai.intellij.AgnosticAi

import com.intellij.openapi.project.Project
import com.intellij.openapi.vfs.VirtualFile
import com.jetbrains.jsonSchema.extension.JsonSchemaFileProvider
import com.jetbrains.jsonSchema.extension.JsonSchemaProviderFactory
import com.jetbrains.jsonSchema.extension.SchemaType

class AgnosticConfigSchemaProvider : JsonSchemaProviderFactory {
    override fun getProviders(project: Project): List<JsonSchemaFileProvider> =
        listOf(AgnosticConfigFileProvider())
}

private class AgnosticConfigFileProvider : JsonSchemaFileProvider {
    override fun isAvailable(file: VirtualFile): Boolean =
        AgnosticAi.isConfigFileName(file.name)

    override fun getName(): String = "agnostic-ai.yaml"

    override fun getSchemaFile(): VirtualFile? = null

    override fun getSchemaType(): SchemaType = SchemaType.remoteSchema

    override fun getRemoteSource(): String =
        "https://raw.githubusercontent.com/Chemaclass/agnostic-ai/main/docs/schemas/config.schema.json"
}
