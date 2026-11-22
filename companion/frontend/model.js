// What the list shows, worked out from the rows the shell sends. No DOM here.

/**
 * The groups, in the order the list shows them. Archived starts folded: it
 * holds what the person archived and what went quiet by itself.
 */
export const GROUPS = [
  { id: 'needs-you', title: 'Needs you', startsFolded: false },
  { id: 'working', title: 'Working', startsFolded: false },
  { id: 'active', title: 'Active', startsFolded: false },
  { id: 'archived', title: 'Archived', startsFolded: true },
]

/** Where conversations run, as the switch beside the search offers them. */
export const PLACES = [
  { id: 'all', label: 'All', title: 'Every conversation' },
  { id: 'local', label: 'Local', title: 'Conversations on this machine' },
  { id: 'cloud', label: 'Cloud', title: 'Conversations that run in a tool\'s cloud' },
]

/** The apps whose conversations live online, not on this machine: a tool's cloud and plain chats. */
const ONLINE_APPS = new Set(['cloud', 'chat'])

/** Whether a conversation runs in `place`. */
const IN_PLACE = {
  all: () => true,
  local: (conversation) => !ONLINE_APPS.has(conversation.app),
  cloud: (conversation) => ONLINE_APPS.has(conversation.app) || conversation.cloud !== undefined,
}

const NEEDS_PERSON = { blocked: true, unread: true }

/**
 * The view narrowed to one place: its rows, how many of them need the
 * person and, in a search, how many match. `all` is the view as it came,
 * with the server's own counts, which also cover weeks the list has not read.
 */
export function viewIn(view, place) {
  if (place === 'all') return view
  const conversations = view.conversations.filter(IN_PLACE[place])
  const narrowed = { ...view, conversations, needsYou: conversations.filter((conversation) => NEEDS_PERSON[conversation.state]).length }
  if (view.matches !== undefined) narrowed.matches = conversations.length
  return narrowed
}

/** The group each state belongs to. */
const GROUP_OF_STATE = {
  blocked: 'needs-you',
  unread: 'needs-you',
  working: 'working',
  active: 'active',
  completed: 'archived',
  dormant: 'archived',
}

const TOOL_LABELS = {
  claude: 'Claude Code',
  codex: 'Codex',
  cursor: 'Cursor',
  chatgpt: 'ChatGPT',
}

/** The agent's latest reply as the server sent it, or '' when it sent none. */
export function replyOf(conversation) {
  return typeof conversation.reply === 'string' ? conversation.reply.trim() : ''
}

/** The title a row shows; a chat whose tool gave none, or whose title may hold a credential, gets a neutral label. */
export function titleOf(conversation) {
  return conversation.title === '' ? 'Untitled conversation' : conversation.title
}

/** Ids are unique within a tool, not across tools. */
export function keyOf(conversation) {
  return conversation.tool + ':' + conversation.id
}

/** A typed name as one line, or undefined when nothing but spaces was typed. The server makes the other checks. */
export function nameOf(typed) {
  const name = typed.split(/\s+/).filter((word) => word !== '').join(' ')
  return name === '' ? undefined : name
}

/**
 * What Enter does with a typed name: `{ keep: true }` when it is the title the row already shows,
 * which may be cut short, so nothing is sent; `{ empty: true }` when nothing was typed; otherwise `{ title }`.
 */
export function renameOf(draft, shown) {
  const title = nameOf(draft)
  if (title === undefined) return { empty: true }
  if (title === shown) return { keep: true }
  return { title }
}

/** Whether a key press only confirms an input method's candidate, such as a Japanese conversion. */
export function confirmsCandidate(press) {
  return press.isComposing === true || press.keyCode === 229
}

export function escapeTarget({ quitConfirmOpen, updateConfirmOpen, deleteConfirmOpen, stopShellOpen, stopSessionOpen, rowMenuOpen, renaming, settingsOpen, picking, mailboxOpen, searching }) {
  if (quitConfirmOpen) return 'quit-confirm'
  if (updateConfirmOpen) return 'update-confirm'
  if (deleteConfirmOpen) return 'delete-confirm'
  if (stopShellOpen) return 'stop-shell'
  if (stopSessionOpen) return 'stop-session'
  if (rowMenuOpen) return 'row-menu'
  if (renaming) return 'rename'
  if (settingsOpen) return 'settings'
  if (picking) return 'pick'
  if (mailboxOpen) return 'mailbox'
  if (searching) return 'search'
  return 'panel'
}

/** What pick mode says when the clipboard holds no text. */
export const NOTHING_TO_SEND = 'Nothing to send: copy some text first.'

/** What pick mode says when the clipboard could not be read. */
export function unreadableClipOf(error) {
  return `Could not read the copied text. ${error.message}`
}

/** The clipboard as pick mode uses it, or undefined when it holds no text to send, such as only an image or a file. */
export function pickableClip(clip) {
  return clip && clip.words > 0 && typeof clip.hash === 'string' && clip.hash !== '' ? clip : undefined
}

/** The pick banner's title: what a click or Enter does. Only the server knows whether copied HTML goes as Markdown, and its reply says so. */
export function pickTitleOf(clip) {
  const words = clip.words === 1 ? 'this word' : `these ${clip.words} words`
  return `Click a chat or press Enter to paste ${words} there`
}

/** The first words of the copied text, for the chip that flies into the picked row. */
export function chipTextOf(clip, words = 4) {
  const all = clip.preview.split(/\s+/).filter((word) => word !== '')
  return all.slice(0, words).join(' ') + (all.length > words ? '…' : '')
}

/** Whether a row can take the copied text. An archived chat cannot; the server decides the rest when it sends. */
export function takesText(conversation) {
  return GROUP_OF_STATE[conversation.state] !== 'archived'
}

/** The apps whose tabs the server can type into, as a live row's route names them: "Switch to Ghostty". */
const TYPABLE_ROUTES = new Set(['Switch to Terminal', 'Switch to Ghostty', 'Switch to iTerm', 'Switch to WezTerm', 'Switch to kitty'])
/** The tools the server types into: they say when they wait for an answer. */
const TYPED_TOOLS = new Set(['claude', 'codex'])

/**
 * The rows the copied text can be typed into, in list order: live, in a Terminal, Ghostty, iTerm, WezTerm or kitty tab, under
 * Needs you, Working or Active. Like the server, it leaves out a chat that waits for an answer, where typed text
 * could answer it, a Cursor chat, which does not say when it waits, and a working chat whose tool may drop typed text:
 * only Claude Code keeps it.
 * Other rows can still be picked; the server then switches to them to paste by hand.
 */
export function typableRows(conversations) {
  return conversations.filter((conversation) => {
    if (conversation.live !== true || conversation.open?.kind !== 'focus' || !TYPABLE_ROUTES.has(conversation.open.label) || !takesText(conversation)) return false
    if (!TYPED_TOOLS.has(conversation.tool) || conversation.state === 'blocked' || conversation.activity === 'waiting') return false
    const working = conversation.state === 'working' || conversation.activity === 'working'
    return !working || conversation.tool === 'claude'
  })
}

/**
 * The row the send shortcut sends to without asking: the only one the text can be typed into, else undefined.
 * With `shown`, the keys of the rows on screen, a row hidden in a folded group is not sent to.
 */
