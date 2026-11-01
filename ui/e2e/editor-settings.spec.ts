import { expect, test, type Page } from '@playwright/test'
import { readFileSync } from 'node:fs'

const fixture = JSON.parse(readFileSync(new URL('../src/fixtures/overview.mock.json', import.meta.url), 'utf8'))

async function serve(page: Page) {
  await page.route('**/api/views/overview', (route) => route.fulfill({ json: structuredClone(fixture) }))
  await page.route('**/api/health', (route) => route.fulfill({ json: { ok: true, root: '/tmp/agnostic-ai', global: false, ui: true } }))
}

const openLink = (page: Page, editor: string) => page.getByRole('link', { name: `Open testing in ${editor}` })

test.describe('editor settings', () => {
  test('keeps the chosen editor for every Open link and after a reload', async ({ page }) => {
    await serve(page)
    await page.goto('/?view=settings&tab=editor')
    const select = page.getByRole('combobox', { name: 'Open files in' })
    await expect(select).toHaveValue('cursor')
    await select.selectOption('zed')
    await page.goto('/?view=context&tab=rules')
    await expect(openLink(page, 'Zed')).toHaveAttribute('href', /^zed:\/\/file\//)
    await page.goto('/?view=settings&tab=editor')
    await expect(page.getByRole('combobox', { name: 'Open files in' })).toHaveValue('zed')
  })

  test('uses a stored VS Code choice from before the Editor tab', async ({ page }) => {
    await serve(page)
    await page.addInitScript(() => window.localStorage.setItem('agnostic-ai.editor', 'vscode'))
    await page.goto('/?view=context&tab=rules')
    await expect(openLink(page, 'VS Code')).toHaveAttribute('href', /^vscode:\/\/file\//)
  })

  test('refuses a link template that cannot open an editor and accepts a valid one', async ({ page }) => {
    await serve(page)
    await page.goto('/?view=settings&tab=editor')
    await page.getByRole('combobox', { name: 'Open files in' }).selectOption('custom')
    const template = page.getByRole('textbox', { name: 'Link template' })
    await expect(template).not.toHaveAttribute('aria-invalid', 'true')
    await expect(page.getByRole('alert')).toHaveText('')
    await template.fill('javascript:alert(1)')
    await expect(page.locator('.editor-settings-error')).toHaveText('That scheme cannot open an editor.')
    await expect(template).toHaveAttribute('aria-invalid', 'true')
    await expect(page.getByRole('alert')).toHaveText('')
    await template.blur()
    await expect(page.getByRole('alert')).toHaveText('That scheme cannot open an editor.')
    await page.goto('/?view=context&tab=rules')
    await expect(openLink(page, 'Cursor')).toHaveAttribute('href', /^cursor:\/\/file\//)
    await page.goto('/?view=settings&tab=editor')
    await template.fill('windsurf://file{path}:{line}')
    await expect(page.locator('.editor-settings-error')).toHaveCount(0)
    await expect(page.getByText('Opens windsurf://file/Users/you/project/AGENTS.md:12')).toBeVisible()
    await page.goto('/?view=context&tab=rules')
    await expect(openLink(page, 'your editor')).toHaveAttribute('href', /^windsurf:\/\/file\//)
  })

  test('opens an http template in a new tab and leaves this page where it is', async ({ page }) => {
    await serve(page)
    await page.context().route('http://localhost:8091/**', (route) => route.fulfill({ contentType: 'text/html', body: 'helper' }))
    await page.addInitScript(() => {
      window.localStorage.setItem('agnostic-ai.editor', 'custom')
      window.localStorage.setItem('agnostic-ai.editor-template', 'http://localhost:8091/?file={path}&line={line}')
    })
    await page.goto('/?view=context&tab=rules')
    const link = openLink(page, 'your editor')
    await expect(link).toHaveAttribute('target', '_blank')
    await expect(link).toHaveAttribute('rel', 'noopener noreferrer')
    const before = page.url()
    const [popup] = await Promise.all([page.waitForEvent('popup'), link.click()])
    await popup.waitForLoadState()
    expect(popup.url()).toMatch(/^http:\/\/localhost:8091\/\?file=\//)
    expect(page.url()).toBe(before)
  })

  test('opens a custom scheme link in this tab, without a target', async ({ page }) => {
    await serve(page)
    await page.addInitScript(() => {
      window.localStorage.setItem('agnostic-ai.editor', 'custom')
      window.localStorage.setItem('agnostic-ai.editor-template', 'windsurf://file{path}:{line}')
    })
    await page.goto('/?view=context&tab=rules')
    const link = openLink(page, 'your editor')
    await expect(link).toHaveAttribute('href', /^windsurf:\/\/file\//)
    await expect(link).not.toHaveAttribute('target', /.+/)
  })

  test('shows the error for a saved invalid template and clears the announcement when the editor changes', async ({ page }) => {
    await serve(page)
    await page.addInitScript(() => {
      window.localStorage.setItem('agnostic-ai.editor', 'custom')
      window.localStorage.setItem('agnostic-ai.editor-template', 'javascript:{path}')
    })
    await page.goto('/?view=settings&tab=editor')
    await expect(page.locator('.editor-settings-error')).toHaveText('That scheme cannot open an editor.')
    await page.getByRole('textbox', { name: 'Link template' }).focus()
    await page.getByRole('textbox', { name: 'Link template' }).blur()
    await expect(page.getByRole('alert')).toHaveText('That scheme cannot open an editor.')
    await page.getByRole('combobox', { name: 'Open files in' }).selectOption('zed')
    await expect(page.getByRole('alert')).toHaveText('')
  })

  test('says so when the browser cannot save the choice, and still applies it', async ({ page }) => {
    await serve(page)
    await page.addInitScript(() => {
      Storage.prototype.setItem = () => { throw new Error('blocked') }
    })
    await page.goto('/?view=settings&tab=editor')
    await page.getByRole('combobox', { name: 'Open files in' }).selectOption('zed')
    await expect(page.getByRole('status').filter({ hasText: 'Could not save in this browser. The choice lasts until you reload.' })).toBeVisible()
    await page.getByRole('tab', { name: /^Config files/ }).click()
    await page.getByRole('link', { name: /^Context/ }).click()
    await page.getByRole('tab', { name: /^Rules/ }).click()
    await expect(openLink(page, 'Zed')).toHaveAttribute('href', /^zed:\/\/file\//)
  })

  test('leads from the hint to the Editor tab', async ({ page }) => {
    await serve(page)
    await page.goto('/?view=context&tab=rules')
    await page.evaluate(() => document.addEventListener('click', (event) => {
      if ((event.target as Element).closest('a[href^="cursor:"]')) event.preventDefault()
    }, true))
    await openLink(page, 'Cursor').click()
    await page.locator('.editor-hint').getByRole('link', { name: 'Settings' }).click()
    await expect(page.getByRole('tab', { name: /^Editor/ })).toHaveAttribute('aria-selected', 'true')
    await expect(page.getByRole('heading', { level: 2, name: 'Editor' })).toBeVisible()
  })

  test('has no Editor tab on the bundled mock data', async ({ page }) => {
    await page.goto('/?mock=1&view=settings')
    await expect(page.getByRole('tab', { name: /^Config files/ })).toBeVisible()
    await expect(page.getByRole('tab', { name: /^Editor/ })).toHaveCount(0)
  })

  for (const width of [1280, 390]) {
    test(`keeps the tab row still when Custom link adds its fields at ${width}px`, async ({ page }) => {
      await serve(page)
      await page.setViewportSize({ width, height: 844 })
      await page.goto('/?view=settings&tab=editor')
      const tabs = page.getByRole('tablist', { name: 'Settings views' })
      const before = await tabs.boundingBox()
      await page.getByRole('combobox', { name: 'Open files in' }).selectOption('custom')
      await page.getByRole('textbox', { name: 'Link template' }).fill('kiro://file{path}')
      const after = await tabs.boundingBox()
      expect(after!.y).toBe(before!.y)
      expect(after!.height).toBe(before!.height)
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
      expect(await page.locator('#main').evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true)
      const box = await page.getByRole('textbox', { name: 'Link template' }).boundingBox()
      if (width <= 760) expect(box!.height).toBeGreaterThanOrEqual(44)
    })
  }
})
