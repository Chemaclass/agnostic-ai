import { expect, test } from '@playwright/test'
import type { ChildProcess } from 'node:child_process'
import { execFile, spawn } from 'node:child_process'
import { mkdir, mkdtemp, readFile, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { createServer } from 'node:net'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { promisify } from 'node:util'

const run = promisify(execFile)
const uiRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..')
const binary = resolve(process.env.AGNOSTIC_E2E_BIN ?? join(uiRoot, '..', process.platform === 'win32' ? 'agnostic-ai.exe' : 'agnostic-ai'))

async function put(file: string, body: string) {
  await mkdir(dirname(file), { recursive: true })
  await writeFile(file, body)
}

async function stop(child: ChildProcess) {
  if (child.exitCode !== null || child.signalCode !== null) return
  await new Promise<void>((done) => {
    const timer = setTimeout(() => child.kill('SIGKILL'), 3_000)
    child.once('exit', () => { clearTimeout(timer); done() })
    child.kill('SIGTERM')
  })
}

async function serverURL(server: ChildProcess) {
  return new Promise<string>((accept, reject) => {
    let output = ''
    const timer = setTimeout(() => reject(new Error(`Server did not start: ${output}`)), 15_000)
    server.once('error', (error) => { clearTimeout(timer); reject(error) })
    server.once('exit', (code) => { clearTimeout(timer); reject(new Error(`Server exited ${code}: ${output}`)) })
    server.stderr?.on('data', (data) => { output += data.toString() })
    server.stdout?.on('data', (data) => {
      output += data.toString()
      const match = output.match(/http:\/\/127\.0\.0\.1:\d+\//)
      if (match) { clearTimeout(timer); accept(match[0]) }
    })
  })
}

async function availablePort() {
  const listener = createServer()
  return new Promise<number>((accept, reject) => {
    listener.once('error', reject)
    listener.listen(0, '127.0.0.1', () => {
      const address = listener.address()
      if (!address || typeof address === 'string') { listener.close(); reject(new Error('No test port')); return }
      listener.close((error) => error ? reject(error) : accept(address.port))
    })
  })
}

async function fixture(global = false, allowedOrigins: string[] = []) {
  const temporary = await mkdtemp(join(tmpdir(), 'agnostic-ui-live-'))
  const project = join(temporary, 'project')
  const home = join(temporary, 'global')
  const userHome = join(temporary, 'user')
  const root = global ? home : project
  const sources = global ? home : join(project, '.agnostic-ai')
  const config = join(root, 'agnostic-ai.yaml')
  const settings = join(sources, 'settings', 'native.yaml')
  const localSettings = join(sources, 'local', 'settings', 'native.yaml')
  const configBytes = '# Keep this layout.\nversion: 1\ntargets:\n    - claude # team tool\nbuiltins: []\n'
  const sharedBytes = '# Shared settings.\nname: native\nx-claude:\n    verbose: false\n'
  const localBytes = '# Personal settings.\nname: native\nx-claude:\n    alwaysThinkingEnabled: true\n'
  const env = {
    ...process.env,
    AGNOSTIC_AI_HOME: home,
    AGNOSTIC_AI_NO_UPDATE_CHECK: '1',
    AGNOSTIC_AI_TARGET: '',
    HOME: userHome,
    USERPROFILE: userHome,
    LOCALAPPDATA: join(temporary, 'cache'),
    XDG_CACHE_HOME: join(temporary, 'cache'),
    NO_COLOR: '1',
  }
  let child: ChildProcess | undefined
  const close = async () => {
    if (child) await stop(child)
    if (!temporary.startsWith(join(tmpdir(), 'agnostic-ui-live-'))) throw new Error('Unexpected fixture directory')
    console.log(`Removing test fixture ${temporary}`)
    await rm(temporary, { recursive: true, force: true })
  }
  try {
    await mkdir(project, { recursive: true })
    await mkdir(home, { recursive: true })
    await mkdir(userHome, { recursive: true })
    await put(config, configBytes)
    await put(settings, sharedBytes)
    await put(localSettings, localBytes)
    await put(join(sources, 'agents', 'reviewer.md'), '---\nname: reviewer\ndescription: Review changes.\ntools: [Read]\n---\nReview one change.\n')
    if (global) {
      await put(config, configBytes.replace('builtins: []', 'builtins: [memory]'))
      await put(join(home, 'local', 'agnostic-ai.yaml'), 'builtins: [memory]\n')
    } else {
      await put(join(project, 'fixture-marker'), 'selected project\n')
      await put(join(project, 'fake-mcp.mjs'), `import {createInterface} from 'node:readline';
import {readFileSync} from 'node:fs';
if(readFileSync('fixture-marker','utf8')!=='selected project\\n')process.exit(2);
createInterface({input:process.stdin}).on('line',line=>{
 const message=JSON.parse(line);if(message.id===undefined)return;
 const result=message.method==='initialize'
  ?{protocolVersion:'2025-03-26',capabilities:{tools:{}},serverInfo:{name:'fixture',version:'1'}}
  :{tools:[{name:'workspace-tool',description:'A local test tool.',inputSchema:{type:'object',properties:{}}}]};
 process.stdout.write(JSON.stringify({jsonrpc:'2.0',id:message.id,result})+'\\n');
});\n`)
      await put(join(sources, 'mcps', 'fixture.yaml'), `name: fixture\ncommand: ${JSON.stringify(process.execPath)}\nargs: [fake-mcp.mjs]\n`)
      const listed = await run(binary, ['mcp', 'tools', '--dir', project, '--timeout', '3s', '--json'], { cwd: temporary, env, timeout: 10_000 })
      expect(listed.stdout).toContain('workspace-tool')
    }
    const args = ['serve', '--addr', '127.0.0.1:0', ...allowedOrigins.flatMap((origin) => ['--allow-origin', origin]), ...(global ? ['--global'] : ['--dir', project])]
    child = spawn(binary, args, { cwd: temporary, env, stdio: ['ignore', 'pipe', 'pipe'] })
    const url = await serverURL(child)
    return { root, url, env, config, configBytes, settings, sharedBytes, localSettings, localBytes, close }
  } catch (error) {
    await close()
    throw error
  }
}

test('live project views, inherited settings, MCP cache, and builtin changes survive reload', async ({ page }, testInfo) => {
  const f = await fixture()
  const browserErrors: string[] = []
  page.on('pageerror', (error) => browserErrors.push(error.message))
  try {
    await page.emulateMedia({ colorScheme: 'light' })
    await page.goto(f.url)
    for (const name of ['Context', 'Authority', 'Tools', 'Lifecycle', 'Settings']) {
      await page.getByRole('navigation').getByRole('link', { name: new RegExp(`^${name}`) }).click()
      await expect(page.getByRole('heading', { level: 1, name, exact: true })).toBeVisible()
    }
    const memory = page.getByRole('switch', { name: 'memory built-in' })
    await expect(memory).toHaveAttribute('aria-checked', 'false')
    await memory.click()
    await expect(memory).toHaveAttribute('aria-checked', 'true')
    await page.reload()
    await expect(memory).toHaveAttribute('aria-checked', 'true')
    await memory.click()
    await expect(memory).toHaveAttribute('aria-checked', 'false')
    expect(await readFile(f.config, 'utf8')).toBe(f.configBytes)

    const verbose = page.getByRole('switch', { name: 'x-claude.verbose', exact: true })
    await expect(verbose).toHaveAttribute('aria-checked', 'false')
    await verbose.click()
    await expect(verbose).toHaveAttribute('aria-checked', 'true')
    await page.reload()
    await expect(verbose).toHaveAttribute('aria-checked', 'true')
    expect(await readFile(f.settings, 'utf8')).toBe(f.sharedBytes)
    expect(await readFile(f.localSettings, 'utf8')).toContain('verbose: true')
    await page.screenshot({ path: testInfo.outputPath('desktop-settings.png'), animations: 'disabled' })

    await page.getByRole('navigation').getByRole('link', { name: /^Tools/ }).click()
    await expect(page.locator('.tool-listing')).toContainText('workspace-tool')
    await page.setViewportSize({ width: 390, height: 844 })
    await page.emulateMedia({ colorScheme: 'dark' })
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark')
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    await page.screenshot({ path: testInfo.outputPath('mobile-tools.png'), animations: 'disabled' })
    expect(browserErrors).toEqual([])
  } finally {
    await f.close()
  }
})

test('live global edits keep the last local builtin disabled and enforce browser boundaries', async ({ page, request }) => {
  const f = await fixture(true)
  try {
    await page.goto(`${f.url}?view=settings`)
    const memory = page.getByRole('switch', { name: 'memory built-in' })
    await expect(memory).toHaveAttribute('aria-checked', 'true')
    await memory.click()
    await expect(memory).toHaveAttribute('aria-checked', 'false')
    await page.reload()
    await expect(memory).toHaveAttribute('aria-checked', 'false')
    expect(await readFile(join(f.root, 'local', 'agnostic-ai.yaml'), 'utf8')).toContain('builtins: []')
    expect(await readFile(f.config, 'utf8')).toBe(f.configBytes.replace('builtins: []', 'builtins: [memory]'))

    const edit = `${f.url}api/edit/builtin`
    const before = await readFile(join(f.root, 'local', 'agnostic-ai.yaml'), 'utf8')
    const foreign = await request.post(edit, { headers: { Origin: 'http://localhost:9876' }, data: { name: 'memory', enabled: true } })
    expect(foreign.status()).toBe(403)
    const wrongHost = await request.get(`${f.url}api/views/authority/schema`, { headers: { Host: 'attacker.example' } })
    expect(wrongHost.status()).toBe(421)
    const wrongType = await request.post(edit, { headers: { 'Content-Type': 'text/plain' }, data: '{}' })
    expect(wrongType.status()).toBe(415)
    const crossSite = await request.post(edit, { headers: { 'Sec-Fetch-Site': 'cross-site' }, data: { name: 'memory', enabled: true } })
    expect(crossSite.status()).toBe(403)
    const missingBoolean = await request.post(edit, { data: { name: 'memory' } })
    expect(missingBoolean.status()).toBe(400)
    expect(await readFile(join(f.root, 'local', 'agnostic-ai.yaml'), 'utf8')).toBe(before)
  } finally {
    await f.close()
  }
})

test('live Authority, overview, and edit errors redact configured credentials', async ({ request }) => {
  const f = await fixture()
  const secret = 'plain-secret-value'
  const deniedSecret = 'another-plain-value'
  const token = 'ghp_0123456789abcdefghijklmnop'
  const rule = `shell(curl --api-key ${secret}:*)`
  const deniedRule = `shell(curl --api-key ${deniedSecret}:*)`
  const permissions = join(f.root, '.agnostic-ai', 'settings', 'permissions.yaml')
  const rules = `name: permissions\npermissions:\n  allow: ['${rule}']\n  deny: ['${deniedRule}']\n`
  try {
    await put(permissions, rules)
    await put(f.settings, f.sharedBytes.replace('verbose: false\n', 'verbose: false\n    customValue: https://user:plain-url-password@host.invalid\n'))
    for (const endpoint of ['authority', 'overview']) {
      const response = await request.get(`${f.url}api/views/${endpoint}`)
      expect(response.status()).toBe(200)
      const body = JSON.stringify(await response.json())
      expect(body).not.toContain(secret)
      expect(body).not.toContain(deniedSecret)
      expect(body).not.toContain('plain-url-password')
      expect(body).toContain('redacted')
    }
    const impact = await request.post(`${f.url}api/views/authority/impact`, { data: { changes: [{ op: 'remove', list: 'allow', rule }] } })
    expect(impact.status()).toBe(200)
    const proposed = await impact.json()
    expect(proposed.valid).toBe(true)
    expect(proposed.changes).toHaveLength(1)
    expect(proposed.changes[0].declaredBefore).toBe('allowed')
    expect(proposed.changes[0].declaredAfter).toBe('default')
    expect(JSON.stringify(proposed)).not.toContain(secret)
    expect(JSON.stringify(proposed)).not.toContain(deniedSecret)
    expect(await readFile(permissions, 'utf8')).toBe(rules)
    const invalid = f.configBytes.replace('builtins: []', `builtins: [${token}]`)
    await put(f.config, invalid)
    const refused = await request.post(`${f.url}api/edit/builtin`, { data: { name: 'memory', enabled: true } })
    expect(refused.status()).toBe(422)
    expect(JSON.stringify(await refused.json())).not.toContain(token)
    expect(await readFile(f.config, 'utf8')).toBe(invalid)
  } finally {
    await f.close()
  }
})

test('the development proxy preserves real API edits with an explicitly allowed origin', async ({ page, request }) => {
  const port = await availablePort()
  const origin = `http://127.0.0.1:${port}`
  const f = await fixture(false, [origin])
  let development: ChildProcess | undefined
  try {
    development = spawn(process.execPath, [join(uiRoot, 'node_modules', 'vite', 'bin', 'vite.js'), '--host', '127.0.0.1', '--port', String(port), '--strictPort'], {
      cwd: uiRoot,
      env: { ...f.env, AGNOSTIC_AI_API_URL: f.url },
      stdio: ['ignore', 'pipe', 'pipe'],
    })
    await serverURL(development)
    const allowed = await request.get(`${f.url}api/views/overview`, { headers: { Origin: origin } })
    expect(allowed.status()).toBe(200)
    expect(allowed.headers()['access-control-allow-origin']).toBe(origin)
    await page.goto(`${origin}/?view=settings`)
    const memory = page.getByRole('switch', { name: 'memory built-in' })
    await expect(memory).toHaveAttribute('aria-checked', 'false')
    await memory.click()
    await expect(memory).toHaveAttribute('aria-checked', 'true')
    await page.reload()
    await expect(memory).toHaveAttribute('aria-checked', 'true')
    expect(await readFile(f.config, 'utf8')).toContain('memory')
  } finally {
    if (development) await stop(development)
    await f.close()
  }
})

test('Caveman previews, applies to every target, and undoes after reload', async ({ page }, testInfo) => {
  const f = await fixture()
  try {
    const config = f.configBytes.replace('- claude # team tool', '- claude # team tool\n    - codex')
    await put(f.config, config)
    await run(binary, ['sync'], { cwd: f.root, env: f.env })
    await page.goto(`${f.url}?view=settings`)
    const caveman = page.getByRole('switch', { name: 'caveman built-in', exact: true })
    await expect(caveman).toBeEnabled()
    await caveman.click()
    const preview = page.getByRole('region', { name: 'Caveman change preview' })
    await expect(preview.locator('.tool-avatar')).toHaveCount(2)
    await expect(preview.locator('.tool-avatar').first()).toHaveAttribute('title', 'Claude Code')
    await expect(preview.locator('.tool-avatar').nth(1)).toHaveAttribute('title', 'Codex')
    await expect(preview).toContainText('.claude/skills/caveman/SKILL.md')
    await expect(preview).toContainText('.agents/skills/caveman/SKILL.md')
    expect(await readFile(f.config, 'utf8')).toBe(config)
    await preview.getByRole('button', { name: 'Cancel', exact: true }).click()
    expect(await readFile(f.config, 'utf8')).toBe(config)
    await caveman.click()
    await preview.locator('summary').filter({ hasText: 'agnostic-ai.yaml' }).click()
    const configDiff = preview.getByLabel('Diff for agnostic-ai.yaml', { exact: true })
    await expect(configDiff).toContainText('caveman')
    await configDiff.scrollIntoViewIfNeeded()
    await page.screenshot({ path: testInfo.outputPath('caveman-preview-desktop.png'), animations: 'disabled' })
    await page.setViewportSize({ width: 390, height: 844 })
    await configDiff.scrollIntoViewIfNeeded()
    await expect(configDiff).toBeInViewport()
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    await page.screenshot({ path: testInfo.outputPath('caveman-preview-mobile.png'), animations: 'disabled' })
    await preview.getByRole('button', { name: 'Apply change', exact: true }).click()
    await expect(caveman).toHaveAttribute('aria-checked', 'true')
    for (const file of ['.claude/skills/caveman/SKILL.md', '.agents/skills/caveman/SKILL.md']) {
      expect(await readFile(join(f.root, file), 'utf8')).toContain('caveman')
    }
    await run(binary, ['sync', '--check'], { cwd: f.root, env: f.env })
    await page.reload()
    await page.getByRole('button', { name: 'Undo last change' }).click()
    await expect(caveman).toHaveAttribute('aria-checked', 'false')
    expect(await readFile(f.config, 'utf8')).toBe(config)
    await run(binary, ['sync', '--check'], { cwd: f.root, env: f.env })
  } finally {
    await f.close()
  }
})

test('Caveman conflicts preserve later edits and explain how to recover', async ({ page }) => {
  const f = await fixture()
  try {
    await run(binary, ['sync'], { cwd: f.root, env: f.env })
    await page.goto(`${f.url}?view=settings`)
    const caveman = page.getByRole('switch', { name: 'caveman built-in', exact: true })
    await expect(caveman).toBeEnabled()
    await caveman.click()
    await expect(page.getByRole('button', { name: 'Apply change', exact: true })).toBeVisible()
    const changed = f.configBytes + '# An intervening edit.\n'
    await put(f.config, changed)
    await page.getByRole('button', { name: 'Apply change', exact: true }).click()
    await expect(page.getByRole('alert')).toContainText('Files changed. Review a fresh preview.')
    expect(await readFile(f.config, 'utf8')).toBe(changed)
    await page.getByRole('button', { name: 'Reload change state' }).click()
    await expect(page.getByRole('button', { name: 'Apply change', exact: true })).not.toBeVisible()
    await expect(caveman).toBeEnabled()
    await caveman.click()
    await page.getByRole('button', { name: 'Apply change', exact: true }).click()
    await expect(caveman).toHaveAttribute('aria-checked', 'true')
    const output = join(f.root, '.claude/skills/caveman/SKILL.md')
    await put(output, 'A later manual change.\n')
    await page.getByRole('button', { name: 'Undo last change' }).click()
    await expect(page.getByRole('alert')).toContainText('Files changed. Review a fresh preview.')
    expect(await readFile(output, 'utf8')).toBe('A later manual change.\n')
  } finally {
    await f.close()
  }
})


test('Caveman reload shows an external config change instead of historical apply state', async ({ page }) => {
  const f = await fixture()
  try {
    await run(binary, ['sync'], { cwd: f.root, env: f.env })
    await page.goto(`${f.url}?view=settings`)
    const caveman = page.getByRole('switch', { name: 'caveman built-in', exact: true })
    await expect(caveman).toBeEnabled()
    await caveman.click()
    await page.getByRole('button', { name: 'Apply change', exact: true }).click()
    await expect(caveman).toHaveAttribute('aria-checked', 'true')
    await put(f.config, f.configBytes)
    await run(binary, ['sync'], { cwd: f.root, env: f.env })
    await page.reload()
    await expect(caveman).toBeEnabled()
    await expect(caveman).toHaveAttribute('aria-checked', 'false')
    await caveman.click()
    await expect(page.getByRole('heading', { name: 'Enable Caveman', exact: true })).toBeVisible()
  } finally {
    await f.close()
  }
})
