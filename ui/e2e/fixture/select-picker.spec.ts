import { expect, test, type Page } from '@playwright/test'
import { readFileSync } from 'node:fs'
import { selectTab } from '../helpers'

// Chromium supports base-select, so these tests hide it to take the path
// Safari and Firefox take: the listbox Select draws itself.

const overview = JSON.parse(readFileSync(new URL('../../src/fixtures/overview.mock.json', import.meta.url), 'utf8'))
const contextView = JSON.parse(readFileSync(new URL('../../src/fixtures/context.mock.json', import.meta.url), 'utf8'))

async function withoutBaseSelect(page: Page) {
  await page.addInitScript(() => {
    const supports = CSS.supports.bind(CSS) as (...args: string[]) => boolean
    CSS.supports = ((...args: string[]) => (args[0] === 'appearance' && args[1] === 'base-select') ? false : supports(...args)) as typeof CSS.supports
  })
}

async function openEditorSettings(page: Page) {
  await withoutBaseSelect(page)
  await page.route('**/api/views/overview', (route) => route.fulfill({ json: structuredClone(overview) }))
  await page.route('**/api/health', (route) => route.fulfill({ json: { ok: true, root: '/tmp/agnostic-ai', global: false, ui: true } }))
  await page.goto('/?view=settings&tab=editor')
  return page.getByRole('combobox', { name: 'Open files in' })
}

test.describe('dropdown without base-select', () => {
  test('opens from the keyboard, moves with arrows and chooses with Enter', async ({ page }) => {
    const picker = await openEditorSettings(page)
    await expect(picker).toHaveText('Cursor')
    await expect(picker).toHaveAttribute('aria-expanded', 'false')
    await picker.focus()
    await page.keyboard.press('ArrowDown')
    const list = page.getByRole('listbox', { name: 'Open files in' })
    await expect(list).toBeVisible()
    await expect(picker).toHaveAttribute('aria-expanded', 'true')
    await expect(list.getByRole('option', { selected: true })).toHaveText('Cursor')
    await page.keyboard.press('ArrowDown')
    await expect(list.getByRole('option', { selected: true })).toHaveText('VS Code')
    await expect(picker).toHaveAttribute('aria-activedescendant', await list.getByRole('option', { name: 'VS Code', exact: true }).getAttribute('id') ?? '')
    await page.keyboard.press('End')
    await expect(list.getByRole('option', { selected: true })).toHaveText('Custom link')
    await page.keyboard.press('Enter')
    await expect(list).toBeHidden()
    await expect(picker).toHaveText('Custom link')
    await expect(picker).toBeFocused()
    await expect(page.getByLabel('Link template')).toBeVisible()
  })

  test('jumps by typed letters and closes on Escape without a change', async ({ page }) => {
    const picker = await openEditorSettings(page)
    await picker.focus()
    await page.keyboard.press('Enter')
    const list = page.getByRole('listbox')
    await expect(list).toBeVisible()
    await page.keyboard.press('z')
    await expect(list.getByRole('option', { selected: true })).toHaveText('Zed')
    await page.keyboard.press('Escape')
    await expect(list).toBeHidden()
    await expect(picker).toBeFocused()
    await expect(picker).toHaveText('Cursor')
  })

  test('opens on click, places the panel under the button and closes on an outside click', async ({ page }) => {
    const picker = await openEditorSettings(page)
    await picker.click()
    const list = page.getByRole('listbox')
    await expect(list).toBeVisible()
    const button = (await picker.boundingBox())!
    const panel = (await list.boundingBox())!
    expect(panel.y).toBeGreaterThanOrEqual(button.y + button.height)
    expect(panel.width).toBeGreaterThanOrEqual(button.width - 1)
    expect(await list.evaluate((element) => getComputedStyle(element).backgroundColor)).not.toBe('rgba(0, 0, 0, 0)')
    await page.mouse.click(5, 5)
    await expect(list).toBeHidden()
    await expect(picker).toHaveText('Cursor')
    const label = page.locator('label', { hasText: 'Open files in' })
    await label.click()
    await expect(list).toBeVisible()
    await label.click()
    await expect(list).toBeHidden()
  })

  test('chooses a coding tool by click and reads its context', async ({ page }) => {
    await withoutBaseSelect(page)
    const view = { ...structuredClone(contextView), targets: ['claude', 'copilot', 'gemini'], fileTargets: ['claude'] }
    const requests: string[] = []
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: overview }))
    await page.route('**/api/views/context**', (route) => {
      const url = new URL(route.request().url())
      requests.push(url.search)
      return route.fulfill({ json: { ...view, target: url.searchParams.get('target') ?? 'claude' } })
    })
    await page.goto('/?view=context')
    await selectTab(page, 'Context size')
    const panel = page.locator('section.context-usage')
    const picker = panel.getByRole('combobox', { name: 'Coding tool' })
    await expect(picker).toHaveText('Claude Code')
    await picker.click()
    await page.getByRole('option', { name: 'GitHub Copilot' }).click()
    await expect(page.getByRole('listbox')).toBeHidden()
    await expect(picker).toHaveText('GitHub Copilot')
    await expect.poll(() => requests.at(-1)).toBe('?target=copilot')
    await expect(panel.getByLabel('Check against a file')).toBeDisabled()
  })

  test('chooses inside a wrapping label without opening again', async ({ page }) => {
    await withoutBaseSelect(page)
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: structuredClone(overview) }))
    await page.goto('/?view=lifecycle')
    await page.getByRole('button', { name: 'Add hook' }).click()
    const form = page.getByRole('form', { name: 'Add a hook' })
    const stage = form.getByRole('combobox', { name: 'Stage' })
    await stage.click()
    await page.getByRole('option', { name: 'After a file edit' }).click()
    await expect(stage).toHaveText('After a file edit')
    await expect(stage).toHaveAttribute('aria-expanded', 'false')
    await expect(page.getByRole('listbox')).toHaveCount(0)
    await stage.click()
    await page.getByRole('listbox', { name: 'Stage' }).click({ position: { x: 2, y: 20 } })
    await expect(page.getByRole('listbox', { name: 'Stage' })).toBeVisible()
  })

  test('keeps the open panel inside a 320px screen', async ({ page }) => {
    await page.setViewportSize({ width: 320, height: 640 })
    const picker = await openEditorSettings(page)
    await picker.click()
    const list = page.getByRole('listbox')
    await expect(list).toBeVisible()
    const box = (await list.boundingBox())!
    expect(box.x).toBeGreaterThanOrEqual(0)
    expect(box.x + box.width).toBeLessThanOrEqual(320)
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(320)
  })
})
