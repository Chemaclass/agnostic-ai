// Conversations: recent chats from Claude Code, Codex and Cursor, the open
// ones first, then by day. The server sends one week per page; the next
// loads when the list nears its end or from the button below it. The
// source chips in the header filter by tool.
// Each row asks the server to reopen the chat where it lives; the page
// only posts which one and shows the reply. A static snapshot has no
// server to read the chats, so it only says what it needs.

import { useEffect, useId, useRef, useState } from 'react'
import { CONVERSATIONS_NEED_SERVER, loadConversations, openConversation, type ConversationOpenOutcome } from '../api'
import type { Conversation, ConversationRoute, ConversationSource, ConversationsView, ConversationTool } from '../contract'
import { baseName } from '../model/config-switch'
import { appLabel, filterConversations, groupConversations, liveBadge, mergePages, relativeTime, replyCommand, sourceLine, toggleTool, WITHHELD_NOTE, withheldLabel, type ToolFilter } from '../model/conversations'
import { effectiveEditor } from '../model/editor'
import { formatTime, plural } from '../model/format'
import { PageHeader, useApp } from './app-context'
import { ErrorPanel } from './Banner'
import { useEditorChoice } from './EditorLink'
import { Badge, CopyButton, Icon, ToolAvatar } from './ui'

const TITLE = 'Conversations'
const INTRO = 'Pick up a chat where you left it, in the tool where it lives.'
const MOCK_OPEN_MESSAGE = 'Mock data: opening a conversation needs a running agnostic-ai serve.'

/** Where loading the week before stands. */
type Older =
  | { state: 'idle' }
  | { state: 'loading'; before: string }
  | { state: 'failed'; before: string; error: Error }

// The next week starts loading this far before the end of the list comes into view.
const LOAD_AHEAD = '0px 0px 400px 0px'

