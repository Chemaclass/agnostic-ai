import { expect, test, type Page } from '@playwright/test'
import { EFFECT_WORDING, MOCK, PILL_WORDING, deriveToolView, fixture, isImpactRequest, openApp, saveScreenshot, scrollToTop, stubImpactInMockMode } from './helpers'

// Domains are found by id: their labels are the server's to choose.
const CLOUD = 'cloud-deployment'
const domainCard = (page: Page, id: string) => page.locator(`a.domain-card[href*="domain=${id}"]`)

async function openAuthority(page: Page) {
  await openApp(page)
  await page.getByRole('navigation', { name: 'Harness' }).getByRole('link', { name: 'Authority' }).click()
  await expect(page.getByRole('heading', { level: 1, name: 'Authority' })).toBeVisible()
  await expect(page.locator('.domain-card').first()).toBeVisible()
}

async function openCloud(page: Page) {
  await openAuthority(page)
  const card = domainCard(page, CLOUD)
  const label = (await card.locator('.domain-card-title').innerText()).trim()
  await card.click()
  await expect(page.getByRole('heading', { level: 1, name: label })).toBeVisible()
}

/** The Railway resource: its undeclared scope, titled with the server's name. */
async function openRailway(page: Page) {
  await openCloud(page)
  const card = page.locator('.provider-group', { hasText: /railway/i }).locator('.resource-card').first()
  await expect(card).toContainText('Scope not declared · credential reach not inspected')
  await card.click()
  await expect(page.getByRole('heading', { level: 1, name: /railway/i })).toBeVisible()
  await expect(page.locator('.page-subtitle')).toHaveText('Scope not declared · credential reach not inspected')
}

const deployRow = (page: Page) => page.locator('tbody.op-group[data-action="deploy"]')