export function straightSendOf(conversations, shown) {
  const rows = typableRows(conversations)
  if (rows.length !== 1) return undefined
  return shown === undefined || shown.includes(keyOf(rows[0])) ? rows[0] : undefined
}

/** The pick banner's title during a straight send: "Sending to Plan the sync dry run". */
export function sendingTitleOf(conversation) {
  return `Sending to ${titleOf(conversation)}`
}

/** The view pick mode shows: only the rows that can take the text, so no Archived, counted again for the foot. */
export function pickViewOf(view) {
  const conversations = view.conversations.filter(takesText)
  const offered = { ...view, conversations, needsYou: conversations.filter((conversation) => NEEDS_PERSON[conversation.state]).length }
  if (view.matches !== undefined) offered.matches = conversations.length
  return offered
}

/** The row pick mode selects among `keys`: the selected one while offered, else the first in `typable`, else the first. */
export function pickKeyOf(keys, typable, active) {
  if (keys.includes(active)) return active
  return keys.find((key) => typable.includes(key)) ?? keys[0]
}

/** Cmd+Shift+V, or Ctrl+Shift+V off a Mac: enters or leaves pick mode while the list is open. */
export function togglesPick(press, isMac) {
  const command = isMac ? press.metaKey === true && !press.ctrlKey : press.ctrlKey === true && !press.metaKey
  return command && press.shiftKey === true && !press.altKey && press.code === 'KeyV' && !press.repeat
}

/** F2 on the selected row starts a rename, as in a file manager. */
export function startsRename(event) {
  return event.key === 'F2' && !event.repeat && !event.metaKey && !event.ctrlKey && !event.altKey && !event.shiftKey
}

/** What a failed open leaves on screen: the reason, and the command to resume by hand only when the server sent one. */
export function openFailureOf(reply) {
  if (reply.opened) return undefined
  return reply.command ? { message: reply.message, command: reply.command } : { message: reply.message }
}

/**
 * The notice for an open that failed after the companion stopped waiting for it, named after its chat when the
 * companion knows it; undefined for the failure `dismissed` names, which a read already on its way may still return.
 * `late` keeps it on screen when the list opens to send copied text, and `id` dismisses it.
 */
export function lateOpenFailureOf(failure, dismissed) {
  if (failure.id === dismissed) return undefined
  const message = failure.title ? failure.title + ': ' + failure.message : failure.message
  const shown = { message, late: true, id: failure.id }
  return failure.command ? { ...shown, command: failure.command } : shown
}

/**
 * What the notice card can be about. `label` names it for a screen reader. `tone` is `error` when something
 * the person asked for did not happen, and `info` when it happened and a detail is left to say.
 */
export const NOTICES = {
  notOpened: { label: 'Why it did not open', tone: 'error' },
  textChanged: { label: 'Why nothing was sent', tone: 'error' },
  notTyped: { label: 'Why nothing was typed', tone: 'error' },
  pasteByHand: { label: 'What to do with the copied text', tone: 'info' },
  cannotDelete: { label: 'Why it cannot be deleted', tone: 'error' },
  notDeleted: { label: 'Why it was not deleted', tone: 'error' },
  notTidied: { label: 'What was not tidied', tone: 'info' },
  shellNotStopped: { label: 'Why the shell could not be stopped', tone: 'error' },
  shellProcessesLeft: { label: 'Processes still running', tone: 'info' },
  sessionNotStopped: { label: 'Why the chat could not be stopped', tone: 'error' },
  subagent: { label: 'How to reach the subagent', tone: 'info' },
}

/** How a screen reader takes a message of this tone: an error at once, anything else when it is free. */
export function roleOfTone(tone) {
  return tone === 'error' ? 'alert' : 'status'
}

/** How the server ends its answer when it brought the conversation forward for a paste by hand; see pasteByHand in send.go. */
export const SWITCHED_TO_PASTE = 'Switched to it. Paste with Cmd+V.'

/**
 * The notice for a send that typed nothing, from what the server said. Switched to the conversation, the person
 * only has to paste. Any other answer is an error, and so is one this sentence no longer matches.
 */
export function unsentNoticeOf(message) {
  return typeof message === 'string' && message.endsWith(SWITCHED_TO_PASTE) ? NOTICES.pasteByHand : NOTICES.notTyped
}

/**
 * The rows of each group that has any, in list order, except that a conversation started from another
 * one in the same group comes right after it. `under` maps such a row's key to the row it sits under.
 */
export function grouped(conversations) {
  return GROUPS.map((group) => {
    const rows = conversations.filter((conversation) => GROUP_OF_STATE[conversation.state] === group.id)
    const drawn = nested(rows)
    return { ...group, ...(group.id === 'needs-you' ? mostUrgentFirst(drawn) : drawn) }
  }).filter((group) => group.rows.length > 0)
}

/** Under Needs you: a permission to give first, then another wait such as a question, then an unread reply. */
function urgencyOf(conversation) {
  if (conversation.state !== 'blocked') return 2
  return conversation.waiting?.for === 'permission' ? 0 : 1
}

/** Each family, a row with the rows drawn under it, ranked by its most urgent row, in list order within a rank. */
function mostUrgentFirst({ rows, under }) {
  const families = []
  for (const conversation of rows) {
    if (!under.has(keyOf(conversation))) families.push({ rows: [], urgency: Infinity })
    const family = families[families.length - 1]
    family.rows.push(conversation)
    family.urgency = Math.min(family.urgency, urgencyOf(conversation))
  }
  families.sort((a, b) => a.urgency - b.urgency)
  return { rows: families.flatMap((family) => family.rows), under }
}

/** What the circle counts: the rows that wait for the person to act, and the rest of the server's count, the unread replies. */
export function waitingOf(view) {
  const blocked = view.conversations.filter((conversation) => conversation.state === 'blocked').length
  return { blocked, unread: view.needsYou - blocked }
}

/** What the circle tells a screen reader, such as "Conversations: 2 waiting for you, 4 unread". */
export function circleLabelOf({ blocked, unread }) {
  const counts = []
  if (blocked > 0) counts.push(`${blocked} waiting for you`)
  if (unread > 0) counts.push(`${unread} unread`)
  return 'Conversations: ' + (counts.length === 0 ? 'none needs you' : counts.join(', '))
}

// A walk that meets a loop gives no top, so every row on it stays a normal row.
const LOOP = Symbol('loop')

/**
 * The top of each row's family in `rows`: the last of its parents, walking up, that is in `rows`,
 * or the row itself. Each walk stops at a row already worked out, so the whole list costs one pass.
 */
function topsOf(rows) {
  const tops = new Map()
  return (conversation) => {
    const path = []
    const onPath = new Set()
    let at = conversation
    let top
    for (;;) {
      const key = keyOf(at)
      if (tops.has(key)) {
        top = tops.get(key)
        break
      }
      path.push(key)
      onPath.add(key)
      const above = at.parent && rows.get(at.parent.tool + ':' + at.parent.id)
      if (!above) {
        top = at
        break
      }
      if (onPath.has(keyOf(above))) {
        top = LOOP
        break
      }
      at = above
    }
    for (const key of path) tops.set(key, top)
    return top
  }
}

/** When a child started: startedAt, else its last write. A time that does not parse counts as the oldest. */
function startedAt(conversation) {
  const time = Date.parse(conversation.startedAt ?? conversation.updatedAt)
  return Number.isNaN(time) ? -Infinity : time
}

