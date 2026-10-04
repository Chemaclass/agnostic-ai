// Pure JVM tests for the config file lookup, in the CLI's order.

package com.agnosticai.intellij

import java.nio.file.Files
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class AgnosticAiConfigFileTest {
    @Test fun findsCurrentAndLegacyNamesPreferringCurrent() {
        val dir = Files.createTempDirectory("aai")
        assertNull(AgnosticAi.configFile(dir))

        Files.writeString(dir.resolve("agnostic.config.yaml"), "targets: [claude]\n")
        assertEquals(dir.resolve("agnostic.config.yaml"), AgnosticAi.configFile(dir))

        Files.writeString(dir.resolve("agnostic-ai.yaml"), "targets:\n  - codex\n")
        assertEquals(dir.resolve("agnostic-ai.yaml"), AgnosticAi.configFile(dir))
        assertEquals(listOf("codex"), AgnosticAi.configuredTargets(dir))
    }
}
