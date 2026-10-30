import { expect, test } from '@playwright/test'
import type { ChildProcess } from 'node:child_process'
import { execFile, spawn } from 'node:child_process'
import { mkdir, mkdtemp, readdir, readFile, rm, stat, symlink, writeFile } from 'node:fs/promises'
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

async function projectFiles(root: string) {
  const files: Record<string, { bytes: string; mode: number }> = {}
  async function visit(directory: string, prefix = '') {
    const entries = await readdir(directory, { withFileTypes: true })
    for (const entry of entries.sort((a, b) => a.name.localeCompare(b.name))) {
      const relative = prefix + entry.name
      if (relative === '.agnostic-ai/.command-lock') continue
      const file = join(directory, entry.name)
      if (entry.isDirectory()) await visit(file, relative + '/')
      else files[relative] = { bytes: (await readFile(file)).toString('base64'), mode: (await stat(file)).mode & 0o777 }
    }
  }
  await visit(root)
  return files
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

async function fixture(global = false, allowedOrigins: string[] = [], gitRepository = false) {
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
      if (gitRepository) await run('git', ['init', '--quiet'], { cwd: project, env })
      await run(binary, ['sync'], { cwd: project, env })
    }
    const args = ['serve', '--addr', '127.0.0.1:0', ...allowedOrigins.flatMap((origin) => ['--allow-origin', origin]), ...(global ? ['--global'] : ['--dir', project])]
    child = spawn(binary, args, { cwd: temporary, env, stdio: ['ignore', 'pipe', 'pipe'] })
    const url = await serverURL(child)
    return { root, userHome, url, env, config, configBytes, settings, sharedBytes, localSettings, localBytes, close }
  } catch (error) {
    await close()
    throw error
  }
}

test('live Authority previews source changes, saves rules, and applies them on the next sync', async ({ page }, testInfo) => {
  const f = await fixture(false, [], true)
  try {
    const original = '# Shared settings.\nname: native\npermissions:\n    deny: [edit(.env)] # Keep this rule.\nx-claude:\n    verbose: false\n'
    await put(f.settings, original)
    const nativeFile = join(f.root, '.claude', 'settings.json')
    const nativeBefore = await readFile(nativeFile, 'utf8')
    await page.goto(`${f.url}?view=authority`)
    const row = page.locator('.field-row').filter({ has: page.getByText('Ask first', { exact: true }) })
    await row.getByRole('button', { name: 'Add rules', exact: true }).click()
    await row.getByRole('textbox', { name: 'Ask first rules', exact: true }).fill('shell(git push *)\nread(private/**)')
    await row.getByRole('button', { name: 'Preview changes' }).click()
    const preview = row.getByRole('region', { name: 'Ask first change preview' })
    await expect(preview).toContainText('.agnostic-ai/settings/native.yaml')
    expect(await readFile(f.settings, 'utf8')).toBe(original)
    expect(await readFile(nativeFile, 'utf8')).toBe(nativeBefore)
    await page.screenshot({ path: testInfo.outputPath('permission-preview-desktop.png'), animations: 'disabled' })
    await page.setViewportSize({ width: 390, height: 844 })
    await page.emulateMedia({ colorScheme: 'dark' })
    await expect(preview.getByRole('button', { name: 'Save rules', exact: true })).toBeVisible()
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    await page.screenshot({ path: testInfo.outputPath('permission-preview-mobile.png'), animations: 'disabled' })
    await preview.getByRole('button', { name: 'Save rules', exact: true }).click()
    await expect(page.locator('.permission-saved')).toContainText('Run agnostic-ai sync')
    const saved = await readFile(f.settings, 'utf8')
    expect(saved).toContain('# Shared settings.')
    expect(saved).toContain('deny: [edit(.env)] # Keep this rule.')
    expect(saved).toContain('shell(git push *)')
    expect(await readFile(nativeFile, 'utf8')).toBe(nativeBefore)
    await page.reload()
    await expect(row).toContainText('read(private/**)')
    await run(binary, ['sync'], { cwd: f.root, env: f.env })
    const native = JSON.parse(await readFile(nativeFile, 'utf8'))
    expect(native.permissions.ask).toEqual(expect.arrayContaining(['Bash(git push *)', 'Read(private/**)']))
    expect(native.permissions.deny).toContain('Edit(.env)')
    await run(binary, ['sync', '--check'], { cwd: f.root, env: f.env })
  } finally { await f.close() }
})

test('live Authority replaces one personal list and resets it without changing other settings', async ({ page }) => {
  const f = await fixture(false, [], true)
  try {
    const shared = '# Shared settings.\nname: native\npermissions:\n    ask: [shell(git push *)]\n    deny: [edit(.env)]\nx-claude:\n    verbose: false\n'
    await put(f.settings, shared)
    await page.goto(`${f.url}?view=authority`)
    await page.getByRole('button', { name: 'Personal (not committed)', exact: true }).click()
    const row = page.locator('.field-row').filter({ has: page.getByText('Ask first', { exact: true }) })
    await row.getByRole('button', { name: 'Edit rules', exact: true }).click()
    await expect(row.getByRole('textbox', { name: 'Ask first rules', exact: true })).toHaveValue('shell(git push *)')
    await row.getByRole('textbox', { name: 'Ask first rules', exact: true }).fill('shell(git push origin main)')
    await row.getByRole('button', { name: 'Preview changes' }).click()
    const preview = row.getByRole('region', { name: 'Ask first change preview' })
    await expect(preview).toContainText('Personal (not committed)')
    await expect(preview).toContainText('.agnostic-ai/local/settings/native.yaml')
    expect(await readFile(f.localSettings, 'utf8')).toBe(f.localBytes)
    await preview.getByRole('button', { name: 'Save rules', exact: true }).click()
    await expect(row).toContainText('Personal overrides this file’s shared rules.')
    expect(await readFile(f.settings, 'utf8')).toBe(shared)
    expect(await readFile(f.localSettings, 'utf8')).toContain('alwaysThinkingEnabled: true')
    await run('git', ['check-ignore', '-q', '.agnostic-ai/local/settings/native.yaml'], { cwd: f.root, env: f.env })
    await run(binary, ['sync'], { cwd: f.root, env: f.env })
    const native = JSON.parse(await readFile(join(f.root, '.claude', 'settings.json'), 'utf8'))
    expect(native.permissions.ask).toEqual(['Bash(git push origin main)'])
    expect(native.permissions.deny).toEqual(['Edit(.env)'])
    await page.reload()
    await row.getByRole('button', { name: 'Reset to shared rules', exact: true }).click()
    await preview.getByRole('button', { name: 'Save rules', exact: true }).click()
    await expect(row).toContainText('Inherited from shared')
    expect(await readFile(f.localSettings, 'utf8')).toBe(f.localBytes)
    expect(await readFile(f.settings, 'utf8')).toBe(shared)
  } finally { await f.close() }
})

test('live Authority rejects a changed preview and preserves the rule draft for a new preview', async ({ page }) => {
  const f = await fixture()
  try {
    await page.goto(`${f.url}?view=authority`)
    const row = page.locator('.field-row').filter({ has: page.getByText('Ask first', { exact: true }) })
    await row.getByRole('button', { name: 'Add rules', exact: true }).click()
    const textbox = row.getByRole('textbox', { name: 'Ask first rules', exact: true })
    await textbox.fill('shell(git push *)')
    await row.getByRole('button', { name: 'Preview changes' }).click()
    const external = f.sharedBytes + '# Added from another editor.\n'
    await put(f.settings, external)
    await row.getByRole('button', { name: 'Save rules', exact: true }).click()
    await expect(row.getByRole('alert')).toContainText('changed')
    await expect(textbox).toHaveValue('shell(git push *)')
    expect(await readFile(f.settings, 'utf8')).toBe(external)
    await row.getByRole('button', { name: 'Reload saved values' }).click()
    await expect(row.getByRole('status')).toContainText('Your draft is kept')
    await row.getByRole('button', { name: 'Preview changes' }).click()
    await row.getByRole('button', { name: 'Save rules', exact: true }).click()
    await expect(page.locator('.permission-saved')).toContainText('Saved')
    expect(await readFile(f.settings, 'utf8')).toContain('# Added from another editor.')
    expect(await readFile(f.settings, 'utf8')).toContain('shell(git push *)')
  } finally { await f.close() }
})

