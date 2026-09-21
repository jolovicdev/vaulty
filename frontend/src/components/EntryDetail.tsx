import { useCallback, useEffect, useState } from 'react'
import {
  Eye,
  EyeOff,
  ExternalLink,
  History as HistoryIcon,
  MoreHorizontal,
  Pencil,
  RotateCcw,
  Trash2,
} from 'lucide-react'
import { api } from '../lib/api'
import { FIELD, type Detail } from '../lib/types'
import { SHORTCUTS } from '../lib/shortcuts'
import { Button, Dots, Empty, IconButton, Secret, Tooltip } from './primitives'
import { CopyButton } from './CopyButton'
import { anchorFromButton, anchorFromEvent, type MenuAnchor } from './Menu'
import { Totp } from './Totp'

/** A revealed value is held for one field of one entry. It carries the entry
 *  id so that switching entries cannot render the previous entry's secret,
 *  not even for the frame before an effect could clear it. */
interface Revealed {
  entryId: string
  field: string
  value: string
}

function formatDate(iso: string): string {
  const d = new Date(iso)
  if (!Number.isFinite(d.getTime())) return ''
  return d.toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' })
}

export function EntryDetail({
  detail,
  onEdit,
  onDelete,
  onRestore,
  onHistory,
  onMenu,
}: {
  detail: Detail | null
  onEdit: () => void
  onDelete: () => void
  onRestore: () => void
  onHistory: () => void
  onMenu: (detail: Detail, anchor: MenuAnchor) => void
}) {
  const [held, setHeld] = useState<Revealed | null>(null)
  const revealed = held && held.entryId === detail?.id ? held : null

  const toggle = useCallback(
    async (field: string) => {
      if (!detail) return
      if (held?.entryId === detail.id && held.field === field) {
        setHeld(null)
        return
      }
      try {
        setHeld({ entryId: detail.id, field, value: await api.reveal(detail.id, field) })
      } catch {
        setHeld(null)
      }
    },
    [detail, held],
  )

  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (SHORTCUTS.reveal.match(e)) {
        e.preventDefault()
        void toggle(FIELD.password)
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [toggle])

  if (!detail) {
    return (
      <section className="flex h-full min-w-0 flex-1 flex-col bg-bg">
        <Empty
          title="Nothing selected"
          action="Pick an entry, or search with"
          keys={SHORTCUTS.search.keys}
        />
      </section>
    )
  }

  return (
    <section
      className="flex h-full min-w-0 flex-1 flex-col overflow-y-auto bg-bg"
      onContextMenu={(e) => {
        // Right-clicking a text field must keep the OS menu for paste.
        if (e.target instanceof HTMLElement && e.target.closest('input, textarea')) return
        e.preventDefault()
        onMenu(detail, anchorFromEvent(e))
      }}
    >
      {/* The masthead carries the hierarchy: the title is the only large
          thing on the screen, and the machine facts sit under it in a dim
          mono line rather than in a status bar somewhere else. */}
      <header className="flex items-start gap-4 px-6 pt-6 pb-5">
        <div className="min-w-0 flex-1">
          <h1
            className={`truncate text-lg leading-tight font-semibold tracking-[-0.02em] ${
              detail.expired ? 'text-warn' : 'text-text-1'
            }`}
          >
            {detail.title || 'Untitled'}
          </h1>
          {/* Mono is for machine values only. The date is one; the group
              name, the revision count and the tags are human language. */}
          <p className="mt-[5px] flex flex-wrap items-center gap-x-2 text-sm text-text-3">
            <span className="truncate">{detail.groupPath}</span>
            <Sep />
            <span className="font-mono tabular">{formatDate(detail.modified)}</span>
            {detail.historyCount > 0 && (
              <>
                <Sep />
                <button type="button" onClick={onHistory} className="hover:text-text-1">
                  {detail.historyCount} {detail.historyCount === 1 ? 'revision' : 'revisions'}
                </button>
              </>
            )}
            {detail.tags.map((t) => (
              <span key={t} className="flex items-center gap-2">
                <Sep />
                <span>{t}</span>
              </span>
            ))}
          </p>
        </div>

        <div className="flex shrink-0 items-center gap-1 pt-[2px]">
          {detail.inRecycleBin ? (
            <Button variant="default" icon={<RotateCcw size={13} />} onClick={onRestore}>
              Restore
            </Button>
          ) : (
            <IconButton label={SHORTCUTS.edit.label} keys={SHORTCUTS.edit.keys} onClick={onEdit}>
              <Pencil size={14} />
            </IconButton>
          )}
          {detail.historyCount > 0 && (
            <IconButton
              label={SHORTCUTS.history.label}
              keys={SHORTCUTS.history.keys}
              onClick={onHistory}
            >
              <HistoryIcon size={14} />
            </IconButton>
          )}
          <IconButton
            label={detail.inRecycleBin ? 'Delete permanently' : SHORTCUTS.del.label}
            keys={SHORTCUTS.del.keys}
            onClick={onDelete}
          >
            <Trash2 size={14} />
          </IconButton>
          <IconButton
            label="All actions for this entry"
            onClick={(e) => onMenu(detail, anchorFromButton(e))}
          >
            <MoreHorizontal size={14} />
          </IconButton>
        </div>
      </header>

      {detail.inRecycleBin && (
        <p
          data-recycled-notice
          className="mx-6 mb-5 border-l-2 border-warn pl-3 text-base text-warn"
        >
          Deleting again removes this for good.
        </p>
      )}

      {/* Fields are separated by space and one hairline per group, not by a
          border around every row. */}
      <div className="flex flex-col gap-6 px-6 pb-8">
        <Group>
          {detail.username && (
            <Row label="Username">
              <span className="min-w-0 flex-1 truncate font-mono text-text-1">
                {detail.username}
              </span>
              <CopyButton
                label={SHORTCUTS.copyUsername.label}
                keys={SHORTCUTS.copyUsername.keys}
                onCopy={() => api.copyField(detail.id, FIELD.username)}
              />
            </Row>
          )}

          {detail.passwordSet && (
            <Row label="Password">
              <span className="min-w-0 flex-1 truncate">
                {revealed?.field === FIELD.password ? <Secret value={revealed.value} /> : <Dots />}
              </span>
              <RevealButton
                shown={revealed?.field === FIELD.password}
                label="password"
                keys={SHORTCUTS.reveal.keys}
                onClick={() => void toggle(FIELD.password)}
              />
              <CopyButton
                label={SHORTCUTS.copyPassword.label}
                keys={SHORTCUTS.copyPassword.keys}
                onCopy={() => api.copyField(detail.id, FIELD.password)}
              />
            </Row>
          )}

          {detail.hasTotp && (
            <Row label="One-time code">
              <Totp entryId={detail.id} />
            </Row>
          )}
        </Group>

        {(detail.url || detail.hasNotes) && (
          <Group>
            {detail.url && (
              <Row label="URL">
                <span className="min-w-0 flex-1 truncate text-text-1">{detail.url}</span>
                <IconButton
                  label={SHORTCUTS.openUrl.label}
                  keys={SHORTCUTS.openUrl.keys}
                  onClick={() => void api.openUrl(detail.id)}
                >
                  <ExternalLink size={13} />
                </IconButton>
                <CopyButton label="Copy URL" onCopy={() => api.copyField(detail.id, FIELD.url)} />
              </Row>
            )}

            {detail.hasNotes && (
              <Row label="Notes" align="start">
                <span className="min-w-0 flex-1">
                  {revealed?.field === FIELD.notes ? (
                    <span className="measure block whitespace-pre-wrap text-text-1">
                      {revealed.value}
                    </span>
                  ) : (
                    <span className="text-text-3">Hidden</span>
                  )}
                </span>
                <RevealButton
                  shown={revealed?.field === FIELD.notes}
                  label="notes"
                  onClick={() => void toggle(FIELD.notes)}
                />
              </Row>
            )}
          </Group>
        )}

        {detail.custom.length > 0 && (
          <Group>
            {detail.custom.map((f) => (
              <Row key={f.key} label={f.key}>
                <span className="min-w-0 flex-1 truncate">
                  {f.protected ? (
                    revealed?.field === f.key ? (
                      <Secret value={revealed.value} />
                    ) : (
                      <Dots />
                    )
                  ) : (
                    <span className="font-mono text-text-1">{f.value}</span>
                  )}
                </span>
                {f.protected && (
                  <>
                    <RevealButton
                      shown={revealed?.field === f.key}
                      label={f.key}
                      onClick={() => void toggle(f.key)}
                    />
                    <CopyButton
                      label={`Copy ${f.key}`}
                      onCopy={() => api.copyField(detail.id, f.key)}
                    />
                  </>
                )}
              </Row>
            ))}
          </Group>
        )}
      </div>
    </section>
  )
}

