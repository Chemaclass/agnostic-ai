'use strict'

const assert = require('node:assert/strict')
const { spawn } = require('node:child_process')
const { mkdtemp, mkdir, rm, writeFile } = require('node:fs/promises')
const { tmpdir } = require('node:os')
const path = require('node:path')

async function stop(child) {
  if (!child?.pid || child.exitCode !== null || child.signalCode !== null) return
  let force, deadline
  try {
    await new Promise((accept, reject) => {
      child.once('exit', accept)
      force = setTimeout(() => child.kill('SIGKILL'), 3_000)
      deadline = setTimeout(() => reject(new Error('Server did not stop after SIGKILL')), 6_000)
      child.kill('SIGTERM')
    })
  } finally {
    clearTimeout(force)
    clearTimeout(deadline)
  }
}

async function verifyBundledUI(binary) {
  const temporary = await mkdtemp(path.join(tmpdir(), 'agnostic-ui-smoke-'))
  let child
  try {
    const project = path.join(temporary, 'project')
    const home = path.join(temporary, 'home')
    await mkdir(project)
    await mkdir(home)
    await writeFile(path.join(project, 'agnostic-ai.yaml'), 'version: 1\ntargets: [claude]\nbuiltins: []\n')
    child = spawn(path.resolve(binary), ['serve', '--addr', '127.0.0.1:0', '--dir', project], {
      cwd: temporary,
      env: {
        ...process.env,
        PATH: '',
        HOME: home,
        USERPROFILE: home,
        AGNOSTIC_AI_HOME: path.join(home, '.agnostic-ai'),
        AGNOSTIC_AI_NO_UPDATE_CHECK: '1',
      },
      stdio: ['ignore', 'pipe', 'pipe'],
    })
    const url = await new Promise((accept, reject) => {
      let output = ''
      const timer = setTimeout(() => reject(new Error(`Server startup timed out: ${output}`)), 15_000)
      child.once('error', (error) => { clearTimeout(timer); reject(error) })
      child.once('exit', (code) => { clearTimeout(timer); reject(new Error(`Server exited ${code}: ${output}`)) })
      const collect = (data) => {
        output = (output + data.toString()).slice(-16_384)
        const match = output.match(/http:\/\/127\.0\.0\.1:\d+\//)
        if (match) { clearTimeout(timer); accept(match[0]) }
      }
      child.stdout.on('data', collect)
      child.stderr.on('data', collect)
    })
    const get = (route) => fetch(new URL(route, url), { signal: AbortSignal.timeout(15_000) })
    const response = await get('/')
    assert.equal(response.status, 200)
    assert.match(response.headers.get('content-type'), /text\/html/)
    const html = await response.text()
    assert.match(html, /<div id="root"><\/div>/)
    const assets = [...html.matchAll(/(?:src|href)="([^"]+\.(?:js|css))"/g)].map((match) => match[1])
    assert.ok(assets.some((asset) => asset.endsWith('.js')), 'missing bundled JavaScript')
    assert.ok(assets.some((asset) => asset.endsWith('.css')), 'missing bundled CSS')
    for (const asset of assets) {
      assert.equal(new URL(asset, url).origin, new URL(url).origin)
      const result = await get(asset)
      assert.equal(result.status, 200, asset)
      assert.doesNotMatch(result.headers.get('content-type'), /text\/html/, asset)
      assert.ok((await result.text()).length > 0, asset)
    }
    assert.equal((await get('/assets/missing.js')).status, 404)
    const unknown = await get('/api/missing')
    assert.equal(unknown.status, 404)
    assert.match(unknown.headers.get('content-type'), /application\/json/)
    const overview = await get('/api/views/overview')
    assert.equal(overview.status, 200)
    assert.equal((await overview.json()).schemaVersion, 'overview/v1')
    await stop(child)
    child = undefined
  } finally {
    await stop(child)
    console.log(`Removing UI smoke fixture ${temporary}`)
    await rm(temporary, { recursive: true, force: true })
  }
}

module.exports = { verifyBundledUI }

if (require.main === module) {
  if (!process.argv[2]) throw new Error('usage: node scripts/ui-smoke.js /path/to/agnostic-ai')
  verifyBundledUI(process.argv[2]).then(() => console.log('Bundled UI works without Node.js on PATH.')).catch((error) => {
    console.error(error.message)
    process.exitCode = 1
  })
}