/** Oldest start first, then by tool and id, so a new message moves no child. Each time is read once. */
function byStart(family) {
  return family
    .map((conversation) => ({ conversation, time: startedAt(conversation), key: keyOf(conversation) }))
    .sort((a, b) => (a.time !== b.time ? (a.time < b.time ? -1 : 1) : a.key === b.key ? 0 : a.key < b.key ? -1 : 1))
    .map((entry) => entry.conversation)
}

/**
 * One group's rows as the list draws them: every descendant right after the root it sits under,
 * oldest first, one level deep. The first row with a tool and id owns its children, so as many rows come out as go in.
 */
function nested(rows) {
  const byKey = new Map()
  for (const conversation of rows) {
    if (!byKey.has(keyOf(conversation))) byKey.set(keyOf(conversation), conversation)
  }
  const topOf = topsOf(byKey)
  const under = new Map()
  const children = new Map()
  for (const conversation of rows) {
    const top = topOf(conversation)
    if (top === LOOP || keyOf(top) === keyOf(conversation)) continue
    under.set(keyOf(conversation), top)
    const family = children.get(top)
    if (family) family.push(conversation)
    else children.set(top, [conversation])
  }
  const drawn = []
  for (const conversation of rows) {
    if (under.has(keyOf(conversation))) continue
    drawn.push(conversation)
    for (const child of byStart(children.get(conversation) ?? [])) drawn.push(child)
  }
  return { rows: drawn, under }
}

/** What a nested row tells a screen reader about the row it sits under. */
export function startedFromOf(parent) {
  return 'Started from ' + titleOf(parent)
}

/**
 * Whether a row shown under Needs you, or one with an unread marker, may still be marked read: the latest
 * list holds it unchanged and still needing the person or still unread. A row that got a new message since it was drawn holds something
 * the person has not seen, so it stays. So does a row the latest list does not hold, or no list:
 * a new message can take a row out of a search, so what is missing may be what changed.
 */
export function stillWaits(view, shown) {
  const key = keyOf(shown)
  const current = view?.conversations.find((conversation) => keyOf(conversation) === key)
  return current !== undefined && (Boolean(NEEDS_PERSON[current.state]) || unreadMarkOf(current)) && current.updatedAt === shown.updatedAt
}

/**
 * Whether a row shown under Active may still be marked complete: the latest list holds it unchanged and still active.
 * A row that got a reply or started working since it was drawn holds something the person has not seen, so it stays.
 */
export function stillActive(view, shown) {
  const key = keyOf(shown)
  const current = view?.conversations.find((conversation) => keyOf(conversation) === key)
  return current !== undefined && current.state === 'active' && current.updatedAt === shown.updatedAt
}

/**
 * A wait the server read from silence carries `quietSince`: "No activity for 7m" says how long nothing came,
 * not that the agent asked. Undefined for any other row. The row's tooltip says it; the meta line's age already shows it.
 */
export function quietNoteOf(conversation, now) {
  if (conversation.state !== 'blocked' || Number.isNaN(Date.parse(conversation.quietSince))) return undefined
  const quiet = ago(conversation.quietSince, now)
  return quiet === 'now' ? 'No activity' : 'No activity for ' + quiet
}

/** The word an approval puts before a tool's detail when the detail is a file: "Approve edit dashboard.tsx". */
const APPROVAL_VERBS = {
  Edit: 'edit',
  MultiEdit: 'edit',
  Write: 'edit',
  NotebookEdit: 'edit',
  Read: 'read',
}

/** How Claude Code names an MCP server's tool: mcp__<server>__<action>. The server sends its detail as "Notion fetch". */
const MCP_TOOL_PREFIX = 'mcp__'

const NEEDS_PERMISSION = { icon: 'lock', label: 'Needs permission', text: 'Needs permission' }

/**
 * The pill a row shows in place of its reply while a tool call waits for the person's approval:
 * `{ icon: 'key', label: 'Approve edit', code: 'dashboard.tsx', text }`, `{ icon: 'key', label: 'Approve Notion fetch', text }`
 * for an MCP tool, `{ icon: 'key', label: 'Approve Bash', text }` when the detail was withheld, or the lock's "Needs permission"
 * when the server could not say which call waits. `text` is the pill read as one line.
 * Undefined for a question or a wait of unknown kind: the reply line shows as it is.
 */
export function approvalOf(conversation) {
  const waiting = conversation.waiting
  if (waiting?.for !== 'permission') return undefined
  if (!waiting.tool) return NEEDS_PERMISSION
  if (waiting.tool.startsWith(MCP_TOOL_PREFIX)) {
    if (!waiting.detail) return NEEDS_PERMISSION
    const label = 'Approve ' + waiting.detail
    return { icon: 'key', label, text: label }
  }
  if (!waiting.detail) {
    const label = 'Approve ' + waiting.tool
    return { icon: 'key', label, text: label }
  }
  const verb = APPROVAL_VERBS[waiting.tool]
  const label = verb ? 'Approve ' + verb : 'Approve'
  return { icon: 'key', label, code: waiting.detail, text: label + ' ' + waiting.detail }
}

/**
 * What a row says about the requests the approvals hook approved for it: "3 approved for you".
 * Undefined when it approved none. The server counts them from its decision log.
 */
export function approvedLabelOf(conversation) {
  const count = conversation.approvals?.approved ?? 0
  return count > 0 ? `${count} approved for you` : undefined
}

/**
 * Why a permission wait reached the person, such as "Always asks: publishing" or "Approvals not supported for
 * Cursor". The server words it from the always-ask rule or the reason approvals did not answer, never from
 * what the session wrote. Undefined on a row that does not wait for a permission.
 */
export function approvalReasonOf(conversation) {
  const reason = conversation.approvals?.reason
  if (typeof reason !== 'string' || reason === '' || conversation.waiting?.for !== 'permission') return undefined
  return reason
}

/**
 * The approved requests a row folds out, newest first: `{ text, time }`, where text is "Bash go test ./..."
 * or the call alone when its detail was left out.
 */
export function approvedCallsOf(conversation) {
  return (conversation.approvals?.recent ?? []).map((call) => ({
    text: call.detail ? `${call.call} ${call.detail}` : call.call,
    time: call.time,
  }))
}

/** The toggle that folds a row's approved requests out: "▸ 3 approved for you", "▾ …" while unfolded. */
export function approvedToggleOf(conversation, open) {
  const label = approvedLabelOf(conversation)
  return label === undefined ? undefined : `${open ? '▾' : '▸'} ${label}`
}

/** The active approvals modes the header names. */
const APPROVALS_MODE_LABELS = {
  review: 'Approvals: your tools\' reviewers answer routine prompts',
  yolo: 'Approvals: routine prompts are approved for you',
}

/**
 * The header line while approvals answer prompts: `{ mode, label }`. Undefined when every prompt reaches
 * the person, as in ask. The Ask me button beside it turns approvals off.
 */
export function approvalsHeaderOf(view) {
  const label = APPROVALS_MODE_LABELS[view?.approvalsMode]
  return label === undefined ? undefined : { mode: view.approvalsMode, label }
}

/**
 * The row's tooltip, one fact a line: how it opens, its folder, how long a silent chat has said nothing,
 * the agent's reply when an approval pill takes the reply line's place, and why approvals left the wait to the person.
 */
