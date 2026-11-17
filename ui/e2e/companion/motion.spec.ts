import { expect, test } from '@playwright/test'

test.beforeEach(async ({ page }) => {
  await page.request.post('/dev/fixture', { data: { name: 'mixed' } })
  await page.emulateMedia({ reducedMotion: 'no-preference' })
  await page.goto('/panel.html')
  await expect(page.locator('.state.is-working')).toBeVisible()
})

test('working moves without changing opacity and is named once in its blue section header', async ({ page }) => {
  const mark = page.locator('.state.is-working')
  await expect(page.locator('.group-title.is-working')).toContainText('Working')
  await expect(page.locator('.group-title.is-working')).toHaveCSS('color', 'rgb(155, 189, 206)')
  await expect(page.locator('.why.is-working')).toHaveCount(0)
  const initial = await mark.locator('.work-traveller').first().evaluate((element) => getComputedStyle(element).transform)
  await expect.poll(() => mark.locator('.work-traveller').first().evaluate((element) => getComputedStyle(element).transform)).not.toBe(initial)
  const opacity = await mark.evaluate((element) => {
    const animations = element.getAnimations({ subtree: true })
    const samples: string[][] = []
    for (const time of [0, 700, 1400, 2800, 4800]) {
      for (const animation of animations) {
        animation.pause()
        animation.currentTime = time
      }
      samples.push([element, ...element.querySelectorAll('*')].map((part) => getComputedStyle(part).opacity))
    }
    return samples
  })
  for (const sample of opacity) expect(sample).toEqual(opacity[0])
})

test('both motion controls leave a still, readable working state', async ({ page }) => {
  const mark = page.locator('.state.is-working')
  await page.emulateMedia({ reducedMotion: 'reduce' })
  await expect.poll(() => mark.evaluate((element) => element.getAnimations({ subtree: true }).length)).toBe(0)
  await expect(page.locator('.group-title.is-working')).toContainText('Working')
  await expect(mark).toBeVisible()

  await page.emulateMedia({ reducedMotion: 'no-preference' })
  await expect.poll(() => mark.evaluate((element) => element.getAnimations({ subtree: true }).length)).toBeGreaterThan(0)
  await page.getByRole('button', { name: 'Settings', exact: true }).click()
  await page.getByRole('switch', { name: 'Animate companion' }).click()
  await expect(page.locator('html')).toHaveAttribute('data-motion', 'off')
  await expect.poll(() => mark.evaluate((element) => element.getAnimations({ subtree: true }).length)).toBe(0)
  await expect(page.locator('.group-title.is-working')).toContainText('Working')
  await expect(mark).toBeVisible()
})

test('redrawing a working row keeps its orbital phase', async ({ page }) => {
  const drift = async () => page.locator('.work-cage').evaluate((element) => {
    const animation = element.getAnimations()[0]
    const delay = animation.effect?.getTiming().delay
    if (delay === undefined) throw new Error('the work cage animation has no timing delay to compare')
    return Number(animation.currentTime) - delay - performance.now()
  })
  // Wait until the animation has a start time before comparing its timeline.
  await expect.poll(() => page.locator('.work-cage').evaluate((element) => Number(element.getAnimations()[0].currentTime))).toBeGreaterThan(100)
  const before = await drift()
  const oldCage = await page.locator('.work-cage').elementHandle()
  // A new arrival replaces every row, so the working row's mark is drawn again.
  await page.request.post('/dev/arrive')
  await expect.poll(() => oldCage?.evaluate((element) => element.isConnected)).toBe(false)
  await expect(page.locator('.work-cage')).toHaveCount(1)
  await expect.poll(() => page.locator('.work-cage').evaluate((element) => Number(element.getAnimations()[0].currentTime))).toBeGreaterThan(100)
  expect(Math.abs(await drift() - before)).toBeLessThan(100)
})

test('adds one miniature fractal per native working sub-agent, with five plain base dots in each system', async ({ page }) => {
  await page.request.post('/dev/fixture', { data: { name: 'workers' } })
  const working = page.getByRole('option', { name: /Build the companion/ })
  await expect(working.locator('.state > .work-system .work-traveller')).toHaveCount(5)
  await expect(working.locator('.work-worker')).toHaveCount(3)
  await expect(working.locator('.work-worker .work-system')).toHaveCount(3)
  await expect(working.locator('.work-worker .work-traveller')).toHaveCount(15)
  await expect(working.locator('.work-worker .work-worker')).toHaveCount(0)
  await expect(working.locator('.work-fractal')).toHaveCount(0)
  await expect(working.locator('.worker-count')).toHaveText('3')
  await expect(working.locator('.worker-count svg')).toHaveCount(1)
  await expect(working.locator('.worker-count')).toHaveAttribute('title', '3 working sub-agents')
  await expect(working.getByRole('img', { name: '3 working sub-agents', exact: true })).toHaveCount(1)
  const waiting = page.getByRole('option', { name: /Review the implementation/ })
  await expect(waiting.locator('.work-worker')).toHaveCount(2)
  await expect(waiting.locator('.why')).toHaveText('Waiting for you')
  const unknown = page.getByRole('option', { name: /Activity not available/ })
  await expect(unknown.locator('.work-worker')).toHaveCount(0)
  await expect(unknown.locator('.worker-count')).toHaveCount(0)
  await page.emulateMedia({ reducedMotion: 'reduce' })
  await expect.poll(() => working.locator('.state').evaluate((element) => element.getAnimations({ subtree: true }).length)).toBe(0)
  await expect(working.locator('.work-worker')).toHaveCount(3)
})

