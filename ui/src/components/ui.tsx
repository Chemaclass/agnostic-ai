// Small presentational pieces shared by every screen.

import { useEffect, useRef, useState, type ReactNode } from 'react'
import type { Effect, Operation } from '../contract'
import { declaredWording, effectTone, effectWording, type Tone } from '../model/effects'

export function Badge({ tone, children, muted, title, className }: { tone: Tone; children: ReactNode; muted?: boolean; title?: string; className?: string }) {
  return (
    <span className={`badge badge-${tone}${muted ? ' badge-muted' : ''}${className ? ` ${className}` : ''}`} title={title}>
      {children}
    </span>
  )
}

const GLYPHS: Record<Effect, string> = {
  allowed: '✓',
  ask: '?',
  denied: '✕',
  unavailable: '∅',
  default: '–',
  unresolved: '…',
}

export function effectGlyph(effect: Effect): string {
  return GLYPHS[effect] ?? '•'
}

/** A badge for one effect, worded conditionally when hypothetical. */
export function EffectBadge({ effect, hypothetical, text, title, muted }: { effect: Effect; hypothetical?: boolean; text?: string; title?: string; muted?: boolean }) {
  return (
    <Badge tone={effectTone(effect)} title={title} muted={muted}>
      <span className="badge-glyph" aria-hidden="true">
        {effectGlyph(effect)}
      </span>
      {text ?? effectWording(effect, hypothetical)}
    </Badge>
  )
}

/** Two declared effects with an arrow: the specs' decision, never an effect claim. */
export function DeclaredTransition({ before, after }: { before: Effect; after: Effect }) {
  return (
    <span className="transition">
      <EffectBadge effect={before} text={declaredWording(before)} muted />
      <span className="transition-arrow" aria-label="becomes">
        →
      </span>
      <EffectBadge effect={after} text={declaredWording(after)} muted />
    </span>
  )
}

/** Two effects with an arrow, for before → after. */
export function EffectTransition({ before, after, hypothetical }: { before: Effect; after: Effect; hypothetical: boolean }) {
  return (
    <span className="transition">
      <EffectBadge effect={before} hypothetical={hypothetical && before !== 'default'} />
      <span className="transition-arrow" aria-label="becomes">
        →
      </span>
      <EffectBadge effect={after} hypothetical={hypothetical && after !== 'default'} />
    </span>
  )
}

export function Chip({ children, title, tone = 'neutral' }: { children: ReactNode; title?: string; tone?: Tone }) {
  return (
    <span className={`chip chip-${tone}`} title={title}>
      {children}
    </span>
  )
}

export function Code({ children }: { children: ReactNode }) {
  return <code className="code">{children}</code>
}

/** Copies text with the Clipboard API, falling back to a hidden textarea. */
export async function copyText(text: string): Promise<boolean> {
  try {
    if (navigator.clipboard && window.isSecureContext) {
      await navigator.clipboard.writeText(text)
      return true
    }
  } catch {
    // Fall back below.
  }
  try {
    const area = document.createElement('textarea')
    area.value = text
    area.setAttribute('readonly', '')
    area.style.position = 'fixed'
    area.style.opacity = '0'
    document.body.appendChild(area)
    area.select()
    const ok = document.execCommand('copy')
    document.body.removeChild(area)
    return ok
  } catch {
    return false
  }
}

export function CopyButton({ text, label, copiedLabel = 'Copied', onCopied, className }: { text: string; label: string; copiedLabel?: string; onCopied?: (ok: boolean) => void; className?: string }) {
  const [state, setState] = useState<'idle' | 'copied' | 'failed'>('idle')
  const timer = useRef<number | undefined>(undefined)
  useEffect(() => () => window.clearTimeout(timer.current), [])
  return (
    <button
      type="button"
      className={`button button-quiet${className ? ` ${className}` : ''}`}
      onClick={async () => {
        const ok = await copyText(text)
        setState(ok ? 'copied' : 'failed')
        onCopied?.(ok)
        window.clearTimeout(timer.current)
        timer.current = window.setTimeout(() => setState('idle'), 2000)
      }}
    >
      {state === 'copied' ? copiedLabel : state === 'failed' ? 'Copy failed' : label}
    </button>
  )
}

