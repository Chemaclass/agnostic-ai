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

    @Test fun projectRootPicksChildrenInNameOrder() {
        Files.writeString(Files.createDirectory(dir.resolve("b")).resolve("agnostic-ai.yaml"), "targets: []\n")
        val a = Files.createDirectory(dir.resolve("a"))
        Files.writeString(a.resolve("agnostic.config.yaml"), "targets: []\n")
        assertEquals(a, AgnosticAi.projectRoot(dir))
    }

    @Test fun projectRootPrefersARootConfigOverAChild() {
        Files.writeString(Files.createDirectory(dir.resolve("module")).resolve("agnostic-ai.yaml"), "targets: []\n")
        Files.writeString(dir.resolve("agnostic.config.yaml"), "targets: []\n")
        assertEquals(dir, AgnosticAi.projectRoot(dir))
    }

    @Test fun localOverrideTargetsReplaceTheBaseList() {
        Files.writeString(dir.resolve("agnostic-ai.yaml"), "targets:\n  - claude\n  - codex\n")
        assertEquals(listOf("claude", "codex"), AgnosticAi.configuredTargets(dir))

        Files.writeString(dir.resolve("agnostic-ai.local.yaml"), "sources:\n  - specs\n")
        assertEquals(listOf("claude", "codex"), AgnosticAi.configuredTargets(dir))

        Files.writeString(dir.resolve("agnostic-ai.local.yaml"), "targets: [claude] # mine\n")
        assertEquals(listOf("claude"), AgnosticAi.configuredTargets(dir))

        Files.writeString(dir.resolve("agnostic-ai.local.yaml"), "targets:\n  - 'cursor'\n")
        assertEquals(listOf("cursor"), AgnosticAi.configuredTargets(dir))

        Files.writeString(dir.resolve("agnostic-ai.local.yaml"), "targets: []\n")
        assertEquals(emptyList<String>(), AgnosticAi.configuredTargets(dir))

        Files.writeString(dir.resolve("agnostic-ai.local.yaml"), "targets: [claude,\n")
        assertEquals(listOf("claude", "codex"), AgnosticAi.configuredTargets(dir))
    }

    @Test fun parseTargetListReadsTheShapesYamlAllows() {
        assertEquals(listOf("cursor"), AgnosticAi.parseTargetList("targets: # mine\r\n  - cursor\r\n"))
        assertEquals(listOf("claude", "codex"), AgnosticAi.parseTargetList("targets:\n  - claude\n# note\n  - codex\nsources: []\n"))
        assertEquals(listOf("claude", "codex"), AgnosticAi.parseTargetList("targets:\n- claude\n- codex\n"))
        assertEquals(listOf("claude", "cursor"), AgnosticAi.parseTargetList("targets: [\n  claude, # first\n  cursor\n]\n"))
        assertEquals(emptyList<String>(), AgnosticAi.parseTargetList("targets:\nsources: []\n"))
        assertNull(AgnosticAi.parseTargetList("targets:\n  nested: x\n"))
        assertNull(AgnosticAi.parseTargetList("version: 1\n"))
    }

    @Test fun schemaAttachesToBothNamesOnly() {
        assertTrue(AgnosticAi.isConfigFileName("agnostic-ai.yaml"))
        assertTrue(AgnosticAi.isConfigFileName("agnostic.config.yaml"))
        assertFalse(AgnosticAi.isConfigFileName("agnostic.config.yml"))
    }
}
