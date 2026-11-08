import { expect, test, type Page } from '@playwright/test'
import { readFileSync } from 'node:fs'

const overview = JSON.parse(readFileSync(new URL('../../src/fixtures/overview.mock.json', import.meta.url), 'utf8'))
const contextView = JSON.parse(readFileSync(new URL('../../src/fixtures/context.mock.json', import.meta.url), 'utf8'))

async function serve(page: Page, view = contextView, data = overview) {
  await page.route('**/api/views/overview', (route) => route.fulfill({ json: data }))
  await page.route('**/api/health', (route) => route.fulfill({ json: { ok: true } }))
  await page.route('**/api/views/context**', (route) => route.fulfill({ json: view }))
}

test.describe('landing', () => {
  test('opens Context on Context size with the numbers, the verdict and next steps', async ({ page, context }) => {
    await serve(page)
    await page.goto('/?view=context')
    await expect(page.getByRole('tab', { name: /^Context size/ })).toHaveAttribute('aria-selected', 'true')
    expect(new URL(page.url()).searchParams.has('tab')).toBe(false)
    await expect(page.getByRole('searchbox', { name: 'Search context' })).toBeVisible()
    await page.reload()
    await expect(page.getByRole('tab', { name: /^Context size/ })).toHaveAttribute('aria-selected', 'true')
    expect(new URL(page.url()).searchParams.has('tab')).toBe(false)
    await expect(page.locator('.page-verdict')).toHaveText('Agents read 184 words when a session starts.')
    await expect(page.locator('.page-facts')).toContainText('316 more on demand')
    await expect(page.locator('.page-facts')).toContainText('Within the 2,000-word budget')
    const panel = page.locator('section.context-usage')
    const budget = panel.locator('.context-budget')
    await expect(budget).toContainText('184 of 2,000 words at startup')
    await expect(budget.getByRole('link', { name: 'Change budget' })).toHaveAttribute('href', 'https://agnostic-ai.org/docs/configuration/#lint')
    await expect(page.getByRole('region', { name: 'Next' })).toHaveCount(0)
    await expect(panel.locator('.context-summary')).not.toContainText(/token|faster|cheaper|context window|lint/i)
    const method = panel.locator('.context-usage-method')
    await expect(method.locator('.context-model')).toBeHidden()
    await method.getByText('How this is counted').click()
    await expect(method.locator('.context-model')).toContainText('No default model or reasoning effort is set for Claude Code')
    await expect(method).toContainText('Startup text loads at the start of every session.')
    await expect(method).toContainText('In bytes: 1,212 at startup, 2,084 on demand.')
    await context.grantPermissions(['clipboard-read', 'clipboard-write'])
    await method.getByRole('button', { name: 'Copy command: agnostic-ai explain --context --target claude' }).click()
    expect(await page.evaluate(() => navigator.clipboard.readText())).toBe('agnostic-ai explain --context --target claude')
    await panel.getByRole('button', { name: 'Copy prompt to review what Claude Code reads at every start' }).click()
    const prompt = await page.evaluate(() => navigator.clipboard.readText())
    expect(prompt).toContain('Review everything claude loads at the start of every session')
    expect(prompt).toContain('Startup is 184 words, within the 2,000-word lint budget (lint.instructions-words).')
    expect(prompt).toContain('- .agnostic-ai/AGNOSTIC_AI.md (entry-point layer): 120 words, 790 bytes, 65% of startup')
    expect(prompt).not.toMatch(/[–—]/)
  })

  test('keeps the words verdict on every Context tab and loads the report once', async ({ page }) => {
    let loads = 0
    await serve(page)
    await page.route('**/api/views/context**', (route) => { loads += 1; return route.fulfill({ json: contextView }) })
    await page.goto('/?view=context&tab=rules')
    await expect(page.locator('.page-verdict')).toHaveText('Agents read 184 words when a session starts.')
    await page.getByRole('tab', { name: /^Context size/ }).click()
    await expect(page.locator('.context-usage-totals')).toBeVisible()
    await page.getByRole('tab', { name: /^Skills/ }).click()
    await expect(page.locator('.page-verdict')).toHaveText('Agents read 184 words when a session starts.')
    expect(loads).toBe(1)
  })

  test('says when startup is over the budget and links the model line to Settings', async ({ page }) => {
    const over = structuredClone(contextView)
    over.startupWordBudget = 150
    const data = structuredClone(overview)
    const model = data.sections.find((s: { id: string }) => s.id === 'settings').fields.find((f: { key: string }) => f.key === 'model')
    model.resolved[0] = { ...model.resolved[0], value: 'sonnet', alias: 'claude-sonnet-4-5' }
    await serve(page, over, data)
    await page.goto('/?view=context')
    const panel = page.locator('section.context-usage')
    await expect(panel.locator('.context-budget')).toContainText('184 of 150 words at startup, 34 over')
    await expect(panel.locator('.context-budget')).toHaveClass(/is-over/)
    await expect(page.locator('.page-facts')).toContainText('34 words over the 150-word budget')
    const method = panel.locator('.context-usage-method')
    await method.getByText('How this is counted').click()
    await expect(method.locator('.context-model')).toContainText('Model for Claude Code: sync writes sonnet (resolved from claude-sonnet-4-5).')
    await method.getByRole('link', { name: 'Change in Settings' }).click()
    await expect(page).toHaveURL(/view=settings&tab=preferences&entry=field%3Amodel/)
    await expect(page.getByRole('tab', { name: /^Preferences/ })).toHaveAttribute('aria-selected', 'true')
    await expect(page.locator('.field-row.is-linked')).toContainText('Default model')
  })

  test('shows only the size when the server sends no budget', async ({ page }) => {
    const noBudget = structuredClone(contextView)
    delete noBudget.startupWordBudget
    await serve(page, noBudget)
    await page.goto('/?view=context')
    const panel = page.locator('section.context-usage')
    await expect(panel.locator('.context-total-words').first()).toHaveText('184 words')
    await expect(panel.locator('.context-budget')).toHaveCount(0)
    await expect(page.locator('.page-facts')).not.toContainText('budget')
  })

  test('opens a static snapshot on Context, falls back to the first tab with content, and keeps deep links', async ({ page }) => {
    await page.route('**/snap/overview.json', (route) => route.fulfill({ json: overview }))
    await page.goto('/?source=snap/')
    await expect(page.getByRole('heading', { level: 1, name: 'Context', exact: true })).toBeVisible()
    await expect(page.getByRole('tab', { name: /^Rules/ })).toHaveAttribute('aria-selected', 'true')
    const nav = page.getByRole('navigation', { name: 'Configuration sections' })
    await expect(nav.getByRole('link', { name: /^Context/ })).toHaveAttribute('aria-current', 'page')
    await expect(nav.getByRole('link', { name: /^Context/ })).toHaveAttribute('href', '?source=snap%2F')
    await expect(nav.getByRole('link', { name: /^Conversations/ })).toHaveAttribute('href', '?view=conversations&source=snap%2F')
    await page.goto('/?source=snap/&tab=usage')
    await expect(page.getByRole('tab', { name: /^Context size/ })).toHaveAttribute('aria-selected', 'true')
    await expect(page.locator('section.context-usage')).toContainText('needs agnostic-ai view running')
    await serve(page)
    await page.goto('/?view=context&tab=skills')
    await expect(page.getByRole('tab', { name: /^Skills/ })).toHaveAttribute('aria-selected', 'true')
    await page.goto('/?view=context&q=reviewer')
    await expect(page.getByRole('tab', { name: /^Agents/ })).toHaveAttribute('aria-selected', 'true')
    await page.goto('/?view=context&entry=item%3Amcp%3Agithub%3A.agnostic-ai%2Fmcps%2Fgithub.yaml')
    await expect(page.locator('.entry-missing')).toBeVisible()
  })

  test('lands on Context size in mock mode and on a phone without sideways scroll', async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 })
    await page.goto('/?view=context&mock=1')
    await expect(page.getByRole('tab', { name: /^Context size/ })).toHaveAttribute('aria-selected', 'true')
    for (const name of ['Copy prompt to review what Claude Code reads at every start', 'Copy prompt to review what Claude Code reads on demand']) {
      expect((await page.getByRole('button', { name, exact: true }).boundingBox())!.height).toBeGreaterThanOrEqual(44)
    }
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390)
    await page.emulateMedia({ colorScheme: 'light', reducedMotion: 'reduce' })
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390)
  })

  test('opens the first tab with content for user configuration, without flipping tabs', async ({ page }) => {
    await serve(page, contextView, { ...structuredClone(overview), global: true })
    await page.route('**/api/health', async (route) => {
      await new Promise((resolve) => setTimeout(resolve, 300))
      await route.fulfill({ json: { ok: true, global: true } })
    })
    await page.goto('/?view=context')
    const rules = page.getByRole('tab', { name: /^Rules/ })
    await expect(rules).toHaveAttribute('aria-selected', 'true')
    await page.waitForTimeout(600)
    await expect(rules).toHaveAttribute('aria-selected', 'true')
    await expect(page.locator('.context-summary')).toBeHidden()
  })

  test('opens the first tab with content when the project has no coding tool', async ({ page }) => {
    await serve(page, contextView, { ...structuredClone(overview), targets: [] })
    await page.route('**/api/views/context**', (route) => route.fulfill({ status: 400, json: { error: 'explain --context:  is not a configured target' } }))
    await page.goto('/?view=context')
    await expect(page.getByRole('tab', { name: /^Rules/ })).toHaveAttribute('aria-selected', 'true')
    await expect(page.getByText('is not a configured target')).toBeHidden()
  })

  test('reaches the first table action by keyboard from the tab row', async ({ page }) => {
    await serve(page)
    await page.goto('/?view=context')
    await page.getByRole('tab', { name: /^Context size/ }).focus()
    await page.keyboard.press('ArrowRight')
    await expect(page.getByRole('tab', { name: /^Guidance/ })).toHaveAttribute('aria-selected', 'true')
    await page.keyboard.press('ArrowLeft')
    await expect(page.getByRole('tab', { name: /^Context size/ })).toBeFocused()
    let hops = 0
    while (hops < 12) {
      await page.keyboard.press('Tab')
      hops++
      if (await page.getByRole('button', { name: 'Copy prompt to review what Claude Code reads at every start' }).evaluate((el) => el === document.activeElement)) break
    }
    expect(hops).toBeLessThanOrEqual(6)
  })
})
