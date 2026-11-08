// The words at the top of a section page: a verdict sentence, a facts
// line, the folded support summary, and the empty state of a type. All
// of it comes from the loaded overview.

import type { ItemFinding, KindCount, KindSupport, Section } from '../contract'
import { builtinEnabled } from './audience-view'
import { isTurnedOff, joinAnd, kindNoun, liveCount, permissionCounts, plural, turnedOff } from './counts'

type VerdictSection = Pick<Section, 'id' | 'kinds' | 'items'> & Partial<Pick<Section, 'fields' | 'native' | 'stages' | 'support'>>

// Verdicts describe what the setup configures, not what an agent will do: a tool that
// does not support a type ignores it, and the facts line says so. The Tools verdict
// says what agents can use only when a set-up tool supports MCP servers.

function counted(section: VerdictSection): string[] {
  return section.kinds.map((kind) => ({ ...kind, count: liveCount(section, kind) })).filter((kind) => kind.count > 0).map((kind) => `${kind.count} ${kindNoun(kind, kind.count)}`)
}

function contextVerdict(section: VerdictSection): string {
  const parts = counted(section)
  return parts.length ? `Your setup configures ${joinAnd(parts)}.` : 'Your setup configures no instructions yet.'
}

function authorityVerdict(section: VerdictSection): string {
  const counts = permissionCounts(section)
  const blocked = counts['permissions.deny']
  const ask = counts['permissions.ask']
  const allowed = counts['permissions.allow']
  if (blocked) return `${plural(blocked, 'action')} ${blocked === 1 ? 'is' : 'are'} blocked${ask ? ` and agents ask before ${ask}` : ''}.`
  if (ask) return `Agents ask before ${plural(ask, 'action')}.`
  if (allowed) return `Agents run ${plural(allowed, 'action')} without asking.`
  return 'Each tool decides when to ask.'
}

function toolsVerdict(section: VerdictSection): string {
  const parts = counted(section)
  const usable = section.support?.find((support) => support.kind === 'mcp')?.supported.length
  if (parts.length) return `${usable ? 'Agents can use' : 'Your setup configures'} ${joinAnd(parts)}.`
  return turnedOff(section).length ? 'No MCP servers are turned on.' : 'Your setup configures no extra tools yet.'
}

// One stage names its moment; several stages give the count.
const STAGE_MOMENTS: Record<string, string> = {
  'session-start': 'when a session starts',
  'prompt-submit': 'when a prompt is sent',
  'before-tool': 'before a tool runs',
  'after-tool': 'after a tool runs',
  'after-edit': 'after a file edit',
  stop: 'when the agent stops',
  'session-end': 'when a session ends',
}

function lifecycleVerdict(section: VerdictSection): string {
  const all = section.items.filter((item) => item.kind === 'hook')
  if (!all.length) return 'Nothing runs around the agent yet.'
  const hooks = all.filter((item) => !isTurnedOff(item))
  if (!hooks.length) return 'No hooks are set to run.'
  const verb = hooks.length === 1 ? 'runs' : 'run'
  const stages = [...new Set(hooks.map((hook) => hook.group).filter((group): group is string => Boolean(group)))]
  if (!stages.length) return `${plural(hooks.length, 'hook')} ${hooks.length === 1 ? 'is' : 'are'} set.`
  if (stages.length > 1) return `${plural(hooks.length, 'hook')} ${verb} at ${plural(stages.length, 'stage')}.`
  const stage = section.stages?.find((entry) => entry.id === stages[0])
  return `${plural(hooks.length, 'hook')} ${verb} ${STAGE_MOMENTS[stages[0]] ?? `at ${stage?.label ?? stages[0]}`}.`
}

function builtinsOn(section: VerdictSection): number {
  return section.items.filter((item) => item.kind === 'builtin' && builtinEnabled(item, 'shared')).length
}

function preferencesSet(section: VerdictSection): number {
  return (section.fields ?? []).filter((field) => field.set.some((source) => !source.local)).length
}

function settingsVerdict(section: VerdictSection): string {
  const builtins = builtinsOn(section)
  if (builtins) return `${plural(builtins, 'built-in')} ${builtins === 1 ? 'is' : 'are'} on.`
  const preferences = preferencesSet(section)
  if (preferences) return `${plural(preferences, 'preference')} ${preferences === 1 ? 'is' : 'are'} set.`
  return 'Your tools run on their own defaults.'
}

const VERDICTS: Record<string, (section: VerdictSection) => string> = {
  context: contextVerdict,
  authority: authorityVerdict,
  tools: toolsVerdict,
  lifecycle: lifecycleVerdict,
  settings: settingsVerdict,
}

/** The page's answer to its question, in one sentence. */
export function sectionVerdict(section: VerdictSection): string {
  return (VERDICTS[section.id] ?? contextVerdict)(section)
}

/** The count the verdict leads with, which the page sets in the accent: "1 environment", "2 built-ins". */
export function verdictKey(section: VerdictSection): string | undefined {
  if (section.id === 'tools') {
    const parts = counted(section)
    return parts.length ? joinAnd(parts) : undefined
  }
  if (section.id === 'settings') {
    const builtins = builtinsOn(section)
    if (builtins) return plural(builtins, 'built-in')
    const preferences = preferencesSet(section)
    return preferences ? plural(preferences, 'preference') : undefined
  }
  return undefined
}

/** "Claude Code and Codex", or "4 of your tools" when naming them would crowd the line. */
function toolNames(labels: string[]): string {
  const unique = [...new Set(labels)]
  return unique.length > 3 ? `${unique.length} of your tools` : joinAnd(unique)
}

