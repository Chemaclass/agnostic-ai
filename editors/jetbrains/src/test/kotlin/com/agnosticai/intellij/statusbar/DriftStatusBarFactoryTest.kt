// Pure JVM tests for the JSON-counting helper. Avoids spinning up the
// IntelliJ test framework so they run fast and don't need an IDE
// download. The helper is the only piece that needs hardening — the
// rest is glue against IDE APIs that live tests would simply restate.

package com.agnosticai.intellij.statusbar

import org.junit.Assert.assertEquals
import org.junit.Test

class DriftStatusBarFactoryTest {
    @Test fun emptyWritesArrayCountsZero() {
        val json = """{"version":"1","command":"sync --check","writes":[],"skipped":[],"errors":[]}"""
        assertEquals(0, parseWritesCount(json))
    }

    @Test fun singleWriteCountsOne() {
        val json = """
            {"version":"1","command":"sync --check",
             "writes":[{"target":"claude","path":"CLAUDE.md","action":"missing","bytes":120}],
             "skipped":[],"errors":[]}
        """.trimIndent()
        assertEquals(1, parseWritesCount(json))
    }

    @Test fun threeWritesCountsThree() {
        val json = """
            {"writes":[
              {"target":"claude","path":"CLAUDE.md","action":"missing","bytes":1},
              {"target":"codex","path":"AGENTS.md","action":"stale","bytes":2},
              {"target":"cursor","path":".cursor/rules/x.mdc","action":"missing","bytes":3}
            ]}
        """.trimIndent()
        assertEquals(3, parseWritesCount(json))
    }

    @Test fun missingWritesKeyReturnsNegativeOne() {
        val json = """{"version":"1","command":"sync"}"""
        assertEquals(-1, parseWritesCount(json))
    }

    @Test fun ignoresWritesValueInOtherKey() {
        // "writes" appearing as a value inside another object should not
        // confuse the matcher. Practically this never happens in the
        // schema, but the parser shouldn't be brittle either.
        val json = """{"comment":"writes count","writes":[{"a":1}]}"""
        assertEquals(1, parseWritesCount(json))
    }
}