export function rowTooltipOf(conversation, now) {
  const lines = [conversation.open.label]
  if (conversation.folder) lines.push(conversation.folder)
  const quiet = quietNoteOf(conversation, now)
  if (quiet) lines.push(quiet)
  const reply = replyOf(conversation)
  if (reply && approvalOf(conversation)) lines.push(reply)
  const reason = approvalReasonOf(conversation)
  if (reason) lines.push(reason)
  if (unreadMarkOf(conversation)) lines.push(RESUMED_UNREAD)
  return lines.join('\n')
}

/** Whether a row carries the unread marker. An archived chat has no read state, so it never does. */
export function unreadMarkOf(conversation) {
  return conversation.unread === true && GROUP_OF_STATE[conversation.state] !== 'archived'
}

/** What a group header adds for its rows with an unread marker, such as "2 unread"; undefined when none. */
export function unreadLabelOf(rows) {
  const count = rows.filter(unreadMarkOf).length
  return count > 0 ? `${count} unread` : undefined
}

/** What a resumed chat's marker says: it is open again, with a reply from before the person has not read. */
export const RESUMED_UNREAD = 'Resumed with a reply from before that you have not read'

/** An archived row offers Hide. Other rows can be marked complete when they have no completion mark and are not working. */
export function rowActionOf(conversation) {
  if (GROUP_OF_STATE[conversation.state] === 'archived') return 'hide'
  if (conversation.state === 'working' || conversation.completedAt) return undefined
  return 'archive'
}

/**
 * What the row menu offers: Mark as read clears a wait or a reply without opening the chat.
 * Mark complete and Hide match the row's own button. Only a chat under Archived that no process holds can be deleted.
 */
export function rowMenuOf(conversation) {
  const offered = NEEDS_PERSON[conversation.state] || unreadMarkOf(conversation) ? ['read', 'rename', 'copy-session-id'] : ['rename', 'copy-session-id']
  if (!conversation.parent && mailboxSessionOf(conversation)) offered.push('send-to', 'claim-files')
  const action = rowActionOf(conversation)
  if (action === 'archive') offered.push('complete')
  if (conversation.completedAt) offered.push('reopen')
  if (action === 'hide') offered.push('hide')
  if (GROUP_OF_STATE[conversation.state] === 'archived' && !conversation.live) offered.push('delete')
  if (conversation.cloud) offered.push('web')
  if (stuckShellsOf(conversation).length > 0) offered.push('stop-shell')
  if (stuckShellsOf(conversation).some((shell) => shell.keptUntil)) offered.push('ask-shell-again')
  if (conversation.live && conversation.stoppable) offered.push('stop-session')
  return offered
}

export function mailboxSessionOf(conversation) {
  return (conversation.tool === 'claude' || conversation.tool === 'codex') && !['cloud', 'chat', 'cowork'].includes(conversation.app)
}

export function threadsOf(conversation) {
  return conversation.threads ?? []
}

const RECEIVER_GROUP_LABELS = { archived: 'Archived · gets it when resumed' }

/** "Companion rebuild · Claude Code": a session named in the mailbox form. */
export function sessionLabelOf(conversation) {
  return `${titleOf(conversation)} · ${toolLabelOf(conversation)}`
}

/**
 * The sessions `from` can message, one group per list group, most recently updated first.
 * Each option reads "Title · Tool · folder"; options that read the same add their age, then the first 8 characters of their ID.
 */
export function receiversOf(conversations, from, now) {
  const fromKey = keyOf(from)
  const folders = folderNamesOf(conversations.map((conversation) => conversation.folder))
  const options = conversations
    .filter((conversation) => mailboxSessionOf(conversation) && keyOf(conversation) !== fromKey)
    .sort((a, b) => Date.parse(b.updatedAt) - Date.parse(a.updatedAt))
    .map((conversation) => {
      const parts = [titleOf(conversation), toolLabelOf(conversation)]
      if (conversation.folder) parts.push(folders.get(conversation.folder) ?? folderName(conversation.folder))
      return {
        value: keyOf(conversation),
        to: { tool: conversation.tool, id: conversation.id },
        title: titleOf(conversation),
        found: sessionLabelOf(conversation),
        group: GROUP_OF_STATE[conversation.state] ?? 'active',
        label: parts.join(' · '),
      }
    })
  const tellApart = (more) => {
    const counts = new Map()
    for (const option of options) counts.set(option.label, (counts.get(option.label) ?? 0) + 1)
    const shared = options.filter((option) => counts.get(option.label) > 1)
    for (const option of shared) option.label += ' · ' + more(option)
  }
  const updated = new Map(conversations.map((conversation) => [keyOf(conversation), conversation.updatedAt]))
  tellApart((option) => ago(updated.get(option.value), now))
  tellApart((option) => option.to.id.slice(0, 8))
  return GROUPS
    .map((group) => ({ id: group.id, label: RECEIVER_GROUP_LABELS[group.id] ?? group.title, options: options.filter((option) => option.group === group.id) }))
    .filter((group) => group.options.length > 0)
}

const TYPE_HELP = {
  question: (name) => `Ask ${name} something. Its reply comes back to this thread.`,
  finding: (name) => `Tell ${name} what you found. It can reply or close the thread.`,
  review_request: (name) => `Ask ${name} to review the changes in this chat's worktree.`,
  handoff: (name) => `Offer ${name} the files you claimed, so it can take them over.`,
}

/** The line under Type: what a message of that type asks of the receiver. */
export function typeHelpOf(type, name) {
  return TYPE_HELP[type]?.(name || 'the other session') ?? ''
}

const ARRIVAL = {
  'needs-you': 'with its next prompt',
  active: 'with its next prompt',
  working: 'with its next prompt, after its current work',
  archived: 'when it is resumed',
}

/** When the receiver reads the message, from its list group. Empty without a known receiver. */
export function arrivalOf(group, name) {
  if (!name || !ARRIVAL[group]) return ''
  return `This waits in the thread. With the mailbox built-in on, ${name} gets it ${ARRIVAL[group]}.`
}

/**
 * What a typed session ID points at among `receivers` (the flat options of receiversOf):
 * one receiver, none, two tools, or the sender itself.
 */
export function typedReceiverOf(receivers, from, typed) {
  const id = typed.trim()
  if (!id) return { status: 'empty' }
  if (id === from.id) return { status: 'sender', note: 'That is the sending session.' }
  const matches = receivers.filter((receiver) => receiver.to.id === id)
  if (matches.length > 1) return { status: 'two', note: 'Two sessions use this ID. Choose from the list.' }
  if (matches.length === 1) return { status: 'found', receiver: matches[0], note: `Found: ${matches[0].found}.` }
  return { status: 'unknown', note: 'No session in the list has this ID. It may not be loaded yet.' }
}

export function reviewModelsOf(from, to) {
  const sending = from?.trim()
  const reviewing = to?.trim()
  return {
    from: sending || 'model unknown',
    to: reviewing || 'model unknown',
    same: Boolean(sending && reviewing && sending.toLowerCase() === reviewing.toLowerCase()),
  }
}

export function handoffLabelOf(status) {
  return { offered: 'Offered', accepted: 'Accepted', declined: 'Declined' }[status]
}

