import assert from "node:assert/strict"
import { execFileSync } from "node:child_process"
import { existsSync, mkdtempSync, readFileSync, realpathSync, rmSync, statSync } from "node:fs"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { pathToFileURL } from "node:url"
import { $ } from "bun"
import type { Plugin } from "@opencode/plugin"
import type { Host } from "@opencode/plugin/host"
import type { Plugin as LegacyPlugin } from "@opencode-ai/plugin"

type Context = Plugin.Context
// Registrations and callback payloads use the released package's domain types.
type ToolEvent = Parameters<Parameters<Context["tool"]["hook"]>[1]>[0]
type BeforeEvent = Exclude<ToolEvent, { status: "completed" | "error" }>
type Handler = (event: ToolEvent) => Promise<void> | void
type SessionInfo = Awaited<ReturnType<Context["session"]["get"]>>

const output = process.argv[2]
const packageRoot = process.argv[3]
assert(output && packageRoot)
assert.equal(process.versions.bun, "1.3.10")
const nativeHost = await import(pathToFileURL(join(packageRoot, "node_modules/@opencode/plugin/dist/host.js")).href) as { Host: typeof Host }
const nativeTool = await import(pathToFileURL(join(packageRoot, "node_modules/@opencode/schema/dist/tool.js")).href) as typeof import("@opencode/schema/tool")
const root = mkdtempSync(join(tmpdir(), "agnostic-opencode-runtime-"))
const workspace = realpathSync(mkdtempSync(join(tmpdir(), "agnostic opencode checkout with spaces ")))
const warnings: unknown[][] = []
const originalError = console.error
console.error = (...values: unknown[]) => { warnings.push(values) }
const delay = (milliseconds: number) => new Promise(resolve => setTimeout(resolve, milliseconds))

function unavailable(): never { throw new Error("fixture accessed an unrelated native API") }

function registry(cwd: string) {
  const handlers = new Map<string, Handler>()
  let disposed = false
  let sessionCalls = 0
  const tool: Context["tool"] = {
    transform: unavailable, reload: unavailable, list: unavailable,
    hook: async (name, callback) => {
      handlers.set(name, event => callback(event as Parameters<typeof callback>[0]))
      return { dispose: async () => { handlers.delete(name); disposed = true } }
    },
  }
  // Only the accessed location is supplied; every unrelated session field fails.
  const get: Context["session"]["get"] = async () => { sessionCalls++; return new Proxy({ location: { directory: cwd } }, {
    get(target, field) { if (field === "then") return undefined; if (field === "location") return target.location; return unavailable() },
  }) as SessionInfo }
  const supplied = { tool, session: { get } } satisfies { tool: Context["tool"]; session: Pick<Context["session"], "get"> }
  const context = new Proxy(supplied, {
    get(target, field) { if (field === "tool") return target.tool; if (field === "session") return target.session; return unavailable() },
  }) as Context
  return { handlers, context, disposed: () => disposed, sessionCalls: () => sessionCalls }
}

function event(file: string, mode = "success"): BeforeEvent {
  return {
    tool: "shell", sessionID: "ses_fixture" as ToolEvent["sessionID"],
    agent: "agent_fixture" as ToolEvent["agent"], messageID: "msg_fixture" as ToolEvent["messageID"],
    id: "call_fixture" as ToolEvent["id"],
    input: { file, mode, heartbeat: file + ".heartbeat", pid: file + ".pid", data: "escaped NUL \0 remains event data" },
  }
}

async function plugin(name: string, body: (invoke: Handler, registered: ReturnType<typeof registry>) => Promise<void>, cwd = workspace) {
  const module: unknown = await nativeHost.Host.load(pathToFileURL(join(output!, name + ".js")).href)
  assert(module && typeof module === "object" && "default" in module)
  const value: unknown = module.default
  assert(value && typeof value === "object" && "id" in value && "setup" in value)
  assert.equal(value.id, "agnostic-ai." + name)
  assert.equal(typeof value.setup, "function")
  const definition = value as Plugin.Plugin
  const registered = registry(cwd)
  const cleanup = await definition.setup(registered.context)
  assert.equal(typeof cleanup, "function")
  const invoke = registered.handlers.values().next().value
  assert(invoke, "native setup must register a tool callback")
  try { await body(invoke, registered) }
  finally { if (cleanup) await cleanup() }
  assert(registered.disposed(), "unload must dispose the native registration")
  assert.equal(registered.handlers.size, 0)
}

