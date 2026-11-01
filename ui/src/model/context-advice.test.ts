import { describe, expect, it } from 'vitest'
import type { ContextContribution } from '../contract'
import { builtinOwner, contextAdvice, startHere } from './context-advice'

const report = { target: 'claude', startup: { words: 872, bytes: 5400 }, file_scope: { words: 46, bytes: 301 }, on_demand: { words: 316, bytes: 2084 } }
const row = (source: string, category: string, load: ContextContribution['load'], words = 50): ContextContribution => ({ source, category, load, words, bytes: words * 6 })

const CATEGORIES: [string, string, ContextContribution['load']][] = [
  ['.agnostic-ai/skills/release/SKILL.md', 'skill discovery', 'startup'],
  ['.agnostic-ai/agents/reviewer.md', 'agent discovery', 'startup'],
  ['.agnostic-ai/skills/release/SKILL.md', 'skill body', 'on-demand'],
  ['.agnostic-ai/agents/reviewer.md', 'agent body', 'on-demand'],
  ['.agnostic-ai/AGNOSTIC_AI.md', 'entry-point layer', 'startup'],
  ['docs/style.md', 'inline import', 'startup'],
  ['.agnostic-ai/rules/testing.md', 'rules section', 'startup'],
  ['.agnostic-ai/reviews/code-quality.md', 'reviews section', 'startup'],
  ['.agnostic-ai/rules/testing.md', 'always-on rule', 'startup'],
  ['.agnostic-ai/rules/go.md', 'scoped rule', 'file-scope'],
  ['.agnostic-ai/rules/go.md', 'conditional rule body', 'on-demand'],
  ['CLAUDE.md', 'generated framing', 'startup'],
  ['.agnostic-ai/other.md', 'something new', 'startup'],
]

describe('context advice', () => {
  it.each(CATEGORIES)('explains %s as %s', (source, category, load) => {
    const advice = contextAdvice(row(source, category, load), report, '/tmp/project')
    expect(advice.what.length).toBeGreaterThan(20)
    expect(advice.why.length).toBeGreaterThan(10)
    expect(advice.actions.length).toBeGreaterThanOrEqual(2)
    expect(advice.prompt).toContain(`Source: ${source}`)
    expect(advice.prompt).toContain(`Kind: ${category}`)
    expect(advice.prompt).toContain('Configuration root: /tmp/project')
    expect(advice.prompt).toContain('Do not apply any change until I agree.')
    expect(advice.prompt).toContain('run agnostic-ai sync, then agnostic-ai explain --context --target claude')
    expect(advice.prompt).toContain('not tokens')
    for (const text of [advice.what, advice.why, advice.prompt, ...advice.actions]) {
      expect(text).not.toMatch(/[–—]/)
      expect(text).not.toContain('undefined')
    }
  })

  it('names the share and the load rule', () => {
    const advice = contextAdvice(row('.agnostic-ai/rules/testing.md', 'always-on rule', 'startup', 270), report)
    expect(advice.prompt).toContain('Size: 270 words, 1,620 bytes (31% of startup content for claude)')
    expect(advice.prompt).toContain('Loads: at the start of every session (startup)')
    expect(advice.prompt).toContain('cuts its startup size')
    expect(advice.prompt).not.toContain('Configuration root')
    expect(advice.actions[0]).toContain('Add globs to .agnostic-ai/rules/testing.md')
  })

  it('keeps the file in the check step', () => {
    const advice = contextAdvice(row('.agnostic-ai/rules/go.md', 'scoped rule', 'file-scope'), { ...report, file: 'src/main.go' })
    expect(advice.what).toContain('matches src/main.go')
    expect(advice.actions.at(-1)).toBe('Run agnostic-ai sync, then agnostic-ai explain --context --target claude --file src/main.go to check the new counts.')
    expect(advice.prompt).toContain('--target claude --file src/main.go')
    expect(advice.prompt).toContain('cuts its size')
  })

  it('maps a built-in spec to the built-in that ships it', () => {
    expect(builtinOwner('builtin:shared-memory-policy')).toBe('memory')
    expect(builtinOwner('builtin:caveman')).toBe('caveman')
    expect(builtinOwner('builtin:unknown-thing')).toBeUndefined()
    expect(builtinOwner('.agnostic-ai/rules/testing.md')).toBeUndefined()
    const advice = contextAdvice(row('builtin:shared-memory-policy', 'always-on rule', 'startup', 270), report)
    expect(advice.builtin).toBe('memory')
    expect(advice.what).toContain('part of the memory built-in')
    expect(advice.actions[0]).toContain('Write .agnostic-ai/rules/shared-memory-policy.md with the same name')
    expect(advice.actions[1]).toContain('turn memory off')
    expect(advice.actions[1]).toContain('removes shared memory for every tool')
    expect(advice.actions[2]).toContain('agnostic-ai.local.yaml')
    expect(advice.prompt).toContain('Built-in: memory')
    const skill = contextAdvice(row('builtin:caveman', 'skill discovery', 'startup'), report)
    expect(skill.actions[0]).toContain('.agnostic-ai/skills/caveman/SKILL.md')
    expect(skill.actions[1]).not.toContain('shared memory')
    const unknown = contextAdvice(row('builtin:mystery', 'always-on rule', 'startup'), report)
    expect(unknown.builtin).toBeUndefined()
    expect(unknown.what).toContain('part of a built-in')
    expect(unknown.prompt).toContain('Built-in: unknown')
  })

  it('picks the three largest startup sources and skips framing', () => {
    const rows = [row('CLAUDE.md', 'generated framing', 'startup', 900), row('a.md', 'always-on rule', 'startup', 10), row('b.md', 'entry-point layer', 'startup', 120), row('c.md', 'skill discovery', 'startup', 30), row('d.md', 'skill discovery', 'startup', 30), row('e.md', 'skill body', 'on-demand', 500)]
    expect(startHere(rows).map((c) => c.source)).toEqual(['b.md', 'c.md', 'd.md'])
    expect(startHere([])).toEqual([])
  })
})
