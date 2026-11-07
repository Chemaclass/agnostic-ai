import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'

const design = readFileSync(new URL('../../DESIGN.md', import.meta.url), 'utf8')
const tokens = readFileSync(new URL('./tokens.css', import.meta.url), 'utf8')

function frontmatterColors(markdown: string): Record<string, string> {
  const front = markdown.match(/^---\n([\s\S]*?)\n---/)?.[1] ?? ''
  const block = front.match(/^colors:\n((?: {2}.+\n?)+)/m)?.[1] ?? ''
  const colors: Record<string, string> = {}
  for (const line of block.split('\n')) {
    const pair = line.match(/^ {2}([a-z-]+):\s*"([^"]+)"/)
    if (pair) colors[pair[1]] = pair[2]
  }
  return colors
}

function cssBlock(css: string, selector: string): Record<string, string> {
  const start = css.indexOf(`${selector} {`)
  if (start < 0) throw new Error(`no ${selector} block in tokens.css`)
  const body = css.slice(start, css.indexOf('}', start))
  return Object.fromEntries([...body.matchAll(/(--[\w-]+):\s*([^;]+);/g)].map((match) => [match[1], match[2].trim()]))
}

function luminance(hex: string): number {
  const value = hex.replace('#', '')
  const channels = [0, 2, 4].map((offset) => parseInt(value.slice(offset, offset + 2), 16) / 255)
  const [r, g, b] = channels.map((c) => (c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4))
  return 0.2126 * r + 0.7152 * g + 0.0722 * b
}

function contrast(a: string, b: string): number {
  const [light, dark] = [luminance(a), luminance(b)].sort((x, y) => y - x)
  return (light + 0.05) / (dark + 0.05)
}

const colors = frontmatterColors(design)
const color = (name: string) => {
  const value = colors[name]
  if (!value) throw new Error(`DESIGN.md has no color ${name}`)
  return value
}

const THEMES = ['light', 'dark'] as const
const TEXT = ['ink', 'soft', 'muted', 'accent', 'accent-strong', 'permitted', 'approval', 'blocked', 'info']
const BACKGROUNDS = ['canvas', 'surface', 'sunken']
const STATES = ['permitted', 'approval', 'blocked', 'info']

describe('Mineral colors in DESIGN.md', () => {
  it('reads every color from the frontmatter', () => {
    expect(Object.keys(colors).length).toBeGreaterThan(40)
    for (const value of Object.values(colors)) expect(value).toMatch(/^(#[0-9a-f]{6}|rgba\(.+\))$/)
  })

  for (const theme of THEMES) {
    it(`gives every ${theme} text token 4.5:1 on canvas, surface and sunken`, () => {
      for (const text of TEXT) {
        for (const background of BACKGROUNDS) {
          const ratio = contrast(color(`${theme}-${text}`), color(`${theme}-${background}`))
          expect(ratio, `${theme}-${text} on ${theme}-${background}`).toBeGreaterThanOrEqual(4.5)
        }
      }
    })

    it(`gives the ${theme} control token 3:1 on canvas, surface and field`, () => {
      for (const background of ['canvas', 'surface', 'field']) {
        const ratio = contrast(color(`${theme}-control`), color(`${theme}-${background}`))
        expect(ratio, `${theme}-control on ${theme}-${background}`).toBeGreaterThanOrEqual(3)
      }
    })

    it(`gives the ${theme} focus ring 3:1 on canvas`, () => {
      expect(contrast(color(`${theme}-focus`), color(`${theme}-canvas`))).toBeGreaterThanOrEqual(3)
    })

    it(`gives rail text and dark state colors 4.5:1 on the ${theme} rail`, () => {
      const suffix = theme === 'dark' ? '-dark' : ''
      for (const background of [`rail${suffix}`, `rail-raised${suffix}`]) {
        for (const text of ['rail-ink', 'rail-dim', ...STATES.map((state) => `dark-${state}`)]) {
          expect(contrast(color(text), color(background)), `${text} on ${background}`).toBeGreaterThanOrEqual(4.5)
        }
      }
    })
  }
})

describe('tokens.css', () => {
  const NAMES: Record<string, string> = {
    canvas: '--canvas',
    surface: '--panel',
    sunken: '--panel-sunken',
    field: '--field',
    line: '--line',
    control: '--line-strong',
    ink: '--text',
    soft: '--text-soft',
    muted: '--text-muted',
    faint: '--text-faint',
    accent: '--accent',
    'accent-strong': '--accent-strong',
    'accent-wash': '--accent-wash',
    focus: '--focus',
    permitted: '--permitted',
    approval: '--approval',
    blocked: '--blocked',
    info: '--info',
  }
  const blocks = { dark: cssBlock(tokens, ':root'), light: cssBlock(tokens, "[data-theme='light']") }

  for (const theme of THEMES) {
    it(`uses the ${theme} DESIGN.md colors`, () => {
      for (const [design, property] of Object.entries(NAMES)) {
        expect(blocks[theme][property], `${property} in ${theme}`).toBe(color(`${theme}-${design}`))
      }
      const suffix = theme === 'dark' ? '-dark' : ''
      expect(blocks[theme]['--rail']).toBe(color(`rail${suffix}`))
      expect(blocks[theme]['--rail-raised']).toBe(color(`rail-raised${suffix}`))
    })
  }

  it('uses the shared rail inks and backdrop', () => {
    expect(blocks.dark['--rail-ink']).toBe(color('rail-ink'))
    expect(blocks.dark['--rail-dim']).toBe(color('rail-dim'))
    expect(blocks.dark['--backdrop']).toBe(color('backdrop'))
  })

  it('gives the rail and the Menu sheet the dark state colors and the same inks in both themes', () => {
    const rail = cssBlock(tokens, '.rail')
    const sheet = cssBlock(tokens, '.menu-sheet')
    for (const state of STATES) expect(rail[`--${state}`], `--${state} on .rail`).toBe(color(`dark-${state}`))
    expect(Object.keys(rail).length).toBeGreaterThan(10)
    expect(sheet).toEqual(rail)
  })
})
