import { useEffect, useId, useLayoutEffect, useRef, useState } from 'react'
import { Icon } from './ui'

export interface ConversationMenuAction {
  id: string
  label: string
  description?: string
  danger?: boolean
  run(): void
}

export function ConversationMenu({ title, actions }: { title: string; actions: ConversationMenuAction[] }) {
  const [open, setOpen] = useState(false)
  const [up, setUp] = useState(false)
  const root = useRef<HTMLSpanElement>(null)
  const trigger = useRef<HTMLButtonElement>(null)
  const panel = useRef<HTMLDivElement>(null)
  const id = useId()
  useLayoutEffect(() => {
    if (!open || !panel.current) return
    setUp(panel.current.getBoundingClientRect().bottom > innerHeight - 12)
    panel.current.querySelector<HTMLButtonElement>('[role="menuitem"]')?.focus()
  }, [open])
  useEffect(() => {
    if (!open) return
    const outside = (event: PointerEvent) => { if (!root.current?.contains(event.target as Node)) setOpen(false) }
    document.addEventListener('pointerdown', outside)
    return () => document.removeEventListener('pointerdown', outside)
  }, [open])
  const draw = (items: ConversationMenuAction[]) => items.map((action) => <button key={action.id} type="button" role="menuitem" className={`button button-quiet${action.danger ? ' is-danger' : ''}`} title={action.description} onClick={() => {
    setOpen(false)
    trigger.current?.focus()
    action.run()
  }}>{action.label}</button>)
  return <span ref={root} className="row-menu conversation-menu" onKeyDown={(event) => {
    if (!open) {
      if (event.key === 'ArrowDown') { event.preventDefault(); setUp(false); setOpen(true) }
      return
    }
    if (event.key === 'Escape') { event.preventDefault(); event.stopPropagation(); setOpen(false); trigger.current?.focus(); return }
    if (event.key === 'Tab') { setOpen(false); return }
    const items = Array.from(panel.current?.querySelectorAll<HTMLButtonElement>('[role="menuitem"]') ?? [])
    const index = items.indexOf(document.activeElement as HTMLButtonElement)
    let next: number | undefined
    if (event.key === 'ArrowDown') next = (index + 1) % items.length
    if (event.key === 'ArrowUp') next = (index - 1 + items.length) % items.length
    if (event.key === 'Home') next = 0
    if (event.key === 'End') next = items.length - 1
    if (next !== undefined) { event.preventDefault(); items[next]?.focus() }
  }}>
    <button ref={trigger} type="button" className="button button-icon row-menu-button" aria-label={`More actions for ${title}`} aria-haspopup="menu" aria-expanded={open} aria-controls={id} onClick={() => { setUp(false); setOpen(!open) }}><Icon name="more" /></button>
    <div ref={panel} id={id} role="menu" aria-label={`Actions for ${title}`} className={`row-menu-panel${up ? ' is-up' : ''}`} hidden={!open}>
      <div role="group" aria-label="Conversation actions">{draw(actions.filter((action) => !action.danger))}</div>
      {actions.some((action) => action.danger) && <><div role="separator" /><div role="group" aria-label="Danger zone">{draw(actions.filter((action) => action.danger))}</div></>}
    </div>
  </span>
}
