import type { ContextContribution, ContextLoad, ContextReport, ContextSize } from '../contract'
import { isBuiltin, sharePercent } from './context-breakdown'
import { LOAD_CONDITIONS, LOAD_LABELS, sizeText } from './context-prompt'

export interface ContextAdvice {
  /** What this source is, in one or two sentences. */
  what: string
  /** Why it loads when it does. */
  why: string
  /** Steps a person can take, most useful first. The last one checks the result. */
  actions: string[]
  /** A prompt for a coding agent that proposes the smallest safe cut. */
  prompt: string
  /** The built-in that ships this spec, when known. */
  builtin?: string
}

export type AdviceReport = Pick<ContextReport, 'target' | 'startup' | 'file_scope' | 'on_demand'> & {
  file?: string
  /** False when the tool cannot check a single file. Unset means it can. */
  takesFile?: boolean
}

const MEMORY_INDEX = '.agnostic-ai/memory'

/** Quotes a path for a shell command only when it holds whitespace or a shell character. */
export function shellQuote(path: string): string {
  return /^[A-Za-z0-9_@%+=:,./-]+$/.test(path) ? path : `'${path.replace(/'/g, `'\\''`)}'`
}

export function explainCommand(target: string, file?: string): string {
  return `agnostic-ai explain --context --target ${target}${file ? ` --file ${shellQuote(file)}` : ''}`
}

/** Which built-in ships each spec name the context report can show as `builtin:<name>`. */
export const BUILTIN_OWNERS: Record<string, string> = {
  'shared-memory-policy': 'memory',
  'shared-memory': 'memory',
  'gemini-memory-tiers': 'memory',
  'kiro-memory-index': 'memory',
  'antigravity-memory-index': 'memory',
  caveman: 'caveman',
  handoff: 'handoff',
}

export function builtinOwner(source: string): string | undefined {
  if (!isBuiltin(source)) return undefined
  return BUILTIN_OWNERS[source.slice('builtin:'.length)]
}

type Kind = 'skill' | 'agent' | 'rule' | 'review' | 'instructions' | 'generated' | 'other'

function kindOf(category: string): Kind {
  if (category.startsWith('skill')) return 'skill'
  if (category.startsWith('agent')) return 'agent'
  if (category === 'always-on rule' || category === 'rules section' || category === 'scoped rule' || category === 'conditional rule body') return 'rule'
  if (category === 'reviews section') return 'review'
  if (category === 'entry-point layer' || category === 'inline import') return 'instructions'
  if (category === 'generated framing') return 'generated'
  return 'other'
}

function specName(source: string): string {
  if (isBuiltin(source)) return source.slice('builtin:'.length)
  const parts = source.split('/')
  const last = parts[parts.length - 1]
  if (last === 'SKILL.md' && parts.length > 1) return parts[parts.length - 2]
  return last.replace(/\.md$/, '')
}

function projectPath(kind: Kind, name: string): string {
  if (kind === 'skill') return `.agnostic-ai/skills/${name}/SKILL.md`
  if (kind === 'agent') return `.agnostic-ai/agents/${name}.md`
  return `.agnostic-ai/rules/${name}.md`
}

function checkStep(target: string, file?: string): string {
  return `Run agnostic-ai sync, then ${explainCommand(target, file)} to check the new counts.`
}

function loadTotal(report: AdviceReport, load: ContextLoad): ContextSize {
  if (load === 'startup') return report.startup
  if (load === 'file-scope') return report.file_scope
  return report.on_demand
}

interface Text { what: string; why: string; actions: string[] }

function describe(c: ContextContribution, report: AdviceReport, name: string): Text {
  const t = report.target
  const file = report.takesFile === false ? undefined : report.file
  const check = checkStep(t, file)
  switch (c.category) {
    case 'skill discovery':
      return {
        what: `The one-line description of the skill ${name}. Every session loads every skill description so ${t} can decide when to use the skill.`,
        why: `${t} needs the description at startup to choose the skill. The skill body loads only when the skill runs.`,
        actions: [
          `Shorten the description in ${c.source} to one sentence that says when to use the skill.`,
          'Remove the skill if nobody uses it, or limit it with targets to the tools that need it.',
          check,
        ],
      }
    case 'agent discovery':
      return {
        what: `The one-line description of the agent ${name}. Every session loads every agent description so ${t} can decide when to hand a task to it.`,
        why: `${t} needs the description at startup to pick the agent. The agent body loads only when the agent runs.`,
        actions: [
          `Shorten the description in ${c.source} to one sentence that says when to use the agent. Claude Code skips an agent with no description, so keep one.`,
          'Remove the agent if nobody uses it, or limit it with targets to the tools that need it.',
          check,
        ],
      }
    case 'skill body':
    case 'agent body': {
      const kind = c.category === 'skill body' ? 'skill' : 'agent'
      return {
        what: `The body of the ${kind} ${name}. It loads only when the ${kind} runs, so it adds nothing at startup.`,
        why: `${t} opens the ${kind} on demand. Only its description counts at startup.`,
        actions: [
          'Keep it. On-demand text adds no startup text.',
          `Trim steps the ${kind} can find in the code or docs, and link to them instead of copying.`,
          kind === 'skill' ? 'Move reference material to a file beside SKILL.md and point to it; the tool reads it only when needed.' : 'Keep the trigger in the description and the procedure in the body.',
          check,
        ],
      }
    }
    case 'entry-point layer':
      if (c.source === MEMORY_INDEX) return memoryIndexText(t, check)
      return {
        what: `Text from the instructions file ${c.source} that sync copies into the file ${t} reads first.`,
        why: 'The instructions file loads at the start of every session in every tool.',
        actions: [
          'Cut sentences that repeat what the code, the README or a rule already says.',
          'Move a task-only procedure into a skill under .agnostic-ai/skills/, which loads only when the task comes up.',
          'Move file-specific guidance into a rule with globs, which loads only for matching files in tools that support that.',
          check,
        ],
      }
    case 'inline import':
      return {
        what: `A file that the instructions file pulls in with an @path line. Sync copies its text into the file ${t} reads first.`,
        why: `${t} cannot follow the @path line itself, so sync.resolve-imports: inline writes the whole file at startup.`,
        actions: [
          `Shorten ${c.source}, or drop its @path line from .agnostic-ai/AGNOSTIC_AI.md.`,
          'If only some tasks need the file, turn it into a skill and point to it from the instructions file.',
          check,
        ],
      }
    case 'rules section':
      return {
        what: `A rule that ${t} reads from a Rules block inside its instructions file.`,
        why: `${t} reads always-on rules from that block, so they load at the start of every session.`,
        actions: [
          `Shorten ${c.source}: keep the rule, drop the explanation.`,
          `Limit the rule with targets to the tools that need it.`,
          `If the rule only matters for some files, check the rules page and the ${t} target page in the docs to see whether ${t} can load it by path.`,
          check,
        ],
      }
    case 'reviews section':
      return {
        what: `A review spec that ${t} reads as a section of its instructions file.`,
        why: `${t} has no separate review file, so the review guidance loads at the start of every session.`,
        actions: [
          `Keep each check in ${c.source} to one line and point to a rule for detail.`,
          'Limit the review spec with targets, or move it under the directory it checks.',
          check,
        ],
      }
    case 'always-on rule': {
      const settings = t === 'claude' ? 'outputs.claude.rules-mode: import or outputs.claude.rules-file' : `outputs.${t}.rules-file`
      const wholeCause = t === 'copilot' ? '' : ` ${t} also loads every rule whole when ${settings} is set, whatever the globs say.`
      return {
        what: `A rule that ${t} loads at the start of every session. It has alwaysApply: true, no globs, or a catch-all glob such as **/*.${wholeCause}`,
        why: 'An always-on rule applies to every file, so the tool reads it before any work starts.',
        actions: [
          `Narrow the globs in ${c.source} to the files it is for (not **/*) and remove alwaysApply: true.`,
          ...(t === 'copilot' ? [] : [`If it still loads at startup, remove ${settings} from agnostic-ai.yaml. Those settings load every rule whole, so globs change nothing.`]),
          'Shorten it: keep the rule, drop the explanation.',
          'Merge it into a related rule when both are short.',
          'Limit the rule with targets to the tools that need it.',
          check,
        ],
      }
    }
    case 'scoped rule':
      return {
        what: `A rule with globs that matches ${report.file || 'the file you named'}. ${t} loads it only while it works on a matching file.`,
        why: 'Scoped rules load with the file, not at startup, so it does not load for other files.',
        actions: [
          'Keep it if the file needs it. It does not load for other files.',
          `Narrow the globs in ${c.source} if it matches more files than it should.`,
          'Shorten the body.',
          check,
        ],
      }
    case 'conditional rule body': {
      const checks = report.takesFile !== false
      return {
        what: `A rule with globs. ${t} loads it only while it works on a matching file.`,
        why: checks ? 'It adds no startup text. Name a file above to see whether it matches.' : `It adds no startup text. This page cannot check a single file for ${t}, so it cannot show which files match.`,
        actions: [
          'Keep it. It loads only with matching files.',
          `Narrow the globs in ${c.source} or shorten the body if it matches often.`,
          ...(checks ? [`Run ${explainCommand(t)} --file <path> to see which files load it.`] : []),
        ],
      }
    }
    case 'generated framing':
      return {
        what: `Headings, source comments and notes that sync writes around your content in ${c.source}. It is not a spec you edit.`,
        why: `${t} reads the whole generated file, so its framing counts at startup.`,
        actions: [
          'Nothing to edit here. It shrinks when the sections it frames go away.',
          check,
        ],
      }
    default:
      return {
        what: `${c.category || 'A source'} from ${c.source}.`,
        why: `It loads ${LOAD_CONDITIONS[c.load]}.`,
        actions: [`Open ${c.source} and shorten what the tool does not need.`, check],
      }
  }
}

const MEMORY_OFF = 'Turning memory off removes shared memory for every tool: no tool reads or saves facts in .agnostic-ai/memory/ any more.'

function memoryIndexText(target: string, check: string): Text {
  return {
    what: `The shared memory index. Sync copies the index lines of .agnostic-ai/memory/ into the file ${target} reads first. The memory built-in owns it.`,
    why: 'The index loads at the start of every session in every tool. Each line points to a fact file that loads only when it is relevant.',
    actions: [
      'Prune index lines in .agnostic-ai/memory/MEMORY.md: delete facts the code or git history already shows, and merge lines about one topic.',
      'Keep each line to one short hook and put the detail in the fact file beside the index.',
      `Open Settings, Built-ins and turn memory off to remove shared memory. ${MEMORY_OFF}`,
      check,
    ],
  }
}

function builtinText(c: ContextContribution, report: AdviceReport, name: string, owner: string | undefined, base: Text): Text {
  const kind = kindOf(c.category)
  const who = owner ? `the ${owner} built-in` : 'a built-in'
  const replacement = projectPath(kind === 'other' || kind === 'generated' || kind === 'instructions' ? 'rule' : kind, name)
  const memoryNote = owner === 'memory' ? ` ${MEMORY_OFF}` : ''
  return {
    what: `${base.what} It ships with agnostic-ai as part of ${who}, so its text is not in this project.`,
    why: base.why,
    actions: [
      `Write ${replacement} with the same name and a shorter body. A project spec with the same kind and name replaces the built-in.`,
      `Open Settings, Built-ins and turn ${owner ?? 'the built-in'} off to remove every spec it adds. The next sync removes its files.${memoryNote}`,
      'To turn it off on your machine only, put the builtins list you keep in agnostic-ai.local.yaml. A local list replaces the shared one.',
      checkStep(report.target, report.file),
    ],
  }
}

export function contextAdvice(c: ContextContribution, report: AdviceReport, root?: string): ContextAdvice {
  const name = specName(c.source)
  const owner = c.source === MEMORY_INDEX ? 'memory' : builtinOwner(c.source)
  const base = describe(c, report, name)
  const text = isBuiltin(c.source) ? builtinText(c, report, name, owner, base) : base
  const share = sharePercent(c.words, loadTotal(report, c.load).words)
  const goal = c.load === 'startup' ? 'cuts its startup size' : 'cuts its size'
  const lines = [
    `Review one source of instruction text that agnostic-ai explain --context ranked for ${report.target}:`,
    '',
    `Source: ${c.source}`,
    `Kind: ${c.category}`,
    `Loads: ${LOAD_CONDITIONS[c.load]} (${c.load})`,
    `Size: ${sizeText(c)} (${share}% of ${LOAD_LABELS[c.load].toLowerCase()} content for ${report.target})`,
  ]
  if (root) lines.push(`Configuration root: ${root}`)
  if (isBuiltin(c.source)) lines.push(`Built-in: ${owner ?? 'unknown'} (ships with agnostic-ai; its text is not in the project)`)
  lines.push('', `What it is: ${text.what}`, `Why it loads: ${text.why}`, 'Options:', ...text.actions.map((a) => `- ${a}`))
  lines.push(
    '',
    `Read the source and propose the smallest safe change that ${goal} without losing instructions the team relies on. State the tradeoffs of each option, such as a tool that would stop loading a rule, or shared memory that would stop working. Edit the canonical specs under .agnostic-ai/, not generated files. Do not apply any change until I agree.`,
    '',
    `After the change, run agnostic-ai sync, then ${explainCommand(report.target, report.takesFile === false ? undefined : report.file)} to check the new counts. Words and bytes measure text size, not tokens; do not claim lower latency or cost.`,
  )
  return { ...text, prompt: lines.join('\n'), builtin: owner }
}

/** The startup sources worth reading first: largest by words, skipping generated framing. */
export function startHere(contributions: ContextContribution[], limit = 3): ContextContribution[] {
  return contributions
    .filter((c) => c.load === 'startup' && c.category !== 'generated framing')
    .sort((a, b) => b.words - a.words || a.source.localeCompare(b.source))
    .slice(0, limit)
}
