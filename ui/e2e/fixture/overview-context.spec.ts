import { expect, test } from '@playwright/test'
import { readFileSync } from 'node:fs'

const fixture = JSON.parse(readFileSync(new URL('../../src/fixtures/overview.mock.json', import.meta.url), 'utf8'))

test.describe('overview', () => {
  test('copies a refine prompt for the global instructions file only', async ({ page, context }) => {
    const data = structuredClone(fixture)
    const contextSection = data.sections.find((section: { id: string }) => section.id === 'context')
    const instructions = { kind: 'instructions', name: 'AGNOSTIC_AI.md', label: 'AGNOSTIC_AI.md', description: 'Loaded at the start of every session: 120 words.', values: [], findings: [] }
    contextSection.items.push(
      { ...instructions, path: 'AGNOSTIC_AI.md', openPath: '/home/me/.agnostic-ai/AGNOSTIC_AI.md', layer: 'global' },
      { ...instructions, path: 'local/AGNOSTIC_AI.md', openPath: '/home/me/.agnostic-ai/local/AGNOSTIC_AI.md', layer: 'global-local' },
    )
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: data }))
    await page.goto('/?view=context&tab=guidance')
    const refine = page.getByRole('button', { name: /^Copy refine prompt/ })
    await expect(refine).toHaveCount(1)
    await expect(page.locator('.item-row', { hasText: 'local/AGNOSTIC_AI.md' }).getByRole('button', { name: /^Copy refine prompt/ })).toHaveCount(0)
    await expect(page.locator('.item-row', { hasText: 'local/AGNOSTIC_AI.md' }).locator('.item-layer')).toHaveText('local')
    await context.grantPermissions(['clipboard-read', 'clipboard-write'])
    await refine.click()
    await expect(page.getByRole('button', { name: 'Copied', exact: true })).toBeVisible()
    const prompt = await page.evaluate(() => navigator.clipboard.readText())
    expect(prompt).toContain('global agent instructions in /home/me/.agnostic-ai/AGNOSTIC_AI.md')
    expect(prompt).toContain('Then wait for my explicit approval.')
  })
})
