import { expect, test, type Page } from '@playwright/test'

// Approve and Deny on a request the approvals hook holds for the companion (issue #3906),
// against the preview's companion-approve fixture.

const NOW = new Date('2026-11-07T15:30:00Z')

async function openPanel(page: Page) {
  const reset = await page.request.post('/dev/fixture', { data: { name: 'companion-approve' } })
  expect(reset.ok()).toBe(true)
  await page.clock.setFixedTime(NOW)
  await page.goto('/panel.html')
  await expect(page.locator('#list > .row').first()).toBeVisible()
}

const row = (page: Page, title: string) => page.locator('#list').getByRole('option').filter({ hasText: title })
const calls = async (page: Page) => (await (await page.request.get('/dev/calls')).json()) as { path: string; body?: Record<string, unknown> }[]

test('a held request shows the whole command before Approve and Deny', async ({ page }) => {
  await openPanel(page)
  const held = row(page, 'Clean the build folder')
  await expect(held.locator('.held-text')).toHaveText(['rm -rf dist && npm run build -- --mode production'])
  await expect(held.locator('.held-label')).toHaveText(['Run'])
  await expect(held.getByRole('button', { name: 'Approve' })).toBeEnabled()
  await expect(held.getByRole('button', { name: 'Deny' })).toBeEnabled()

  const edit = row(page, 'Webhooks v2 dashboard')
  await expect(edit.locator('.held-label')).toHaveText(['Edit', 'Replace', 'With'])
  await expect(edit.locator('.held-text')).toHaveText(['/Users/you/Code/kombo/src/dashboard.tsx', 'const retries = 3', 'const retries = 5'])
  await expect(edit.locator('.held-more')).toHaveText('+1 more')
})

test('Approve sends the id and digest the row showed, opens nothing, and the request leaves the row', async ({ page }) => {
  await openPanel(page)
  const held = row(page, 'Clean the build folder')
  await held.getByRole('button', { name: 'Approve' }).click()
  await expect(held.locator('.held-request')).toHaveCount(0)
  const sent = (await calls(page)).filter((call) => call.path === '/api/approvals/decide')
  expect(sent.map((call) => call.body)).toEqual([{ id: 'a1b2', digest: 'digest-a1b2', decision: 'approve' }])
  expect((await calls(page)).map((call) => call.path)).not.toContain('/api/open')
})

test('Deny on the first of two requests shows the next one, whose buttons come on after a moment', async ({ page }) => {
  await openPanel(page)
  const edit = row(page, 'Webhooks v2 dashboard')
  await edit.getByRole('button', { name: 'Deny' }).click()
  await expect(edit.locator('.held-text')).toHaveText(['npm test'])
  await expect(edit.locator('.held-more')).toHaveCount(0)
  // A double click must not answer the request that just took the first one's place.
  await expect(edit.getByRole('button', { name: 'Approve' })).toBeDisabled()
  await page.clock.runFor(1000)
  await expect(edit.getByRole('button', { name: 'Approve' })).toBeEnabled()
})

test('a request that changed before the click says so', async ({ page }) => {
  await openPanel(page)
  await page.route('**/api/approvals/decide', (route) => route.fulfill({ status: 409, json: { error: 'This request changed. Check it again.' } }))
  await row(page, 'Clean the build folder').getByRole('button', { name: 'Approve' }).click()
  await expect(page.getByText('This request changed. Check it again.')).toBeVisible()
})

test('no key answers a held request', async ({ page }) => {
  await openPanel(page)
  await page.keyboard.press('ArrowDown')
  await page.keyboard.press('Enter')
  await page.keyboard.press('y')
  expect((await calls(page)).map((call) => call.path)).not.toContain('/api/approvals/decide')
})

test('a held Codex patch offers Answer in terminal, which lets the hook go', async ({ page }) => {
  await openPanel(page)
  const codex = row(page, 'Rename the config keys')
  await expect(codex.locator('.held-label')).toHaveText(['In', 'Patch'])
  await expect(codex.locator('.held-text').first()).toHaveText('/Users/you/Code/kombo')
  await expect(codex.locator('.held-text').nth(1)).toContainText('+MaxRetries int')
  await expect(row(page, 'Clean the build folder').getByRole('button', { name: 'Answer in terminal' })).toHaveCount(0)
  await codex.getByRole('button', { name: 'Answer in terminal' }).click()
  await expect(codex.locator('.held-request')).toHaveCount(0)
  const sent = (await calls(page)).filter((call) => call.path === '/api/approvals/decide')
  expect(sent.map((call) => call.body)).toEqual([{ id: 'a7b8', digest: 'digest-a7b8', decision: 'release' }])
})
