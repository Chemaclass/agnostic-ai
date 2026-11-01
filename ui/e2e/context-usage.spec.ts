import { expect, test } from '@playwright/test'
import { readFileSync } from 'node:fs'
import { selectTab } from './helpers'

const overview = JSON.parse(readFileSync(new URL('../src/fixtures/overview.mock.json', import.meta.url), 'utf8'))
const contextView = JSON.parse(readFileSync(new URL('../src/fixtures/context.mock.json', import.meta.url), 'utf8'))

test.describe('context size', () => {
  test('shows the mock duplicate with its sources, review prompt, and refresh', async ({ page, context }) => {
    await page.goto('/?mock=1')
    await selectTab(page, 'Context size')
    const panel = page.locator('section.context-usage')
    await expect(panel.getByRole('heading', { name: 'Context size' })).toBeVisible()
    await expect(panel.getByLabel('Coding tool')).toHaveValue('claude')
    await expect(panel.locator('.context-usage-totals')).toContainText('184 words, 1,212 bytes')
    await expect(panel.locator('.context-contributions > summary')).toBeVisible()
    await expect(panel.getByRole('table', { name: /^Startup/ })).toBeHidden()
    const duplicate = panel.locator('.context-opportunity')
    await expect(duplicate).toHaveCount(1)
    await expect(duplicate).toContainText('Run the full test suite before you push.')
    await expect(duplicate).toContainText('Could drop 18 words, 94 bytes from startup content for claude.')
    await expect(duplicate.getByRole('list', { name: 'Sources' })).toContainText('.agnostic-ai/rules/testing.md')
    await context.grantPermissions(['clipboard-read', 'clipboard-write'])
    await duplicate.getByRole('button', { name: /^Copy review prompt/ }).click()
    await expect(duplicate.getByRole('button', { name: 'Copied', exact: true })).toBeVisible()
    const prompt = await page.evaluate(() => navigator.clipboard.readText())
    expect(prompt).toContain('Target: claude')
    expect(prompt).toContain('Do not apply any change until I agree.')
    await panel.getByRole('button', { name: 'Reread specs' }).click()
    await expect(panel.locator('.context-usage-compare')).toContainText('Before reread')
    await expect(panel.locator('.context-usage-compare')).toContainText('Now')
    await expect(panel).not.toContainText(/tokens? saved|faster/i)
  })

  test('keeps the controls readable on a phone and explains the estimate on request', async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 })
    await page.goto('/?mock=1&tab=usage')
    const panel = page.locator('section.context-usage')
    await expect(panel.getByLabel('Coding tool')).toBeVisible()
    await expect(panel.getByLabel('Check against a file')).toHaveAccessibleDescription('Name a file to add the scoped rules that match it. Results update as you type.')
    await expect(panel.locator('.context-total-words').first()).toHaveText('184 words')
    const method = panel.locator('.context-usage-method')
    await expect(method.locator('p')).toBeHidden()
    await method.getByText('How this is counted').click()
    await expect(method.locator('p')).toContainText('not model tokens or live context')
    for (const control of await panel.locator('form input, form select, form button').all()) {
      const box = await control.boundingBox()
      expect(box).not.toBeNull()
      expect(box!.x).toBeGreaterThanOrEqual(0)
      expect(box!.x + box!.width).toBeLessThanOrEqual(390)
    }
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390)
    await panel.getByText(/sources, largest first/).click()
    await panel.getByRole('button', { name: 'What can I do?' }).first().click()
    await expect(panel.locator('.context-advice')).toBeVisible()
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390)
  })

  test('refreshes the same target and shows the updated findings', async ({ page }) => {
    const after = structuredClone(contextView)
    after.report.startup = { words: 166, bytes: 1118 }
    after.report.opportunities = []
    const requests: string[] = []
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: overview }))
    await page.route('**/api/views/context**', (route) => {
      requests.push(new URL(route.request().url()).search)
      route.fulfill({ json: requests.length === 1 ? contextView : after })
    })
    await page.goto('/?view=context')
    await selectTab(page, 'Context size')
    const panel = page.locator('section.context-usage')
    await expect(panel.locator('.context-opportunity')).toHaveCount(1)
    await panel.getByRole('button', { name: 'Reread specs' }).click()
    await expect(panel).toContainText('No duplicated passages found for claude.')
    await expect(panel.locator('.context-usage-compare')).toContainText('Before reread184 words, 1,212 bytes at startup, 1 duplicated passage.')
    await expect(panel.locator('.context-usage-compare')).toContainText('Now166 words, 1,118 bytes at startup, 0 duplicated passages.')
    await expect(panel.locator('.context-usage-compare')).toContainText('ChangeStartup -18 words, File-scoped no change, On demand no change.')
    expect(requests).toEqual(['', '?target=claude'])
  })

  test('counts scoped rules for a named file and keeps the file on refresh', async ({ page }) => {
    const scoped = structuredClone(contextView)
    scoped.report.file = 'src/main.go'
    scoped.report.file_scope = { words: 46, bytes: 301 }
    scoped.report.on_demand = { words: 270, bytes: 1783 }
    scoped.report.contributions = scoped.report.contributions.map((c: { category: string; load: string }) => c.category === 'conditional rule body' ? { ...c, category: 'scoped rule', load: 'file-scope' } : c)
    const requests: string[] = []
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: overview }))
    await page.route('**/api/views/context**', (route) => {
      const search = new URL(route.request().url()).search
      requests.push(search)
      route.fulfill({ json: search.includes('file=') ? scoped : contextView })
    })
    await page.goto('/?view=context')
    await selectTab(page, 'Context size')
    const panel = page.locator('section.context-usage')
    await expect(panel).toContainText('Scoped rules count as on demand until you name a file.')
    await expect(panel.locator('.context-usage-totals')).not.toContainText('File-scoped')
    await panel.getByLabel('Check against a file').fill('src/main.go')
    await panel.getByRole('button', { name: 'Reread specs' }).click()
    await expect(panel.locator('.context-usage-totals')).toContainText('File-scoped, additional46 words, 301 bytes')
    await expect(panel).not.toContainText('Scoped rules count as on demand')
    await panel.getByText(/sources, largest first/).click()
    await expect(panel.getByRole('table', { name: /^File-scoped/ })).toContainText('scoped rule')
    await panel.getByRole('button', { name: 'Reread specs' }).click()
    await expect.poll(() => requests.length).toBe(3)
    expect(requests.slice(1)).toEqual(['?target=claude&file=src%2Fmain.go', '?target=claude&file=src%2Fmain.go'])
  })

  test('hides the previous tool report when another tool fails', async ({ page }) => {
    const two = { ...structuredClone(contextView), targets: ['claude', 'gemini'] }
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: overview }))
    await page.route('**/api/views/context**', (route) => {
      if (new URL(route.request().url()).searchParams.get('target') === 'gemini') return route.fulfill({ status: 400, json: { error: 'explain --context: gemini is not a configured target' } })
      return route.fulfill({ json: two })
    })
    await page.goto('/?view=context')
    await selectTab(page, 'Context size')
    const panel = page.locator('section.context-usage')
    await expect(panel.locator('.context-opportunity')).toHaveCount(1)
    await panel.getByLabel('Coding tool').selectOption('gemini')
    await expect(panel.getByRole('alert')).toContainText('gemini is not a configured target')
    await expect(panel.locator('.context-opportunity')).toHaveCount(0)
    await expect(panel.locator('.context-usage-totals')).toHaveCount(0)
    await panel.getByLabel('Coding tool').selectOption('claude')
    await expect(panel.locator('.context-opportunity')).toHaveCount(1)
  })

  test('drops the file for a tool without file-scoped rules', async ({ page }) => {
    const two = { ...structuredClone(contextView), targets: ['claude', 'copilot'], fileTargets: ['claude'] }
    const requests: string[] = []
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: overview }))
    await page.route('**/api/views/context**', (route) => {
      const url = new URL(route.request().url())
      requests.push(url.search)
      return route.fulfill({ json: { ...two, target: url.searchParams.get('target') ?? 'claude' } })
    })
    await page.goto('/?view=context')
    await selectTab(page, 'Context size')
    const panel = page.locator('section.context-usage')
    await expect(panel.locator('.context-opportunity')).toHaveCount(1)
    await panel.getByLabel('Check against a file').fill('src/main.go')
    await panel.getByRole('button', { name: 'Reread specs' }).click()
    await expect.poll(() => requests.at(-1)).toBe('?target=claude&file=src%2Fmain.go')
    await panel.getByLabel('Coding tool').selectOption('copilot')
    await expect.poll(() => requests.at(-1)).toBe('?target=copilot')
    await expect(panel.getByLabel('Check against a file')).toBeDisabled()
    await expect(panel.locator('.context-usage-file-hint')).toHaveText('This page cannot check a single file for copilot.')
  })

  test('shows the reason for user configuration', async ({ page }) => {
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: overview }))
    await page.route('**/api/views/context**', (route) => route.fulfill({ json: { schemaVersion: '1', supported: false, reason: 'Context size is reported for a project. Open a project to see it.', targets: [], target: '' } }))
    await page.goto('/?view=context')
    await selectTab(page, 'Context size')
    const panel = page.locator('section.context-usage')
    await expect(panel).toContainText('Open a project to see it.')
    await expect(panel.getByLabel('Coding tool')).toHaveCount(0)
  })

  test('applies a typed file after a pause and keeps tool and file in the URL', async ({ page }) => {
    const requests: string[] = []
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: overview }))
    await page.route('**/api/views/context**', (route) => {
      requests.push(new URL(route.request().url()).search)
      route.fulfill({ json: contextView })
    })
    await page.goto('/?view=context&tab=usage')
    const panel = page.locator('section.context-usage')
    await expect(panel.locator('.context-usage-totals')).toBeVisible()
    const before = requests.length
    await page.clock.install()
    await page.clock.pauseAt(Date.now() + 60_000)
    const input = panel.getByLabel('Check against a file')
    await input.pressSequentially('src/main.go', { delay: 40 })
    await page.clock.runFor(499)
    expect(requests.slice(before)).toEqual([])
    await page.clock.runFor(1)
    await expect.poll(() => requests.slice(before)).toEqual(['?target=claude&file=src%2Fmain.go'])
    await expect(page).toHaveURL(/tab=usage&target=claude&file=src%2Fmain\.go/)
    await page.reload()
    await expect(panel.getByLabel('Check against a file')).toHaveValue('src/main.go')
    await expect(panel.getByLabel('Coding tool')).toHaveValue('claude')
  })

  test('loads the default tool when the address names one that is gone', async ({ page }) => {
    const requests: string[] = []
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: overview }))
    await page.route('**/api/views/context**', (route) => {
      const url = new URL(route.request().url())
      requests.push(url.search)
      if (url.searchParams.get('target') === 'gemini') return route.fulfill({ status: 400, json: { error: 'explain --context: gemini is not a configured target' } })
      return route.fulfill({ json: contextView })
    })
    await page.goto('/?view=context&tab=usage&target=gemini&file=src%2Fmain.go')
    const panel = page.locator('section.context-usage')
    await expect(panel.locator('.context-opportunity')).toHaveCount(1)
    await expect(panel.getByRole('alert')).toHaveCount(0)
    await expect(panel.getByLabel('Coding tool')).toHaveValue('claude')
    await expect(page).not.toHaveURL(/target=|file=/)
    expect(requests).toEqual(['?target=gemini&file=src%2Fmain.go', ''])
  })

  test('keeps the address when the first load fails for another reason', async ({ page }) => {
    const requests: string[] = []
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: overview }))
    await page.route('**/api/views/context**', (route) => {
      requests.push(new URL(route.request().url()).search)
      return route.fulfill({ status: 500, json: { error: 'a spec is half edited' } })
    })
    await page.goto('/?view=context&tab=usage&target=claude&file=src%2Fmain.go')
    const panel = page.locator('section.context-usage')
    await expect(panel.getByRole('alert')).toContainText('a spec is half edited')
    await expect(page).toHaveURL(/target=claude&file=src%2Fmain\.go/)
    expect(requests).toEqual(['?target=claude&file=src%2Fmain.go'])
  })

  test('keeps a trailing space in the file box after the request lands', async ({ page }) => {
    const requests: string[] = []
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: overview }))
    await page.route('**/api/views/context**', (route) => {
      requests.push(new URL(route.request().url()).search)
      route.fulfill({ json: contextView })
    })
    await page.goto('/?view=context&tab=usage')
    const panel = page.locator('section.context-usage')
    await expect(panel.locator('.context-usage-totals')).toBeVisible()
    const input = panel.getByLabel('Check against a file')
    await input.pressSequentially('src/main.go ')
    await expect.poll(() => requests.at(-1)).toBe('?target=claude&file=src%2Fmain.go')
    await expect(page).toHaveURL(/file=src%2Fmain\.go(&|$)/)
    await expect(panel.getByText(/^Checked .* for src\/main\.go\.$/)).toBeVisible()
    await expect(input).toHaveValue('src/main.go ')
  })

  test('clears a search when the Context size tab opens', async ({ page }) => {
    await page.goto('/?mock=1&q=testing')
    await expect(page).toHaveURL(/q=testing/)
    await selectTab(page, 'Context size')
    await expect(page.locator('section.context-usage')).toBeVisible()
    await expect(page).not.toHaveURL(/q=/)
    await page.goto('/?mock=1&tab=usage&q=testing')
    await expect(page.locator('section.context-usage')).toBeVisible()
    await expect(page).not.toHaveURL(/q=/)
  })

  test('opens advice for a source and copies a prompt that names it', async ({ page, context }) => {
    const noDuplicates = structuredClone(contextView)
    noDuplicates.report.opportunities = []
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: overview }))
    await page.route('**/api/views/context**', (route) => route.fulfill({ json: noDuplicates }))
    await page.goto('/?view=context&tab=usage')
    const panel = page.locator('section.context-usage')
    await expect(panel.getByRole('table', { name: /^Startup/ })).toBeVisible()
    const row = panel.getByRole('row', { name: /\.agnostic-ai\/rules\/testing\.md/ })
    const toggle = row.getByRole('button', { name: /^(What can I do\?|Hide advice)$/ })
    await expect(toggle).toHaveAttribute('aria-expanded', 'false')
    await toggle.click()
    await expect(toggle).toHaveAttribute('aria-expanded', 'true')
    await expect(toggle).toHaveText('Hide advice')
    const advice = panel.locator('.context-advice')
    await expect(advice).toContainText('What it is')
    await expect(advice).toContainText('loads at the start of every session')
    await expect(advice).toContainText('Narrow the globs in .agnostic-ai/rules/testing.md')
    await context.grantPermissions(['clipboard-read', 'clipboard-write'])
    await advice.getByRole('button', { name: 'Copy advice prompt for .agnostic-ai/rules/testing.md' }).click()
    const prompt = await page.evaluate(() => navigator.clipboard.readText())
    expect(prompt).toContain('Source: .agnostic-ai/rules/testing.md')
    expect(prompt).toContain('Kind: always-on rule')
    expect(prompt).toContain('20% of startup content for claude')
    expect(prompt).toContain('agnostic-ai explain --context --target claude')
    expect(prompt).not.toMatch(/[–—]/)
    await toggle.click()
    await expect(advice).toHaveCount(0)
  })

  test('Start here opens the largest startup source and a built-in links to Settings', async ({ page }) => {
    const data = structuredClone(contextView)
    data.report.opportunities = []
    data.report.contributions = [
      { source: 'builtin:shared-memory-policy', category: 'always-on rule', load: 'startup', words: 270, bytes: 1800 },
      ...data.report.contributions,
    ]
    data.report.startup = { words: 454, bytes: 3012 }
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: overview }))
    await page.route('**/api/views/context**', (route) => route.fulfill({ json: data }))
    await page.goto('/?view=context&tab=usage')
    const panel = page.locator('section.context-usage')
    const start = panel.getByRole('list', { name: 'Start here' })
    await expect(start.getByRole('listitem')).toHaveCount(3)
    await expect(start.getByRole('listitem').first()).toContainText('builtin:shared-memory-policy')
    await expect(start.getByRole('listitem').first()).toContainText('59% of startup')
    await start.getByRole('button').first().click()
    const row = panel.getByRole('row', { name: /builtin:shared-memory-policy/ })
    await expect(row.getByRole('button', { name: 'Hide advice' })).toBeFocused()
    const advice = panel.locator('.context-advice')
    await expect(advice).toContainText('part of the memory built-in')
    await expect(advice).toContainText('removes shared memory for every tool')
    await advice.getByRole('button', { name: 'Open Built-ins' }).click()
    await expect(page).toHaveURL(/view=settings&tab=builtins/)
    await expect(page.getByRole('tab', { name: /^Built-ins/ })).toHaveAttribute('aria-selected', 'true')
  })

  test('shows the top ten sources per load with Show all', async ({ page }) => {
    const data = structuredClone(contextView)
    data.report.opportunities = []
    data.report.contributions = Array.from({ length: 12 }, (_, i) => ({ source: `.agnostic-ai/rules/rule-${String(i).padStart(2, '0')}.md`, category: 'always-on rule', load: 'startup', words: 100 - i, bytes: 600 - i }))
    data.report.startup = { words: data.report.contributions.reduce((n: number, c: { words: number }) => n + c.words, 0), bytes: 7000 }
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: overview }))
    await page.route('**/api/views/context**', (route) => route.fulfill({ json: data }))
    await page.goto('/?view=context&tab=usage')
    const panel = page.locator('section.context-usage')
    const table = panel.getByRole('table', { name: /^Startup/ })
    await expect(table.locator('tbody tr')).toHaveCount(10)
    await expect(panel.getByText('1 kind')).toHaveCount(0)
    await expect(panel.locator('.context-kinds')).toContainText('always-on rule')
    await expect(panel.locator('.context-kinds')).toContainText('12 sources')
    await panel.getByRole('button', { name: 'Show all 12' }).click()
    await expect(table.locator('tbody tr')).toHaveCount(12)
    await panel.getByRole('button', { name: 'Show top 10' }).click()
    await expect(table.locator('tbody tr')).toHaveCount(10)
  })

  test('opens the source list by default only without duplicates', async ({ page }) => {
    const noDuplicates = structuredClone(contextView)
    noDuplicates.report.opportunities = []
    let withDuplicates = true
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: overview }))
    await page.route('**/api/views/context**', (route) => route.fulfill({ json: withDuplicates ? contextView : noDuplicates }))
    await page.goto('/?view=context&tab=usage')
    const panel = page.locator('section.context-usage')
    await expect(panel.locator('.context-opportunity')).toHaveCount(1)
    await expect(panel.getByRole('table', { name: /^Startup/ })).toBeHidden()
    withDuplicates = false
    await panel.getByRole('button', { name: 'Reread specs' }).click()
    await expect(panel).toContainText('No duplicated passages found for claude.')
    await expect(panel.getByRole('table', { name: /^Startup/ })).toBeVisible()
  })

  test('is the first Context tab and hides the section search', async ({ page }) => {
    await page.goto('/?mock=1')
    const tabs = page.getByRole('tablist').first().getByRole('tab')
    await expect(tabs.first()).toHaveText(/^Context size/)
    await expect(page.getByRole('search')).toHaveCount(1)
    await selectTab(page, 'Context size')
    await expect(page.getByRole('search')).toHaveCount(0)
    await selectTab(page, 'Guidance')
    await expect(page.getByRole('search')).toHaveCount(1)
  })
})