function Sep() {
  return <span className="text-line-strong">·</span>
}

/** Group is a run of related fields with one hairline above it. */
function Group({ children }: { children: React.ReactNode }) {
  return <div className="flex flex-col border-t border-line pt-[10px]">{children}</div>
}

/** Row is a label and a value on one line. The label column is fixed so
 *  every value in the pane starts at the same x. */
function Row({
  label,
  align = 'center',
  children,
}: {
  label: string
  align?: 'center' | 'start'
  children: React.ReactNode
}) {
  return (
    <div
      className={`flex gap-4 py-[6px] ${align === 'center' ? 'items-center' : 'items-start'}`}
    >
      <span className="w-[104px] shrink-0 pt-[2px] text-sm text-text-3">{label}</span>
      <span className="flex min-w-0 flex-1 items-center gap-1">{children}</span>
    </div>
  )
}

function RevealButton({
  shown,
  label,
  keys,
  onClick,
}: {
  shown: boolean | undefined
  label: string
  keys?: string
  onClick: () => void
}) {
  const text = shown ? `Hide ${label}` : `Reveal ${label}`
  return (
    <Tooltip label={text} keys={keys}>
      <button
        type="button"
        aria-label={text}
        onClick={onClick}
        className="row-transition inline-flex h-[28px] w-[28px] shrink-0 items-center justify-center rounded-sm text-text-3 hover:bg-row-hover hover:text-text-1"
      >
        {shown ? <EyeOff size={13} /> : <Eye size={13} />}
      </button>
    </Tooltip>
  )
}

/** RestoreButton is used by the history dialog, which needs the same
 *  affordance without the pane around it. */
export function RestoreButton({ onClick }: { onClick: () => void }) {
  return (
    <Button variant="default" icon={<RotateCcw size={13} />} onClick={onClick}>
      Restore
    </Button>
  )
}
