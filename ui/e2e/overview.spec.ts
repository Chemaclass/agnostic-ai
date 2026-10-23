import { expect, test } from '@playwright/test'
import { MOCK, openApp, saveScreenshot } from './helpers'

test.describe('overview', () => {
  test('opens on Context with five sections and the sync state of each tool', async ({ page }) => {
    await openApp(page)
    await expect(page.getByRole('heading', { level: 1, name: 'Context' })).toBeVisible()
    await expect(page.locator('.page-subtitle')).toHaveText('What is the agent told?')
    for (const kind of ['Instructions', 'Rules', 'Agents', 'Skills']) await expect(page.getByRole('heading', { level: 2, name: new RegExp(`^${kind}`) })).toBeVisible()
    const nav = page.getByRole('navigation', { name: 'Harness' })
    await expect(nav.getByRole('link')).toHaveCount(5)
    for (const name of ['Context', 'Authority', 'Tools', 'Lifecycle', 'Settings']) await expect(nav.getByRole('link', { name: new RegExp(`^${name}`) })).toBeVisible()
    if (MOCK) await expect(page.locator('.topbar .chip', { hasText: 'out of sync' })).toHaveCount(3)
    await expect(page.getByRole('combobox')).toHaveCount(0)
    const open = page.locator('.item-row', { hasText: 'agents/' }).first().getByRole('link', { name: /^Open in Cursor/ })
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
    await expect(page.locator('.stage').first().locator('.stage-tools .chip').first()).toBeVisible()
    await saveScreenshot(page, '02-lifecycle.png')
  })

  test('shows built-ins as switches and config values as chips and text', async ({ page }) => {
    await openApp(page, '?view=settings')
    await expect(page.getByRole('heading', { level: 1, name: 'Settings' })).toBeVisible()
    const builtins = page.locator('.item-row', { has: page.locator('.switch') })
    await expect(builtins).toHaveCount(5)
    await expect(builtins.filter({ hasText: 'memory' }).locator('.switch-text')).toHaveText(/On|Off/)
    const config = page.locator('.item-row', { hasText: 'agnostic-ai.yaml' })
    await expect(config.locator('.value-row', { hasText: 'targets' }).locator('.chip').first()).toBeVisible()
    await saveScreenshot(page, '03-settings.png')
  })

  test('shows a check finding on the item it names', async ({ page }) => {
    test.skip(!MOCK, 'The finding is specific to the demo project fixture.')
    await openApp(page, '?view=authority')
    await expect(page.getByRole('heading', { level: 1, name: 'Authority' })).toBeVisible()
    const permissions = page.locator('.item-row', { hasText: 'permissions.yaml' }).first()
    await expect(permissions).toHaveClass(/has-findings/)
    await expect(permissions.locator('.finding')).toContainText('LINT038')
    await expect(permissions.locator('.value-row', { hasText: 'permissions.allow' }).locator('.chip').first()).toBeVisible()
    await expect(page.getByText('Capability domains')).toHaveCount(0)
    await expect(page.getByText('Uncertainties')).toHaveCount(0)
    await saveScreenshot(page, '04-authority.png')
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
})