export function ConversationsPage() {
  const app = useApp()
  const { editor: chosen, template } = useEditorChoice()
  const editor = effectiveEditor(chosen, template)
  const [pages, setPages] = useState<ConversationsView[]>([])
  const [error, setError] = useState<Error>()
  const [older, setOlder] = useState<Older>({ state: 'idle' })
  const [tool, setTool] = useState<ToolFilter>('all')
  const olderRequest = useRef<AbortController | undefined>(undefined)
  const sentinel = useRef<HTMLDivElement>(null)
  const olderButton = useRef<HTMLButtonElement>(null)
  const endNote = useRef<HTMLParagraphElement>(null)
  const focusEndNote = useRef(false)
  const needsServer = app.source.kind === 'static'
  const next = pages.at(-1)?.next

  // The first page. A new editor changes every link, so it starts over.
  useEffect(() => {
    if (needsServer) return
    const controller = new AbortController()
    olderRequest.current?.abort()
    olderRequest.current = undefined
    setOlder({ state: 'idle' })
    setError(undefined)
    loadConversations(app.source, editor, undefined, controller.signal).then(
      (loaded) => {
        if (!controller.signal.aborted) setPages([loaded.data])
      },
      (failure: Error) => {
        if (!controller.signal.aborted && failure.name !== 'AbortError') setError(failure)
      },
    )
    return () => controller.abort()
  }, [needsServer, app.source, editor])

  useEffect(() => () => olderRequest.current?.abort(), [])

  // One week at a time: while a request runs, every other ask is ignored.
  const loadOlder = (before: string) => {
    if (olderRequest.current) return
    const controller = new AbortController()
    olderRequest.current = controller
    setOlder({ state: 'loading', before })
    loadConversations(app.source, editor, before, controller.signal).then(
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
        <PageHeader title={TITLE} subtitle={INTRO} />
        <p className="muted">{CONVERSATIONS_NEED_SERVER} A static snapshot has no chats to list.</p>
      </>
    )
  }
  if (!loaded) {
    return (
      <>
        <PageHeader title={TITLE} subtitle={INTRO} />
        {error ? <ErrorPanel error={error} /> : <p className="loading" aria-busy="true">Loading conversations…</p>}
      </>
    )
  }

  const rows = mergePages(pages)
  const sources = pages.at(-1)!.tools
  const query = app.state.q ?? ''
  const setQuery = (value: string) => app.navigate({ q: value || undefined }, { replace: true })
  const matching = filterConversations(rows, query, tool)
  const now = new Date()
  const groups = groupConversations(matching, now)
  const filtering = query.trim() !== '' || tool !== 'all'
  const canOpen = app.source.kind === 'api'
  const loading = older.state === 'loading'

  return (
    <>
      <PageHeader title={TITLE} subtitle={INTRO}>
        <Sources sources={sources} rows={rows} tool={tool} onToggle={(chosen) => setTool(toggleTool(tool, chosen))} />
      </PageHeader>
      {error && <ErrorPanel error={error} />}
      <Search
        query={query}
        onQuery={setQuery}
        status={filtering ? `${matching.length} of ${plural(rows.length, 'conversation')} match.` : plural(rows.length, 'conversation')}
      />
      {rows.length === 0 && <p className="muted">{next ? 'No conversation in the past week.' : 'No conversations to show.'}</p>}
      {rows.length > 0 && groups.length === 0 && <p className="muted">No conversation matches.</p>}
      {groups.map((group) => {
        const headingId = `conversations-${group.id}`
        return (
          <section className="block conversation-group" aria-labelledby={headingId} key={group.id}>
            <h2 id={headingId}>{group.label} <span className="muted kind-count">{group.conversations.length}</span></h2>
            <ul className="conversation-list">
              {group.conversations.map((conversation) => (
                <ConversationRow key={`${conversation.tool}:${conversation.id}`} conversation={conversation} now={now} editor={editor} canOpen={canOpen} />
              ))}
            </ul>
          </section>
        )
      })}
      <div className="conversations-more">
        <div ref={sentinel} className="conversations-sentinel" aria-hidden="true" />
        {older.state === 'failed' && <p className="conversations-more-error" role="alert">Could not load the week before. {older.error.message}</p>}
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
      </div>
    </>
  )
}

// Each found source is a toggle that shows only its tool; a source that was
// not found has nothing to show, so it is plain text with the server's note.
function Sources({ sources, rows, tool, onToggle }: { sources: ConversationSource[]; rows: Conversation[]; tool: ToolFilter; onToggle: (tool: ConversationTool) => void }) {
  if (sources.length === 0) return null
  return (
    <ul className="conversation-sources" aria-label="Sources">
      {sources.map((source) => {
        const text = (
          <>
            <span aria-hidden="true"><ToolAvatar target={source.tool} label={source.label} /></span>
            <span className="conversation-source-text">
              <span className="conversation-source-name">{source.label}</span>
              <span className="conversation-source-count">{sourceLine(source, rows)}</span>
            </span>
          </>
        )
        if (!source.found) {
          return (
            <li key={source.tool} className="conversation-source is-missing">
              <span className="conversation-source-chip">{text}</span>
              {source.note && <span className="muted conversation-source-note">{source.note}</span>}
            </li>
          )
        }
        const pressed = tool === source.tool
        return (
          <li key={source.tool} className="conversation-source">
            <button
              type="button"
              className="conversation-source-chip conversation-source-toggle"
              aria-pressed={pressed}
              title={pressed ? 'Show every tool' : `Show only ${source.label}`}
              onClick={() => onToggle(source.tool)}
            >
              {text}
            </button>
            {source.note && <span className="muted conversation-source-note">{source.note}</span>}
          </li>
        )
      })}
    </ul>
  )
}

