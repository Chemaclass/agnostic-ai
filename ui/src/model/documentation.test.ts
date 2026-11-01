import { existsSync, readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'
import { DOC_FALLBACK, DOC_TOPICS, documentationUrl } from './documentation'

const content = fileURLToPath(new URL('../../../docs/site/content', import.meta.url))
const EXTRA_PATHS = ['/docs/cli-reference/inspect/#serve']

function pageFile(path: string): string {
  const clean = path.replace(/#.*$/, '').replace(/\/$/, '')
  const asPage = `${content}${clean}.md`
  return existsSync(asPage) ? asPage : `${content}${clean}/_index.md`
}

/** Zola's anchor for a heading: lowercase, non-alphanumerics to one hyphen, unless the heading sets {#id}. */
function anchors(markdown: string): Set<string> {
  const ids = new Set<string>()
  let fenced = false
  for (const line of markdown.split('\n')) {
    if (line.startsWith('```')) fenced = !fenced
    if (fenced) continue
    const heading = /^#{1,6}\s+(.*?)\s*$/.exec(line)
    if (!heading) continue
    const explicit = /\{#([a-z0-9-]+)[^}]*\}\s*$/.exec(heading[1])
    if (explicit) { ids.add(explicit[1]); continue }
    ids.add(heading[1].toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, ''))
  }
  return ids
}

describe('documentation links', () => {
  const links = [...Object.entries(DOC_TOPICS).map(([topic, doc]) => [topic, doc.path] as const), ['fallback', DOC_FALLBACK.path] as const, ...EXTRA_PATHS.map((path) => ['literal', path] as const)]

  it.each(links)('%s points at an existing page and anchor: %s', (_topic, path) => {
    expect(path).toMatch(/^\/docs\/.*\/(#[a-z0-9-]+)?$/)
    const file = pageFile(path)
    expect(existsSync(file), `${path} has no page under docs/site/content`).toBe(true)
    const hash = path.split('#')[1]
    if (hash) expect(anchors(readFileSync(file, 'utf8')), `${path}: anchor #${hash} not found in ${file}`).toContain(hash)
  })

  it('labels each topic for the Docs link sentence', () => {
    for (const [topic, doc] of Object.entries(DOC_TOPICS)) {
      expect(doc.label, topic).toMatch(/^[a-zA-Z][a-zA-Z -]+$/)
      expect(doc.label).not.toMatch(/[–—]/)
    }
    expect(documentationUrl('/docs/spec-format/rules/')).toBe('https://agnostic-ai.org/docs/spec-format/rules/')
  })
})
