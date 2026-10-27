import { describe, expect, it } from 'vitest'
import { instructionsRefinePrompt } from './refine-prompt'

describe('instructions refine prompt', () => {
  it('names the file and limits the work to it', () => {
    const prompt = instructionsRefinePrompt('/home/me/.agnostic-ai/AGNOSTIC_AI.md')
    expect(prompt).toContain('global agent instructions in /home/me/.agnostic-ai/AGNOSTIC_AI.md')
    expect(prompt).toContain('edit that file alone')
  })

  it('recommends skill and agent candidates without moving content', () => {
    const prompt = instructionsRefinePrompt('AGNOSTIC_AI.md')
    expect(prompt).toContain('every skill in skills/ and every agent in agents/')
    expect(prompt).toContain('A candidates table')
    expect(prompt).toContain('Keep the content in this file. Candidates are recommendations')
  })

  it('stops for approval before editing, then syncs and checks', () => {
    const prompt = instructionsRefinePrompt('AGNOSTIC_AI.md')
    expect(prompt.indexOf('read-only review until I explicitly approve')).toBeLessThan(prompt.indexOf('## 1.'))
    expect(prompt.indexOf('Then wait for my explicit approval.')).toBeLessThan(prompt.indexOf('Write the approved version.'))
    expect(prompt).toContain('agnostic-ai sync --global --plan')
    expect(prompt).toContain('agnostic-ai sync --global --check')
  })
})
