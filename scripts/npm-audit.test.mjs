import assert from "node:assert/strict"
import { test } from "node:test"
import { evaluateAudit } from "./npm-audit.mjs"

const advisory = "https://github.com/advisories/GHSA-fc5q-39xc-vjpf"
const beforeExpiry = Date.parse("2026-11-17T00:00:00Z")
const picomatch = {
  version: "4.0.7",
  resolved: "https://registry.npmjs.org/picomatch/-/picomatch-4.0.7.tgz",
  integrity: "sha512-qcJu88Q2IWqJsDD529JKMdwGm/dvInW4HvQnRwiH9JtihJvzGOscDtHE3x1pBKeUOTysQ8kVmLnJ2kJu7yhcGA==",
  dev: true,
}

const finding = (name, isDirect, range, via, effects) => ({ name, severity: "high", isDirect, via, effects, range, nodes: [`node_modules/${name}`], fixAvailable: true })
const leaf = (effects) => finding("picomatch", false, "*", [{
  source: 1247710, name: "picomatch", dependency: "picomatch", title: "picomatch vulnerable to ReDoS through single-branch repeated extglobs",
  url: advisory, severity: "high", cwe: ["CWE-1333"], range: "<=4.0.7",
}], effects)
const fdir = () => finding("fdir", false, ">=5.2.1", ["picomatch"], ["tinyglobby"])

const reports = {
  "ui/package-lock.json": () => [
    finding("@vitejs/plugin-react", true, ">=6.0.0-beta.0", ["vite"], []),
    fdir(),
    leaf(["fdir", "tinyglobby", "vite", "vitest"]),
    finding("tinyglobby", false, "*", ["fdir", "picomatch"], ["vite", "vitest"]),
    finding("vite", true, ">=6.3.0-beta.0", ["picomatch", "tinyglobby"], ["@vitejs/plugin-react", "vitest"]),
    finding("vitest", true, ">=3.1.2", ["picomatch", "tinyglobby", "vite"], []),
  ],
  "editors/vscode/package-lock.json": () => [
    finding("@vscode/vsce", true, ">=3.9.3-0", ["tinyglobby"], []),
    fdir(),
    leaf(["fdir", "tinyglobby"]),
    finding("tinyglobby", false, "*", ["fdir", "picomatch"], ["@vscode/vsce"]),
  ],
  "tests/vendor/opencode/package-lock.json": () => [
    finding("@npmcli/run-script", false, ">=10.0.3", ["node-gyp"], []),
    fdir(),
    finding("node-gyp", false, ">=11.2.0", ["tinyglobby"], ["@npmcli/run-script"]),
    leaf(["fdir", "tinyglobby"]),
    finding("tinyglobby", false, "*", ["fdir", "picomatch"], ["node-gyp"]),
  ],
}

function fixture(lockPath = "ui/package-lock.json") {
  const findings = reports[lockPath]()
  const lock = { packages: Object.fromEntries(findings.map(({ name }) => [`node_modules/${name}`, { version: "1.0.0", dev: true }])) }
  lock.packages["node_modules/picomatch"] = { ...picomatch }
  return {
    lockPath,
    lock,
    report: { auditReportVersion: 2, vulnerabilities: Object.fromEntries(findings.map((value) => [value.name, value])), metadata: { vulnerabilities: { total: findings.length } } },
  }
}

const result = (report, status = Object.keys(report.vulnerabilities).length ? 1 : 0) => ({ status, stdout: JSON.stringify(report) })
const evaluate = ({ report, lockPath, lock }, now = beforeExpiry) => evaluateAudit(result(report), lockPath, lock, now)

for (const lockPath of Object.keys(reports)) {
  test(`excepts the exact picomatch 4.0.7 development findings in ${lockPath}`, () => {
    const value = fixture(lockPath)
    const verdict = evaluate(value)
    assert.deepEqual(verdict.remaining, {})
    assert.deepEqual(verdict.ignored, Object.keys(value.report.vulnerabilities))
  })
}

test("the exception expires on December 17", () => {
  const value = fixture()
  const verdict = evaluate(value, Date.parse("2026-12-17T00:00:00Z"))
  assert.equal(Object.keys(verdict.remaining).length, 6)
  assert.deepEqual(verdict.ignored, [])
})

