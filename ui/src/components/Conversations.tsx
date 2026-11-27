import { useEffect, useEffectEvent, useId, useRef, useState, type FormEvent, type ReactNode, type RefObject } from 'react'
import { CONVERSATIONS_NEED_SERVER, loadConversations, findConversation, markConversation, renameConversation, deleteConversation, openConversation, stopConversationShell, type ConversationOpenOutcome } from '../api'
import type { Conversation, ConversationHelper, ConversationOpenRequest, ConversationMarkRequest, ConversationProcessRef, ConversationRoute, ConversationSource, ConversationsView, StuckShell } from '../contract'
import { baseName } from '../model/config-switch'
import { appLabel, CONVERSATION_PLACES, conversationGroupOf, conversationSearchStatus, chatText, conversationsVerdict, filterConversations, groupConversations, helperDetails, helperOpenRequest, helperRows, HELPERS_SHOWN, helperTitle, helperTooltip, liveBadge, needsYouReason, conversationToolLabel, conversationPlaceLabel, mergePages, relativeTime, replyCommand, rowCommand, sameFirstPage, selectedStuckShell, shellWarning, shownPrompt, stateBadge, toggleTool, toolChoices, WITHHELD_NOTE, withheldLabel, type ToolFilter, type PlaceFilter } from '../model/conversations'
import type { DataSource } from '../model/url'
import { effectiveEditor } from '../model/editor'
import { formatTime, plural } from '../model/format'
import { PageHeader, useApp } from './app-context'
import { ErrorPanel, Notice } from './Banner'
import { useEditorChoice } from './EditorLink'
import { Badge, CopyButton, Icon, copyText, ToolAvatar } from './ui'

import { BLANK_NAME_MESSAGE, canComplete, canRead, CLOUD_RESUME_TOOLTIP, cloudResumeOf, conversationMenuOf, DANGER_ITEMS, deletedOf, deleteReason, MARK_ALL_COMPLETE_TOOLTIP, MARK_ALL_READ_TOOLTIP, MENU_LABELS, MENU_TOOLTIPS, NEEDS_SERVER_MESSAGE, renameOf, unchangedForMark, type ConversationMenuItem } from '../model/conversation-actions'
import { ConversationMenu, type ConversationMenuAction } from './ConversationMenu'
import { ConversationDeleteDialog } from './ConversationDeleteDialog'
import { useConversationFeedbackFocus, type ConversationFocusIntent } from './useConversationFeedbackFocus'

const TITLE = 'Conversations'
const NEEDS_SERVER = CONVERSATIONS_NEED_SERVER
const INTRO = 'Pick up where you left off, in the tool where it lives.'
const SEARCH_NOTE = 'Search covers every conversation, loaded or not.'