/** Who answered a handoff, read from its accept or decline event. */
export function handoffAnswerOf(thread, message) {
  const answer = { accepted: 'accept', declined: 'decline' }[message.handoff]
  const event = (thread.events ?? []).findLast((e) => e.n === message.n && e.action === answer)
  if (!event) return handoffLabelOf(message.handoff)
  return `${handoffLabelOf(message.handoff)} by ${event.by === 'agent' ? 'the agent' : 'you'}`
}

export function senderKindOf(message) {
  return message.from?.by === 'agent' ? 'Sent by the agent' : undefined
}

/** The model an agent says it runs. The mailbox cannot check it. */
export function statedModelOf(thread, message) {
  if (message.model) return `Says it runs ${message.model}`
  const model = thread.models?.[keyOf(message.from)]
  return model?.statedBy === 'agent' && model.name ? `Says it runs ${model.name}` : undefined
}

export function claimStatusOf(claim) {
  return claim.stale ? 'Stale claim' : 'Active claim'
}

export function claimCountOf(count) {
  return `${count} ${count === 1 ? 'claim' : 'claims'}`
}

export function conflictLabelOf({ path }, holder) {
  return holder ? `Path conflict: ${path} is held by ${holder}.` : `Path conflict: no session holds ${path}.`
}

export function threadLabelOf({ count, cap, waiting, held }) {
  return `${count} of ${cap} messages${waiting ? `, ${waiting} waiting` : ''}${held ? `, ${held} held for you` : ''}`
}

export function messageStateLabelOf(state) {
  return {
    held: 'Held for you', waiting: 'Waiting', reserved: 'Delivering', delivered: 'Delivered',
    staged: 'Staged', rejected: 'Rejected', cancelled: 'Cancelled',
  }[state]
}

export function messageActionsOf({ state }, { status }) {
  switch (state) {
    case 'held':
      return ['approve', 'reject']
    case 'waiting':
      return status === 'paused' ? [] : ['type', 'copy']
    case 'reserved':
      return status === 'open' || status === 'closing' ? ['type', 'copy'] : []
    case 'staged':
      return status === 'open' || status === 'closing' ? ['queue'] : []
    default:
      return []
  }
}

export function threadActionsOf({ status, count, cap }) {
  switch (status) {
    case 'open':
      return ['reply', 'pause', 'close', ...(count >= cap ? ['raise-cap'] : []), 'stop']
    case 'paused':
      return ['reply', 'resume', 'stop']
    case 'closing':
      return ['stop']
    default:
      return []
  }
}

/** The question before Stop chat ends a running chat's process. */
export function stopSessionConfirmOf(conversation) {
  return {
    title: `Stop "${titleOf(conversation)}"?`,
    body: 'This ends the Claude Code process on this computer. Shell commands it started may keep running. The transcript stays, so you can resume later.',
  }
}

/** What the toast says once a chat's process was asked to end. */
export function stoppedSessionOf({ forced = false, stillRunning = false }) {
  if (stillRunning) {
    return {
      message: forced
        ? 'The Claude Code process is still running after a force stop.'
        : 'The Claude Code process is still running after the stop request.',
      problem: true,
    }
  }
  return { message: forced ? 'Stopped. It did not end when asked, so it was forced to.' : 'Stopped.' }
}

export function stuckShellsOf(conversation) {
  return conversation?.live ? conversation.stuckShells ?? [] : []
}

function shellAge(shell, now) {
  const age = ago(shell.since, now)
  return age === 'now' ? 'less than a minute' : age
}

export function shellCommandOf(shell) {
  return shell.command || 'Command hidden'
}

export function shellWarningOf(shell, now) {
  const owner = shell.helperName ? shell.helperName + ' left a shell' : 'Shell'
  return `${owner} running for ${shellAge(shell, now)}: ${shellCommandOf(shell)}`
}

export function currentShellOf(conversation, selected) {
  return stuckShellsOf(conversation).find((shell) =>
    shell.pid === selected.pid &&
    shell.procStart === selected.procStart &&
    shell.command === selected.command &&
    shell.since === selected.since &&
    shell.processCount === selected.processCount
  )
}

export function stopShellDetailsOf(shell, now) {
  const processes = shell.processCount === 1 ? '1 process' : `${shell.processCount} processes`
  return `Running for ${shellAge(shell, now)}. This stops ${processes}.`
}

export function stoppedShellOf({ stillRunning = [] }) {
  return stillRunning.length > 0
    ? `Shell stop sent. Processes still running (PIDs ${stillRunning.map(({ pid }) => pid).join(', ')}).`
    : 'Shell stopped.'
}

/** What the stop confirm adds while the agent works: the shell may still be part of its turn. */
export function stopShellWarningOf(conversation) {
  return conversation.state === 'working' ? 'The agent is still working and may need this shell.' : undefined
}

/** The icon in a waiting row's state mark: a terminal for a shell wait, so it reads apart from a permission wait without color. */
export function stateIconOf(conversation) {
  if (conversation.state !== 'blocked') return undefined
  return conversation.waiting?.for === 'shell' ? 'terminal' : 'lock'
}

/** How long Keep running holds a shell quiet after it was kept that many times before: 30 min, doubling to 4 h. */
export function keepWaitOf(keepsBefore) {
  return Math.min(30 * MINUTE * 2 ** keepsBefore, 4 * HOUR)
}

function keptRecordOf(kept, shell) {
  return kept.find((record) => record.pid === shell.pid && record.procStart === shell.procStart)
}

/** The record Keep running saves for a shell of a row: a wait one step longer than the last one. */
export function keepOf(conversation, shell, kept, now) {
  const keeps = keptRecordOf(kept, shell)?.keeps ?? 0
  return {
    tool: conversation.tool,
    id: conversation.id,
    pid: shell.pid,
    procStart: shell.procStart,
    wakeAt: new Date(now + keepWaitOf(keeps)).toISOString(),
    keeps: keeps + 1,
  }
}

function waitText(ms) {
  const minutes = Math.max(1, Math.ceil(ms / MINUTE))
  if (minutes < 60) return `${minutes} min`
  const hours = Math.floor(minutes / 60)
  const rest = minutes % 60
  return rest === 0 ? `${hours} h` : `${hours} h ${rest} min`
}

/** A kept shell's grey line, until its wake time. */
export function keptShellLineOf(shell, now) {
  return `Kept running · asks again in ${waitText(Date.parse(shell.keptUntil) - now)}: ${shellCommandOf(shell)}`
}

/**
 * The view as the person sees it with their kept shells: each stuck shell held quiet carries `keptUntil`, and a row
 * that waits only on such shells sits under Active, out of the Needs you count. A due record, or a shell with another
 * PID or start time, changes nothing. A working row stays working.
 */
export function keptViewOf(view, kept, now) {
  if (!view || kept.length === 0) return view
  let released = 0
  const conversations = view.conversations.map((conversation) => {
    const shells = stuckShellsOf(conversation)
    if (shells.length === 0) return conversation
    const marked = shells.map((shell) => {
      const record = keptRecordOf(kept, shell)
      return record && Date.parse(record.wakeAt) > now ? { ...shell, keptUntil: record.wakeAt } : shell
    })
    const quiet = { ...conversation, stuckShells: marked }
    if (conversation.state !== 'blocked' || conversation.waiting?.for !== 'shell' || !marked.every((shell) => shell.keptUntil)) return quiet
    released += 1
    const { waiting, ...active } = quiet
    return { ...active, state: 'active' }
  })
  return { ...view, conversations, needsYou: Math.max(0, view.needsYou - released) }
}