function Search({ query, onQuery, status }: { query: string; onQuery: (value: string) => void; status: string }) {
  const searchId = useId()
  const input = useRef<HTMLInputElement>(null)
  const describedBy = `${searchId}-status ${searchId}-note`
  return (
    <div className="section-search conversations-search" role="search" aria-label="Search conversations">
      <label htmlFor={searchId}>Search conversations</label>
      <div className="section-search-controls">
        <div className="section-search-input">
          <Icon name="search" />
          <input ref={input} id={searchId} type="search" value={query} onChange={(event) => onQuery(event.target.value)} placeholder="Title, folder, branch, or prompt" aria-describedby={describedBy} />
        </div>
        {query && (
          <button type="button" className="button button-quiet" onClick={() => {
            onQuery('')
            input.current?.focus()
          }}>Clear search</button>
        )}
      </div>
      <p id={`${searchId}-status`} className="muted section-search-status" role="status">{status}</p>
      <p id={`${searchId}-note`} className="muted conversations-search-note">Search covers the weeks loaded so far.</p>
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

function ConversationRow({ conversation, now, editor, canOpen }: { conversation: Conversation; now: Date; editor: string; canOpen: boolean }) {
  const [pending, setPending] = useState(false)
  const [status, setStatus] = useState<OpenStatus>()
  const title = conversation.title || 'Untitled'
  const withheld = conversation.withheld ?? []
  const opening = (conversation.opening ?? []).slice(0, 3)
  const route = conversation.open

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
  }

  return (
    <li className={`conversation${conversation.live ? ' is-live' : ''}`} data-tool={conversation.tool}>
      <ToolAvatar target={conversation.tool} />
      <div className="conversation-body">
        <h3 className="conversation-title">{title}</h3>
        {withheld.includes('title') && <Withheld label={withheldLabel('title')} />}
        <p className="conversation-meta">
          <span>{appLabel(conversation.app)}</span>
          {conversation.live && <LiveBadge conversation={conversation} />}
          {conversation.folder && <span className="conversation-folder" title={conversation.folder}>{baseName(conversation.folder)}</span>}
          {conversation.branch && <span className="conversation-branch">{conversation.branch}</span>}
          <time dateTime={conversation.updatedAt} title={formatTime(conversation.updatedAt)}>{relativeTime(conversation.updatedAt, now)}</time>
        </p>
        {withheld.includes('opening')
          ? <Withheld label={withheldLabel('opening')} />
          : opening.length > 0 && <div className="conversation-opening">{opening.map((line, index) => <p key={index}>{line}</p>)}</div>}
        {withheld.includes('prompt')
          ? <Withheld label={withheldLabel('prompt')} />
          : conversation.prompt && <p className="conversation-prompt"><span className="conversation-label">Last prompt</span> {conversation.prompt}</p>}
      </div>
      <div className="conversation-actions">
        <button type="button" className="button button-primary conversation-open" aria-disabled={pending || undefined} aria-label={`${route.label}: ${title}`} onClick={open}>
          {route.label}
        </button>
        {route.command && <CopyButton text={route.command} label="Copy command" ariaLabel={`Copy the command that resumes ${title}`} />}
      </div>
      <div className="conversation-result">
        <p className={`conversation-status${status ? ` is-${status.tone}` : ''}`} role="status">{pending ? 'Opening…' : status?.message ?? ''}</p>
        {status?.command && <CopyButton text={status.command} label="Copy command" ariaLabel={`Copy the command the server suggests for ${title}`} />}
      </div>
    </li>
  )
}

function LiveBadge({ conversation }: { conversation: Conversation }) {
  const badge = liveBadge(conversation.activity)
  return (
    <Badge tone={badge.tone} className="conversation-live" title={badge.description}>
      <span aria-hidden="true">{badge.label}</span>
      <span className="sr-only">{badge.description}</span>
    </Badge>
  )
}

function Withheld({ label }: { label: string }) {
  return <p className="conversation-withheld"><span className="conversation-label">{label}</span> {WITHHELD_NOTE}</p>
}
