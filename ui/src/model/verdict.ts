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
  const parts: string[] = []
  const actions = (count: number) => (parts.length ? `${count}` : plural(count, 'action'))
  if (counts['permissions.allow']) parts.push(`allow ${actions(counts['permissions.allow'])}`)
  if (counts['permissions.ask']) parts.push(`ask before ${actions(counts['permissions.ask'])}`)
  if (counts['permissions.deny']) parts.push(`block ${actions(counts['permissions.deny'])}`)
  return parts.length ? `Your rules ${joinAnd(parts)}.` : 'No allow, ask or block rules yet.'
}

function toolsVerdict(section: VerdictSection): string {
  const parts = counted(section)
  const usable = section.support?.find((support) => support.kind === 'mcp')?.supported.length
  if (parts.length) return `Your ${usable ? 'agents can use' : 'setup configures'} ${joinAnd(parts)}.`
  return turnedOff(section).length ? 'No MCP servers are turned on.' : 'Your setup configures no extra tools yet.'
}

function lifecycleVerdict(section: VerdictSection): string {
  const all = section.items.filter((item) => item.kind === 'hook')
  if (!all.length) return 'Nothing runs around the agent yet.'
  const hooks = all.filter((item) => !isTurnedOff(item))
  if (!hooks.length) return 'No hooks are set to run.'
  const verb = hooks.length === 1 ? 'is' : 'are'
  const stages = new Set(hooks.map((hook) => hook.group).filter(Boolean)).size
  return stages ? `${plural(hooks.length, 'hook')} ${verb} set to run at ${plural(stages, 'stage')} of a session.` : `${plural(hooks.length, 'hook')} ${verb} set.`
}

function settingsVerdict(section: VerdictSection): string {
  const builtins = section.items.filter((item) => item.kind === 'builtin' && builtinEnabled(item, 'shared')).length
  const preferences = (section.fields ?? []).filter((field) => field.set.some((source) => !source.local)).length
  const parts = [builtins ? `${plural(builtins, 'built-in')} on` : '', preferences ? `${plural(preferences, 'preference')} set` : ''].filter(Boolean)
  return parts.length ? `Your tools have ${joinAnd(parts)}.` : 'Your tools run on their own defaults.'
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

/** The permission mode a settings file already sets; a local file overrides the shared one. */
function defaultMode(section: VerdictSection): string | undefined {
  const field = section.fields?.find((f) => f.key === 'permissions.default-mode')
  const sources = [...(field?.set ?? [])].sort((a, b) => Number(Boolean(b.local)) - Number(Boolean(a.local)))
  return sources.flatMap((source) => source.values).find((value) => value.key === field?.key && value.value)?.value
}

function sectionOwnFacts(section: VerdictSection, global: boolean): string[] {
  if (section.id === 'authority') {
    const paths = section.items.flatMap((item) => item.values).filter((value) => value.key === 'protected.paths').reduce((sum, value) => sum + (value.list?.length ?? 0), 0)
    // Outside the global view the default mode applies only through global sync.
    const mode = global ? defaultMode(section) : undefined
    return [...(mode ? [`Default mode: ${mode}`] : []), ...(paths ? [plural(paths, 'protected path')] : [])]
  }
  if (section.id === 'tools' && section.native?.length) return [`${plural(section.native.length, 'more server')} found in your tools`]
  if (section.id === 'lifecycle' && section.stages?.length) return [`${plural(section.stages.length, 'stage')} in a session`]
  if (section.id === 'settings') {
    const files = section.items.filter((item) => item.kind === 'config').length
    return files ? [plural(files, 'config file')] : []
  }
  return []
}

/** "Kiro ignores permissions": configured types that some set-up tools do not apply. */
export function ignoredFacts(section: VerdictSection, toolName: (target: string) => string): string[] {
  return (section.support ?? []).flatMap((support) => {
    const kind = section.kinds.find((k) => k.kind === support.kind)
    if (!kind || !liveCount(section, kind) || !support.unsupported.length) return []
    const total = support.supported.length + support.unsupported.length
    const who = support.unsupported.length <= 3 ? joinAnd(support.unsupported.map(toolName)) : `${support.unsupported.length} of ${total} tools`
    return [`${who} ${support.unsupported.length === 1 ? 'ignores' : 'ignore'} ${kindNoun(support, 2)}`]
  })
}

/** Facts beside the verdict that it does not already say. */
export function sectionFacts(section: VerdictSection, toolName: (target: string) => string = (target) => target, global = false): string[] {
  const off = turnedOff(section)
  return [...sectionOwnFacts(section, global), ...(off.length ? [`${joinAnd(off)} turned off`] : []), ...ignoredFacts(section, toolName)]
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
