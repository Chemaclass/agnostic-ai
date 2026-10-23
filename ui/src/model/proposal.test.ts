import { describe, expect, it } from 'vitest'
import { findResource } from './domains'
import {
  buildProposal,
  commandCovers,
  describeChange,
  directionOf,
  globCovers,
  parseRule,
  previewProposal,
  proposalAction,
  ruleForOperation,
  ruleMatchesOperation,
} from './proposal'
import { loadAuthorityFixture, usingMockAuthority } from './testing'

describe('rule grammar', () => {
  it('parses the neutral rule forms', () => {
    expect(parseRule('mcp:railway/deploy')).toEqual({ kind: 'mcp', server: 'railway', tool: 'deploy' })
    expect(parseRule('mcp:github')).toEqual({ kind: 'mcp', server: 'github' })
    expect(parseRule('mcp__railway__deploy')).toEqual({ kind: 'mcp', server: 'railway', tool: 'deploy' })
    expect(parseRule('shell(git push:*)')).toEqual({ kind: 'scoped', scope: 'shell', arg: 'git push:*' })
    expect(parseRule('read')).toEqual({ kind: 'bare', scope: 'read' })
    expect(parseRule('???').kind).toBe('unknown')
  })

  it('judges command and glob containment', () => {
    expect(commandCovers('git:*', 'git push:*')).toBe(true)
    expect(commandCovers('git *', 'git status')).toBe(true)
    expect(commandCovers('git push:*', 'git push:*')).toBe(true)
    expect(commandCovers('git push:*', 'git:*')).toBe(false)
    expect(commandCovers('git status', 'git status')).toBe(true)
    expect(commandCovers('git status', 'git status:*')).toBe(false)
    expect(commandCovers('*', 'rm:*')).toBe(true)
    expect(globCovers('**', '.env')).toBe(true)
    expect(globCovers('src/**', 'src/**')).toBe(true)
    expect(globCovers('*.env', '.env')).toBe(true)
    expect(globCovers('src/**', '.github/**')).toBe(false)
  })

  it('directs changes only between known decisions', () => {
    expect(directionOf('allowed', 'ask')).toEqual({ direction: 'narrowed' })
    expect(directionOf('denied', 'allowed')).toEqual({ direction: 'widened' })
    expect(directionOf('denied', 'unavailable')).toEqual({ direction: 'moved' })
    expect(directionOf('allowed', 'default').direction).toBe('indeterminate')
    expect(directionOf('unresolved', 'denied').reason).toMatch(/no established order/)
  })

  it('describes a change in plain words', () => {
    expect(describeChange({ op: 'add', list: 'ask', rule: 'mcp:railway/deploy' })).toBe('Add mcp:railway/deploy to the ask list')
    expect(describeChange({ op: 'remove', list: 'allow', rule: 'read' })).toBe('Remove read from the allow list')
  })
})

describe('client proposal preview', () => {
  const view = loadAuthorityFixture()

  it('always derives some rule for every operation of any fixture', () => {
    for (const r of view.resources) for (const op of r.operations) expect(ruleForOperation(r, op).length, op.id).toBeGreaterThan(0)
  })

  it('previews nothing for an empty proposal and reports a rule that matches nothing', () => {
    expect(previewProposal(view, []).changes).toEqual([])
    const preview = previewProposal(view, [{ op: 'add', list: 'deny', rule: 'mcp:no-such-server/thing' }])
    expect(preview.changes).toEqual([])
    expect(preview.unmatched).toHaveLength(1)
  })

  it('builds the body the impact endpoint expects', () => {
    const changes = [{ op: 'add' as const, list: 'deny' as const, rule: 'shell(rm:*)' }]
    expect(buildProposal(view, changes)).toEqual({ agentId: view.agent.id, target: view.harness.target, revision: view.snapshot.revision, changes })
  })

  describe.runIf(usingMockAuthority)('on the mock', () => {
    const railway = findResource(view, 'mcp-railway-scope')!
    const deploy = railway.operations.find((o) => o.action === 'deploy')!
    const pushRes = findResource(view, 'shell-git-push')!
    const push = pushRes.operations[0]

    it('offers the three row actions with the rules they would write', () => {
      expect(ruleForOperation(railway, deploy)).toBe('mcp:railway/deploy')
      expect(proposalAction('revoke', railway, deploy).changes).toEqual([{ op: 'remove', list: 'allow', rule: 'mcp:railway/deploy' }])
      expect(proposalAction('require-approval', railway, deploy).changes).toEqual([{ op: 'add', list: 'ask', rule: 'mcp:railway/deploy' }])
      expect(proposalAction('block', railway, deploy).changes).toEqual([{ op: 'add', list: 'deny', rule: 'mcp:railway/deploy' }])
      expect(proposalAction('revoke', pushRes, push).enabled).toBe(false)
      expect(proposalAction('require-approval', pushRes, push).enabled).toBe(false)
      const files = findResource(view, 'files-github')!
      expect(ruleForOperation(files, files.operations.find((o) => o.action === 'edit')!)).toBe('edit(.github/**)')
    })

    it('matches rules to operations by the neutral grammar', () => {
      expect(ruleMatchesOperation('mcp:railway', railway, deploy)).toBe(true)
      expect(ruleMatchesOperation('mcp:railway/logs', railway, deploy)).toBe(false)
      expect(ruleMatchesOperation('shell(git:*)', pushRes, push)).toBe(true)
      const env = findResource(view, 'files-env')!
      expect(ruleMatchesOperation('edit', env, env.operations.find((o) => o.action === 'edit')!)).toBe(true)
      expect(ruleMatchesOperation('edit', env, env.operations.find((o) => o.action === 'read')!)).toBe(false)
    })

    it('narrows Railway deploys when an ask rule is added', () => {
      const preview = previewProposal(view, [{ op: 'add', list: 'ask', rule: 'mcp:railway/deploy' }])
      expect(preview.changes).toEqual([
        expect.objectContaining({
          operationId: deploy.id,
          declaredBefore: 'allowed',
          declaredAfter: 'ask',
          effectiveBefore: 'allowed',
          effectiveAfter: 'ask',
          direction: 'narrowed',
        }),
      ])
    })

    it('reports a revoked allow as indeterminate, because the default has no known order', () => {
      const preview = previewProposal(view, [{ op: 'remove', list: 'allow', rule: 'mcp:railway/deploy' }])
      expect(preview.changes).toEqual([expect.objectContaining({ declaredAfter: 'default', effectiveAfter: 'default', direction: 'indeterminate' })])
    })

    it('blocks git push with a deny rule', () => {
      const preview = previewProposal(view, [{ op: 'add', list: 'deny', rule: 'shell(git push:*)' }])
      expect(preview.changes).toEqual([expect.objectContaining({ operationId: push.id, effectiveBefore: 'ask', effectiveAfter: 'denied', direction: 'narrowed' })])
    })

    it('keeps an agent-scope exclusion over any change to the settings lists', () => {
      // rm is outside the coding agent's can list, so it stays unavailable.
      const removed = previewProposal(view, [{ op: 'remove', list: 'deny', rule: 'shell(rm:*)' }])
      expect(removed.changes).toEqual([])
      expect(removed.unmatched).toEqual([])
      expect(previewProposal(view, [{ op: 'add', list: 'allow', rule: 'shell(rm:*)' }]).changes).toEqual([])
    })
  })
})