test.describe('authority', () => {
  test('opens from the navigation with the capability domains', async ({ page }) => {
    await openAuthority(page)
    await expect(page.getByRole('navigation', { name: 'Harness' }).getByRole('link', { name: 'Authority' })).toHaveAttribute('aria-current', 'page')
    const domains = page.locator('.domain-card')
    if (MOCK) {
      await expect(domains).toHaveCount(8)
      await expect(domainCard(page, CLOUD)).toContainText('Cloud Infrastructure & Deployment')
    }
    await expect(domainCard(page, CLOUD)).toBeVisible()
    await expect(domainCard(page, 'secrets-identity')).toBeVisible()
    await expect(page.getByText('Runtime enforcement: not observed')).toBeVisible()
    await saveScreenshot(page, '01-authority-domains.png')
  })

  test('opens Cloud Infrastructure & Deployment with providers and resources', async ({ page }) => {
    await openCloud(page)
    const railway = page.locator('.provider-group', { hasText: /railway/i })
    await expect(railway.locator('.provider-chip')).toHaveText(/railway/i)
    const card = railway.locator('.resource-card').first()
    await expect(card.locator('.status-pill')).toHaveText(PILL_WORDING)
    await expect(card).toContainText(/deploy/i)
    await saveScreenshot(page, '02-cloud-resources.png')
  })

  test('selects the Railway scope and reads the deploy row', async ({ page }) => {
    await openRailway(page)
    const table = page.locator('table.ops-table')
    await expect(table.locator('thead')).toContainText(/Effective on /)
    for (const column of ['Operation', 'Declared', 'Granted by', 'Installed', 'Runtime']) await expect(table.locator('thead')).toContainText(column)

    const row = deployRow(page)
    await expect(row.locator('.op-row th')).toContainText(/Deploy/i)
    await expect(row.locator('td[data-label="Declared"]')).toContainText('Declared:')
    const effective = row.locator('td.effective-cell .badge').first()
    await expect(effective).toHaveText(EFFECT_WORDING)
    // A hypothetical effect is never shown as a plain "Permitted".
    const installed = (await row.locator('td[data-label="Installed"] .badge').innerText()).trim()
    if (/Not generated|Stale|Not checked/.test(installed)) await expect(effective).not.toHaveText(/^\W*Permitted$/)
    await expect(row.locator('td[data-label="Granted by"]')).toContainText('mcp:railway/deploy')
    await expect(row.locator('td[data-label="Granted by"]')).toContainText(/permissions\.yaml/)
    await expect(row.locator('td[data-label="Runtime"]')).toHaveText('Not observed')

    // Alternate routes never claim that no route exists.
    const routes = row.locator('.op-notes')
    await expect(routes).toContainText('Alternate routes')
    await expect(routes.locator('.routes-label').first()).toHaveText('Checked')
    await expect(routes.locator('.routes-label').nth(1)).toHaveText('Not modeled')
    await expect(routes).not.toContainText(/\bnone\b/i)

    // Why? lists every grant with the winner marked, and every coding tool.
    await row.getByRole('button', { name: /Why\?/ }).click()
    const why = row.locator('.op-why')
    await expect(why.locator('.grant-list li.is-winner')).toHaveCount(1)
    await expect(why.locator('.grant-list li.is-winner')).toContainText('Wins')
    await expect(why.locator('.enforcement-table tbody tr')).not.toHaveCount(0)
    await expect(why.locator('.enforcement-table tr[aria-current="true"]')).toHaveCount(1)
    await row.getByRole('button', { name: /Why\?/ }).click()

    await scrollToTop(page, '#ops-title')
    await saveScreenshot(page, '03-resource-detail.png')
  })

  test('opens the permission circuit and selects a resource from it', async ({ page }) => {
    await openCloud(page)
    const circuitButton = page.getByRole('button', { name: 'Circuit' })
    await circuitButton.click()
    await expect(circuitButton).toHaveAttribute('aria-pressed', 'true')
    const svg = page.getByTestId('permission-circuit')
    await expect(svg).toBeVisible()
    const pills = await svg.locator('.pill').count()
    expect(pills).toBeGreaterThan(0)
    await expect(svg.locator('path.edge')).toHaveCount(pills * 2)
    await expect(svg.locator('.node-subject')).toContainText('SUBJECT')
    await expect(page.locator('.legend')).toContainText('How to read it')
    await scrollToTop(page, '.view-switch', 96)
    await saveScreenshot(page, '04-permission-circuit.png')

    await svg.locator('a.node-resource').first().click()
    await expect(page.locator('table.ops-table')).toBeVisible()
    await expect(page.getByTestId('permission-circuit').locator('a.node-resource[aria-current="true"]')).toHaveCount(1)
  })

  test('runs a proposal: client preview first, then the server impact', async ({ page, context }) => {
    await context.grantPermissions(['clipboard-read', 'clipboard-write'])
    await stubImpactInMockMode(page)
    await openRailway(page)

    const row = deployRow(page)
    await row.getByRole('button', { name: /^Propose/ }).click()
    const requireApproval = page.locator('.propose-popover [data-action="require-approval"]')
    const choice = (await requireApproval.isEnabled()) ? requireApproval : page.locator('.propose-popover [data-action="block"]')
    await choice.click()

    const panel = page.locator('section.proposal')
    await expect(panel.getByRole('heading', { name: 'Proposal' })).toBeVisible()
    await expect(panel.locator('.proposal-head .badge')).toHaveText('Proposed')
    await expect(panel.getByText('Proposed · client preview, not validated')).toBeVisible()
    await expect(panel.locator('.change-list')).toContainText('mcp:railway/deploy')
    await expect(panel.locator('.preview table')).toContainText(/Deploy/i)
    // The circuit marks the affected edge as proposed.
    await expect(page.getByTestId('permission-circuit').locator('.proposed-tag').first()).toHaveText('Proposed')
    // Apply is not supported: the button is disabled and the server's own words say why.
    const apply = panel.getByRole('button', { name: 'Apply' })
    await expect(apply).toBeDisabled()
    const applyNote = page.locator(`[id="${await apply.getAttribute('aria-describedby')}"]`)
    await expect(applyNote).not.toBeEmpty()
    if (MOCK) await expect(applyNote).toContainText(/not supported/i)

    const request = page.waitForRequest(isImpactRequest)
    await panel.getByRole('button', { name: 'Validate on server' }).click()
    const body = (await request).postDataJSON()
    expect(body).toMatchObject({ agentId: expect.any(String), target: expect.any(String), revision: expect.any(String) })
    expect(body.changes).toEqual([expect.objectContaining({ op: 'add', rule: 'mcp:railway/deploy' })])

    const impact = panel.locator('.impact')
    await expect(impact).toHaveAttribute('aria-live', 'polite')
    await expect(impact.locator('.impact-result')).toBeVisible()
    const valid = (await impact.locator('.impact-result h3').innerText()).includes('Validated, not applied')
    if (valid) {
      await expect(panel.locator('.proposal-head .badge')).toHaveText('Validated, not applied')
      await expect(impact).toContainText(/operations? change|No operation changes/)
      await expect(impact.locator('.diff').first()).toBeVisible()
      await expect(impact.getByRole('button', { name: 'Copy diff' }).first()).toBeVisible()
      await expect(page.getByTestId('permission-circuit').locator('.proposed-tag').first()).toHaveText('Validated, not applied')
      // Once validated, the client preview steps aside for the server's result.
      await expect(panel.getByText('Proposed · client preview, not validated')).toHaveCount(0)
    } else {
      await expect(impact.locator('.impact-invalid')).toContainText('Not valid')
    }
    await scrollToTop(page, 'section.proposal')
    await saveScreenshot(page, '05-proposal-impact.png')
    if (valid) {
      await impact.getByRole('button', { name: 'Copy diff' }).first().click()
      await expect(impact.getByRole('button', { name: 'Diff copied' })).toBeVisible()
    }
  })

  test('opens the JSON inspector with the endpoint, schema, and revision', async ({ page, context }) => {
    await context.grantPermissions(['clipboard-read', 'clipboard-write'])
    await openRailway(page)
    const button = page.getByRole('button', { name: 'JSON', exact: true })
    await button.click()
    await expect(button).toHaveAttribute('aria-pressed', 'true')
    const drawer = page.getByRole('dialog', { name: 'JSON inspector' })
    await expect(drawer).toBeVisible()
    await expect(drawer.locator('.inspector-meta')).toContainText(MOCK ? 'Mock fixture' : 'GET /api/views/authority')
    await expect(drawer.locator('.inspector-meta')).toContainText('authority/v1')
    const revision = (await page.locator('.footer code').first().innerText()).trim()
    await expect(drawer.locator('.inspector-meta')).toContainText(revision)
    await expect(drawer.locator('pre.json')).toContainText('"schemaVersion"')
    await drawer.getByRole('button', { name: 'Copy JSON' }).click()
    await expect(drawer.getByRole('button', { name: 'JSON copied' })).toBeVisible()
    await saveScreenshot(page, '06-json-inspector.png')

    await drawer.getByRole('button', { name: 'Overview' }).click()
    await expect(drawer.locator('.inspector-meta')).toContainText('overview/v1')
    await page.keyboard.press('Escape')
    await expect(drawer).toHaveCount(0)
    await expect(button).toBeFocused()
  })

  test('shows a section overview for Control', async ({ page }) => {
    await openApp(page, '?view=control')
    await expect(page.getByRole('heading', { level: 1, name: 'Control' })).toBeVisible()
    await expect(page.locator('.page-subtitle')).not.toBeEmpty()
    await expect(page.getByText('Deep view not built yet; Authority is the reference implementation')).toBeVisible()
    await expect(page.locator('.support-table')).toBeVisible()
    await expect(page.getByRole('heading', { name: 'Items' })).toBeVisible()
    await saveScreenshot(page, '07-section-overview-control.png')
  })

  test('stops circuit motion under reduced motion', async ({ page }) => {
    await openCloud(page)
    await page.getByRole('button', { name: 'Circuit' }).click()
    const permitted = page.getByTestId('permission-circuit').locator('path.edge-permitted').first()
    test.skip((await permitted.count()) === 0, 'No permitted edge to animate in this domain.')
    expect(await permitted.evaluate((el) => getComputedStyle(el).animationName)).toBe('flow')
    await page.emulateMedia({ reducedMotion: 'reduce' })
    expect(await permitted.evaluate((el) => getComputedStyle(el).animationName)).toBe('none')
    expect(await permitted.evaluate((el) => getComputedStyle(el).strokeDasharray)).toBe('none')
    // Static arrowheads still show the direction.
    expect(await permitted.evaluate((el) => el.closest('svg')!.querySelectorAll('path.edge-out[marker-end]').length)).toBeGreaterThan(0)
  })

  test('reads a static snapshot with no server', async ({ page }) => {
    const authority = fixture('authority.mock.json') as { snapshot: { revision: string } }
    await page.route('**/snapshot/authority.json', (route) => route.fulfill({ json: authority }))
    await page.route('**/snapshot/overview.json', (route) => route.fulfill({ json: fixture('overview.mock.json') }))
    await page.route('**/snapshot/authority.*.*.json', (route) => route.fulfill({ status: 404, body: 'not found' }))
    await page.route('**/api/**', (route) => route.abort())

    await page.goto('./?source=./snapshot&view=authority')
    await expect(page.locator('.chip-source')).toHaveText(`Static snapshot · revision ${authority.snapshot.revision}`)
    await expect(page.getByRole('heading', { level: 1, name: 'Authority' })).toBeVisible()

    await page.getByRole('combobox', { name: 'Agent' }).selectOption({ label: 'Reviewer' })
    await expect(page.locator('.banner-info')).toContainText('This snapshot holds no view for Reviewer')

    await page.goto('./?source=./snapshot&view=authority&domain=cloud-deployment&resource=mcp-railway-scope')
    const row = deployRow(page)
    await row.getByRole('button', { name: /^Propose/ }).click()
    await page.locator('.propose-popover [data-action="require-approval"]').click()
    const panel = page.locator('section.proposal')
    await expect(panel.getByText('Proposed · client preview, not validated')).toBeVisible()
    await expect(panel.getByRole('button', { name: 'Validate on server' })).toBeDisabled()
    await expect(panel.locator('.proposal-actions')).toContainText('needs a running server')
  })

  test('reads a per-tool snapshot file and keeps unresolved effects apart from the declared policy', async ({ page }) => {
    await page.route('**/snapshot/authority.json', (route) => route.fulfill({ json: fixture('authority.mock.json') }))
    await page.route('**/snapshot/overview.json', (route) => route.fulfill({ json: fixture('overview.mock.json') }))
    await page.route('**/snapshot/authority.coding-agent.codex.json', (route) => route.fulfill({ json: deriveToolView('codex') }))
    await page.route('**/snapshot/authority.coding-agent.cursor.json', (route) => route.fulfill({ json: deriveToolView('cursor') }))
    await page.route('**/api/**', (route) => route.abort())

    await page.goto('./?source=./snapshot&view=authority&domain=cloud-deployment&resource=mcp-railway-scope')
    await expect(deployRow(page).locator('td.effective-cell .badge').first()).toHaveText(/Would be permitted once synced/)
    await page.getByRole('combobox', { name: 'Coding tool' }).selectOption({ label: 'Codex' })
    await expect(page.getByRole('combobox', { name: 'Coding tool' })).toHaveValue('codex')
    await expect(page.locator('.banner-info')).toHaveCount(0)
    await expect(page.locator('table.ops-table thead')).toContainText('Effective on Codex')

    // Codex writes nothing for the rule: unresolved, with the declared policy muted beside it.
    const effective = deployRow(page).locator('td.effective-cell')
    await expect(effective.locator('.badge').first()).toHaveText(/Unresolved on this tool$/)
    await expect(effective.locator('.badge-muted')).toHaveText(/Declared: permitted/)
    // The circuit marks declared ≠ effective.
    await expect(page.getByTestId('permission-circuit').locator('.declared-marker').first()).toBeVisible()

    // Cursor writes Write(src/**) but its matching is not documented.
    await page.getByRole('combobox', { name: 'Coding tool' }).selectOption({ label: 'Cursor' })
    await page.goto('./?source=./snapshot&view=authority&target=cursor&domain=filesystem-execution&resource=files-src')
    const edit = page.locator('tbody.op-group[data-action="edit"] td.effective-cell')
    await expect(edit.locator('.badge').first()).toHaveText(/Unresolved on this tool · rule written, matching not verified/)
    await expect(edit.locator('.badge-muted')).toHaveText(/Declared: permitted/)
  })
})
