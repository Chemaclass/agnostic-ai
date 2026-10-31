import assert from "node:assert/strict"
import { mkdirSync, readFileSync, writeFileSync } from "node:fs"
import { createRequire } from "node:module"
import { dirname, join } from "node:path"
import { fileURLToPath } from "node:url"
import { Glob } from "@opencode/util/glob"

export async function assertGlobDependencies(output) {
  const root = join(output, "glob-dependencies")
  mkdirSync(join(root, "nested"), { recursive: true })
  for (const file of [".hidden.yaml", "config.yaml", "nested/rule.md", "nested/skip.txt"]) {
    writeFileSync(join(root, file), file)
  }
  const pattern = "**/*.{md,yaml}"
  const expected = [".hidden.yaml", "config.yaml", "nested/rule.md"]
  const normalized = files => files.map(file => file.replaceAll("\\", "/")).sort()
  assert.deepEqual(normalized(await Glob.scan(pattern, { cwd: root, dot: true })), expected)
  assert.deepEqual(normalized(Glob.scanSync(pattern, { cwd: root, dot: true })), expected)
  assert.equal(Glob.match(pattern, ".hidden.yaml"), true)
  assert.equal(Glob.match(pattern, "nested/rule.md"), true)
  assert.equal(Glob.match(pattern, "nested/skip.txt"), false)

  const here = dirname(fileURLToPath(import.meta.url))
  const packages = JSON.parse(readFileSync(join(here, "package-lock.json"), "utf8")).packages
  const parents = Object.keys(packages).filter(name => name.endsWith("/rimraf"))
  assert(parents.length > 0, "the fixture must exercise the rimraf glob dependency")
  for (const parent of parents) {
    const load = createRequire(join(here, parent, "package.json"))
    const { glob, globSync } = load("glob")
    assert.deepEqual(normalized(await glob(pattern, { cwd: root, dot: true, nodir: true })), expected)
    assert.deepEqual(normalized(globSync(pattern, { cwd: root, dot: true, nodir: true })), expected)
  }
  console.log("OpenCode matching: public util scans/matches and rimraf CommonJS glob APIs passed")
}
