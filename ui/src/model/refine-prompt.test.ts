import { describe, expect, it } from 'vitest'
import { instructionsRefinePrompt } from './refine-prompt'

describe('instructions refine prompt', () => {
  it('names the file and limits the work to it', () => {
    const prompt = instructionsRefinePrompt('/home/me/.agnostic-ai/AGNOSTIC_AI.md')
    expect(prompt).toContain('global agent instructions in /home/me/.agnostic-ai/AGNOSTIC_AI.md')
    expect(prompt).toContain('work on that file alone')
  })

  it('stops for approval before editing, then syncs and checks', () => {
    const prompt = instructionsRefinePrompt('AGNOSTIC_AI.md')
    expect(prompt.indexOf('Then wait for my approval.')).toBeLessThan(prompt.indexOf('Write the approved version.'))
    expect(prompt).toContain('agnostic-ai sync --global --plan')
    expect(prompt).toContain('agnostic-ai sync --global --check')
  })
})
