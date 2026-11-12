import { expect, test, type Page } from '@playwright/test'

// The list's ⋯ menu, the quit question and the hidden circle, on the real companion pages served by companion/dev/preview.mjs.

async function openPanel(page: Page) {
  const reset = await page.request.post('/dev/fixture', { data: { name: 'mixed' } })
  expect(reset.ok()).toBe(true)
  await page.goto('/panel.html')
  await expect(page.locator('#list > .row, #list > .empty').first()).toBeVisible()
}

async function openSettings(page: Page) {
  await page.getByRole('button', { name: 'Settings', exact: true }).click()
  await expect(page.getByRole('region', { name: 'Settings', exact: true })).toBeVisible()
}

function sent(page: Page, path: string) {
  const paths: string[] = []
  page.on('request', (request) => {
    if (request.method() === 'POST' && new URL(request.url()).pathname === path) paths.push(path)
  })
  return paths
}

test('the keys move through the menu and end on Quit, after Hide', async ({ page }) => {
  await openPanel(page)
  await openSettings(page)

  await page.getByRole('button', { name: 'Settings', exact: true }).focus()
  const menu = page.getByRole('region', { name: 'Settings', exact: true })
  const order = [
    menu.getByRole('switch', { name: 'Animate companion' }),
    menu.getByRole('switch', { name: 'Dim when idle' }),
    menu.getByRole('switch', { name: 'Start at login' }),
    menu.getByRole('button', { name: 'Open from anywhere' }),
    menu.getByRole('button', { name: /^Hide Agnostic AI companion/ }),
    menu.getByRole('button', { name: 'Quit Agnostic AI companion' }),
  ]
  for (const item of order) {
    await page.keyboard.press('Tab')
    await expect(item).toBeFocused()
  }
})

test('Escape closes the menu and keeps the list; a second Escape hides the list', async ({ page }) => {
  await openPanel(page)
  const hides = sent(page, '/api/panel/hide')
  await openSettings(page)

  await page.keyboard.press('Escape')
  await expect(page.getByRole('region', { name: 'Settings', exact: true })).toBeHidden()
  await expect(page.getByLabel('Search every conversation')).toBeVisible()
  expect(hides).toEqual([])

  const hidden = page.waitForRequest((request) => new URL(request.url()).pathname === '/api/panel/hide')
  await page.keyboard.press('Escape')
  await hidden
})

const quitQuestion = (page: Page) => page.getByRole('alertdialog', { name: 'Quit Agnostic AI companion?' })

async function askQuit(page: Page) {
  await openSettings(page)
  await page.getByRole('button', { name: 'Quit Agnostic AI companion' }).click()
  await expect(quitQuestion(page)).toBeVisible()
}

test('Quit asks first, says how to start again, and focuses Cancel', async ({ page }) => {
  await openPanel(page)
  const quits = sent(page, '/api/quit')

  await askQuit(page)

  await expect(quitQuestion(page)).toContainText('the shortcut will not bring it back')
  await expect(quitQuestion(page)).toContainText('run agnostic-ai companion, or ask your AI tool to run it')
  await expect(quitQuestion(page)).toContainText('The shortcut works only after Hide.')
  await expect(quitQuestion(page).getByRole('button', { name: 'Cancel' })).toBeFocused()
  await expect(page.getByRole('region', { name: 'Settings', exact: true })).toBeHidden()
  expect(quits).toEqual([])
})

test('Quit in the question asks the shell to end the companion', async ({ page }) => {
  await openPanel(page)
  await askQuit(page)

  const quit = page.waitForRequest((request) => request.method() === 'POST' && new URL(request.url()).pathname === '/api/quit')
  await quitQuestion(page).getByRole('button', { name: 'Quit', exact: true }).click()
  await quit
  await expect.poll(async () => (await (await page.request.get('/dev/calls')).json())[0]?.path).toBe('/api/quit')
})

test('Hide instead hides the companion and does not quit', async ({ page }) => {
  await openPanel(page)
  const quits = sent(page, '/api/quit')
  await askQuit(page)

  const hidden = page.waitForRequest((request) => request.method() === 'POST' && new URL(request.url()).pathname === '/api/hide')
  await quitQuestion(page).getByRole('button', { name: 'Hide instead' }).click()
  await hidden
  await expect(quitQuestion(page)).toBeHidden()
  expect((await (await page.request.get('/api/appearance')).json()).presence).toBe('hidden')
  expect(quits).toEqual([])
})

