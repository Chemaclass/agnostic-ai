import { useEffect, useRef, type KeyboardEvent } from 'react'

export interface SectionTabLabel {
  id: string
  label: string
  count?: string | number
  findings: number
}

export function SectionTabs({ section, label, tabs, active, select }: { section: string; label: string; tabs: SectionTabLabel[]; active: string; select: (id: string) => void }) {
  const list = useRef<HTMLDivElement>(null)
  useEffect(() => {
    const container = list.current
    const tab = document.getElementById(`tab-${section}-${active}`)
    if (!container || !tab) return
    const reveal = () => {
      const viewport = container.getBoundingClientRect()
      const selected = tab.getBoundingClientRect()
      if (selected.left < viewport.left + 8) container.scrollLeft -= Math.ceil(viewport.left + 8 - selected.left)
      else if (selected.right > viewport.right - 8) container.scrollLeft += Math.ceil(selected.right - viewport.right + 8)
    }
    reveal()
    const observer = new ResizeObserver(reveal)
    observer.observe(container)
    for (const child of container.children) observer.observe(child)
    return () => observer.disconnect()
  }, [active, section, tabs])
  const keyDown = (event: KeyboardEvent<HTMLButtonElement>, index: number) => {
    let next: number
    if (event.key === 'ArrowRight') next = (index + 1) % tabs.length
    else if (event.key === 'ArrowLeft') next = (index - 1 + tabs.length) % tabs.length
    else if (event.key === 'Home') next = 0
    else if (event.key === 'End') next = tabs.length - 1
    else return
    event.preventDefault()
    select(tabs[next].id)
    const tab = document.getElementById(`tab-${section}-${tabs[next].id}`)
    tab?.focus({ preventScroll: true })
  }
  return <div ref={list} className="section-tabs" data-section={section} role="tablist" aria-label={`${label} views`}>
    {tabs.map((tab, index) => <button
      key={tab.id}
      type="button"
      role="tab"
      id={`tab-${section}-${tab.id}`}
      aria-controls={`panel-${section}-${tab.id}`}
      aria-selected={active === tab.id}
      tabIndex={active === tab.id ? 0 : -1}
      onClick={() => select(tab.id)}
      onKeyDown={(event) => keyDown(event, index)}
    >
      <span>{tab.label}</span>
      {tab.count !== undefined && <span className="section-tab-count">{tab.count}</span>}
      {tab.findings > 0 && <span className="section-tab-findings" aria-label={`${tab.findings} findings`} title={`${tab.findings} findings`}>{tab.findings}<span className="sr-only"> findings</span></span>}
    </button>)}
  </div>
}
