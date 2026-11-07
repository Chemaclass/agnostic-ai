import { useEffect, useRef, type KeyboardEvent } from 'react'

export interface SectionTabLabel {
  id: string
  label: string
  title?: string
  count?: string | number
  findings: number
}

// Matches the faded edge in components.css, so the selected tab never sits under it.
const EDGE = 32

export function SectionTabs({ section, label, tabs, active, select }: { section: string; label: string; tabs: SectionTabLabel[]; active: string; select: (id: string) => void }) {
  const list = useRef<HTMLDivElement>(null)
  useEffect(() => {
    const container = list.current
    const tab = document.getElementById(`tab-${section}-${active}`)
    if (!container || !tab) return
    // A faded edge hints that more tabs scroll into view on that side.
    const fade = () => {
      container.toggleAttribute('data-more-start', container.scrollLeft > 1)
      container.toggleAttribute('data-more-end', container.scrollLeft + container.clientWidth < container.scrollWidth - 1)
    }
    const reveal = () => {
      const viewport = container.getBoundingClientRect()
      const selected = tab.getBoundingClientRect()
      if (selected.left < viewport.left + EDGE) container.scrollLeft -= Math.ceil(viewport.left + EDGE - selected.left)
      else if (selected.right > viewport.right - EDGE) container.scrollLeft += Math.ceil(selected.right - viewport.right + EDGE)
      fade()
    }
    reveal()
    const observer = new ResizeObserver(reveal)
    observer.observe(container)
    for (const child of container.children) observer.observe(child)
    container.addEventListener('scroll', fade, { passive: true })
    return () => {
      observer.disconnect()
      container.removeEventListener('scroll', fade)
    }
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
      title={tab.title}
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
