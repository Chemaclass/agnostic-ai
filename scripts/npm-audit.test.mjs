import assert from "node:assert/strict"
import { test } from "node:test"
import { evaluateAudit } from "./npm-audit.mjs"

const leafPath = "node_modules/vscode-languageclient/node_modules/minimatch"
const parentPath = "node_modules/vscode-languageclient"
const beforeExpiry = Date.parse("2026-10-31T00:00:00Z")

function fixture() {
  return {
    report: {
      auditReportVersion: 2,
      vulnerabilities: {
        minimatch: { name: "minimatch", severity: "moderate", isDirect: false, range: "<11.0.1", nodes: [leafPath], effects: ["vscode-languageclient"], via: [
          { name: "minimatch", dependency: "minimatch", url: "https://github.com/advisories/GHSA-g3hv-75wg-m249", range: "<11.0.1", severity: "moderate" },
        ] },
        "vscode-languageclient": { name: "vscode-languageclient", severity: "moderate", isDirect: true, nodes: [parentPath], effects: [], via: ["minimatch"] },
      },
      metadata: { vulnerabilities: { total: 2 } },
    },
    lock: { packages: {
      [leafPath]: { version: "5.1.9", resolved: "https://registry.npmjs.org/minimatch/-/minimatch-5.1.9.tgz", integrity: "sha512-7o1wEA2RyMP7Iu7GNba9vc0RWWGACJOCZBJX2GJWip0ikV+wcOsgVuY9uE8CPiyQhkGFSlhuSkZPavN7u1c2Fw==" },
      [parentPath]: { version: "9.0.1", dependencies: { minimatch: "^5.1.0" } },
    } },
  }
}

const result = (report, status = Object.keys(report.vulnerabilities).length ? 1 : 0) => ({ status, stdout: JSON.stringify(report) })

test("only the exact reviewed legacy leaf and its sole parent cause are excepted", () => {
  const { report, lock } = fixture()
  const verdict = evaluateAudit(result(report), "editors/vscode/package-lock.json", lock, beforeExpiry)
  assert.deepEqual(verdict.remaining, {})
  assert.deepEqual(verdict.ignored, ["minimatch", "vscode-languageclient"])
})

for (const [name, change] of [
  ["another leaf version", ({ lock }) => { lock.packages[leafPath].version = "10.2.6" }],
  ["different tarball bytes", ({ lock }) => { lock.packages[leafPath].integrity = "sha512-different" }],
  ["different registry source", ({ lock }) => { lock.packages[leafPath].resolved = "https://example.com/minimatch.tgz" }],
  ["another client version", ({ lock }) => { lock.packages[parentPath].version = "10.1.2" }],
  ["another dependency range", ({ lock }) => { lock.packages[parentPath].dependencies.minimatch = "^10" }],
  ["another advisory", ({ report }) => { report.vulnerabilities.minimatch.via[0].url = "https://github.com/advisories/GHSA-other" }],
  ["additional advisory", ({ report }) => { report.vulnerabilities.minimatch.via.push({ url: "https://github.com/advisories/GHSA-other" }) }],
  ["another affected range", ({ report }) => { report.vulnerabilities.minimatch.via[0].range = "<12" }],
  ["higher severity", ({ report }) => { report.vulnerabilities.minimatch.via[0].severity = "high" }],
  ["additional affected path", ({ report }) => { report.vulnerabilities.minimatch.nodes.push("node_modules/other/node_modules/minimatch") }],
  ["another parent cause", ({ report }) => { report.vulnerabilities["vscode-languageclient"].via.push("other") }],
  ["another parent path", ({ report }) => { report.vulnerabilities["vscode-languageclient"].nodes = ["node_modules/other/node_modules/vscode-languageclient"] }],
  ["another downstream package", ({ report }) => { report.vulnerabilities.minimatch.effects.push("other") }],
]) {
  test(`rejects ${name}`, () => {
    const value = fixture()
    change(value)
    const verdict = evaluateAudit(result(value.report), "editors/vscode/package-lock.json", value.lock, beforeExpiry)
    assert.equal(Object.keys(verdict.remaining).length, 2)
    assert.deepEqual(verdict.ignored, [])
  })
}

test("the same advisory and versions in another lockfile still fail", () => {
  const { report, lock } = fixture()
  assert.equal(Object.keys(evaluateAudit(result(report), "tests/vendor/opencode/package-lock.json", lock, beforeExpiry).remaining).length, 2)
})

test("unrelated findings remain failures beside the excepted leaf", () => {
  const { report, lock } = fixture()
  report.vulnerabilities.other = { name: "other", via: [{ url: "https://github.com/advisories/GHSA-other" }] }
  report.metadata.vulnerabilities.total++
  assert.deepEqual(Object.keys(evaluateAudit(result(report), "editors/vscode/package-lock.json", lock, beforeExpiry).remaining), ["other"])
})

test("the exception expires and a clean report needs no exception", () => {
  const { report, lock } = fixture()
  const expiry = Date.parse("2026-11-14T00:00:00Z")
  assert.equal(Object.keys(evaluateAudit(result(report), "editors/vscode/package-lock.json", lock, expiry).remaining).length, 2)
  report.vulnerabilities = {}
  report.metadata.vulnerabilities.total = 0
  assert.deepEqual(evaluateAudit(result(report), "editors/vscode/package-lock.json", lock, expiry), { remaining: {}, ignored: [] })
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
    assert.throws(() => evaluateAudit(processResult, "editors/vscode/package-lock.json", fixture().lock, beforeExpiry))
  })
}

test("an inconsistent successful exit cannot hide findings", () => {
  const { report, lock } = fixture()
  assert.throws(() => evaluateAudit(result(report, 0), "editors/vscode/package-lock.json", lock, beforeExpiry))
})

test("a failing process with no findings is still an error", () => {
  const { report, lock } = fixture()
  report.vulnerabilities = {}
  report.metadata.vulnerabilities.total = 0
  assert.throws(() => evaluateAudit(result(report, 1), "editors/vscode/package-lock.json", lock, beforeExpiry))
})

test("a new audit report format requires review", () => {
  const { report, lock } = fixture()
  report.auditReportVersion = 3
  assert.throws(() => evaluateAudit(result(report), "editors/vscode/package-lock.json", lock, beforeExpiry))
})
