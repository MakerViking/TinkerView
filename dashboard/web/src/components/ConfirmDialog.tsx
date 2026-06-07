import { type ReactNode } from 'react'
import { Modal } from '@/components/Modal'
import { Actions } from '@/components/devices/forms'

// ConfirmDialog is a styled "are you sure?" dialog replacing the native confirm().
// Reuses the Modal + Actions primitives; the confirm button is danger-styled for
// destructive actions (deletes).
export function ConfirmDialog({
  title,
  message,
  confirmLabel = 'Confirm',
  danger,
  onConfirm,
  onClose,
}: {
  title: string
  message: ReactNode
  confirmLabel?: string
  danger?: boolean
  onConfirm: () => void
  onClose: () => void
}) {
  return (
    <Modal title={title} onClose={onClose}>
      <form
        onSubmit={(e) => {
          e.preventDefault()
          onConfirm()
          onClose()
        }}
      >
        <div className="text-sm text-slate-300">{message}</div>
        <Actions onCancel={onClose} submitLabel={confirmLabel} danger={danger} />
      </form>
    </Modal>
  )
}
