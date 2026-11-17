import { spawnSync } from "node:child_process"
import { readFileSync } from "node:fs"
import { dirname, resolve } from "node:path"
import { fileURLToPath, pathToFileURL } from "node:url"

const exception = {
  advisory: "https://github.com/advisories/GHSA-fc5q-39xc-vjpf",
  expires: "2026-12-17T00:00:00Z",
  leaf: {
    name: "picomatch",
    version: "4.0.7",
    resolved: "https://registry.npmjs.org/picomatch/-/picomatch-4.0.7.tgz",
    integrity: "sha512-qcJu88Q2IWqJsDD529JKMdwGm/dvInW4HvQnRwiH9JtihJvzGOscDtHE3x1pBKeUOTysQ8kVmLnJ2kJu7yhcGA==",
    range: "<=4.0.7",
  },
  locks: {
    "ui/package-lock.json": {
      "@vitejs/plugin-react": [true, ">=6.0.0-beta.0", ["vite"], []],
      fdir: [false, ">=5.2.1", ["picomatch"], ["tinyglobby"]],
      picomatch: [false, "*", [], ["fdir", "tinyglobby", "vite", "vitest"]],
      tinyglobby: [false, "*", ["fdir", "picomatch"], ["vite", "vitest"]],
      vite: [true, ">=6.3.0-beta.0", ["picomatch", "tinyglobby"], ["@vitejs/plugin-react", "vitest"]],
      vitest: [true, ">=3.1.2", ["picomatch", "tinyglobby", "vite"], []],
    },
    "editors/vscode/package-lock.json": {
      "@vscode/vsce": [true, ">=3.9.3-0", ["tinyglobby"], []],
      fdir: [false, ">=5.2.1", ["picomatch"], ["tinyglobby"]],
      picomatch: [false, "*", [], ["fdir", "tinyglobby"]],
      tinyglobby: [false, "*", ["fdir", "picomatch"], ["@vscode/vsce"]],
    },
    "tests/vendor/opencode/package-lock.json": {
      "@npmcli/run-script": [false, ">=10.0.3", ["node-gyp"], []],
      fdir: [false, ">=5.2.1", ["picomatch"], ["tinyglobby"]],
      "node-gyp": [false, ">=11.2.0", ["tinyglobby"], ["@npmcli/run-script"]],
      picomatch: [false, "*", [], ["fdir", "tinyglobby"]],
      tinyglobby: [false, "*", ["fdir", "picomatch"], ["node-gyp"]],
    },
  },
}
const sameList = (values, expected) => Array.isArray(values) && values.length === expected.length && values.every((value, index) => value === expected[index])

function matchesAdvisoryCause(via) {
  const { name, range } = exception.leaf
  if (!Array.isArray(via) || via.length !== 1) return false
  const cause = via[0]
  return cause?.name === name && cause.dependency === name && cause.url === exception.advisory && cause.severity === "high" && cause.range === range
}

function exceptedFindings(findings, lock, lockPath, now) {
  const expected = exception.locks[lockPath]
  if (!expected || !Number.isFinite(now) || now >= Date.parse(exception.expires)) return []
  const { name, version, resolved, integrity } = exception.leaf
  const leaf = lock.packages?.[`node_modules/${name}`]
  if (leaf?.version !== version || leaf.resolved !== resolved || leaf.integrity !== integrity) return []
  const names = Object.keys(expected)
  const matches = names.every((key) => {
    const [isDirect, range, via, effects] = expected[key]
    const finding = findings[key]
    const path = `node_modules/${key}`
    return finding?.name === key && finding.severity === "high" && finding.isDirect === isDirect && finding.range === range &&
      sameList(finding.nodes, [path]) && lock.packages?.[path]?.dev === true && sameList(finding.effects, effects) &&
      (key === name ? matchesAdvisoryCause(finding.via) : sameList(finding.via, via))
  })
  return matches ? names : []
}

export function evaluateAudit(result, lockPath, lock, now = Date.now()) {
  if (result.error || result.signal || result.status !== 0 && result.status !== 1) throw new Error("npm audit did not finish with a supported exit status")
  let report
  try { report = JSON.parse(result.stdout) }
  catch { throw new Error("npm audit returned invalid JSON") }
  const findings = report?.vulnerabilities
  if (report?.error || report?.auditReportVersion !== 2 || !findings || typeof findings !== "object" || Array.isArray(findings) || report.metadata?.vulnerabilities?.total !== Object.keys(findings).length) {
    throw new Error("npm audit returned an incomplete or unsupported report")
  }
  if ((result.status === 0) !== (Object.keys(findings).length === 0)) throw new Error("npm audit exit status disagrees with its findings")
  const remaining = { ...findings }
  const ignored = exceptedFindings(findings, lock, lockPath, now)
  for (const name of ignored) delete remaining[name]
  return { remaining, ignored }
}

function runAudits() {
  const root = resolve(dirname(fileURLToPath(import.meta.url)), "..")
  const tracked = spawnSync("git", ["ls-files", "-z", "--", "**/package-lock.json", "package-lock.json"], { cwd: root, encoding: "utf8" })
  if (tracked.error || tracked.status !== 0) throw new Error("cannot list tracked npm lockfiles")
  const locks = tracked.stdout.split("\0").filter(Boolean)
  if (locks.length === 0) throw new Error("no tracked npm lockfiles found")
  let failed = false
  for (const lockPath of locks) {
    console.log(`::group::${dirname(lockPath)}`)
    try {
      const lock = JSON.parse(readFileSync(resolve(root, lockPath), "utf8"))
      const result = spawnSync("npm", ["audit", "--prefix", dirname(lockPath), "--package-lock-only", "--ignore-scripts", "--include=dev", "--include=optional", "--audit-level=low", "--json"], {
        cwd: root, encoding: "utf8", timeout: 60000, maxBuffer: 8 * 1024 * 1024,
      })
      if (result.stderr) process.stderr.write(result.stderr)
      const verdict = evaluateAudit(result, lockPath, lock)
      if (verdict.ignored.length > 0) console.log(`Scoped exception: ${exception.leaf.name} ${exception.leaf.version} in development tools (${verdict.ignored.join(", ")}), ${exception.advisory}, expires ${exception.expires}`)
      if (Object.keys(verdict.remaining).length > 0) {
        failed = true
        console.error(JSON.stringify(verdict.remaining, null, 2))
      } else {
        console.log(`No unexcepted vulnerabilities (${verdict.ignored.length} exact development notices excepted)`)
      }
    } catch (error) {
      failed = true
      console.error(`${lockPath}: ${error.message}`)
    } finally {
      console.log("::endgroup::")
    }
  }
  return failed ? 1 : 0
}

if (process.argv[1] && import.meta.url === pathToFileURL(resolve(process.argv[1])).href) {
  try { process.exitCode = runAudits() }
  catch (error) { console.error(error.message); process.exitCode = 1 }
}
