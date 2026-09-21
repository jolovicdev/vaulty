import { useState } from 'react'
import { api } from '../lib/api'
import type { ConflictChoice, ConflictResult } from '../lib/types'
import { Button, Dialog } from './primitives'

const CHOICES: { id: ConflictChoice; label: string; detail: string }[] = [
  {
    id: 'merge',
    label: 'Merge the two',
    detail:
      'Entry by entry, the newer modification time wins. Entries only on disk are added. Nothing is discarded silently.',
  },
  {
    id: 'copy',
    label: 'Save a copy',
    detail: 'Keeps both files. This session is written beside the vault with a timestamp.',
  },
  {
    id: 'reload',
    label: 'Reload from disk',
    detail: 'Throws away the changes made in this session and reads the file again.',
  },
]

/** Shown when the vault file changed underneath us, which is what a sync
 *  client does. Nothing is written until the user picks one of the three. */
export function ConflictDialog({
  onClose,
  onResolved,
}: {
  onClose: () => void
  onResolved: (r: ConflictResult) => void
}) {
  const [busy, setBusy] = useState<ConflictChoice | null>(null)
  const [error, setError] = useState('')

  async function choose(choice: ConflictChoice) {
    setBusy(choice)
    setError('')
    try {
      onResolved(await api.resolveConflict(choice))
      onClose()
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(null)
    }
  }

  return (
    <Dialog
      title="The vault changed on disk"
      onClose={onClose}
      width="w-[540px]"
    >
      <div className="flex flex-col gap-4 p-4">
        <p className="max-w-[34rem] text-base leading-relaxed text-text-2">
          Something else wrote to this file after it was opened here, most likely a sync client.
          Your changes have not been saved, and the file has not been touched.
        </p>

        {/* The button is the only place each action is named; a heading
            above it would say the same thing twice. */}
        <ul className="flex flex-col gap-4">
          {CHOICES.map((c) => (
            <li key={c.id} className="flex flex-col gap-2 border-t border-line pt-4">
              <Button
                variant={c.id === 'merge' ? 'primary' : 'default'}
                disabled={busy !== null}
                onClick={() => void choose(c.id)}
                className="self-start"
              >
                {busy === c.id ? 'Working' : c.label}
              </Button>
              <p className="measure text-base leading-relaxed text-text-3">{c.detail}</p>
            </li>
          ))}
        </ul>

        {error && (
          <p role="alert" className="text-base text-err">
            {error}
          </p>
        )}
      </div>
    </Dialog>
  )
}