test('live Authority selects a personal-only list by declared spec across multiple settings files', async ({ page }) => {
  const f = await fixture()
  try {
    await put(f.settings, 'name: workspace\nx-claude:\n  verbose: false\n')
    await put(f.localSettings, 'name: workspace\npermissions:\n  ask: [shell(git push origin main)]\nx-claude:\n  alwaysThinkingEnabled: true\n')
    await put(join(f.root, '.agnostic-ai', 'settings', 'other.yaml'), 'name: other\neffort: high\n')
    await page.goto(`${f.url}?view=authority`)
    const row = page.locator('.field-row').filter({ has: page.getByText('Ask first', { exact: true }) })
    await row.getByLabel('Settings file for Ask first').selectOption('.agnostic-ai/settings/native.yaml')
    await row.getByRole('button', { name: 'View personal settings' }).click()
    await row.getByRole('button', { name: 'Edit rules', exact: true }).click()
    await expect(row.getByRole('textbox', { name: 'Ask first rules' })).toHaveValue('shell(git push origin main)')
    await row.getByRole('textbox', { name: 'Ask first rules' }).fill('shell(git push origin release)')
    await row.getByRole('button', { name: 'Preview changes' }).click()
    await row.getByRole('button', { name: 'Save rules', exact: true }).click()
    await expect(page.locator('.permission-saved')).toContainText('.agnostic-ai/local/settings/native.yaml')
    expect(await readFile(f.localSettings, 'utf8')).toContain('shell(git push origin release)')
    expect(await readFile(f.localSettings, 'utf8')).toContain('alwaysThinkingEnabled: true')
    expect(await readFile(f.settings, 'utf8')).not.toContain('permissions')
    await row.getByRole('button', { name: 'Reset to shared rules', exact: true }).click()
    await row.getByRole('button', { name: 'Save rules', exact: true }).click()
    await expect.poll(() => readFile(f.localSettings, 'utf8')).not.toContain('permissions')
  } finally { await f.close() }
})

test('live Authority keeps editing the first personal-only spec through save and reset', async ({ page }) => {
  const f = await fixture(false, [], true)
  try {
    await rm(f.settings)
    await rm(f.localSettings)
    await page.goto(`${f.url}?view=authority`)
    await page.getByRole('button', { name: 'Personal (not committed)', exact: true }).click()
    const row = page.locator('.field-row').filter({ has: page.getByText('Ask first', { exact: true }) })
    await row.getByRole('button', { name: 'Add rules', exact: true }).click()
    await row.getByRole('textbox', { name: 'Ask first rules', exact: true }).fill('shell(git push origin main)')
    await row.getByRole('button', { name: 'Preview changes' }).click()
    await row.getByRole('button', { name: 'Save rules', exact: true }).click()
    await row.getByRole('button', { name: 'Edit rules', exact: true }).click()
    await expect(row.getByRole('textbox', { name: 'Ask first rules', exact: true })).toHaveValue('shell(git push origin main)')
    await row.getByRole('textbox', { name: 'Ask first rules', exact: true }).fill('shell(git push origin release)')
    await row.getByRole('button', { name: 'Preview changes' }).click()
    await row.getByRole('button', { name: 'Save rules', exact: true }).click()
    await row.getByRole('button', { name: 'Reset to shared rules', exact: true }).click()
    await row.getByRole('button', { name: 'Save rules', exact: true }).click()
    await row.getByRole('button', { name: 'Add rules', exact: true }).click()
    await row.getByRole('textbox', { name: 'Ask first rules', exact: true }).fill('read(private/**)')
    await row.getByRole('button', { name: 'Preview changes' }).click()
    await row.getByRole('button', { name: 'Save rules', exact: true }).click()
    await expect(row.getByRole('button', { name: 'Edit rules', exact: true })).toBeVisible()
    const source = join(f.root, '.agnostic-ai', 'local', 'settings', 'permissions.yaml')
    expect(await readFile(source, 'utf8')).toContain('read(private/**)')
  } finally { await f.close() }
})

test('live Authority explains whole-group personal clears and refuses masked shared edits', async ({ page, request }) => {
  const f = await fixture()
  try {
    const shared = 'name: native\npermissions:\n  allow: [read]\n'
    const local = 'name: native\npermissions: null\n'
    await put(f.settings, shared)
    await put(f.localSettings, local)
    await page.goto(`${f.url}?view=authority`)
    const row = page.locator('.field-row').filter({ has: page.getByText('Allowed without asking', { exact: true }) })
    await expect(row.locator('.field-source')).toContainText('read')
    await expect(row).toContainText('clears every permission list')
    await expect(row.getByRole('button', { name: /^(Add rules|Edit rules|Reset to shared rules)$/ })).toHaveCount(0)
    await page.getByRole('button', { name: 'Personal (not committed)', exact: true }).click()
    await expect(row).toContainText('clears every permission list')
    await expect(row.getByRole('button', { name: /^(Add rules|Edit rules|Reset to shared rules)$/ })).toHaveCount(0)
    const response = await request.post(`${f.url}api/edit/permissions`, { data: { action: 'preview', audience: 'shared', path: '.agnostic-ai/settings/native.yaml', key: 'permissions.allow', before: ['read'], rules: ['read(src/**)'] } })
    expect(response.status()).toBeGreaterThanOrEqual(400)
    expect(await response.text()).toContain('clears every permission list')
    expect(await readFile(f.settings, 'utf8')).toBe(shared)
    expect(await readFile(f.localSettings, 'utf8')).toBe(local)
  } finally { await f.close() }
})

test('live Authority asks for a valid source after deleting a personal-only spec beside another source', async ({ page }) => {
  const f = await fixture()
  try {
    await rm(f.settings)
    await rm(f.localSettings)
    await page.goto(`${f.url}?view=authority`)
    await page.getByRole('button', { name: 'Personal (not committed)', exact: true }).click()
    const row = page.locator('.field-row').filter({ has: page.getByText('Ask first', { exact: true }) })
    await row.getByRole('button', { name: 'Add rules', exact: true }).click()
    await row.getByRole('textbox', { name: 'Ask first rules', exact: true }).fill('shell(git push origin main)')
    await row.getByRole('button', { name: 'Preview changes' }).click()
    await row.getByRole('button', { name: 'Save rules', exact: true }).click()
    await expect(row.getByRole('button', { name: 'Edit rules', exact: true })).toBeVisible()
    await put(join(f.root, '.agnostic-ai', 'settings', 'other.yaml'), 'name: other\neffort: high\n')
    await page.reload()
    const source = row.getByLabel('Settings file for Ask first')
    await source.selectOption('.agnostic-ai/local/settings/permissions.yaml')
    await row.getByRole('button', { name: 'Reset to shared rules', exact: true }).click()
    await row.getByRole('button', { name: 'Save rules', exact: true }).click()
    await expect(source).toBeVisible()
    await expect(source).toHaveValue('')
    await expect(row.getByRole('button', { name: 'Add rules', exact: true })).toBeDisabled()
    await source.selectOption('.agnostic-ai/settings/other.yaml')
    await row.getByRole('button', { name: 'Add rules', exact: true }).click()
    await row.getByRole('textbox', { name: 'Ask first rules', exact: true }).fill('read(private/**)')
    await row.getByRole('button', { name: 'Preview changes' }).click()
    await row.getByRole('button', { name: 'Save rules', exact: true }).click()
    await expect(page.locator('.permission-saved')).toContainText('.agnostic-ai/local/settings/other.yaml')
    expect(await readFile(join(f.root, '.agnostic-ai', 'local', 'settings', 'other.yaml'), 'utf8')).toContain('read(private/**)')
  } finally { await f.close() }
})