for (const [name, change] of [
  ["another picomatch version", ({ lock }) => { lock.packages["node_modules/picomatch"].version = "4.0.6" }],
  ["a changed integrity", ({ lock }) => { lock.packages["node_modules/picomatch"].integrity = "sha512-different" }],
  ["another registry source", ({ lock }) => { lock.packages["node_modules/picomatch"].resolved = "https://example.com/picomatch.tgz" }],
  ["a picomatch node outside development", ({ lock }) => { delete lock.packages["node_modules/picomatch"].dev }],
  ["a dependent outside development", ({ lock }) => { lock.packages["node_modules/vite"].dev = false }],
  ["a different advisory URL", ({ report }) => { report.vulnerabilities.picomatch.via[0].url = "https://github.com/advisories/GHSA-other" }],
  ["an additional advisory", ({ report }) => { report.vulnerabilities.picomatch.via.push({ url: "https://github.com/advisories/GHSA-other" }) }],
  ["another affected range", ({ report }) => { report.vulnerabilities.picomatch.via[0].range = "<5" }],
  ["higher severity", ({ report }) => { report.vulnerabilities.picomatch.via[0].severity = "critical" }],
  ["an additional picomatch path", ({ report }) => { report.vulnerabilities.picomatch.nodes.push("node_modules/other/node_modules/picomatch") }],
  ["a new dependent", ({ report }) => { report.vulnerabilities.picomatch.effects.push("other") }],
  ["another dependent cause", ({ report }) => { report.vulnerabilities.vite.via.push("other") }],
  ["another dependent range", ({ report }) => { report.vulnerabilities.vitest.range = "*" }],
]) {
  test(`rejects ${name}`, () => {
    const value = fixture()
    change(value)
    const verdict = evaluate(value)
    assert.equal(Object.keys(verdict.remaining).length, 6)
    assert.deepEqual(verdict.ignored, [])
  })
}

test("an extra finding keeps failing beside the excepted ones", () => {
  const value = fixture()
  value.report.vulnerabilities.other = { name: "other", via: [{ url: "https://github.com/advisories/GHSA-other" }] }
  value.report.metadata.vulnerabilities.total++
  assert.deepEqual(Object.keys(evaluate(value).remaining), ["other"])
})

test("a missing dependent rejects the exception", () => {
  const value = fixture()
  delete value.report.vulnerabilities.vitest
  value.report.metadata.vulnerabilities.total--
  assert.equal(Object.keys(evaluate(value).remaining).length, 5)
})

test("the same findings in another lockfile still fail", () => {
  const value = fixture()
  value.lockPath = "site/package-lock.json"
  assert.equal(Object.keys(evaluate(value).remaining).length, 6)
})

test("a clean report needs no exception", () => {
  const value = fixture()
  value.report.vulnerabilities = {}
  value.report.metadata.vulnerabilities.total = 0
  assert.deepEqual(evaluate(value, Date.parse("2027-01-01T00:00:00Z")), { remaining: {}, ignored: [] })
})

for (const [name, processResult] of [
  ["process error", { status: 1, error: new Error("network failure"), stdout: "{}" }],
  ["unsupported status", { status: 2, stdout: "{}" }],
  ["terminated process", { status: null, signal: "SIGTERM", stdout: "{}" }],
  ["invalid JSON", { status: 1, stdout: "not json" }],
  ["audit error", { status: 1, stdout: '{"error":{"code":"ENOAUDIT"}}' }],
  ["incomplete findings", { status: 1, stdout: '{"auditReportVersion":2,"vulnerabilities":{},"metadata":{"vulnerabilities":{"total":2}}}' }],
]) {
  test(`fails closed on ${name}`, () => {
    assert.throws(() => evaluateAudit(processResult, "ui/package-lock.json", fixture().lock, beforeExpiry))
  })
}

test("an inconsistent successful exit cannot hide findings", () => {
  const { report, lock } = fixture()
  assert.throws(() => evaluateAudit(result(report, 0), "ui/package-lock.json", lock, beforeExpiry))
})

test("a failing process with no findings is still an error", () => {
  const { report, lock } = fixture()
  report.vulnerabilities = {}
  report.metadata.vulnerabilities.total = 0
  assert.throws(() => evaluateAudit(result(report, 1), "ui/package-lock.json", lock, beforeExpiry))
})

test("a new audit report format requires review", () => {
  const { report, lock } = fixture()
  report.auditReportVersion = 3
  assert.throws(() => evaluateAudit(result(report), "ui/package-lock.json", lock, beforeExpiry))
})
