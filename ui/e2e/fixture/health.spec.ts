import { expect, test, type Page } from '@playwright/test'
import { readFileSync } from 'node:fs'
import { openApp, saveScreenshot } from '../helpers'
import { itemEntryId } from '../../src/model/identity'
import type { ItemFinding, SectionItem } from '../../src/contract'

const fixture = JSON.parse(readFileSync(new URL('../../src/fixtures/overview.mock.json', import.meta.url), 'utf8'))

type Overview = typeof fixture
// The rail row names the counts first, then the status word.
const healthRow = (page: Page, summary: string) => page.locator('.sidenav').getByRole('link', { name: new RegExp(`^Setup health: ${summary} `) })
const section = (data: Overview, id: string) => data.sections.find((s: { id: string }) => s.id === id)

// One lint error on a hook and three doctor warnings on skills; the fixture's own finding is removed.
function scenario() {
  const data = structuredClone(fixture)
  for (const s of data.sections) {
    s.findings = []
    for (const item of s.items) item.findings = []
  }
  const hook: SectionItem = { kind: 'hook', name: 'prepare', label: 'Prepare session', description: '', path: '.agnostic-ai/hooks/prepare.yaml', layer: 'project', group: 'session-start', values: [], findings: [] }
  hook.findings = [{ source: 'lint', code: 'LINT001', severity: 'error', message: 'The hook has no command.', path: hook.path, fix: 'Add a command.' }]
  const lifecycle = section(data, 'lifecycle')
  lifecycle.items = [hook]
  lifecycle.kinds[0].count = 1
  const context = section(data, 'context')
  for (const name of ['review', 'deploy', 'docs']) {
    const finding: ItemFinding = { source: 'doctor', severity: 'warning', message: `Skill ${name} is not synced.`, path: `.agnostic-ai/skills/${name}/SKILL.md` }
    context.items.push({ kind: 'skill', name, label: name, description: '', path: `.agnostic-ai/skills/${name}/SKILL.md`, layer: 'project', values: [], findings: [finding] })
  }
  return { data, hook }
}

async function serve(page: Page, data: Overview) {
  await page.route('**/api/views/overview', (route) => route.fulfill({ json: data }))
}

