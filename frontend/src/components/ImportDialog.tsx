import { useEffect, useState } from 'react'
import { api } from '../lib/api'
import type { ImportPreview } from '../lib/types'
import { Button, Dialog } from './primitives'

/** Imports another password manager's export. It says what the file holds
 *  before anything is written, because the result lands in the vault in one
 *  save, and names what could not come across. */
export function ImportDialog({
  path,
  onClose,
  onImported,
}: {
  path: string
  onClose: () => void
  onImported: () => void
}) {
  const [preview, setPreview] = useState<ImportPreview | null>(null)
  const [busy, setBusy] = useState(false)
  const [done, setDone] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    api
      .previewImport(path)
      .then(setPreview)
      .catch((e: unknown) => setError(e instanceof Error ? e.message : String(e)))
  }, [path])

  async function run() {
    setBusy(true)
    setError('')
    try {
      await api.importFile(path)
      onImported()
      setDone(true)
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(false)
    }
  }

  const entries = preview && `${preview.entries} ${preview.entries === 1 ? 'entry' : 'entries'}`

  return (
    <Dialog
      title={preview ? `Import from ${preview.source}` : 'Import'}
      onClose={onClose}
      width="w-[500px]"
      footer={
        <div className="flex items-center gap-2">
          {done ? (
            <Button variant="primary" onClick={onClose}>
              Done
            </Button>
          ) : (
            <>
              <Button variant="primary" disabled={!preview || busy} onClick={() => void run()}>
                {busy ? 'Importing' : `Import ${entries ?? ''}`.trim()}
              </Button>
              <Button variant="quiet" onClick={onClose}>
                Cancel
              </Button>
            </>
          )}
        </div>
      }
    >
      <div className="flex flex-col gap-4 p-4">
        <p className="truncate font-mono text-sm text-text-3">{path}</p>

        {preview && !done && (
          <p className="measure text-base leading-relaxed text-text-2">
            {entries} go into a new group, {preview.group}. Each vault or folder in the export
            becomes a group inside it.
          </p>
        )}
        {done && preview && (
          <p className="measure text-base leading-relaxed text-text-2">
            Imported {entries} into the group {preview.group}. The export file still holds every
            password unencrypted, so delete it once you have checked the entries here.
          </p>
        )}

        {preview && preview.warnings.length > 0 && (
          <ul className="flex flex-col gap-1 border-t border-line pt-4">
            {preview.warnings.map((w) => (
              <li key={w} className="text-base text-warn">
                {w}
              </li>
            ))}
          </ul>
        )}

        {error && (
          <p role="alert" className="text-base text-err">
            {error}
          </p>
        )}
      </div>
    </Dialog>
  )
}
