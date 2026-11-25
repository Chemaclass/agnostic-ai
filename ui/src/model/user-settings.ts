import menu from '../../../companion/frontend/menu.json'
import type { ApprovalsMode, CompanionSwitch, SettingKey, SettingsSyncPlan, SettingsSyncReply, SettingsView } from '../contract'

export interface ModeChoice {
  value: ApprovalsMode
  label: string
  /** What the choice does, under its label. */
  hint: string
}

export const COMPANION_SECTIONS = menu.sections

export function companionMenuItem(id: string) {
  const item = menu.items.find((item) => item.id === id)
  if (!item) throw new Error(`The companion menu has no item ${id}`)
  return item
}

export function companionShortcutHelp(isMac: boolean) {
  return menu.shortcutHelp.map((row) => ({ ...row, keys: row.id === 'paste' ? (isMac ? ['⌘', '⇧', 'V'] : ['Ctrl', 'Shift', 'V']) : row.keys }))
}

function modeChoice(choice: { value: string; label: string }): ModeChoice {
  switch (choice.value) {
    case 'ask': return { ...choice, value: 'ask', hint: 'Every permission prompt reaches you.' }
    case 'review': return { ...choice, value: 'review', hint: "The tool's own reviewer answers routine prompts." }
    case 'yolo': return { ...choice, value: 'yolo', hint: 'agnostic-ai approves routine calls. Risky ones still ask.' }
    default: throw new Error(`The companion menu has an unknown approvals mode ${choice.value}`)
  }
}

export const MODE_CHOICES: ModeChoice[] = (companionMenuItem('approvalsMode').choices ?? []).map(modeChoice)

/** The line a locked control shows. */
export const LOCKED_HINT = 'To change this here, run agnostic-ai view --scope user again, or use the companion.'

/** Whether setting key to value makes approvals less strict, which needs the session key. */
export function loosens(key: SettingKey, value: string | boolean): boolean {
  if (key === 'companion.checkForUpdates') return value === false
  if (key.startsWith('companion.')) return false
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

type AppearanceSwitch = Exclude<CompanionSwitch, 'checkForUpdates'>

const appearanceSwitch = (id: string): id is AppearanceSwitch => id === 'animate' || id === 'dimWhenIdle' || id === 'smallCompanion'

export const COMPANION_SWITCHES = menu.items.flatMap(({ id, label }) => appearanceSwitch(id) ? [{ key: id, label }] : [])

export const COMPANION_APPROVALS_LABEL = companionMenuItem('approvalsCompanion').label
export const COMPANION_APPROVALS_HINT = companionMenuItem('approvalsCompanion').hint ?? ''
export const APPROVALS_SETUP_HINT = 'Setup changes your tools’ settings files, so you can review the changes first.'

export function companionApprovalsNeedSetup(doc: SettingsView): boolean {
  return doc.approvals.companion && Boolean(doc.approvals.problem || doc.approvals.companionUnavailable || Object.keys(doc.approvals.companionSetup ?? {}).length || doc.sync.reasons.some((reason) =>
    /^(claude|codex): the hook is not installed/.test(reason) || reason === 'approvals is not in builtins in the home config'))
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
  if (approvals.problem) lines.push('The approvals settings could not be read. Check your home configuration.')
  if (doc.sync.needed) lines.push(approvalSetupLine(doc.sync.reasons, toolName))
  if (approvals.companionUnavailable) lines.push('Add Claude Code or Codex to your configured tools to answer requests from the companion.')
  for (const [tool, setup] of Object.entries(approvals.companionSetup ?? {})) {
    if (setup === 'trust') lines.push('Open /hooks in Codex and trust the permission entry.')
    if (setup === 'disabled') lines.push('Open /hooks in Codex and enable the permission entry.')
    if (setup === 'reviewer') lines.push('Review mode turns off companion answers for Codex. Set Codex to Ask me to answer from the companion.')
    if (setup === 'unavailable') lines.push('Check permissions in ' + toolName(tool) + '’s settings before answering from the companion.')
  }
  const { tools, projects, alwaysAsk } = approvals.overrides
  if (tools || projects) {
    const parts = [tools ? counted(tools, 'tool', 'tools') : '', projects ? counted(projects, 'folder', 'folders') : ''].filter(Boolean)
    lines.push(`${parts.join(' and ')} ${tools + projects === 1 ? 'has its' : 'have their'} own mode. Edit them in the home config.`)
  }
  if (alwaysAsk) lines.push(`${counted(alwaysAsk, 'extra pattern always asks', 'extra patterns always ask')}.`)
  if (approvals.shadowed.length) lines.push(`Your local home config hides ${approvals.shadowed.join(', ')} from the shared one.`)
  return lines
}

function approvalSetupLine(reasons: string[], toolName: (target: string) => string): string {
  const missing = reasons.filter((reason) => reason.split(': ')[1]?.startsWith('the hook is not installed'))
    .map((reason) => toolName(reason.split(': ')[0]))
  if (missing.length === 0 || missing.length !== reasons.length) return 'Approval changes still need setup.'
  const tools = new Intl.ListFormat('en', { type: 'conjunction' }).format(missing)
  return `${tools} ${missing.length === 1 ? 'needs' : 'need'} setup.`
}

/** What the sync dialog shows before it runs. */
export function syncConfirm(plan: SettingsSyncPlan): { title: string; body: string; files: string[]; commands: string[]; notes: string[]; go?: string } {
  const files = plan.files.map((file) => `${file.change} ${file.path}`)
  if (files.length === 0) return { title: 'Nothing to apply', body: 'Your tools already match your user configuration.', files, commands: [], notes: plan.notes }
  return { title: 'Finish setup?', body: 'Review all pending changes to your tool settings, including approvals, before applying them.', files, commands: plan.commands ?? [], notes: plan.notes, go: 'Apply' }
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