export function useMediaQuery(query: string): boolean {
  const get = () => (typeof window !== 'undefined' && window.matchMedia ? window.matchMedia(query).matches : false)
  const [matches, setMatches] = useState(get)
  useEffect(() => {
    if (!window.matchMedia) return
    const list = window.matchMedia(query)
    const onChange = () => setMatches(list.matches)
    onChange()
    list.addEventListener('change', onChange)
    return () => list.removeEventListener('change', onChange)
  }, [query])
  return matches
}

export function useReducedMotion(): boolean {
  return useMediaQuery('(prefers-reduced-motion: reduce)')
}

/** Small counts by effect, worded conditionally when anything is hypothetical. */
export function EffectCounts({ counts, hypothetical, compact }: { counts: Record<Effect, number>; hypothetical: boolean; compact?: boolean }) {
  const order: Effect[] = ['allowed', 'ask', 'denied', 'unavailable', 'default', 'unresolved']
  const words: Record<Effect, string> = {
    allowed: 'permitted',
    ask: 'need approval',
    denied: 'blocked',
    unavailable: 'unavailable',
    default: 'no rule',
    unresolved: 'unresolved',
  }
  const present = order.filter((e) => counts[e] > 0)
  const total = present.reduce((sum, e) => sum + counts[e], 0)
  if (total === 0) return <p className="counts counts-empty">No operations here</p>
  return (
    <div className={`counts${compact ? ' counts-compact' : ''}`}>
      <div className="count-bar" aria-hidden="true">
        {present.map((e) => (
          <span key={e} className={`count-seg seg-${effectTone(e)}`} style={{ flexGrow: counts[e] }} />
        ))}
      </div>
      <p className="count-text">
        {hypothetical && <span className="count-condition">Once synced: </span>}
        {present.map((e, i) => (
          <span key={e} className={`count-item tone-${effectTone(e)}`}>
            {i > 0 && <span className="sep" aria-hidden="true"> · </span>}
            {counts[e]} {words[e]}
          </span>
        ))}
      </p>
    </div>
  )
}

/** One dot per operation, coloured by effect, with the label beside it. */
export function OperationList({ operations }: { operations: Operation[] }) {
  return (
    <ul className="op-list">
      {operations.map((op) => (
        <li key={op.id}>
          <span className={`op-dot dot-${effectTone(op.effect)}`} aria-hidden="true" />
          <span className="sr-only">{effectWording(op.effect, op.hypothetical)}: </span>
          {op.label}
        </li>
      ))}
    </ul>
  )
}

const ICON_PATHS: Record<string, ReactNode> = {
  context: <path d="M12 3 21 12 12 21 3 12Z M12 8 16 12 12 16 8 12Z" />,
  authority: <path d="M12 3 21 12 12 21 3 12Z" />,
  tools: <path d="M9 9h6v6H9Z M9 9V6.5A2.5 2.5 0 1 0 6.5 9H9 M15 9V6.5A2.5 2.5 0 1 1 17.5 9H15 M9 15v2.5A2.5 2.5 0 1 1 6.5 15H9 M15 15v2.5a2.5 2.5 0 1 0 2.5-2.5H15" />,
  lifecycle: <path d="M19 12a7 7 0 1 1-2.05-4.95 M19 4v4h-4" />,
  settings: <path d="M4 7h10 M18 7h2 M4 17h4 M12 17h8 M14 5v4 M10 15v4" />,
  warning: <path d="M12 4 21 20H3Z M12 10v4 M12 17h.01" />,
  arrow: <path d="M7 17 17 7 M9 7h8v8" />,
}

export function Icon({ name, className }: { name: string; className?: string }) {
  return (
    <svg className={`icon${className ? ` ${className}` : ''}`} viewBox="0 0 24 24" width="20" height="20" aria-hidden="true" focusable="false">
      <g fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
        {ICON_PATHS[name] ?? ICON_PATHS.authority}
      </g>
    </svg>
  )
}
