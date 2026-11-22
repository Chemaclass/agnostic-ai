import { expect, test, type Page } from '@playwright/test'

// The bubble window is see-through. Anything the page paints on its edge,
// such as a shadow cut by the window bounds, shows as a square around the circle.

async function strongestEdgeAlpha(page: Page) {
  const png = await page.screenshot({ omitBackground: true })
  return page.evaluate(async (src) => {
    const image = new Image()
    image.src = src
    await image.decode()
    const canvas = document.createElement('canvas')
    canvas.width = image.width
    canvas.height = image.height
    const context = canvas.getContext('2d')!
    context.drawImage(image, 0, 0)
    const { data } = context.getImageData(0, 0, canvas.width, canvas.height)
    let strongest = 0
    for (let y = 0; y < canvas.height; y++) {
      for (let x = 0; x < canvas.width; x++) {
        const onEdge = x === 0 || y === 0 || x === canvas.width - 1 || y === canvas.height - 1
        if (onEdge) strongest = Math.max(strongest, data[(y * canvas.width + x) * 4 + 3])
      }
    }
    return strongest
  }, `data:image/png;base64,${png.toString('base64')}`)
}

async function expectBadgeInside(page: Page, side: number) {
  const box = await page.locator('#badge').boundingBox()
  expect(box).not.toBeNull()
  expect(box!.x).toBeGreaterThanOrEqual(0)
  expect(box!.y).toBeGreaterThanOrEqual(0)
  expect(box!.x + box!.width).toBeLessThanOrEqual(side)
  expect(box!.y + box!.height).toBeLessThanOrEqual(side)
}

for (const side of [92, 64]) {
  test(`the ${side}-point bubble window paints nothing on its edge, at rest and under the pointer`, async ({ page }) => {
    await page.request.post('/dev/fixture', { data: { name: 'mixed' } })
    await page.setViewportSize({ width: side, height: side })
    await page.goto('/')
    await expect(page.locator('#badge.is-blocked')).toBeVisible()

    await expectBadgeInside(page, side)
    expect(await strongestEdgeAlpha(page)).toBe(0)

    await page.locator('#bubble').hover()
    await expect(page.locator('#bubble')).toHaveCSS('transform', 'matrix(1.06, 0, 0, 1.06, 0, 0)')

    await expectBadgeInside(page, side)
    expect(await strongestEdgeAlpha(page)).toBe(0)
  })
}

test('a kept shell leaves the circle count until its wake time', async ({ page }) => {
  await page.request.post('/dev/fixture', { data: { name: 'stuck-shells' } })
  await page.goto('/')
  await expect(page.locator('#badge.is-blocked')).toBeVisible()
  await expect(page.locator('#count')).toHaveText('1')

  const kept = await page.request.post('/api/kept-shells/keep', {
    data: { tool: 'claude', id: 'shell-blocked', pid: 531, procStart: '2026-11-07T00:30:00.000Z', wakeAt: '2999-01-01T00:00:00.000Z', keeps: 1 },
  })
  expect(kept.ok()).toBe(true)
  await expect(page.locator('#badge')).toBeHidden()
  await expect(page.locator('#bubble')).toHaveAttribute('aria-label', 'Conversations: none needs you')
})

test('the badge shows a terminal for a shell wait and a lock for a permission, with the count of those rows', async ({ page }) => {
  await page.request.post('/dev/fixture', { data: { name: 'stuck-shells' } })
  await page.goto('/')
  await expect(page.locator('#badge.is-blocked.is-terminal')).toBeVisible()
  await expect(page.locator('#badge .terminal')).toBeVisible()
  await expect(page.locator('#badge .lock')).toBeHidden()
  await expect(page.locator('#count')).toHaveText('1')

  await page.request.post('/dev/fixture', { data: { name: 'auto-approvals' } })
  await page.goto('/')
  await expect(page.locator('#badge .lock')).toBeVisible()
  await expect(page.locator('#badge .terminal')).toBeHidden()
  await expect(page.locator('#count')).toHaveText('2')
})

test('a click on the circle asks for the first waiting row only while one waits', async ({ page }) => {
  await page.request.post('/dev/fixture', { data: { name: 'stuck-shells' } })
  await page.goto('/')
  await expect(page.locator('#count')).toHaveText('1')
  const asked = page.waitForRequest((request) => request.url().endsWith('/api/panel/toggle'))
  await page.locator('#bubble').click()
  expect((await asked).postDataJSON()).toEqual({ jump: true })

  await page.request.post('/dev/fixture', { data: { name: 'empty' } })
  await page.goto('/')
  await expect(page.locator('#badge')).toBeHidden()
  const quiet = page.waitForRequest((request) => request.url().endsWith('/api/panel/toggle'))
  await page.locator('#bubble').click()
  expect((await quiet).postDataJSON()).toEqual({ jump: false })
})

test('a path conflict counts both its rows on the circle, with a lock', async ({ page }) => {
  await page.request.post('/dev/fixture', { data: { name: 'mailbox-claim-conflict' } })
  await page.goto('/')
  await expect(page.locator('#badge.is-blocked')).toBeVisible()
  await expect(page.locator('#badge .lock')).toBeVisible()
  await expect(page.locator('#count')).toHaveText('2')

  await page.request.post('/api/mailbox/conflict/dismiss', { data: { tool: 'claude', id: '84fc6d21-820c-4c88-9729-ff8ee40de1f3', worktree: '/Users/you/Code/agnostic-ai', path: 'internal/mailbox/handoff.go' } })
  await page.goto('/')
  await expect(page.locator('#badge .lock')).toBeHidden()
})
