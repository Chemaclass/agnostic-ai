import { useEffect, useRef, useState, type RefObject } from 'react'

export interface ConversationFocusIntent {
  context: string
  target: 'row' | 'bulk'
  origin: Element | null
  owner: Element | null
  phase: 'post' | 'refresh'
  cancelled: boolean
}

export function useConversationFeedbackFocus(context: string, row: RefObject<HTMLParagraphElement | null>, bulk: RefObject<HTMLParagraphElement | null>) {
  const intents = useRef(new Set<ConversationFocusIntent>())
  const scheduled = useRef<ConversationFocusIntent | undefined>(undefined)
  const ready = useRef<ConversationFocusIntent | undefined>(undefined)
  const currentContext = useRef(context)
  const [revision, setRevision] = useState(0)
  currentContext.current = context
  for (const intent of intents.current) if (intent.context !== context) intent.cancelled = true

  useEffect(() => {
    const intent = ready.current
    ready.current = undefined
    if (!intent) return
    intents.current.delete(intent)
    if (!intent.cancelled && intent.context === currentContext.current) (intent.target === 'row' ? row : bulk).current?.focus()
  }, [revision, row, bulk])

  useEffect(() => {
    const cancel = () => { for (const intent of intents.current) intent.cancelled = true }
    const focus = (event: FocusEvent) => {
      if (!(event.target instanceof Node)) return
      for (const intent of intents.current) {
        if (event.target === intent.origin) continue
        if (intent.phase === 'refresh' && intent.target === 'row' && intent.owner?.contains(event.target)) continue
        intent.cancelled = true
      }
    }
    document.addEventListener('pointerdown', cancel, true)
    document.addEventListener('keydown', cancel, true)
    document.addEventListener('input', cancel, true)
    document.addEventListener('focusin', focus, true)
    return () => {
      document.removeEventListener('pointerdown', cancel, true)
      document.removeEventListener('keydown', cancel, true)
      document.removeEventListener('input', cancel, true)
      document.removeEventListener('focusin', focus, true)
    }
  }, [])

  return {
    start(target: ConversationFocusIntent['target']): ConversationFocusIntent {
      const origin = document.activeElement
      const intent: ConversationFocusIntent = { context: currentContext.current, target, origin, owner: origin?.closest('li.conversation') ?? null, phase: 'post', cancelled: false }
      intents.current.add(intent)
      return intent
    },
    schedule(intent: ConversationFocusIntent) {
      if (scheduled.current) { scheduled.current.cancelled = true; intents.current.delete(scheduled.current) }
      intent.phase = 'refresh'
      scheduled.current = intent
    },
    take() {
      const intent = scheduled.current
      scheduled.current = undefined
      return intent
    },
    complete(intent?: ConversationFocusIntent) {
      if (!intent) return
      ready.current = intent
      setRevision((current) => current + 1)
    },
    abandon(intent?: ConversationFocusIntent) {
      if (!intent) return
      intent.cancelled = true
      intents.current.delete(intent)
    },
  }
}