// How long typing pauses before the search goes to the server.
const SEARCH_PAUSE_MS = 300

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
  const cloudNote = pages.reduce<string | undefined>((note, page) => page.cloudNote ?? note, undefined)
  const matchesCloudNote = useEffectEvent((note: string | undefined) => note === cloudNote)
  const [error, setError] = useState<Error>()
  const [refresh, setRefresh] = useState(0)
  const forceFresh = useRef(false)
  const [actionNotice, setActionNotice] = useState<(ShellFeedback & { key: string })>()
  const feedbackTarget = useRef<HTMLParagraphElement>(null)
  const [bulkPending, setBulkPending] = useState(false)
  const [bulkFeedback, setBulkFeedback] = useState('')
  const bulkTarget = useRef<HTMLParagraphElement>(null)
  const context = JSON.stringify([app.source, editor, app.state.q ?? '', app.state.place ?? 'all'])
  const latestContext = useRef(context)
  latestContext.current = context
  const feedbackFocus = useConversationFeedbackFocus(context, feedbackTarget, bulkTarget)
  const [shellFeedback, setShellFeedback] = useState<ReadonlyMap<string, ShellFeedback>>(() => new Map())
  const reloadAfterWrite = (focusIntent: ConversationFocusIntent | undefined, sent: boolean) => {
    if (focusIntent) feedbackFocus.schedule(focusIntent)
    if (sent) forceFresh.current = true
    setRefresh((count) => count + 1)
  }
  const updateShellFeedback = (key: string, feedback?: ShellFeedback, focusIntent?: ConversationFocusIntent, sent = true) => {
    setShellFeedback((current) => {
      if (!feedback && !current.has(key)) return current
      const next = new Map(current)
      if (feedback) next.set(key, feedback)
      else next.delete(key)
      return next
    })
    if (feedback) reloadAfterWrite(focusIntent, sent)
  }
  const [older, setOlder] = useState<Older>({ state: 'idle' })
  const [tool, setTool] = useState<ToolFilter>('all')
  const keepAvailableTool = (sources: ConversationSource[]) => setTool((current) => current === 'all' || sources.some((source) => source.tool === current && source.found) ? current : 'all')
  const place = app.state.place ?? 'all'
  const setPlace = (place: PlaceFilter) => app.navigate({ place: place === 'all' ? undefined : place })
  const [collapsed, setCollapsed] = useState<ReadonlySet<string>>(() => new Set(['archived']))
  const query = app.state.q ?? ''
  // The search goes to the server, which searches every conversation, once typing pauses.
  const [searched, setSearched] = useState(query.trim())
  const [displayedQuery, setDisplayedQuery] = useState('')
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
  // A kept copy shows until a fresh one with different visible content lands.
  useEffect(() => {
    if (needsServer) return
    const controller = new AbortController()
    const focusForRefresh = feedbackFocus.take()
    olderRequest.current?.abort()
    olderRequest.current = undefined
    setOlder({ state: 'idle' })
    setError(undefined)
    // Only the list without a search is kept for a quick return.
    // After a write the fresh first page replaces every loaded week, so no older row stays stale.
    const forced = forceFresh.current
    const cached = searched ? undefined : firstPages.get(cacheKey)
    if (cached && !forced) setPages((current) => current[0] === cached ? current : [cached])
    setSearching(searched !== '')
    loadConversations(app.source, editor, { query: searched || undefined }, controller.signal).then(
      (loaded) => {
        if (controller.signal.aborted) return
        if (forced) forceFresh.current = false
        setSearching(false)
        feedbackFocus.complete(focusForRefresh)
        setDisplayedQuery(searched)
        keepAvailableTool(loaded.data.tools)
        for (const conversation of loaded.data.conversations) {
          if (conversation.live) openedChats.delete(rowKey(conversation))
        }
        setOpened(new Set(openedChats))
        if (!forced && cached && sameFirstPage(cached, loaded.data) && matchesCloudNote(loaded.data.cloudNote)) return
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
        feedbackFocus.complete(focusForRefresh)
        setError(failure)
      },
    )
    return () => { controller.abort(); feedbackFocus.abandon(focusForRefresh) }
  }, [needsServer, app.source, editor, cacheKey, searched, refresh])

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
        keepAvailableTool(loaded.data.tools)
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

  const bulkMark = async (shown: Conversation[], as: 'seen' | 'completed') => {
    if (bulkPending || searching || query.trim() !== searched || query.trim() !== displayedQuery) return
    const focusIntent = feedbackFocus.start('bulk')
    if (app.source.kind !== 'api') { setBulkFeedback(NEEDS_SERVER_MESSAGE); feedbackFocus.complete(focusIntent); return }
    const selected = shown.filter(as === 'seen' ? canRead : canComplete)
    const depth = pages.length
    const startedIn = latestContext.current
    let saved = 0
    let skipped = 0
    let failure = ''
    let sent = false
    setBulkPending(true)
    setBulkFeedback('')
    try {
      for (const original of selected) {
        if (latestContext.current !== startedIn) { failure = 'The search or place changed. Remaining conversations were not changed.'; break }
        const current = await findConversation(app.source, editor, depth, original, searched || undefined)
        if (latestContext.current !== startedIn) { failure = 'The search or place changed. Remaining conversations were not changed.'; break }
        if (!current || filterConversations([current], '', tool, place).length === 0 || !unchangedForMark(original, current, as)) { skipped++; continue }
        try {
          sent = true
          await markConversation({ tool: original.tool, id: original.id, as })
          saved++
        } catch (error) {
          const message = error instanceof Error ? error.message : 'Could not change the conversation.'
          setShellFeedback((current) => new Map(current).set(rowKey(original), { failed: true, message }))
          failure = `${original.title || 'Untitled conversation'}: ${message}`
          break
        }
      }
    } catch (error) {
      failure = error instanceof Error ? error.message : 'Could not change the conversation.'
    } finally {
      setBulkPending(false)
      setBulkFeedback(`${saved} ${saved === 1 ? 'conversation' : 'conversations'} ${as === 'seen' ? 'marked as read' : 'marked complete'}.${skipped ? ` ${skipped} changed and skipped.` : ''}${failure ? ` ${failure}` : ''}`)
      reloadAfterWrite(focusIntent, sent)
    }
  }

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
  // The server searched every week; place and tool narrow the loaded rows.
  const names = new Map(rows.map((row) => [rowKey(row), row.title]))
  const matching = filterConversations(rows, '', tool, place)
  const groups = groupConversations(matching, now)
  const matches = pages[0]?.matches
  const filtered = tool !== 'all' || place !== 'all'
  const queryMatchesPage = query.trim() === displayedQuery
  const searchFailed = error !== undefined && searched === query.trim()
  const pendingSearch = !searchFailed && (searching || !queryMatchesPage)
  const bulkReady = queryMatchesPage && query.trim() === searched && !searching
  const status = searchFailed && !queryMatchesPage ? 'Search could not finish.' : conversationSearchStatus(pendingSearch, matches, matching.length, rows.length, filtered)
  const emptyMessage = next && (filtered || matches !== 0) ? 'No matches in the loaded weeks. Load the week before to check older conversations.' : 'No conversation matches.'
  const canOpen = app.source.kind === 'api'
  const loading = older.state === 'loading'
  const noticeGroup = actionNotice && groups.find((group) => group.conversations.some((row) => rowKey(row) === actionNotice.key))
  const noticeOutside = actionNotice && (!noticeGroup || collapsed.has(noticeGroup.id))

  return (
    <>
      <PageHeader title={TITLE} verdict={loaded ? conversationsVerdict(rows, now, next !== undefined, pages[0]?.needsYou) : <span aria-hidden="true">{skeletonBar('conversations-skeleton-verdict')}</span>}>
        {loaded
          ? <p className="page-facts"><span className="page-fact">{plural(rows.length, 'conversation')} loaded{next ? ' so far' : ''}</span><span className="page-fact">{INTRO}</span></p>
          : <p className="page-facts" aria-hidden="true">{skeletonBar('conversations-skeleton-facts')}</p>}
      </PageHeader>
      {error && <ErrorPanel error={error} />}
      <Search
        query={query}
        onQuery={setQuery}
        status={loaded ? status : ''}
        place={place}
        onPlace={setPlace}
        tools={loaded
          ? <ToolFilterButtons sources={pages.at(-1)!.tools} tool={tool} onChoose={setTool} />
          : <ToolFilterSkeleton />}
      />
      {noticeOutside && <p ref={feedbackTarget} tabIndex={-1} role="status" className="conversation-action-feedback">{actionNotice.message}</p>}
      {bulkFeedback && <p ref={bulkTarget} tabIndex={-1} role="status" className="conversation-action-feedback">{bulkFeedback}</p>}
      {!loaded && <ListSkeleton />}
      {loaded && queryMatchesPage && !pendingSearch && groups.length === 0 && <p className="muted empty-matches">{emptyMessage}</p>}
      {groups.map((group) => {
        const headingId = `conversations-${group.id}`
        return (
          <section className="block conversation-group" aria-labelledby={headingId} key={group.id}>
            {((group.id === 'needs-you' && !collapsed.has(group.id)) || group.id === 'active') && <div className="conversation-group-actions"><button type="button" className="button" title={group.id === 'needs-you' ? MARK_ALL_READ_TOOLTIP : MARK_ALL_COMPLETE_TOOLTIP} disabled={bulkPending || !bulkReady || !group.conversations.some(group.id === 'needs-you' ? canRead : canComplete)} onClick={() => bulkMark(group.conversations, group.id === 'needs-you' ? 'seen' : 'completed')}>{group.id === 'needs-you' ? 'Mark all as read' : 'Mark all complete'}</button></div>}
            <details open={!collapsed.has(group.id)} onToggle={(event) => {
              const closed = !event.currentTarget.open
              setCollapsed((current) => {
                if (current.has(group.id) === closed) return current
                const next = new Set(current)
                if (closed) next.add(group.id)
                else next.delete(group.id)
                return next
              })
            }}>
              <summary className="conversation-group-heading"><h2 id={headingId}>{group.label} <span className="muted kind-count">{group.conversations.length}</span></h2></summary>
              <ul className="conversation-list">
              {group.conversations.map((conversation) => (
                <ConversationRow key={rowKey(conversation)} conversation={conversation} inPinnedGroup={group.id === 'pinned'} reason={needsYouReason(conversation, now, (ref) => names.get(rowKey(ref)))} under={group.under.get(rowKey(conversation))} now={now} editor={editor} canOpen={canOpen} opened={opened.has(rowKey(conversation))} onOpened={() => markOpened(rowKey(conversation))} onMutationStart={() => feedbackFocus.start('row')} feedbackRef={!noticeOutside && actionNotice?.key === rowKey(conversation) ? feedbackTarget : undefined} onMutation={(feedback, focusIntent, sent = true) => { setActionNotice({ ...feedback, key: rowKey(conversation) }); updateShellFeedback(rowKey(conversation), feedback, focusIntent, sent) }} shellFeedback={shellFeedback.get(rowKey(conversation))} onShellFeedback={(feedback) => updateShellFeedback(rowKey(conversation), feedback)} />
              ))}
              </ul>
            </details>
          </section>
        )
      })}
      {loaded && <div className="conversations-more">
        <div ref={sentinel} className="conversations-sentinel" aria-hidden="true" />
        {older.state === 'failed' && <Notice tone="error" title="Could not load the week before." details={older.error.message} />}
        {next && (
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
        )}
        {(cloudNote || !next) && (
          <p ref={endNote} className="muted conversations-end" tabIndex={-1}>{cloudNote || "That's every conversation."}</p>
        )}
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

function ToolFilterButtons({ sources, tool, onChoose }: { sources: ConversationSource[]; tool: ToolFilter; onChoose: (tool: ToolFilter) => void }) {
  const choices = toolChoices(sources)
  if (choices.length === 0) return null
  return (
    <div className="segmented conversation-tools" role="group" aria-label="Filter by tool">
      {choices.map((choice) => (
        <button key={choice.tool} type="button" data-tool={choice.tool} aria-pressed={tool === choice.tool} title={choice.description} onClick={() => onChoose(toggleTool(tool, choice.tool))}>
          {choice.label}
        </button>
      ))}
    </div>
  )
}

function Search({ query, onQuery, status, tools, place, onPlace }: { query: string; onQuery: (value: string) => void; status: string; tools: ReactNode; place: PlaceFilter; onPlace: (place: PlaceFilter) => void }) {
  const searchId = useId()
  const input = useRef<HTMLInputElement>(null)
  return (
    <div className="section-search conversations-search" role="search" aria-label="Search conversations">
      <label htmlFor={searchId} className="sr-only">Search conversations</label>
      <div className="section-search-input">
        <Icon name="search" />
        <input ref={input} id={searchId} type="search" value={query} onChange={(event) => onQuery(event.target.value)} placeholder="Search every conversation" title={SEARCH_NOTE} aria-describedby={`${searchId}-status ${searchId}-note`} />
        {query && <button type="button" className="conversation-clear" aria-label="Clear search" title="Clear search" onClick={() => {
          onQuery('')
          input.current?.focus()
        }}><Icon name="close" /></button>}
      </div>
      <div className="conversation-filters">
        <div className="segmented conversation-places" role="group" aria-label="Filter by place">
          {CONVERSATION_PLACES.map((choice) => <button key={choice.id} type="button" aria-pressed={place === choice.id} title={choice.title} onClick={() => onPlace(choice.id)}>{choice.label}</button>)}
        </div>
        {tools}
      </div>
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
  /** What the command resumes: the row, or a Codex subagent that opens on its own. */
  subject: string
}

function openStatus(outcome: ConversationOpenOutcome, route: ConversationRoute, subject: string): OpenStatus {
  if (outcome.kind === 'failed') return { tone: 'failed', message: outcome.message, subject }
  const { reply } = outcome
  return { tone: reply.opened ? 'opened' : 'not-opened', message: reply.message, command: replyCommand(reply, route), subject }
}

const rowKey = (conversation: Pick<Conversation, 'tool' | 'id'>) => `${conversation.tool}:${conversation.id}`

interface ShellFeedback {
  message: string
  failed: boolean
}

function ConversationRow({ conversation, inPinnedGroup, reason, under, now, editor, canOpen, opened, onOpened, shellFeedback, onShellFeedback, onMutation, onMutationStart, feedbackRef }: { conversation: Conversation; inPinnedGroup: boolean; reason?: string; under?: Conversation; now: Date; editor: string; canOpen: boolean; opened: boolean; onOpened: () => void; shellFeedback?: ShellFeedback; onShellFeedback: (feedback?: ShellFeedback) => void; onMutation: (feedback: ShellFeedback, focusIntent: ConversationFocusIntent, sent?: boolean) => void; onMutationStart: () => ConversationFocusIntent; feedbackRef?: RefObject<HTMLParagraphElement | null> }) {
  const [pending, setPending] = useState(false)
  const [status, setStatus] = useState<OpenStatus>()
  const [shellSelection, setShellSelection] = useState<StuckShell>()
  const [deleting, setDeleting] = useState(false)
  const [renaming, setRenaming] = useState(false)
  const mutationBusy = useRef(false)
  const refocusMore = useRef(false)
  const openButton = useRef<HTMLButtonElement>(null)
  const actions = useRef<HTMLDivElement>(null)
  const title = chatText(conversation.title || 'Untitled')
  const withheld = conversation.withheld ?? []
  const prompt = withheld.includes('prompt') ? undefined : shownPrompt(title, conversation.prompt)
  const route = conversation.open
  const command = opened || pending ? undefined : rowCommand(route)
  const shells = conversation.stuckShells ?? []
  const chooseShell = (shell: StuckShell) => {
    setStatus(undefined)
    onShellFeedback()
    setShellSelection(shell)
  }

  const open = async (request: ConversationOpenRequest = { tool: conversation.tool, id: conversation.id, editor }, ownRow = true, subject = title.text) => {
    if (pending) return
    if (!canOpen) {
      onShellFeedback()
      setStatus({ tone: 'not-opened', message: MOCK_OPEN_MESSAGE, subject })
      return
    }
    setPending(true)
    setStatus(undefined)
    onShellFeedback()
    const outcome = await openConversation(request)
    setStatus(openStatus(outcome, route, subject))
    setPending(false)
    if (ownRow && outcome.kind === 'reply' && outcome.reply.opened && !outcome.reply.guessed) {
      // The command it hides may hold the focus.
      if (actions.current?.contains(document.activeElement) && document.activeElement !== openButton.current) openButton.current?.focus()
      onOpened()
    }
  }

  const focusMore = () => actions.current?.querySelector<HTMLButtonElement>('.row-menu-button')?.focus()
  const change = async (as: ConversationMarkRequest['as']) => {
    if (mutationBusy.current) return
    const focusIntent = onMutationStart()
    if (!canOpen) { onMutation({ failed: true, message: NEEDS_SERVER_MESSAGE }, focusIntent, false); return }
    mutationBusy.current = true
    try {
      await markConversation({ tool: conversation.tool, id: conversation.id, as })
      onMutation({ failed: false, message: MARKED[as] }, focusIntent)
    } catch (error) { onMutation({ failed: true, message: error instanceof Error ? error.message : 'Could not change the conversation.' }, focusIntent) }
    finally { mutationBusy.current = false }
  }
  const copy = async (text: string, what: string) => {
    onShellFeedback()
    const ok = await copyText(text)
    setStatus({ tone: ok ? 'opened' : 'failed', message: ok ? `${what} copied.` : 'Copy failed.', subject: title.text })
  }
  const startRename = () => {
    if (!canOpen) { onMutation({ failed: true, message: NEEDS_SERVER_MESSAGE }, onMutationStart(), false); return }
    setRenaming(true)
  }
  const saveRename = async (name: string): Promise<string | undefined> => {
    try {
      await renameConversation({ tool: conversation.tool, id: conversation.id, title: name })
    } catch (error) {
      return error instanceof Error ? error.message : 'Could not rename the conversation.'
    }
    const focusIntent = onMutationStart()
    setRenaming(false)
    onMutation({ failed: false, message: 'Renamed.' }, focusIntent)
    return undefined
  }
  const startDelete = () => {
    if (!canOpen) { onMutation({ failed: true, message: NEEDS_SERVER_MESSAGE }, onMutationStart(), false); return }
    const refusal = deleteReason(conversation)
    if (refusal) { onMutation({ failed: true, message: refusal }, onMutationStart(), false); return }
    setDeleting(true)
  }
  useEffect(() => {
    if (!deleting) return
    const refusal = deleteReason(conversation)
    if (!refusal) return
    setDeleting(false)
    onShellFeedback()
    setStatus({ tone: 'failed', message: refusal, subject: title.text })
    refocusMore.current = true
  }, [deleting, conversation])
  useEffect(() => {
    if (deleting || !refocusMore.current) return
    refocusMore.current = false
    focusMore()
  }, [deleting])
  const localResume = command
  const resume = cloudResumeOf(conversation) ?? localResume
  const run: Record<ConversationMenuItem, () => void> = {
    read: () => { void change('seen') },
    rename: startRename,
    pin: () => { void change('pinned') },
    unpin: () => { void change('unpinned') },
    complete: () => { void change('completed') },
    reopen: () => { void change('open') },
    hide: () => { void change('hidden') },
    web: () => { if (conversation.cloud) void open({ tool: conversation.tool, id: conversation.cloud.id, editor }, false) },
    'copy-session-id': () => { void copy(conversation.id, 'Session ID') },
    'copy-resume': () => { if (resume) void copy(resume, 'Resume command') },
    delete: startDelete,
    'stop-shell': () => {},
  }
  const menu: ConversationMenuAction[] = conversationMenuOf(conversation, { localResume, server: canOpen }).flatMap((item): ConversationMenuAction[] => {
    if (item === 'stop-shell') return shells.map((shell) => ({ id: shellKey(shell), label: shells.length === 1 ? MENU_LABELS[item] : `Stop shell (PID ${shell.pid})…`, description: MENU_TOOLTIPS[item], danger: true, run: () => chooseShell(shell) }))
    const description = item === 'copy-resume' ? (cloudResumeOf(conversation) ? CLOUD_RESUME_TOOLTIP : `Copies the command that resumes ${title.text}`) : MENU_TOOLTIPS[item]
    return [{ id: item, label: MENU_LABELS[item], description, danger: DANGER_ITEMS.has(item), run: run[item] }]
  })

  const statusTone = shellFeedback ? (shellFeedback.failed ? 'failed' : 'opened') : status?.tone

  const place = conversationPlaceLabel(conversation)
  const toolLabel = conversationToolLabel(conversation)
  const activityBadge = conversation.live && !conversation.state && !reason ? liveBadge(conversation, now) : undefined
  const originalGroup = inPinnedGroup && !reason ? conversationGroupOf(conversation) : undefined
  const showState = originalGroup !== undefined || (conversation.state === 'active' && conversation.unread)
  const badge = activityBadge ?? (showState ? stateBadge(conversation.state, conversation.unread, originalGroup) : undefined)
  return (
    <li className={`conversation row-hover${conversation.live ? ' is-live' : ''}${under ? ' is-nested' : ''}`} data-tool={conversation.tool}>
      <span aria-hidden="true"><ToolAvatar target={conversation.tool} /></span>
      <div className="conversation-body">
        <h3 className="conversation-title">{title.shell ? <code>{title.text}</code> : title.text}</h3>
        {renaming && <RenameField shown={conversation.title} subject={title.text} onSave={saveRename} onClose={(refocus) => { setRenaming(false); if (refocus) focusMore() }} />}
        {reason && <p className="conversation-reason">{reason}</p>}
        {under && <p className="sr-only">Started from {chatText(under.title || 'Untitled').text}</p>}
        {withheld.includes('title') && <Withheld label={withheldLabel('title')} />}
        {withheld.includes('reply')
          ? <Withheld label={withheldLabel('reply')} />
          : conversation.reply && <p className="conversation-reply" title={conversation.reply}><span className="sr-only">Latest reply: </span>{conversation.reply}</p>}
        <p className="conversation-meta">
          {badge && <Badge tone={badge.tone} className={activityBadge ? 'conversation-live' : 'conversation-state'} title={badge.description}>
            <span aria-hidden="true">{badge.label}</span>
            <span className="sr-only">{badge.description}</span>
          </Badge>}
          <span className="conversation-tool" title={`${toolLabel}, ${appLabel(conversation.app)}`}>{toolLabel}<span className="sr-only">, {appLabel(conversation.app)}</span></span>
          {place && <span className="conversation-place">{place}</span>}
          {conversation.folder && <span className="conversation-folder" title={conversation.folder}>{baseName(conversation.folder)}</span>}
          {conversation.branch && <span className="conversation-branch">{conversation.branch}</span>}
          <time dateTime={conversation.updatedAt} title={formatTime(conversation.updatedAt)}>{relativeTime(conversation.updatedAt, now)}</time>
        </p>
        {withheld.includes('prompt')
          ? <Withheld label={withheldLabel('prompt')} />
          : prompt && <p className="conversation-prompt" title={prompt.text}><span className="conversation-label">Last prompt:</span> {prompt.shell ? <code>{prompt.text}</code> : prompt.text}</p>}
        {shells.map((shell) => (
          <p className="conversation-shell-warning" key={shellKey(shell)}>
            {shellWarning(shell, now)}. <ShellCommand command={shell.command} />
            {shells.length > 1 && <> (PID {shell.pid})</>}
          </p>
        ))}
      </div>
      <div className="conversation-actions" ref={actions}>
        <button ref={openButton} type="button" className={`button conversation-open${conversation.live ? ' button-primary' : ''}`} aria-disabled={pending || undefined} aria-label={`${route.label}: ${title.text}`} onClick={() => open()}>
          {route.label}
        </button>
        <ConversationMenu title={title.text} actions={menu} />
      </div>
      <Helpers conversation={conversation} now={now} pending={pending} onOpen={(helper) => {
        const request = helperOpenRequest(conversation, helper, editor)
        const ownRow = request.id === conversation.id
        // A Codex subagent opens its own thread, which is not this row.
        open(request, ownRow, ownRow ? title.text : helperTitle(helper))
      }} />
      <div className="conversation-result">
        <p ref={feedbackRef} tabIndex={-1} className={`conversation-status${statusTone ? ` is-${statusTone}` : ''}`} role="status">{pending ? 'Opening…' : shellFeedback?.message ?? status?.message ?? ''}</p>
        {status?.command && <CopyButton text={status.command} label="Copy command" ariaLabel={`Copy the command the server suggests for ${status.subject}`} />}
      </div>
      {deleting && <ConversationDeleteDialog row={conversation} onClose={(confirmed) => { setDeleting(false); if (!confirmed) focusMore() }} onConfirm={async () => {
        const focusIntent = onMutationStart()
        try {
          const reply = await deleteConversation({ tool: conversation.tool, id: conversation.id })
          onMutation({ failed: false, message: deletedOf(reply) }, focusIntent)
        } catch (error) { onMutation({ failed: true, message: error instanceof Error ? error.message : 'Could not delete the conversation.' }, focusIntent) }
      }} />}
      {shellSelection && (
        <StopShellDialog
          conversation={conversation}
          shell={canOpen ? selectedStuckShell(shells, shellSelection) : undefined}
          now={now}
          onClose={() => setShellSelection(undefined)}
          onResult={(feedback) => {
            // Unmount with the same render that shows the result: the
            // dialog's close event comes a task later, and until then the
            // closed dialog still holds the shell command.
            setShellSelection(undefined)
            onShellFeedback(feedback)
            openButton.current?.focus()
          }}
        />
      )}
    </li>
  )
}

const MARKED: Readonly<Record<ConversationMarkRequest['as'], string>> = {
  seen: 'Marked as read.',
  completed: 'Marked complete.',
  pinned: 'Pinned.',
  unpinned: 'Unpinned.',
  open: 'Completion undone.',
  hidden: 'Hidden from agnostic-ai. The transcript stays.',
}

function RenameField({ shown, subject, onSave, onClose }: { shown: string; subject: string; onSave(name: string): Promise<string | undefined>; onClose(refocus: boolean): void }) {
  const [draft, setDraft] = useState(shown)
  const [problem, setProblem] = useState<string>()
  const [saving, setSaving] = useState(false)
  const input = useRef<HTMLInputElement>(null)
  const problemId = useId()
  useEffect(() => {
    input.current?.focus()
    input.current?.select()
  }, [])
  const save = async () => {
    if (saving) return
    const decision = renameOf(draft, shown)
    if ('keep' in decision) { onClose(true); return }
    if ('empty' in decision) { setProblem(BLANK_NAME_MESSAGE); return }
    setSaving(true)
    const refusal = await onSave(decision.title)
    setSaving(false)
    if (refusal) { setProblem(refusal); input.current?.focus() }
  }
  return (
    <div className="conversation-rename">
      <input ref={input} className="conversation-rename-input" aria-label={`New name for ${subject}`} aria-describedby={problem ? problemId : undefined} aria-invalid={problem ? true : undefined} value={draft} readOnly={saving} onChange={(event) => { setDraft(event.target.value); setProblem(undefined) }} onBlur={(event) => { if (!saving && !problem && !event.currentTarget.parentElement?.contains(event.relatedTarget)) onClose(false) }} onKeyDown={(event) => {
        if (event.nativeEvent.isComposing || event.keyCode === 229) return
        if (event.key === 'Enter') { event.preventDefault(); void save() }
        if (event.key === 'Escape' && !saving) { event.preventDefault(); event.stopPropagation(); onClose(true) }
      }} />
      {problem && <p id={problemId} className="conversation-rename-problem" role="alert">{problem}</p>}
    </div>
  )
}

function shellKey(shell: ConversationProcessRef): string {
  return `${shell.pid}:${shell.procStart}`
}

function ShellCommand({ command }: { command: string }) {
  return command ? <code title={command}>{command}</code> : <span>Command hidden</span>
}

function StopShellDialog({ conversation, shell, now, onClose, onResult }: { conversation: Conversation; shell?: StuckShell; now: Date; onClose: () => void; onResult: (feedback: ShellFeedback) => void }) {
  const dialog = useRef<HTMLDialogElement>(null)
  const cancel = useRef<HTMLButtonElement>(null)
  const title = useId()
  const summary = useId()
  const [stopping, setStopping] = useState(false)

  useEffect(() => {
    dialog.current?.showModal()
    cancel.current?.focus()
  }, [])

  const stop = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (!shell || stopping) return
    setStopping(true)
    let message: string
    let failed = false
    try {
      const { stillRunning } = await stopConversationShell({ tool: conversation.tool, id: conversation.id, pid: shell.pid, procStart: shell.procStart })
      message = stillRunning?.length
        ? `Stop signal sent. Processes still running (PID${stillRunning.length === 1 ? '' : 's'} ${stillRunning.map(({ pid }) => pid).join(', ')}).`
        : 'Shell stopped.'
    } catch (error) {
      failed = true
      message = error instanceof Error ? error.message : 'Could not stop the shell.'
    }
    dialog.current?.close()
    onResult({ message, failed })
  }

  return (
    <dialog ref={dialog} className="support-dialog conversation-shell-dialog" aria-labelledby={title} aria-describedby={summary} onClose={onClose} onCancel={(event) => { if (stopping) event.preventDefault() }}>
      <form className="support-dialog-body" onSubmit={stop}>
        <div className="support-dialog-head"><h2 id={title}>Stop this shell?</h2></div>
        <div id={summary}>
          {shell ? <>
            <p>{shellWarning(shell, now)}. <ShellCommand command={shell.command} /></p>
            <p>Send a stop signal to {plural(shell.processCount, 'process', 'processes')} in this group.</p>
          </> : <p>The shell details changed. Close this dialog to choose again.</p>}
        </div>
        <div className="conversation-shell-dialog-actions">
          <button ref={cancel} type="button" className="button" disabled={stopping} onClick={() => dialog.current?.close()}>Cancel</button>
          <button type="submit" className="button button-primary" disabled={!shell || stopping}>{stopping ? 'Stopping…' : 'Stop shell'}</button>
        </div>
      </form>
    </dialog>
  )
}

// Working subagents sit under their chat, newest first. Showing the rest
// moves the focus to the first one it shows, as its button goes away.
function Helpers({ conversation, now, pending, onOpen }: { conversation: Conversation; now: Date; pending: boolean; onOpen: (helper: ConversationHelper) => void }) {
  const [showAll, setShowAll] = useState(false)
  const firstShown = useRef<HTMLButtonElement>(null)
  const focusShown = useRef(false)
  const { rows, more } = helperRows(conversation, showAll)
  useEffect(() => {
    if (!focusShown.current) return
    focusShown.current = false
    firstShown.current?.focus()
  }, [showAll])
  if (rows.length === 0) return null
  return (
    <div className="conversation-helpers">
      <ul aria-label="Working subagents">
        {rows.map((helper, index) => (
          <li key={`${helper.tool}:${helper.id}`}>
            <button ref={index === HELPERS_SHOWN ? firstShown : undefined} type="button" className="conversation-helper" title={helperTooltip(helper)} aria-disabled={pending || undefined} onClick={() => onOpen(helper)}>
              <span className="conversation-helper-title">{helperTitle(helper)}</span>
              <span className="conversation-helper-meta">{helperDetails(helper, now)}</span>
            </button>
          </li>
        ))}
      </ul>
      {more > 0 && (
        <button type="button" className="button conversation-helpers-more" onClick={() => {
          focusShown.current = true
          setShowAll(true)
        }}>
          Show {more} more {more === 1 ? 'subagent' : 'subagents'}
        </button>
      )}
    </div>
  )
}

function Withheld({ label }: { label: string }) {
  return <p className="conversation-withheld"><span className="conversation-label">{label}</span> {WITHHELD_NOTE}</p>
}
