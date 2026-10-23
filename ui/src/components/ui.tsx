// Small presentational pieces shared by every screen.

import { useEffect, useRef, useState, type ReactNode } from 'react'
import claudeIcon from '../assets/tools/claude.svg'
import codexIcon from '../assets/tools/codex.svg'
import cursorIcon from '../assets/tools/cursor.svg'

export type Tone = 'permitted' | 'approval' | 'blocked' | 'info' | 'neutral' | 'accent'

export function Badge({ tone, children, muted, title, className }: { tone: Tone; children: ReactNode; muted?: boolean; title?: string; className?: string }) {
  return (
    <span className={`badge badge-${tone}${muted ? ' badge-muted' : ''}${className ? ` ${className}` : ''}`} title={title}>
      {children}
    </span>
  )
}

/** A read-only on/off switch: it shows a boolean setting, it does not change it. */
export function Switch({ on, label }: { on: boolean; label?: string }) {
  return (
    <span className={`switch${on ? ' is-on' : ''}`}>
      <span className="switch-track" aria-hidden="true">
        <span className="switch-thumb" />
      </span>
      <span className="switch-text">{label ?? (on ? 'On' : 'Off')}</span>
    </span>
  )
}

const TOOL_ICONS: Record<string, string> = { claude: claudeIcon, codex: codexIcon, cursor: cursorIcon }

/** A coding tool's logo in a circle, or its initial when there is none; label is its tooltip. */
export function ToolAvatar({ target, label }: { target: string; label: string }) {
  const icon = TOOL_ICONS[target]
  return (
    <span className="tool-avatar" role="img" aria-label={label} title={label}>
      {icon ? <img src={icon} alt="" width={18} height={18} /> : <span aria-hidden="true">{target.charAt(0).toUpperCase()}</span>}
    </span>
  )
}

/** An on/off switch that changes a setting; pending while the change saves. */
export function Toggle({ on, label, disabled, pending, onToggle }: { on: boolean; label: string; disabled?: boolean; pending?: boolean; onToggle: () => void }) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={on}
      aria-label={label}
      aria-busy={pending || undefined}
      className={`switch switch-control${on ? ' is-on' : ''}${pending ? ' is-pending' : ''}`}
      disabled={disabled || pending}
      onClick={onToggle}
    >
      <span className="switch-track" aria-hidden="true">
        <span className="switch-thumb" />
      </span>
      <span className="switch-text">{on ? 'On' : 'Off'}</span>
    </button>
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

const ICON_PATHS: Record<string, ReactNode> = {
  context: <path d="M12 3 21 12 12 21 3 12Z M12 8 16 12 12 16 8 12Z" />,
  authority: <path d="M12 3 21 12 12 21 3 12Z" />,
  tools: <path d="M9 9h6v6H9Z M9 9V6.5A2.5 2.5 0 1 0 6.5 9H9 M15 9V6.5A2.5 2.5 0 1 1 17.5 9H15 M9 15v2.5A2.5 2.5 0 1 1 6.5 15H9 M15 15v2.5a2.5 2.5 0 1 0 2.5-2.5H15" />,
  lifecycle: <path d="M19 12a7 7 0 1 1-2.05-4.95 M19 4v4h-4" />,
  settings: <path d="M4 7h10 M18 7h2 M4 17h4 M12 17h8 M14 5v4 M10 15v4" />,
  warning: <path d="M12 4 21 20H3Z M12 10v4 M12 17h.01" />,
  sun: <path d="M12 8a4 4 0 1 1 0 8 4 4 0 0 1 0-8Z M12 2v2 M12 20v2 M4.9 4.9l1.4 1.4 M17.7 17.7l1.4 1.4 M2 12h2 M20 12h2 M4.9 19.1l1.4-1.4 M17.7 6.3l1.4-1.4" />,
  moon: <path d="M20 14.5A8 8 0 0 1 9.5 4 8 8 0 1 0 20 14.5Z" />,
  edit: <path d="M4 20h4L19 9a2.8 2.8 0 0 0-4-4L4 16Z M13.5 6.5l4 4" />,
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
