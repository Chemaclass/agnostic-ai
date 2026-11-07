import { expect, test, type Page } from '@playwright/test'
import { readFileSync } from 'node:fs'
import { openApp, saveScreenshot } from './helpers'
import { itemEntryId } from '../src/model/identity'
import type { ItemFinding, SectionItem } from '../src/contract'

const fixture = JSON.parse(readFileSync(new URL('../src/fixtures/overview.mock.json', import.meta.url), 'utf8'))

type Overview = typeof fixture
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
  test('counts findings in the top bar, lists them by severity, and drops a fixed one after a recheck', async ({ page }) => {
    const { data, hook } = scenario()
    await serve(page, data)
    await page.goto('/?view=settings')
    const badge = page.locator('.topbar').getByRole('link', { name: 'Setup health: 1 error, 3 warnings' })
    await expect(badge).toHaveText('1 error, 3 warnings', { useInnerText: true })
    await expect(badge).toHaveClass(/is-error/)
    await badge.click()
    await expect(page).toHaveURL(/[?&]view=health(&|$)/)
    await expect(page.getByRole('heading', { level: 1, name: 'Setup health' })).toBeFocused()
    await expect(badge).toHaveAttribute('aria-current', 'page')
    await expect(page.locator('.sidenav').getByRole('link', { name: /^Setup health/ })).toHaveAttribute('aria-current', 'page')

    const errors = page.locator('.health-group', { has: page.getByRole('heading', { name: 'Errors 1' }) })
    const warnings = page.locator('.health-group', { has: page.getByRole('heading', { name: 'Warnings 3' }) })
    await expect(errors.locator('.finding')).toHaveCount(1)
    await expect(errors.getByRole('heading', { level: 3, name: 'Lifecycle' })).toBeVisible()
    await expect(warnings.locator('.finding')).toHaveCount(3)
    await expect(warnings.getByRole('heading', { level: 3, name: 'Context' })).toBeVisible()
    const finding = errors.locator('.finding', { hasText: 'LINT001' })
    await expect(finding.getByRole('button', { name: 'Copy fix prompt' })).toBeVisible()
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
    await expect(feedback).toContainText('This lint finding no longer appears.')
    await expect(page.locator('.topbar').getByRole('link', { name: 'Setup health: 3 warnings' })).toBeVisible()
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
    await expect(page.locator('.health-recheck-result')).toHaveText('Rechecked every finding. 1 finding no longer appears. 3 findings remain.')
    expect(loads - before).toBe(1)
    await expect(page.locator('.health-group .finding', { hasText: 'LINT001' })).toHaveCount(0)
    await expect(page.locator('.topbar').getByRole('link', { name: 'Setup health: 3 warnings' })).toBeVisible()
  })

  test('shows one recheck result when an item has an error and a warning', async ({ page }) => {
    const { data, hook } = scenario()
    const warning: ItemFinding = { source: 'lint', code: 'LINT002', severity: 'warning', message: 'The hook has no timeout.', path: hook.path }
    section(data, 'lifecycle').items[0].findings.push(warning)
    await serve(page, data)
    await page.goto('/?view=health')
    await expect(page.locator('.topbar').getByRole('link', { name: 'Setup health: 1 error, 4 warnings' })).toBeVisible()
    section(data, 'lifecycle').items[0].findings = [warning]
    await page.locator('.finding', { hasText: 'LINT001' }).getByRole('button', { name: 'Recheck', exact: true }).click()
    await expect(page.locator('.finding-rechecked')).toHaveCount(1)
    await expect(page.locator('.health-group', { has: page.getByRole('heading', { name: /^Errors/ }) }).locator('.finding-rechecked')).toHaveCount(1)
    await expect(page.locator('.finding', { hasText: 'LINT002' })).toHaveCount(1)
    await expect(page.locator('.topbar').getByRole('link', { name: 'Setup health: 4 warnings' })).toBeVisible()
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
    const badge = page.locator('.topbar').getByRole('link', { name: 'Setup health: No findings' })
    await expect(badge).toHaveClass(/is-clean/)
    await expect(page.getByText('Lint and doctor found nothing to fix in this configuration.')).toBeVisible()
    await expect(page.getByRole('button', { name: /Copy one fix prompt/ })).toHaveCount(0)
  })

  test('works from data with no server, without Recheck', async ({ page }) => {
    await openApp(page, '?view=health&mock=1')
    await expect(page.locator('.topbar').getByRole('link', { name: 'Setup health: 1 warning' })).toBeVisible()
    const finding = page.locator('.health-group .finding', { hasText: 'LINT038' })
    await expect(finding).toBeVisible()
    await expect(finding.getByRole('button', { name: 'Recheck' })).toHaveCount(0)
    await expect(finding.locator('.finding-recheck-result')).toHaveText('Rechecking a finding needs a running agnostic-ai serve.')
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
    await expect(page.locator('.finding-rechecked')).toContainText('This lint finding no longer appears.')
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
      const badge = page.locator('.topbar').getByRole('link', { name: 'Setup health: 1 error, 3 warnings' })
      await expect(badge).toBeInViewport({ ratio: 1 })
      expect((await badge.boundingBox())!.height).toBeGreaterThanOrEqual(44)
      expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(width)
      await expect(badge).toHaveText(/^\s*1E\s*3W\s*$/, { useInnerText: true })
    }
    await page.locator('.topbar').getByRole('link', { name: /^Setup health/ }).click()
    const current = page.locator('.health-badge[aria-current="page"]')
    expect(await current.evaluate((element) => getComputedStyle(element).boxShadow)).not.toBe('none')
  })

  test('counts a global setup finding and lists it as Global setup', async ({ page }) => {
    const { data } = scenario()
    const settings = section(data, 'settings')
    settings.findings = [{ source: 'lint', code: 'LINT007', severity: 'warn', message: 'Unknown key modl.', path: 'agents/helper.md', openPath: '/home/u/.agnostic-ai/agents/helper.md', layer: 'global' }]
    await serve(page, data)
    await page.goto('/?view=health')
    await expect(page.locator('.topbar').getByRole('link', { name: 'Setup health: 1 error, 4 warnings' })).toBeVisible()
    const warnings = page.locator('.health-group', { has: page.getByRole('heading', { name: 'Warnings 4' }) })
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
    await expect(page.locator('.topbar').getByRole('link', { name: 'Setup health: No findings' })).toBeVisible()
    const gaps = page.getByRole('region', { name: /^Reach the best setup/ })
    await expect(gaps).toContainText('8 of 9 improvements applied')
    await expect(gaps.getByRole('listitem')).toHaveCount(1)
    const gap = gaps.getByRole('listitem').first()
    await expect(gap).toContainText('Skills · No skill yet · Supported by claude, codex')
    await expect(gap.getByRole('button', { name: 'Copy prompt for Skills' })).toHaveText('Copy prompt')
    await context.grantPermissions(['clipboard-read', 'clipboard-write'])
    await gap.getByRole('button', { name: 'Copy prompt for Skills' }).click()
    expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(fix)
    await gap.getByRole('link', { name: 'Context' }).click()
    await expect(page.getByRole('heading', { level: 1, name: 'Context' })).toBeVisible()
  })

  test('says every improvement is applied and lists none', async ({ page }) => {
    const data = structuredClone(fixture)
    data.health = { gaps: [], improvements: { applied: 9, total: 9 } }
    await serve(page, data)
    await page.goto('/?view=health')
    const gaps = page.getByRole('region', { name: /^Reach the best setup/ })
    await expect(gaps).toContainText('All 9 improvements applied')
    await expect(gaps.getByRole('list')).toHaveCount(0)
  })
})

