// Records every visible element's computed style for each view, width and color scheme,
// so a CSS refactor can prove it changed nothing.
//   node scripts/computed-styles.mjs out.json          write a snapshot
//   node scripts/computed-styles.mjs --compare a b     list the differences between two snapshots
import { chromium } from '@playwright/test'
import { spawn } from 'node:child_process'
import { readFileSync, writeFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..')
const views = ['conversations', 'context', 'authority', 'tools', 'lifecycle', 'settings', 'health']
const widths = [1440, 900, 760, 390]
const schemes = ['light', 'dark']
const port = Number(process.env.PORT ?? 5199)

if (process.argv[2] === '--compare') {
  const [a, b] = process.argv.slice(3).map((file) => JSON.parse(readFileSync(file, 'utf8')))
  let diffs = 0
  for (const name of new Set([...Object.keys(a.snapshots), ...Object.keys(b.snapshots)])) {
    const left = new Map(a.snapshots[name] ?? [])
    const right = new Map(b.snapshots[name] ?? [])
    for (const path of new Set([...left.keys(), ...right.keys()])) {
      const x = left.has(path) ? a.styles[left.get(path)] : undefined
      const y = right.has(path) ? b.styles[right.get(path)] : undefined
      if (x === y) continue
      diffs++
      if (diffs <= 40) {
        const props = new Map((x ?? '').split(';').map((p) => p.split(':')))
        const changed = (y ?? '').split(';').map((p) => p.split(':')).filter(([k, v]) => props.get(k) !== v).map(([k, v]) => `${k}: ${props.get(k)} -> ${v}`)
        console.log(`${name} ${path}\n  ${x === undefined ? 'missing before' : y === undefined ? 'missing after' : changed.slice(0, 6).join('\n  ')}`)
      }
    }
  }
  console.log(diffs === 0 ? 'identical' : `${diffs} differences`)
  process.exit(diffs === 0 ? 0 : 1)
}

const out = process.argv[2]
if (!out) throw new Error('usage: node scripts/computed-styles.mjs <out.json>')

const server = spawn('npx', ['vite', '--port', String(port), '--strictPort', '--host', '127.0.0.1'], { cwd: root, stdio: 'ignore' })
const base = `http://127.0.0.1:${port}/`

const collect = () => {
  const names = (style) => Array.from({ length: style.length }, (_, i) => style[i])
  const describe = (style) => names(style).sort().map((n) => `${n}:${style.getPropertyValue(n)}`).join(';')
  const pathOf = (el) => {
    const parts = []
    for (let node = el; node && node !== document.documentElement; node = node.parentElement) {
      const index = node.parentElement ? Array.prototype.indexOf.call(node.parentElement.children, node) : 0
      parts.unshift(`${node.tagName.toLowerCase()}[${index}]`)
    }
    return parts.join('>')
  }
  const rows = []
  for (const el of [document.documentElement, ...document.body.querySelectorAll('*')]) {
    if (['SCRIPT', 'STYLE', 'LINK', 'META', 'TITLE'].includes(el.tagName)) continue
    const rect = el.getBoundingClientRect()
    if (rect.width === 0 && rect.height === 0) continue
    const path = el === document.documentElement ? 'html' : pathOf(el)
    rows.push([path, describe(getComputedStyle(el))])
    for (const pseudo of ['::before', '::after']) {
      const style = getComputedStyle(el, pseudo)
      if (style.content !== 'none' && style.content !== 'normal') rows.push([`${path}${pseudo}`, describe(style)])
    }
  }
  return rows
}

const styles = []
const ids = new Map()
const snapshots = {}
const record = async (page, name) => {
  await page.evaluate(() => document.fonts.ready)
  await page.waitForTimeout(500)
  snapshots[name] = (await page.evaluate(collect)).map(([path, style]) => {
    if (!ids.has(style)) ids.set(style, styles.push(style) - 1)
    return [path, ids.get(style)]
  })
}

let browser
try {
  for (let i = 0; ; i++) {
    if (server.exitCode !== null) throw new Error('dev server exited')
    try {
      if ((await fetch(base)).ok) break
    } catch {}
    if (i > 100) throw new Error('dev server did not start')
    await new Promise((r) => setTimeout(r, 300))
  }
  browser = await chromium.launch({ executablePath: process.env.E2E_CHROMIUM_PATH || undefined })
  for (const colorScheme of schemes) {
    for (const width of widths) {
      const context = await browser.newContext({ viewport: { width, height: 900 }, colorScheme })
      const page = await context.newPage()
      for (const view of views) {
        await page.goto(`${base}?view=${view}&mock=1`)
        await page.getByRole('heading', { level: 1 }).first().waitFor()
        await record(page, `${view} ${width} ${colorScheme}`)
      }
      await page.goto(`${base}?view=settings&mock=1`)
      await page.getByRole('heading', { level: 1 }).first().waitFor()
      await page.keyboard.press(process.platform === 'darwin' ? 'Meta+K' : 'Control+K')
      await page.getByRole('dialog').first().waitFor()
      await record(page, `search ${width} ${colorScheme}`)
      await page.keyboard.press('Escape')
      await page.getByRole('button', { name: 'Overview JSON', exact: true }).click()
      await page.getByRole('dialog', { name: 'JSON inspector' }).waitFor()
      await record(page, `inspector ${width} ${colorScheme}`)
      await context.close()
    }
  }
} finally {
  await browser?.close()
  server.kill()
}

writeFileSync(out, JSON.stringify({ snapshots, styles }))
const names = Object.keys(snapshots)
const elements = names.reduce((sum, name) => sum + snapshots[name].length, 0)
console.log(`${names.length} snapshots, ${elements} elements, ${styles.length} distinct styles`)
