import { expect, test, type Page } from '@playwright/test'
import { openApp, saveScreenshot, waitForFeed } from './helpers'

const current = (page: Page) => page.locator('.feed-card[aria-current="true"]')

async function cardTitle(page: Page, index: number): Promise<string> {
  return (await page.locator('.feed-card .card-title').nth(index).innerText()).trim()
}

test.describe('feed', () => {
  test('ranks the cards and highlights one at a time', async ({ page }) => {
    await openApp(page)
    await expect(page.getByRole('heading', { level: 1, name: 'Feed' })).toBeVisible()
    const count = await waitForFeed(page)
    if (count === 0) {
      await expect(page.getByText(/Nothing to decide\. Every check is clean at revision/)).toBeVisible()
      await saveScreenshot(page, '00-feed.png')
      test.skip(true, 'The feed is empty for this project; the empty state was checked instead.')
    }

    await expect(current(page)).toHaveCount(1)
    await expect(page.getByRole('link', { name: /^Feed/ })).toHaveAttribute('aria-current', 'page')

    // Most consequential first: priorities never decrease down the list.
    const priorities = (await page.locator('.feed-item').evaluateAll((items) => items.map((i) => Number((i as HTMLElement).dataset.priority)))) as number[]
    expect(priorities).toEqual([...priorities].sort((a, b) => a - b))

    // The highlighted card shows everything a decision needs.
    const card = current(page)
    await expect(card.locator('.card-meta .badge')).toBeVisible()
    await expect(card.locator('.card-kind')).toHaveText(/Decision|Insight/)
    await expect(card.locator('.card-title')).not.toBeEmpty()
    await expect(card.locator('.card-consequence')).not.toBeEmpty()
    await expect(card.locator('.card-evidence code').first()).toBeVisible()
    await expect(card.locator('.card-actions button').first()).toBeVisible()
    await expect(card.getByText('Dismiss and snooze are kept in this browser.')).toBeVisible()
    // The revision never appears in the feed body, only in the footer.
    const revision = (await page.locator('.footer code').first().innerText()).trim()
    await expect(page.locator('main')).not.toContainText(revision)

    await saveScreenshot(page, '00-feed.png')
  })

  test('moves with j and k, dismisses with n, and keeps the dismissal in this browser', async ({ page }) => {
    await openApp(page)
    const count = await waitForFeed(page)
    test.skip(count < 2, 'Keyboard navigation needs at least two cards.')

    const first = await cardTitle(page, 0)
    const second = await cardTitle(page, 1)
    await expect(current(page).locator('.card-title')).toHaveText(first)

    await page.keyboard.press('j')
    await expect(current(page).locator('.card-title')).toHaveText(second)
    await expect(current(page)).toBeFocused()
    await page.keyboard.press('k')
    await expect(current(page).locator('.card-title')).toHaveText(first)

    await page.keyboard.press('n')
    await expect(page.locator('.feed-card .card-title', { hasText: first })).toHaveCount(0)
    await expect(page.locator('.feed-status')).toContainText(`Dismissed “${first}”. Kept in this browser.`)
    await expect(current(page).locator('.card-title')).toHaveText(second)

    const stored = await page.evaluate(() => window.localStorage.getItem('agnostic-ai.feed.v1'))
    expect(stored).toContain(first)

    // The dismissal survives a reload.
    await page.reload()
    await waitForFeed(page)
    await expect(page.locator('.feed-card .card-title', { hasText: first })).toHaveCount(0)

    const toggle = page.getByRole('button', { name: /Show dismissed/ })
    await expect(toggle).toHaveAttribute('aria-pressed', 'false')
    await toggle.click()
    await expect(toggle).toHaveAttribute('aria-pressed', 'true')
    const hidden = page.locator('.hidden-item', { hasText: first })
    await expect(hidden).toContainText('Dismissed')
    await hidden.getByRole('button', { name: 'Restore' }).click()
    await expect(page.locator('.feed-card .card-title', { hasText: first })).toHaveCount(1)
  })

  test('accepts the first action with y', async ({ page, context }) => {
    await context.grantPermissions(['clipboard-read', 'clipboard-write'])
    await openApp(page)
    const count = await waitForFeed(page)
    test.skip(count === 0, 'The feed is empty.')

    const kind = await page.locator('.feed-item').first().getAttribute('data-first-action')
    const title = await cardTitle(page, 0)
    await page.keyboard.press('y')
    if (kind === 'open' || kind === 'propose') {
      await expect(page).toHaveURL(/view=(?!feed)/)
      await expect(page.getByRole('heading', { level: 1 })).not.toHaveText('Feed')
      if (kind === 'propose') {
        await expect(page.getByRole('heading', { name: 'Proposal' })).toBeVisible()
        await expect(page.locator('.proposal-head .badge')).toHaveText('Proposed')
      }
      // Coming back, the card is listed after the pending ones as acted on.
      await page.goBack()
      await waitForFeed(page)
      await expect(page.locator('.feed-card', { hasText: title }).locator('.card-acted')).toHaveText('Acted on in this session')
    } else if (kind === 'copy-command') {
      await expect(page.locator('.feed-status')).toContainText(/Copied:|Copy failed/)
    } else {
      await expect(page.locator('.feed-card .card-title', { hasText: title })).toHaveCount(0)
    }
  })

  test('a touch swipe left dismisses the highlighted card', async ({ page }) => {
    await openApp(page)
    const count = await waitForFeed(page)
    test.skip(count === 0, 'The feed is empty.')
    const title = await cardTitle(page, 0)
    const card = current(page)
    const box = (await card.boundingBox())!
    const y = box.y + 40
    const init = { pointerType: 'touch', isPrimary: true, pointerId: 7, bubbles: true }
    await card.dispatchEvent('pointerdown', { ...init, clientX: box.x + box.width / 2, clientY: y })
    await card.dispatchEvent('pointermove', { ...init, clientX: box.x + box.width / 2 - 60, clientY: y })
    await card.dispatchEvent('pointermove', { ...init, clientX: box.x + box.width / 2 - 160, clientY: y })
    await card.dispatchEvent('pointerup', { ...init, clientX: box.x + box.width / 2 - 160, clientY: y })
    await expect(page.locator('.feed-card .card-title', { hasText: title })).toHaveCount(0)
    await expect(page.locator('.feed-status')).toContainText('Kept in this browser.')
  })

  test('commits a decision without animation under reduced motion', async ({ page }) => {
    await page.emulateMedia({ reducedMotion: 'reduce' })
    await openApp(page)
    const count = await waitForFeed(page)
    test.skip(count === 0, 'The feed is empty.')
    const animation = await current(page).evaluate((el) => getComputedStyle(el).animationName)
    expect(animation).toBe('none')
    const title = await cardTitle(page, 0)
    await page.keyboard.press('n')
    // No slide-out class is ever applied; the card leaves at once.
    await expect(page.locator('.leaving-left, .leaving-right')).toHaveCount(0)
    await expect(page.locator('.feed-card .card-title', { hasText: title })).toHaveCount(0, { timeout: 150 })
  })

  test('fits a phone screen', async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 })
    await openApp(page)
    await waitForFeed(page)
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth)
    expect(overflow).toBeLessThanOrEqual(0)
    await expect(page.getByRole('navigation', { name: 'Harness' })).toBeVisible()
    await saveScreenshot(page, '08-mobile-feed.png')
  })

  test('has no horizontal page scroll at 360 pixels wide', async ({ page }) => {
    await page.setViewportSize({ width: 360, height: 740 })
    for (const query of ['', '?view=authority', '?view=authority&domain=cloud-deployment', '?view=control', '?view=inspect']) {
      await openApp(page, query)
      await expect(page.locator('main h1')).toBeVisible()
      await page.waitForLoadState('networkidle')
      const overflow = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth)
      expect(overflow, `horizontal overflow on "${query || 'feed'}"`).toBeLessThanOrEqual(0)
    }
  })
})
