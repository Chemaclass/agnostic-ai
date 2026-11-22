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
    menu.getByRole('switch', { name: 'Small companion' }),
    menu.getByRole('switch', { name: 'Start at login' }),
    menu.getByRole('switch', { name: 'Ask me before an agent sends a message' }),
    menu.getByRole('button', { name: 'Open from anywhere' }),
    menu.getByRole('button', { name: 'Send copied text' }),
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
  await expect(quitQuestion(page)).toContainText('Open the app again, or run agnostic-ai companion to restart it.')
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

test('Enter on Quit with a row selected asks, and neither opens the row nor hides the list', async ({ page }) => {
  await openPanel(page)
  const opens = sent(page, '/api/open')
  const hides = sent(page, '/api/panel/hide')
  await page.getByLabel('Search every conversation').focus()
  await page.keyboard.press('ArrowDown')
  await openSettings(page)

  await page.getByRole('button', { name: 'Quit Agnostic AI companion' }).focus()
  await page.keyboard.press('Enter')

  await expect(quitQuestion(page)).toBeVisible()
  await page.waitForTimeout(200)
  expect(opens).toEqual([])
  expect(hides).toEqual([])
})

test('the quit question keeps the keys: Tab goes round its three buttons', async ({ page }) => {
  await openPanel(page)
  const marks = sent(page, '/api/mark')
  await askQuit(page)
  const question = quitQuestion(page)

  const order = ['Hide instead', 'Quit', 'Cancel', 'Hide instead']
  for (const name of order) {
    await page.keyboard.press('Tab')
    await expect(question.getByRole('button', { name, exact: true })).toBeFocused()
  }
  await page.keyboard.press('Shift+Tab')
  await expect(question.getByRole('button', { name: 'Cancel' })).toBeFocused()
  await page.keyboard.press('Enter')

  await expect(question).toBeHidden()
  expect(marks).toEqual([])
})

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

test('Hide in the menu asks the shell to hide', async ({ page }) => {
  await openPanel(page)
  await openSettings(page)

  const hide = page.getByRole('button', { name: 'Hide Agnostic AI companion', exact: true })
  await expect(hide).toBeVisible()
  const hidden = page.waitForRequest((request) => request.method() === 'POST' && new URL(request.url()).pathname === '/api/hide')
  await hide.click()
  await hidden
  const appearance = await (await page.request.get('/api/appearance')).json()
  expect(appearance.presence).toBe('hidden')
})

test('Hide stays plain after a new shortcut', async ({ page }) => {
  await openPanel(page)
  await openSettings(page)

  await page.request.post('/api/preferences/shortcut', { data: { shortcut: 'Ctrl+Shift+K' } })

  await expect(page.getByRole('button', { name: 'Hide Agnostic AI companion', exact: true })).toBeVisible()
})

