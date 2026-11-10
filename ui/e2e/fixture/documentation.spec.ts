import { expect, test } from '@playwright/test'
import { readFileSync } from 'node:fs'

const fixture = JSON.parse(readFileSync(new URL('../../src/fixtures/overview.mock.json', import.meta.url), 'utf8'))

test('shows one Docs link per page at the end of the facts line, for the open tab, including empty sections', async ({ page }) => {
  const data = structuredClone(fixture)
  data.sections.find((section: { id: string }) => section.id === 'context').kinds.push(
    { kind: 'review', label: 'Reviews', count: 0 },
    { kind: 'ignore', label: 'Ignore patterns', count: 0 },
    { kind: 'command', label: 'Commands', count: 0 },
  )
  const tools = data.sections.find((section: { id: string }) => section.id === 'tools')
  tools.kinds.push({ kind: 'environment', label: 'Environments', count: 0 })
  tools.items = []
  tools.native = []
  await page.route('**/api/views/overview', (route) => route.fulfill({ json: data }))
  const cases = [
    ['context', 'guidance', 'configuration/#entry-point-files'],
    ['context', 'rules', 'spec-format/rules/'],
    ['context', 'agents', 'spec-format/agents/'],
    ['context', 'skills', 'spec-format/skills/'],
    ['context', 'command', 'spec-format/commands/'],
    ['context', 'usage', 'cli-reference/inspect/#rank-context-contributions'],
    ['authority', '', 'spec-format/settings/#permission-rules'],
    ['tools', 'servers', 'spec-format/mcps/'],
    ['tools', 'native', 'cli-reference/import/#import'],
    ['tools', 'environment', 'spec-format/environments/'],
    ['lifecycle', '', 'spec-format/hooks/#a-typical-session'],
    ['settings', 'builtins', 'configuration/#built-ins'],
    ['settings', 'targets', 'configuration/#targets'],
    ['settings', 'preferences', 'spec-format/settings/'],
    ['settings', 'configuration', 'configuration/'],
  ]
  for (const [view, tab, path] of cases) {
    await page.goto(`/?view=${view}${tab ? `&tab=${tab}` : ''}`)
    const links = page.getByRole('link', { name: /^Documentation for/ })
    await expect(links).toHaveCount(1)
    const link = page.locator('.page-facts').getByRole('link', { name: /^Documentation for/ })
    await expect(link).toHaveAttribute('href', `https://agnostic-ai.org/docs/${path}`)
    await expect(link).toHaveAttribute('target', '_blank')
    await expect(link).toHaveAttribute('rel', 'noopener noreferrer')
  }
})

test('keeps help reachable on a phone and opens it without losing the selected tab', async ({ page, context }) => {
  await page.setViewportSize({ width: 390, height: 844 })
  await page.route('**/api/views/overview', (route) => route.fulfill({ json: fixture }))
  await context.route('https://agnostic-ai.org/docs/spec-format/skills/', (route) => route.fulfill({ contentType: 'text/html', body: '<title>Skill documentation</title>' }))
  await page.goto('/?view=context&tab=skills')
  const help = page.locator('.page-facts').getByRole('link', { name: 'Documentation for skills (opens in a new tab)' })
  await expect(page.locator('.empty-state').filter({ hasText: 'No skills yet.' })).toBeVisible()
  await expect(page.locator('.empty-state').getByRole('link', { name: /^Documentation for/ })).toHaveCount(0)
  await expect(help).toBeVisible()
  const bounds = await help.boundingBox()
  expect(bounds!.x).toBeGreaterThanOrEqual(0)
  expect(bounds!.x + bounds!.width).toBeLessThanOrEqual(390)
  expect(bounds!.height).toBeGreaterThanOrEqual(44)
  const popupPromise = page.waitForEvent('popup')
  await help.click()
  const popup = await popupPromise
  await expect(popup).toHaveTitle('Skill documentation')
  await expect(page.getByRole('tab', { name: /^Skills/ })).toHaveAttribute('aria-selected', 'true')
  await expect(page).toHaveURL(/tab=skills/)
})
