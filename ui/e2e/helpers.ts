// Shared helpers for the end-to-end tests.
//
//   E2E_BASE_URL  the running client (default http://127.0.0.1:4310, the Go server)
//   E2E_MOCK=1    append ?mock=1 so the client reads its bundled fixtures; the
//                 impact endpoint is then stubbed inside the page
//   E2E_SCREENSHOT_DIR  where screenshots go (default docs/internal/assets/metaprogramming/authority)

import { expect, type Page, type Request } from '@playwright/test'
import { mkdirSync, readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import type { Change, EffectChange, ImpactResult, Proposal } from '../src/contract'
import { normalizeAuthorityView } from '../src/model/normalize'
import { previewProposal } from '../src/model/proposal'

const here = dirname(fileURLToPath(import.meta.url))
export const MOCK = process.env.E2E_MOCK === '1'
export const SCREENSHOT_DIR = process.env.E2E_SCREENSHOT_DIR ?? resolve(here, '../../docs/internal/assets/metaprogramming/authority')

/** A relative app URL, so a base URL with a path prefix keeps working. */
export function appUrl(query = ''): string {
  const params = new URLSearchParams(query.replace(/^\?/, ''))
  if (MOCK) params.set('mock', '1')
  const search = params.toString()
  return search ? `./?${search}` : './'
}

export async function openApp(page: Page, query = ''): Promise<void> {
  await page.goto(appUrl(query))
}

/** Scrolls so the element's top sits just below the sticky header. */
export async function scrollToTop(page: Page, selector: string, offset = 84): Promise<void> {
  await page.locator(selector).first().waitFor()
  await page.evaluate(
    ([sel, off]) => {
      const el = document.querySelector(sel as string)
      if (el) window.scrollTo(0, el.getBoundingClientRect().top + window.scrollY - (off as number))
    },
    [selector, offset],
  )
}

export async function saveScreenshot(page: Page, name: string, options: { fullPage?: boolean } = {}): Promise<void> {
  mkdirSync(SCREENSHOT_DIR, { recursive: true })
  // Park the pointer in a corner so no hover state shows in the picture.
  await page.mouse.move(1, 1)
  await page.screenshot({ path: resolve(SCREENSHOT_DIR, name), animations: 'disabled', fullPage: options.fullPage === true })
}

/** Reads the mock fixtures from disk, for stubs. */
export function fixture(name: string): unknown {
  return JSON.parse(readFileSync(resolve(here, '../src/fixtures', name), 'utf8'))
}

const PERMISSIONS = '.agnostic-ai/settings/permissions.yaml'

function demoPermissions(): string {
  return readFileSync(resolve(here, '../../internal/authority/testdata/demo-project', PERMISSIONS), 'utf8')
}

/** Applies changes to the demo permissions file as text, as the server would. */
function applyToYaml(text: string, changes: Change[]): string {
  const lines = text.split('\n')
  for (const change of changes) {
    const header = lines.findIndex((l) => l.trim() === `${change.list}:`)
    let end = header + 1
    while (header >= 0 && end < lines.length && /^\s+- /.test(lines[end])) end++
    if (change.op === 'remove') {
      for (let i = header + 1; i < end; i++) {
        if (lines[i].trim() === `- ${change.rule}`) {
          lines.splice(i, 1)
          break
        }
      }
    } else if (header >= 0) {
      lines.splice(end, 0, `    - ${change.rule}`)
    } else {
      const at = lines[lines.length - 1] === '' ? lines.length - 1 : lines.length
      lines.splice(at, 0, `  ${change.list}:`, `    - ${change.rule}`)
    }
  }
  return lines.join('\n')
}

/** A plausible ImpactResult for a proposal, built from the mock view. */
export function stubImpact(proposal: Proposal): ImpactResult {
  const view = normalizeAuthorityView(fixture('authority.mock.json'))
  const preview = previewProposal(view, proposal.changes)
  const changes: EffectChange[] = preview.changes.flatMap((c) =>
    ['coding-agent', 'session'].map((subjectId) => ({
      subjectId,
      resourceId: c.resourceId,
      resourceLabel: c.resourceLabel,
      operationId: c.operationId,
      operationLabel: c.operationLabel,
      declaredBefore: c.declaredBefore,
      declaredAfter: c.declaredAfter,
      effectiveBefore: c.effectiveBefore,
      effectiveAfter: c.effectiveAfter,
      direction: c.direction,
      ...(c.reason ? { reason: c.reason } : {}),
    })),
  )
  const count = (d: string) => changes.filter((c) => c.direction === d).length
  const before = demoPermissions()
  return {
    schemaVersion: 'authority/v1',
    status: 'proposed',
    target: proposal.target ?? 'claude',
    baseRevision: proposal.revision ?? view.snapshot.revision,
    proposedRevision: 'e2e0stub0001',
    valid: true,
    problems: [],
    changes,
    summary: { changed: changes.length, widened: count('widened'), narrowed: count('narrowed'), moved: count('moved'), indeterminate: count('indeterminate') },
    affectedSubjects: changes.length ? ['coding-agent', 'session'] : [],
    diff: [{ path: PERMISSIONS, before, after: applyToYaml(before, proposal.changes) }],
  }
}

/** In mock mode, answers the impact endpoint inside the page; against a server, does nothing. */
export async function stubImpactInMockMode(page: Page): Promise<void> {
  if (!MOCK) return
  await page.route('**/api/views/authority/impact', async (route) => {
    const proposal = route.request().postDataJSON() as Proposal
    await route.fulfill({ json: stubImpact(proposal) })
  })
}

export function isImpactRequest(request: Request): boolean {
  return request.url().includes('/api/views/authority/impact') && request.method() === 'POST'
}

/** Every wording an effective badge may use, conditional or not. */
export const EFFECT_WORDING =
  /Permitted|Would be permitted once synced|Would require approval once synced|Would be blocked once synced|Blocked|Unavailable through this route|No rule; the tool's default|Unresolved on this tool/

/** A resource status pill: an operation and its effect, or a summary such as "Read only". */
export const PILL_WORDING = /permitted|approval|blocked|unavailable through this route|no rule|unresolved on this tool|read only/i

type LooseView = {
  harness: { target: string; label: string; permissionSupport: string; available: Array<{ target: string; label: string }>; installed: { state: string; files: Array<{ path: string; state: string }> }; note?: string }
  agent: { id: string }
  resources: Array<{ id: string; operations: Array<{ id: string; declared: string; effect: string; resolution: string; native?: string[]; enforcement: Array<{ target: string; status: string; effect: string; native?: string[] }> }> }>
  edges: Array<{ operationId: string; effect: string }>
  domains: Array<{ resourceIds: string[]; summary: Record<string, number> }>
}

/**
 * Derives the view for another coding tool from the mock's own
 * enforcement matrix, to stand in for a static snapshot file such as
 * authority.coding-agent.codex.json. Test-only: the server decides this.
 */
export function deriveToolView(target: string): unknown {
  const view = structuredClone(fixture('authority.mock.json')) as LooseView
  const option = view.harness.available.find((h) => h.target === target)!
  view.harness = {
    ...view.harness,
    target,
    label: option.label,
    permissionSupport: target === 'codex' ? 'hooks-only' : 'native-rules',
    installed: { state: 'not-generated', files: [{ path: target === 'codex' ? '.codex/config.toml' : '.cursor/cli.json', state: 'not-generated' }] },
    note: `Derived for the end-to-end tests from the ${option.label} enforcement column.`,
  }
  for (const resource of view.resources) {
    for (const op of resource.operations) {
      const e = op.enforcement.find((x) => x.target === target)!
      op.effect = e.effect
      op.native = e.native
      op.resolution =
        e.status === 'hook' ? 'hook' : e.status === 'not-expressed' ? 'not-expressed' : e.status === 'not-applicable' ? 'no-rule' : e.effect === 'unresolved' ? 'translated-unverified' : op.declared === 'unavailable' ? 'agent-scope' : 'translated'
    }
  }
  const effects = new Map(view.resources.flatMap((r) => r.operations.map((o) => [o.id, o.effect] as const)))
  for (const edge of view.edges) edge.effect = effects.get(edge.operationId)!
  for (const domain of view.domains) {
    const summary: Record<string, number> = { allowed: 0, ask: 0, denied: 0, unavailable: 0, default: 0, unresolved: 0 }
    for (const r of view.resources.filter((res) => domain.resourceIds.includes(res.id))) for (const o of r.operations) summary[o.effect]++
    domain.summary = summary
  }
  return view
}
