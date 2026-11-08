import { createContext, useContext, type ReactNode } from 'react'
import { createPortal } from 'react-dom'

/** The end of the page toolbar, where a page puts its one primary action. */
export const ToolbarActionSlot = createContext<HTMLElement | null>(null)

// A portal lets the page body keep the state its action opens, such as a form under the toolbar.
export function ToolbarAction({ children }: { children: ReactNode }) {
  const slot = useContext(ToolbarActionSlot)
  return slot ? createPortal(children, slot) : null
}
