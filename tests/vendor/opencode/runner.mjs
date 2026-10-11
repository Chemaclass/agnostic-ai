import assert from "node:assert/strict"
import { spawnSync } from "node:child_process"
import { mkdtempSync, readdirSync, readFileSync, rmSync, writeFileSync } from "node:fs"
import { tmpdir } from "node:os"
import { basename, dirname, join, resolve } from "node:path"
import { fileURLToPath } from "node:url"
import ts from "typescript"

const here = dirname(fileURLToPath(import.meta.url))
const fixtures = process.env.AGNOSTIC_AI_OPENCODE_FIXTURE_DIR
assert(fixtures, "AGNOSTIC_AI_OPENCODE_FIXTURE_DIR must name the rendered fixture directory")
writeFileSync(join(fixtures, "package.json"), '{"type":"module"}\n')
const names = readdirSync(fixtures).filter(name => name.endsWith(".ts"))
assert.equal(names.length, 9, "all native fixtures must be present")
const output = mkdtempSync(join(tmpdir(), "agnostic-opencode-typed-"))
try {
  writeFileSync(join(output, "package.json"), '{"type":"module"}\n')
  const files = [join(here, "runtime.ts"), ...names.map(name => join(fixtures, name))]
  const program = ts.createProgram(files, {
    strict: true, noUncheckedIndexedAccess: true, target: ts.ScriptTarget.ES2022,
    module: ts.ModuleKind.NodeNext, moduleResolution: ts.ModuleResolutionKind.NodeNext,
    skipLibCheck: true, typeRoots: [join(here, "node_modules/@types")],
    paths: {
      "@opencode/plugin": [join(here, "node_modules/@opencode/plugin/dist/promise/index.d.ts")],
      "@opencode/plugin/host": [join(here, "node_modules/@opencode/plugin/dist/host.d.ts")],
      "@opencode-ai/plugin": [join(here, "node_modules/@opencode-ai/plugin/dist/index.d.ts")],
    },
  })
  const diagnostics = ts.getPreEmitDiagnostics(program)
  assert.equal(diagnostics.length, 0, ts.formatDiagnosticsWithColorAndContext(diagnostics, {
    getCanonicalFileName: name => name, getCurrentDirectory: () => here, getNewLine: () => "\n",
  }))
  const emitted = program.emit(undefined, (filename, text, _bom, _error, sources) => {
    if (!filename.endsWith(".js")) return
    writeFileSync(join(output, basename(sources?.[0]?.fileName ?? filename).replace(/\.ts$/, ".js")), text)
  })
  assert.equal(emitted.emitSkipped, false)
  const architecture = process.arch === "arm64" ? "aarch64" : process.arch
  const platform = process.platform === "win32" ? "windows" : process.platform
  const bun = join(here, "node_modules/@oven", `bun-${platform}-${architecture}`, "bin", process.platform === "win32" ? "bun.exe" : "bun")
  const result = spawnSync(bun, [join(output, "runtime.js"), output, here], {
    stdio: "inherit", timeout: 60000, env: { ...process.env, HOOK_INHERITED_ENV: "inherited" },
  })
  assert.ifError(result.error)
  assert.equal(result.status, 0, "native plugin behavior failed")
  console.log("OpenCode hooks: released types, public loader, V1 Bun control and V2 native child behavior passed")
} finally {
  assert(resolve(output).startsWith(resolve(tmpdir()) + "/") || process.platform === "win32")
  rmSync(output, { recursive: true, force: true })
}
