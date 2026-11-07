import { expect, test } from '@playwright/test'
import { readFileSync } from 'node:fs'

const fixture = JSON.parse(readFileSync(new URL('../../src/fixtures/overview.mock.json', import.meta.url), 'utf8'))
const idle = { schemaVersion: 'builtin-change/v1', status: 'idle', enabled: false, supported: false, undoAvailable: false, targets: [], changes: [], findings: [] }

test.describe('copy or move a spec to the other level', () => {
  test('previews the destination, moves the agent, and names both sync commands', async ({ page }) => {
    const requests: Record<string, unknown>[] = []
    let moved = false
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: fixture }))
    await page.route('**/api/changes/builtin', (route) => route.fulfill({ json: idle }))
    await page.route('**/api/edit/spec/move', async (route) => {
      const body = route.request().postDataJSON() as Record<string, unknown>
      requests.push(body)
      if (body.action === 'apply') moved = true
      await route.fulfill({ json: { source: '.agnostic-ai/agents/reviewer.md', destination: '~/.agnostic-ai/agents/reviewer.md', exists: false, files: ['reviewer.md'], syncCommands: body.mode === 'move' ? ['agnostic-ai sync', 'agnostic-ai sync --global'] : ['agnostic-ai sync --global'], expected: 'rev-1', ...(moved ? { ok: true } : {}) } })
    })
    await page.goto('./?view=context&tab=agents')
    await page.getByRole('button', { name: 'Copy to global: reviewer' }).click()
    const panel = page.getByRole('region', { name: 'Copy to global: reviewer' })
    await expect(panel).toContainText('~/.agnostic-ai/agents/reviewer.md')
    await expect(panel).toContainText('Nothing is there yet.')
    await expect(panel.getByRole('checkbox')).toHaveCount(0)
    await panel.getByRole('button', { name: 'Move' }).click()
    await expect(page.getByRole('status').filter({ hasText: 'Moved reviewer to ~/.agnostic-ai/agents/reviewer.md.' })).toContainText('agnostic-ai sync --global')
    expect(requests[0]).toMatchObject({ action: 'preview', kind: 'agent', name: 'reviewer', from: 'project', to: 'global', mode: 'copy', overwrite: false })
    expect(requests[1]).toMatchObject({ action: 'apply', mode: 'move', overwrite: false, expected: 'rev-1' })
  })

  test('keeps Copy and Move off until Replace it is ticked', async ({ page }) => {
    const applied: Record<string, unknown>[] = []
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: fixture }))
    await page.route('**/api/changes/builtin', (route) => route.fulfill({ json: idle }))
    await page.route('**/api/edit/spec/move', async (route) => {
      const body = route.request().postDataJSON() as Record<string, unknown>
      if (body.action === 'apply') applied.push(body)
      await route.fulfill({ json: { source: '.agnostic-ai/rules/testing.md', destination: '~/.agnostic-ai/rules/testing.md', exists: true, files: ['testing.md'], syncCommands: ['agnostic-ai sync --global'], expected: 'rev-2' } })
    })
    await page.goto('./?view=context&tab=rules')
    await page.getByRole('button', { name: 'Copy to global: testing' }).click()
    const panel = page.getByRole('region', { name: 'Copy to global: testing' })
    await expect(panel).toContainText('A spec is already there.')
    await expect(panel.getByRole('button', { name: 'Copy', exact: true })).toBeDisabled()
    await expect(panel.getByRole('button', { name: 'Move' })).toBeDisabled()
    await panel.getByRole('checkbox', { name: 'Replace it' }).check()
    await panel.getByRole('button', { name: 'Copy', exact: true }).click()
    await expect(page.getByRole('status').filter({ hasText: 'Copied testing' })).toBeVisible()
    expect(applied).toEqual([expect.objectContaining({ mode: 'copy', overwrite: true, expected: 'rev-2' })])
  })

  test('shows the server refusal in the panel', async ({ page }) => {
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: fixture }))
    await page.route('**/api/changes/builtin', (route) => route.fulfill({ json: idle }))
    await page.route('**/api/edit/spec/move', (route) => route.fulfill({ status: 409, json: { error: 'the global setup does not exist yet; create it first', missing: true, root: '/home/me/.agnostic-ai' } }))
    await page.goto('./?view=context&tab=agents')
    await page.getByRole('button', { name: 'Copy to global: reviewer' }).click()
    await expect(page.getByRole('alert')).toContainText('Open global settings')
  })

  test('names the failed action in the error', async ({ page }) => {
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: fixture }))
    await page.route('**/api/changes/builtin', (route) => route.fulfill({ json: idle }))
    await page.route('**/api/edit/spec/move', async (route) => {
      const body = route.request().postDataJSON() as Record<string, unknown>
      if (body.action === 'apply') return route.fulfill({ status: 409, json: { error: 'the destination changed; preview again' } })
      await route.fulfill({ json: { source: '.agnostic-ai/agents/reviewer.md', destination: '~/.agnostic-ai/agents/reviewer.md', exists: false, files: ['reviewer.md'], syncCommands: ['agnostic-ai sync --global'], expected: 'rev-1' } })
    })
    await page.goto('./?view=context&tab=agents')
    await page.getByRole('button', { name: 'Copy to global: reviewer' }).click()
    await page.getByRole('region', { name: 'Copy to global: reviewer' }).getByRole('button', { name: 'Move' }).click()
    await expect(page.getByRole('alert')).toContainText('Could not move to global')
    await expect(page.getByRole('alert')).toContainText('the destination changed')
  })
})
