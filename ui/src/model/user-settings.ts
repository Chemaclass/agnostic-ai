import type { ApprovalsMode, SettingKey, SettingsSyncPlan, SettingsSyncReply, SettingsView } from '../contract'

export interface ModeChoice {
  value: ApprovalsMode
  label: string
  /** What the choice does, under its label. */
  hint: string
}

export const MODE_CHOICES: ModeChoice[] = [
  { value: 'ask', label: 'Ask me', hint: 'Every permission prompt reaches you.' },
  { value: 'review', label: "Let the tool's reviewer decide", hint: "The tool's own reviewer answers routine prompts." },
  { value: 'yolo', label: 'Approve routine calls for me', hint: 'The approvals hook approves routine calls. Risky ones still ask.' },
]

/** The line a locked control shows. */
export const LOCKED_HINT = 'To change this here, run agnostic-ai view --scope user again, or use the companion.'

/** Whether setting key to value makes approvals less strict, which needs the session key. */
export function loosens(key: SettingKey, value: string | boolean): boolean {
  if (key === 'approvals.mode') return value !== 'ask'
  if (key === 'approvals.companion') return value === true
  return value === false
}

/** Whether the page may offer this change: tightening always, loosening only when unlocked. */
export function canChange(doc: Pick<SettingsView, 'canLoosen'>, key: SettingKey, value: string | boolean): boolean {
  return doc.canLoosen || !loosens(key, value)
}

/** The question a looser mode asks before it saves; Ask me saves at once. */
export function modeConfirm(mode: string): { title: string; body: string; go: string } | undefined {
  if (mode === 'review') return { title: "Let the tool's reviewer decide?", body: "The tool's own reviewer answers routine permission prompts for you. Prompts it cannot judge still reach you.", go: 'Turn on' }
  if (mode === 'yolo') return { title: 'Approve routine calls for you?', body: 'The approvals hook approves routine calls one at a time. Risky ones, such as publishing or deleting, still ask.', go: 'Turn on' }
  return undefined
}

const counted = (n: number, one: string, many: string) => `${n} ${n === 1 ? one : many}`

/**
 * The status lines under the approvals controls: why the mode is not in
 * effect yet, what has its own mode, what a local block hides, and a
 * block that does not load.
 */
export function approvalsStatus(doc: SettingsView, toolName: (target: string) => string): string[] {
  const lines: string[] = []
  const approvals = doc.approvals
  if (approvals.problem) lines.push(`The approvals block does not load: ${approvals.problem}`)
  if (doc.sync.needed) lines.push(`Not in effect yet: ${syncReasons(doc.sync.reasons, toolName)}.`)
  const { tools, projects, alwaysAsk } = approvals.overrides
  if (tools || projects) {
    const parts = [tools ? counted(tools, 'tool', 'tools') : '', projects ? counted(projects, 'folder', 'folders') : ''].filter(Boolean)
    lines.push(`${parts.join(' and ')} ${tools + projects === 1 ? 'has its' : 'have their'} own mode. Edit them in the home config.`)
  }
  if (alwaysAsk) lines.push(`${counted(alwaysAsk, 'extra pattern always asks', 'extra patterns always ask')}.`)
  if (approvals.shadowed.length) lines.push(`Your local home config hides ${approvals.shadowed.join(', ')} from the shared one.`)
  return lines
}

/** The reasons a sync fixes, short: tools with the same reason share one clause, and the advice after a semicolon is what Apply does. */
function syncReasons(reasons: string[], toolName: (target: string) => string): string {
  const byReason = new Map<string, string[]>()
  const clauses: string[] = []
  for (const reason of reasons) {
    const match = /^([a-z-]+): ([^;]*)/.exec(reason)
    if (!match) {
      clauses.push(reason)
      continue
    }
    const [, tool, text] = match
    if (!byReason.has(text)) {
      byReason.set(text, [])
      clauses.push(text)
    }
    byReason.get(text)!.push(toolName(tool))
  }
  const list = new Intl.ListFormat('en', { type: 'conjunction' })
  return clauses.map((clause) => (byReason.has(clause) ? `${clause} for ${list.format(byReason.get(clause)!)}` : clause)).join('; ')
}

/** What the sync dialog shows before it runs. */
export function syncConfirm(plan: SettingsSyncPlan): { title: string; body: string; files: string[]; notes: string[]; go?: string } {
  const files = plan.files.map((file) => `${file.change} ${file.path}`)
  if (files.length === 0) return { title: 'Nothing to apply', body: 'Your tools already match your user configuration.', files, notes: plan.notes }
  return { title: 'Apply to my tools?', body: 'This applies all pending changes of your user configuration, not only approvals:', files, notes: plan.notes, go: 'Apply' }
}

/** What the page says after a sync. */
export function syncResult(reply: SettingsSyncReply): string {
  const written = reply.written.length
  return [written === 0 ? 'Nothing needed writing.' : `Applied: ${counted(written, 'file', 'files')} written.`, ...reply.notes].join(' ')
}

/** What the page says when a sync failed. */
export function syncFailure(message: string, command?: string): string {
  return command ? `${message} Run ${command} in a terminal.` : message
}
