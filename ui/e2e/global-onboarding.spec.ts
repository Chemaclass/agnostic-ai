import { expect, test } from '@playwright/test'
import type { ChildProcess } from 'node:child_process'
import { spawn } from 'node:child_process'
import { access, mkdir, mkdtemp, readFile, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const uiRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..')
const binary = resolve(process.env.AGNOSTIC_E2E_BIN ?? join(uiRoot, '..', process.platform === 'win32' ? 'agnostic-ai.exe' : 'agnostic-ai'))
// A private port, away from the default 4310 range and the preview port.
const PORT = 4274
const URL = `http://127.0.0.1:${PORT}/`

async function exists(path: string) {
  try {
    await access(path)
    return true
  } catch {
    return false
  }
}

async function stop(child: ChildProcess) {
  if (child.exitCode !== null || child.signalCode !== null) return
  await new Promise<void>((done) => {
    const timer = setTimeout(() => child.kill('SIGKILL'), 3_000)
    child.once('exit', () => { clearTimeout(timer); done() })
    child.kill('SIGTERM')
  })
}

async function waitForServer(child: ChildProcess) {
  await new Promise<void>((accept, reject) => {
    let output = ''
    const timer = setTimeout(() => reject(new Error(`Server did not start: ${output}`)), 15_000)
    child.once('exit', (code) => { clearTimeout(timer); reject(new Error(`Server exited ${code}: ${output}`)) })
    child.stderr?.on('data', (data) => { output += data.toString() })
    child.stdout?.on('data', (data) => {
      output += data.toString()
      if (output.includes(URL)) { clearTimeout(timer); accept() }
    })
  })
}

test('a project page offers to create the missing global setup and then opens it', async ({ page, context }) => {
  const temporary = await mkdtemp(join(tmpdir(), 'agnostic-ui-onboarding-'))
  const project = join(temporary, 'project')
  const home = join(temporary, 'global')
  const userHome = join(temporary, 'user')
  let child: ChildProcess | undefined
  try {
    await mkdir(userHome, { recursive: true })
    await mkdir(project, { recursive: true })
    await writeFile(join(project, 'agnostic-ai.yaml'), 'version: 1\ntargets:\n    - claude\nbuiltins: []\n')
    child = spawn(binary, ['serve', '--addr', `127.0.0.1:${PORT}`, '--dir', project], {
      cwd: temporary,
      env: {
        ...process.env,
        AGNOSTIC_AI_HOME: home,
        AGNOSTIC_AI_NO_UPDATE_CHECK: '1',
        AGNOSTIC_AI_TARGET: '',
        HOME: userHome,
        USERPROFILE: userHome,
        XDG_CACHE_HOME: join(temporary, 'cache'),
        LOCALAPPDATA: join(temporary, 'cache'),
        NO_COLOR: '1',
      },
      stdio: ['ignore', 'pipe', 'pipe'],
    })
    await waitForServer(child)

    // The first switch reaches the real server, which starts nothing for a missing home.
    // The second would start a server on the default range, so the test answers it.
    let switches = 0
    await page.route('**/api/switch', async (route) => {
      switches += 1
      if (switches === 1) return route.continue()
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ url: URL, root: home, global: true, started: false }) })
    })

    await page.goto(URL)
    await page.getByRole('button', { name: 'Open global settings' }).click()

    const panel = page.getByRole('dialog', { name: 'No global setup yet' })
    await expect(panel).toBeVisible()
    await expect(panel).toContainText('One folder for your own rules, skills, agents, hooks, settings and MCP servers.')
    await expect(panel).toContainText(home)
    await expect(panel.getByRole('checkbox', { name: 'Also run git init' })).toBeChecked()
    expect(await exists(home)).toBe(false)
    // The blank tab opened before the request is closed, so the panel is in the tab the user sees.
    await expect.poll(() => context.pages().length).toBe(1)
    await expect(panel.getByRole('button', { name: 'Create global setup' })).toBeFocused()
    await expect(page.getByRole('button', { name: 'Open global settings' })).toBeDisabled()

    const popup = page.waitForEvent('popup')
    await panel.getByRole('button', { name: 'Create global setup' }).click()
    const tab = await popup
    await expect(panel).toBeHidden()
    await expect(page.locator('.config-switch').getByRole('status')).toHaveText(`Created ${home}. Add files, then run "agnostic-ai sync --global".`)
    await expect.poll(() => tab.url()).toBe(URL)

    expect(await readFile(join(home, 'agnostic-ai.yaml'), 'utf8')).toBe('version: 1\n')
    expect(await readFile(join(home, '.gitignore'), 'utf8')).toBe('/local/\n')
    expect(await readFile(join(home, 'AGNOSTIC_AI.md'), 'utf8')).toBe('')
    expect(context.pages().length).toBe(2)
    expect(switches).toBe(2)
    await context.close()
  } finally {
    if (child) await stop(child)
    if (!temporary.startsWith(join(tmpdir(), 'agnostic-ui-onboarding-'))) throw new Error('Unexpected fixture directory')
    await rm(temporary, { recursive: true, force: true })
  }
})
