import { expect, test } from '@playwright/test'
import { readFileSync } from 'node:fs'

const fixture = JSON.parse(readFileSync(new URL('../src/fixtures/overview.mock.json', import.meta.url), 'utf8'))
// Reserve the width of a classic scrollbar on the narrowest viewport.
const narrowContent = '@media (max-width: 320px) { .app { max-width: 305px; } }'

test('keeps configuration and editor controls on the second phone header row', async ({ page }) => {
  const overview = structuredClone(fixture)
  overview.snapshot.project.name = 'agnostic-ai'
  await page.route('**/api/views/overview', (route) => route.fulfill({ json: overview }))
  await page.route('**/api/health', (route) => route.fulfill({ json: { ok: true, root: '/tmp/agnostic-ai', global: false, ui: true } }))
  await page.setViewportSize({ width: 320, height: 844 })
  await page.goto('/?view=context&tab=rules')
  await page.addStyleTag({ content: narrowContent })
  const header = page.locator('.topbar')
  const choice = header.getByRole('group', { name: 'Open files in' })
  const editor = choice.getByRole('combobox', { name: 'Editor' })
  const configuration = header.getByRole('button', { name: 'Open global settings' })
  await expect(editor).toBeVisible()
  await editor.selectOption('vscode')
  await expect(page.getByRole('link', { name: 'Open testing in VS Code' })).toHaveAttribute('href', /^vscode:\/\/file\//)

  for (const width of [320, 390]) {
    await page.setViewportSize({ width, height: 844 })
    await expect(configuration).toBeInViewport()
    await expect(editor).toBeInViewport()
    const headerBox = await header.boundingBox()
    const configurationBox = await configuration.boundingBox()
    const editorBox = await editor.boundingBox()
    expect(headerBox!.height).toBeLessThanOrEqual(160)
    expect(await header.locator('.brand-name').evaluate((element) => {
      const range = document.createRange()
      range.selectNodeContents(element)
      return range.getClientRects().length
    })).toBe(1)
    expect(configurationBox!.height).toBeGreaterThanOrEqual(44)
    expect(editorBox!.height).toBeGreaterThanOrEqual(44)
    expect(configurationBox!.y).toBeLessThan(editorBox!.y + editorBox!.height)
    expect(configurationBox!.y + configurationBox!.height).toBeGreaterThan(editorBox!.y)
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(width)
  }

  await page.reload()
  await expect(editor).toHaveValue('vscode')
  await page.setViewportSize({ width: 1280, height: 900 })
  await expect(editor).toBeHidden()
  await expect(choice.getByRole('radio', { name: 'VS Code' })).toHaveAttribute('aria-checked', 'true')
  await choice.getByRole('radio', { name: 'Cursor' }).click()
  await expect(page.getByRole('link', { name: 'Open testing in Cursor' })).toHaveAttribute('href', /^cursor:\/\/file\//)
})

test('keeps global configuration context and the project switch compact on phones', async ({ page }) => {
  const overview = structuredClone(fixture)
  const root = '/Users/chema/Code/Chemaclass/agnostic-ai-home'
  overview.snapshot.project.name = 'global (/Users/chema/.agnostic-ai)'
  overview.snapshot.project.root = root
  await page.route('**/api/views/overview', (route) => route.fulfill({ json: overview }))
  await page.route('**/api/health', (route) => route.fulfill({ json: { ok: true, root, global: true, ui: true, projectRoot: '/Users/chema/Code/Chemaclass/agnostic-ai' } }))
  await page.setViewportSize({ width: 390, height: 844 })
  await page.goto('/?view=settings&tab=preferences')
  await page.addStyleTag({ content: narrowContent })
  const header = page.locator('.topbar')
  const project = header.getByRole('button', { name: 'Open project agnostic-ai' })
  const editor = header.getByRole('combobox', { name: 'Editor' })
  await expect(project).toBeVisible()

  for (const width of [390, 320]) {
    await page.setViewportSize({ width, height: 844 })
    await expect(header.locator('.project-title .chip')).toBeVisible()
    await expect(header.locator('.project-title .chip')).toHaveText('Global')
    await expect(header.locator('.project-root')).toHaveText(root)
    await expect(project).toBeInViewport()
    await expect(editor).toBeInViewport()
    expect((await header.boundingBox())!.height).toBeLessThanOrEqual(160)
    expect((await project.boundingBox())!.height).toBeGreaterThanOrEqual(44)
    expect((await editor.boundingBox())!.height).toBeGreaterThanOrEqual(44)
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(width)
  }
})
