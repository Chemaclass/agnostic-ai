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
    await page.goto('/')
    await expect(page.getByRole('tab', { name: /^Context size/ })).toHaveAttribute('aria-selected', 'true')
    expect(new URL(page.url()).searchParams.has('tab')).toBe(false)
    await expect(page.getByRole('searchbox', { name: 'Search context' })).toBeVisible()
    await page.reload()
    await expect(page.getByRole('tab', { name: /^Context size/ })).toHaveAttribute('aria-selected', 'true')
    expect(new URL(page.url()).searchParams.has('tab')).toBe(false)
    const panel = page.locator('section.context-usage')
    const verdict = panel.locator('.context-verdict')
    await expect(verdict).toHaveText('Within the 2,000-word startup budget. Change budget')
    await expect(verdict.getByRole('link', { name: 'Change budget' })).toHaveAttribute('href', 'https://agnostic-ai.org/docs/configuration/#lint')
    const next = page.getByRole('region', { name: 'Next' })
    await expect(next.getByRole('listitem')).toHaveCount(3)
    await expect(next).not.toContainText('agnostic-ai explain')
    for (const part of [panel.locator('.context-summary'), next]) await expect(part).not.toContainText(/token|faster|cheaper|context window|lint/i)
    const method = panel.locator('.context-usage-method')
    await expect(method.locator('.context-model')).toBeHidden()
    await method.getByText('How this is counted').click()
    await expect(method.locator('.context-model')).toContainText('No default model or reasoning effort is set for Claude Code')
    await expect(method).toContainText('Startup text loads at the start of every session.')
    await context.grantPermissions(['clipboard-read', 'clipboard-write'])
    await next.getByRole('button', { name: 'Copy command: agnostic-ai explain --context --target claude' }).click()
    expect(await page.evaluate(() => navigator.clipboard.readText())).toBe('agnostic-ai explain --context --target claude')
    await next.getByRole('button', { name: 'Copy prompt to review startup for Claude Code' }).click()
    const prompt = await page.evaluate(() => navigator.clipboard.readText())
    expect(prompt).toContain('Review everything claude loads at the start of every session')
    expect(prompt).toContain('Startup is 184 words, within the 2,000-word lint budget (lint.instructions-words).')
    expect(prompt).toContain('- .agnostic-ai/AGNOSTIC_AI.md (entry-point layer): 120 words, 790 bytes, 65% of startup')
    expect(prompt).not.toMatch(/[–—]/)
    await next.getByRole('button', { name: /^Show \.agnostic-ai\/AGNOSTIC_AI\.md/ }).click()
    const row = page.getByRole('row', { name: /\.agnostic-ai\/AGNOSTIC_AI\.md/ }).first()
    await expect(row.getByRole('button', { name: 'Hide advice' })).toBeFocused()
    await expect(page.locator('.context-advice')).toContainText('What it is')
    await next.getByRole('button', { name: /^Show \.agnostic-ai\/AGNOSTIC_AI\.md/ }).click()
    await expect(row.getByRole('button', { name: 'Hide advice' })).toBeFocused()
  })

  test('says when startup is over the budget and links the model line to Settings', async ({ page }) => {
    const over = structuredClone(contextView)
    over.startupWordBudget = 150
    const data = structuredClone(overview)
    const model = data.sections.find((s: { id: string }) => s.id === 'settings').fields.find((f: { key: string }) => f.key === 'model')
    model.resolved[0] = { ...model.resolved[0], value: 'sonnet', alias: 'claude-sonnet-4-5' }
    await serve(page, over, data)
    await page.goto('/')
    const panel = page.locator('section.context-usage')
    await expect(panel.locator('.context-verdict')).toHaveText('34 words over the 150-word startup budget. Change budget')
    await expect(panel.locator('.context-verdict')).toHaveClass(/is-over/)
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
    await page.goto('/')
    const panel = page.locator('section.context-usage')
    await expect(panel.locator('.context-total-words').first()).toHaveText('184 words')
    await expect(panel.locator('.context-verdict')).toHaveCount(0)
  })

  test('falls back to the first tab with content for a static snapshot and keeps deep links', async ({ page }) => {
    await page.route('**/snap/overview.json', (route) => route.fulfill({ json: overview }))
    await page.goto('/?source=snap/')
    await expect(page.getByRole('tab', { name: /^Rules/ })).toHaveAttribute('aria-selected', 'true')
    await page.goto('/?source=snap/&tab=usage')
    await expect(page.getByRole('tab', { name: /^Context size/ })).toHaveAttribute('aria-selected', 'true')
    await expect(page.locator('section.context-usage')).toContainText('needs a running agnostic-ai serve')
    await serve(page)
    await page.goto('/?tab=skills')
    await expect(page.getByRole('tab', { name: /^Skills/ })).toHaveAttribute('aria-selected', 'true')
    await page.goto('/?q=reviewer')
    await expect(page.getByRole('tab', { name: /^Agents/ })).toHaveAttribute('aria-selected', 'true')
    await page.goto('/?view=context&entry=item%3Amcp%3Agithub%3A.agnostic-ai%2Fmcps%2Fgithub.yaml')
    await expect(page.locator('.entry-missing')).toBeVisible()
  })

  test('lands on Context size in mock mode and on a phone without sideways scroll', async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 })
    await page.goto('/?mock=1')
    await expect(page.getByRole('tab', { name: /^Context size/ })).toHaveAttribute('aria-selected', 'true')
    const next = page.getByRole('region', { name: 'Next' })
    await expect(next).toBeVisible()
    for (const name of ['Copy command: agnostic-ai explain --context --target claude', 'Copy prompt to review startup for Claude Code']) {
      expect((await next.getByRole('button', { name, exact: true }).boundingBox())!.height).toBeGreaterThanOrEqual(44)
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
    await page.goto('/')
    const rules = page.getByRole('tab', { name: /^Rules/ })
    await expect(rules).toHaveAttribute('aria-selected', 'true')
    await page.waitForTimeout(600)
    await expect(rules).toHaveAttribute('aria-selected', 'true')
    await expect(page.locator('.context-summary')).toBeHidden()
  })

  test('opens the first tab with content when the project has no coding tool', async ({ page }) => {
    await serve(page, contextView, { ...structuredClone(overview), targets: [] })
    await page.route('**/api/views/context**', (route) => route.fulfill({ status: 400, json: { error: 'explain --context:  is not a configured target' } }))
    await page.goto('/')
    await expect(page.getByRole('tab', { name: /^Rules/ })).toHaveAttribute('aria-selected', 'true')
    await expect(page.getByText('is not a configured target')).toBeHidden()
  })

  test('reaches the first action by keyboard from the tab row', async ({ page }) => {
    await serve(page)
    await page.goto('/')
    await page.getByRole('tab', { name: /^Context size/ }).focus()
    await page.keyboard.press('ArrowRight')
    await expect(page.getByRole('tab', { name: /^Guidance/ })).toHaveAttribute('aria-selected', 'true')
    await page.keyboard.press('ArrowLeft')
    await expect(page.getByRole('tab', { name: /^Context size/ })).toBeFocused()
    let hops = 0
    while (hops < 12) {
      await page.keyboard.press('Tab')
      hops++
      if (await page.getByRole('button', { name: /^Show \.agnostic-ai\/AGNOSTIC_AI\.md/ }).evaluate((el) => el === document.activeElement)) break
    }
    expect(hops).toBeLessThanOrEqual(9)
  })
})
