import { type ReactNode, useCallback, useState } from 'react'
import { Modal } from '@/components/Modal'
import { Button } from '@/components/ui'
import { IconWarning } from '@/components/icons'

// ConfirmDialog is a styled "are you sure?" dialog replacing the native confirm().
// The confirm button is danger-styled for destructive actions (deletes/stops).
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
        <div className="flex gap-3">
          {danger && (
            <div className="grid h-9 w-9 shrink-0 place-items-center rounded-md bg-rec/10 text-rec">
              <IconWarning className="text-[18px]" />
            </div>
          )}
          <div className="text-sm leading-relaxed text-ink-200">{message}</div>
        </div>
        <div className="mt-5 flex justify-end gap-2">
          <Button type="button" variant="ghost" onClick={onClose}>
            Cancel
          </Button>
          <Button type="submit" variant={danger ? 'danger' : 'primary'} autoFocus>
            {confirmLabel}
          </Button>
        </div>
      </form>
    </Modal>
  )
}

interface ConfirmRequest {
  title: string
  message: ReactNode
  confirmLabel?: string
  danger?: boolean
  onConfirm: () => void
}

// useConfirm gives any component an imperative, styled replacement for the
// native confirm(): call ask({...}) and render `confirmNode` somewhere in JSX.
export function useConfirm() {
  const [req, setReq] = useState<ConfirmRequest | null>(null)
  const ask = useCallback((r: ConfirmRequest) => setReq(r), [])
  const confirmNode = req ? (
    <ConfirmDialog
      title={req.title}
      message={req.message}
      confirmLabel={req.confirmLabel}
      danger={req.danger}
      onConfirm={req.onConfirm}
      onClose={() => setReq(null)}
    />
  ) : null
  return { ask, confirmNode }
}
