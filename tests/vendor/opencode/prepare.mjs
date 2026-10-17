import assert from "node:assert/strict"
import { spawnSync } from "node:child_process"
import { mkdtempSync, rmSync } from "node:fs"
import { tmpdir } from "node:os"
import { dirname, join, resolve } from "node:path"
import { fileURLToPath } from "node:url"

const here = dirname(fileURLToPath(import.meta.url))
const root = resolve(here, "../../..")
const fixtures = mkdtempSync(join(tmpdir(), "agnostic-opencode-rendered-"))
try {
  const env = { ...process.env, AGNOSTIC_AI_OPENCODE_FIXTURE_DIR: fixtures, AGNOSTIC_AI_OPENCODE_NODE: process.execPath }
  for (const [command, args] of [
    ["go", ["test", "./internal/adapters/opencode", "-run", "^TestEmit_HookV2RuntimeFixtures$", "-count=1"]],
    [process.execPath, [join(here, "runner.mjs")]],
  ]) {
    const result = spawnSync(command, args, { cwd: root, env, stdio: "inherit", timeout: 90000 })
    assert.ifError(result.error)
    assert.equal(result.status, 0, `${command} fixture validation failed`)
  }
} finally {
  rmSync(fixtures, { recursive: true, force: true })
}
