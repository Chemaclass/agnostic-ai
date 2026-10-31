import { spawnSync } from "node:child_process"
import { readFileSync } from "node:fs"
import { dirname, resolve } from "node:path"
import { fileURLToPath, pathToFileURL } from "node:url"

const advisory = "https://github.com/advisories/GHSA-g3hv-75wg-m249"
const expires = "2026-11-14T00:00:00Z"
const parentPath = "node_modules/vscode-languageclient"
const leafPath = `${parentPath}/node_modules/minimatch`
const integrity = "sha512-7o1wEA2RyMP7Iu7GNba9vc0RWWGACJOCZBJX2GJWip0ikV+wcOsgVuY9uE8CPiyQhkGFSlhuSkZPavN7u1c2Fw=="
const only = (values, expected) => Array.isArray(values) && values.length === 1 && values[0] === expected

function knownLegacyFinding(report, lock, lockPath, now) {
  if (lockPath !== "editors/vscode/package-lock.json" || !Number.isFinite(now) || now >= Date.parse(expires)) return false
  const leaf = report.vulnerabilities.minimatch
  const parent = report.vulnerabilities["vscode-languageclient"]
  const native = lock.packages?.[leafPath]
  const client = lock.packages?.[parentPath]
  if (native?.version !== "5.1.9" || native.integrity !== integrity || native.resolved !== "https://registry.npmjs.org/minimatch/-/minimatch-5.1.9.tgz") return false
  if (client?.version !== "9.0.1" || client.dependencies?.minimatch !== "^5.1.0") return false
  if (leaf?.name !== "minimatch" || leaf.severity !== "moderate" || leaf.isDirect !== false || leaf.range !== "<11.0.1") return false
  if (!only(leaf.nodes, leafPath) || !only(leaf.effects, "vscode-languageclient") || !Array.isArray(leaf.via) || leaf.via.length !== 1) return false
  const cause = leaf.via[0]
  if (cause?.name !== "minimatch" || cause.dependency !== "minimatch" || cause.url !== advisory || cause.range !== "<11.0.1" || cause.severity !== "moderate") return false
  return parent?.name === "vscode-languageclient" && parent.severity === "moderate" && parent.isDirect === true &&
    only(parent.nodes, parentPath) && only(parent.via, "minimatch") && Array.isArray(parent.effects) && parent.effects.length === 0
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
  const ignored = []
  if (knownLegacyFinding(report, lock, lockPath, now)) {
    for (const name of ["minimatch", "vscode-languageclient"]) {
      delete remaining[name]
      ignored.push(name)
    }
  }
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
      if (verdict.ignored.length > 0) console.log(`Scoped exception: minimatch 5.1.9 under vscode-languageclient 9.0.1, ${advisory}, expires ${expires}`)
      if (Object.keys(verdict.remaining).length > 0) {
        failed = true
        console.error(JSON.stringify(verdict.remaining, null, 2))
      } else {
        console.log(`No unexcepted vulnerabilities (${verdict.ignored.length} exact legacy notices excepted)`)
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