function records(file: string): Record<string, unknown>[] {
  return readFileSync(file, "utf8").trim().split("\n").map(line => JSON.parse(line) as Record<string, unknown>)
}

async function ready(path: string) {
  const deadline = Date.now() + 5000
  while (!existsSync(path)) {
    assert(Date.now() < deadline, "child fixture did not become ready")
    await delay(20)
  }
}

function alive(pid: number) {
  try {
    process.kill(pid, 0)
    return !execFileSync("ps", ["-o", "stat=", "-p", String(pid)], { encoding: "utf8" }).trim().startsWith("Z")
  } catch { return false }
}

function forceFixtureDescendantCleanup(file: string) {
  if (!existsSync(file + ".pid")) return
  const pid = Number(readFileSync(file + ".pid", "utf8"))
  assert(Number.isSafeInteger(pid) && pid > 0, "fixture child PID must be positive")
  try { process.kill(pid, "SIGKILL") } catch {}
}

async function descendantStopped(file: string) {
  const pid = Number(readFileSync(file + ".pid", "utf8"))
  const deadline = Date.now() + 2500
  while (alive(pid)) { assert(Date.now() < deadline, `owned descendant ${pid} survived cleanup`); await delay(30) }
  if (existsSync(file + ".heartbeat")) {
    const modified = statSync(file + ".heartbeat").mtimeMs
    await delay(150)
    assert.equal(statSync(file + ".heartbeat").mtimeMs, modified)
  }
}