function sectionOwnFacts(section: VerdictSection, tools?: ToolTally): string[] {
  if (section.id === 'authority') {
    const counts = permissionCounts(section)
    const allowed = counts['permissions.allow']
    // The verdict leads with blocked and ask rules; the allowed count is the fact it leaves out.
    if (!allowed && !counts['permissions.ask'] && !counts['permissions.deny']) return ['No allow, ask or block rules yet']
    return allowed && (counts['permissions.ask'] || counts['permissions.deny']) ? [`${allowed} run${allowed === 1 ? 's' : ''} without asking`] : []
  }
  if (section.id === 'tools' && section.native?.length) {
    const labels = section.native.flatMap((server) => server.holders.map((holder) => holder.label))
    return [`${plural(section.native.length, 'more server')} found in ${labels.length ? toolNames(labels) : 'your tools'}`]
  }
  if (section.id === 'lifecycle' && section.stages?.length) {
    const empty = section.stages.filter((stage) => !section.items.some((item) => item.kind === 'hook' && item.group === stage.id && !isTurnedOff(item))).length
    return empty ? [`${plural(empty, 'stage')} ${empty === 1 ? 'has' : 'have'} no hook`] : []
  }
  if (section.id === 'settings') {
    // The verdict names built-ins first; preferences become a fact only beside them.
    const preferences = builtinsOn(section) ? preferencesSet(section) : 0
    return [...(preferences ? [`${plural(preferences, 'preference')} set`] : []), ...(tools ? [`${tools.on} of ${tools.total} tools on`] : [])]
  }
  return []
}

// More facts than this crowd the line; tool support has its own link.
const MAX_FACTS = 3

export interface ToolTally {
  on: number
  total: number
}

/** Facts beside the verdict that it does not already say. Settings counts the coding tools that are on. */
export function sectionFacts(section: VerdictSection, tools?: ToolTally): string[] {
  const off = turnedOff(section)
  return [...sectionOwnFacts(section, tools), ...(off.length ? [`${joinAnd(off)} turned off`] : [])].slice(0, MAX_FACTS)
}

export interface FindingTally {
  errors: number
  warnings: number
}

export function tallyFindings(findings: ItemFinding[]): FindingTally {
  const errors = findings.filter((finding) => finding.severity === 'error').length
  return { errors, warnings: findings.length - errors }
}

/** "1 error, 2 warnings", or undefined with none. */
export function findingTallyText({ errors, warnings }: FindingTally): string | undefined {
  const parts = [errors ? plural(errors, 'error') : '', warnings ? plural(warnings, 'warning') : ''].filter(Boolean)
  return parts.length ? parts.join(', ') : undefined
}

/** Every finding on the page: the section's own and each item's. */
export function sectionFindings(section: Pick<Section, 'findings' | 'items'>): ItemFinding[] {
  return [...section.findings, ...section.items.flatMap((item) => item.findings)]
}

/** "3 of 3 tools support rules, agents, and skills": one clause per support level. */
export function supportSummary(support: KindSupport[]): string {
  if (!support.length) return ''
  const total = support[0].supported.length + support[0].unsupported.length
  if (!total) return 'No coding tool is set up yet'
  const groups = new Map<number, string[]>()
  for (const kind of support) {
    const nouns = groups.get(kind.supported.length) ?? []
    nouns.push(kindNoun(kind, 2))
    groups.set(kind.supported.length, nouns)
  }
  return [...groups.entries()].sort(([a], [b]) => b - a).map(([count, nouns], index) => {
    const tools = index === 0 ? `${count} of ${total} ${total === 1 ? 'tool' : 'tools'}` : `${count} of ${total}`
    return `${tools} ${count === 1 ? 'supports' : 'support'} ${joinAnd(nouns)}`
  }).join('; ')
}

const KIND_PURPOSE: Record<string, string> = {
  instructions: 'An instructions file is what every tool reads at the start of a session.',
  rule: 'A rule is a convention the agent follows, such as how to test or name things.',
  agent: 'An agent is a helper with its own role, such as a reviewer.',
  skill: 'A skill is a workflow the agent runs on request, such as cutting a release.',
  command: 'A command is a prompt you start by name.',
  review: 'Review instructions say what a code review checks in this project.',
  ignore: 'An ignore file lists files the agent must not read, such as secrets or build output.',
  mcp: 'An MCP server gives the agent more tools, such as a database or an issue tracker.',
  hook: 'A hook is a command that runs at a set point in a session, such as before a tool call.',
  builtin: 'A built-in is a ready-made skill or agent that comes with agnostic-ai.',
  settings: 'A preference sets how your tools behave, such as the model they use.',
  config: 'A config file says which tools agnostic-ai sets up.',
}

export interface EmptyState {
  title: string
  purpose?: string
  tools?: string
}

/** What an empty type is and which tools use it; "No matches." while a filter hides everything. */
export function emptyState(kind: Pick<KindCount, 'kind' | 'label'>, support: KindSupport | undefined, toolName: (target: string) => string, filtering = false): EmptyState {
  if (filtering) return { title: 'No matches.' }
  const noun = kindNoun(kind, 2)
  const tools = support && (support.supported.length
    ? `${joinAnd(support.supported.map(toolName))} ${support.supported.length === 1 ? 'supports' : 'support'} ${noun}.`
    : `None of your tools support ${noun} yet.`)
  return { title: `No ${noun} yet.`, purpose: KIND_PURPOSE[kind.kind], tools }
}