test('waiting and idle parents keep ordinary still dots while workers retain their own working appearance', async ({ page }) => {
  const plainWaiting = await page.locator('.state.is-blocked').evaluate((element) => {
    const style = getComputedStyle(element)
    return { color: style.backgroundColor, width: style.width, shadow: style.boxShadow }
  })
  await page.request.post('/dev/fixture', { data: { name: 'workers' } })
  const working = page.getByRole('option', { name: /Build the companion/ })
  const waiting = page.getByRole('option', { name: /Review the implementation/ })
  const core = waiting.locator('.state')
  await expect(waiting.locator('.work-worker')).toHaveCount(2)
  await expect(core).toHaveClass('state is-blocked')
  await expect(core).toHaveCSS('background-color', plainWaiting.color)
  await expect(core).toHaveCSS('width', plainWaiting.width)
  await expect(core).toHaveCSS('box-shadow', plainWaiting.shadow)
  await expect(core.locator(':scope > .work-system')).toHaveCount(0)
  expect(await core.evaluate((element) => element.getAnimations().length)).toBe(0)

  const workingCore = await working.locator('.state > .work-system').evaluate((element) => getComputedStyle(element, '::before').backgroundImage)
  const satellite = waiting.locator('.work-worker .work-system').first()
  expect(await satellite.evaluate((element) => getComputedStyle(element, '::before').backgroundImage)).toBe(workingCore)
  await expect.poll(() => satellite.evaluate((element) => element.getAnimations({ subtree: true }).length)).toBeGreaterThan(0)

  for (const [name, state] of [
    ['Idle parent with a worker', 'active'],
    ['Codex parent after its turn', 'active'],
    ['Waiting parent already read', 'blocked'],
  ]) {
    const row = page.getByRole('option', { name: new RegExp(name) })
    await expect(row.locator('.state')).toHaveClass(`state is-${state}`)
    await expect(row.locator('.state > .work-system')).toHaveCount(0)
    await expect(row.locator('.work-worker')).toHaveCount(1)
  }
  await page.emulateMedia({ reducedMotion: 'reduce' })
  await expect.poll(() => core.evaluate((element) => element.getAnimations({ subtree: true }).length)).toBe(0)
  await expect(waiting.locator('.work-worker')).toHaveCount(2)
})

/** Sends the copied text to the first row and says whether a chip flew and whether the banner slid in. */
async function sendAndWatch(page: import('@playwright/test').Page) {
  await page.evaluate(() => {
    const seen = { chip: false }
    ;(window as unknown as { seen: typeof seen }).seen = seen
    new MutationObserver(() => {
      if (document.querySelector('.pick-chip')) seen.chip = true
    }).observe(document.getElementById('card')!, { childList: true })
  })
  await page.keyboard.press('Meta+Shift+KeyV')
  const banner = page.getByRole('region', { name: 'Send the copied text' })
  await expect(banner).toBeVisible()
  const slid = await banner.evaluate((element) => element.classList.contains('is-entering'))
  const sent = page.waitForRequest((request) => request.url().endsWith('/api/send'))
  await page.keyboard.press('Enter')
  await sent
  await expect(banner).toBeHidden()
  const chip = await page.evaluate(() => (window as unknown as { seen: { chip: boolean } }).seen.chip)
  return { chip, slid }
}

/** Presses the send shortcut on one typable row and says whether the banner named the row and a chip flew. */
async function sendStraightAndWatch(page: import('@playwright/test').Page) {
  await page.request.post('/dev/fixture', { data: { name: 'one-terminal' } })
  await page.goto('/panel.html')
  await expect(page.locator('#list > .row').first()).toBeVisible()
  await page.evaluate(() => {
    const seen = { chip: false, named: false }
    ;(window as unknown as { seen: typeof seen }).seen = seen
    new MutationObserver(() => {
      if (document.querySelector('.pick-chip')) seen.chip = true
      if (document.getElementById('pick-title')?.textContent === 'Sending to Plan the sync dry run') seen.named = true
    }).observe(document.getElementById('card')!, { childList: true, subtree: true, characterData: true })
  })
  const sent = page.waitForRequest((request) => request.url().endsWith('/api/send'))
  await page.request.post('/dev/send-shortcut')
  expect((await sent).postDataJSON()).toMatchObject({ tool: 'claude', id: 'active-2' })
  await expect(page.getByRole('region', { name: 'Send the copied text' })).toBeHidden()
  return page.evaluate(() => (window as unknown as { seen: { chip: boolean; named: boolean } }).seen)
}

test('a straight send names the row and flies the chip into it, and with reduced motion it sends at once', async ({ page }) => {
  expect(await sendStraightAndWatch(page)).toEqual({ chip: true, named: true })

  await page.emulateMedia({ reducedMotion: 'reduce' })
  expect((await sendStraightAndWatch(page)).chip).toBe(false)
})

test('sending flies a chip into the picked row, and with animation off nothing moves', async ({ page }) => {
  expect(await sendAndWatch(page)).toEqual({ chip: true, slid: true })
  await expect(page.locator('.pick-chip')).toHaveCount(0)

  await page.goto('/panel.html')
  await page.emulateMedia({ reducedMotion: 'reduce' })
  await expect(page.locator('.state.is-working')).toBeVisible()
  expect(await sendAndWatch(page)).toEqual({ chip: false, slid: false })
})