for (const [how, cancel] of [
  ['Cancel', (page: Page) => quitQuestion(page).getByRole('button', { name: 'Cancel' }).click()],
  ['Escape', (page: Page) => page.keyboard.press('Escape')],
  ['a click away', (page: Page) => page.getByLabel('Search every conversation').click()],
] as const) {
  test(`${how} answers the quit question with no change`, async ({ page }) => {
    await openPanel(page)
    const quits = sent(page, '/api/quit')
    const hides = sent(page, '/api/hide')
    await askQuit(page)

    await cancel(page)

    await expect(quitQuestion(page)).toBeHidden()
    await expect(page.getByLabel('Search every conversation')).toBeVisible()
    expect(quits).toEqual([])
    expect(hides).toEqual([])
    expect((await (await page.request.get('/api/appearance')).json()).presence).toBe('shown')
  })
}

test('Quit in the right-click menu opens the list on the question', async ({ page }) => {
  await openPanel(page)
  const quits = sent(page, '/api/quit')

  await page.request.post('/dev/ask-quit')

  await expect(quitQuestion(page)).toBeVisible()
  await expect(quitQuestion(page).getByRole('button', { name: 'Cancel' })).toBeFocused()
  expect(quits).toEqual([])
})

async function openCircle(page: Page, fixture = 'mixed') {
  const reset = await page.request.post('/dev/fixture', { data: { name: fixture } })
  expect(reset.ok()).toBe(true)
  await page.goto('/')
}

const circle = (page: Page) => page.locator('#bubble')

test('Hide in the menu names the shortcut and asks the shell to hide', async ({ page }) => {
  await openPanel(page)
  await openSettings(page)

  const hide = page.getByRole('button', { name: 'Hide Agnostic AI companion (⌃⌥Space brings it back)' })
  await expect(hide).toBeVisible()
  const hidden = page.waitForRequest((request) => request.method() === 'POST' && new URL(request.url()).pathname === '/api/hide')
  await hide.click()
  await hidden
  const appearance = await (await page.request.get('/api/appearance')).json()
  expect(appearance.presence).toBe('hidden')
})

test('Hide follows a new shortcut', async ({ page }) => {
  await openPanel(page)
  await openSettings(page)

  await page.request.post('/api/preferences/shortcut', { data: { shortcut: 'Ctrl+Shift+K' } })

  await expect(page.getByRole('button', { name: 'Hide Agnostic AI companion (⌃⇧K brings it back)' })).toBeVisible()
})

test('hidden, a chat that starts waiting brings the circle back at 25% with its count', async ({ page }) => {
  await openCircle(page)
  await expect(page.locator('#badge')).toHaveText('2')
  expect((await page.request.post('/api/hide')).status()).toBe(204)
  await expect(page.locator('html')).toHaveAttribute('data-presence', 'hidden')

  await page.request.post('/dev/arrive')

  await expect(page.locator('html')).toHaveAttribute('data-presence', 'faded')
  await expect(circle(page)).toHaveCSS('opacity', '0.25')
  await expect(page.locator('#badge')).toHaveText('3')
  await circle(page).hover()
  await expect(circle(page)).toHaveCSS('opacity', '0.25')
})

test('a click on the faded circle brings back full color, and it stays', async ({ page }) => {
  await openCircle(page)
  await page.request.post('/api/hide')
  await page.request.post('/dev/arrive')
  await expect(circle(page)).toHaveCSS('opacity', '0.25')

  await circle(page).click()

  await expect(page.locator('html')).toHaveAttribute('data-presence', 'shown')
  await expect(circle(page)).toHaveCSS('opacity', '1')
  await page.request.post('/dev/write', { data: { tool: 'claude', id: 'arrived-1' } })
  await expect(circle(page)).toHaveCSS('opacity', '1')
})

