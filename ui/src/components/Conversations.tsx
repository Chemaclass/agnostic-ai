// Conversations: recent chats from Claude Code, Codex and Cursor, the open
// ones first, then by day. The server sends one week per page; the next
// loads when the list nears its end or from the button below it. The
// tool buttons beside the search filter by tool.
// Each row asks the server to reopen the chat where it lives; the page
// only posts which one and shows the reply. A static snapshot has no
// server to read the chats, so it only says what it needs.

import { useEffect, useId, useRef, useState, type ReactNode } from 'react'
import { CONVERSATIONS_NEED_SERVER, loadConversations, openConversation, type ConversationOpenOutcome } from '../api'
import type { Conversation, ConversationRoute, ConversationSource, ConversationsView } from '../contract'
import { baseName } from '../model/config-switch'
import { appLabel, chatText, conversationsVerdict, filterConversations, groupConversations, liveBadge, mergePages, relativeTime, replyCommand, rowCommand, sameFirstPage, shownPrompt, stateBadge, toggleTool, toolChoices, WITHHELD_NOTE, withheldLabel, type ToolFilter } from '../model/conversations'
import type { DataSource } from '../model/url'
import { effectiveEditor } from '../model/editor'
import { formatTime, plural } from '../model/format'
import { PageHeader, useApp } from './app-context'
import { ErrorPanel, Notice } from './Banner'
import { useEditorChoice } from './EditorLink'
import { Badge, CopyButton, Icon, RowActions, ToolAvatar, toolName } from './ui'

const TITLE = 'Conversations'
const NEEDS_SERVER = CONVERSATIONS_NEED_SERVER
const INTRO = 'Pick up where you left off, in the tool where it lives.'
const SEARCH_NOTE = 'Search covers every conversation, loaded or not.'

// How long typing pauses before the search goes to the server.
const SEARCH_PAUSE_MS = 300

/** The line under the search box: a search on its way, its matches across every week, or what a tool button kept. */
function searchStatus(searching: boolean, matches: number | undefined, kept: number, loaded: number, toolChosen: boolean): string {
  if (searching) return 'Searching…'
  if (matches !== undefined && toolChosen) return `${kept} of ${plural(matches, 'conversation')} match.`
  if (matches !== undefined) return `${plural(matches, 'conversation')} ${matches === 1 ? 'matches' : 'match'}.`
  if (toolChosen) return `${kept} of ${plural(loaded, 'conversation')} match.`
  return ''
}

const MOCK_OPEN_MESSAGE = 'Mock data: opening a conversation needs agnostic-ai view running.'

/** Where loading the week before stands. */
type Older =
  | { state: 'idle' }
  | { state: 'loading'; before: string }
  | { state: 'failed'; before: string; error: Error }

// The next week starts loading this far before the end of the list comes into view.
const LOAD_AHEAD = '0px 0px 400px 0px'

// The last first page for each source and editor, kept until the page
// reloads. A return to the page shows it at once while a fresh copy
// loads, since which chats are open changes from one minute to the next.
const firstPages = new Map<string, ConversationsView>()
const firstPageKey = (source: DataSource, editor: string) => `${JSON.stringify(source)} ${editor}`

// Chats opened from this page may run now, so they offer no resume command
// until a fresh list reports them open. Kept beside the first pages, which a return redraws.
const openedChats = new Set<string>()

const SKELETON_TOOLS = 3
const SKELETON_ROWS = 4

