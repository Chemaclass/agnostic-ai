import { expect, test } from '@playwright/test'
import { readFileSync } from 'node:fs'
import { MOCK, openApp, saveScreenshot, selectTab } from '../helpers'

const fixture = JSON.parse(readFileSync(new URL('../../src/fixtures/overview.mock.json', import.meta.url), 'utf8'))

test.describe('overview', () => {
  test('keeps recheck feedback when a finding moves to another section', async ({ page }) => {
    const data = structuredClone(fixture)
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: data }))
    await page.goto('/?view=authority')
    const finding = page.locator('.finding', { hasText: 'LINT038' })
    await expect(finding).toBeVisible()
    const authority = data.sections.find((section: { id: string }) => section.id === 'authority')
    const settings = data.sections.find((section: { id: string }) => section.id === 'settings')
    const index = authority.items.findIndex((item: { name: string }) => item.name === 'permissions')
    settings.items.push(authority.items.splice(index, 1)[0])
    await finding.getByRole('button', { name: 'Recheck' }).click()
    await expect(page.getByRole('status').filter({ hasText: 'This finding still appears after the recheck.' })).toBeVisible()
  })

  test('keeps recheck feedback when a finding moves but its item stays', async ({ page }) => {
    const data = structuredClone(fixture)
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: data }))
    await page.goto('/?view=authority')
    const finding = page.locator('.finding', { hasText: 'LINT038' })
    await expect(finding).toBeVisible()
    const authority = data.sections.find((section: { id: string }) => section.id === 'authority')
    const settings = data.sections.find((section: { id: string }) => section.id === 'settings')
    const item = authority.items.find((entry: { name: string }) => entry.name === 'permissions')
    settings.findings.push(item.findings.pop())
    await finding.getByRole('button', { name: 'Recheck' }).click()
    await expect(page.getByRole('status').filter({ hasText: 'This finding still appears after the recheck.' })).toBeVisible()
  })

  test('keeps recheck feedback when a section finding moves to another section', async ({ page }) => {
    const data = structuredClone(fixture)
    const authority = data.sections.find((section: { id: string }) => section.id === 'authority')
    const settings = data.sections.find((section: { id: string }) => section.id === 'settings')
    const item = authority.items.find((entry: { name: string }) => entry.name === 'permissions')
    authority.findings.push(item.findings.pop())
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: data }))
    await page.goto('/?view=authority')
    await page.getByRole('button', { name: 'Review 1 finding', exact: true }).click()
    const finding = page.locator('.finding', { hasText: 'LINT038' })
    await expect(finding).toBeVisible()
    settings.items.push({ ...item, findings: [authority.findings.pop()] })
    await finding.getByRole('button', { name: 'Recheck' }).click()
    await expect(page.getByRole('status').filter({ hasText: 'This finding still appears after the recheck.' })).toBeVisible()
  })

  test('shows the rules table and the settings beside it without folding unset ones away', async ({ page }) => {
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: fixture }))
    await page.goto('/?view=authority')
    await expect(page.locator('.authority-rule').first()).toBeVisible()
    await expect(page.locator('.unconfigured-fields')).toHaveCount(0)
    await expect(page.locator('.authority-also-row', { hasText: 'Default permission mode' })).toContainText('Not set')
  })

  test('shows a check finding on the item it names', async ({ page }) => {
    test.skip(!MOCK, 'The finding is specific to the demo project fixture.')
    await openApp(page, '?view=authority')
    await expect(page.getByRole('heading', { level: 1, name: 'Authority' })).toBeVisible()
    const permissions = page.locator('.item-row', { hasText: 'permissions.yaml' }).first()
    await expect(permissions).toHaveClass(/has-findings/)
    await expect(permissions.locator('.finding')).toContainText('LINT038')
    await expect(permissions.locator('.finding-source')).not.toContainText('running agnostic-ai')
    await expect(permissions.getByRole('button', { name: 'Copy path of .agnostic-ai/settings/permissions.yaml' })).toBeVisible()
    await expect(permissions.getByRole('button', { name: 'Recheck' })).toHaveCount(0)
    await expect(permissions.locator('.finding-recheck-result')).toHaveCount(0)
    await expect(page.getByRole('list', { name: 'Saved Allowed without asking rules' }).getByRole('listitem').first()).toContainText('Allowed')
    await expect(page.locator('.authority-also-row', { hasText: 'Default permission mode' })).toContainText('Not set')
    await expect(page.getByText('Capability domains')).toHaveCount(0)
    await expect(page.getByText('Uncertainties')).toHaveCount(0)
    await saveScreenshot(page, '04-authority.png')
  })

  test('opens a finding at its line in the chosen editor, with Copy path beside it', async ({ page, context }) => {
    const path = 'C:/Users/Some One/project #1/.agnostic-ai/settings/permissions.yaml'
    const data = structuredClone(fixture)
    const authority = data.sections.find((section: { id: string }) => section.id === 'authority')
    const item = authority.items.find((it: { name: string }) => it.name === 'permissions')
    item.findings = [
      { ...item.findings[0], path, line: 7, openPath: path },
      { source: 'doctor', code: 'gone', severity: 'warn', message: 'Names a file that is not on disk', path: 'missing.yaml' },
    ]
    await context.grantPermissions(['clipboard-read', 'clipboard-write'])
    await page.route('**/api/views/overview', (route) => route.fulfill({ json: data }))
    await page.goto('/?view=authority')
    const finding = page.locator('.finding', { hasText: 'LINT038' })
    await expect(finding.locator('.finding-source')).toContainText(`${path}:7`)
    const open = finding.getByRole('link', { name: `Open ${path} in Cursor at line 7` })
    await expect(open).toHaveAttribute('href', 'cursor://file/C:/Users/Some%20One/project%20%231/.agnostic-ai/settings/permissions.yaml:7')

    const nav = page.getByRole('navigation', { name: 'Configuration sections' })
    const chooseInSettings = async (label: string) => {
      await nav.getByRole('link', { name: /^Settings/ }).click()
      await selectTab(page, 'Editor')
      await page.getByRole('combobox', { name: 'Open files in' }).selectOption({ label })
      await nav.getByRole('link', { name: /^Authority/ }).click()
    }
    await chooseInSettings('VS Code')
    const vscode = finding.getByRole('link', { name: `Open ${path} in VS Code at line 7` })
    await expect(vscode).toHaveAttribute('href', 'vscode://file/C:/Users/Some%20One/project%20%231/.agnostic-ai/settings/permissions.yaml:7')
    await expect(page.locator('.item-row', { hasText: 'protected' }).first().getByRole('link', { name: /^Open .+ in VS Code$/ })).toHaveAttribute('href', /^vscode:\/\/file\/.+protected\.yaml$/)

    await page.evaluate(() => document.addEventListener('click', (event) => {
      if ((event.target as Element).closest('a[href^="vscode:"], a[href^="cursor:"]')) event.preventDefault()
    }, true))
    await vscode.focus()
    await page.keyboard.press('Enter')
    await expect(finding.locator('.editor-hint')).toHaveText('Allow your browser to open VS Code. If nothing opens, change the editor in Settings or copy the path.')
    const linkBox = await vscode.boundingBox()
    const hintBox = await finding.locator('.editor-hint').boundingBox()
    expect(hintBox!.y).toBeGreaterThanOrEqual(linkBox!.y + linkBox!.height)
    await finding.getByRole('button', { name: `Copy path of ${path}` }).click()
    expect(await page.evaluate(() => navigator.clipboard.readText())).toBe('C:\\Users\\Some One\\project #1\\.agnostic-ai\\settings\\permissions.yaml')

    const gone = page.locator('.finding', { hasText: 'Names a file that is not on disk' })
    await expect(gone.locator('.finding-source')).toContainText('missing.yaml')
    await expect(gone.locator('.finding-source')).toContainText('This path is not a file on disk, so it cannot be opened in an editor.')
    await expect(gone.getByRole('button', { name: 'Copy path of missing.yaml' })).toBeVisible()
    await expect(gone.getByRole('link')).toHaveCount(0)
    await saveScreenshot(page, '04b-finding-editor.png')

    await finding.locator('.editor-hint').getByRole('link', { name: 'Settings' }).click()
    await expect(page.getByRole('combobox', { name: 'Open files in' })).toHaveValue('vscode')
    await page.getByRole('combobox', { name: 'Open files in' }).selectOption({ label: 'Cursor' })
    await nav.getByRole('link', { name: /^Authority/ }).click()
    await finding.getByRole('link', { name: `Open ${path} in Cursor at line 7` }).click()
    await expect(finding.locator('.editor-hint')).toHaveText('Allow your browser to open Cursor. If nothing opens, change the editor in Settings or copy the path.')

    await chooseInSettings('VS Code')
    await page.reload()
    await expect(page.locator('.finding', { hasText: 'LINT038' }).getByRole('link', { name: `Open ${path} in VS Code at line 7` })).toBeVisible()
    await page.setViewportSize({ width: 390, height: 844 })
    await page.evaluate(() => window.scrollTo(0, 0))
    await chooseInSettings('Cursor')
    await expect(finding.getByRole('link', { name: `Open ${path} in Cursor at line 7` })).toHaveAttribute('href', 'cursor://file/C:/Users/Some%20One/project%20%231/.agnostic-ai/settings/permissions.yaml:7')
  })
})
