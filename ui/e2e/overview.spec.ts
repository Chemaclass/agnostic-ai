import { expect, test } from '@playwright/test'
import { readFileSync } from 'node:fs'
import { MOCK, openApp, saveScreenshot, selectTab } from './helpers'
import { findingEntryIds, itemEntryId } from '../src/model/identity'
import type { SectionItem } from '../src/contract'

const fixture = JSON.parse(readFileSync(new URL('../src/fixtures/overview.mock.json', import.meta.url), 'utf8'))

async function sourceOnly(page: import('@playwright/test').Page) {
  await page.route('**/api/changes/builtin', (route) => route.fulfill({ json: { schemaVersion: 'builtin-change/v1', status: 'idle', enabled: false, supported: false, undoAvailable: false, targets: [], changes: [], findings: [] } }))
}

test.describe('overview', () => {
  test('keeps recheck feedback when a finding moves to another section', async ({ page }) => {
    const data = structuredClone(fixture)
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: data }))
    await page.goto('/?view=authority')
    const finding = page.locator('.finding', { hasText: 'LINT038' })
    await expect(finding).toBeVisible()
    const authority = data.sections.find((section: { id: string }) => section.id === 'authority')
    const settings = data.sections.find((section: { id: string }) => section.id === 'settings')
    const index = authority.items.findIndex((item: { name: string }) => item.name === 'permissions')
    settings.items.push(authority.items.splice(index, 1)[0])
    await finding.getByRole('button', { name: 'Recheck' }).click()
    await expect(page.getByRole('status').filter({ hasText: 'This finding still appears after the recheck.' })).toBeVisible()
  })

  test('keeps recheck feedback when a finding moves but its item stays', async ({ page }) => {
    const data = structuredClone(fixture)
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: data }))
    await page.goto('/?view=authority')
    const finding = page.locator('.finding', { hasText: 'LINT038' })
    await expect(finding).toBeVisible()
    const authority = data.sections.find((section: { id: string }) => section.id === 'authority')
    const settings = data.sections.find((section: { id: string }) => section.id === 'settings')
    const item = authority.items.find((entry: { name: string }) => entry.name === 'permissions')
    settings.findings.push(item.findings.pop())
    await finding.getByRole('button', { name: 'Recheck' }).click()
    await expect(page.getByRole('status').filter({ hasText: 'This finding still appears after the recheck.' })).toBeVisible()
  })

  test('keeps recheck feedback when a section finding moves to another section', async ({ page }) => {
    const data = structuredClone(fixture)
    const authority = data.sections.find((section: { id: string }) => section.id === 'authority')
    const settings = data.sections.find((section: { id: string }) => section.id === 'settings')
    const item = authority.items.find((entry: { name: string }) => entry.name === 'permissions')
    authority.findings.push(item.findings.pop())
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: data }))
    await page.goto('/?view=authority')
    await page.getByRole('button', { name: 'Review 1 finding', exact: true }).click()
    const finding = page.locator('.finding', { hasText: 'LINT038' })
    await expect(finding).toBeVisible()
    settings.items.push({ ...item, findings: [authority.findings.pop()] })
    await finding.getByRole('button', { name: 'Recheck' }).click()
    await expect(page.getByRole('status').filter({ hasText: 'This finding still appears after the recheck.' })).toBeVisible()
  })

  test('copies a refine prompt for the global instructions file only', async ({ page, context }) => {
    const data = structuredClone(fixture)
    const contextSection = data.sections.find((section: { id: string }) => section.id === 'context')
    const instructions = { kind: 'instructions', name: 'AGNOSTIC_AI.md', label: 'AGNOSTIC_AI.md', description: 'Loaded at the start of every session: 120 words.', values: [], findings: [] }
    contextSection.items.push(
      { ...instructions, path: 'AGNOSTIC_AI.md', openPath: '/home/me/.agnostic-ai/AGNOSTIC_AI.md', layer: 'global' },
      { ...instructions, path: 'local/AGNOSTIC_AI.md', openPath: '/home/me/.agnostic-ai/local/AGNOSTIC_AI.md', layer: 'global-local' },
    )
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: data }))
    await page.goto('/?view=context')
    const refine = page.getByRole('button', { name: /^Copy refine prompt/ })
    await expect(refine).toHaveCount(1)
    await expect(page.locator('.item-row', { hasText: 'local/AGNOSTIC_AI.md' }).getByRole('button', { name: /^Copy refine prompt/ })).toHaveCount(0)
    await context.grantPermissions(['clipboard-read', 'clipboard-write'])
    await refine.click()
    await expect(page.getByRole('button', { name: 'Copied', exact: true })).toBeVisible()
    const prompt = await page.evaluate(() => navigator.clipboard.readText())
    expect(prompt).toContain('global agent instructions in /home/me/.agnostic-ai/AGNOSTIC_AI.md')
    expect(prompt).toContain('Then wait for my explicit approval.')
  })

  test('shows the brand once and keeps other project names as plain context', async ({ page }) => {
    await page.route('**/api/health', (route) => route.fulfill({ json: {} }))
    const data = structuredClone(fixture)
    data.snapshot.project.name = 'agnostic-ai'
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: data }))
    await page.goto('/?view=context')
    await expect(page.locator('.support-table')).toBeVisible()
    await expect(page.locator('.brand-name')).toHaveText('agnostic-ai')
    await expect(page.locator('.project-name')).toHaveCount(0)
    await expect(page.locator('.topbar-context')).toHaveCount(0)
    data.snapshot.project.name = 'billing-service'
    data.snapshot.project.root = '/work/clients/billing-service'
    await page.reload()
    await expect(page.locator('.project-title')).toHaveText('billing-service')
    await expect(page.locator('.project-root')).toHaveText('/work/clients/billing-service')
    await expect(page.locator('.project-name')).not.toHaveClass(/chip/)
  })

  test('offers reload when a boolean save is stale', async ({ page }) => {
    const data = structuredClone(fixture)
    const settings = data.sections.find((section: { id: string }) => section.id === 'settings')
    settings.fields.find((field: { key: string }) => field.key === 'x-').set = [{ spec: 'native', path: '.agnostic-ai/settings/native.yaml', values: [{ key: 'x-claude.verbose', type: 'bool', value: 'false' }] }]
    let failRefresh = false
    await page.route('**/api/views/overview', (route) => {
      if (failRefresh) { failRefresh = false; return route.fulfill({ status: 500, json: { error: 'Refresh failed.' } }) }
      return route.fulfill({ json: data })
    })
    await page.route('**/api/changes/builtin', (route) => route.fulfill({ json: { schemaVersion: 'builtin-change/v1', status: 'idle', supported: false, enabled: false, undoAvailable: false, targets: [], changes: [], findings: [] } }))
    let sent: unknown
    await page.route('**/api/edit/value', (route) => {
      sent = route.request().postDataJSON()
      settings.fields.find((field: { key: string }) => field.key === 'x-').set[0].values[0].value = 'true'
      failRefresh = true
      return route.fulfill({ status: 409, json: { error: 'The value changed. Git protection was kept.' } })
    })
    await page.goto('/?view=settings&tab=preferences')
    const toggle = page.getByRole('switch', { name: 'x-claude.verbose' })
    await toggle.click()
    await expect(page.getByRole('alert')).toContainText('The value changed. Git protection was kept.')
    await expect(page.getByRole('button', { name: 'Reload saved values' })).toBeVisible()
    await expect(toggle).toHaveAttribute('aria-checked', 'true')
    expect(sent).toEqual({ audience: 'shared', path: '.agnostic-ai/settings/native.yaml', key: 'x-claude.verbose', value: true, expected: false })
    await page.getByRole('button', { name: 'Reload saved values' }).click()
    await expect(page.getByText('The overview could not be refreshed.')).toBeVisible()
    await expect(toggle).toHaveAttribute('aria-checked', 'true')
    await expect(page.getByText('The value changed. Git protection was kept.')).toBeVisible()
    await page.getByRole('button', { name: 'Retry', exact: true }).click()
    await expect(page.getByText('The overview could not be refreshed.')).toHaveCount(0)
    await expect(toggle).toHaveAttribute('aria-checked', 'true')
    await expect(page.getByText('The value changed. Git protection was kept.')).toHaveCount(0)
  })

  test('offers reload when a built-in save is stale', async ({ page }) => {
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: fixture }))
    await page.route('**/api/changes/builtin', (route) => route.fulfill({ json: { schemaVersion: 'builtin-change/v1', status: 'idle', supported: false, enabled: false, undoAvailable: false, targets: [], changes: [], findings: [] } }))
    let sent: unknown
    await page.route('**/api/edit/builtin', (route) => {
      sent = route.request().postDataJSON()
      return route.fulfill({ status: 409, json: { error: 'The built-in changed.' } })
    })
    await page.goto('/?view=settings')
    const toggle = page.getByRole('switch', { name: 'memory built-in' })
    const before = await toggle.getAttribute('aria-checked')
    await toggle.click()
    await expect(page.getByRole('alert')).toContainText('The built-in changed.')
    await expect(page.getByRole('button', { name: 'Reload saved values' })).toBeVisible()
    await expect(toggle).toHaveAttribute('aria-checked', before === 'true' ? 'false' : 'true')
    expect(sent).toEqual({ audience: 'shared', name: 'memory', enabled: before !== 'true', expected: before === 'true' })
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
    await selectTab(page, 'Config files')
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
    await page.goto('/?view=settings&tab=preferences')
    const native = page.locator('.native-group', { hasText: 'x-ellipsis' })
    await expect(native.locator('.native-group-title')).toHaveText('x-ellipsis')
    await expect(native.locator('dt').first()).toHaveText('environment.repository.name')
    await native.getByRole('switch', { name: 'x-ellipsis.pull_requests.draft' }).click()
    expect(edit).toEqual({ audience: 'shared', path: '.agnostic-ai/settings/native.yaml', key: 'x-ellipsis.pull_requests.draft', value: true, expected: false })
    await page.getByRole('button', { name: 'Review 1 finding', exact: true }).click()
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
    await page.getByRole('button', { name: 'Review 1 finding', exact: true }).click()
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
    for (const [tab, kind] of [['Guidance', 'Instructions'], ['Rules', 'Rules'], ['Agents', 'Agents'], ['Skills', 'Skills']] as const) {
      await selectTab(page, tab)
      await expect(page.getByRole('heading', { level: 2, name: new RegExp(`^${kind}`) })).toBeVisible()
    }
    await selectTab(page, 'Agents')
    const nav = page.getByRole('navigation', { name: 'Configuration sections' })
    await expect(nav.getByRole('link')).toHaveCount(5)
    for (const name of ['Context', 'Authority', 'Tools', 'Lifecycle', 'Settings']) await expect(nav.getByRole('link', { name: new RegExp(`^${name}`) })).toBeVisible()
    await expect(page.locator('.topbar .chip', { hasText: 'sync' })).toHaveCount(0)
    const theme = page.getByRole('button', { name: /^Switch to (light|dark) theme$/ })
    const before = await theme.getAttribute('aria-label')
    await theme.click()
    await expect(theme).not.toHaveAttribute('aria-label', before ?? '')
    await expect(page.locator('.topbar').getByRole('combobox')).toHaveCount(0)
    const open = page.locator('.item-row', { hasText: 'agents/' }).first().getByRole('link', { name: /^Open .+ in Cursor$/ })
    if (MOCK) await expect(open).toHaveCount(0)
    else await expect(open).toHaveAttribute('href', /^cursor:\/\/file\/.+\.md$/)
    await saveScreenshot(page, '01-context.png')
  })

  test('lists every lifecycle stage in session order, even with no hooks', async ({ page }) => {
    await openApp(page, '?view=lifecycle')
    await expect(page.getByRole('heading', { level: 1, name: 'Lifecycle' })).toBeVisible()
    const stages = page.locator('.stage-title')
    await expect(stages.nth(0)).toHaveText('Session start')
    await expect(stages.nth(6)).toHaveText('Session end')
    await expect(page.locator('.stages > li.stage-step')).toHaveCount(7)
    await expect(page.locator('.stage-description')).toHaveCount(7)
    await expect(page.locator('.stage-description').first()).toBeVisible()
    if (MOCK) {
      await expect(page.getByRole('region', { name: /^Hooks/ })).toContainText('None configured.')
      await expect(page.locator('details.stage')).toHaveCount(0)
      await expect(page.locator('.stage-count')).toHaveCount(0)
      await expect(page.getByRole('button', { name: 'Expand all', exact: true })).toHaveCount(0)
      await expect(page.getByRole('button', { name: 'Collapse all', exact: true })).toHaveCount(0)
      const avatar = page.locator('.stage').first().locator('.tool-avatar').first()
      await expect(avatar).toHaveAttribute('title', /^Claude Code - SessionStart$/)
      await expect(avatar.locator('img')).toBeVisible()
      await saveScreenshot(page, '02-lifecycle-empty.png')
    }
  })

  test('opens lifecycle stages one at a time or all at once when a hook exists', async ({ page }) => {
    const data = structuredClone(fixture)
    const lifecycle = data.sections.find((section: { id: string }) => section.id === 'lifecycle')
    const hook: SectionItem = { kind: 'hook', name: 'prepare', label: 'Prepare session', description: '', path: '.agnostic-ai/hooks/prepare.yaml', layer: 'project', group: 'session-start', values: [], findings: [] }
    lifecycle.items = [hook]
    lifecycle.kinds[0].count = 1
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: data }))
    await page.goto('/?view=lifecycle')
    await expect(page.getByRole('heading', { level: 1, name: 'Lifecycle' })).toBeVisible()
    await expect(page.locator('.stage-empty')).toHaveCount(6)
    await expect(page.locator('.stage-count')).toHaveText(['1 hook', ...Array(6).fill('No hooks')])
    await expect(page.locator('details.stage')).toHaveCount(7)
    await expect(page.locator('.stage[open]')).toHaveCount(0)
    await expect(page.locator('.stages > li > details.stage')).toHaveCount(7)
    await expect(page.locator('.stage summary .stage-description')).toHaveCount(7)
    await expect(page.locator('.stage summary .stage-count')).toHaveCount(7)
    await expect(page.locator('.stage').first().locator('.stage-tools')).toBeHidden()
    await saveScreenshot(page, '02-lifecycle-collapsed.png')
    await page.locator('.stage').first().locator('summary').focus()
    await page.keyboard.press('Enter')
    await expect(page.locator('.stage[open]')).toHaveCount(1)
    const avatar = page.locator('.stage').first().locator('.tool-avatar').first()
    await expect(avatar).toHaveAttribute('title', /^Claude Code - SessionStart$/)
    await expect(avatar.locator('img')).toBeVisible()
    await page.getByRole('button', { name: 'Expand all', exact: true }).click()
    await expect(page.locator('.stage[open]')).toHaveCount(7)
    await page.getByRole('button', { name: 'Collapse all', exact: true }).click()
    await expect(page.locator('.stage[open]')).toHaveCount(0)
    await page.getByRole('button', { name: 'Expand all', exact: true }).click()
    await page.getByRole('link', { name: 'Context', exact: false }).first().click()
    await expect(page.getByRole('button', { name: 'Expand all', exact: true })).toHaveCount(0)
    await page.getByRole('link', { name: 'Lifecycle', exact: false }).first().click()
    await expect(page.locator('.stage[open]')).toHaveCount(0)
    await saveScreenshot(page, '02-lifecycle.png')
  })

  test('opens the stage and focuses a linked hook or its finding', async ({ page }) => {
    const data = structuredClone(fixture)
    const lifecycle = data.sections.find((section: { id: string }) => section.id === 'lifecycle')
    const hook: SectionItem = { kind: 'hook', name: 'prepare', label: 'Prepare session', description: '', path: '.agnostic-ai/hooks/prepare.yaml', layer: 'project', group: 'session-start', values: [], findings: [] }
    lifecycle.items = [hook]
    lifecycle.kinds[0].count = 1
    hook.findings = [{ source: 'lint', code: 'LINT001', severity: 'warning', message: 'Check the session hook.', path: hook.path }]
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: data }))
    const entry = itemEntryId(hook)
    for (const id of [entry, ...findingEntryIds(hook.findings, entry)]) {
      await page.goto(`/?view=lifecycle&entry=${encodeURIComponent(id)}`)
      await expect(page.locator('.stage[open]')).toHaveCount(1)
      await expect(page.locator('.stage').first()).toHaveAttribute('open', '')
      await expect(page.locator(`[data-entry="${id}"]`)).toBeFocused()
      await expect(page.locator(`[data-entry="${id}"]`)).toBeVisible()
    }
  })

  test('keeps a linked hook stage open and focuses resolved recheck feedback until dismissed', async ({ page }) => {
    const data = structuredClone(fixture)
    const lifecycle = data.sections.find((section: { id: string }) => section.id === 'lifecycle')
    const hook: SectionItem = { kind: 'hook', name: 'prepare', label: 'Prepare session', description: '', path: '.agnostic-ai/hooks/prepare.yaml', layer: 'project', group: 'session-start', values: [], findings: [] }
    lifecycle.items = [hook]
    lifecycle.kinds[0].count = 1
    hook.findings = [{ source: 'lint', code: 'LINT001', severity: 'warning', message: 'Check the session hook.', path: hook.path }]
    let finishRefresh: (() => void) | undefined
    let refreshing = false
    await page.route('**/api/views/overview', async (route) => {
      if (refreshing) await new Promise<void>((resolve) => { finishRefresh = resolve })
      return route.fulfill({ json: data })
    })
    const id = findingEntryIds(hook.findings, itemEntryId(hook))[0]
    await page.goto(`/?view=lifecycle&q=no-such-hook&entry=${encodeURIComponent(id)}`)
    const stage = page.locator('.stage').first()
    const finding = stage.locator('.finding')
    await expect(finding).toBeFocused()
    refreshing = true
    await finding.getByRole('button', { name: 'Recheck', exact: true }).click()
    await expect(finding.getByRole('button', { name: 'Rechecking…', exact: true })).toBeFocused()
    await expect(stage).toHaveAttribute('open', '')
    hook.findings = []
    await expect.poll(() => Boolean(finishRefresh)).toBe(true)
    finishRefresh!()
    const feedback = stage.locator('.finding-rechecked')
    await expect(feedback).toBeVisible()
    await expect(feedback).toBeFocused()
    await expect(feedback).toContainText('This lint finding no longer appears.')
    await expect(page.getByRole('searchbox', { name: 'Search lifecycle' })).toHaveValue('no-such-hook')
    await expect(stage).toHaveAttribute('open', '')
    await page.getByRole('button', { name: 'Collapse all', exact: true }).click()
    await expect(stage).not.toHaveAttribute('open')
    await page.getByRole('button', { name: 'Expand all', exact: true }).click()
    await expect(feedback).toBeVisible()
    await feedback.getByRole('button', { name: 'Dismiss', exact: true }).click()
    await expect(feedback).toHaveCount(0)
    await expect(stage).not.toHaveAttribute('open')
    await expect(page.getByRole('heading', { name: 'Hooks 1', exact: true })).toBeFocused()
  })

  test('keeps a linked hook stage open and focuses last known feedback when lint is incomplete', async ({ page }) => {
    const data = structuredClone(fixture)
    const lifecycle = data.sections.find((section: { id: string }) => section.id === 'lifecycle')
    const hook: SectionItem = { kind: 'hook', name: 'prepare', label: 'Prepare session', description: '', path: '.agnostic-ai/hooks/prepare.yaml', layer: 'project', group: 'session-start', values: [], findings: [] }
    lifecycle.items = [hook]
    lifecycle.kinds[0].count = 1
    hook.findings = [{ source: 'lint', code: 'LINT001', severity: 'warning', message: 'Check the session hook.', path: hook.path }]
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: data }))
    const id = findingEntryIds(hook.findings, itemEntryId(hook))[0]
    await page.goto(`/?view=lifecycle&entry=${encodeURIComponent(id)}`)
    const stage = page.locator('.stage').first()
    const finding = stage.locator('.finding')
    await expect(finding).toBeFocused()
    hook.findings = []
    lifecycle.findings.push({ source: 'lint', code: 'lint-incomplete', severity: 'warning', message: 'Lint stopped.' })
    await finding.getByRole('button', { name: 'Recheck', exact: true }).click()
    const feedback = stage.locator('.finding-recheck-result')
    await expect(feedback).toBeVisible()
    await expect(feedback).toBeFocused()
    await expect(feedback).toContainText('Lint stopped before it finished')
    await expect(stage).toHaveAttribute('open', '')
    const search = page.getByRole('searchbox', { name: 'Search lifecycle' })
    await search.fill('no-such-hook')
    await expect(search).toBeFocused()
    await expect(feedback).toBeVisible()
    await expect(stage.locator('.stage-count')).toHaveText('No matching hooks')
    await page.getByRole('button', { name: 'Collapse all', exact: true }).click()
    await expect(stage).not.toHaveAttribute('open')
    await page.getByRole('button', { name: 'Expand all', exact: true }).click()
    await expect(feedback).toBeVisible()
  })

  test('shows built-ins as switches and config values as chips and text', async ({ page }) => {
    await openApp(page, '?view=settings')
    await expect(page.getByRole('heading', { level: 1, name: 'Settings' })).toBeVisible()
    const builtins = page.locator('.item-row', { has: page.locator('.switch') })
    await expect(builtins).toHaveCount(5)
    await expect(builtins.filter({ hasText: 'memory' }).locator('.switch-text')).toHaveText(/On|Off/)
    await selectTab(page, 'Config files')
    const config = page.locator('.item-row', { hasText: 'agnostic-ai.yaml' })
    const open = config.getByRole('link', { name: /^Open .+ in Cursor$/ })
    if (MOCK) await expect(open).toHaveCount(0)
    else await expect(open).toHaveAttribute('href', /^cursor:\/\/file\/.+\.yaml$/)
    await expect(page.locator('.effective-config .value-row', { hasText: 'targets' }).locator('.tool-avatar').first()).toBeVisible()
    await saveScreenshot(page, '03-settings.png')
  })

  test('shows a check finding on the item it names', async ({ page }) => {
    test.skip(!MOCK, 'The finding is specific to the demo project fixture.')
    await openApp(page, '?view=authority')
    await expect(page.getByRole('heading', { level: 1, name: 'Authority' })).toBeVisible()
    const permissions = page.locator('.item-row', { hasText: 'permissions.yaml' }).first()
    await expect(permissions).toHaveClass(/has-findings/)
    await expect(permissions.locator('.finding')).toContainText('LINT038')
    await expect(permissions.locator('.finding-source')).toContainText('Files open in an editor only from a running agnostic-ai serve.')
    await expect(permissions.getByRole('button', { name: 'Copy path of .agnostic-ai/settings/permissions.yaml' })).toBeVisible()
    await expect(page.getByRole('group', { name: 'Open files in' })).toHaveCount(0)
    await expect(permissions.getByRole('button', { name: 'Recheck' })).toHaveCount(0)
    await expect(permissions.locator('.finding-recheck-result')).toHaveText('Rechecking a finding needs a running agnostic-ai serve.')
    await expect(page.locator('.field-row', { hasText: 'Allowed without asking' }).locator('.chip').first()).toBeVisible()
    await expect(page.locator('.field-row', { hasText: 'Default permission mode' })).toContainText('Not set')
    await expect(page.getByText('Capability domains')).toHaveCount(0)
    await expect(page.getByText('Uncertainties')).toHaveCount(0)
    await saveScreenshot(page, '04-authority.png')
  })

  test('opens a finding at its line in the chosen editor, with Copy path beside it', async ({ page, context }) => {
    const path = 'C:/Users/Some One/project #1/.agnostic-ai/settings/permissions.yaml'
    const data = structuredClone(fixture)
    const authority = data.sections.find((section: { id: string }) => section.id === 'authority')
    const item = authority.items.find((it: { name: string }) => it.name === 'permissions')
    item.findings = [
      { ...item.findings[0], path, line: 7, openPath: path },
      { source: 'doctor', code: 'gone', severity: 'warn', message: 'Names a file that is not on disk', path: 'missing.yaml' },
    ]
    await context.grantPermissions(['clipboard-read', 'clipboard-write'])
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: data }))
    await page.goto('/?view=authority')
    const finding = page.locator('.finding', { hasText: 'LINT038' })
    await expect(finding.locator('.finding-source')).toContainText(`${path}:7`)
    const open = finding.getByRole('link', { name: `Open ${path} in Cursor at line 7` })
    await expect(open).toHaveAttribute('href', 'cursor://file/C:/Users/Some%20One/project%20%231/.agnostic-ai/settings/permissions.yaml:7')

    const choice = page.getByRole('group', { name: 'Open files in' })
    await expect(choice.getByRole('button', { name: 'Cursor' })).toHaveAttribute('aria-pressed', 'true')
    await expect(page.locator('.topbar').getByRole('group', { name: 'Open files in' })).toBeInViewport()
    await choice.getByRole('button', { name: 'VS Code' }).focus()
    const switchWidth = (await choice.boundingBox())!.width
    await page.keyboard.press('Space')
    await expect(choice.getByRole('button', { name: 'VS Code' })).toHaveAttribute('aria-pressed', 'true')
    expect(Math.abs((await choice.boundingBox())!.width - switchWidth)).toBeLessThanOrEqual(1)
    await page.keyboard.press('ArrowLeft')
    await expect(choice.getByRole('button', { name: 'Cursor' })).toHaveAttribute('aria-pressed', 'true')
    await expect(choice.getByRole('button', { name: 'Cursor' })).toBeFocused()
    await page.keyboard.press('ArrowRight')
    await expect(choice.getByRole('button', { name: 'VS Code' })).toHaveAttribute('aria-pressed', 'true')
    const vscode = finding.getByRole('link', { name: `Open ${path} in VS Code at line 7` })
    await expect(vscode).toHaveAttribute('href', 'vscode://file/C:/Users/Some%20One/project%20%231/.agnostic-ai/settings/permissions.yaml:7')
    await expect(page.locator('.item-row', { hasText: 'protected' }).first().getByRole('link', { name: /^Open .+ in VS Code$/ })).toHaveAttribute('href', /^vscode:\/\/file\/.+protected\.yaml$/)

    await page.evaluate(() => document.addEventListener('click', (event) => {
      if ((event.target as Element).closest('a[href^="vscode:"]')) event.preventDefault()
    }, true))
    await vscode.focus()
    await page.keyboard.press('Enter')
    await expect(finding.locator('.editor-hint')).toHaveText('Allow your browser to open VS Code. If nothing opens, choose an installed editor above or copy the path.')
    const linkBox = await vscode.boundingBox()
    const hintBox = await finding.locator('.editor-hint').boundingBox()
    expect(hintBox!.y).toBeGreaterThanOrEqual(linkBox!.y + linkBox!.height)
    await choice.getByRole('button', { name: 'Cursor' }).click()
    await expect(finding.locator('.editor-hint')).toHaveText('Allow your browser to open Cursor. If nothing opens, choose an installed editor above or copy the path.')
    await choice.getByRole('button', { name: 'VS Code' }).click()
    await finding.getByRole('button', { name: `Copy path of ${path}` }).click()
    expect(await page.evaluate(() => navigator.clipboard.readText())).toBe('C:\\Users\\Some One\\project #1\\.agnostic-ai\\settings\\permissions.yaml')

    const gone = page.locator('.finding', { hasText: 'Names a file that is not on disk' })
    await expect(gone.locator('.finding-source')).toContainText('missing.yaml')
    await expect(gone.locator('.finding-source')).toContainText('This path is not a file on disk, so it cannot be opened in an editor.')
    await expect(gone.getByRole('button', { name: 'Copy path of missing.yaml' })).toBeVisible()
    await expect(gone.getByRole('link')).toHaveCount(0)
    await saveScreenshot(page, '04b-finding-editor.png')

    await page.reload()
    await expect(page.getByRole('group', { name: 'Open files in' }).getByRole('button', { name: 'VS Code' })).toHaveAttribute('aria-pressed', 'true')
    await expect(page.locator('.finding', { hasText: 'LINT038' }).getByRole('link', { name: `Open ${path} in VS Code at line 7` })).toBeVisible()
    await page.setViewportSize({ width: 390, height: 844 })
    await page.evaluate(() => window.scrollTo(0, 0))
    await expect(page.locator('.topbar').getByRole('group', { name: 'Open files in' })).toBeInViewport()
    await choice.getByRole('combobox', { name: 'Editor', exact: true }).selectOption('cursor')
    await expect(finding.getByRole('link', { name: `Open ${path} in Cursor at line 7` })).toHaveAttribute('href', 'cursor://file/C:/Users/Some%20One/project%20%231/.agnostic-ai/settings/permissions.yaml:7')
  })

  test('lists each MCP server with its cached tools or how to list them', async ({ page }) => {
    await openApp(page, '?view=tools')
    await expect(page.getByRole('heading', { level: 1, name: 'Tools' })).toBeVisible()
    const block = page.getByRole('region', { name: /^MCP servers/ })
    await expect(block.locator('.item-row').first()).toBeVisible()
    if (MOCK) {
      await expect(block.locator('.tools-unlisted')).toHaveText('Tools not listed yet. Run agnostic-ai mcp tools to list them.')
      await expect(block.locator('.item-row .tool-state')).toHaveCount(0)
    }
  })

  test('says tools are not listed on each server only when others have a list', async ({ page }) => {
    const data = structuredClone(fixture)
    const servers: SectionItem[] = data.sections.find((section: { id: string }) => section.id === 'tools')!.items
    servers[0].tools = { state: 'listed', command: 'agnostic-ai mcp tools', tools: [{ name: 'query_metrics' }] }
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: data }))
    await page.goto('./?view=tools')
    const block = page.getByRole('region', { name: /^MCP servers/ })
    await expect(block.locator('.tools-unlisted')).toHaveCount(0)
    await expect(block.locator('.item-row').first().locator('.tool-listing')).toContainText('query_metrics')
    await expect(block.locator('.item-row .tool-state')).toHaveCount(servers.length - 1)
  })

  test("lists MCP servers found only in a tool's own config, with the import hint", async ({ page }) => {
    test.skip(!MOCK, 'the mock fixture holds a native server')
    await openApp(page, '?view=tools&tab=native')
    const block = page.getByRole('region', { name: /Found in a tool's own config/ })
    const row = block.locator('.item-row', { hasText: 'notion' })
    await expect(row).toContainText('Claude Code only')
    await expect(row).toContainText('~/.claude.json')
    await expect(row).toContainText('http')
    await expect(row.locator('.tool-state')).toContainText('Run agnostic-ai import --global claude to make it a spec.')
    await expect(row.getByRole('button', { name: 'Make it a spec' })).toHaveCount(0)
    await page.getByRole('searchbox').fill('notion')
    await expect(block.locator('.kind-count')).toHaveText('1 of 1 match')
    await page.getByRole('searchbox').fill('github')
    await selectTab(page, 'Found in tools')
    await expect(block.locator('.kind-count')).toHaveText('0 of 1 match')
    await expect(block).toContainText('No matches.')
  })

  test('previews one native server and saves the reviewed spec', async ({ page }) => {
    const data = structuredClone(fixture)
    const tools = data.sections.find((section: { id: string }) => section.id === 'tools')!
    const native = tools.native![0]
    const calls: Record<string, unknown>[] = []
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: data }))
    await page.route('**/api/edit/mcp', async (route) => {
      const body = route.request().postDataJSON() as Record<string, unknown>
      calls.push(body)
      if (body.action === 'apply') tools.native = []
      await route.fulfill({ json: { file: '.agnostic-ai/mcps/notion.yaml', spec: 'name: notion\ntype: http\nurl: https://mcp.notion.com/mcp\n', variables: [], targets: ['claude', 'codex'], syncCommand: 'agnostic-ai sync', expected: 'reviewed-server' } })
    })
    await page.goto('./?view=tools&tab=native')
    const row = page.getByRole('region', { name: /Found in a tool's own config/ }).locator('.item-row', { hasText: native.name })
    const scope = page.getByRole('region', { name: /Found in a tool's own config/ }).locator('.edit-scope')
    await expect(scope.getByRole('group', { name: 'Save new specs for' })).toBeVisible()
    await expect(scope).toContainText('Shared files, tracked by Git.')
    await expect(scope.getByRole('button', { name: 'Shared (committed)', exact: true })).toHaveText('Shared')
    const scopeBox = await scope.boundingBox()
    const listBox = await row.locator('..').boundingBox()
    expect(scopeBox!.y + scopeBox!.height).toBeLessThanOrEqual(listBox!.y)
    await row.getByRole('button', { name: 'Make it a spec' }).click()
    const preview = row.getByRole('region', { name: `${native.name} spec preview` })
    await expect(preview).toContainText('.agnostic-ai/mcps/notion.yaml')
    await expect(preview).toContainText('After sync, this server reaches')
    await preview.scrollIntoViewIfNeeded()
    await saveScreenshot(page, 'mcp-spec-preview-desktop.png')
    await page.setViewportSize({ width: 390, height: 844 })
    await preview.scrollIntoViewIfNeeded()
    await expect(preview.getByRole('button', { name: 'Apply', exact: true })).toBeVisible()
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    await saveScreenshot(page, 'mcp-spec-preview-mobile.png')
    await preview.getByRole('button', { name: 'Apply', exact: true }).click()
    await expect(page.getByRole('status').filter({ hasText: 'Saved .agnostic-ai/mcps/notion.yaml.' })).toContainText('Run agnostic-ai sync')
    await expect(row).toHaveCount(0)
    await expect(page.getByRole('group', { name: 'Save new specs for' })).toHaveCount(0)
    expect(calls).toEqual([
      { action: 'preview', audience: 'shared', name: native.name, target: native.holders[0].target, path: native.holders[0].path, scope: native.holders[0].scope },
      { action: 'apply', audience: 'shared', name: native.name, target: native.holders[0].target, path: native.holders[0].path, scope: native.holders[0].scope, expected: 'reviewed-server' },
    ])
    await page.reload()
    await expect(page.getByRole('status').filter({ hasText: 'Saved .agnostic-ai/mcps/notion.yaml.' })).toContainText('Run agnostic-ai sync')
  })

  test('drops a native server preview when its audience changes and shows import refusals', async ({ page }) => {
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: fixture }))
    await page.route('**/api/edit/mcp', async (route) => {
      if (route.request().postDataJSON().audience === 'personal') {
        await route.fulfill({ status: 422, json: { error: 'args holds a credential import cannot rewrite; write a ${NAME} reference by hand' } })
        return
      }
      await route.fulfill({ json: { file: '.agnostic-ai/mcps/notion.yaml', spec: 'name: notion\n', variables: [], targets: ['claude'], syncCommand: 'agnostic-ai sync', expected: 'reviewed-server' } })
    })
    await page.goto('./?view=tools&tab=native')
    const row = page.getByRole('region', { name: /Found in a tool's own config/ }).locator('.item-row', { hasText: 'notion' })
    await row.getByRole('button', { name: 'Make it a spec' }).click()
    await expect(row.getByRole('region', { name: 'notion spec preview' })).toBeVisible()
    await page.getByRole('button', { name: 'Personal (not committed)', exact: true }).click()
    await expect(row.getByRole('region', { name: 'notion spec preview' })).toHaveCount(0)
    await row.getByRole('button', { name: 'Make it a spec' }).click()
    await expect(row.getByRole('alert')).toContainText('args holds a credential import cannot rewrite')
    await expect(row.getByRole('button', { name: 'Apply', exact: true })).toHaveCount(0)
  })

  test('keeps the header and navigation in place while the content scrolls', async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 500 })
    if (MOCK) await page.route('**/assets/overview.mock-*.js', async (route) => {
      await new Promise((resolve) => setTimeout(resolve, 1000))
      await route.continue()
    })
    await openApp(page, '?view=lifecycle')
    await expect(page.locator('.footer')).toHaveCount(0)
    await expect.poll(() => page.evaluate(() => document.scrollingElement!.scrollHeight > window.innerHeight)).toBe(true)
    const navTop = await page.locator('.sidenav').evaluate((el) => el.getBoundingClientRect().top)
    await page.evaluate(() => window.scrollTo({ top: document.scrollingElement!.scrollHeight }))
    expect(await page.evaluate(() => window.scrollY)).toBeGreaterThan(0)
    expect(await page.locator('.topbar').evaluate((el) => el.getBoundingClientRect().top)).toBe(0)
    expect(await page.locator('.sidenav').evaluate((el) => el.getBoundingClientRect().top)).toBeCloseTo(navTop, 0)
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
    await page.route('**/api/changes/builtin/cancel', (route) => route.fulfill({ json: state }))
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
    expect(posted).toEqual([{ audience: 'shared', name: 'memory', enabled: true, expected: false }])

    refuse = true
    await toggle.click()
    await expect(page.getByRole('alert')).toContainText('the file was restored')
    await expect(page.getByRole('alert')).toContainText('memory clashes with skill memory')
    await expect(toggle).toHaveAttribute('aria-checked', 'true')
  })

  test('saves settings and built-ins as personal, marks personal values, and resets them', async ({ page }) => {
    const data = structuredClone(fixture)
    const settings = data.sections.find((section: { id: string }) => section.id === 'settings')
    const field = (key: string) => settings.fields.find((f: { key: string }) => f.key === key)
    const team = '.agnostic-ai/settings/defaults.yaml'
    const local = '.agnostic-ai/local/settings/defaults.yaml'
    Object.assign(field('model'), {
      set: [
        { spec: 'defaults', path: local, local: true, values: [{ key: 'model', type: 'text', value: 'opus' }] },
        { spec: 'defaults', path: team, values: [{ key: 'model', type: 'text', value: 'sonnet' }] },
      ],
      editable: { kind: 'text', path: team, paths: [team], localPath: local, syncCommand: 'agnostic-ai sync' },
    })
    Object.assign(field('effort'), {
      set: [{ spec: 'defaults', path: team, values: [{ key: 'effort', type: 'text', value: 'high' }] }],
      editable: { kind: 'text', path: team, paths: [team], localPath: local, syncCommand: 'agnostic-ai sync' },
    })
    for (const item of settings.items) {
      if (item.name === 'caveman') Object.assign(item, { enabled: true, local: true })
      if (item.name === 'memory') item.enabled = false
    }
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: data }))
    await page.route('**/api/changes/builtin', (route) => route.fulfill({
      json: { schemaVersion: 'builtin-change/v1', name: 'caveman', status: 'idle', enabled: false, supported: false, undoAvailable: false, targets: [], changes: [], findings: [] },
    }))
    const posted: unknown[] = []
    for (const path of ['**/api/edit/field', '**/api/edit/builtin']) {
      await page.route(path, (route) => {
        const body = route.request().postDataJSON()
        posted.push(body)
        if (body.key === 'model' && body.value === null) field('model').set = field('model').set.filter((source: { local?: boolean }) => !source.local)
        if (body.name === 'caveman') Object.assign(settings.items.find((item: { name: string }) => item.name === 'caveman'), { enabled: body.enabled, local: false })
        return route.fulfill({ json: { ok: true } })
      })
    }
    await page.goto('/?view=settings&tab=preferences')
    const shared = page.getByRole('button', { name: 'Shared (committed)', exact: true })
    const personal = page.getByRole('button', { name: 'Personal (not committed)', exact: true })
    await expect(shared).toHaveAttribute('aria-pressed', 'true')

    const model = page.locator('.field-row').filter({ has: page.getByText('Default model', { exact: true }) })
    await expect(model.locator('.badge', { hasText: 'personal' })).toHaveCount(0)
    await expect(model).toContainText('Your personal value overrides the shared value.')
    await expect(model.getByRole('textbox', { name: 'Default model value' })).toHaveValue('sonnet')
    await personal.click()
    await expect(model.locator('.badge', { hasText: 'personal' })).toBeVisible()
    await model.getByRole('button', { name: 'Reset to shared value' }).click()
    await expect.poll(() => posted.at(-1)).toEqual({ audience: 'personal', path: team, key: 'model', value: null, expected: 'opus' })

    await selectTab(page, 'Built-ins')
    const caveman = page.locator('.item-row').filter({ has: page.getByRole('switch', { name: 'caveman built-in' }) })
    await expect(caveman.locator('.badge', { hasText: 'personal' })).toBeVisible()
    await caveman.getByRole('button', { name: 'Reset caveman to shared value' }).click()
    await expect.poll(() => posted.at(-1)).toEqual({ audience: 'personal', name: 'caveman', enabled: false, expected: true })

    await personal.click()
    await expect(personal).toHaveAttribute('aria-pressed', 'true')
    await selectTab(page, 'Preferences')
    const effort = page.locator('.field-row').filter({ has: page.getByText('Reasoning effort', { exact: true }) })
    await expect(effort.locator('.badge', { hasText: 'personal' })).toHaveCount(0)
    await expect(effort).toContainText(`Saves to ${local}`)
    await effort.getByRole('textbox', { name: 'Reasoning effort value' }).fill('low')
    await effort.getByRole('button', { name: 'Save' }).click()
    await expect.poll(() => posted.at(-1)).toEqual({ audience: 'personal', path: team, key: 'effort', value: 'low', expected: null })
    await expect(model.getByRole('textbox', { name: 'Default model value' })).toHaveValue('sonnet')
    await selectTab(page, 'Built-ins')
    await page.getByRole('switch', { name: 'memory built-in' }).click()
    await expect.poll(() => posted.at(-1)).toEqual({ audience: 'personal', name: 'memory', enabled: true, expected: false })

    await page.reload()
    await expect(personal).toHaveAttribute('aria-pressed', 'true')
  })

  const refreshFailures: { name: string; fail: (route: import('@playwright/test').Route) => Promise<void>; message: RegExp }[] = [
    { name: 'an HTTP error', fail: (route) => route.fulfill({ status: 500, json: { error: 'audit-simulated-overview-failure' } }), message: /answered 500: audit-simulated-overview-failure/ },
    { name: 'a network failure', fail: (route) => route.abort('connectionrefused'), message: /Could not reach/ },
    { name: 'a malformed response', fail: (route) => route.fulfill({ status: 200, contentType: 'text/plain', body: 'not json' }), message: /did not return JSON/ },
  ]
  for (const failure of refreshFailures) {
    test(`shows ${failure.name} when the refresh after a save fails, then recovers on retry`, async ({ page }) => {
      await sourceOnly(page)
      const data = structuredClone(fixture)
      const settings = data.sections.find((section: { id: string }) => section.id === 'settings')
      const team = '.agnostic-ai/settings/defaults.yaml'
      const field = (key: string) => settings.fields.find((f: { key: string }) => f.key === key)
      const edit = { kind: 'text', path: team, paths: [team], syncCommand: 'agnostic-ai sync' }
      Object.assign(field('effort'), { set: [{ spec: 'defaults', path: team, values: [{ key: 'effort', type: 'text', value: 'high' }] }], editable: edit })
      Object.assign(field('model'), { set: [{ spec: 'defaults', path: team, values: [{ key: 'model', type: 'text', value: 'sonnet' }] }], editable: edit })
      let failing = false
      await page.route('**/api/views/overview', (route) => {
        if (failing) return failure.fail(route)
        const view = structuredClone(data)
        const effort = view.sections.find((section: { id: string }) => section.id === 'settings').fields.find((f: { key: string }) => f.key === 'effort')
        effort.set[0].values[0].value = saved
        return route.fulfill({ json: view })
      })
      let saved = 'high'
      await page.route('**/api/edit/field', (route) => {
        saved = route.request().postDataJSON().value
        return route.fulfill({ json: { ok: true } })
      })
      await page.goto('/?view=settings&tab=preferences')
      const effort = page.locator('.field-row').filter({ has: page.getByText('Reasoning effort', { exact: true }) })
      const model = page.locator('.field-row').filter({ has: page.getByText('Default model', { exact: true }) })
      await model.getByRole('textbox', { name: 'Default model value' }).fill('draft-model')

      failing = true
      await effort.getByRole('textbox', { name: 'Reasoning effort value' }).fill('low')
      await effort.getByRole('button', { name: 'Update' }).click()
      const alert = page.getByRole('alert').filter({ hasText: 'could not be refreshed' })
      await expect(alert).toContainText('Your change was saved, but the overview could not be refreshed.')
      await expect(alert).toContainText(failure.message)
      await expect(alert).toContainText('previous overview')
      await expect(effort.getByRole('button', { name: 'Update' })).toBeDisabled()
      await expect(model.getByRole('textbox', { name: 'Default model value' })).toBeDisabled()
      await expect(model.getByRole('textbox', { name: 'Default model value' })).toHaveValue('draft-model')
      await expect(page).toHaveURL(/view=settings/)

      await alert.getByRole('button', { name: 'Retry' }).click()
      await expect(alert).toBeVisible()
      failing = false
      await alert.getByRole('button', { name: 'Retry' }).click()
      await expect(alert).toHaveCount(0)
      await expect(effort.getByRole('textbox', { name: 'Reasoning effort value' })).toHaveValue('low')
      await expect(effort.getByRole('textbox', { name: 'Reasoning effort value' })).toBeEnabled()
      await expect(model.getByRole('textbox', { name: 'Default model value' })).toHaveValue('draft-model')
    })
  }

  test('keeps an unsaved draft and shows the changed saved value when a retry brings a different source', async ({ page }) => {
    await sourceOnly(page)
    const data = structuredClone(fixture)
    const settings = data.sections.find((section: { id: string }) => section.id === 'settings')
    const team = '.agnostic-ai/settings/defaults.yaml'
    const edit = { kind: 'text', path: team, paths: [team], syncCommand: 'agnostic-ai sync' }
    const model = settings.fields.find((f: { key: string }) => f.key === 'model')
    const effort = settings.fields.find((f: { key: string }) => f.key === 'effort')
    Object.assign(model, { set: [{ spec: 'defaults', path: team, values: [{ key: 'model', type: 'text', value: 'sonnet' }] }], editable: edit })
    Object.assign(effort, { set: [{ spec: 'defaults', path: team, values: [{ key: 'effort', type: 'text', value: 'high' }] }], editable: edit })
    let failing = false
    let onDisk = 'sonnet'
    await page.route('**/api/views/overview', (route) => {
      if (failing) return route.fulfill({ status: 500, json: { error: 'down' } })
      const view = structuredClone(data)
      view.sections.find((section: { id: string }) => section.id === 'settings').fields.find((f: { key: string }) => f.key === 'model').set[0].values[0].value = onDisk
      return route.fulfill({ json: view })
    })
    await page.route('**/api/edit/field', (route) => route.fulfill({ json: { ok: true } }))
    await page.goto('/?view=settings&tab=preferences')
    const modelRow = page.locator('.field-row').filter({ has: page.getByText('Default model', { exact: true }) })
    const effortRow = page.locator('.field-row').filter({ has: page.getByText('Reasoning effort', { exact: true }) })
    await modelRow.getByRole('textbox', { name: 'Default model value' }).fill('my-draft')

    failing = true
    await effortRow.getByRole('textbox', { name: 'Reasoning effort value' }).fill('low')
    await effortRow.getByRole('button', { name: 'Update' }).click()
    const alert = page.getByRole('alert').filter({ hasText: 'could not be refreshed' })
    await expect(alert).toBeVisible()
    await page.getByRole('button', { name: 'JSON' }).click()
    await expect(page.getByText('Not refreshed: this is the previous overview')).toBeVisible()
    await page.getByRole('button', { name: 'Close the JSON inspector' }).click()

    failing = false
    onDisk = 'opus'
    await alert.getByRole('button', { name: 'Retry' }).click()
    await expect(alert).toHaveCount(0)
    await expect(modelRow.getByRole('textbox', { name: 'Default model value' })).toHaveValue('my-draft')
    await expect(modelRow.getByRole('status')).toContainText('The saved value changed after you started editing')
    await expect(modelRow.getByRole('status')).toContainText('opus')
  })

  test('shows an error panel, not stale data, when the first overview load fails', async ({ page }) => {
    await page.route('**/api/views/overview', (route) => route.fulfill({ status: 500, json: { error: 'audit-simulated-overview-failure' } }))
    await page.goto('/?view=settings')
    await expect(page.getByRole('alert')).toContainText('audit-simulated-overview-failure')
    await expect(page.getByRole('alert')).not.toContainText('previous overview')
  })

  test('resets a personal boolean and the override of the spec chosen to save to', async ({ page }) => {
    await sourceOnly(page)
    const data = structuredClone(fixture)
    const settings = data.sections.find((section: { id: string }) => section.id === 'settings')
    const field = (key: string) => settings.fields.find((f: { key: string }) => f.key === key)
    const nativeLocal = '.agnostic-ai/local/settings/native.yaml'
    Object.assign(field('x-'), { set: [{ spec: 'native', path: nativeLocal, local: true, values: [{ key: 'x-claude.verbose', type: 'bool', value: 'true' }, { key: 'x-claude.theme', type: 'text', value: 'dark' }] }] })
    settings.items.push({ kind: 'settings', name: 'native', label: 'native', path: nativeLocal, layer: 'project-user', localKeys: ['x-claude.verbose'], values: [{ key: 'x-claude.verbose', type: 'bool', value: 'true' }], findings: [] })
    const [a, b, localB] = ['.agnostic-ai/settings/a.yaml', '.agnostic-ai/settings/b.yaml', '.agnostic-ai/local/settings/b.yaml']
    Object.assign(field('model'), {
      set: [
        { spec: 'b', path: localB, local: true, values: [{ key: 'model', type: 'text', value: 'opus' }] },
        { spec: 'b', path: b, values: [{ key: 'model', type: 'text', value: 'sonnet' }] },
        { spec: 'a', path: a, values: [{ key: 'model', type: 'text', value: 'haiku' }] },
      ],
      editable: { kind: 'text', path: a, paths: [a, b], localPath: '.agnostic-ai/local/settings/a.yaml', syncCommand: 'agnostic-ai sync' },
    })
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: data }))
    const posted: unknown[] = []
    for (const path of ['**/api/edit/field', '**/api/edit/value']) {
      await page.route(path, (route) => {
        posted.push(route.request().postDataJSON())
        return route.fulfill({ json: { ok: true } })
      })
    }
    await page.goto('/?view=settings&tab=preferences')
    await page.getByRole('button', { name: 'Shared (committed)', exact: true }).click()

    const native = page.locator('.field-row').filter({ has: page.getByText('Native keys', { exact: true }) })
    await page.getByRole('button', { name: 'Personal (not committed)', exact: true }).click()
    await expect(native.getByRole('button', { name: 'Reset x-claude.verbose to shared value' })).toHaveCount(1)
    await native.getByRole('button', { name: 'Reset x-claude.verbose to shared value' }).click()
    await expect.poll(() => posted.at(-1)).toEqual({ audience: 'personal', path: nativeLocal, key: 'x-claude.verbose', value: null, expected: true })

    const model = page.locator('.field-row').filter({ has: page.getByText('Default model', { exact: true }) })
    await page.getByRole('button', { name: 'Shared (committed)', exact: true }).click()
    await expect(model.getByRole('button', { name: 'Reset to shared value' })).toHaveCount(0)
    await model.getByRole('combobox').selectOption(b)
    await expect(model).toContainText('Your personal value overrides the shared value.')
    await model.getByRole('button', { name: 'View personal settings' }).click()
    await expect(model.getByRole('textbox', { name: 'Default model value' })).toHaveValue('opus')
    await model.getByRole('button', { name: 'Reset to shared value' }).click()
    await expect.poll(() => posted.at(-1)).toEqual({ audience: 'personal', path: b, key: 'model', value: null, expected: 'opus' })
  })

  test('shows a configured value from another source beside the editor and hides only the value the editor shows', async ({ page }) => {
    const data = structuredClone(fixture)
    const settings = data.sections.find((section: { id: string }) => section.id === 'settings')
    const field = (key: string) => settings.fields.find((f: { key: string }) => f.key === key)
    const [team, local, pack] = ['.agnostic-ai/settings/defaults.yaml', '.agnostic-ai/local/settings/defaults.yaml', '.agnostic-ai/packs/team/settings/team.yaml']
    const editable = { kind: 'text', path: team, paths: [team], localPath: local, syncCommand: 'agnostic-ai sync' }
    Object.assign(field('model'), { set: [{ spec: 'team', path: pack, values: [{ key: 'model', type: 'text', value: 'sonnet' }] }], editable })
    Object.assign(field('effort'), { set: [{ spec: 'defaults', path: local, local: true, values: [{ key: 'effort', type: 'text', value: 'low' }] }], editable })
    Object.assign(field('session-retention-days'), { set: [{ spec: 'defaults', path: team, values: [{ key: 'session-retention-days', type: 'text', value: '30' }] }], editable: { ...editable, kind: 'number' } })
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: data }))
    await page.goto('/?view=settings&tab=preferences')
    await page.getByRole('button', { name: 'Shared (committed)', exact: true }).click()

    const model = page.locator('.field-row').filter({ has: page.getByText('Default model', { exact: true }) })
    await expect(model.locator('.field-source')).toContainText('sonnet')
    await expect(model.locator('.field-source')).toContainText(`from ${pack}`)
    await expect(model.getByRole('textbox', { name: 'Default model value' })).toHaveAttribute('placeholder', 'Not set in this file')
    await expect(model).toContainText(`Saves to ${team}`)

    const effort = page.locator('.field-row').filter({ has: page.getByText('Reasoning effort', { exact: true }) })
    await page.getByText('Unconfigured fields', { exact: false }).click()
    await expect(effort).toContainText('Your personal value overrides the shared value.')
    await expect(effort.locator('.field-source')).toHaveCount(0)
    await expect(effort.getByRole('textbox', { name: 'Reasoning effort value' })).toHaveValue('')

    const retention = page.locator('.field-row').filter({ has: page.getByText('Session retention', { exact: true }) })
    await expect(retention.getByRole('spinbutton', { name: 'Session retention value' })).toHaveValue('30')
    await expect(retention.locator('.field-source')).toHaveCount(0)
    await expect(retention).toContainText(`from ${team}`)

    await page.getByRole('button', { name: 'Personal (not committed)', exact: true }).click()
    await expect(effort.getByRole('textbox', { name: 'Reasoning effort value' })).toHaveValue('low')
    await expect(effort.locator('.field-source')).toHaveCount(0)
    await expect(model.locator('.field-source')).toContainText(`from ${pack}`)
    await expect(retention.getByRole('spinbutton', { name: 'Session retention value' })).toHaveValue('30')
    await expect(retention.locator('.field-source')).toContainText(`from ${team}`)
  })

  test('opens the JSON inspector on the overview document', async ({ page, context }) => {
    await context.grantPermissions(['clipboard-read', 'clipboard-write'])
    await openApp(page)
    const button = page.getByRole('button', { name: 'JSON', exact: true })
    await button.click()
    const drawer = page.getByRole('dialog', { name: 'JSON inspector' })
    await expect(drawer.locator('.inspector-meta')).toContainText('overview/v1')
    await expect(button).toBeInViewport({ ratio: 1 })
    await expect(button).toHaveAttribute('aria-pressed', 'true')
    const headerBottom = await page.locator('.topbar').evaluate((element) => element.getBoundingClientRect().bottom)
    expect(await drawer.evaluate((element) => element.getBoundingClientRect().top)).toBeGreaterThanOrEqual(headerBottom - 1)
    await drawer.getByRole('button', { name: 'Copy JSON' }).click()
    await expect(drawer.getByRole('button', { name: 'JSON copied' })).toBeVisible()
    await page.keyboard.press('Escape')
    await expect(drawer).toHaveCount(0)
    await expect(button).toBeFocused()
  })

  test('edits global voice with a switch, mode menu, and language box', async ({ page }) => {
    await sourceOnly(page)
    const data = structuredClone(fixture)
    const fields = data.sections.find((section: { id: string }) => section.id === 'settings').fields
    const path = 'settings/defaults.yaml'
    for (const [key, kind, allowed] of [['voice.enabled', 'boolean', undefined], ['voice.mode', 'text', ['hold', 'tap']], ['voice.language', 'text', undefined]] as const) {
      Object.assign(fields.find((field: { key: string }) => field.key === key), { key, label: key === 'voice.enabled' ? 'Voice dictation' : key === 'voice.mode' ? 'Voice mode' : 'Voice language', description: '', set: [], editable: { kind, path, allowed, syncCommand: 'agnostic-ai sync --global' } })
    }
    const posted: Record<string, unknown>[] = []
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: data }))
    await page.route('**/api/edit/field', (route) => {
      const body = route.request().postDataJSON()
      posted.push(body)
      const field = fields.find((field: { key: string }) => field.key === body.key)
      field.set = body.value === null ? [] : [{ spec: 'defaults', path, values: [{ key: body.key, type: typeof body.value === 'boolean' ? 'bool' : 'text', value: String(body.value) }] }]
      return route.fulfill({ json: { ok: true } })
    })
    await page.goto('/?view=settings&tab=preferences')
    await page.getByRole('button', { name: 'Shared (committed)', exact: true }).click()
    await page.getByText('Unconfigured fields', { exact: false }).click()
    const voice = page.locator('.field-row').filter({ has: page.getByText('Voice dictation', { exact: true }) })
    await voice.getByRole('switch', { name: 'Voice dictation value' }).click()
    await voice.getByRole('button', { name: 'Save', exact: true }).click()
    await expect.poll(() => posted.at(-1)).toEqual({ audience: 'shared', path, key: 'voice.enabled', value: true, expected: null })
    await expect(voice.getByRole('switch')).toBeChecked()
    await voice.getByRole('switch').click()
    await voice.getByRole('button', { name: 'Update', exact: true }).click()
    await expect.poll(() => posted.at(-1)).toEqual({ audience: 'shared', path, key: 'voice.enabled', value: false, expected: 'true' })
    const mode = page.locator('.field-row').filter({ has: page.getByText('Voice mode', { exact: true }) })
    await mode.getByRole('combobox', { name: 'Voice mode value' }).selectOption('tap')
    await mode.getByRole('button', { name: 'Save', exact: true }).click()
    await expect.poll(() => posted.at(-1)).toMatchObject({ key: 'voice.mode', value: 'tap' })
    const language = page.locator('.field-row').filter({ has: page.getByText('Voice language', { exact: true }) })
    await language.getByRole('textbox', { name: 'Voice language value' }).fill('es')
    await language.getByRole('button', { name: 'Save', exact: true }).click()
    await expect.poll(() => posted.at(-1)).toMatchObject({ key: 'voice.language', value: 'es' })
    for (const field of [voice, mode, language]) {
      await expect(field).toContainText('Run agnostic-ai sync --global to apply.')
      await field.getByRole('button', { name: 'Clear', exact: true }).click()
      await expect.poll(() => posted.at(-1)?.value).toBeNull()
    }
  })

  test('shows configured project voice values read-only', async ({ page }) => {
    const data = structuredClone(fixture)
    const fields = data.sections.find((section: { id: string }) => section.id === 'settings').fields
    const reason = 'Voice is written only by sync --global. Open Settings in agnostic-ai ui --global.'
    for (const [key, label, value] of [['voice.enabled', 'Voice dictation', 'false'], ['voice.mode', 'Voice mode', 'tap'], ['voice.language', 'Voice language', 'es']]) {
      Object.assign(fields.find((field: { key: string }) => field.key === key), { key, label, description: '', set: [{ spec: 'voice', path: '.agnostic-ai/settings/voice.yaml', values: [{ key, type: key === 'voice.enabled' ? 'bool' : 'text', value }] }], editable: { kind: key === 'voice.enabled' ? 'boolean' : 'text', disabledReason: reason } })
    }
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: data }))
    await page.goto('/?view=settings&tab=preferences')
    for (const label of ['Voice dictation', 'Voice mode', 'Voice language']) {
      const field = page.locator('.field-row').filter({ has: page.getByText(label, { exact: true }) })
      await expect(field).toContainText(reason)
      await expect(field.getByRole('switch')).toHaveCount(0)
      await expect(field.getByRole('textbox')).toHaveCount(0)
      await expect(field.getByRole('combobox')).toHaveCount(0)
    }
  })

  test('starts a field edit on the spec in effect and links global instructions only for retention', async ({ page }) => {
    await sourceOnly(page)
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
    await page.goto('/?view=settings&tab=preferences')
    const model = page.locator('.field-row').filter({ has: page.getByText('Default model', { exact: true }) })
    await expect(model.getByRole('combobox')).toHaveValue(paths[1])
    await expect(model.getByRole('textbox', { name: 'Default model value' })).toHaveValue('opus')
    await expect(model).toContainText('Run agnostic-ai sync --global to apply.')
    await model.getByRole('textbox', { name: 'Default model value' }).fill('haiku')
    await model.getByRole('button', { name: 'Update' }).click()
    await expect.poll(() => edit).toEqual({ audience: 'shared', path: paths[1], key: 'model', value: 'haiku', expected: 'opus' })
    const effort = page.locator('.field-row').filter({ has: page.getByText('Reasoning effort', { exact: true }) })
    await expect(effort).toContainText('edit the spec directly')
    await expect(effort.locator('a', { hasText: 'Global view instructions' })).toHaveCount(0)
    const retention = page.locator('.field-row').filter({ has: page.getByText('Session retention', { exact: true }) })
    await expect(retention.locator('a', { hasText: 'Global view instructions' })).toHaveCount(1)
  })
})
