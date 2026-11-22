import { test } from 'node:test'
import assert from 'node:assert/strict'
import { existsSync, readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { HELP_TOPICS } from '../frontend/model.js'

const content = fileURLToPath(new URL('../../docs/site/content', import.meta.url))

function pageFile(path) {
  const clean = path.replace(/#.*$/, '').replace(/\/$/, '')
  const asPage = `${content}${clean}.md`
  return existsSync(asPage) ? asPage : `${content}${clean}/_index.md`
}

/** Zola's anchor for a heading: lowercase, non-alphanumerics to one hyphen, unless the heading sets {#id}. */
function anchorsOf(markdown) {
  const ids = new Set()
  let fenced = false
  for (const line of markdown.split('\n')) {
    if (line.startsWith('```')) fenced = !fenced
    if (fenced) continue
    const heading = /^#{1,6}\s+(.*?)\s*$/.exec(line)
    if (!heading) continue
    const explicit = /\{#([a-z0-9-]+)[^}]*\}\s*$/.exec(heading[1])
    ids.add(explicit ? explicit[1] : heading[1].toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, ''))
  }
  return ids
}

for (const [topic, { path }] of Object.entries(HELP_TOPICS)) {
  test(`help link ${topic} points at an existing docs page and anchor: ${path}`, () => {
    const file = pageFile(path)
    assert.ok(existsSync(file), `${path} has no page under docs/site/content`)
    const hash = path.split('#')[1]
    if (hash) assert.ok(anchorsOf(readFileSync(file, 'utf8')).has(hash), `${path}: anchor #${hash} not found in ${file}`)
  })
}

test('a heading gives its slug, an explicit id wins, and a fenced line is no heading', () => {
  const ids = anchorsOf('## Hand off files\n### Routes {#api}\n```\n# not a heading\n```\n')
  assert.deepEqual([...ids], ['hand-off-files', 'api'])
})