test('hidden, an unread reply brings back the faded circle without changing the waiting count', async ({ page }) => {
  await openCircle(page)
  await expect(page.locator('#badge')).toHaveText('1')
  expect((await page.request.post('/api/hide')).status()).toBe(204)
  await expect(page.locator('html')).toHaveAttribute('data-presence', 'hidden')

  await page.request.post('/dev/arrive')

  await expect(page.locator('html')).toHaveAttribute('data-presence', 'faded')
  await expect(circle(page)).toHaveCSS('opacity', '0.25')
  await expect(circle(page)).toHaveAccessibleName('Conversations: 1 waiting for you, 2 unread')
  await expect(page.locator('#badge')).toHaveText('1')
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

test('Dim when idle does not lift a hidden idle circle to 55%', async ({ page }) => {
  await openCircle(page, 'empty')
  await page.request.post('/dev/appearance', { data: { dimWhenIdle: true } })
  await expect(circle(page)).toHaveCSS('opacity', '0.55')

  await page.request.post('/api/hide')

  await expect(circle(page)).toHaveCSS('opacity', '0.25')
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
  const items: { id: string; label: string }[] = await (await page.request.get('/menu.json')).json()
  await openPanel(page)
  await openSettings(page)

  // This fixture cannot update itself, so it has no update entry and no switch for it.
  const labels = await settingsMenu(page).locator('.menu-label, .menu-item').allTextContents()
  expect(labels).toEqual(items.filter((item) => item.id !== 'update' && item.id !== 'checkForUpdates').map((item) => item.label))
  await expect(settingsMenu(page).getByRole('switch', { name: 'Animate companion' })).toHaveAttribute('aria-checked', 'true')
  await expect(settingsMenu(page).getByRole('switch', { name: 'Dim when idle' })).toHaveAttribute('aria-checked', 'false')
  await expect(settingsMenu(page).getByRole('switch', { name: 'Small companion' })).toHaveAttribute('aria-checked', 'false')
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

test('the approval switch saves the mailbox setting and starts on', async ({ page }) => {
  await openPanel(page)
  await openSettings(page)
  const ask = settingsMenu(page).getByRole('switch', { name: 'Ask me before an agent sends a message' })
  await expect(ask).toHaveAttribute('aria-checked', 'true')

  const saved = page.waitForRequest((request) => new URL(request.url()).pathname === '/api/mailbox/settings' && request.method() === 'POST')
  await ask.click()
  expect((await saved).postDataJSON()).toEqual({ approveAgentSends: false })

  await expect(ask).toHaveAttribute('aria-checked', 'false')
  expect((await (await page.request.get('/api/mailbox/settings')).json()).approveAgentSends).toBe(false)
})

test('the menu still opens when the mailbox setting cannot be read, without its switch', async ({ page }) => {
  await page.route('**/api/mailbox/settings', (route) => route.fulfill({ status: 502, contentType: 'application/json', body: '{"error":"The server is restarting."}' }))
  await openPanel(page)
  await openSettings(page)
  await expect(settingsMenu(page).getByRole('switch', { name: 'Start at login' })).toBeVisible()
  await expect(settingsMenu(page).getByRole('switch', { name: 'Ask me before an agent sends a message' })).toBeHidden()
})

test('switching Small companion in the menu saves it', async ({ page }) => {
  await openPanel(page)
  await openSettings(page)

  const saved = page.waitForRequest((request) => new URL(request.url()).pathname === '/api/appearance/small-companion')
  await settingsMenu(page).getByRole('switch', { name: 'Small companion' }).click()
  expect((await saved).postDataJSON()).toEqual({ enabled: true })

  await expect(settingsMenu(page).getByRole('switch', { name: 'Small companion' })).toHaveAttribute('aria-checked', 'true')
  expect((await (await page.request.get('/api/appearance')).json()).smallCompanion).toBe(true)
})

test('the small window shows the circle, its mark, ring and badge at two thirds', async ({ page }) => {
  await page.request.post('/dev/fixture', { data: { name: 'mixed' } })
  // The shell sizes the native window: 92 points for the regular circle, 64 for the small one.
  await page.setViewportSize({ width: 92, height: 92 })
  await page.goto('/')
  await expect(page.locator('#badge')).toBeVisible()
  await expect(page.locator('#bubble')).toHaveCSS('width', '60px')
  await expect(page.locator('.mark')).toHaveCSS('width', '32px')
  await expect(page.locator('.ring')).toHaveCSS('width', '68px')
  await expect(page.locator('#badge')).toHaveCSS('height', '22px')

  await page.setViewportSize({ width: 64, height: 64 })

  await expect(page.locator('#bubble')).toHaveCSS('width', '40px')
  await expect(page.locator('.mark')).toHaveCSS('width', '21px')
  await expect(page.locator('.ring')).toHaveCSS('width', '46px')
  await expect(page.locator('#badge')).toHaveCSS('height', '16px')
})

test('a change made in the right-click menu shows in the open menu', async ({ page }) => {
  await openPanel(page)
  await openSettings(page)

  // The tests post what the right-click menu would change.
  await page.request.post('/dev/appearance', { data: { animate: false, dimWhenIdle: true, smallCompanion: true } })
  await page.request.post('/api/preferences/start-at-login', { data: { enabled: true } })

  await expect(settingsMenu(page).getByRole('switch', { name: 'Animate companion' })).toHaveAttribute('aria-checked', 'false')
  await expect(settingsMenu(page).getByRole('switch', { name: 'Dim when idle' })).toHaveAttribute('aria-checked', 'true')
  await expect(settingsMenu(page).getByRole('switch', { name: 'Small companion' })).toHaveAttribute('aria-checked', 'true')
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

test('Send copied text in the right-click menu opens the menu on the send shortcut', async ({ page }) => {
  await openPanel(page)

  await page.request.post('/dev/open-send-settings')

  await expect(settingsMenu(page)).toBeVisible()
  const send = settingsMenu(page).getByRole('button', { name: 'Send copied text' })
  await expect(send).toBeFocused()
  await expect(send).not.toHaveClass(/is-recording/)
})

test('keys every app uses are refused and the old shortcut stays', async ({ page }) => {
  await openPanel(page)
  await openSettings(page)
  const posts = sent(page, '/api/preferences/shortcut')
  const open = settingsMenu(page).getByRole('button', { name: 'Open from anywhere' })

  for (const [keys, spelled] of [['Meta+KeyQ', 'Cmd+Q'], ['Meta+KeyC', 'Cmd+C'], ['Control+Tab', 'Ctrl+Tab']]) {
    await open.click()
    await page.keyboard.press(keys)
    await expect(settingsMenu(page)).toContainText(`${spelled} is a shortcut every app uses; choose other keys`)
    await expect(open).toHaveText('⌃⌥Space')
  }
  expect(posts).toEqual([])

  await open.click()
  await page.keyboard.press('Control+Alt+KeyK')
  await expect(open).toHaveText('⌃⌥K')
  await expect(settingsMenu(page)).not.toContainText('every app uses')
})

test('the send shortcut changes on its own, and the open shortcut\'s keys are refused', async ({ page }) => {
  await openPanel(page)
  await openSettings(page)
  const open = settingsMenu(page).getByRole('button', { name: 'Open from anywhere' })
  const send = settingsMenu(page).getByRole('button', { name: 'Send copied text' })
  await expect(send).toHaveText('⌃⌥V')

  await send.click()
  await expect(send).toHaveText('Press the new keys')
  await expect(open).not.toHaveClass(/is-recording/)
  await page.keyboard.press('Control+Alt+KeyB')
  await expect(send).toHaveText('⌃⌥B')
  await expect(open).toHaveText('⌃⌥Space')

  await send.click()
  await page.keyboard.press('Control+Alt+Space')
  await expect(settingsMenu(page)).toContainText('Ctrl+Alt+Space already opens the list; choose other keys')
  await expect(send).toHaveText('⌃⌥B')
  const preferences = await (await page.request.get('/api/preferences')).json()
  expect(preferences).toMatchObject({ shortcut: 'Ctrl+Alt+Space', sendShortcut: 'Ctrl+Alt+B' })
})

test('a known update shows a badge, asks in the panel, then shows the first step', async ({ page }) => {
  const reset = await page.request.post('/dev/fixture', { data: { name: 'update-available' } })
  expect(reset.ok()).toBe(true)
  await page.goto('/panel.html')
  const button = page.getByRole('button', { name: 'Settings: update to v0.97.0 available', exact: true })
  await expect(button).toBeVisible()
  await expect(button).toHaveAttribute('data-update', '0.97.0')
  const starts = sent(page, '/api/update/start')

  await button.click()
  const menu = page.getByRole('region', { name: 'Settings', exact: true })
  const entry = menu.getByRole('button', { name: 'Update to v0.97.0', exact: true })
  await expect(menu.locator('.menu-item')).toHaveText(['Update to v0.97.0', 'Hide Agnostic AI companion', 'Quit Agnostic AI companion'])
  await entry.click()

  await expect(menu).toBeHidden()
  const question = page.getByRole('alertdialog', { name: 'Update to v0.97.0 and restart?' })
  await expect(question).toBeVisible()
  await expect(question.getByRole('button', { name: 'Later' })).toBeFocused()
  expect(starts).toEqual([])
  await question.getByRole('button', { name: 'Update and restart' }).click()

  await expect(question).toBeHidden()
  await expect(page.getByRole('region', { name: 'Update' }).getByRole('status')).toHaveText('Downloading v0.97.0…')
  expect(starts).toEqual(['/api/update/start'])
})

test('Later and Escape close the update question without updating', async ({ page }) => {
  const reset = await page.request.post('/dev/fixture', { data: { name: 'update-available' } })
  expect(reset.ok()).toBe(true)
  await page.goto('/panel.html')
  const starts = sent(page, '/api/update/start')
  const question = page.getByRole('alertdialog', { name: 'Update to v0.97.0 and restart?' })
  for (const close of [() => question.getByRole('button', { name: 'Later' }).click(), () => page.keyboard.press('Escape')]) {
    await page.getByRole('button', { name: 'Settings: update to v0.97.0 available', exact: true }).click()
    await page.getByRole('region', { name: 'Settings', exact: true }).getByRole('button', { name: 'Update to v0.97.0', exact: true }).click()
    await expect(question).toBeVisible()
    await close()
    await expect(question).toBeHidden()
  }
  expect(starts).toEqual([])
})

test('a failed update names the reason and the log, and Retry starts it again', async ({ page }) => {
  const reset = await page.request.post('/dev/fixture', { data: { name: 'update-failed' } })
  expect(reset.ok()).toBe(true)
  await page.goto('/panel.html')
  const status = page.getByRole('region', { name: 'Update' })
  await expect(status.getByRole('status')).toHaveText('The update did not finish: the download did not match its checksum.')
  await expect(status).toContainText('Details: /Users/you/Library/Application Support/agnostic-ai/companion-update.log')
  const starts = sent(page, '/api/update/start')

  await status.getByRole('button', { name: 'Retry' }).click()

  await expect(status.getByRole('status')).toHaveText('Downloading v0.97.0…')
  await expect(status.getByRole('button', { name: 'Retry' })).toBeHidden()
  expect(starts).toEqual(['/api/update/start'])
})

test('Dismiss hides the update message until the update changes', async ({ page }) => {
  const reset = await page.request.post('/dev/fixture', { data: { name: 'update-failed' } })
  expect(reset.ok()).toBe(true)
  await page.goto('/panel.html')
  const status = page.getByRole('region', { name: 'Update' })
  await expect(status).toBeVisible()

  await status.getByRole('button', { name: 'Dismiss' }).click()

  await expect(status).toBeHidden()
  await status.page().request.post('/api/update/start')
  await expect(status.getByRole('status')).toHaveText('Downloading v0.97.0…')
})

test('a companion that cannot update itself shows no update entry and no badge', async ({ page }) => {
  await openPanel(page)
  await expect(page.locator('#menu-button')).not.toHaveAttribute('data-update')
  await openSettings(page)
  await expect(page.getByRole('region', { name: 'Settings', exact: true }).getByRole('button', { name: /update/i })).toHaveCount(0)
})

test('a terminal install that cannot update itself copies the command instead', async ({ page, context }) => {
  await context.grantPermissions(['clipboard-read', 'clipboard-write'])
  const reset = await page.request.post('/dev/fixture', { data: { name: 'update-copy' } })
  expect(reset.ok()).toBe(true)
  await page.goto('/panel.html')
  const starts = sent(page, '/api/update/start')

  await page.getByRole('button', { name: 'Settings: update to v0.97.0 available', exact: true }).click()
  await page.getByRole('region', { name: 'Settings', exact: true }).getByRole('button', { name: 'Update to v0.97.0', exact: true }).click()
  const question = page.getByRole('alertdialog', { name: 'Update to v0.97.0', exact: true })
  await expect(question).toContainText('agnostic-ai upgrade')
  await question.getByRole('button', { name: 'Copy command' }).click()

  await expect(page.locator('#toast')).toHaveText('Copied agnostic-ai upgrade. Paste it in a terminal.')
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe('agnostic-ai upgrade')
  expect(starts).toEqual([])
})

test('a failed terminal update offers Retry and the command to copy', async ({ page, context }) => {
  await context.grantPermissions(['clipboard-read', 'clipboard-write'])
  const reset = await page.request.post('/dev/fixture', { data: { name: 'update-failed-terminal' } })
  expect(reset.ok()).toBe(true)
  await page.goto('/panel.html')
  const status = page.getByRole('region', { name: 'Update' })
  await expect(status.getByRole('status')).toHaveText('The update did not finish: the package manager could not run from the companion.')
  await expect(status.getByRole('button', { name: 'Retry' })).toBeVisible()

  await status.getByRole('button', { name: 'Copy command' }).click()

  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe('agnostic-ai upgrade')
})

test('Check for updates automatically is on by default and turns off', async ({ page }) => {
  const reset = await page.request.post('/dev/fixture', { data: { name: 'update-available' } })
  expect(reset.ok()).toBe(true)
  await page.goto('/panel.html')
  const saves = sent(page, '/api/preferences/check-for-updates')
  await page.getByRole('button', { name: 'Settings: update to v0.97.0 available', exact: true }).click()
  const automatic = settingsMenu(page).getByRole('switch', { name: 'Check for updates automatically' })
  await expect(automatic).toHaveAttribute('aria-checked', 'true')

  await automatic.click()

  await expect(automatic).toHaveAttribute('aria-checked', 'false')
  expect(saves).toEqual(['/api/preferences/check-for-updates'])
  await expect(settingsMenu(page).getByRole('button', { name: 'Update to v0.97.0', exact: true })).toBeEnabled()
})

test('an older separate command line tool shows both versions and its upgrade command', async ({ page, context }) => {
  await context.grantPermissions(['clipboard-read', 'clipboard-write'])
  const reset = await page.request.post('/dev/fixture', { data: { name: 'update-older-cli' } })
  expect(reset.ok()).toBe(true)
  await page.goto('/panel.html')
  const status = page.getByRole('region', { name: 'Update' })
  await expect(status.getByRole('status')).toHaveText('The agnostic-ai command at /opt/homebrew/bin/agnostic-ai is v0.95.0, older than this app (v0.96.0). Run this in a terminal to upgrade it.')
  await expect(status.getByRole('button', { name: 'Retry' })).toBeHidden()

  await status.getByRole('button', { name: 'Copy command' }).click()

  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe("'/opt/homebrew/bin/agnostic-ai' upgrade")
})

test('where the companion cannot update itself, the question and a failure open the release page', async ({ page }) => {
  const reset = await page.request.post('/dev/fixture', { data: { name: 'update-copy' } })
  expect(reset.ok()).toBe(true)
  await page.goto('/panel.html')
  const opened = sent(page, '/api/update/release-page')

  await page.getByRole('button', { name: 'Settings: update to v0.97.0 available', exact: true }).click()
  await settingsMenu(page).getByRole('button', { name: 'Update to v0.97.0', exact: true }).click()
  const question = page.getByRole('alertdialog', { name: 'Update to v0.97.0', exact: true })
  await question.getByRole('button', { name: 'Release page' }).click()

  await expect(question).toBeHidden()
  await expect.poll(() => opened).toEqual(['/api/update/release-page'])

  await page.request.post('/dev/fixture', { data: { name: 'update-failed-terminal' } })
  await page.goto('/panel.html')
  await page.getByRole('region', { name: 'Update' }).getByRole('button', { name: 'Release page' }).click()
  await expect.poll(() => opened.length).toBe(2)
})

test('a disk image update question offers no release page', async ({ page }) => {
  const reset = await page.request.post('/dev/fixture', { data: { name: 'update-available' } })
  expect(reset.ok()).toBe(true)
  await page.goto('/panel.html')

  await page.getByRole('button', { name: 'Settings: update to v0.97.0 available', exact: true }).click()
  await settingsMenu(page).getByRole('button', { name: 'Update to v0.97.0', exact: true }).click()

  await expect(page.getByRole('alertdialog').getByRole('button', { name: 'Release page' })).toBeHidden()
})
