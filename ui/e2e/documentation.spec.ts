import { expect, test } from '@playwright/test'
import { readFileSync } from 'node:fs'

const fixture = JSON.parse(readFileSync(new URL('../src/fixtures/overview.mock.json', import.meta.url), 'utf8'))

test('links each source section to its documentation, including empty sections', async ({ page }) => {
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
    ['context', 'guidance', 'Instructions', 'configuration/#entry-point-files'],
    ['context', 'guidance', 'Reviews', 'spec-format/reviews/'],
    ['context', 'guidance', 'Ignore patterns', 'spec-format/ignore/'],
    ['context', 'rules', 'Rules', 'spec-format/rules/'],
    ['context', 'agents', 'Agents', 'spec-format/agents/'],
    ['context', 'skills', 'Skills', 'spec-format/skills/'],
    ['context', 'command', 'Commands', 'spec-format/commands/'],
    ['context', 'usage', 'Context size', 'cli-reference/inspect/#rank-context-contributions'],
    ['authority', '', 'Permissions', 'spec-format/settings/#permission-rules'],
    ['tools', 'servers', 'MCP servers', 'spec-format/mcps/'],
    ['tools', 'native', 'Found in tools', 'cli-reference/start/#import'],
    ['tools', 'environment', 'Environments', 'spec-format/environments/'],
    ['lifecycle', '', 'Hooks', 'spec-format/hooks/#a-typical-session'],
    ['settings', 'builtins', 'Built-ins', 'configuration/#built-ins'],
    ['settings', 'preferences', 'Other settings', 'spec-format/settings/'],
    ['settings', 'configuration', 'Config', 'configuration/'],
  ]
  for (const [view, tab, heading, path] of cases) {
    await page.goto(`/?view=${view}${tab ? `&tab=${tab}` : ''}`)
    const title = page.getByRole('heading', { level: 2, name: new RegExp(`^${heading}(?:\\s|$)`) })
    const link = title.locator('..').getByRole('link', { name: /^Documentation for/ })
    await expect(link).toBeVisible()
    await expect(link).toHaveAttribute('href', `https://agnostic-ai.org/docs/${path}`)
    await expect(link).toHaveAttribute('target', '_blank')
    await expect(link).toHaveAttribute('rel', 'noopener noreferrer')
    await expect(page.getByRole('heading', { name: 'Coding tool support', exact: true }).locator('..').getByRole('link')).toHaveAttribute('href', 'https://agnostic-ai.org/docs/target-behavior/')
  }
})

test('keeps help reachable on a phone and opens it without losing the selected tab', async ({ page, context }) => {
  await page.setViewportSize({ width: 390, height: 844 })
  await page.route('**/api/views/overview', (route) => route.fulfill({ json: fixture }))
  await context.route('https://agnostic-ai.org/docs/spec-format/skills/', (route) => route.fulfill({ contentType: 'text/html', body: '<title>Skill documentation</title>' }))
  await page.goto('/?view=context&tab=skills')
  const help = page.getByRole('link', { name: 'Documentation for skills (opens in a new tab)' })
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
