import { expect, test, type Page } from '@playwright/test'
import { readFileSync } from 'node:fs'
import { pageHeader } from '../helpers'

const fixture = JSON.parse(readFileSync(new URL('../../src/fixtures/overview.mock.json', import.meta.url), 'utf8'))

async function serve(page: Page) {
  await page.route('**/api/views/overview', (route) => route.fulfill({ json: structuredClone(fixture) }))
  await page.route('**/api/health', (route) => route.fulfill({ json: { ok: true, root: '/tmp/agnostic-ai', global: false, ui: true } }))
  await page.emulateMedia({ reducedMotion: 'reduce' })
}

// The opacity the reader sees: the element's own times every ancestor's.
const opacity = (page: Page, name: string) => page.getByRole('link', { name }).evaluate((element) => {
  let seen = 1
  for (let node: Element | null = element; node; node = node.parentElement) seen *= Number(getComputedStyle(node).opacity)
  return String(seen)
})

test.describe('shared page shell', () => {
  for (const [width, size] of [[1440, '40px'], [390, '30px']] as const) {
    test(`leads with the verdict and keeps the section name as a hidden h1 at ${width}px`, async ({ page }) => {
      await serve(page)
      await page.setViewportSize({ width, height: 900 })
      for (const [view, name] of [['context', 'Context'], ['authority', 'Authority'], ['tools', 'Tools'], ['lifecycle', 'Lifecycle'], ['settings', 'Settings']]) {
        await page.goto(`/?view=${view}`)
        const heading = page.getByRole('heading', { level: 1, name, exact: true })
        await expect(heading).toHaveCount(1)
        await expect(heading).toHaveClass(/\bsr-only\b/)
        const verdict = pageHeader(page, name).locator('.page-verdict')
        await expect(verdict).toBeInViewport()
        expect(await verdict.evaluate((element) => getComputedStyle(element).fontSize)).toBe(size)
        // No visible monospace eyebrow above the verdict.
        expect(await pageHeader(page, name).evaluate((header) => [...header.querySelectorAll('*')].filter((element) => element.getBoundingClientRect().height > 1 && /mono/i.test(getComputedStyle(element).fontFamily) && (element.textContent ?? '').toUpperCase() === (element.textContent ?? '')).length)).toBe(0)
        await expect(page.locator('.page-facts')).not.toContainText('No findings')
      }
    })
  }

  test('ends the facts line with Tool support and Docs, and keeps Docs out of the toolbar', async ({ page }) => {
    await serve(page)
    await page.setViewportSize({ width: 1440, height: 900 })
    await page.goto('/?view=settings')
    const facts = page.locator('.page-facts')
    const support = facts.getByRole('button', { name: 'Tool support', exact: true })
    const docs = facts.getByRole('link', { name: /^Documentation for/ })
    await expect(support).toBeVisible()
    await expect(docs).toBeVisible()
    const factsBox = (await facts.boundingBox())!
    const docsBox = (await docs.boundingBox())!
    expect(docsBox.x + docsBox.width).toBeCloseTo(factsBox.x + factsBox.width, 0)
    await expect(page.locator('.section-search').getByRole('link', { name: /^Documentation for/ })).toHaveCount(0)
    await support.click()
    await expect(page.getByRole('dialog', { name: 'Coding tool support' }).getByRole('table', { name: 'Coding tool support' })).toBeVisible()
  })

  test('puts the Project | Local explanation in the switch tooltip, not on the page', async ({ page }) => {
    await serve(page)
    await page.addInitScript(() => window.localStorage.setItem('agnostic-ai.edit-scope', 'shared'))
    await page.goto('/?view=settings')
    const scope = page.locator('.section-search .edit-scope')
    await expect(scope.getByRole('group', { name: 'View and edit' })).toBeVisible()
    await expect(scope).toHaveAttribute('title', 'Project files, tracked by Git. Local values win for you.')
    await expect(page.getByText('View and edit', { exact: true })).toHaveCount(0)
    await expect(scope.getByRole('status')).toHaveClass(/\bsr-only\b/)
    await scope.getByRole('button', { name: /^Local/ }).click()
    await expect(scope).toHaveAttribute('title', 'Local files in .agnostic-ai/local, ignored by Git. Local values win.')
    await expect(scope.getByRole('status')).toHaveText('Local files in .agnostic-ai/local, ignored by Git. Local values win.')
  })

  test('shows a row\'s secondary actions on hover and on keyboard focus, with the row filled', async ({ page }) => {
    await serve(page)
    await page.setViewportSize({ width: 1440, height: 900 })
    await page.goto('/?view=context&tab=rules')
    const open = 'Open testing in Cursor'
    const row = page.locator('.item-row').filter({ has: page.getByRole('link', { name: open }) })
    const fill = () => row.evaluate((element) => getComputedStyle(element).backgroundColor)
    await page.mouse.move(0, 0)
    const resting = await fill()
    expect(await opacity(page, open)).toBe('0')
    await row.locator('.item-main strong').hover()
    await expect.poll(() => opacity(page, open)).toBe('1')
    expect(await fill()).not.toBe(resting)
    await page.mouse.move(0, 0)
    await expect.poll(() => opacity(page, open)).toBe('0')
    await page.getByRole('link', { name: open }).focus()
    await expect.poll(() => opacity(page, open)).toBe('1')
    expect(await fill()).not.toBe(resting)
  })

  test('puts a row\'s secondary actions behind a ⋯ menu at 390px', async ({ page }) => {
    await serve(page)
    await page.setViewportSize({ width: 390, height: 844 })
    await page.goto('/?view=context&tab=rules')
    const more = page.getByRole('button', { name: 'More actions for testing' })
    const open = page.getByRole('link', { name: 'Open testing in Cursor' })
    await expect(more).toBeVisible()
    await expect(more).toHaveAttribute('aria-expanded', 'false')
    await expect(open).toBeHidden()
    const box = (await more.boundingBox())!
    expect(box.height).toBeGreaterThanOrEqual(44)
    expect(box.width).toBeGreaterThanOrEqual(44)
    await more.click()
    await expect(more).toHaveAttribute('aria-expanded', 'true')
    await expect(open).toBeVisible()
    const menu = (await page.locator(`#${await more.getAttribute('aria-controls')}`).boundingBox())!
    expect(menu.x).toBeGreaterThanOrEqual(0)
    expect(menu.x + menu.width).toBeLessThanOrEqual(390)
    await page.keyboard.press('Escape')
    await expect(open).toBeHidden()
    await expect(more).toBeFocused()
    await more.click()
    await page.locator('.page-verdict').click()
    await expect(open).toBeHidden()
  })

  test('keeps the ⋯ menu of a list\'s last row whole at 390px', async ({ page }) => {
    await serve(page)
    await page.setViewportSize({ width: 390, height: 844 })
    await page.goto('/?view=settings&tab=configuration')
    const more = page.getByRole('button', { name: 'More actions for agnostic-ai.yaml' })
    await more.click()
    const panel = page.locator(`#${await more.getAttribute('aria-controls')}`)
    const copy = panel.getByRole('button', { name: /^Copy path/ })
    await copy.scrollIntoViewIfNeeded()
    // The last action in the panel is the one a clipped list would hide.
    const box = (await copy.boundingBox())!
    expect(await page.evaluate(([x, y]) => document.elementFromPoint(x, y)?.closest('.row-menu-panel') !== null, [box.x + box.width / 2, box.y + box.height - 2])).toBe(true)
  })
})
