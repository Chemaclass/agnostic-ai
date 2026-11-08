import { expect, test } from '@playwright/test'
import { readFileSync } from 'node:fs'
import { openToolSupport } from '../helpers'

const fixture = JSON.parse(readFileSync(new URL('../../src/fixtures/overview.mock.json', import.meta.url), 'utf8'))

function withReview() {
  const data = structuredClone(fixture)
  const context = data.sections.find((section: { id: string }) => section.id === 'context')
  context.kinds.push({ kind: 'review', label: 'Review instructions', count: 1 }, { kind: 'ignore', label: 'Ignore files', count: 1 })
  context.items.push(
    { kind: 'review', name: 'review', label: 'review', description: 'What reviewers check.', path: '.agnostic-ai/review.md', layer: 'project', values: [], findings: [] },
    { kind: 'ignore', name: 'ignore', label: 'ignore', description: 'Files agents skip.', path: '.agnostic-ai/ignore', layer: 'project', values: [], findings: [] },
  )
  context.support.push(
    { kind: 'review', label: 'Review instructions', supported: ['claude'], unsupported: [] },
    { kind: 'ignore', label: 'Ignore files', supported: ['claude'], unsupported: [] },
  )
  return data
}

test('opens the tab that holds a kind from its support row and moves to its heading', async ({ page }) => {
  await page.setViewportSize({ width: 1280, height: 700 })
  await page.route('**/api/views/overview', (route) => route.fulfill({ json: withReview() }))
  await page.goto('/?view=context&tab=agents')
  await openToolSupport(page)
  const guidance = page.getByRole('tab', { name: /^Guidance/ })
  await expect(guidance).toHaveAttribute('title', 'Instructions, review instructions, ignore files')
  await page.getByRole('table', { name: 'Coding tool support' }).getByRole('link', { name: 'Review instructions', exact: true }).click()
  await expect(guidance).toHaveAttribute('aria-selected', 'true')
  const heading = page.getByRole('heading', { level: 2, name: /^Review instructions/ })
  await expect(heading).toBeFocused()
  await expect(heading).toBeInViewport()
  await expect(page).toHaveURL(/tab=guidance/)
  await expect(page).toHaveURL(/entry=kind%3Areview/)
  await expect(page.locator('.entry-missing')).toHaveCount(0)
  await expect(page.getByRole('dialog', { name: 'Coding tool support' })).toBeHidden()
  await page.evaluate(() => window.scrollTo({ top: 0 }))
  await (await openToolSupport(page)).getByRole('link', { name: 'Review instructions', exact: true }).click()
  await expect(heading).toBeFocused()
  await expect(heading).toBeInViewport()
  await (await openToolSupport(page)).getByRole('link', { name: 'Agents', exact: true }).click()
  await expect(page.getByRole('tab', { name: /^Agents/ })).toHaveAttribute('aria-selected', 'true')
  await expect(page.getByRole('heading', { level: 2, name: /^Agents/ })).toBeFocused()
})

for (const [view, kind, heading] of [['tools', 'MCP servers', /^MCP servers/], ['lifecycle', 'Hooks', /^Hooks/], ['authority', 'Permissions', /^Permissions/], ['settings', 'Preferences', /^Preferences/]] as const) {
  test(`links the ${kind} support row to its heading on ${view}`, async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 700 })
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: fixture }))
    await page.goto(`/?view=${view}`)
    await openToolSupport(page)
    await page.getByRole('table', { name: 'Coding tool support' }).getByRole('link', { name: kind, exact: true }).click()
    const target = page.getByRole('heading', { level: 2, name: heading })
    await expect(target).toBeFocused()
    // Authority names its rules table for screen readers only, so the table itself must scroll into view.
    await expect(view === 'authority' ? page.locator('.authority-rule').first() : target).toBeInViewport()
    await expect(page.locator('.entry-missing')).toHaveCount(0)
  })
}
