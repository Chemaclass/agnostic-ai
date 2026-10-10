#!/usr/bin/env bash

function test_provenance_requires_verified_release_identity_and_tarball() {
  local script_dir
  script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
  node - "$script_dir/npm-verify-provenance.js" <<'JS'
const assert = require('node:assert/strict')
const { verifyPackage } = require(process.argv[2])
const repo = 'https://github.com/Chemaclass/agnostic-ai'
const sha = 'a'.repeat(40)
const integrity = 'sha512-' + Buffer.from('digest').toString('base64')
function fixture() {
  const statement = {
    predicateType: 'https://slsa.dev/provenance/v1',
    subject: [{ name: 'pkg:npm/agnostic-ai@1.2.3', digest: { sha512: Buffer.from('digest').toString('hex') } }],
    predicate: { buildDefinition: {
      buildType: 'https://github.com/npm/cli/gha/v2',
      externalParameters: { workflow: { repository: repo, path: '.github/workflows/release.yml', ref: 'refs/tags/v1.2.3' } },
      resolvedDependencies: [{ uri: `git+${repo}@refs/tags/v1.2.3`, digest: { gitCommit: sha } }]
    }, runDetails: { builder: { id: 'https://github.com/actions/runner/github-hosted' } } }
  }
  const bundle = { dsseEnvelope: { payload: Buffer.from(JSON.stringify(statement)).toString('base64') } }
  const calls = []
  const manifest = { _integrity: integrity, _signatures: [{}], _attestations: { url: 'https://registry.npmjs.org/-/npm/v1/attestations/agnostic-ai@1.2.3' } }
  const attestations = [ { predicateType: statement.predicateType, bundle }, { predicateType: 'https://github.com/npm/attestation/tree/main/specs/publish/v0.1', bundle: { dsseEnvelope: { payload: Buffer.from(JSON.stringify({ predicateType: 'https://github.com/npm/attestation/tree/main/specs/publish/v0.1', subject: statement.subject })).toString('base64'), signatures: [{ keyid: 'key' }] }, verificationMaterial: { tlogEntries: [{ integratedTime: '1700000000' }] } } } ]
  const deps = {
    tuf: { initTUF: async () => ({ getTarget: async () => JSON.stringify({ keys: [{ keyId: 'key', publicKey: { rawBytes: 'pub', validFor: {} } }] }) }) },
    pacote: {
      manifest: async (spec, options) => { calls.push(['manifest', spec, options]); return manifest },
      tarball: async (spec, options) => { calls.push(['tarball', spec, options]); return Buffer.from('tarball') }
    },
    fetch: { json: async () => ({ attestations }) },
    sigstore: { verify: async (value, options) => { calls.push(['sigstore', value, options]) } }
  }
  return { deps, manifest, attestations, statement, bundle, calls, update() { bundle.dsseEnvelope.payload = Buffer.from(JSON.stringify(statement)).toString('base64') } }
}
async function run(f) { return verifyPackage('agnostic-ai', '1.2.3', sha, f.deps) }
async function reject(change) { const f = fixture(); change(f); f.update(); await assert.rejects(run(f)) }
;(async () => {
  const f = fixture(); await run(f)
  const manifestCall = f.calls.find(c => c[0] === 'manifest')
  assert.equal(manifestCall[2].verifySignatures, true)
  assert.equal(manifestCall[2].verifyAttestations, true)
  assert.equal(manifestCall[2]['//registry.npmjs.org/:_keys'][0].keyid, 'key')
  assert.equal(f.calls.find(c => c[0] === 'tarball')[2].integrity, integrity)
  const policy = f.calls.find(c => c[0] === 'sigstore' && c[2].certificateIssuer)[2]
  assert.equal(policy.certificateIssuer, 'https://token.actions.githubusercontent.com')
  assert.equal(policy.certificateIdentityURI, `${repo}/.github/workflows/release.yml@refs/tags/v1.2.3`)
  await reject(f => { delete f.manifest._attestations })
  await reject(f => { f.manifest._signatures = [] })
  await reject(f => { f.attestations.pop() })
  await reject(f => { f.attestations.shift() })
  await reject(f => { f.statement.predicate.buildDefinition.externalParameters.workflow.repository = 'https://github.com/attacker/repo' })
  await reject(f => { f.statement.predicate.buildDefinition.externalParameters.workflow.path = '.github/workflows/other.yml' })
  await reject(f => { f.statement.predicate.buildDefinition.externalParameters.workflow.ref = 'refs/heads/main' })
  await reject(f => { f.statement.predicate.buildDefinition.resolvedDependencies[0].digest.gitCommit = 'b'.repeat(40) })
  await reject(f => { f.statement.subject[0].name = 'pkg:npm/attacker@1.2.3' })
  await reject(f => { f.statement.subject[0].digest.sha512 = 'wrong' })
  await reject(f => { f.attestations[1].bundle.dsseEnvelope.signatures[0].keyid = 'unknown' })
  await reject(f => { f.deps.pacote.manifest = async () => { throw Error('invalid signature') } })
  await reject(f => { f.deps.sigstore.verify = async () => { throw Error('wrong certificate identity') } })
  await reject(f => { f.deps.pacote.tarball = async () => { throw Error('integrity mismatch') } })
})().catch(error => { console.error(error.message); process.exitCode = 1 })
JS
  assert_successful_code "$?"
}
