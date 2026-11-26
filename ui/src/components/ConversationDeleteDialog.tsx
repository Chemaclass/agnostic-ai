import { useEffect, useId, useRef, useState } from 'react'
import type { Conversation } from '../contract'
import { deleteConfirmOf, deleteReason } from '../model/conversation-actions'

export function ConversationDeleteDialog({ row, onClose, onConfirm }: { row: Conversation; onClose(confirmed: boolean): void; onConfirm(): Promise<void> }) {
  const dialog = useRef<HTMLDialogElement>(null)
  const cancel = useRef<HTMLButtonElement>(null)
  const initial = useRef(row.updatedAt)
  const [pending, setPending] = useState(false)
  const confirmed = useRef(false)
  const heading = useId()
  const confirm = deleteConfirmOf(row)
  const refusal = deleteReason(row) || (initial.current !== row.updatedAt ? 'The conversation changed. Close this dialog and choose again.' : undefined)
  useEffect(() => {
    dialog.current?.showModal()
    cancel.current?.focus()
  }, [])
  return <dialog ref={dialog} className="support-dialog conversation-edit-dialog" aria-labelledby={heading} onClose={() => onClose(confirmed.current)} onCancel={(event) => { if (pending) event.preventDefault() }}>
    <form className="support-dialog-body" onSubmit={async (event) => {
      event.preventDefault()
      if (pending || refusal) return
      setPending(true)
      confirmed.current = true
      await onConfirm()
      dialog.current?.close()
    }}>
      <h2 id={heading}>{confirm.title}</h2>
      <p>{refusal || confirm.body}</p>
      <div className="conversation-shell-dialog-actions">
        <button ref={cancel} type="button" className="button" disabled={pending} onClick={() => dialog.current?.close()}>Cancel</button>
        <button type="submit" className="button is-danger" disabled={pending || !!refusal}>{pending ? 'Deleting…' : 'Delete permanently'}</button>
      </div>
    </form>
  </dialog>
}
