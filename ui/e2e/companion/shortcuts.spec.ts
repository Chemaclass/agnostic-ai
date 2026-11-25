import { expect, test, type Page } from '@playwright/test'

async function openPanel(page: Page, fixture = 'mixed') {
  await page.request.post('/dev/fixture', { data: { name: fixture } })
  await page.goto('/panel.html')
  await expect(page.locator('#list > .row').first()).toBeVisible()
}

const sheet = (page: Page) => page.getByRole('dialog', { name: 'Keyboard shortcuts', exact: true })
const firstRow = (page: Page) => page.locator('#list > .row').first()

async function openFromMenu(page: Page) {
  await page.locator('#menu-button').click()
  const opener = page.getByRole('button', { name: 'Keyboard shortcuts', exact: true })
  await opener.click()
  await expect(sheet(page)).toBeVisible()
  await expect(sheet(page).getByRole('button', { name: 'Close', exact: true })).toBeFocused()
  await expect(sheet(page).getByRole('button', { name: 'Close', exact: true })).toBeInViewport()
  return opener
}

test('the menu opens the shared list and Escape restores its visible action', async ({ page }) => {
  await openPanel(page)
  const opener = await openFromMenu(page)
  const metadata = await (await page.request.get('/menu.json')).json()
  await expect(sheet(page).locator('#shortcut-help-panel dt')).toHaveCount(metadata.shortcutHelp.length)
  for (const row of metadata.shortcutHelp) await expect(sheet(page)).toContainText(row.description)
  await expect(sheet(page).locator('#shortcut-help-panel')).toContainText('⌘⇧V')
  await page.keyboard.press('Tab')
  await expect(sheet(page).getByRole('region', { name: 'Keyboard shortcut list' })).toBeFocused()
  await page.keyboard.press('Tab')
  await expect(sheet(page).getByRole('button', { name: 'Close', exact: true })).toBeFocused()
  await page.keyboard.press('Escape')
  await expect(sheet(page)).toBeHidden()
  await expect(opener).toBeVisible()
  await expect(opener).toBeFocused()
})

test('Shift question mark opens from a row and restores that row after a redraw', async ({ page }) => {
  await openPanel(page)
  const row = firstRow(page)
  const key = await row.getAttribute('data-key')
  await row.focus()
  await page.keyboard.press('Shift+Slash')
  await expect(sheet(page)).toBeVisible()
  await page.request.post('/dev/arrive')
  await expect(page.locator('#list')).toContainText('Answer the review comments')
  await page.keyboard.press('Escape')
  await expect(sheet(page)).toBeHidden()
  await expect(page.locator(`.row[data-key="${key}"]`)).toBeFocused()
})

test('question mark in empty search or editable fields stays text', async ({ page }) => {
  await openPanel(page)
  const query = page.getByRole('textbox', { name: 'Search every conversation' })
  await query.focus()
  await page.keyboard.type('?')
  await expect(query).toHaveValue('?')
  await expect(sheet(page)).toBeHidden()
  await query.fill('')
  await expect(firstRow(page)).toBeVisible()
  for (const tag of ['input', 'textarea', 'div']) {
    const field = await firstRow(page).evaluateHandle((row, tag) => {
      const field = document.createElement(tag)
      if (tag === 'div') field.contentEditable = 'true'
      row.append(field)
      field.focus()
      return field
    }, tag)
    await page.keyboard.type('?')
    await expect(sheet(page)).toBeHidden()
    await field.evaluate((field) => field.remove())
  }
})

test('question mark does not open while search contains text or another menu is open', async ({ page }) => {
  let releaseMenu!: () => void
  let menuRequested!: () => void
  const pendingMenu = new Promise<void>((resolve) => { releaseMenu = resolve })
  const requestedMenu = new Promise<void>((resolve) => { menuRequested = resolve })
  await page.route('**/menu.json', async (route) => {
    const response = await route.fetch()
    menuRequested()
    await pendingMenu
    await route.fulfill({ response })
  })
  await openPanel(page)
  const query = page.getByRole('textbox', { name: 'Search every conversation' })
  await query.fill('migration')
  await expect(firstRow(page)).toBeVisible()
  await firstRow(page).focus()
  await page.keyboard.press('?')
  await expect(sheet(page)).toBeHidden()
  await query.fill('')
  await expect(firstRow(page)).toBeVisible()
  await page.locator('#menu-button').click()
  await requestedMenu
  const settingsMenu = page.getByRole('region', { name: 'Settings', exact: true })
  await expect(settingsMenu).toBeHidden()
  releaseMenu()
  await expect(settingsMenu).toBeVisible()
  await firstRow(page).focus()
  await page.keyboard.press('?')
  await expect(sheet(page)).toBeHidden()
  await expect(settingsMenu).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(settingsMenu).toBeHidden()
  await firstRow(page).click({ button: 'right' })
  const rowMenu = page.getByRole('menu', { name: 'Conversation', exact: true })
  await expect(rowMenu).toBeVisible()
  await firstRow(page).focus()
  await page.keyboard.press('?')
  await expect(sheet(page)).toBeHidden()
  await expect(rowMenu).toBeVisible()
})

test('modifiers, repeat and input method candidates do not open the sheet', async ({ page }) => {
  await openPanel(page)
  await firstRow(page).focus()
  for (const press of [{ ctrlKey: true }, { altKey: true }, { metaKey: true }, { repeat: true }, { isComposing: true }, { keyCode: 229 }]) {
    await firstRow(page).dispatchEvent('keydown', { key: '?', bubbles: true, ...press })
    await expect(sheet(page)).toBeHidden()
  }
})

