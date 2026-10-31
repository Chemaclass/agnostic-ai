// Small presentational pieces shared by every screen.

import { useEffect, useRef, useState, type ReactNode, type SelectHTMLAttributes } from 'react'
import claudeIcon from '../assets/tools/claude.svg'
import codexIcon from '../assets/tools/codex.svg'
import cursorIcon from '../assets/tools/cursor.svg'
import geminiIcon from '../assets/tools/gemini.svg'
import qwenIcon from '../assets/tools/qwen.svg'
import copilotIcon from '../assets/tools/copilot.svg'
import aiderIcon from '../assets/tools/aider.png'
import clineIcon from '../assets/tools/cline.svg'
import windsurfIcon from '../assets/tools/windsurf.svg'
import continueIcon from '../assets/tools/continue.svg'
import ampIcon from '../assets/tools/amp.svg'
import zedIcon from '../assets/tools/zed.svg'
import warpIcon from '../assets/tools/warp.svg'
import opencodeIcon from '../assets/tools/opencode.svg'
import antigravityIcon from '../assets/tools/antigravity.svg'
import junieIcon from '../assets/tools/junie.svg'
import kiroIcon from '../assets/tools/kiro.svg'
import crushIcon from '../assets/tools/crush.svg'
import traeIcon from '../assets/tools/trae.svg'
import julesIcon from '../assets/tools/jules.svg'
import gooseIcon from '../assets/tools/goose.svg'
import grokIcon from '../assets/tools/grok.svg'
import augmentIcon from '../assets/tools/augment.svg'
import qoderIcon from '../assets/tools/qoder.svg'
import vibeIcon from '../assets/tools/vibe.svg'
import kimiIcon from '../assets/tools/kimi.svg'
import piIcon from '../assets/tools/pi.svg'
import openhandsIcon from '../assets/tools/openhands.svg'
import factoryIcon from '../assets/tools/factory.svg'
import kiloIcon from '../assets/tools/kilo.svg'
import ellipsisIcon from '../assets/tools/ellipsis.svg'
import codebuddyIcon from '../assets/tools/codebuddy.svg'

export const APPLE_PLATFORM = typeof navigator !== 'undefined' && /Mac|iPhone|iPad/.test(navigator.platform)

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

const TOOL_ICONS: Record<string, { icon: string; name: string }> = {
  claude: { icon: claudeIcon, name: 'Claude Code' },
  codex: { icon: codexIcon, name: 'Codex' },
  cursor: { icon: cursorIcon, name: 'Cursor' },
  gemini: { icon: geminiIcon, name: 'Gemini CLI' },
  qwen: { icon: qwenIcon, name: 'Qwen Code' },
  copilot: { icon: copilotIcon, name: 'GitHub Copilot' },
  aider: { icon: aiderIcon, name: 'Aider' },
  cline: { icon: clineIcon, name: 'Cline' },
  windsurf: { icon: windsurfIcon, name: 'Windsurf / Devin' },
  continue: { icon: continueIcon, name: 'Continue' },
  amp: { icon: ampIcon, name: 'Amp' },
  zed: { icon: zedIcon, name: 'Zed' },
  warp: { icon: warpIcon, name: 'Warp' },
  opencode: { icon: opencodeIcon, name: 'OpenCode' },
  antigravity: { icon: antigravityIcon, name: 'Google Antigravity' },
  junie: { icon: junieIcon, name: 'Junie' },
  kiro: { icon: kiroIcon, name: 'Kiro' },
  crush: { icon: crushIcon, name: 'Crush' },
  trae: { icon: traeIcon, name: 'Trae' },
  jules: { icon: julesIcon, name: 'Jules' },
  goose: { icon: gooseIcon, name: 'Goose' },
  grok: { icon: grokIcon, name: 'Grok Build' },
  augment: { icon: augmentIcon, name: 'Augment' },
  qoder: { icon: qoderIcon, name: 'Qoder' },
  vibe: { icon: vibeIcon, name: 'Mistral Vibe' },
  kimi: { icon: kimiIcon, name: 'Kimi Code CLI' },
  pi: { icon: piIcon, name: 'Pi' },
  openhands: { icon: openhandsIcon, name: 'OpenHands' },
  factory: { icon: factoryIcon, name: 'Factory' },
  kilo: { icon: kiloIcon, name: 'Kilo Code' },
  ellipsis: { icon: ellipsisIcon, name: 'Ellipsis' },
  codebuddy: { icon: codebuddyIcon, name: 'CodeBuddy Code' },
}

export function ToolAvatar({ target, label }: { target: string; label?: string }) {
  const tool = Object.hasOwn(TOOL_ICONS, target) ? TOOL_ICONS[target] : undefined
  const name = label ?? tool?.name ?? target
  return (
    <span className="tool-avatar" data-target={target} role="img" aria-label={name} title={name}>
      {tool ? <img src={tool.icon} alt="" width={18} height={18} /> : <span aria-hidden="true">{target.charAt(0).toUpperCase()}</span>}
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

export function CopyButton({ text, label, copiedLabel = 'Copied', onCopied, className, ariaLabel }: { text: string; label: string; copiedLabel?: string; onCopied?: (ok: boolean) => void; className?: string; ariaLabel?: string }) {
  const [state, setState] = useState<'idle' | 'copied' | 'failed'>('idle')
  const timer = useRef<number | undefined>(undefined)
  useEffect(() => () => window.clearTimeout(timer.current), [])
  return (
    <button
      type="button"
      className={`button button-quiet${className ? ` ${className}` : ''}`}
      aria-label={state === 'idle' ? ariaLabel : undefined}
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

export function Select({ children, ...props }: SelectHTMLAttributes<HTMLSelectElement>) {
  return (
    <span className="select-control">
      <select {...props}>{children}</select>
      <Icon name="chevron" className="select-chevron" />
    </span>
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
  chevron: <path d="m6 9 6 6 6-6" />,
  arrow: <path d="M7 17 17 7 M9 7h8v8" />,
  search: <path d="M10.5 4a6.5 6.5 0 1 1 0 13 6.5 6.5 0 0 1 0-13Z M15.3 15.3 20 20" />,
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