test('chats already waiting keep a hidden circle hidden, and Hide hides a faded one again', async ({ page }) => {
  await openCircle(page)
  await page.request.post('/api/hide')
  await page.request.post('/dev/arrive')
  await expect(page.locator('html')).toHaveAttribute('data-presence', 'faded')

  await page.request.post('/api/hide')
  await expect(page.locator('html')).toHaveAttribute('data-presence', 'hidden')
  // The same chat, still waiting, is listed again: nothing new waits.
  await page.request.post('/dev/arrive')
  await page.waitForTimeout(300)

  await expect(page.locator('html')).toHaveAttribute('data-presence', 'hidden')
})

test('the faded circle stays at 25% with Dim when idle on', async ({ page }) => {
  await openCircle(page)
  await page.request.post('/dev/appearance', { data: { dimWhenIdle: true } })
  await page.request.post('/api/hide')
  await page.request.post('/dev/arrive')

  await expect(circle(page)).toHaveCSS('opacity', '0.25')
})

const settingsMenu = (page: Page) => page.getByRole('region', { name: 'Settings', exact: true })

test('the menu shows the shared list, in its order, with the values in effect', async ({ page }) => {
  const items: { label: string }[] = await (await page.request.get('/menu.json')).json()
  await openPanel(page)
  await openSettings(page)

  const labels = await settingsMenu(page).locator('.menu-label, .menu-item').allTextContents()
  expect(labels).toEqual(items.map((item) => (item.label.startsWith('Hide') ? `${item.label} (⌃⌥Space brings it back)` : item.label)))
  await expect(settingsMenu(page).getByRole('switch', { name: 'Animate companion' })).toHaveAttribute('aria-checked', 'true')
  await expect(settingsMenu(page).getByRole('switch', { name: 'Dim when idle' })).toHaveAttribute('aria-checked', 'false')
  await expect(settingsMenu(page).getByRole('switch', { name: 'Start at login' })).toHaveAttribute('aria-checked', 'false')
})

test('switching Animate in the menu stops the circle on its own page', async ({ page, context }) => {
  await openPanel(page)
  const circlePage = await context.newPage()
  await circlePage.goto('/')
  await expect(circlePage.locator('html')).toHaveAttribute('data-motion', 'on')
  await openSettings(page)

  const saved = page.waitForRequest((request) => new URL(request.url()).pathname === '/api/appearance/animate')
  await settingsMenu(page).getByRole('switch', { name: 'Animate companion' }).click()
  expect((await saved).postDataJSON()).toEqual({ enabled: false })

  await expect(settingsMenu(page).getByRole('switch', { name: 'Animate companion' })).toHaveAttribute('aria-checked', 'false')
  await expect(circlePage.locator('html')).toHaveAttribute('data-motion', 'off')
})

test('switching Dim when idle in the menu saves it', async ({ page }) => {
  await openPanel(page)
  await openSettings(page)

  await settingsMenu(page).getByRole('switch', { name: 'Dim when idle' }).click()

  await expect(settingsMenu(page).getByRole('switch', { name: 'Dim when idle' })).toHaveAttribute('aria-checked', 'true')
  expect((await (await page.request.get('/api/appearance')).json()).dimWhenIdle).toBe(true)
})

test('a change made in the right-click menu shows in the open menu', async ({ page }) => {
  await openPanel(page)
  await openSettings(page)

  // The tests post what the right-click menu would change.
  await page.request.post('/dev/appearance', { data: { animate: false, dimWhenIdle: true } })
  await page.request.post('/api/preferences/start-at-login', { data: { enabled: true } })

  await expect(settingsMenu(page).getByRole('switch', { name: 'Animate companion' })).toHaveAttribute('aria-checked', 'false')
  await expect(settingsMenu(page).getByRole('switch', { name: 'Dim when idle' })).toHaveAttribute('aria-checked', 'true')
  await expect(settingsMenu(page).getByRole('switch', { name: 'Start at login' })).toHaveAttribute('aria-checked', 'true')
})

test('Open from anywhere in the right-click menu opens the menu on the shortcut, without recording', async ({ page }) => {
  await openPanel(page)

  await page.request.post('/dev/open-settings')

  await expect(settingsMenu(page)).toBeVisible()
  const shortcut = settingsMenu(page).getByRole('button', { name: 'Open from anywhere' })
  await expect(shortcut).toBeFocused()
  await expect(shortcut).not.toHaveClass(/is-recording/)
})