export function ConversationsPage() {
  const app = useApp()
  const { editor: chosen, template } = useEditorChoice()
  const editor = effectiveEditor(chosen, template)
  const cacheKey = firstPageKey(app.source, editor)
  const [pages, setPages] = useState<ConversationsView[]>(() => {
    const cached = firstPages.get(cacheKey)
    return cached ? [cached] : []
  })
  const [error, setError] = useState<Error>()
  const [older, setOlder] = useState<Older>({ state: 'idle' })
  const [tool, setTool] = useState<ToolFilter>('all')
  const query = app.state.q ?? ''
  // The search goes to the server, which searches every conversation, once typing pauses.
  const [searched, setSearched] = useState(query.trim())
  const [searching, setSearching] = useState(false)
  const [opened, setOpened] = useState<ReadonlySet<string>>(() => new Set(openedChats))
  const markOpened = (key: string) => {
    openedChats.add(key)
    setOpened(new Set(openedChats))
  }
  const olderRequest = useRef<AbortController | undefined>(undefined)
  const sentinel = useRef<HTMLDivElement>(null)
  const olderButton = useRef<HTMLButtonElement>(null)
  const endNote = useRef<HTMLParagraphElement>(null)
  const focusEndNote = useRef(false)
  const needsServer = app.source.kind === 'static'
  const next = pages.at(-1)?.next

  useEffect(() => {
    const timer = window.setTimeout(() => setSearched(query.trim()), SEARCH_PAUSE_MS)
    return () => window.clearTimeout(timer)
  }, [query])

  // The first page. A new editor or search changes every row, so it starts over.
  // A kept copy shows until the fresh one lands, which replaces it only
  // when a row or source changed.
  useEffect(() => {
    if (needsServer) return
    const controller = new AbortController()
    olderRequest.current?.abort()
    olderRequest.current = undefined
    setOlder({ state: 'idle' })
    setError(undefined)
    // Only the list without a search is kept for a quick return.
    const cached = searched ? undefined : firstPages.get(cacheKey)
    if (cached) setPages((current) => current[0] === cached ? current : [cached])
    setSearching(searched !== '')
    loadConversations(app.source, editor, { query: searched || undefined }, controller.signal).then(
      (loaded) => {
        if (controller.signal.aborted) return
        setSearching(false)
        for (const conversation of loaded.data.conversations) {
          if (conversation.live) openedChats.delete(rowKey(conversation))
        }
        setOpened(new Set(openedChats))
        if (cached && sameFirstPage(cached, loaded.data)) return
        if (!searched) firstPages.set(cacheKey, loaded.data)
        // A week loaded or loading after the kept page may leave a gap
        // before the fresh one, so the list starts over from it.
        olderRequest.current?.abort()
        olderRequest.current = undefined
        setOlder({ state: 'idle' })
        setPages([loaded.data])
      },
      (failure: Error) => {
        if (controller.signal.aborted || failure.name === 'AbortError') return
        setSearching(false)
        setError(failure)
      },
    )
    return () => controller.abort()
  }, [needsServer, app.source, editor, cacheKey, searched])

  useEffect(() => () => olderRequest.current?.abort(), [])

  // One week at a time: while a request runs, every other ask is ignored.
  const loadOlder = (before: string) => {
    if (olderRequest.current) return
    const controller = new AbortController()
    olderRequest.current = controller
    setOlder({ state: 'loading', before })
    loadConversations(app.source, editor, { query: searched || undefined, before }, controller.signal).then(
      (loaded) => {
        if (controller.signal.aborted) return
        olderRequest.current = undefined
        focusEndNote.current = document.activeElement === olderButton.current && loaded.data.next === undefined
        setPages((current) => [...current, loaded.data])
        setOlder({ state: 'idle' })
      },
      (failure: Error) => {
        if (controller.signal.aborted || failure.name === 'AbortError') return
        olderRequest.current = undefined
        setOlder({ state: 'failed', before, error: failure })
      },
    )
  }

  // The button that loaded the last week goes away with it, so focus moves to the note that replaces it.
  useEffect(() => {
    if (!focusEndNote.current) return
    focusEndNote.current = false
    endNote.current?.focus()
  }, [pages])

  // Scrolling near the end loads the week before. The observer reads the
  // latest state through this ref; a failed week waits for Try again.
  const nearEnd = useRef(() => {})
  nearEnd.current = () => {
    if (next && older.state === 'idle') loadOlder(next)
  }
  const loaded = pages.length > 0
  useEffect(() => {
    const element = sentinel.current
    if (!element || typeof IntersectionObserver === 'undefined') return
    const observer = new IntersectionObserver((entries) => {
      if (entries.some((entry) => entry.isIntersecting)) nearEnd.current()
    }, { rootMargin: LOAD_AHEAD })
    observer.observe(element)
    return () => observer.disconnect()
  }, [loaded])

  if (needsServer) {
    return (
      <>
        <PageHeader title={TITLE} verdict={NEEDS_SERVER}>
          <p className="page-facts">A static snapshot has no chats to list.</p>
        </PageHeader>
      </>
    )
  }
  const setQuery = (value: string) => app.navigate({ q: value || undefined }, { replace: true })
  if (!loaded && error) {
    return (
      <>
        <PageHeader title={TITLE} subtitle={INTRO} />
        <ErrorPanel error={error} />
      </>
    )
  }

  // Until the first page arrives, placeholders hold the places of the
  // verdict, facts, chips and rows. The heading and search box are the
  // same elements before and after, so focus and typing carry over.
  const rows = mergePages(pages)
  const now = new Date()
  // The server already searched; the tool buttons narrow what it sent.
  const matching = filterConversations(rows, '', tool)
  const groups = groupConversations(matching, now)
  const matches = pages[0]?.matches
  const canOpen = app.source.kind === 'api'
  const loading = older.state === 'loading'

  return (
    <>
      <PageHeader title={TITLE} verdict={loaded ? conversationsVerdict(rows, now, next !== undefined) : <span aria-hidden="true">{skeletonBar('conversations-skeleton-verdict')}</span>}>
        {loaded
          ? <p className="page-facts"><span className="page-fact">{plural(rows.length, 'conversation')} loaded{next ? ' so far' : ''}</span><span className="page-fact">{INTRO}</span></p>
          : <p className="page-facts" aria-hidden="true">{skeletonBar('conversations-skeleton-facts')}</p>}
      </PageHeader>
      {error && <ErrorPanel error={error} />}
      <Search
        query={query}
        onQuery={setQuery}
        status={loaded ? searchStatus(searching, matches, matching.length, rows.length, tool !== 'all') : ''}
        tools={loaded
          ? <ToolFilterButtons sources={pages.at(-1)!.tools} rows={rows} tool={tool} onChoose={setTool} />
          : <ToolFilterSkeleton />}
      />
      {!loaded && <ListSkeleton />}
      {loaded && groups.length === 0 && (matches === 0 || rows.length > 0) && <p className="muted empty-matches">No conversation matches.</p>}
      {groups.map((group) => {
        const headingId = `conversations-${group.id}`
        return (
          <section className="block conversation-group" aria-labelledby={headingId} key={group.id}>
            <h2 id={headingId}>{group.label} <span className="muted kind-count">{group.conversations.length}</span></h2>
            <ul className="conversation-list">
              {group.conversations.map((conversation) => (
                <ConversationRow key={rowKey(conversation)} conversation={conversation} under={group.under.get(rowKey(conversation))} now={now} editor={editor} canOpen={canOpen} opened={opened.has(rowKey(conversation))} onOpened={() => markOpened(rowKey(conversation))} />
              ))}
            </ul>
          </section>
        )
      })}
      {loaded && <div className="conversations-more">
        <div ref={sentinel} className="conversations-sentinel" aria-hidden="true" />
        {older.state === 'failed' && <Notice tone="error" title="Could not load the week before." details={older.error.message} />}
        {next ? (
          <button
            ref={olderButton}
            type="button"
            className="button"
            aria-disabled={loading || undefined}
            aria-busy={loading || undefined}
            onClick={() => loadOlder(older.state === 'failed' ? older.before : next)}
          >
            {loading ? 'Loading the week before…' : older.state === 'failed' ? 'Try again' : 'Load the week before'}
          </button>
        ) : <p ref={endNote} className="muted conversations-end" tabIndex={-1}>That's every conversation.</p>}
      </div>}
    </>
  )
}

// The first load ever shows placeholder bars, each one line of the text
// that replaces it, so nothing moves when the list arrives.
function skeletonBar(className = '') {
  return <span className={`skeleton skeleton-line ${className}`.trim()} />
}

const skeletonAvatar = <span className="skeleton conversations-skeleton-avatar" />

function ToolFilterSkeleton() {
  return (
    <div className="segmented conversation-tools" aria-hidden="true">
      {Array.from({ length: SKELETON_TOOLS }, (_, index) => <span key={index} className="conversations-skeleton-tool">{skeletonBar()}</span>)}
    </div>
  )
}

function ListSkeleton() {
  return (
    <section className="block conversation-group conversations-skeleton" aria-busy="true" aria-label="Loading conversations">
      <p className="sr-only">Loading conversations…</p>
      <h2 aria-hidden="true">{skeletonBar('conversations-skeleton-heading')}</h2>
      <ul className="conversation-list" aria-hidden="true">
        {Array.from({ length: SKELETON_ROWS }, (_, index) => (
          <li key={index} className="conversation">
            {skeletonAvatar}
            <div className="conversation-body">
              <p className="conversation-title">{skeletonBar('conversations-skeleton-title')}</p>
              <p className="conversation-meta">{skeletonBar('conversations-skeleton-meta')}</p>
              <p className="conversation-prompt">{skeletonBar()}</p>
            </div>
            <span className="button conversations-skeleton-button">{skeletonBar()}</span>
          </li>
        ))}
      </ul>
    </section>
  )
}

// One button per tool whose store was found, after All. Pressing the chosen
// tool again shows every tool. Each button's title says how many of the
// tool's chats are loaded, since a week loads at a time.
function ToolFilterButtons({ sources, rows, tool, onChoose }: { sources: ConversationSource[]; rows: Conversation[]; tool: ToolFilter; onChoose: (tool: ToolFilter) => void }) {
  const choices = toolChoices(sources, rows)
  if (choices.length === 0) return null
  return (
    <div className="segmented conversation-tools" role="group" aria-label="Filter by tool">
      <button type="button" aria-pressed={tool === 'all'} onClick={() => onChoose('all')}>
        All <span className="conversation-tool-count">{rows.length}</span>
      </button>
      {choices.map((choice) => (
        <button key={choice.tool} type="button" data-tool={choice.tool} aria-pressed={tool === choice.tool} title={choice.description} onClick={() => onChoose(toggleTool(tool, choice.tool))}>
          {choice.label} <span className="conversation-tool-count">{choice.loaded}</span>
        </button>
      ))}
    </div>
  )
}

function Search({ query, onQuery, status, tools }: { query: string; onQuery: (value: string) => void; status: string; tools: ReactNode }) {
  const searchId = useId()
  const input = useRef<HTMLInputElement>(null)
  return (
    <div className="section-search conversations-search" role="search" aria-label="Search conversations">
      <label htmlFor={searchId} className="sr-only">Search conversations</label>
      <div className="section-search-input">
        <Icon name="search" />
        <input ref={input} id={searchId} type="search" value={query} onChange={(event) => onQuery(event.target.value)} placeholder="Title, folder, branch, or prompt" title={SEARCH_NOTE} aria-describedby={`${searchId}-status ${searchId}-note`} />
      </div>
      {tools}
      {query && (
        <button type="button" className="button" onClick={() => {
          onQuery('')
          input.current?.focus()
        }}>Clear search</button>
      )}
      <p id={`${searchId}-status`} className="section-search-status" role="status">{status}</p>
      <p id={`${searchId}-note`} className="sr-only">{SEARCH_NOTE}</p>
    </div>
  )
}

interface OpenStatus {
  tone: 'opened' | 'not-opened' | 'failed'
  message: string
  /** A command to resume it by hand that the row does not already offer. */
  command?: string
}

function openStatus(outcome: ConversationOpenOutcome, route: ConversationRoute): OpenStatus {
  if (outcome.kind === 'failed') return { tone: 'failed', message: outcome.message }
  const { reply } = outcome
  return { tone: reply.opened ? 'opened' : 'not-opened', message: reply.message, command: replyCommand(reply, route) }
}

const rowKey = (conversation: Conversation) => `${conversation.tool}:${conversation.id}`

function ConversationRow({ conversation, under, now, editor, canOpen, opened, onOpened }: { conversation: Conversation; under?: Conversation; now: Date; editor: string; canOpen: boolean; opened: boolean; onOpened: () => void }) {
  const [pending, setPending] = useState(false)
  const [status, setStatus] = useState<OpenStatus>()
  const openButton = useRef<HTMLButtonElement>(null)
  const actions = useRef<HTMLDivElement>(null)
  const title = chatText(conversation.title || 'Untitled')
  const withheld = conversation.withheld ?? []
  const prompt = withheld.includes('prompt') ? undefined : shownPrompt(title, conversation.prompt)
  const route = conversation.open
  const command = opened ? undefined : rowCommand(route)

  const open = async () => {
    if (pending) return
    if (!canOpen) {
      setStatus({ tone: 'not-opened', message: MOCK_OPEN_MESSAGE })
      return
    }
    setPending(true)
    setStatus(undefined)
    const outcome = await openConversation({ tool: conversation.tool, id: conversation.id, editor })
    setStatus(openStatus(outcome, route))
    setPending(false)
    if (outcome.kind === 'reply' && outcome.reply.opened) {
      // The command it hides may hold the focus.
      if (actions.current?.contains(document.activeElement) && document.activeElement !== openButton.current) openButton.current?.focus()
      onOpened()
    }
  }

  // Open now rows always show the ink pill; the others turn their link into it on hover and focus.
  return (
    <li className={`conversation row-hover${conversation.live ? ' is-live' : ''}${under ? ' is-nested' : ''}`} data-tool={conversation.tool}>
      <span aria-hidden="true"><ToolAvatar target={conversation.tool} /></span>
      <div className="conversation-body">
        <h3 className="conversation-title">{title.shell ? <code>{title.text}</code> : title.text}</h3>
        {under && <p className="sr-only">Started from {chatText(under.title || 'Untitled').text}</p>}
        {withheld.includes('title') && <Withheld label={withheldLabel('title')} />}
        {withheld.includes('reply')
          ? <Withheld label={withheldLabel('reply')} />
          : conversation.reply && <p className="conversation-reply" title={conversation.reply}><span className="sr-only">Latest reply: </span>{conversation.reply}</p>}
        <p className="conversation-meta">
          {conversation.live && <LiveBadge conversation={conversation} now={now} />}
          <StateBadge conversation={conversation} />
          <span className="conversation-tool" title={`${toolName(conversation.tool)}, ${appLabel(conversation.app)}`}>{toolName(conversation.tool)}<span className="sr-only">, {appLabel(conversation.app)}</span></span>
          {conversation.folder && <span className="conversation-folder" title={conversation.folder}>{baseName(conversation.folder)}</span>}
          {conversation.branch && <span className="conversation-branch">{conversation.branch}</span>}
          <time dateTime={conversation.updatedAt} title={formatTime(conversation.updatedAt)}>{relativeTime(conversation.updatedAt, now)}</time>
        </p>
        {withheld.includes('prompt')
          ? <Withheld label={withheldLabel('prompt')} />
          : prompt && <p className="conversation-prompt" title={prompt.text}><span className="conversation-label">Last prompt:</span> {prompt.shell ? <code>{prompt.text}</code> : prompt.text}</p>}
      </div>
      <div className="conversation-actions" ref={actions}>
        <button ref={openButton} type="button" className={`button conversation-open${conversation.live ? ' button-primary' : ''}`} aria-disabled={pending || undefined} aria-label={`${route.label}: ${title.text}`} onClick={open}>
          {route.label}
        </button>
        {command && (
          <RowActions label={title.text}>
            <CopyButton text={command} label="Copy command" ariaLabel={`Copy the command that resumes ${title.text}`} />
          </RowActions>
        )}
      </div>
      <div className="conversation-result">
        <p className={`conversation-status${status ? ` is-${status.tone}` : ''}`} role="status">{pending ? 'Opening…' : status?.message ?? ''}</p>
        {status?.command && <CopyButton text={status.command} label="Copy command" ariaLabel={`Copy the command the server suggests for ${title.text}`} />}
      </div>
    </li>
  )
}

function LiveBadge({ conversation, now }: { conversation: Conversation; now: Date }) {
  const badge = liveBadge(conversation, now)
  if (!badge) return null
  return (
    <Badge tone={badge.tone} className="conversation-live" title={badge.description}>
      <span aria-hidden="true">{badge.label}</span>
      <span className="sr-only">{badge.description}</span>
    </Badge>
  )
}

function StateBadge({ conversation }: { conversation: Conversation }) {
  const badge = stateBadge(conversation.state)
  if (!badge) return null
  return (
    <Badge tone={badge.tone} className="conversation-state" title={badge.description}>
      <span aria-hidden="true">{badge.label}</span>
      <span className="sr-only">{badge.description}</span>
    </Badge>
  )
}

function Withheld({ label }: { label: string }) {
  return <p className="conversation-withheld"><span className="conversation-label">{label}</span> {WITHHELD_NOTE}</p>
}