/** The records Ask about this shell again removes: those of the row's shells. */
export function keptShellsOfRow(conversation, kept) {
  return stuckShellsOf(conversation).map((shell) => keptRecordOf(kept, shell)).filter(Boolean)
}

/**
 * The records a full list shows are over: the shell is gone from a live chat that the server reads shells for, one
 * waiting on a shell or listing other shells, and that is not working: the server leaves out a shell it cannot tell
 * from the turn's own work. A chat that waits on a permission or a question, or is idle, lists no shells at all, so
 * its records stay until the companion drops them a day after their wake time.
 */
export function finishedKeepsOf(view, kept) {
  const readsShells = (conversation) => conversation.waiting?.for === 'shell' || stuckShellsOf(conversation).length > 0
  const settled = view.conversations.filter((conversation) => conversation.live && conversation.state !== 'working' && readsShells(conversation))
  const live = new Map(settled.map((conversation) => [keyOf(conversation), conversation]))
  return kept.filter((record) => {
    const conversation = live.get(keyOf(record))
    return conversation && !keptRecordOf(stuckShellsOf(conversation), record)
  })
}

/** What the open request names to show a shared row's claude.ai page; undefined on a row that is not shared. */
export function cloudPageOf(conversation) {
  return conversation.cloud ? { tool: conversation.tool, id: conversation.cloud.id, title: conversation.title } : undefined
}

/** What a shared row's marker and second action say. */
export const SHARED_TITLE = 'Also open on claude.ai'
export const CLOUD_PAGE_LABEL = 'Open on claude.ai'

/** What the server deletes for each tool it can delete, as the confirmation says it. */
const DELETED_DATA = {
  claude:
    'This removes the session from this computer: its transcript, its subagent transcripts, saved tool output, file backups and pasted images, and its name and marks in agnostic-ai. ' +
    "It cannot be undone. The prompts you typed stay in Claude Code's own prompt history.",
}

/**
 * What Delete permanently asks before it sends anything: `{ unsupported }` when the server said this chat
 * cannot be deleted from here, otherwise `{ title, details, body }`.
 */
export function deleteConfirmOf(conversation, now, names) {
  if (conversation.deleteUnsupported) return { unsupported: conversation.deleteUnsupported }
  return {
    title: `Delete "${titleOf(conversation)}" permanently?`,
    details: detailsOf(conversation, now, names),
    body: DELETED_DATA[conversation.tool] ?? 'This removes the conversation from this computer. It cannot be undone.',
  }
}

const SIZE_UNITS = ['KB', 'MB', 'GB']

/** "Freed 12 MB" after a delete; empty when the server freed nothing it could count. */
export function freedOf(bytes) {
  if (!Number.isFinite(bytes) || bytes <= 0) return ''
  if (bytes < 1000) return `Freed ${bytes} bytes`
  let size = bytes
  let unit = -1
  while (size >= 1000 && unit < SIZE_UNITS.length - 1) {
    size /= 1000
    unit += 1
  }
  const shown = size < 10 ? Math.round(size * 10) / 10 : Math.round(size)
  return `Freed ${shown} ${SIZE_UNITS[unit]}`
}

/** What the panel says once a chat is deleted: the bytes it freed, and what the server could not tidy. */
export function deletedOf(reply) {
  const freed = freedOf(reply.freedBytes)
  return { message: freed === '' ? 'Deleted.' : `Deleted. ${freed}.`, note: reply.note }
}

/** What a row is, said when it is not a coding session on this machine: one in a tool's cloud, a plain chat or a Cowork task. */
const PLACE_LABELS = {
  cloud: 'Cloud',
  chat: 'Chat',
  cowork: 'Cowork',
}

/** The product a chat or Cowork task belongs to, where it is not the tool's coding agent. */
const APP_TOOL_LABELS = {
  chat: { claude: 'Claude' },
  cowork: { claude: 'Claude' },
}

/** The tool a row names: "Claude" for a Claude chat, "Claude Code" for a Claude Code session. */
function toolLabelOf(conversation) {
  return APP_TOOL_LABELS[conversation.app]?.[conversation.tool] ?? TOOL_LABELS[conversation.tool]
}

/**
 * "Claude Code · agnostic-ai · 5m": the tool, where it runs when not here or Shared when also on claude.ai, the folder's name and how long ago it was written.
 * A chat reads "Claude · Chat · 2h".
 * `names` holds what folderNamesOf gave for the folders in view; without one the folder shows its last name.
 */
export function detailsOf(conversation, now, names = new Map()) {
  const parts = [toolLabelOf(conversation)]
  const place = conversation.cloud ? 'Shared' : PLACE_LABELS[conversation.app]
  if (place) parts.push(place)
  if (conversation.folder) parts.push(names.get(conversation.folder) ?? folderName(conversation.folder))
  parts.push(ago(conversation.updatedAt, now))
  return parts.join(' · ')
}

/** How many subagents show under a chat before "+N more". */
export const HELPERS_SHOWN = 5

/** Agent types that say nothing beyond "a subagent", so their meta line leaves them out. */
const PLAIN_KINDS = new Set(['general-purpose', 'default'])

/** The subagents a chat shows: only those still working. A finished subagent leaves the list and the count. */
export function workingHelpersOf(conversation) {
  return (conversation.helpers ?? []).filter((helper) => helper.state === 'working')
}

/** The toggle on the line under a chat's meta line: "▸ 3 subagents", "▾ 1 subagent" while unfolded. It counts working subagents only. */
export function helpersToggleOf(conversation, open) {
  const count = workingHelpersOf(conversation).length
  return `${open ? '▾' : '▸'} ${count} subagent${count === 1 ? '' : 's'}`
}

/** Newest first. */
function byAge(a, b) {
  return Date.parse(b.updatedAt) - Date.parse(a.updatedAt)
}

/**
 * The working subagents a chat shows unfolded, newest first, and how many more a "+N more" button hides:
 * HELPERS_SHOWN at most, or every one once the person asked for all.
 */
export function helperRowsOf(conversation, showAll) {
  const ordered = workingHelpersOf(conversation).sort(byAge)
  if (showAll || ordered.length <= HELPERS_SHOWN) return { rows: ordered, more: 0 }
  return { rows: ordered.slice(0, HELPERS_SHOWN), more: ordered.length - HELPERS_SHOWN }
}

/** A subagent's title; one whose tool gave none, or whose title may hold a credential, gets a neutral label. */
export function helperTitleOf(helper) {
  return helper.title === '' ? 'Untitled subagent' : helper.title
}

/** A subagent's latest message, the line under its title; empty before it wrote one. */
export function helperReplyOf(helper) {
  return replyOf(helper)
}

/** "Claude Code · Explore · 1m": the subagent's tool, its agent type when that says something, and its age. */
export function helperDetailsOf(helper, now) {
  const parts = [TOOL_LABELS[helper.tool]]
  if (helper.kind && !PLAIN_KINDS.has(helper.kind)) parts.push(helper.kind)
  parts.push(ago(helper.updatedAt, now))
  return parts.join(' · ')
}

/**
 * What a click on a subagent opens: a Codex thread opens itself, as a Codex chat does.
 * A Claude Code subagent runs inside its chat and has no place of its own, so its chat opens, naming the subagent.
 */