for (const width of [390, 1280]) {
  test(`keeps the report space stable while changing coding tools at ${width}px`, async ({ page }) => {
    const two = { ...structuredClone(contextView), targets: ['claude', 'gemini'], fileTargets: ['claude'] }
    let finish: (() => void) | undefined
    const pending = new Promise<void>((resolve) => { finish = resolve })
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: overview }))
    await page.route('**/api/views/context**', async (route) => {
      const target = new URL(route.request().url()).searchParams.get('target') ?? 'claude'
      if (target === 'gemini') await pending
      return route.fulfill({ json: { ...two, target, report: target === 'gemini' ? { ...two.report, startup: { words: 97, bytes: 700 } } : two.report } })
    })
    await page.setViewportSize({ width, height: 844 })
    await page.goto('/?view=context&tab=usage')
    const panel = page.locator('section.context-usage')
    await expect(panel.locator('.context-usage-totals')).toBeVisible()
    await panel.getByLabel('Check against a file').fill('src/features/context/components/a-long-directory-name/a-long-component-name.tsx')
    await panel.getByRole('button', { name: 'Reread specs' }).click()
    await expect(panel.locator('.context-usage-status')).toContainText('a-long-component-name.tsx')
    const support = page.getByRole('heading', { name: 'Coding tool support', exact: true })
    const position = async () => (await support.boundingBox())!.y + await page.evaluate(() => window.scrollY)
    const before = await position()
    await panel.getByLabel('Coding tool').selectOption('gemini')
    await expect(panel).toHaveAttribute('aria-busy', 'true')
    await expect(panel.locator('.context-usage-totals')).toHaveCount(0)
    expect(Math.abs(await position() - before)).toBeLessThanOrEqual(1)
    await expect(panel.locator('.context-usage-status')).toContainText('Checking context size for gemini')
    finish!()
    await expect(panel).not.toHaveAttribute('aria-busy', 'true')
    await expect(panel.locator('.context-usage-totals')).toBeVisible()
    await expect(panel.getByLabel('Coding tool')).toHaveValue('gemini')
    await expect(panel.locator('.context-total-words').first()).toHaveText('97 words')
  })
}