try {
  if (process.platform === "win32") {
    const warningStart = warnings.length
    await plugin("guard", async (invoke, registered) => {
      const file = join(root, "unsupported-windows.jsonl")
      await invoke({ ...event(file), tool: "shell-extra" })
      await assert.rejects(Promise.resolve(invoke(event(file))), /native V2 hooks are unsupported on Windows/)
      assert(!existsSync(file), "Windows refusal must not launch a command")
      assert.equal(registered.sessionCalls(), 0, "Windows refusal must precede session lookup")
    }, workspace + "\0invalid")
    for (const name of ["post", "after"]) {
      await plugin(name, async invoke => {
        const file = join(root, name + "-unsupported-windows.jsonl")
        await assert.rejects(Promise.resolve(invoke({ ...event(file), status: "completed", result: { content: [] } })), /native V2 hooks are unsupported on Windows/)
        assert(!existsSync(file), "Windows after callback must not launch a command")
      })
    }
    assert(warnings.slice(warningStart).some(values => values.some(value => String(value).includes("native V2 hooks are unsupported on Windows"))), "setup must explain unsupported Windows V2")
  } else {
    await plugin("guard", async invoke => {
      const file = join(root, "invalid-cwd.jsonl")
      await assert.rejects(Promise.resolve(invoke(event(file))), /working directory must not contain NUL/)
      assert(!existsSync(file), "invalid cwd must refuse before launching a guard")
    }, workspace + "\0ignored")
    await plugin("guard", async invoke => {
      const file = join(root, "success.jsonl")
      const input = event(file)
      await invoke({ ...input, tool: "shell-extra" })
      assert(!existsSync(file), "literal matcher must not match a longer tool name")
      await invoke(input)
      const values = records(file)
      assert.equal(values.length, 2, "commands must run sequentially once each")
      for (const value of values) {
        assert.equal(value.cwd, workspace, "cwd must follow the triggering session")
        assert.equal(value.raw, JSON.stringify(input) + "\n")
        assert.equal(value.target, "opencode")
        assert.equal(value.custom, "per-hook")
        assert.equal(value.inherited, "inherited")
        assert.deepEqual(value.argv, ["two words", "", "'quote\"", "a;b", "unicode-ñ", "space and trailing\\", "two\\\\"])
      }
      assert.equal(process.env.AGNOSTIC_AI_TARGET, undefined, "target env must stay invocation-local")
      assert.equal(process.env.HOOK_TEST_ENV, undefined, "hook env must stay invocation-local")
      const blocked = join(root, "blocked.jsonl")
      await assert.rejects(Promise.resolve(invoke(event(blocked, "block"))), /guard refused operation/)
      assert.equal(records(blocked).length, 1, "block must stop later commands")
      const failure = join(root, "failure.jsonl")
      await invoke(event(failure, "failure"))
      assert.equal(records(failure).length, 2, "ordinary command errors retain sequential behavior")
    })
    for (const name of ["post", "after"]) {
      await plugin(name, async invoke => {
        const errorFile = join(root, name + "-error.jsonl")
        const failed: ToolEvent = { ...event(errorFile), status: "error", error: new nativeTool.Tool.Error({ message: "tool failed" }) }
        await invoke(failed)
        assert.equal(existsSync(errorFile), name === "after", "PostToolUse filters errors; native after receives them")
        const completeFile = join(root, name + "-completed.jsonl")
        await invoke({ ...event(completeFile), status: "completed", result: { content: [] } })
        assert.equal(records(completeFile).length, 1)
      })
    }
    await plugin("timeout", async invoke => {
      const file = join(root, "timeout.jsonl")
      await invoke(event(file, "tree"))
      await descendantStopped(file)
      assert(warnings.some(values => values.some(value => String(value).includes("timed out after 1 seconds"))))
    })
    await plugin("timeout", async invoke => {
      const file = join(root, "block-held-stderr.jsonl")
      try {
        await assert.rejects(Promise.resolve(invoke(event(file, "block-orphan"))), /guard refused operation with held stderr/)
        await descendantStopped(file)
      } finally { forceFixtureDescendantCleanup(file) }
    })
    const unloadFile = join(root, "unload.jsonl")
    let pending: Promise<void> | undefined
    await plugin("guard", async invoke => {
      pending = Promise.resolve(invoke(event(unloadFile, "tree")))
      void pending.catch(() => {})
      await ready(unloadFile + ".pid")
    })
    assert(pending)
    await assert.rejects(pending, /plugin unloaded/)
    await descendantStopped(unloadFile)
    await plugin("spawn-failure", async invoke => {
      const invocation = Promise.resolve(invoke(event(join(root, "spawn.jsonl"))))
      await invocation
    })
    assert(warnings.some(values => values.some(value => String(value).includes("ENOENT"))))
    await plugin("parse-failure", async invoke => { await assert.rejects(Promise.resolve(invoke(event(join(root, "parse.jsonl"))))) })
    await plugin("shell", async invoke => { await invoke(event(join(root, "shell.jsonl"))) })
    await assert.rejects(plugin("bad-matcher", async () => {}), /regular expression|unterminated|invalid|character class/i)
  }
  const legacy: unknown = await nativeHost.Host.load(pathToFileURL(join(output, "v1.js")).href)
  assert(legacy && typeof legacy === "object" && "V1Plugin" in legacy && typeof legacy.V1Plugin === "function")
  const nativeSDK = await import(pathToFileURL(join(packageRoot, "node_modules/@opencode-ai/sdk/dist/client.js")).href) as typeof import("@opencode-ai/sdk/client")
  const legacyInput: Parameters<LegacyPlugin>[0] = {
    client: nativeSDK.createOpencodeClient({ baseUrl: "http://127.0.0.1:1", fetch: unavailable }),
    project: { id: "fixture-project", worktree: workspace, time: { created: 0 } },
    directory: workspace,
    worktree: workspace,
    serverUrl: new URL("http://127.0.0.1:1"),
    $,
  }
  const callbacks = await (legacy.V1Plugin as LegacyPlugin)(legacyInput)
  const before = callbacks["tool.execute.before"]
  assert(before)
  await assert.rejects(before({ tool: "bash", sessionID: "ses_fixture", callID: "call_fixture" }, { args: {} }), /blocked by hook v1/)
  console.log("Native hook behavior assertions passed")
} finally {
  console.error = originalError
  rmSync(root, { recursive: true, force: true })
  rmSync(workspace, { recursive: true, force: true })
}