test('live Settings keeps a cleared personal tool value distinct from shared inheritance', async ({ page }) => {
  const f = await fixture()
  try {
    await put(f.config, f.configBytes.replace('    - claude # team tool\n', '    - claude # team tool\n    - codex\n'))
    await put(f.settings, 'name: native\nmodel:\n  default: sonnet\n  codex: gpt-5\n')
    await put(f.localSettings, 'name: native\nmodel:\n  codex: null\n')
    await page.goto(`${f.url}?view=settings`)
    await page.getByRole('button', { name: 'Personal (not committed)', exact: true }).click()
    const row = page.locator('.field-row').filter({ has: page.getByText('Default model', { exact: true }) })
    await expect(row.locator('.field-source').filter({ hasText: 'gpt-5' })).toHaveCount(0)
    await row.getByText('What each tool gets').click()
    await row.getByRole('combobox', { name: 'Tool', exact: true }).selectOption('codex')
    const input = row.getByRole('textbox', { name: 'Default model for Codex value', exact: true })
    await expect(input).toHaveValue('')
    await expect(input).toHaveAttribute('placeholder', 'Cleared for you')
    await row.getByRole('button', { name: 'Preview personal reset', exact: true }).click()
    const preview = row.getByRole('region', { name: 'Default model for Codex change preview' })
    await expect(preview).toBeVisible()
    await preview.getByRole('button', { name: 'Apply change', exact: true }).click()
    await expect.poll(() => readFile(f.localSettings, 'utf8')).not.toContain('null')
    await expect(input).toHaveValue('gpt-5')
  } finally { await f.close() }
})

test('live Settings shows the selected audience and edits only personal values', async ({ page }) => {
  const f = await fixture()
  try {
    const sharedConfig = f.configBytes.replace('builtins: []', 'builtins: [handoff]')
    const sharedSettings = 'name: native\nmodel: sonnet\neffort: high\nx-claude:\n  verbose: false\n'
    await put(f.config, sharedConfig)
    await put(join(f.root, 'agnostic-ai.local.yaml'), 'builtins: [caveman]\n')
    await put(f.settings, sharedSettings)
    await put(f.localSettings, 'name: native\nmodel: opus\nx-claude:\n  verbose: true\n')
    await page.goto(`${f.url}?view=settings`)
    const caveman = page.getByRole('switch', { name: 'caveman built-in' })
    const handoff = page.getByRole('switch', { name: 'handoff built-in' })
    const verbose = page.getByRole('switch', { name: 'x-claude.verbose', exact: true })
    const model = page.getByRole('textbox', { name: 'Default model value', exact: true })
    await expect(caveman).toHaveAttribute('aria-checked', 'false')
    await expect(handoff).toHaveAttribute('aria-checked', 'true')
    await expect(caveman).toBeDisabled()
    await expect(verbose).toHaveAttribute('aria-checked', 'false')
    await expect(verbose).toBeDisabled()
    await expect(model).toHaveValue('sonnet')

    await page.getByRole('button', { name: 'Personal (not committed)', exact: true }).click()
    await expect(caveman).toHaveAttribute('aria-checked', 'true')
    await expect(handoff).toHaveAttribute('aria-checked', 'false')
    await expect(caveman).toBeEnabled()
    await expect(verbose).toHaveAttribute('aria-checked', 'true')
    await expect(model).toHaveValue('opus')
    const effort = page.locator('.field-row').filter({ has: page.getByText('Reasoning effort', { exact: true }) })
    await expect(effort).toContainText('Inherited from shared')
    await expect(effort.getByRole('combobox', { name: 'Reasoning effort value', exact: true })).toHaveValue('high')
    await verbose.click()
    await expect(verbose).toHaveAttribute('aria-checked', 'false')
    expect(await readFile(f.settings, 'utf8')).toBe(sharedSettings)
    expect(await readFile(f.config, 'utf8')).toBe(sharedConfig)
    expect(await readFile(f.localSettings, 'utf8')).toContain('verbose: false')
    expect(await readFile(f.localSettings, 'utf8')).toContain('model: opus')
    await page.reload()
    await expect(page.getByRole('button', { name: 'Personal (not committed)', exact: true })).toHaveAttribute('aria-pressed', 'true')
    await expect(caveman).toHaveAttribute('aria-checked', 'true')
    await expect(verbose).toHaveAttribute('aria-checked', 'false')
    await page.getByRole('button', { name: 'Reset x-claude.verbose to shared value', exact: true }).click()
    await expect(verbose).toHaveAttribute('aria-checked', 'false')
    await expect.poll(() => readFile(f.localSettings, 'utf8')).not.toContain('verbose')
  } finally { await f.close() }
})