export function helperOpenOf(parent, helper) {
  if (helper.tool === 'codex') return { tool: 'codex', id: helper.id, title: helperTitleOf(helper) }
  return { tool: parent.tool, id: parent.id, ...(parent.title ? { title: parent.title } : {}), helper: helper.id }
}

export function helperTooltipOf(helper) {
  return helper.tool === 'codex' ? 'Open this Codex thread' : 'Open the chat it runs in, and show how to reach this subagent'
}

/**
 * The notice an opened Claude Code subagent leaves: which one it was and how to reach it; undefined for any other
 * open. It is a `hint`: any failure that comes after it takes its place.
 */
export function subagentNoticeOf(reply) {
  return reply.opened && reply.helper ? { message: reply.message, hint: true } : undefined
}

/** Whether a late open failure may take the notice's place: none shows, or only a hint does. */
export function lateFailureReplaces(shown) {
  return !shown || shown.hint === true
}

/** A folder as its names with the separators between them kept: ["Users", "/", "me", "/", "api"]. */
function piecesOf(folder) {
  const pieces = folder.replace(/[\\/]+$/, '').split(/([\\/]+)/)
  return pieces[0] === '' ? pieces.slice(2) : pieces
}

/** The last `count` names of a folder, written with its own separators. */
function tail(pieces, count) {
  return pieces.slice(-(2 * count - 1)).join('')
}

function folderName(folder) {
  const pieces = piecesOf(folder)
  return pieces.length === 0 ? folder : tail(pieces, 1)
}

/**
 * The name each folder shows: its last name, with as many parent folders as tell it apart
 * from the other folders in `folders` that end the same way, as in "shop/api" and "billing/api".
 */
export function folderNamesOf(folders) {
  const paths = new Map()
  for (const folder of folders) {
    const pieces = folder ? piecesOf(folder) : []
    if (pieces.length > 0) paths.set(pieces.join(''), pieces)
  }
  const shown = new Map([...paths.keys()].map((path) => [path, 1]))
  let growing = true
  while (growing) {
    growing = false
    const sharing = new Map()
    for (const [path, pieces] of paths) {
      const name = tail(pieces, shown.get(path))
      sharing.set(name, (sharing.get(name) ?? 0) + 1)
    }
    for (const [path, pieces] of paths) {
      const count = shown.get(path)
      if (2 * count - 1 < pieces.length && sharing.get(tail(pieces, count)) > 1) {
        shown.set(path, count + 1)
        growing = true
      }
    }
  }
  const result = new Map()
  for (const folder of folders) {
    const pieces = folder ? piecesOf(folder) : []
    if (pieces.length > 0) result.set(folder, tail(pieces, shown.get(pieces.join(''))))
  }
  return result
}

const MINUTE = 60 * 1000
const HOUR = 60 * MINUTE
const DAY = 24 * HOUR

/** A short age: now, 5m, 3h, 2d, 6w. */
export function ago(stamp, now) {
  const age = now - Date.parse(stamp)
  if (age < MINUTE) return 'now'
  if (age < HOUR) return Math.floor(age / MINUTE) + 'm'
  if (age < DAY) return Math.floor(age / HOUR) + 'h'
  if (age < 14 * DAY) return Math.floor(age / DAY) + 'd'
  return Math.floor(age / (7 * DAY)) + 'w'
}

/**
 * The view with a later page's rows after its own. A row both hold, which
 * the first page keeps for needing the person while a later read finds it
 * read, shows once: the later copy, read last.
 */
export function afterPage(view, later) {
  const again = new Set(later.conversations.map(keyOf))
  const kept = view.conversations.filter((conversation) => !again.has(keyOf(conversation)))
  return { ...view, next: later.next, conversations: [...kept, ...later.conversations] }
}

/** The line at the foot: how the search went. Empty without a search. */
export function summaryOf(view, query) {
  if (query !== '') {
    if (view.matches === 0) return 'No conversation matches'
    // More matched than the list reads: say so instead of showing fewer rows than the count.
    if (view.conversations.length < view.matches) return `${view.matches} conversations match; the newest ${view.conversations.length} show`
    return view.matches === 1 ? '1 conversation matches' : `${view.matches} conversations match`
  }
  // Without a search the foot says nothing: the Needs you group already counts.
  return ''
}

/** The row after or before `key` among `keys`; the first row when `key` is not listed. step is 1 or -1. */
export function neighbour(keys, key, step) {
  if (keys.length === 0) return undefined
  const at = keys.indexOf(key)
  if (at === -1) return keys[0]
  return keys[Math.min(Math.max(at + step, 0), keys.length - 1)]
}

/** How far, in pixels, a press must travel before it drags the bubble. Less is a click with an unsteady hand. */
export const DRAG_DISTANCE = 4

/** Whether the pointer moved far enough from where it was pressed to be dragging. */
export function isDrag(pressed, pointer) {
  return Math.hypot(pointer.x - pressed.x, pointer.y - pressed.y) >= DRAG_DISTANCE
}

/** The key names Wails takes in a shortcut, by the browser's name for the physical key. Letters and digits are handled apart. */
const SHORTCUT_KEYS = {
  Space: 'Space',
  Enter: 'Enter',
  Tab: 'Tab',
  Backspace: 'Backspace',
  Delete: 'Delete',
  Home: 'Home',
  End: 'End',
  ArrowLeft: 'Left',
  ArrowRight: 'Right',
  ArrowUp: 'Up',
  ArrowDown: 'Down',
  F1: 'F1', F2: 'F2', F3: 'F3', F4: 'F4', F5: 'F5', F6: 'F6',
  F7: 'F7', F8: 'F8', F9: 'F9', F10: 'F10', F11: 'F11', F12: 'F12',
}

function shortcutKey(code) {
  const letter = /^Key([A-Z])$/.exec(code)
  if (letter) return letter[1]
  const digit = /^Digit([0-9])$/.exec(code)
  if (digit) return digit[1]
  return SHORTCUT_KEYS[code]
}

/**
 * The shortcut a key press spells, as Wails writes it ("Ctrl+Alt+Space"), or undefined while it is
 * not one yet: a shortcut needs a modifier other than Shift, so it cannot swallow plain typing, and a key
 * Wails can name. On a Mac the Command key is "Cmd"; elsewhere the Windows or Super key is "Super".
 */
export function shortcutOf(press, isMac) {
  const key = shortcutKey(press.code)
  if (key === undefined) return undefined
  const parts = []
  if (press.ctrlKey) parts.push('Ctrl')
  if (press.altKey) parts.push('Alt')
  if (press.metaKey) parts.push(isMac ? 'Cmd' : 'Super')
  if (parts.length === 0) return undefined
  if (press.shiftKey) parts.push('Shift')
  return [...parts, key].join('+')
}

