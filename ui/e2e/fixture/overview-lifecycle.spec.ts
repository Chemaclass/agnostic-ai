import { expect, test } from '@playwright/test'
import { readFileSync } from 'node:fs'
import { saveScreenshot } from '../helpers'
import { findingEntryIds, itemEntryId } from '../../src/model/identity'
import type { SectionItem } from '../../src/contract'

const fixture = JSON.parse(readFileSync(new URL('../../src/fixtures/overview.mock.json', import.meta.url), 'utf8'))

test.describe('overview', () => {
  test('lists every lifecycle stage in session order, even with no hooks', async ({ page }) => {
    await page.goto('./?view=lifecycle&mock=1')
    await expect(page.getByRole('heading', { level: 1, name: 'Lifecycle' })).toBeVisible()
    const stages = page.locator('.stage-title')
    await expect(stages.nth(0)).toHaveText('Session start')
    await expect(stages.nth(6)).toHaveText('Session end')
    await expect(page.locator('.stages > li.stage-step')).toHaveCount(7)
    await expect(page.locator('.stage-description')).toHaveCount(7)
    await expect(page.locator('.stage-description').first()).toBeVisible()
    const hooks = page.getByRole('region', { name: 'Hooks', exact: true })
    await expect(hooks.getByRole('heading', { level: 2, name: 'Hooks', exact: true })).toBeVisible()
    await expect(hooks.locator('.empty-state-title')).toHaveText('No hooks yet.')
    await expect(hooks).not.toContainText('None configured.')
    await expect(page.locator('details.stage')).toHaveCount(0)
    await expect(page.locator('.stage-count')).toHaveCount(0)
    await expect(page.getByRole('button', { name: 'Expand all', exact: true })).toHaveCount(0)
    await expect(page.getByRole('button', { name: 'Collapse all', exact: true })).toHaveCount(0)
    await expect(page.getByRole('button', { name: 'Add hook', exact: true })).toHaveCount(0)
    const avatar = page.locator('.stage').first().locator('.tool-avatar').first()
    await expect(avatar).toHaveAttribute('title', /^Claude Code - SessionStart$/)
    await expect(avatar.locator('img')).toBeVisible()
    await saveScreenshot(page, '02-lifecycle-empty.png')
  })

  test('shows the stages with hooks, opens them one at a time or all at once, and reveals every stage', async ({ page }) => {
    const data = structuredClone(fixture)
    const lifecycle = data.sections.find((section: { id: string }) => section.id === 'lifecycle')
    const hook: SectionItem = { kind: 'hook', name: 'prepare', label: 'Prepare session', description: '', path: '.agnostic-ai/hooks/prepare.yaml', layer: 'project', group: 'session-start', values: [], findings: [] }
    lifecycle.items = [hook]
    lifecycle.kinds[0].count = 1
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: data }))
    await page.goto('/?view=lifecycle')
    await expect(page.getByRole('heading', { level: 1, name: 'Lifecycle' })).toBeVisible()
    await expect(page.locator('.page-verdict')).toHaveText('1 hook runs when a session starts.')
    await expect(page.locator('.page-facts')).toContainText('6 stages have no hook')
    await expect(page.locator('.stage-count')).toHaveText('1 hook')
    await expect(page.locator('details.stage')).toHaveCount(1)
    await expect(page.locator('.stage[open]')).toHaveCount(0)
    await expect(page.locator('.stage-title')).toHaveText(['Session start'])
    await expect(page.locator('.stage-summary')).not.toContainText('01')
    await saveScreenshot(page, '02-lifecycle-collapsed.png')
    await page.locator('.stage').first().locator('.stage-head').focus()
    await page.keyboard.press('Enter')
    await expect(page.locator('.stage[open]')).toHaveCount(1)
    const block = page.locator('.hook-block')
    await expect(block.locator('.hook-block-title')).toHaveText('Prepare session')
    await expect(block.locator('.hook-block-meta')).toHaveText('Project .agnostic-ai/hooks/prepare.yaml')
    await expect(page.locator('.stage').first().locator('.tool-avatar').first()).toHaveAttribute('title', /^Claude Code - /)
    await page.getByRole('button', { name: 'Collapse all', exact: true }).click()
    await expect(page.locator('.stage[open]')).toHaveCount(0)

    const more = page.getByRole('button', { name: 'Show all 7 stages', exact: true })
    await more.click()
    await expect(page.locator('details.stage')).toHaveCount(7)
    await expect(page.locator('.stage-title')).toHaveText(data.sections.find((section: { id: string }) => section.id === 'lifecycle').stages.map((stage: { label: string }) => stage.label))
    await page.getByRole('button', { name: 'Expand all', exact: true }).click()
    await expect(page.locator('.stage[open]')).toHaveCount(7)
    const avatar = page.locator('.stage').nth(1).locator('.tool-avatar').first()
    await expect(avatar).toHaveAttribute('title', /^Claude Code - /)
    await page.getByRole('button', { name: 'Show only stages with hooks', exact: true }).click()
    await expect(page.locator('details.stage')).toHaveCount(1)

    await page.getByRole('link', { name: 'Context', exact: false }).first().click()
    await expect(page.getByRole('button', { name: 'Expand all', exact: true })).toHaveCount(0)
    await page.getByRole('link', { name: 'Lifecycle', exact: false }).first().click()
    await expect(page.locator('.stage[open]')).toHaveCount(0)
    await saveScreenshot(page, '02-lifecycle.png')
  })

  test('groups tool-specific events and shows a built-in hook with its call and where to change it', async ({ page, context }) => {
    const data = structuredClone(fixture)
    const lifecycle = data.sections.find((section: { id: string }) => section.id === 'lifecycle')
    const launcher = `sh -c 'd=$(pwd); bin="$d/node_modules/.bin/agnostic-ai"; [ -x "$bin" ] || bin=agnostic-ai; exec "$bin" project -- hook memory'`
    lifecycle.stages.push(
      { id: 'native:SubagentStart', label: 'SubagentStart', description: "A tool's own event name: it runs only on the tool that defines SubagentStart.", targets: [] },
      { id: 'native:TaskStart', label: 'TaskStart', description: "A tool's own event name: it runs only on the tool that defines TaskStart.", targets: [] },
    )
    const builtin = (name: string, group: string, description: string): SectionItem => ({ kind: 'hook', name, label: name, description, path: `builtin:${name}`, layer: 'builtin', group, values: [], fields: [{ key: 'command', type: 'text', value: launcher }], findings: [] })
    lifecycle.items = [
      builtin('memory-session-start', 'session-start', 'Load the shared memory index into the session.'),
      builtin('memory-subagent-start', 'native:SubagentStart', 'Load the personal memory index into each Claude Code subagent.'),
      builtin('memory-cline-task-start', 'native:TaskStart', 'Load the shared memory index when a Cline task starts.'),
    ]
    lifecycle.kinds[0].count = 3
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: data }))
    await page.goto('/?view=lifecycle')
    await expect(page.locator('.page-verdict')).toHaveText('3 hooks run at 3 stages.')
    await expect(page.locator('.stage-title')).toHaveText(['Session start', 'SubagentStart', 'TaskStart'])
    await expect(page.getByRole('heading', { level: 3, name: 'Events only one tool uses' })).toBeVisible()
    const native = page.getByRole('list', { name: 'Events only one tool uses' })
    await expect(native.locator('.stage-description')).toHaveText(['Load the personal memory index into each Claude Code subagent.', 'Load the shared memory index when a Cline task starts.'])
    await expect(page.getByText("A tool's own event name")).toHaveCount(0)
    await expect(page.getByRole('button', { name: 'Show all 9 stages', exact: true })).toBeVisible()

    await page.locator('.stage').first().locator('.stage-head').click()
    const block = page.locator('.hook-block').first()
    await expect(block.locator('.hook-block-title')).toHaveText('Load the shared memory index into the session.')
    await expect(block.locator('.hook-block-meta')).toHaveText('Built in')
    await expect(block.locator('code:visible')).toHaveCount(0)
    const command = block.locator('details.hook-command')
    await command.getByText('Command', { exact: true }).click()
    await expect(command.locator('.hook-call > code')).toHaveText('agnostic-ai project -- hook memory')
    await expect(command.locator('.hook-name')).toHaveText('memory-session-start')
    const full = command.locator('details.hook-launcher')
    await expect(full.locator('code')).toBeHidden()
    await full.getByText('Full command').click()
    await expect(full.locator('code')).toHaveText(launcher)
    await context.grantPermissions(['clipboard-read', 'clipboard-write'])
    await command.getByRole('button', { name: 'Copy command of memory-session-start' }).click()
    expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(launcher)
    await saveScreenshot(page, '02-lifecycle-builtin.png')

    await block.getByRole('link', { name: 'Change in Settings' }).click()
    await expect(page).toHaveURL(/view=settings&tab=builtins/)
    await expect(page.getByRole('tab', { name: /^Built-ins/ })).toHaveAttribute('aria-selected', 'true')
  })

  test('puts Add hook at the end of the toolbar', async ({ page }) => {
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: structuredClone(fixture) }))
    await page.goto('/?view=lifecycle')
    const add = page.locator('.section-toolbar-action').getByRole('button', { name: 'Add hook', exact: true })
    await expect(add).toHaveClass(/button-primary/)
    await expect(page.getByRole('search', { name: 'Search Lifecycle' }).getByRole('button', { name: 'Add hook' })).toHaveCount(0)
    await add.click()
    await expect(page.getByRole('form', { name: 'Add a hook' })).toBeVisible()
    await expect(add).toHaveAttribute('aria-expanded', 'true')
    await page.getByRole('button', { name: 'Cancel', exact: true }).click()
    await expect(page.getByRole('form', { name: 'Add a hook' })).toHaveCount(0)
    await page.goto('/?view=context')
    await expect(page.locator('.section-toolbar-action')).toBeEmpty()
  })

  test('shows a turned-off hook as off in its stage', async ({ page }) => {
    const data = structuredClone(fixture)
    const lifecycle = data.sections.find((section: { id: string }) => section.id === 'lifecycle')
    const hook = (name: string, group: string, off: boolean): SectionItem => ({ kind: 'hook', name, label: name, description: '', path: `.agnostic-ai/hooks/${name}.yaml`, layer: 'project', group, values: [], fields: off ? [{ key: 'disabled', type: 'bool', value: 'true' }] : [], findings: [] })
    lifecycle.items = [hook('format', 'after-edit', false), hook('old-check', 'stop', true)]
    lifecycle.kinds[0].count = 2
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: data }))
    await page.goto('/?view=lifecycle')
    const live = page.locator('.stage-step').filter({ hasText: 'After a file edit' })
    const off = page.locator('.stage-step').filter({ hasText: 'Stop' })
    await expect(live).toHaveClass(/has-hooks/)
    await expect(off).not.toHaveClass(/has-hooks/)
    await expect(off).toHaveClass(/is-off/)
    await expect(off.locator('.stage-count')).toHaveText('1 turned off')
    await off.locator('.stage-head').click()
    await expect(off.locator('.hook-block.is-off')).toContainText('Turned off')
    await expect(live.locator('.hook-block.is-off')).toHaveCount(0)
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
    await expect(feedback).toContainText('This finding no longer appears.')
    await expect(page.getByRole('searchbox', { name: 'Search lifecycle' })).toHaveValue('no-such-hook')
    await expect(stage).toHaveAttribute('open', '')
    await page.getByRole('button', { name: 'Collapse all', exact: true }).click()
    await expect(stage).not.toHaveAttribute('open')
    await page.getByRole('button', { name: 'Expand all', exact: true }).click()
    await expect(feedback).toBeVisible()
    await feedback.getByRole('button', { name: 'Dismiss', exact: true }).click()
    await expect(feedback).toHaveCount(0)
    // The hook no longer has a finding or a match, so its stage leaves the filtered list.
    await expect(page.locator('details.stage')).toHaveCount(0)
    await expect(page.locator('.stage-empty')).toHaveText('No matching hooks.')
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
    await expect(feedback).toContainText('The configuration check stopped before it finished')
    await expect(stage).toHaveAttribute('open', '')
    const search = page.getByRole('searchbox', { name: 'Search lifecycle' })
    await search.fill('no-such-hook')
    await expect(search).toBeFocused()
    await expect(feedback).toBeVisible()
    await expect(stage.locator('.stage-count')).toHaveCount(0)
    await page.getByRole('button', { name: 'Collapse all', exact: true }).click()
    await expect(stage).not.toHaveAttribute('open')
    await page.getByRole('button', { name: 'Expand all', exact: true }).click()
    await expect(feedback).toBeVisible()
  })
})
