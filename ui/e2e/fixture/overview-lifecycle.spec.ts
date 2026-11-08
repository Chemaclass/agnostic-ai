import { expect, test } from '@playwright/test'
import { readFileSync } from 'node:fs'
import { saveScreenshot, findingDetails } from '../helpers'
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

  test('opens lifecycle stages one at a time or all at once when a hook exists', async ({ page }) => {
    const data = structuredClone(fixture)
    const lifecycle = data.sections.find((section: { id: string }) => section.id === 'lifecycle')
    const hook: SectionItem = { kind: 'hook', name: 'prepare', label: 'Prepare session', description: '', path: '.agnostic-ai/hooks/prepare.yaml', layer: 'project', group: 'session-start', values: [], findings: [] }
    lifecycle.items = [hook]
    lifecycle.kinds[0].count = 1
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: data }))
    await page.goto('/?view=lifecycle')
    await expect(page.getByRole('heading', { level: 1, name: 'Lifecycle' })).toBeVisible()
    await expect(page.locator('.stage-empty')).toHaveCount(0)
    await expect(page.locator('.stage-count')).toHaveText('1 hook')
    await expect(page.locator('details.stage')).toHaveCount(7)
    await expect(page.locator('.stage[open]')).toHaveCount(0)
    await expect(page.locator('.stages > li > details.stage')).toHaveCount(7)
    await expect(page.locator('.stage summary .stage-description')).toHaveCount(7)
    await expect(page.locator('.stage summary .stage-count')).toHaveCount(1)
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
    await off.locator('summary').click()
    await expect(off.locator('.item-row.is-off')).toContainText('Turned off')
    await expect(live.locator('.item-row.is-off')).toHaveCount(0)
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
    await (await findingDetails(finding)).getByRole('button', { name: 'Recheck', exact: true }).click()
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
    await (await findingDetails(finding)).getByRole('button', { name: 'Recheck', exact: true }).click()
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
