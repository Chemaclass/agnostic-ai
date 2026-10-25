import { expect, test } from '@playwright/test'
import { readFileSync } from 'node:fs'
import { MOCK, openApp, saveScreenshot } from './helpers'

const fixture = JSON.parse(readFileSync(new URL('../src/fixtures/overview.mock.json', import.meta.url), 'utf8'))

test.describe('overview', () => {
  test('shows the brand once and keeps other project names as plain context', async ({ page }) => {
    const data = structuredClone(fixture)
    data.snapshot.project.name = 'agnostic-ai'
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: data }))
    await page.goto('/?view=context')
    await expect(page.locator('.support-table')).toBeVisible()
    await expect(page.locator('.brand-name')).toHaveText('agnostic-ai')
    await expect(page.locator('.project-name')).toHaveCount(0)
    await expect(page.locator('.topbar-context')).toHaveCount(0)
    data.snapshot.project.name = 'billing-service'
    await page.reload()
    await expect(page.locator('.project-name')).toHaveText('Project billing-service')
    await expect(page.locator('.project-name')).not.toHaveClass(/chip/)
  })

  test('loads a logo for every registered target', async ({ page }) => {
    const data = structuredClone(fixture)
    const capabilities = readFileSync(new URL('../../docs/site/data/capabilities.toml', import.meta.url), 'utf8')
    const targets = capabilities.split('[[targets]]').slice(1).map((block) => ({
      target: block.match(/id = "([^"]+)"/)?.[1],
      label: block.match(/name = "([^"]+)"/)?.[1],
      event: 'SessionStart',
    }))
    expect(targets.length).toBeGreaterThan(3)
    data.sections.find((section: { id: string }) => section.id === 'lifecycle').stages[0].targets = targets
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: data }))
    await page.goto('/?view=lifecycle')
    const avatars = page.locator('.stage').first().locator('.tool-avatar')
    await expect(avatars).toHaveCount(targets.length)
    for (const target of targets) {
      const avatar = page.locator('.stage').first().locator(`.tool-avatar[data-target="${target.target}"]`)
      await expect(avatar).toHaveAttribute('title', `${target.label} - SessionStart`)
      const image = avatar.locator('img')
      await expect(image).toBeVisible()
      await expect.poll(() => image.evaluate((element: HTMLImageElement) => element.naturalWidth)).toBeGreaterThan(0)
    }
  })

  test('uses named tool logos in every support table and configured target list', async ({ page }) => {
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: fixture }))
    for (const view of ['context', 'authority', 'tools', 'lifecycle', 'settings']) {
      await page.goto(`/?view=${view}`)
      const tools = page.locator('.support-table .tool-avatar')
      await expect(tools.first()).toBeVisible()
      for (const avatar of await tools.all()) {
        await expect(avatar).toHaveAttribute('title', /Claude Code|Codex|Cursor/)
        await expect(avatar.locator('img')).toBeVisible()
      }
    }
    const targets = page.locator('.value-row', { hasText: 'targets' }).locator('.tool-avatar')
    await expect(targets).toHaveCount(fixture.targets.length)
    await expect(targets.first()).toHaveAttribute('title', 'Claude Code')
  })

  test('groups native keys, keeps edit keys, and copies a finding fix prompt', async ({ page, context }) => {
    const data = structuredClone(fixture)
    const settings = data.sections.find((section: { id: string }) => section.id === 'settings')
    settings.fields.find((field: { key: string }) => field.key === 'x-').set = [{
      spec: 'native', path: '.agnostic-ai/settings/native.yaml', values: [
        { key: 'x-ellipsis.environment.repository.name', type: 'text', value: 'agnostic-ai' },
        { key: 'x-ellipsis.pull_requests.draft', type: 'bool', value: 'false' },
      ],
    }]
    settings.findings = [{
      source: 'lint', severity: 'warn', code: 'LINT038', path: '.agnostic-ai/settings/permissions.yaml', line: 3,
      message: 'Bare read covers every file.', explanation: 'This rule grants all file reads.', fix: 'Use read(src/**).',
    }]
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: data }))
    let edit: unknown
    await page.route('**/api/edit/value', (route) => {
      edit = route.request().postDataJSON()
      return route.fulfill({ json: { ok: true } })
    })
    await page.goto('/?view=settings')
    const native = page.locator('.native-group', { hasText: 'x-ellipsis' })
    await expect(native.locator('.native-group-title')).toHaveText('x-ellipsis')
    await expect(native.locator('dt').first()).toHaveText('environment.repository.name')
    await native.getByRole('switch', { name: 'x-ellipsis.pull_requests.draft' }).click()
    expect(edit).toEqual({ path: '.agnostic-ai/settings/native.yaml', key: 'x-ellipsis.pull_requests.draft', value: true })
    const finding = page.locator('.finding', { hasText: 'LINT038' })
    await expect(finding.getByText('This rule grants all file reads.', { exact: true })).not.toBeVisible()
    await finding.getByText('Details', { exact: true }).click()
    await expect(finding.getByText('This rule grants all file reads.', { exact: true })).toBeVisible()
    await expect(finding.getByText('Use read(src/**).', { exact: true })).toBeVisible()
    const preview = finding.getByRole('textbox', { name: 'Fix prompt', exact: true })
    await expect(preview).toHaveJSProperty('readOnly', true)
    expect(await preview.inputValue()).toContain('Suggested fix: Use read(src/**).')
    await context.grantPermissions(['clipboard-read', 'clipboard-write'])
    await finding.getByRole('button', { name: 'Copy fix prompt' }).click()
    await expect(finding.getByRole('button', { name: 'Copied', exact: true })).toBeVisible()
    const prompt = await page.evaluate(() => navigator.clipboard.readText())
    expect(prompt).toBe(await preview.inputValue())
    expect(prompt).toContain('LINT038')
    expect(prompt).toContain(`Configuration root: ${data.snapshot.project.root}`)
    expect(prompt).toContain('.agnostic-ai/settings/permissions.yaml:3')
    expect(prompt).toContain('Use read(src/**).')
    expect(prompt).toContain('sync --check')
  })

  test('shows the fix prompt when a finding has no explanation or suggested fix', async ({ page }) => {
    const data = structuredClone(fixture)
    data.sections.find((section: { id: string }) => section.id === 'settings').findings = [{
      source: 'doctor', severity: 'warn', message: 'The selected tool is unavailable.',
    }]
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: data }))
    await page.goto('/?view=settings')
    const finding = page.locator('.finding', { hasText: 'The selected tool is unavailable.' })
    await finding.getByText('Details', { exact: true }).click()
    await expect(finding.getByRole('textbox', { name: 'Fix prompt', exact: true })).toHaveValue(/The selected tool is unavailable[.]/)
    await expect(finding.getByText('Suggested fix', { exact: true })).toHaveCount(0)
  })

  test('expands long findings and copies the complete message while collapsed', async ({ page, context }) => {
    const data = structuredClone(fixture)
    const message = 'The session-start hook keeps the first 6000 bytes of the memory indexes. ' +
      'These entries exceed the current budget: ' + Array.from({ length: 24 }, (_, i) => `project-learning-${i + 1}.md`).join(', ') + '. Shorten the index to include these entries.'
    data.sections.find((section: { id: string }) => section.id === 'settings').findings = [
      { source: 'lint', severity: 'warn', code: 'LINT039', message, path: '.agnostic-ai/local/memory/MEMORY.md' },
      { source: 'doctor', severity: 'error', message: 'The configuration file cannot be read.' },
    ]
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: data }))
    await context.grantPermissions(['clipboard-read', 'clipboard-write'])
    await page.goto('/?view=settings')
    const finding = page.locator('.finding', { hasText: 'LINT039' })
    const text = finding.locator('.finding-message')
    const expand = finding.getByRole('button', { name: 'Show full message', exact: true })
    await expect(expand).toHaveAttribute('aria-expanded', 'false')
    const collapsedHeight = await text.evaluate((element) => element.clientHeight)
    expect(await text.evaluate((element) => element.scrollHeight)).toBeGreaterThan(collapsedHeight)
    await finding.getByRole('button', { name: 'Copy fix prompt' }).click()
    expect(await page.evaluate(() => navigator.clipboard.readText())).toContain(message)
    await expand.click()
    await expect(finding.getByRole('button', { name: 'Show less' })).toHaveAttribute('aria-expanded', 'true')
    expect(await text.evaluate((element) => element.clientHeight)).toBeGreaterThan(collapsedHeight)
    await expect(text).toHaveText(message)
    await finding.getByRole('button', { name: 'Show less' }).click()
    await page.setViewportSize({ width: 390, height: 844 })
    await expand.click()
    expect(await page.locator('#main').evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true)
    const short = page.locator('.finding-error')
    await expect(short.getByRole('button', { name: 'Show full message' })).toHaveCount(0)
    await expect(short.locator('.finding-message')).toHaveText('The configuration file cannot be read.')
  })

  test('keeps configured settings visible and reveals unset fields on demand', async ({ page }) => {
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: fixture }))
    await page.goto('/?view=authority')
    const unset = page.locator('.field-row', { hasText: 'Default permission mode' })
    await expect(unset).not.toBeVisible()
    await expect(page.locator('.field-row.is-set').first()).toBeVisible()
    await page.locator('.unconfigured-fields > summary').click()
    await expect(unset).toBeVisible()
    await expect(unset).toContainText('Not set')
  })

  test('keeps controls before warning details and links to the findings', async ({ page }) => {
    const data = structuredClone(fixture)
    data.sections.find((section: { id: string }) => section.id === 'settings').findings = [{
      source: 'lint', severity: 'warn', code: 'LINT011', message: 'The session context exceeds its budget.',
    }]
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: data }))
    await page.setViewportSize({ width: 390, height: 844 })
    await page.goto('/?view=settings')
    const review = page.getByRole('button', { name: 'Review 1 finding', exact: true })
    await expect(review).toBeInViewport()
    await expect(page.getByRole('heading', { name: /^Built-ins/, level: 2 })).toBeInViewport()
    await review.click()
    await expect(page.getByRole('heading', { name: 'Other findings', exact: true })).toBeFocused()
    await expect(page.getByText('The session context exceeds its budget.', { exact: true })).toBeInViewport()
    await expect(page.locator('.topbar')).toBeInViewport({ ratio: 1 })
    await expect(page.locator('.sidenav')).toBeInViewport({ ratio: 1 })
  })

  test('stacks mobile tool support groups without losing table labels', async ({ page }) => {
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: fixture }))
    await page.setViewportSize({ width: 390, height: 844 })
    await page.goto('/?view=tools')
    const table = page.getByRole('table', { name: 'Coding tool support' })
    await expect(table).toBeVisible()
    await expect(table.getByRole('columnheader', { name: 'Supported on', exact: true })).toHaveCount(1)
    const status = table.locator('tbody td').first()
    await expect(status.locator('.support-status-label')).toBeVisible()
    expect((await status.boundingBox())?.width).toBeGreaterThan(300)
    expect(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)).toBe(false)
  })

  test('opens on Context with five sections and an icon theme toggle', async ({ page }) => {
    await openApp(page)
    await expect(page.getByRole('heading', { level: 1, name: 'Context' })).toBeVisible()
    await expect(page.locator('.page-subtitle')).toHaveText('What is the agent told?')
    for (const kind of ['Instructions', 'Rules', 'Agents', 'Skills']) await expect(page.getByRole('heading', { level: 2, name: new RegExp(`^${kind}`) })).toBeVisible()
    const nav = page.getByRole('navigation', { name: 'Configuration sections' })
    await expect(nav.getByRole('link')).toHaveCount(5)
    for (const name of ['Context', 'Authority', 'Tools', 'Lifecycle', 'Settings']) await expect(nav.getByRole('link', { name: new RegExp(`^${name}`) })).toBeVisible()
    await expect(page.locator('.topbar .chip', { hasText: 'sync' })).toHaveCount(0)
    const theme = page.getByRole('button', { name: /^Switch to (light|dark) theme$/ })
    const before = await theme.getAttribute('aria-label')
    await theme.click()
    await expect(theme).not.toHaveAttribute('aria-label', before ?? '')
    await expect(page.getByRole('combobox')).toHaveCount(0)
    const open = page.locator('.item-row', { hasText: 'agents/' }).first().getByRole('link', { name: /^Edit .+ in Cursor$/ })
    await expect(open).toHaveAttribute('href', /^cursor:\/\/file\/.+\.md$/)
    await saveScreenshot(page, '01-context.png')
  })

  test('lists every lifecycle stage in session order, even with no hooks', async ({ page }) => {
    await openApp(page, '?view=lifecycle')
    await expect(page.getByRole('heading', { level: 1, name: 'Lifecycle' })).toBeVisible()
    const stages = page.locator('.stage-title')
    await expect(stages.nth(0)).toHaveText('Session start')
    await expect(stages.nth(6)).toHaveText('Session end')
    if (MOCK) await expect(page.locator('.stage-empty')).toHaveCount(7)
    const avatar = page.locator('.stage').first().locator('.tool-avatar').first()
    await expect(avatar).toHaveAttribute('title', /^Claude Code - SessionStart$/)
    await expect(avatar.locator('img')).toBeVisible()
    await saveScreenshot(page, '02-lifecycle.png')
  })

  test('shows built-ins as switches and config values as chips and text', async ({ page }) => {
    await openApp(page, '?view=settings')
    await expect(page.getByRole('heading', { level: 1, name: 'Settings' })).toBeVisible()
    const builtins = page.locator('.item-row', { has: page.locator('.switch') })
    await expect(builtins).toHaveCount(5)
    await expect(builtins.filter({ hasText: 'memory' }).locator('.switch-text')).toHaveText(/On|Off/)
    const config = page.locator('.item-row', { hasText: 'agnostic-ai.yaml' })
    await expect(config.getByRole('link', { name: /^Edit .+ in Cursor$/ })).toHaveAttribute('href', /^cursor:\/\/file\/.+\.yaml$/)
    await expect(config.locator('.value-row', { hasText: 'targets' }).locator('.tool-avatar').first()).toBeVisible()
    await saveScreenshot(page, '03-settings.png')
  })

  test('shows a check finding on the item it names', async ({ page }) => {
    test.skip(!MOCK, 'The finding is specific to the demo project fixture.')
    await openApp(page, '?view=authority')
    await expect(page.getByRole('heading', { level: 1, name: 'Authority' })).toBeVisible()
    const permissions = page.locator('.item-row', { hasText: 'permissions.yaml' }).first()
    await expect(permissions).toHaveClass(/has-findings/)
    await expect(permissions.locator('.finding')).toContainText('LINT038')
    await expect(page.locator('.field-row', { hasText: 'Allowed without asking' }).locator('.chip').first()).toBeVisible()
    await expect(page.locator('.field-row', { hasText: 'Default permission mode' })).toContainText('Not set')
    await expect(page.getByText('Capability domains')).toHaveCount(0)
    await expect(page.getByText('Uncertainties')).toHaveCount(0)
    await saveScreenshot(page, '04-authority.png')
  })

  test('lists each MCP server with its cached tools or how to list them', async ({ page }) => {
    await openApp(page, '?view=tools')
    await expect(page.getByRole('heading', { level: 1, name: 'Tools' })).toBeVisible()
    const servers = page.locator('.item-row', { has: page.locator('.tool-listing') })
    await expect(servers.first()).toBeVisible()
    if (MOCK) await expect(servers.first().locator('.tool-state')).toContainText('agnostic-ai mcp tools')
  })

  test('keeps the header and navigation in place while the content scrolls', async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 500 })
    await openApp(page, '?view=lifecycle')
    await expect(page.locator('.footer')).toHaveCount(0)
    const main = page.locator('#main')
    expect(await main.evaluate((el) => el.scrollHeight > el.clientHeight)).toBe(true)
    expect(await page.evaluate(() => document.scrollingElement!.scrollHeight <= window.innerHeight + 1)).toBe(true)
    const navTop = await page.locator('.sidenav').evaluate((el) => el.getBoundingClientRect().top)
    await main.evaluate((el) => el.scrollTo({ top: el.scrollHeight }))
    expect(await page.locator('.topbar').evaluate((el) => el.getBoundingClientRect().top)).toBe(0)
    expect(await page.locator('.sidenav').evaluate((el) => el.getBoundingClientRect().top)).toBe(navTop)
  })

  test('follows the system colour scheme until the person picks a theme', async ({ page }) => {
    await page.emulateMedia({ colorScheme: 'light' })
    await openApp(page)
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'light')
    await page.emulateMedia({ colorScheme: 'dark' })
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark')
    await page.getByRole('button', { name: 'Switch to light theme' }).click()
    await page.emulateMedia({ colorScheme: 'dark' })
    await page.reload()
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'light')
  })

  test('keeps the Caveman switch aligned while its preview and errors use the full row', async ({ page }) => {
    const state = { schemaVersion: 'builtin-change/v1', name: 'caveman', status: 'idle', enabled: false, supported: true, undoAvailable: false, targets: [], changes: [], findings: [] }
    let refuse = false
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: fixture }))
    await page.route('**/api/changes/builtin', (route) => route.fulfill({ json: state }))
    await page.route('**/api/changes/builtin/preview', (route) => route.fulfill(refuse
      ? { status: 422, json: { error: 'The snapshot contains a symbolic link.' } }
      : { json: { ...state, status: 'proposed', id: 'preview', enabled: true, config: 'agnostic-ai.yaml', targets: ['claude'] } }))
    await page.goto('/?view=settings')
    const row = page.locator('.item-row', { has: page.getByRole('switch', { name: 'caveman built-in', exact: true }) })
    const toggle = row.locator('.item-heading').getByRole('switch', { name: 'caveman built-in', exact: true })
    await expect(toggle).toBeEnabled()
    const memory = page.getByRole('switch', { name: 'memory built-in', exact: true })
    const aligned = async () => {
      const first = await toggle.boundingBox()
      const other = await memory.boundingBox()
      expect(first).not.toBeNull()
      expect(other).not.toBeNull()
      const desktop = page.viewportSize()!.width > 760
      const edge = (box: NonNullable<typeof first>) => box.x + (desktop ? box.width : 0)
      expect(Math.abs(edge(first!) - edge(other!))).toBeLessThan(1)
    }
    await aligned()
    await toggle.click()
    const preview = row.getByRole('region', { name: 'Caveman change preview' })
    await expect(preview).toBeVisible()
    expect((await preview.boundingBox())!.width).toBeGreaterThan((await row.boundingBox())!.width * 0.9)
    await expect(row.locator('.item-heading .builtin-change-preview')).toHaveCount(0)
    await row.getByRole('button', { name: 'Cancel', exact: true }).click()
    refuse = true
    await toggle.click()
    await expect(page.getByRole('alert')).toContainText('The snapshot contains a symbolic link.')
    await aligned()
    await page.setViewportSize({ width: 390, height: 844 })
    await aligned()
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  })

  test('keeps global built-ins source-only with one sync instruction', async ({ page }) => {
    let memory = false
    let refuse = false
    const posted: unknown[] = []
    await page.route('**/api/changes/builtin', (route) => route.fulfill({
      json: { schemaVersion: 'builtin-change/v1', name: 'caveman', status: 'idle', enabled: false, supported: false, undoAvailable: false, targets: [], changes: [], findings: [] },
    }))
    await page.route('**/api/views/overview', async (route) => {
      const view = structuredClone(fixture)
      for (const it of view.sections.find((s: { id: string }) => s.id === 'settings').items) if (it.name === 'memory') it.enabled = memory
      await route.fulfill({ json: view })
    })
    await page.route('**/api/edit/builtin', async (route) => {
      posted.push(route.request().postDataJSON())
      if (refuse) return route.fulfill({ status: 422, json: { error: 'the edit did not validate, so the file was restored', problems: ['agnostic-ai.yaml: builtins: memory clashes with skill memory'] } })
      memory = !memory
      return route.fulfill({ json: { ok: true } })
    })
    await page.goto('./?view=settings')
    const toggle = page.getByRole('switch', { name: 'memory built-in' })
    await expect(toggle).toHaveAttribute('aria-checked', 'false')
    await expect(page.getByText('Saves source files.')).toHaveCount(1)
    await expect(page.getByText('agnostic-ai sync --global', { exact: true })).toBeVisible()
    await expect(page.getByRole('switch')).toHaveCount(5)
    await toggle.click()
    await expect(toggle).toHaveAttribute('aria-checked', 'true')
    expect(posted).toEqual([{ name: 'memory', enabled: true }])

    refuse = true
    await toggle.click()
    await expect(page.getByRole('alert')).toContainText('the file was restored')
    await expect(page.getByRole('alert')).toContainText('memory clashes with skill memory')
    await expect(toggle).toHaveAttribute('aria-checked', 'true')
  })

  test('opens the JSON inspector on the overview document', async ({ page, context }) => {
    await context.grantPermissions(['clipboard-read', 'clipboard-write'])
    await openApp(page)
    const button = page.getByRole('button', { name: 'JSON', exact: true })
    await button.click()
    const drawer = page.getByRole('dialog', { name: 'JSON inspector' })
    await expect(drawer.locator('.inspector-meta')).toContainText('overview/v1')
    await drawer.getByRole('button', { name: 'Copy JSON' }).click()
    await expect(drawer.getByRole('button', { name: 'JSON copied' })).toBeVisible()
    await page.keyboard.press('Escape')
    await expect(drawer).toHaveCount(0)
    await expect(button).toBeFocused()
  })

  test('starts a field edit on the spec in effect and links global instructions only for retention', async ({ page }) => {
    const data = structuredClone(fixture)
    const fields = data.sections.find((section: { id: string }) => section.id === 'settings').fields
    const field = (key: string) => fields.find((f: { key: string }) => f.key === key)
    const paths = ['.agnostic-ai/settings/a.yaml', '.agnostic-ai/settings/b.yaml']
    Object.assign(field('model'), {
      set: [
        { spec: 'a', path: paths[0], values: [{ key: 'model', type: 'text', value: 'sonnet' }] },
        { spec: 'b', path: paths[1], values: [{ key: 'model', type: 'text', value: 'opus' }] },
      ],
      editable: { kind: 'text', path: paths[1], paths, syncCommand: 'agnostic-ai sync --global' },
    })
    Object.assign(field('effort'), {
      set: [{ spec: 'a', path: paths[0], values: [{ key: 'effort.claude', type: 'text', value: 'high' }] }],
      editable: { kind: 'text', paths, disabledReason: 'This field has a non-scalar value; edit the spec directly.' },
    })
    Object.assign(field('session-retention-days'), {
      editable: { kind: 'number', path: paths[0], disabledReason: 'Session retention is written only by sync --global. Open Settings in agnostic-ai ui --global.' },
    })
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: data }))
    let edit: unknown
    await page.route('**/api/edit/field', (route) => {
      edit = route.request().postDataJSON()
      return route.fulfill({ json: { ok: true } })
    })
    await page.goto('/?view=settings')
    const model = page.locator('.field-row').filter({ has: page.getByText('Default model', { exact: true }) })
    await expect(model.getByRole('combobox')).toHaveValue(paths[1])
    await expect(model.getByRole('textbox', { name: 'Default model value' })).toHaveValue('opus')
    await expect(model).toContainText('Run agnostic-ai sync --global to apply.')
    await model.getByRole('textbox', { name: 'Default model value' }).fill('haiku')
    await model.getByRole('button', { name: 'Update' }).click()
    await expect.poll(() => edit).toEqual({ path: paths[1], key: 'model', value: 'haiku' })
    const effort = page.locator('.field-row').filter({ has: page.getByText('Reasoning effort', { exact: true }) })
    await expect(effort).toContainText('edit the spec directly')
    await expect(effort.locator('a', { hasText: 'Global view instructions' })).toHaveCount(0)
    const retention = page.locator('.field-row').filter({ has: page.getByText('Session retention', { exact: true }) })
    await expect(retention.locator('a', { hasText: 'Global view instructions' })).toHaveCount(1)
  })
})
