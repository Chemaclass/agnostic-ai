import { expect, test, type Page } from '@playwright/test'
import { readFileSync } from 'node:fs'

const overview = JSON.parse(readFileSync(new URL('../src/fixtures/overview.mock.json', import.meta.url), 'utf8'))
const contextView = JSON.parse(readFileSync(new URL('../src/fixtures/context.mock.json', import.meta.url), 'utf8'))

async function serve(page: Page, view = contextView, data = overview) {
  await page.route('**/api/views/overview', (route) => route.fulfill({ json: data }))
  await page.route('**/api/health', (route) => route.fulfill({ json: { ok: true } }))
  await page.route('**/api/views/context**', (route) => route.fulfill({ json: view }))
}

test.describe('landing', () => {
  test('opens Context on Context size with a startup check and next steps', async ({ page, context }) => {
    await serve(page)
    await page.goto('/')
    await expect(page.getByRole('tab', { name: /^Context size/ })).toHaveAttribute('aria-selected', 'true')
    expect(new URL(page.url()).searchParams.has('tab')).toBe(false)
    await expect(page.getByRole('searchbox', { name: 'Search context' })).toBeVisible()
    const check = page.getByRole('region', { name: 'Startup check' })
    await expect(check.locator('.context-verdict')).toHaveText('Startup is 184 words, within the 2,000-word budget lint checks (lint.instructions-words).')
    await expect(check.getByRole('link', { name: 'lint.instructions-words' })).toHaveAttribute('href', 'https://agnostic-ai.org/docs/configuration/#lint')
    await expect(check.locator('.context-model')).toContainText('No default model or reasoning effort is set for Claude Code')
    await expect(check.getByRole('list', { name: 'Next' }).getByRole('listitem')).toHaveCount(3)
    await expect(check).toContainText('agnostic-ai explain --context --target claude')
    await expect(check).not.toContainText(/token|faster|cheaper|context window/i)
    await context.grantPermissions(['clipboard-read', 'clipboard-write'])
    await check.getByRole('button', { name: 'Copy a startup review prompt for claude' }).click()
    const prompt = await page.evaluate(() => navigator.clipboard.readText())
    expect(prompt).toContain('Review everything claude loads at the start of every session')
    expect(prompt).toContain('- .agnostic-ai/AGNOSTIC_AI.md (entry-point layer): 120 words, 790 bytes, 65% of startup')
    expect(prompt).not.toMatch(/[–—]/)
    await check.getByRole('button', { name: /^Show \.agnostic-ai\/AGNOSTIC_AI\.md/ }).click()
    const row = page.getByRole('row', { name: /\.agnostic-ai\/AGNOSTIC_AI\.md/ }).first()
    await expect(row.getByRole('button', { name: 'Hide advice' })).toBeFocused()
    await expect(page.locator('.context-advice')).toContainText('What it is')
  })

  test('says when startup is over the budget and links the model line to Settings', async ({ page }) => {
    const over = structuredClone(contextView)
    over.startupWordBudget = 150
    const data = structuredClone(overview)
    const model = data.sections.find((s: { id: string }) => s.id === 'settings').fields.find((f: { key: string }) => f.key === 'model')
    model.resolved[0] = { ...model.resolved[0], value: 'sonnet', alias: 'claude-sonnet-4-5' }
    await serve(page, over, data)
    await page.goto('/')
    const check = page.getByRole('region', { name: 'Startup check' })
    await expect(check.locator('.context-verdict')).toHaveText('Startup is 184 words, 34 words over the 150-word budget lint checks (lint.instructions-words); agnostic-ai lint reports LINT011.')
    await expect(check.locator('.context-verdict')).toHaveClass(/is-over/)
    await expect(check.locator('.context-model')).toContainText('Model for Claude Code: sync writes sonnet (resolved from claude-sonnet-4-5).')
    await check.getByRole('link', { name: 'Change in Settings' }).click()
    await expect(page).toHaveURL(/view=settings&tab=preferences&entry=field%3Amodel/)
    await expect(page.getByRole('tab', { name: /^Preferences/ })).toHaveAttribute('aria-selected', 'true')
    await expect(page.locator('.field-row.is-linked')).toContainText('Default model')
  })

  test('shows only the size when the server sends no budget', async ({ page }) => {
    const noBudget = structuredClone(contextView)
    delete noBudget.startupWordBudget
    await serve(page, noBudget)
    await page.goto('/')
    await expect(page.getByRole('region', { name: 'Startup check' }).locator('.context-verdict')).toHaveText('Startup is 184 words.')
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
    const check = page.getByRole('region', { name: 'Startup check' })
    await expect(check).toBeVisible()
    for (const name of ['Copy the explain command for claude', 'Copy a startup review prompt for claude']) {
      expect((await check.getByRole('button', { name, exact: true }).boundingBox())!.height).toBeGreaterThanOrEqual(44)
    }
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390)
    await page.emulateMedia({ colorScheme: 'light', reducedMotion: 'reduce' })
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390)
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
