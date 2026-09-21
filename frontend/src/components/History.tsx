import { useEffect, useState } from 'react'
import { api } from '../lib/api'
import { FIELD, type HistoryVersion } from '../lib/types'
import { Dialog, Empty, Secret } from './primitives'
import { RestoreButton } from './EntryDetail'

function formatDate(iso: string): string {
  const d = new Date(iso)
  if (!Number.isFinite(d.getTime())) return 'unknown'
  return d.toLocaleString(undefined, {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  })
}

export function History({
  entryId,
  onClose,
  onRestored,
}: {
  entryId: string
  onClose: () => void
  onRestored: () => void
}) {
  const [versions, setVersions] = useState<HistoryVersion[]>([])
  const [revealed, setRevealed] = useState<{ index: number; value: string } | null>(null)

  useEffect(() => {
    void (async () => {
      setVersions(await api.history(entryId))
    })()
  }, [entryId])

  async function restore(index: number) {
    await api.restoreVersion(entryId, index)
    onRestored()
    onClose()
  }

  return (
    <Dialog
      title="Entry history"
      description={`${versions.length} earlier ${versions.length === 1 ? 'version' : 'versions'}`}
      onClose={onClose}
      width="w-[560px]"
    >
      {versions.length === 0 ? (
        <div className="h-[160px]">
          <Empty title="This entry has never been edited" />
        </div>
      ) : (
        <ul>
          {[...versions].reverse().map((v) => (
            <li
              key={v.index}
              className="flex items-center gap-3 border-b border-line/60 px-4 py-[9px] last:border-0"
            >
              <span className="flex min-w-0 flex-1 flex-col">
                <span className="truncate text-base text-text-1">{v.title || 'Untitled'}</span>
                <span className="flex items-center gap-2 text-sm text-text-3">
                  <span className="font-mono tabular">{formatDate(v.modified)}</span>
                  {v.username && <span className="truncate font-mono">{v.username}</span>}
                </span>
              </span>
              {revealed?.index === v.index ? (
                <Secret value={revealed.value} className="text-sm" />
              ) : (
                <button
                  type="button"
                  onClick={() =>
                    void (async () =>
                      setRevealed({
                        index: v.index,
                        value: await api.revealHistory(entryId, v.index, FIELD.password),
                      }))()
                  }
                  className="shrink-0 text-sm text-text-3 hover:text-text-1"
                >
                  Show password
                </button>
              )}
              <RestoreButton onClick={() => void restore(v.index)} />
            </li>
          ))}
        </ul>
      )}
    </Dialog>
  )
}