/** Shortcuts that copy, paste, undo, quit, close, interrupt a terminal or switch apps and tabs: taken globally, every app loses them. */
const SHARED_SHORTCUTS = ['Ctrl+C', 'Ctrl+D', 'Ctrl+Z', 'Ctrl+Tab', 'Ctrl+Shift+Tab']
const SYSTEM_SHORTCUTS = {
  mac: new Set([...SHARED_SHORTCUTS, 'Cmd+Q', 'Cmd+W', 'Cmd+A', 'Cmd+C', 'Cmd+V', 'Cmd+X', 'Cmd+Z', 'Cmd+Shift+Z', 'Cmd+H', 'Cmd+M', 'Cmd+Space', 'Ctrl+Space', 'Cmd+Tab', 'Cmd+Shift+Tab']),
  other: new Set([...SHARED_SHORTCUTS, 'Ctrl+Q', 'Ctrl+W', 'Ctrl+A', 'Ctrl+V', 'Ctrl+X', 'Ctrl+Y', 'Ctrl+Shift+Z', 'Alt+Tab', 'Alt+Shift+Tab', 'Alt+F4', 'Ctrl+Alt+Delete', 'Ctrl+Alt+Backspace', 'Super+L']),
}

/** Why a shortcut cannot be chosen, or undefined when it can. */
export function refusedShortcutOf(shortcut, isMac) {
  if (!SYSTEM_SHORTCUTS[isMac ? 'mac' : 'other'].has(shortcut)) return undefined
  return `${shortcut} is a shortcut every app uses; choose other keys`
}

/** How each part of a shortcut reads on a Mac, where the modifiers have symbols. */
const MAC_KEYCAPS = { Ctrl: '⌃', Alt: '⌥', Shift: '⇧', Cmd: '⌘', Super: '⌘' }

/** The keycaps that show a shortcut: ["⌃", "⌥", "Space"] on a Mac, ["Ctrl", "Alt", "Space"] elsewhere. */
export function keycapsOf(shortcut, isMac) {
  const parts = shortcut.split('+')
  if (!isMac) return parts
  return parts.map((part) => MAC_KEYCAPS[part] ?? part)
}

const MENU_KINDS = new Set(['switch', 'shortcut', 'action'])

/**
 * The Hide item's label: plain while the shortcut works, which the item above already shows,
 * otherwise with the command that brings the companion back. The shell's hideLabel says the same.
 */
export function hideLabelOf(label, preferences) {
  return preferences.shortcutProblem ? `${label} (agnostic-ai companion brings it back)` : label
}

/** The update entry's label in its state; the shell's updateLabel says the same. */
export function updateLabelOf(label, update) {
  if (update.state === 'checking') return 'Checking for updates…'
  if (update.state === 'available' || update.state === 'failed') return `Update to v${update.version}`
  if (update.state === 'updating') return `Updating to v${update.version}…`
  return label
}

/**
 * Shows the newest update status. since() returns a show for a read started now,
 * which drops the reply when an update event arrived while it was read.
 */
export function updateFeed(show) {
  let events = 0
  return {
    event(update) {
      events += 1
      show(update)
    },
    since() {
      const at = events
      return (update) => {
        if (events === at) show(update)
      }
    },
  }
}

const UPDATE_STEPS = {
  downloading: (update) => `Downloading v${update.version}…`,
  verifying: () => 'Checking the download…',
  restarting: () => 'Restarting…',
}

/**
 * What the panel says about the update: {text, retry?, log?, command?, releasePage?}, or undefined when there is nothing to say.
 * A check the companion made on its own says nothing once it ends.
 */
export function updateMessageOf(update) {
  const note = olderCLINoteOf(update)
  const withNote = (text) => (note ? { text: `${text} ${note.text}`, command: note.command } : { text })
  if (update.result === 'release-changed') return { text: update.reason, log: update.log }
  if (update.result === 'unreachable') return withNote('Could not check for updates. Check your connection and try again.')
  if (update.result === 'latest') return withNote(`No newer version is available for this Mac. You have v${update.current}.`)
  if (update.state === 'checking') return { text: 'Checking for updates…' }
  if (update.state === 'updating') return { text: UPDATE_STEPS[update.step]?.(update) ?? `Updating to v${update.version}…` }
  if (update.state === 'failed') {
    const failed = { text: `The update did not finish: ${update.reason}.`, retry: true, log: update.log }
    return update.command ? { ...failed, command: update.command, releasePage: true } : failed
  }
  return note
}

/** The note about a separate agnostic-ai command older than the disk image app, or undefined. */
function olderCLINoteOf(update) {
  if (!update.olderCLI) return undefined
  const { path, version, command } = update.olderCLI
  return { text: `The agnostic-ai command at ${path} is v${version}, older than this app (v${update.current}). Run this in a terminal to upgrade it.`, command }
}

/** The question before an update: {title, body, go, command?, releasePage?}; with a command, the person runs it in a terminal; the release page says what changed. */
export function updateConfirmOf(update) {
  if (update.command && update.state !== 'failed') {
    return {
      title: `Update to v${update.version}`,
      body: 'The companion cannot run this update itself. Run this in a terminal: it updates the command line tool and the companion together. The release page lists what changed.',
      go: 'Copy command',
      command: update.command,
      releasePage: true,
    }
  }
  const body = update.terminal
    ? 'Runs agnostic-ai upgrade: it upgrades the command line tool the way you installed it, then replaces and restarts the companion. Your settings and chats stay.'
    : 'The app and its command line tool are replaced together. Your settings and chats stay. The companion is gone for a few seconds while it restarts.'
  return { title: `Update to v${update.version} and restart?`, body, go: 'Update and restart' }
}

/** The settings button's name while an update is known, or undefined when there is none. */
export function updateBadgeOf(update) {
  if (!update.version || update.state === 'updating') return undefined
  return `Settings: update to v${update.version} available`
}

/**
 * The settings menu's rows, from menu.json and the shell's three replies:
 * {id, label, kind, hint?, value, disabled?}. A switch's value is its
 * entry in the appearance or the preferences; a shortcut's is its keys, the preference of the same id.
 * Hide's label names the way back. The update entry shows only where the companion can update itself.
 */
export function menuRowsOf(items, { preferences, appearance, mailbox, update = { state: 'unavailable' } }, isMac = false) {
  const values = { ...appearance, ...preferences, ...mailbox }
  const updates = new Set(['update', 'checkForUpdates'])
  return items.filter((item) => !updates.has(item.id) || update.state !== 'unavailable').map((item) => {
    if (!MENU_KINDS.has(item.kind)) throw new Error(`the menu item ${item.id} has the unknown kind ${item.kind}`)
    let value
    if (item.page && typeof values[item.id] !== 'boolean') return { ...item, unavailable: true }
    if (item.kind === 'switch') {
      if (typeof values[item.id] !== 'boolean') throw new Error(`no value for ${item.id}`)
      value = values[item.id]
    }
    if (item.kind === 'shortcut') {
      if (typeof preferences[item.id] !== 'string') throw new Error(`no keys for ${item.id}`)
      value = preferences[item.id]
    }
    if (item.id === 'hide') return { ...item, label: hideLabelOf(item.label, preferences), value }
    if (item.id === 'update') return { ...item, label: updateLabelOf(item.label, update), value, disabled: update.state === 'checking' || update.state === 'updating' }
    return { ...item, value }
  })
}

const listed = new Intl.ListFormat('en', { type: 'conjunction' })

/** Why some tools list nothing: their store needs a command this computer lacks, such as sqlite3 on Windows. */
export function missingToolsNoteOf(view) {
  const lacking = (view.tools ?? []).filter((tool) => tool.needs)
  if (lacking.length === 0) return undefined
  const commands = [...new Set(lacking.map((tool) => tool.needs))]
  return `${listed.format(lacking.map((tool) => tool.label))} chats are missing: reading them needs the ${listed.format(commands)} command.`
}