test('live project views, inherited settings, MCP cache, and builtin changes survive reload', async ({ page }, testInfo) => {
  const f = await fixture(false, [], true)
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
    const memoryPreview = page.getByRole('region', { name: 'Memory change preview' })
    await expect(memory).toHaveAttribute('aria-checked', 'false')
    await expect(memoryPreview).toContainText('.git/info/exclude')
    await memoryPreview.getByRole('button', { name: 'Apply change', exact: true }).click()
    await expect(memory).toHaveAttribute('aria-checked', 'true')
    await run(binary, ['sync', '--check'], { cwd: f.root, env: f.env })
    const status = await run('git', ['status', '--porcelain', '--untracked-files=all'], { cwd: f.root, env: f.env })
    expect(status.stdout).not.toContain('settings.local.json')
    await page.reload()
    await expect(memory).toHaveAttribute('aria-checked', 'true')
    await memory.click()
    await memoryPreview.getByRole('button', { name: 'Apply change', exact: true }).click()
    await expect(memory).toHaveAttribute('aria-checked', 'false')
    await run(binary, ['sync', '--check'], { cwd: f.root, env: f.env })
    expect(await readFile(f.config, 'utf8')).toBe(f.configBytes)

    await page.getByRole('button', { name: 'Personal (not committed)', exact: true }).click()
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

test('live personal built-in outside the shared block goes to info/exclude and resets cleanly', async ({ page }) => {
  const f = await fixture(false, [], true)
  try {
    await put(f.config, f.configBytes.replace('builtins: []', 'builtins: []\ngitignore:\n    enabled: true'))
    await run(binary, ['sync'], { cwd: f.root, env: f.env })
    const git = (...args: string[]) => run('git', ['-c', 'user.name=Test', '-c', 'user.email=test@example.com', '-c', 'commit.gpgsign=false', ...args], { cwd: f.root, env: f.env })
    await git('add', '-A')
    await git('commit', '--quiet', '-m', 'Team setup')
    const exclude = join(f.root, '.git', 'info', 'exclude')
    const personalBlock = '# >>> agnostic-ai (personal)'

    await page.goto(`${f.url}?view=settings`)
    await page.getByRole('button', { name: 'Personal (not committed)', exact: true }).click()
    const caveman = page.getByRole('switch', { name: 'caveman built-in' })
    await caveman.click()
    await page.getByRole('region', { name: 'Caveman change preview' }).getByRole('button', { name: 'Apply change', exact: true }).click()
    await expect(caveman).toHaveAttribute('aria-checked', 'true')
    await stat(join(f.root, '.claude', 'skills', 'caveman', 'SKILL.md'))
    expect((await git('status', '--porcelain', '--untracked-files=all')).stdout).toBe('')
    expect(await readFile(exclude, 'utf8')).toContain(personalBlock)

    await page.locator('.item-row').filter({ has: caveman }).getByRole('button', { name: 'Reset caveman to shared value' }).click()
    await page.getByRole('region', { name: 'Caveman change preview' }).getByRole('button', { name: 'Apply change', exact: true }).click()
    await expect(caveman).toHaveAttribute('aria-checked', 'false')
    await expect(stat(join(f.root, 'agnostic-ai.local.yaml'))).rejects.toThrow()
    expect((await git('status', '--porcelain', '--untracked-files=all')).stdout).toBe('')
    expect(await readFile(exclude, 'utf8').catch(() => '')).not.toContain(personalBlock)
  } finally {
    await f.close()
  }
})

test('live personal edits stay out of Git, reach sync, and reset to the shared value', async ({ page }) => {
  const f = await fixture(false, [], true)
  try {
    const shared = join(f.root, '.agnostic-ai', 'settings', 'defaults.yaml')
    const overlay = join(f.root, '.agnostic-ai', 'local', 'settings', 'defaults.yaml')
    await put(shared, 'name: defaults\ndescription: Team defaults.\nmodel: sonnet\n')
    await put(f.config, f.configBytes.replace('builtins: []', 'builtins: [handoff]\ngitignore:\n    enabled: true'))
    await run(binary, ['sync'], { cwd: f.root, env: f.env })
    const git = (...args: string[]) => run('git', ['-c', 'user.name=Test', '-c', 'user.email=test@example.com', '-c', 'commit.gpgsign=false', ...args], { cwd: f.root, env: f.env })
    await git('add', '-A')
    await git('commit', '--quiet', '-m', 'Team setup')

    await page.goto(`${f.url}?view=settings`)
    await page.getByRole('button', { name: 'Personal (not committed)', exact: true }).click()
    const model = page.locator('.field-row').filter({ has: page.getByText('Default model', { exact: true }) })
    await model.getByRole('textbox', { name: 'Default model value' }).fill('opus')
    await model.getByRole('button', { name: 'Preview and apply' }).click()
    await page.getByRole('region', { name: 'Default model change preview' }).getByRole('button', { name: 'Apply change' }).click()
    await expect(model.locator('.badge', { hasText: 'personal' })).toBeVisible()
    expect((await git('status', '--porcelain', '--untracked-files=all')).stdout).toBe('')
    const handoff = page.getByRole('switch', { name: 'handoff built-in' })
    await expect(handoff).toHaveAttribute('aria-checked', 'true')
    await handoff.click()
    const preview = page.getByRole('region', { name: 'Handoff change preview' })
    await expect(preview).toContainText('agnostic-ai.local.yaml')
    await preview.getByRole('button', { name: 'Apply change', exact: true }).click()
    await expect(handoff).toHaveAttribute('aria-checked', 'false')
    const handoffRow = page.locator('.item-row').filter({ has: handoff })
    await expect(handoffRow.locator('.badge', { hasText: 'personal' })).toBeVisible()

    expect((await git('status', '--porcelain', '--untracked-files=all')).stdout).toBe('')
    for (const personal of ['agnostic-ai.local.yaml', '.agnostic-ai/local/settings/defaults.yaml']) await git('check-ignore', '--quiet', personal)
    await run(binary, ['sync'], { cwd: f.root, env: f.env })
    expect(JSON.parse(await readFile(join(f.root, '.claude', 'settings.json'), 'utf8')).model).toBe('opus')
    await expect(stat(join(f.root, '.claude', 'skills', 'handoff', 'SKILL.md'))).rejects.toThrow()
    expect(await readFile(shared, 'utf8')).toBe('name: defaults\ndescription: Team defaults.\nmodel: sonnet\n')

    await page.getByRole('button', { name: 'Shared (committed)', exact: true }).click()
    await expect(model.getByRole('textbox', { name: 'Default model value' })).toHaveValue('sonnet')
    await expect(handoff).toHaveAttribute('aria-checked', 'true')
    await expect(handoff).toBeDisabled()
    await model.getByRole('button', { name: 'View personal settings' }).click()
    await model.getByRole('button', { name: 'Preview personal reset' }).click()
    await page.getByRole('region', { name: 'Default model change preview' }).getByRole('button', { name: 'Apply change' }).click()
    await expect(model.locator('.badge', { hasText: 'personal' })).toHaveCount(0)
    await expect(model.getByRole('textbox', { name: 'Default model value' })).toHaveValue('sonnet')
    await expect(stat(overlay)).rejects.toThrow()
    await handoffRow.getByRole('button', { name: 'Reset handoff to shared value' }).click()
    await preview.getByRole('button', { name: 'Apply change', exact: true }).click()
    await expect(handoff).toHaveAttribute('aria-checked', 'true')
    await expect(stat(join(f.root, 'agnostic-ai.local.yaml'))).rejects.toThrow()
    expect((await git('status', '--porcelain', '--untracked-files=all')).stdout).toBe('')
  } finally {
    await f.close()
  }
})

test('live first personal save before any ignore rule stays out of Git', async ({ page }) => {
  const f = await fixture(false, [], true)
  try {
    const overlay = '.agnostic-ai/local/settings/defaults.yaml'
    await rm(f.localSettings)
    await rm(join(f.root, '.gitignore'))
    await put(join(f.root, '.agnostic-ai', 'settings', 'defaults.yaml'), 'name: defaults\ndescription: Team defaults.\nmodel: sonnet\n')
    const git = (...args: string[]) => run('git', ['-c', 'user.name=Test', '-c', 'user.email=test@example.com', '-c', 'commit.gpgsign=false', ...args], { cwd: f.root, env: f.env })
    await git('add', '-A')
    await git('commit', '--quiet', '-m', 'Team setup')

    await page.goto(`${f.url}?view=settings`)
    await page.getByRole('button', { name: 'Personal (not committed)', exact: true }).click()
    const model = page.locator('.field-row').filter({ has: page.getByText('Default model', { exact: true }) })
    await model.getByRole('textbox', { name: 'Default model value' }).fill('opus')
    await model.getByRole('button', { name: 'Preview and apply' }).click()
    await expect(stat(join(f.root, overlay))).rejects.toThrow()
    await page.getByRole('region', { name: 'Default model change preview' }).getByRole('button', { name: 'Apply change' }).click()
    await expect(model.locator('.badge', { hasText: 'personal' })).toBeVisible()

    expect(await readFile(join(f.root, overlay), 'utf8')).toContain('model: opus')
    await git('check-ignore', '--quiet', overlay)
    await git('add', '-A')
    expect((await git('ls-files', '--', overlay)).stdout).toBe('')
  } finally {
    await f.close()
  }
})

test('live global edits keep the last local builtin disabled and enforce browser boundaries', async ({ page, request }) => {
  const f = await fixture(true)
  const nativeBefore = await projectFiles(f.userHome)
  try {
    await page.goto(`${f.url}?view=settings`)
    const memory = page.getByRole('switch', { name: 'memory built-in' })
    await expect(memory).toHaveAttribute('aria-checked', 'true')
    await expect(memory).toBeDisabled()
    await page.getByRole('button', { name: 'Personal (not committed)', exact: true }).click()
    await expect(page.getByText('Saves source files. Run agnostic-ai sync --global to apply them to your tools.', { exact: true })).toHaveCount(1)
    await memory.click()
    await expect(memory).toHaveAttribute('aria-checked', 'false')
    await page.reload()
    await expect(memory).toHaveAttribute('aria-checked', 'false')
    expect(await readFile(join(f.root, 'local', 'agnostic-ai.yaml'), 'utf8')).toContain('builtins: []')
    expect(await readFile(f.config, 'utf8')).toBe(f.configBytes.replace('builtins: []', 'builtins: [memory]'))

    expect(await projectFiles(f.userHome)).toEqual(nativeBefore)
    await expect(page.getByRole('button', { name: 'Apply change', exact: true })).toHaveCount(0)

    const edit = `${f.url}api/edit/builtin`
    const before = await readFile(join(f.root, 'local', 'agnostic-ai.yaml'), 'utf8')
    const foreign = await request.post(edit, { headers: { Origin: 'http://localhost:9876' }, data: { audience: 'shared', name: 'memory', enabled: true } })
    expect(foreign.status()).toBe(403)
    const wrongHost = await request.get(`${f.url}api/views/authority/schema`, { headers: { Host: 'attacker.example' } })
    expect(wrongHost.status()).toBe(421)
    const wrongType = await request.post(edit, { headers: { 'Content-Type': 'text/plain' }, data: '{}' })
    expect(wrongType.status()).toBe(415)
    const crossSite = await request.post(edit, { headers: { 'Sec-Fetch-Site': 'cross-site' }, data: { audience: 'shared', name: 'memory', enabled: true } })
    expect(crossSite.status()).toBe(403)
    const missingBoolean = await request.post(edit, { data: { audience: 'personal', name: 'memory' } })
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
    const refused = await request.post(`${f.url}api/edit/builtin`, { data: { audience: 'shared', name: 'memory', enabled: true } })
    expect(refused.status()).toBe(422)
    expect(JSON.stringify(await refused.json())).not.toContain(token)
    expect(await readFile(f.config, 'utf8')).toBe(invalid)
  } finally {
    await f.close()
  }
})

test('live global Tools lists a server held only in ~/.claude.json and none of its values', async ({ page, request }) => {
  const f = await fixture(true)
  const token = 'ghp_live_native_token_123'
  const claudeJSON = join(f.userHome, '.claude.json')
  const bytes = JSON.stringify({
    mcpServers: {
      notion: { type: 'http', url: 'https://mcp.notion.com/mcp' },
      private: { command: 'private-mcp', args: ['--token', token], env: { API_TOKEN: token }, headers: { Authorization: `Bearer ${token}` } },
    },
    projects: { [f.root]: { mcpServers: { local: { command: 'local-mcp' } } } },
  })
  try {
    await writeFile(claudeJSON, bytes, { mode: 0o600 })
    const response = await request.get(`${f.url}api/views/overview`)
    expect(response.status()).toBe(200)
    const body = JSON.stringify(await response.json())
    expect(body).not.toContain(token)
    expect(body).not.toContain('mcp.notion.com')
    await page.goto(`${f.url}?view=tools`)
    const block = page.getByRole('region', { name: /Found in a tool's own config/ })
    const row = block.locator('.item-row', { hasText: 'notion' })
    await expect(row).toContainText('Claude Code only')
    await expect(row).toContainText('~/.claude.json')
    await expect(row).toContainText('Run agnostic-ai import --global claude to make it a spec.')
    await expect(block.locator('.item-row', { hasText: 'private' })).toBeVisible()
    await expect(block.locator('.item-row', { hasText: 'local' })).toHaveCount(0)
    expect(await page.locator('body').innerText()).not.toContain(token)
    expect(await readFile(claudeJSON, 'utf8')).toBe(bytes)
    expect((await stat(claudeJSON)).mode & 0o777).toBe(0o600)
  } finally {
    await f.close()
  }
})

test('live global Tools makes one native server a spec and refuses credentials it cannot rewrite', async ({ page }) => {
  const f = await fixture(true)
  const token = 'ghp_live_spec_token_123456789'
  const claudeJSON = join(f.userHome, '.claude.json')
  const bytes = JSON.stringify({ mcpServers: {
    notion: { type: 'http', url: 'https://mcp.notion.com/mcp' },
    github: { command: 'github-mcp', env: { GITHUB_TOKEN: token } },
    private: { command: 'sh', args: ['-c', `exec npx mcp-remote https://x.example/mcp?token='${token}'`] },
  } })
  try {
    await writeFile(claudeJSON, bytes, { mode: 0o600 })
    await page.goto(`${f.url}?view=tools`)
    const block = page.getByRole('region', { name: /Found in a tool's own config/ })
    const notion = block.locator('.item-row', { hasText: 'notion' })
    await notion.getByRole('button', { name: 'Make it a spec' }).click()
    const preview = notion.getByRole('region', { name: 'notion spec preview' })
    await expect(preview).toContainText('mcps/notion.yaml')
    await expect(preview).toContainText('url: https://mcp.notion.com/mcp')
    await expect(preview).toContainText('After sync, this server reaches')
    await expect(readFile(join(f.root, 'mcps', 'notion.yaml'))).rejects.toThrow()
    await preview.getByRole('button', { name: 'Apply', exact: true }).click()
    await expect(notion).toHaveCount(0)
    await expect(page.getByRole('region', { name: /^MCP servers/ }).locator('.item-row', { hasText: 'notion' })).toBeVisible()
    const saved = await readFile(join(f.root, 'mcps', 'notion.yaml'), 'utf8')
    expect(saved).toContain('url: https://mcp.notion.com/mcp')
    expect(saved).toContain('type: http')
    expect(saved).not.toContain('command:')
    await page.reload()
    await expect(page.getByRole('status').filter({ hasText: 'Saved mcps/notion.yaml.' })).toContainText('Run agnostic-ai sync --global')

    const github = block.locator('.item-row', { hasText: 'github' })
    await github.getByRole('button', { name: 'Make it a spec' }).click()
    const githubPreview = github.getByRole('region', { name: 'github spec preview' })
    await expect(githubPreview).toContainText('GITHUB_TOKEN: ${GITHUB_TOKEN}')
    await expect(githubPreview).toContainText('Set these environment variables before syncing: GITHUB_TOKEN')
    expect(await githubPreview.innerText()).not.toContain(token)
    await githubPreview.getByRole('button', { name: 'Apply', exact: true }).click()
    await expect(github).toHaveCount(0)
    const githubSpec = await readFile(join(f.root, 'mcps', 'github.yaml'), 'utf8')
    expect(githubSpec).toContain('${GITHUB_TOKEN}')
    expect(githubSpec).not.toContain(token)

    const privateRow = block.locator('.item-row', { hasText: 'private' })
    await privateRow.getByRole('button', { name: 'Make it a spec' }).click()
    await expect(privateRow.getByRole('alert')).toContainText('credential import cannot rewrite')
    await expect(privateRow.getByRole('button', { name: 'Apply', exact: true })).toHaveCount(0)
    await expect(readFile(join(f.root, 'mcps', 'private.yaml'))).rejects.toThrow()
    expect(await page.locator('body').innerText()).not.toContain(token)
    expect(await readFile(claudeJSON, 'utf8')).toBe(bytes)
    expect((await stat(claudeJSON)).mode & 0o777).toBe(0o600)
    const dryRun = await run(binary, ['sync', '--global', '--dry-run'], { cwd: f.root, env: f.env })
    expect(dryRun.stdout).toContain('notion')
    expect(await readFile(claudeJSON, 'utf8')).toBe(bytes)
  } finally {
    await f.close()
  }
})

test('live project Tools writes a personal MCP spec and preserves native files', async ({ page }) => {
  const f = await fixture(false, [], true)
  const native = join(f.root, '.mcp.json')
  try {
    const before = await readFile(native, 'utf8')
    const contents = JSON.parse(before)
    contents.mcpServers.notion = { type: 'http', url: 'https://mcp.notion.com/mcp' }
    const bytes = JSON.stringify(contents)
    await writeFile(native, bytes)
    await page.goto(`${f.url}?view=tools`)
    await page.getByRole('button', { name: 'Personal (not committed)', exact: true }).click()
    const row = page.getByRole('region', { name: /Found in a tool's own config/ }).locator('.item-row', { hasText: 'notion' })
    await row.getByRole('button', { name: 'Make it a spec' }).click()
    const preview = row.getByRole('region', { name: 'notion spec preview' })
    await expect(preview).toContainText('.agnostic-ai/local/mcps/notion.yaml')
    await expect(preview).toContainText('for you only')
    await preview.getByRole('button', { name: 'Apply', exact: true }).click()
    await expect(row).toHaveCount(0)
    expect(await readFile(join(f.root, '.agnostic-ai', 'local', 'mcps', 'notion.yaml'), 'utf8')).toContain('type: http')
    expect(await readFile(native, 'utf8')).toBe(bytes)
    await run('git', ['check-ignore', '-q', '.agnostic-ai/local/mcps/notion.yaml'], { cwd: f.root, env: f.env })
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
    const memoryPreview = page.getByRole('region', { name: 'Memory change preview' })
    await expect(memory).toHaveAttribute('aria-checked', 'false')
    await memoryPreview.getByRole('button', { name: 'Apply change', exact: true }).click()
    await expect(memory).toHaveAttribute('aria-checked', 'true')
    await run(binary, ['sync', '--check'], { cwd: f.root, env: f.env })
    await page.reload()
    await expect(memory).toHaveAttribute('aria-checked', 'true')
    expect(await readFile(f.config, 'utf8')).toContain('memory')
  } finally {
    if (development) await stop(development)
    await f.close()
  }
})

test('Handoff then Caveman apply to every target and share one undo after reload', async ({ page }, testInfo) => {
  const f = await fixture()
  try {
    const tool = join(f.root, '.agnostic-ai', 'local', 'refactor-sync-guards', 'tools', 'bashunit')
    await mkdir(dirname(tool), { recursive: true })
    const toolSource = join(f.root, 'auxiliary-tool')
    await put(toolSource, '#!/bin/sh\necho test\n')
    await symlink(toolSource, tool)
    const config = f.configBytes.replace('- claude # team tool', '- claude # team tool\n    - codex')
    await put(f.config, config)
    await run(binary, ['sync'], { cwd: f.root, env: f.env })
    await page.goto(`${f.url}?view=settings`)
    const handoff = page.getByRole('switch', { name: 'handoff built-in', exact: true })
    await handoff.click()
    await page.getByRole('region', { name: 'Handoff change preview' }).getByRole('button', { name: 'Apply change', exact: true }).click()
    await expect(handoff).toHaveAttribute('aria-checked', 'true')
    for (const file of ['.claude/skills/handoff/SKILL.md', '.agents/skills/handoff/SKILL.md']) {
      expect(await readFile(join(f.root, file), 'utf8')).toContain('handoff')
    }
    await run(binary, ['sync', '--check'], { cwd: f.root, env: f.env })
    const afterHandoff = await readFile(f.config, 'utf8')
    const caveman = page.getByRole('switch', { name: 'caveman built-in', exact: true })
    await expect(caveman).toBeEnabled()
    await caveman.click()
    const preview = page.getByRole('region', { name: 'Caveman change preview' })
    await preview.getByText('Applies to 2 coding tools', { exact: true }).click()
    const tools = preview.locator('.preview-targets .tool-avatar')
    await expect(tools).toHaveCount(2)
    await expect(tools.first()).toHaveAttribute('title', 'Claude Code')
    await expect(tools.nth(1)).toHaveAttribute('title', 'Codex')
    await expect(preview).toContainText('.claude/skills/caveman/SKILL.md')
    await expect(preview).toContainText('.agents/skills/caveman/SKILL.md')
    expect(await readFile(f.config, 'utf8')).toBe(afterHandoff)
    await expect(page.getByRole('button', { name: 'Undo last change', exact: true })).toHaveCount(1)
    await preview.getByRole('button', { name: 'Cancel', exact: true }).click()
    await expect(page.getByRole('button', { name: 'Undo last change', exact: true })).toHaveCount(1)
    expect(await readFile(f.config, 'utf8')).toBe(afterHandoff)
    await caveman.click()
    await preview.getByText(/^Changed files \(/).click()
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
    await expect(page.getByRole('button', { name: 'Undo last change', exact: true })).toHaveCount(1)
    await page.getByRole('button', { name: 'Undo last change' }).click()
    await expect(caveman).toHaveAttribute('aria-checked', 'false')
    await expect(handoff).toHaveAttribute('aria-checked', 'true')
    await expect(page.getByRole('button', { name: 'Undo last change', exact: true })).toHaveCount(0)
    expect(await readFile(f.config, 'utf8')).toBe(afterHandoff)
    await run(binary, ['sync', '--check'], { cwd: f.root, env: f.env })
  } finally {
    await f.close()
  }
})

test('Caveman preview repairs pending Handoff outputs and undo restores the original drift', async ({ page, request }) => {
  const f = await fixture()
  try {
    const config = f.configBytes.replace('- claude # team tool', '- claude # team tool\n    - codex')
    await put(f.config, config)
    await run(binary, ['sync'], { cwd: f.root, env: f.env })
    const legacy = await request.post(`${f.url}api/edit/builtin`, { data: { audience: 'shared', name: 'handoff', enabled: true } })
    expect(legacy.status()).toBe(200)
    await put(f.localSettings, f.localBytes.replace('alwaysThinkingEnabled: true', 'alwaysThinkingEnabled: false'))
    const before = await projectFiles(f.root)
    expect(before['.claude/skills/handoff/SKILL.md']).toBeUndefined()
    expect(before['.agents/skills/handoff/SKILL.md']).toBeUndefined()
    const settingsBefore = await readFile(join(f.root, '.claude/settings.json'), 'utf8')
    expect(JSON.parse(settingsBefore).alwaysThinkingEnabled).toBe(true)

    await page.goto(`${f.url}?view=settings`)
    await page.getByRole('switch', { name: 'caveman built-in', exact: true }).click()
    const preview = page.getByRole('region', { name: 'Caveman change preview' })
    await expect(preview).toContainText('pending source changes')
    for (const file of ['.claude/skills/handoff/SKILL.md', '.agents/skills/handoff/SKILL.md', '.claude/settings.json']) {
      await expect(preview).toContainText(file)
    }
    expect(await projectFiles(f.root)).toEqual(before)
    await preview.getByRole('button', { name: 'Apply change', exact: true }).click()
    await expect(page.getByRole('switch', { name: 'caveman built-in', exact: true })).toHaveAttribute('aria-checked', 'true')
    for (const tool of ['.claude', '.agents']) {
      for (const name of ['handoff', 'caveman']) {
        expect(await readFile(join(f.root, tool, 'skills', name, 'SKILL.md'), 'utf8')).toContain(name)
      }
    }
    expect(JSON.parse(await readFile(join(f.root, '.claude/settings.json'), 'utf8')).alwaysThinkingEnabled).toBe(false)
    await run(binary, ['sync', '--check'], { cwd: f.root, env: f.env })
    await page.reload()
    await page.getByRole('button', { name: 'Undo last change', exact: true }).click()
    await expect(page.getByRole('switch', { name: 'caveman built-in', exact: true })).toHaveAttribute('aria-checked', 'false')
    await expect(page.getByRole('switch', { name: 'handoff built-in', exact: true })).toHaveAttribute('aria-checked', 'true')
    expect(await projectFiles(f.root)).toEqual(before)
  } finally {
    await f.close()
  }
})

test('a cancelled preview stays cancelled and keeps the previous undo through reload, navigation, and filtering', async ({ page }) => {
  const f = await fixture()
  try {
    const before = await projectFiles(f.root)
    await page.goto(`${f.url}?view=settings`)
    const handoff = page.getByRole('switch', { name: 'handoff built-in', exact: true })
    await handoff.click()
    await page.getByRole('region', { name: 'Handoff change preview' }).getByRole('button', { name: 'Apply change', exact: true }).click()
    await expect(handoff).toHaveAttribute('aria-checked', 'true')
    await page.getByRole('switch', { name: 'caveman built-in', exact: true }).click()
    await page.reload()
    const preview = page.getByRole('region', { name: 'Caveman change preview' })
    await expect(preview).toBeVisible()
    await expect(page.getByRole('region', { name: 'Handoff change preview' })).toHaveCount(0)
    await expect(page.getByRole('button', { name: 'Undo last change', exact: true })).toHaveCount(1)
    const search = page.getByRole('searchbox', { name: 'Search settings' })
    await search.fill('handoff')
    await expect(preview).toHaveCount(0)
    await search.fill('')
    await expect(preview).toBeVisible()
    await preview.getByRole('button', { name: 'Cancel', exact: true }).click()
    await expect(preview).toHaveCount(0)
    await page.reload()
    await expect(handoff).toBeEnabled()
    await expect(preview).toHaveCount(0)
    await page.getByRole('navigation').getByRole('link', { name: /^Tools/ }).click()
    await page.getByRole('navigation').getByRole('link', { name: /^Settings/ }).click()
    await expect(handoff).toBeEnabled()
    await expect(preview).toHaveCount(0)
    await page.getByRole('button', { name: 'Undo last change', exact: true }).click()
    await expect(handoff).toHaveAttribute('aria-checked', 'false')
    expect(await projectFiles(f.root)).toEqual(before)
    await run(binary, ['sync', '--check'], { cwd: f.root, env: f.env })
  } finally {
    await f.close()
  }
})

test('Caveman conflicts preserve later edits and explain how to recover', async ({ page }) => {
  const f = await fixture()
  try {
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

test('live Settings picks an unset effort from its levels, updates, and clears its new spec', async ({ page }) => {
  const f = await fixture()
  const created = join(f.root, '.agnostic-ai', 'settings', 'defaults.yaml')
  try {
    await rm(f.settings)
    await rm(f.localSettings)
    await page.goto(`${f.url}?view=settings`)
    await page.getByText('Unconfigured fields', { exact: false }).click()
    const effort = page.locator('.field-row').filter({ has: page.getByText('Reasoning effort', { exact: true }) })
    const level = effort.getByRole('combobox', { name: 'Reasoning effort value' })
    await expect(effort.getByRole('textbox', { name: 'Reasoning effort value' })).toHaveCount(0)
    await expect(level.locator('option', { hasText: /^low$/ })).toHaveCount(1)
    await level.selectOption('high')
    await effort.getByRole('button', { name: 'Preview and apply' }).click()
    await page.getByRole('region', { name: 'Reasoning effort change preview' }).getByRole('button', { name: 'Apply change' }).click()
    await expect(effort).toContainText('from .agnostic-ai/settings/defaults.yaml')
    expect(await readFile(created, 'utf8')).toContain('effort: high')
    await level.selectOption('low')
    await effort.getByRole('button', { name: 'Preview and apply' }).click()
    await page.getByRole('region', { name: 'Reasoning effort change preview' }).getByRole('button', { name: 'Apply change' }).click()
    await expect.poll(() => readFile(created, 'utf8')).toContain('effort: low')
    await effort.getByRole('button', { name: 'Preview removal' }).click()
    await page.getByRole('region', { name: 'Reasoning effort change preview' }).getByRole('button', { name: 'Apply change' }).click()
    await expect(effort).toContainText('Not set')
    await expect(readFile(created)).rejects.toThrow()
  } finally { await f.close() }
})

test('live Settings keeps a pack model visible while the editor targets the project spec', async ({ page }) => {
  const f = await fixture()
  const pack = '.agnostic-ai/packs/team/settings/team.yaml'
  try {
    await rm(f.settings)
    await rm(f.localSettings)
    await put(join(f.root, 'agnostic.packs.lock'), 'version: 1\npacks:\n  - name: team\n')
    await put(join(f.root, pack), 'name: team\nmodel: sonnet\n')
    await page.goto(`${f.url}?view=settings`)
    const model = page.locator('.field-row').filter({ has: page.getByText('Default model', { exact: true }) })
    await expect(model.locator('.field-source')).toContainText('sonnet')
    await expect(model.locator('.field-source')).toContainText(`from ${pack}`)
    const input = model.getByRole('textbox', { name: 'Default model value' })
    await expect(input).toHaveValue('')
    await expect(input).toHaveAttribute('placeholder', 'Not set in this file')
    await expect(model).toContainText('Saves to .agnostic-ai/settings/defaults.yaml')
    await input.fill('opus')
    await model.getByRole('button', { name: 'Preview and apply' }).click()
    await page.getByRole('region', { name: 'Default model change preview' }).getByRole('button', { name: 'Apply change' }).click()
    await expect(model.locator('.field-source')).toHaveCount(1)
    await expect(model.locator('.field-source')).toContainText(`from ${pack}`)
    await expect(input).toHaveValue('opus')
    await expect(model).toContainText('from .agnostic-ai/settings/defaults.yaml')
  } finally { await f.close() }
})

test('live Settings previews, cancels, applies, and undoes an effort change for Claude Code and Codex', async ({ page }) => {
  const f = await fixture()
  try {
    await put(f.config, f.configBytes.replace('    - claude # team tool\n', '    - claude # team tool\n    - codex\n'))
    await run(binary, ['sync'], { cwd: f.root, env: f.env })
    const before = await projectFiles(f.root)
    await page.goto(`${f.url}?view=settings`)
    await page.getByText('Unconfigured fields', { exact: false }).click()
    const effort = page.locator('.field-row').filter({ has: page.getByText('Reasoning effort', { exact: true }) })
    await effort.getByRole('combobox', { name: 'Reasoning effort value' }).selectOption('high')
    await expect(effort.getByRole('button', { name: 'Save', exact: true })).toHaveCount(0)
    await expect(effort.getByRole('button', { name: 'Update', exact: true })).toHaveCount(0)
    await expect(effort.getByRole('button', { name: 'Clear', exact: true })).toHaveCount(0)
    await effort.getByRole('button', { name: 'Preview and apply' }).click()
    const preview = page.getByRole('region', { name: 'Reasoning effort change preview' })
    await expect(preview).toContainText('Effort set to high')
    for (const path of ['.agnostic-ai/settings/native.yaml', '.claude/settings.json', '.codex/config.toml', '.agnostic-ai/.sync-state']) {
      await expect(preview).toContainText(path)
    }
    expect(await projectFiles(f.root)).toEqual(before)
    await preview.getByRole('button', { name: 'Cancel', exact: true }).click()
    await expect(preview).toHaveCount(0)
    expect(await projectFiles(f.root)).toEqual(before)

    await effort.getByRole('button', { name: 'Preview and apply' }).click()
    await expect(preview).toBeVisible()
    await page.reload()
    await page.getByText('Unconfigured fields', { exact: false }).click()
    await preview.getByRole('button', { name: 'Apply change', exact: true }).click()
    await expect(page.getByText('Last applied: effort set to high.')).toBeVisible()
    await expect(effort.getByRole('combobox', { name: 'Reasoning effort value' })).toHaveValue('high')
    await run(binary, ['sync', '--check'], { cwd: f.root, env: f.env })
    expect(await readFile(join(f.root, '.codex', 'config.toml'), 'utf8')).toContain('model_reasoning_effort = "high"')

    await page.getByRole('button', { name: 'Undo last change' }).click()
    await expect(page.getByText('Previous files restored.')).toBeVisible()
    expect(await projectFiles(f.root)).toEqual(before)
  } finally { await f.close() }
})

test('live Settings explains which model each tool gets from shared, personal, and per-tool settings', async ({ page }) => {
  const f = await fixture()
  try {
    await put(f.config, f.configBytes.replace('    - claude # team tool\n', '    - claude # team tool\n    - codex\n'))
    await put(f.settings, 'name: native\nmodel: opus\neffort: high\n')
    await put(f.localSettings, 'name: native\nmodel: sonnet\n')
    await put(join(f.root, '.agnostic-ai', 'settings', 'z-codex.yaml'), 'name: z-codex\ntargets: [codex]\nmodel: gpt-5.5\n')
    await run(binary, ['sync'], { cwd: f.root, env: f.env })
    await page.goto(`${f.url}?view=settings`)
    const model = page.locator('.field-row').filter({ has: page.getByText('Default model', { exact: true }) })
    await model.getByText('What each tool gets').click()
    const groups = model.getByRole('list', { name: 'Default model by tool' })
    await expect(groups).toContainText('Claude Code: sonnet')
    await expect(groups).toContainText('Codex: gpt-5.5')

    const detail = model.locator('.field-resolution-detail')
    await expect(detail).toContainText('Sync writes sonnet.')
    await expect(detail).toContainText('From .agnostic-ai/local/settings/native.yaml')
    await expect(detail).toContainText('opus in .agnostic-ai/settings/native.yaml: Your personal file .agnostic-ai/local/settings/native.yaml overrides it.')
    await expect(detail).toContainText('in .agnostic-ai/settings/z-codex.yaml: This spec does not apply to Claude Code.')
    await expect(detail).toContainText('Installed Claude Code files match what sync would write.')
    await expect(detail).toContainText('agnostic-ai does not see what a running tool uses.')

    await model.getByRole('combobox', { name: 'Tool' }).selectOption('codex')
    await expect(detail).toContainText('Sync writes gpt-5.5.')
    await expect(detail).toContainText('sonnet in .agnostic-ai/local/settings/native.yaml: It is a Claude model name, which Codex cannot load, so sync leaves it out.')
    await page.getByRole('button', { name: 'Personal (not committed)', exact: true }).click()
    await expect(model.getByRole('button', { name: 'Preview personal reset', exact: true })).toBeEnabled()

    const effort = page.locator('.field-row').filter({ has: page.getByText('Reasoning effort', { exact: true }) })
    await effort.getByText('What each tool gets').click()
    await expect(effort.getByRole('list', { name: 'Reasoning effort by tool' })).toContainText('Claude Code, Codex: high')
  } finally { await f.close() }
})

test('live Settings rejects a stale preview and keeps the draft until reloaded', async ({ page }) => {
  const f = await fixture()
  try {
    await put(f.settings, 'name: native\nmodel: sonnet\neffort: high\n')
    await rm(f.localSettings)
    await run(binary, ['sync'], { cwd: f.root, env: f.env })
    await page.goto(`${f.url}?view=settings`)
    const model = page.locator('.field-row').filter({ has: page.getByText('Default model', { exact: true }) })
    const input = model.getByRole('textbox', { name: 'Default model value' })
    await expect(input).toHaveValue('sonnet')
    await put(f.settings, 'name: native\nmodel: opus\neffort: high\n')
    await input.fill('haiku')
    await model.getByRole('button', { name: 'Preview and apply' }).click()
    await expect(model.getByRole('alert')).toContainText('model changed after this page loaded and is now opus')
    expect(await readFile(f.settings, 'utf8')).toBe('name: native\nmodel: opus\neffort: high\n')
    await expect(input).toHaveValue('haiku')
    await page.route('**/api/views/overview', (route) => route.abort())
    await model.getByRole('button', { name: 'Reload saved values' }).click()
    const failed = page.getByRole('alert').filter({ hasText: 'The overview could not be refreshed.' })
    await expect(failed).toBeVisible()
    await expect(failed).not.toContainText('Your change was saved')
    await expect(input).toHaveValue('haiku')
    await page.unroute('**/api/views/overview')
    await failed.getByRole('button', { name: 'Retry' }).click()
    await expect(model.getByRole('status')).toContainText('is now opus. Your draft is kept')
    await expect(input).toHaveValue('haiku')
    await model.getByRole('button', { name: 'Preview and apply' }).click()
    const preview = page.getByRole('region', { name: 'Default model change preview' })
    await expect(preview).toBeVisible()
    expect(await readFile(f.settings, 'utf8')).toBe('name: native\nmodel: opus\neffort: high\n')
    await preview.getByRole('button', { name: 'Apply change' }).click()
    await expect.poll(() => readFile(f.settings, 'utf8')).toBe('name: native\nmodel: haiku\neffort: high\n')
    await run(binary, ['sync', '--check'], { cwd: f.root, env: f.env })
  } finally { await f.close() }
})

test('live Settings rejects a stale personal preview from another tab', async ({ page, browser }) => {
  const f = await fixture()
  const other = await browser.newPage()
  try {
    await put(f.settings, 'name: native\nmodel: sonnet\n')
    await rm(f.localSettings)
    await run(binary, ['sync'], { cwd: f.root, env: f.env })
    for (const tab of [page, other]) {
      await tab.goto(`${f.url}?view=settings`)
      await tab.getByRole('button', { name: 'Personal (not committed)', exact: true }).click()
    }
    const model = (tab: typeof page) => tab.locator('.field-row').filter({ has: tab.getByText('Default model', { exact: true }) })
    await model(other).getByRole('textbox', { name: 'Default model value' }).fill('haiku')
    await model(other).getByRole('button', { name: 'Preview and apply' }).click()
    await other.getByRole('region', { name: 'Default model change preview' }).getByRole('button', { name: 'Apply change' }).click()
    await expect(model(other).locator('.badge', { hasText: 'personal' })).toBeVisible()
    await model(page).getByRole('textbox', { name: 'Default model value' }).fill('opus')
    await model(page).getByRole('button', { name: 'Preview and apply' }).click()
    await expect(model(page).getByRole('alert')).toContainText('is now haiku')
    expect(await readFile(f.localSettings, 'utf8')).toContain('model: haiku')
    await expect(model(page).getByRole('textbox', { name: 'Default model value' })).toHaveValue('opus')
  } finally { await other.close(); await f.close() }
})

test('live Settings keeps a draft when a delayed overview contains a newer value', async ({ page }) => {
  const f = await fixture()
  try {
    await put(f.settings, 'name: native\nmodel: sonnet\neffort: high\n')
    await rm(f.localSettings)
    await run(binary, ['sync'], { cwd: f.root, env: f.env })
    await page.goto(`${f.url}?view=settings`)
    const model = page.locator('.field-row').filter({ has: page.getByText('Default model', { exact: true }) })
    const input = model.getByRole('textbox', { name: 'Default model value' })
    await input.fill('haiku')
    let release = () => {}
    const held = new Promise<void>((resolve) => { release = resolve })
    await page.route('**/api/views/overview', async (route) => { await held; await route.continue() })
    const effort = page.locator('.field-row').filter({ has: page.getByText('Reasoning effort', { exact: true }) })
    await effort.getByRole('combobox', { name: 'Reasoning effort value' }).selectOption('low')
    await effort.getByRole('button', { name: 'Preview and apply' }).click()
    await page.getByRole('region', { name: 'Reasoning effort change preview' }).getByRole('button', { name: 'Apply change' }).click()
    await expect.poll(() => readFile(f.settings, 'utf8')).toBe('name: native\nmodel: sonnet\neffort: low\n')
    await put(f.settings, 'name: native\nmodel: opus\neffort: low\n')
    const refreshed = page.waitForResponse('**/api/views/overview')
    release()
    await refreshed
    await page.unroute('**/api/views/overview')
    await expect(input).toHaveValue('haiku')
    await model.getByRole('button', { name: 'Preview and apply' }).click()
    await expect(model.getByRole('alert')).toContainText('is now opus')
    expect(await readFile(f.settings, 'utf8')).toBe('name: native\nmodel: opus\neffort: low\n')
    await expect(input).toHaveValue('haiku')
  } finally { await f.close() }
})

test('live finding opens its file on the server disk in the chosen editor', async ({ page }) => {
  const f = await fixture()
  try {
    const permissions = join(f.root, '.agnostic-ai', 'settings', 'permissions.yaml')
    await put(permissions, 'permissions:\n  allow: [read]\n')
    await page.goto(`${f.url}?view=authority`)
    const finding = page.locator('.finding', { hasText: 'LINT038' })
    const open = finding.getByRole('link', { name: /^Open .+ in Cursor$/ })
    const href = await open.getAttribute('href')
    expect(href).toMatch(/^cursor:\/\/file\/.+\/\.agnostic-ai\/settings\/permissions\.yaml$/)
    expect(await stat(fileURLToPath(`file://${new URL(href!).pathname}`))).toBeTruthy()
    await page.getByRole('group', { name: 'Open files in' }).getByRole('button', { name: 'VS Code' }).click()
    await expect(finding.getByRole('link', { name: /^Open .+ in VS Code$/ })).toHaveAttribute('href', href!.replace(/^cursor:/, 'vscode:'))
    await expect(finding.getByRole('button', { name: /^Copy path of / })).toBeVisible()
  } finally {
    await f.close()
  }
})

test('live Recheck tells a fixed finding from a remaining, new, or unchecked one without leaving the page', async ({ page }) => {
  const f = await fixture()
  try {
    const permissions = join(f.root, '.agnostic-ai', 'settings', 'permissions.yaml')
    await put(permissions, 'permissions:\n  allow: [read, edit]\n')
    await page.goto(`${f.url}?view=authority`)
    const finding = (word: string) => page.locator('.finding:not(.finding-rechecked)', { hasText: `bare ${word} now covers` })
    const read = finding('read')
    await expect(read.getByRole('link', { name: /^Open .+ in Cursor$/ })).toBeVisible()
    await expect(read.getByRole('button', { name: 'Copy fix prompt' })).toBeVisible()
    await page.getByRole('searchbox').fill('permissions')
    const url = page.url()

    await put(permissions, 'permissions:\n  allow: ["read(src/**)", edit, shell]\n')
    await read.getByRole('button', { name: 'Recheck' }).click()
    const gone = page.locator('.finding-rechecked')
    await expect(gone.getByRole('status')).toHaveText('This lint finding no longer appears. 1 new lint finding appears in .agnostic-ai/settings/permissions.yaml.')
    await expect(gone).toBeFocused()
    await expect(read).toHaveCount(0)
    await expect(finding('shell').getByText('New since recheck')).toBeVisible()
    await expect(page.getByRole('searchbox')).toHaveValue('permissions')
    expect(page.url()).toBe(url)

    const edit = finding('edit')
    await edit.getByRole('button', { name: 'Recheck' }).click()
    await expect(edit.locator('.finding-recheck-result')).toHaveText('This finding still appears after the recheck.')
    await expect(gone).toHaveCount(0)

    await page.route('**/api/views/overview', (route) => route.abort())
    await edit.getByRole('button', { name: 'Recheck' }).click()
    await expect(page.getByRole('alert').filter({ hasText: 'The overview could not be refreshed.' })).toBeVisible()
    await expect(edit.locator('.finding-recheck-result')).toHaveText('The recheck did not finish, so this finding may still apply.')
    await expect(edit).toContainText('bare edit now covers every file edit')

    await page.unroute('**/api/views/overview')
    await page.getByRole('alert').getByRole('button', { name: 'Retry' }).click()
    await expect(page.getByRole('alert')).toHaveCount(0)
    await expect(edit.locator('.finding-recheck-result')).toHaveText('')

    await rm(permissions)
    await edit.getByRole('button', { name: 'Recheck' }).click()
    const other = page.locator('section.block', { has: page.getByRole('heading', { name: 'Other findings' }) })
    await expect(other.locator('.finding-rechecked .finding-recheck-result')).toHaveText('This lint finding no longer appears.')
    await expect(other.locator('.finding-rechecked')).toBeFocused()
  } finally { await f.close() }
})
