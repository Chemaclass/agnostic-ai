import { expect, test } from '@playwright/test'

test('keeps select arrows inset and leaves room for labels in both themes', async ({ page }) => {
  await page.goto('/?mock=1&tab=usage')
  const select = page.getByRole('combobox', { name: 'Coding tool' })
  const arrow = select.locator('..').locator('.select-chevron')
  await expect(select).toBeVisible()
  for (const width of [390, 1280]) {
    await page.setViewportSize({ width, height: 844 })
    for (const theme of ['dark', 'light'] as const) {
      await page.emulateMedia({ colorScheme: theme })
      const controlBox = await select.boundingBox()
      const arrowBox = await arrow.boundingBox()
      expect(controlBox!.x + controlBox!.width - arrowBox!.x - arrowBox!.width).toBeGreaterThanOrEqual(10)
      expect(await select.evaluate((element) => parseFloat(getComputedStyle(element).paddingInlineEnd))).toBeGreaterThanOrEqual(34)
      expect(await arrow.evaluate((element) => getComputedStyle(element).pointerEvents)).toBe('none')
      expect(await arrow.evaluate((element) => getComputedStyle(element).color)).toBe(await page.locator('.context-usage-file-hint').evaluate((element) => getComputedStyle(element).color))
    }
    await select.focus()
    await expect(select).toBeFocused()
  }
})

test('uses the system select arrow in forced colors', async ({ page }) => {
  await page.emulateMedia({ forcedColors: 'active' })
  await page.goto('/?mock=1&tab=usage')
  const select = page.getByRole('combobox', { name: 'Coding tool' })
  await expect(select).toBeVisible()
  await expect(select.locator('..').locator('.select-chevron')).toBeHidden()
  expect(await select.evaluate((element) => getComputedStyle(element).appearance)).toBe('auto')
})
