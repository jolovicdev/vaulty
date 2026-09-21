import { useState } from 'react'
import { api } from '../lib/api'
import { Button, Dialog } from './primitives'

/** What the user was trying to do when the unsaved work got in the way. */
export type UnsavedIntent = { kind: 'quit' } | { kind: 'switch'; path: string; create: boolean }

/** Asked when leaving a vault that has changes not on disk, whether by
 *  closing the window or opening another vault.
 *
 *  This is the app's own dialog rather than an OS one because Windows can
 *  only show Yes and No for a question, and this question has three
 *  answers. Every path out of it either proceeds or returns to the app. */
export function UnsavedDialog({
  intent,
  onClose,
  onProceed,
}: {
  intent: UnsavedIntent
  onClose: () => void
  onProceed: (intent: UnsavedIntent) => void
}) {
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const verb = intent.kind === 'quit' ? 'quit' : 'switch'

  async function go(save: boolean) {
    setBusy(true)
    setError('')
    try {
      if (save) await api.save()
      if (intent.kind === 'quit') {
        await api.confirmQuit(false)
        return
      }
      onProceed(intent)
    } catch (e) {
      // A failed save keeps the app where it is: proceeding would throw
      // away the work the user just asked to keep.
      setError(e instanceof Error ? e.message : String(e))
      setBusy(false)
    }
  }

  return (
    <Dialog
      title="Unsaved changes"
      onClose={onClose}
      width="w-[460px]"
      footer={
        <div className="flex items-center gap-2">
          <Button variant="primary" disabled={busy} onClick={() => void go(true)}>
            Save and {verb}
          </Button>
          <Button variant="danger" disabled={busy} onClick={() => void go(false)}>
            Discard and {verb}
          </Button>
          <Button variant="quiet" disabled={busy} onClick={onClose}>
            Keep working
          </Button>
        </div>
      }
    >
      <div className="p-4">
        <p className="measure text-base leading-relaxed text-text-2">
          This vault has changes that are not on disk yet.
        </p>
        {error && (
          <p role="alert" className="measure mt-3 text-base leading-relaxed text-err">
            {error}
            <br />
            Nothing was discarded, so you can resolve it.
          </p>
        )}
      </div>
    </Dialog>
  )
}