test('global shortcuts follow changed preferences while the sheet stays open', async ({ page }) => {
  await openPanel(page)
  await openFromMenu(page)
  const globals = sheet(page).locator('#shortcut-help-global')
  await expect(globals).toContainText('⌃⌥Space')
  await page.request.post('/api/preferences/shortcut', { data: { shortcut: 'Cmd+Shift+K' } })
  await page.request.post('/api/preferences/send-shortcut', { data: { shortcut: 'Cmd+Alt+B' } })
  await expect(globals).toContainText('⌘⇧K')
  await expect(globals).toContainText('⌘⌥B')
  await sheet(page).getByRole('button', { name: 'Close', exact: true }).click()
  await page.getByRole('button', { name: 'Keyboard shortcuts', exact: true }).click()
  await expect(globals).toContainText('⌘⇧K')
})

test('the sheet holds list keys and the paste picker suppresses question mark help', async ({ page }) => {
  await openPanel(page)
  await firstRow(page).focus()
  await page.keyboard.press('?')
  await expect(sheet(page)).toBeVisible()
  await page.keyboard.press('F2')
  await page.keyboard.press('Meta+Shift+V')
  await page.keyboard.press('ArrowDown')
  await expect(sheet(page)).toBeVisible()
  await expect(page.locator('.rename-input')).toHaveCount(0)
  await expect(page.locator('#pick')).toBeHidden()
  await page.keyboard.press('Escape')
  await page.keyboard.press('Meta+Shift+V')
  await expect(page.locator('#pick')).toBeVisible()
  await firstRow(page).focus()
  await page.keyboard.press('?')
  await expect(sheet(page)).toBeHidden()
})

test('Rename keeps its accessible name and shows F2 only for supported rows', async ({ page }) => {
  await openPanel(page)
  await firstRow(page).click({ button: 'right' })
  const rename = page.getByRole('menuitem', { name: 'Rename…', exact: true })
  await expect(rename).toBeVisible()
  await expect(rename.locator('kbd')).toHaveText('F2')
  await page.keyboard.press('Escape')
  await openPanel(page, 'approvals')
  const cloud = page.locator('.row').filter({ hasText: 'Release notes draft' })
  await cloud.click({ button: 'right' })
  await expect(rename).toBeHidden()
  await expect(page.locator('#row-menu kbd:visible')).toHaveCount(0)
})


test('a menu click restores its action when the platform leaves focus in search', async ({ page }) => {
  await openPanel(page)
  await page.locator('#menu-button').click()
  const opener = page.getByRole('button', { name: 'Keyboard shortcuts', exact: true })
  await opener.evaluate((button) => button.addEventListener('mousedown', (event) => event.preventDefault()))
  await page.getByRole('textbox', { name: 'Search every conversation' }).focus()
  await opener.click()
  await expect(sheet(page)).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(opener).toBeFocused()
})

test('an initial preferences read keeps a newer live event', async ({ page }) => {
  const errors: string[] = []
  page.on('pageerror', (error) => errors.push(error.message))
  let release!: () => void
  let readReady!: () => void
  const pending = new Promise<void>((resolve) => { release = resolve })
  const ready = new Promise<void>((resolve) => { readReady = resolve })
  await page.route('**/api/preferences', async (route) => {
    const response = await route.fetch()
    const stale = await response.json()
    readReady()
    await pending
    await route.fulfill({ json: stale })
  })
  await openPanel(page)
  await page.evaluate(async () => {
    const runtimePath = '/wails/runtime.js'
    const runtime = await import(runtimePath)
    runtime.Events.On('preferences', () => { document.documentElement.dataset.preferencesHeard = 'true' })
  })
  await firstRow(page).focus()
  await page.keyboard.press('?')
  await ready
  await page.request.post('/api/preferences/shortcut', { data: { shortcut: 'Cmd+Shift+K' } })
  await expect(page.locator('html')).toHaveAttribute('data-preferences-heard', 'true')
  release()
  await expect(sheet(page).locator('#shortcut-help-global')).toContainText('⌘⇧K')
  expect(errors).toEqual([])
  await page.keyboard.press('Escape')
  await expect(firstRow(page)).toBeFocused()
})


test('a short window scrolls the help rows and keeps Close visible', async ({ page }) => {
  await page.setViewportSize({ width: 432, height: 520 })
  await openPanel(page)
  await openFromMenu(page)
  const content = sheet(page).getByRole('region', { name: 'Keyboard shortcut list' })
  const close = sheet(page).getByRole('button', { name: 'Close', exact: true })
  expect(await content.evaluate((node) => node.scrollHeight > node.clientHeight)).toBe(true)
  await expect(close).toBeFocused()
  await page.keyboard.press('Tab')
  await expect(content).toBeFocused()
  const start = await content.evaluate((node) => node.scrollTop)
  await page.keyboard.press('PageDown')
  await expect.poll(() => content.evaluate((node) => node.scrollTop)).toBeGreaterThan(start)
  await page.keyboard.press('End')
  await expect(sheet(page).locator('#shortcut-help-panel > div').last()).toBeInViewport()
  const end = await content.evaluate((node) => node.scrollTop)
  await page.keyboard.press('ArrowUp')
  await expect.poll(() => content.evaluate((node) => node.scrollTop)).toBeLessThan(end)
  await page.keyboard.press('Shift+Tab')
  await expect(close).toBeFocused()
  await page.keyboard.press('Shift+Tab')
  await expect(content).toBeFocused()
  await page.keyboard.press('Tab')
  await expect(close).toBeFocused()
  await sheet(page).getByRole('button', { name: 'Close', exact: true }).click()
  await expect(page.getByRole('button', { name: 'Keyboard shortcuts', exact: true })).toBeFocused()
})
