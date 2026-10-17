'use strict'

const path = require('node:path')
const { createRequire } = require('node:module')

const registry = 'https://registry.npmjs.org/'
const repository = 'https://github.com/Chemaclass/agnostic-ai'
const workflow = '.github/workflows/release.yml'
const provenanceType = 'https://slsa.dev/provenance/v1'
const publishType = 'https://github.com/npm/attestation/tree/main/specs/publish/v0.1'

async function verifyPackage(name, version, commit, deps) {
  if (!/^[a-f0-9]{40}$/.test(commit)) throw new Error('expected release commit is required')
  const tuf = await deps.tuf.initTUF({})
  const { keys } = JSON.parse(await tuf.getTarget('registry.npmjs.org/keys.json'))
  const options = {
    registry,
    verifySignatures: true,
    verifyAttestations: true,
    '//registry.npmjs.org/:_keys': keys.map(key => ({
      keyid: key.keyId,
      pemkey: `-----BEGIN PUBLIC KEY-----\n${key.publicKey.rawBytes}\n-----END PUBLIC KEY-----`,
      expires: key.publicKey.validFor.end || null,
    })),
  }
  const spec = `${name}@${version}`
  const manifest = await deps.pacote.manifest(spec, options)
  if (!manifest._signatures?.length || !manifest._attestations || !manifest._integrity) {
    throw new Error('required registry signature or provenance is missing')
  }
  const url = new URL(manifest._attestations.url)
  if (url.origin !== new URL(registry).origin) throw new Error('unexpected attestation registry')
  const { attestations } = await deps.fetch.json(url.href, { registry })
  const provenance = attestations.find(value => value.predicateType === provenanceType)
  if (!provenance || !attestations.some(value => value.predicateType === publishType)) {
    throw new Error('required build or registry publish attestation is missing')
  }
  const publish = attestations.find(value => value.predicateType === publishType)
  const keyid = publish.bundle.dsseEnvelope.signatures[0].keyid
  const key = options['//registry.npmjs.org/:_keys'].find(value => value.keyid === keyid)
  const signedAt = Number(publish.bundle.verificationMaterial.tlogEntries[0].integratedTime) * 1000
  if (!key || !Number.isFinite(signedAt) || (key.expires && signedAt >= Date.parse(key.expires))) {
    throw new Error('registry publish attestation has no valid trusted signing key')
  }
  await deps.sigstore.verify(publish.bundle, { keySelector: () => key.pemkey })
  const ref = `refs/tags/v${version}`
  await deps.sigstore.verify(provenance.bundle, {
    certificateIssuer: 'https://token.actions.githubusercontent.com',
    certificateIdentityURI: `${repository}/${workflow}@${ref}`,
  })
  const purl = `pkg:npm/${name.replace('@', '%40')}@${version}`
  const digest = Buffer.from(manifest._integrity.replace(/^sha512-/, ''), 'base64').toString('hex')
  for (const value of [provenance, publish]) {
    const signed = JSON.parse(Buffer.from(value.bundle.dsseEnvelope.payload, 'base64').toString('utf8'))
    if (signed.predicateType !== value.predicateType || signed.subject?.length !== 1 ||
        signed.subject[0].name !== purl || signed.subject[0].digest?.sha512 !== digest) {
      throw new Error('attestation subject does not match the package version and tarball integrity')
    }
  }
  const statement = JSON.parse(Buffer.from(provenance.bundle.dsseEnvelope.payload, 'base64').toString('utf8'))
  const definition = statement.predicate?.buildDefinition
  const sourceWorkflow = definition?.externalParameters?.workflow
  if (statement.predicateType !== provenanceType ||
      sourceWorkflow?.repository !== repository || sourceWorkflow?.path !== workflow || sourceWorkflow?.ref !== ref ||
      !definition.resolvedDependencies?.some(source => source.uri === `git+${repository}@${ref}` && source.digest?.gitCommit === commit)) {
    throw new Error('signed source does not match the release workflow, tag and commit')
  }
  await deps.pacote.tarball(spec, { registry, integrity: manifest._integrity })
}

module.exports = { verifyPackage }

if (require.main === module) {
  const [name, version, commit, npmRoot] = process.argv.slice(2)
  try {
    // Use the release's pinned npm verifier and trust root without another dependency.
    const npmRequire = createRequire(path.join(npmRoot, 'npm', 'package.json'))
    const deps = {
      pacote: npmRequire('pacote'),
      sigstore: npmRequire('sigstore'),
      tuf: npmRequire('@sigstore/tuf'),
      fetch: npmRequire('npm-registry-fetch'),
    }
    verifyPackage(name, version, commit, deps).catch(error => {
      console.error(`npm-provenance: ${name}@${version}: ${error.message}`)
      process.exitCode = 1
    })
  } catch (error) {
    console.error(`npm-provenance: ${name}@${version}: ${error.message}`)
    process.exitCode = 1
  }
}