test.describe('setup health', () => {
  test('counts findings in the rail, lists them by severity, and drops a fixed one after a recheck', async ({ page }) => {
    const { data, hook } = scenario()
    await serve(page, data)
    await page.goto('/?view=settings')
    const badge = healthRow(page, '1 error, 3 warnings')
    await expect(badge).toHaveAccessibleName('Setup health: 1 error, 3 warnings Needs a fix')
    await expect(badge.locator('.nav-dot')).toHaveClass(/is-error/)
    await badge.click()
    await expect(page).toHaveURL(/[?&]view=health(&|$)/)
    await expect(page.getByRole('heading', { level: 1, name: 'Setup health' })).toBeFocused()
    await expect(page.locator('.page-verdict')).toHaveText('1 thing needs a fix, and 3 more are worth a look.')
    await expect(page.locator('.page-facts')).toContainText('Checked just now')
    await expect(badge).toHaveAttribute('aria-current', 'page')

    const errors = page.locator('.health-group', { has: page.getByRole('heading', { name: 'Needs a fix 1' }) })
    const warnings = page.locator('.health-group', { has: page.getByRole('heading', { name: 'Worth a look 3' }) })
    await expect(errors.locator('.finding')).toHaveCount(1)
    await expect(errors.getByRole('heading', { level: 3, name: 'Lifecycle' })).toBeVisible()
    await expect(warnings.locator('.finding')).toHaveCount(3)
    await expect(warnings.getByRole('heading', { level: 3, name: 'Context' })).toBeVisible()
    const finding = errors.locator('.finding', { hasText: 'LINT001' })
    await expect(finding.getByRole('button', { name: 'Copy prompt' })).toBeVisible()
    await expect(finding.locator('.finding-label')).toHaveText('Needs a fix')
    await expect(finding.getByText('Details', { exact: true })).toBeVisible()
    await saveScreenshot(page, 'setup-health.png', { fullPage: true })

    await errors.getByRole('link', { name: 'Prepare session' }).click()
    await expect(page).toHaveURL(/view=lifecycle/)
    await expect(page.locator(`[data-entry="${itemEntryId(hook)}"]`)).toBeFocused()
    await page.goBack()
    await expect(page).toHaveURL(/view=health/)

    hook.findings = []
    await page.locator('.health-group .finding', { hasText: 'LINT001' }).getByRole('button', { name: 'Recheck', exact: true }).click()
    const feedback = page.locator('.finding-rechecked')
    await expect(feedback).toHaveCount(1)
    await expect(feedback).toContainText('This finding no longer appears.')
    await expect(healthRow(page, '3 warnings')).toBeVisible()
  })

  test('rechecks every section at once and says how many findings remain', async ({ page }) => {
    const { data, hook } = scenario()
    let loads = 0
    await page.route('**/api/views/overview', (route) => {
      loads++
      return route.fulfill({ json: data })
    })
    await page.goto('/?view=health')
    const recheckAll = page.getByRole('button', { name: 'Recheck all' })
    await expect(recheckAll).toBeVisible()
    const before = loads
    hook.findings = []
    await recheckAll.click()
    const result = page.locator('.health-recheck-result')
    await expect(result).toHaveText('Rechecked every finding. 1 finding no longer appears. 3 findings remain.')
    await expect(result.locator('.banner')).toHaveClass(/banner-warning/)
    expect(loads - before).toBe(1)
    await expect(page.locator('.health-group .finding', { hasText: 'LINT001' })).toHaveCount(0)
    await expect(healthRow(page, '3 warnings')).toBeVisible()
  })

  test('shows one recheck result when an item has an error and a warning', async ({ page }) => {
    const { data, hook } = scenario()
    const warning: ItemFinding = { source: 'lint', code: 'LINT002', severity: 'warning', message: 'The hook has no timeout.', path: hook.path }
    section(data, 'lifecycle').items[0].findings.push(warning)
    await serve(page, data)
    await page.goto('/?view=health')
    await expect(healthRow(page, '1 error, 4 warnings')).toBeVisible()
    section(data, 'lifecycle').items[0].findings = [warning]
    await page.locator('.finding', { hasText: 'LINT001' }).getByRole('button', { name: 'Recheck', exact: true }).click()
    await expect(page.locator('.finding-rechecked')).toHaveCount(1)
    await expect(page.locator('.health-group', { has: page.getByRole('heading', { name: /^Needs a fix/ }) }).locator('.finding-rechecked')).toHaveCount(1)
    await expect(page.locator('.finding', { hasText: 'LINT002' })).toHaveCount(1)
    await expect(healthRow(page, '4 warnings')).toBeVisible()
  })

  test('copies one prompt for every finding, errors first', async ({ page, context }) => {
    const { data } = scenario()
    await serve(page, data)
    await context.grantPermissions(['clipboard-read', 'clipboard-write'])
    await page.goto('/?view=health')
    await page.getByRole('button', { name: 'Copy one fix prompt for all 4 findings' }).click()
    const prompt = await page.evaluate(() => navigator.clipboard.readText())
    expect(prompt).toMatch(/^Fix these 4 agnostic-ai findings/)
    expect(prompt.indexOf('1. Lifecycle: Prepare session')).toBeLessThan(prompt.indexOf('2. Context: review'))
  })

  test('links a section finding to its folded list', async ({ page }) => {
    const { data } = scenario()
    section(data, 'settings').findings = [{ source: 'doctor', severity: 'warning', message: 'The configuration file is old.' }]
    await serve(page, data)
    await page.goto('/?view=health')
    await page.locator('.health-place', { hasText: 'The configuration file is old.' }).getByRole('link', { name: 'Other findings' }).click()
    await expect(page).toHaveURL(/view=settings/)
    await expect(page.locator('.finding', { hasText: 'The configuration file is old.' })).toBeFocused()
  })

  test('says when there is nothing to fix', async ({ page }) => {
    const { data } = scenario()
    section(data, 'lifecycle').items[0].findings = []
    for (const item of section(data, 'context').items) item.findings = []
    await serve(page, data)
    await page.goto('/?view=health')
    const badge = healthRow(page, 'No findings')
    await expect(badge).toContainText('All clear')
    await expect(badge.locator('.nav-dot')).toHaveClass(/is-clean/)
    await expect(page.locator('.page-verdict')).toHaveText('Nothing needs a fix.')
    await expect(page.locator('.health-group')).toHaveCount(0)
    await expect(page.getByRole('button', { name: /Copy one fix prompt/ })).toHaveCount(0)
  })

  test('works from data with no server, without Recheck', async ({ page }) => {
    await openApp(page, '?view=health&mock=1')
    await expect(healthRow(page, '1 warning')).toBeVisible()
    const finding = page.locator('.health-group .finding', { hasText: 'LINT038' })
    await expect(finding).toBeVisible()
    await expect(finding.getByRole('button', { name: 'Recheck' })).toHaveCount(0)
    await expect(finding.locator('.finding-recheck-result')).toHaveCount(0)
    await expect(page.locator('.page-facts')).toContainText('From a saved snapshot')
    await expect(page.locator('.health-track-bar')).toHaveCount(14)
    await expect(page.locator('.health-track-bar.is-on')).toHaveCount(2)
    await expect(page.locator('.health-track')).toContainText('2 of 14 features your tools support are in use')
    await expect(page.locator('.page > .banner-info')).toContainText('Opening files, rechecks and switches need agnostic-ai view.')
    await expect(page.getByRole('button', { name: 'Copy the fix prompt for the one finding' })).toBeVisible()
  })

  test('leaves a recheck result from this page off the section page', async ({ page }) => {
    const { data, hook } = scenario()
    await serve(page, data)
    await page.goto('/?view=health')
    const recheck = page.locator('.finding', { hasText: 'LINT001' }).getByRole('button', { name: 'Recheck', exact: true })
    await expect(recheck).toBeVisible()
    hook.findings = []
    await recheck.click()
    await expect(page.locator('.finding-rechecked')).toContainText('This finding no longer appears.')
    await page.locator('.health-group .finding-rechecked').locator('xpath=ancestor::div[contains(@class,"health-place")]').getByRole('link', { name: 'Prepare session' }).click()
    await expect(page).toHaveURL(/view=lifecycle/)
    await expect(page.locator(`[data-entry="${itemEntryId(hook)}"]`)).toBeFocused()
    await expect(page.locator('.finding-rechecked')).toHaveCount(0)
    await expect(page.locator('.section-warnings[open]')).toHaveCount(0)
  })

  test('keeps the badge compact on phones', async ({ page }) => {
    const { data } = scenario()
    await serve(page, data)
    for (const width of [320, 390]) {
      await page.setViewportSize({ width, height: 844 })
      await page.goto('/?view=health')
      const badge = page.locator('.topbar').getByRole('link', { name: 'Setup health: 1 error, 3 warnings', exact: true })
      await expect(badge).toBeInViewport({ ratio: 1 })
      expect((await badge.boundingBox())!.height).toBeGreaterThanOrEqual(44)
      expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(width)
      await expect(badge).toHaveText(/^\s*1E\s*3W\s*$/, { useInnerText: true })
    }
    await page.locator('.topbar').getByRole('link', { name: /^Setup health/ }).click()
    const current = page.locator('.health-badge[aria-current="page"]')
    expect(await current.evaluate((element) => getComputedStyle(element).borderColor)).toBe(await current.evaluate((element) => getComputedStyle(element).color))
  })

  test('counts a global setup finding and lists it as Global setup', async ({ page }) => {
    const { data } = scenario()
    const settings = section(data, 'settings')
    settings.findings = [{ source: 'lint', code: 'LINT007', severity: 'warn', message: 'Unknown key modl.', path: 'agents/helper.md', openPath: '/home/u/.agnostic-ai/agents/helper.md', layer: 'global' }]
    await serve(page, data)
    await page.goto('/?view=health')
    await expect(healthRow(page, '1 error, 4 warnings')).toBeVisible()
    const warnings = page.locator('.health-group', { has: page.getByRole('heading', { name: 'Worth a look 4' }) })
    const place = warnings.locator('.health-place', { hasText: 'Global setup' })
    await expect(place.locator('.finding')).toHaveCount(1)
    await expect(place.locator('.finding .badge', { hasText: 'global' })).toBeVisible()
    await place.getByText('Details', { exact: true }).click()
    await expect(place.locator('textarea')).toHaveValue(/Source: \/home\/u\/\.agnostic-ai\/agents\/helper\.md/)
  })

  test('lists what the enabled tools could get below the findings, without counting it', async ({ page, context }) => {
    const data = structuredClone(fixture)
    for (const s of data.sections) {
      s.findings = []
      for (const item of s.items) item.findings = []
    }
    const fix = 'This agnostic-ai project has no skills, and claude, codex support them.'
    data.health = { gaps: [{ id: 'kind:skill', section: 'context', label: 'Skills', supported: ['claude', 'codex'], missing: 'no skill yet', fix }], improvements: { applied: 8, total: 9 } }
    await serve(page, data)
    await page.goto('/?view=health')
    await expect(healthRow(page, 'No findings')).toBeVisible()
    await expect(page.locator('.health-track')).toContainText('8 of 9 features your tools support are in use')
    await expect(page.locator('.page-facts')).toContainText('1 feature you could add')
    const gaps = page.getByRole('region', { name: /^Reach the best setup/ })
    await expect(gaps.getByRole('listitem')).toHaveCount(1)
    const gap = gaps.getByRole('listitem').first()
    await expect(gap.locator('.health-gap-name')).toHaveText('Skills')
    await expect(gap.locator('.health-gap-text')).toContainText('No skill yet. Supported by Claude Code and Codex.')
    await expect(gap.getByRole('button', { name: 'Copy prompt for Skills' })).toHaveText('Copy prompt')
    await context.grantPermissions(['clipboard-read', 'clipboard-write'])
    await gap.getByRole('button', { name: 'Copy prompt for Skills' }).click()
    expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(fix)
    await gap.getByRole('link', { name: 'Context' }).click()
    await expect(page.getByRole('heading', { level: 1, name: 'Context' })).toBeVisible()
  })

  test('says every feature is in use and lists none', async ({ page }) => {
    const data = structuredClone(fixture)
    data.health = { gaps: [], improvements: { applied: 9, total: 9 } }
    await serve(page, data)
    await page.goto('/?view=health')
    await expect(page.locator('.health-track')).toContainText('All 9 features your tools support are in use')
    await expect(page.locator('.health-track-bar:not(.is-on)')).toHaveCount(0)
    await expect(page.getByRole('region', { name: /^Reach the best setup/ })).toHaveCount(0)
  })

  test('keeps the exact counts but no bars for an oversized feature total', async ({ page }) => {
    const data = structuredClone(fixture)
    data.health = { gaps: [], improvements: { applied: 1, total: 4294967296 } }
    await serve(page, data)
    await page.goto('/?view=health')
    await expect(page.locator('.health-track')).toContainText('1 of 4294967296 features your tools support are in use')
    await expect(page.locator('.health-track-bar')).toHaveCount(0)
  })

  test('hides the feature track without improvements data', async ({ page }) => {
    const data = structuredClone(fixture)
    data.health = { gaps: [] }
    await serve(page, data)
    await page.goto('/?view=health')
    await expect(page.locator('.page-verdict')).toBeVisible()
    await expect(page.locator('.health-track')).toHaveCount(0)
  })
})

