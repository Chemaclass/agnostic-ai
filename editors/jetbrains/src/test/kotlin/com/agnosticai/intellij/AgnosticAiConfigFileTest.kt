// Pure JVM tests for the config file lookup, in the CLI's order.

package com.agnosticai.intellij

import java.nio.file.Files
import java.nio.file.Path
import kotlin.io.path.ExperimentalPathApi
import kotlin.io.path.deleteRecursively
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class AgnosticAiConfigFileTest {
    private val dir: Path = Files.createTempDirectory("aai")

    @OptIn(ExperimentalPathApi::class)
    @After fun cleanUp() = dir.deleteRecursively()

    @Test fun findsCurrentAndLegacyNamesPreferringCurrent() {
        assertNull(AgnosticAi.configFile(dir))

        Files.writeString(dir.resolve("agnostic.config.yaml"), "targets:\n  - claude\n")
        assertEquals(dir.resolve("agnostic.config.yaml"), AgnosticAi.configFile(dir))
        assertEquals(listOf("claude"), AgnosticAi.configuredTargets(dir))

        Files.writeString(dir.resolve("agnostic-ai.yaml"), "targets:\n  - codex\n")
        assertEquals(dir.resolve("agnostic-ai.yaml"), AgnosticAi.configFile(dir))
        assertEquals(listOf("codex"), AgnosticAi.configuredTargets(dir))
    }

    @Test fun projectRootFindsAChildHoldingOnlyTheNewName() {
        val module = Files.createDirectory(dir.resolve("module"))
        Files.writeString(module.resolve("agnostic-ai.yaml"), "targets: []\n")
        assertEquals(module, AgnosticAi.projectRoot(dir))
    }

    @Test fun schemaAttachesToBothNamesOnly() {
        assertTrue(AgnosticAi.isConfigFileName("agnostic-ai.yaml"))
        assertTrue(AgnosticAi.isConfigFileName("agnostic.config.yaml"))
        assertFalse(AgnosticAi.isConfigFileName("agnostic.config.yml"))
    }
}
