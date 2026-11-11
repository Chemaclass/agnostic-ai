import { expect, test } from '@playwright/test'
import { readFileSync } from 'node:fs'
import { MOCK, openApp, saveScreenshot, selectTab } from '../helpers'

const fixture = JSON.parse(readFileSync(new URL('../../src/fixtures/overview.mock.json', import.meta.url), 'utf8'))

async function sourceOnly(page: import('@playwright/test').Page) {
  await page.route('**/api/changes/builtin', (route) => route.fulfill({ json: { schemaVersion: 'builtin-change/v1', status: 'idle', enabled: false, supported: false, undoAvailable: false, targets: [], changes: [], findings: [] } }))
}

test.describe('overview', () => {
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
    await expect(page.getByRole('button', { name: 'Reload' })).toBeVisible()
    await expect(toggle).toHaveAttribute('aria-checked', 'true')
    expect(sent).toEqual({ audience: 'shared', path: '.agnostic-ai/settings/native.yaml', key: 'x-claude.verbose', value: true, expected: false })
    await page.getByRole('button', { name: 'Reload' }).click()
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
    await expect(page.getByRole('button', { name: 'Reload' })).toBeVisible()
    await expect(toggle).toHaveAttribute('aria-checked', before === 'true' ? 'false' : 'true')
    expect(sent).toEqual({ audience: 'shared', name: 'memory', enabled: before !== 'true', expected: before === 'true' })
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
    await finding.getByRole('button', { name: 'Copy prompt' }).click()
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
      // A code the UI has no plain title for shows its raw message, collapsed when long.
      { source: 'lint', severity: 'warn', code: 'LINT099', message, path: '.agnostic-ai/local/memory/MEMORY.md' },
      { source: 'doctor', severity: 'error', message: 'The configuration file cannot be read.' },
    ]
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: data }))
    await context.grantPermissions(['clipboard-read', 'clipboard-write'])
    await page.goto('/?view=settings')
    const finding = page.locator('.finding', { hasText: 'LINT099' })
    const text = finding.locator('.finding-message')
    const expand = finding.getByRole('button', { name: 'Show full message', exact: true })
    await expect(expand).toHaveAttribute('aria-expanded', 'false')
    const collapsedHeight = await text.evaluate((element) => element.clientHeight)
    expect(await text.evaluate((element) => element.scrollHeight)).toBeGreaterThan(collapsedHeight)
    await finding.getByRole('button', { name: 'Copy prompt' }).click()
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
    await expect(page.getByRole('tab', { name: /^Built-ins/ })).toBeInViewport()
    await review.click()
    await expect(page.getByRole('heading', { name: 'Other findings', exact: true })).toBeFocused()
    await expect(page.getByText('Your tools load more text at startup than the startup budget allows.', { exact: true })).toBeInViewport()
    await expect(page.locator('.topbar')).toBeInViewport({ ratio: 1 })
    await expect(page.locator('.sidenav')).toBeInViewport({ ratio: 1 })
  })

  test('shows built-ins as switches and config values as chips and text', async ({ page }) => {
    await openApp(page, '?view=settings')
    await expect(page.getByRole('heading', { level: 1, name: 'Settings' })).toBeVisible()
    const builtins = page.locator('.item-row', { has: page.locator('.switch') })
    await expect(builtins).toHaveCount(6)
    await expect(builtins.filter({ hasText: 'memory' }).locator('.switch-text')).toHaveText(/On|Off/)
    await selectTab(page, 'Config files')
    const config = page.locator('.item-row', { hasText: 'agnostic-ai.yaml' })
    const open = config.getByRole('link', { name: /^Open .+ in Cursor$/ })
    if (MOCK) await expect(open).toHaveCount(0)
    else await expect(open).toHaveAttribute('href', /^cursor:\/\/file\/.+\.yaml$/)
    await expect(page.locator('.effective-config .value-row', { hasText: 'targets' }).locator('.tool-avatar').first()).toBeVisible()
    await saveScreenshot(page, '03-settings.png')
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
    // The row menu swaps in after a resize re-renders, so the check retries until layout settles.
    const aligned = () => expect(async () => {
      const first = await toggle.boundingBox()
      const other = await memory.boundingBox()
      expect(first).not.toBeNull()
      expect(other).not.toBeNull()
      const desktop = page.viewportSize()!.width > 760
      const edge = (box: NonNullable<typeof first>) => box.x + (desktop ? box.width : 0)
      expect(Math.abs(edge(first!) - edge(other!))).toBeLessThan(1)
    }).toPass()
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
    const shared = page.getByRole('button', { name: 'Project (committed)', exact: true })
    const personal = page.getByRole('button', { name: 'Local (not committed)', exact: true })
    await expect(shared).toHaveAttribute('aria-pressed', 'true')

    const model = page.locator('.field-row').filter({ has: page.getByText('Default model', { exact: true }) })
    await expect(model.locator('.badge', { hasText: 'local' })).toHaveCount(0)
    await expect(model).toContainText('Your local value overrides the project value.')
    await expect(model.getByRole('textbox', { name: 'Default model value' })).toHaveValue('sonnet')
    await personal.click()
    await expect(model.locator('.badge', { hasText: 'local' })).toBeVisible()
    await model.getByRole('button', { name: 'Reset to the project value' }).click()
    await expect.poll(() => posted.at(-1)).toEqual({ audience: 'personal', path: team, key: 'model', value: null, expected: 'opus' })

    await selectTab(page, 'Built-ins')
    const caveman = page.locator('.item-row').filter({ has: page.getByRole('switch', { name: 'caveman built-in' }) })
    await expect(caveman.locator('.badge', { hasText: 'local' })).toBeVisible()
    await caveman.getByRole('button', { name: 'Reset caveman to the project value' }).click()
    await expect.poll(() => posted.at(-1)).toEqual({ audience: 'personal', name: 'caveman', enabled: false, expected: true })

    await personal.click()
    await expect(personal).toHaveAttribute('aria-pressed', 'true')
    await selectTab(page, 'Preferences')
    const effort = page.locator('.field-row').filter({ has: page.getByText('Reasoning effort', { exact: true }) })
    await expect(effort.locator('.badge', { hasText: 'local' })).toHaveCount(0)
    await expect(effort).toContainText(`Saves to ${local}`)
    await effort.getByRole('textbox', { name: 'Reasoning effort value' }).fill('low')
    await effort.getByRole('button', { name: 'Apply', exact: true }).click()
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
      await effort.getByRole('button', { name: 'Apply', exact: true }).click()
      const alert = page.getByRole('alert').filter({ hasText: 'could not be refreshed' })
      await expect(alert).toContainText('Your change was saved, but the overview could not be refreshed.')
      await expect(alert).toContainText(failure.message)
      await expect(alert).toContainText('previous overview')
      await expect(effort.getByRole('button', { name: 'Apply', exact: true })).toBeDisabled()
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
    await effortRow.getByRole('button', { name: 'Apply', exact: true }).click()
    const alert = page.getByRole('alert').filter({ hasText: 'could not be refreshed' })
    await expect(alert).toBeVisible()
    await page.getByRole('button', { name: 'Overview JSON', exact: true }).click()
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
    await page.getByRole('button', { name: 'Project (committed)', exact: true }).click()

    const native = page.locator('.field-row').filter({ has: page.getByText('Native keys', { exact: true }) })
    await page.getByRole('button', { name: 'Local (not committed)', exact: true }).click()
    await expect(native.getByRole('button', { name: 'Reset x-claude.verbose to the project value' })).toHaveCount(1)
    await native.getByRole('button', { name: 'Reset x-claude.verbose to the project value' }).click()
    await expect.poll(() => posted.at(-1)).toEqual({ audience: 'personal', path: nativeLocal, key: 'x-claude.verbose', value: null, expected: true })

    const model = page.locator('.field-row').filter({ has: page.getByText('Default model', { exact: true }) })
    await page.getByRole('button', { name: 'Project (committed)', exact: true }).click()
    await expect(model.getByRole('button', { name: 'Reset to the project value' })).toHaveCount(0)
    await model.getByRole('combobox').selectOption(b)
    await expect(model).toContainText('Your local value overrides the project value.')
    await model.getByRole('button', { name: 'View local settings' }).click()
    await expect(model.getByRole('textbox', { name: 'Default model value' })).toHaveValue('opus')
    await model.getByRole('button', { name: 'Reset to the project value' }).click()
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
    await page.getByRole('button', { name: 'Project (committed)', exact: true }).click()

    const model = page.locator('.field-row').filter({ has: page.getByText('Default model', { exact: true }) })
    await expect(model.locator('.field-source')).toContainText('sonnet')
    await expect(model.locator('.field-source')).toContainText(`from ${pack}`)
    await expect(model.getByRole('textbox', { name: 'Default model value' })).toHaveAttribute('placeholder', 'Not set in this file')
    await expect(model).toContainText(`Saves to ${team}`)

    const effort = page.locator('.field-row').filter({ has: page.getByText('Reasoning effort', { exact: true }) })
    await page.getByText('Unconfigured fields', { exact: false }).click()
    await expect(effort).toContainText('Your local value overrides the project value.')
    await expect(effort.locator('.field-source')).toHaveCount(0)
    await expect(effort.getByRole('textbox', { name: 'Reasoning effort value' })).toHaveValue('')

    const retention = page.locator('.field-row').filter({ has: page.getByText('Session retention', { exact: true }) })
    await expect(retention.getByRole('spinbutton', { name: 'Session retention value' })).toHaveValue('30')
    await expect(retention.locator('.field-source')).toHaveCount(0)
    await expect(retention).toContainText(`from ${team}`)

    await page.getByRole('button', { name: 'Local (not committed)', exact: true }).click()
    await expect(effort.getByRole('textbox', { name: 'Reasoning effort value' })).toHaveValue('low')
    await expect(effort.locator('.field-source')).toHaveCount(0)
    await expect(model.locator('.field-source')).toContainText(`from ${pack}`)
    await expect(retention.getByRole('spinbutton', { name: 'Session retention value' })).toHaveValue('30')
    await expect(retention.locator('.field-source')).toContainText(`from ${team}`)
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
    await page.getByRole('button', { name: 'Project (committed)', exact: true }).click()
    await page.getByText('Unconfigured fields', { exact: false }).click()
    const voice = page.locator('.field-row').filter({ has: page.getByText('Voice dictation', { exact: true }) })
    await voice.getByRole('switch', { name: 'Voice dictation value' }).click()
    await voice.getByRole('button', { name: 'Apply', exact: true }).click()
    await expect.poll(() => posted.at(-1)).toEqual({ audience: 'shared', path, key: 'voice.enabled', value: true, expected: null })
    await expect(voice.getByRole('switch')).toBeChecked()
    await voice.getByRole('switch').click()
    await voice.getByRole('button', { name: 'Apply', exact: true }).click()
    await expect.poll(() => posted.at(-1)).toEqual({ audience: 'shared', path, key: 'voice.enabled', value: false, expected: 'true' })
    const mode = page.locator('.field-row').filter({ has: page.getByText('Voice mode', { exact: true }) })
    expect(await mode.getByRole('combobox', { name: 'Voice mode value' }).evaluate((element) => getComputedStyle(element).paddingRight)).toBe('36px')
    await mode.getByRole('combobox', { name: 'Voice mode value' }).selectOption('tap')
    await mode.getByRole('button', { name: 'Apply', exact: true }).click()
    await expect.poll(() => posted.at(-1)).toMatchObject({ key: 'voice.mode', value: 'tap' })
    const language = page.locator('.field-row').filter({ has: page.getByText('Voice language', { exact: true }) })
    await language.getByRole('textbox', { name: 'Voice language value' }).fill('es')
    await language.getByRole('button', { name: 'Apply', exact: true }).click()
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
    await model.getByRole('button', { name: 'Apply', exact: true }).click()
    await expect.poll(() => edit).toEqual({ audience: 'shared', path: paths[1], key: 'model', value: 'haiku', expected: 'opus' })
    const effort = page.locator('.field-row').filter({ has: page.getByText('Reasoning effort', { exact: true }) })
    await expect(effort).toContainText('edit the spec directly')
    await expect(effort.locator('a', { hasText: 'Global view instructions' })).toHaveCount(0)
    const retention = page.locator('.field-row').filter({ has: page.getByText('Session retention', { exact: true }) })
    await expect(retention.locator('a', { hasText: 'Global view instructions' })).toHaveCount(1)
  })
})
