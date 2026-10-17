import { describe, expect, it } from 'vitest'
import type { Operation } from '../contract'
import {
  EFFECT_WORDING,
  declaredWording,
  effectWording,
  effectiveDisplay,
  excludedBy,
  installedDisplay,
  listPhrase,
} from './effects'
import { findResource } from './domains'
import { loadAuthorityFixture, usingMockAuthority } from './testing'

function op(partial: Partial<Operation>): Operation {
  return {
    id: 'r:op',
    action: 'deploy',
    label: 'Deploy',
    declared: 'allowed',
    effect: 'allowed',
    resolution: 'translated',
    installed: 'in-sync',
    hypothetical: false,
    runtime: 'not-observed',
    grants: [],
    conditions: [],
    alternateRoutes: [],
    routeCoverage: { checked: [], notModeled: [] },
    enforcement: [],
    explanation: '',
    ...partial,
  }
}

describe('effect label lookup', () => {
  it('uses the product wording for each effect', () => {
    expect(EFFECT_WORDING).toEqual({
      allowed: 'Permitted',
      ask: 'Permitted, approval required',
      denied: 'Blocked',
      unavailable: 'Unavailable through this route',
      default: "No rule; the tool's default applies",
      unresolved: 'Unresolved on this tool',
    })
  })

  it('words a hypothetical effect conditionally and never as a plain "Permitted"', () => {
    expect(effectWording('allowed', true)).toBe('Would be permitted once synced')
    expect(effectWording('ask', true)).toBe('Would require approval once synced')
    expect(effectWording('denied', true)).toBe('Would be blocked once synced')
    for (const effect of ['allowed', 'ask', 'denied', 'unavailable', 'default'] as const) {
      expect(effectWording(effect, true)).not.toBe(EFFECT_WORDING[effect])
      expect(effectWording(effect, true)).toMatch(/once synced/)
    }
  })

  it('shows a rule that is written but unverified as unresolved, with the declared policy beside it', () => {
    const display = effectiveDisplay(op({ effect: 'unresolved', declared: 'allowed', resolution: 'translated-unverified' }))
    expect(display.text).toBe('Unresolved on this tool · rule written, matching not verified')
    expect(display.secondary).toEqual({ text: 'Declared: permitted', tone: 'neutral' })
    const notExpressed = effectiveDisplay(op({ effect: 'unresolved', declared: 'denied', resolution: 'not-expressed' }))
    expect(notExpressed.text).toBe('Unresolved on this tool')
    expect(notExpressed.secondary?.text).toBe('Declared: blocked')
  })

  it('names the excluding list from the winning grant of an unavailable operation', () => {
    const unavailable = op({
      effect: 'unavailable',
      declared: 'unavailable',
      resolution: 'agent-scope',
      decided: { rule: 'mcpServers: [github]', list: 'mcpServers', scope: 'agent', source: { path: 'a.md', layer: 'project' }, wins: true },
    })
    expect(excludedBy(unavailable)).toBe("Excluded by the agent's mcpServers list")
    expect(effectiveDisplay(unavailable).text).toBe('Unavailable through this route')
    expect(excludedBy(op({}))).toBeUndefined()
    expect(listPhrase('readonly')).toBe("the agent's readonly flag")
  })

  it('takes explanations from the view when present', () => {
    const view = loadAuthorityFixture()
    const first = view.resources[0].operations[0]
    const display = effectiveDisplay(first, view.explanations)
    expect(display.explanation).toBe(view.explanations.effects[first.effect])
    expect(installedDisplay('not-generated', view.explanations).note).toBe(
      view.explanations.installed['not-generated'] ?? 'Sync has not written the generated file yet.',
    )
    expect(installedDisplay('stale').note).toMatch(/older version/)
  })

  it('keeps declared wording distinct from effective wording', () => {
    expect(declaredWording('ask')).toBe('Declared: permitted, approval required')
    expect(declaredWording('denied')).toBe('Declared: blocked')
  })

  it.runIf(usingMockAuthority)('words the mock Railway deploy as hypothetical, because nothing is generated', () => {
    const view = loadAuthorityFixture()
    const deploy = findResource(view, 'mcp-railway-scope')!.operations.find((o) => o.action === 'deploy')!
    expect(effectiveDisplay(deploy, view.explanations).text).toBe('Would be permitted once synced')
  })
})
