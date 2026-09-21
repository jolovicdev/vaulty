import { useEffect, useRef } from 'react'
import { Clock, MoreHorizontal, Plus, Search, Timer, X } from 'lucide-react'
import type { Meta } from '../lib/types'
import { Empty, IconButton, Keys, PaneHeader } from './primitives'
import { anchorFromButton, anchorFromEvent, type MenuAnchor } from './Menu'
import { SHORTCUTS } from '../lib/shortcuts'

/** Relative time, because an exact timestamp in a list is noise. */
function ago(iso: string): string {
  const then = new Date(iso).getTime()
  if (!Number.isFinite(then)) return ''
  const days = Math.floor((Date.now() - then) / 86_400_000)
  if (days <= 0) return 'today'
  if (days === 1) return 'yesterday'
  if (days < 30) return `${days}d`
  if (days < 365) return `${Math.floor(days / 30)}mo`
  return `${Math.floor(days / 365)}y`
}

export function EntryList({
  entries,
  selectedId,
  query,
  searching,
  onQuery,
  onSelect,
  onNew,
  onRowMenu,
}: {
  entries: Meta[]
  selectedId: string
  query: string
  searching: boolean
  onQuery: (q: string) => void
  onSelect: (id: string) => void
  onNew: () => void
  onRowMenu: (entry: Meta, anchor: MenuAnchor) => void
}) {
  const searchInput = useRef<HTMLInputElement>(null)
  const listRef = useRef<HTMLUListElement>(null)

  // Keep the selected row in view when the selection moves by keyboard.
  useEffect(() => {
    listRef.current
      ?.querySelector(`[data-id="${selectedId}"]`)
      ?.scrollIntoView({ block: 'nearest' })
  }, [selectedId])

  useEffect(() => {
    function focusSearch(e: KeyboardEvent) {
      if (SHORTCUTS.search.match(e)) {
        e.preventDefault()
        searchInput.current?.focus()
        searchInput.current?.select()
      }
    }
    window.addEventListener('keydown', focusSearch)
    return () => window.removeEventListener('keydown', focusSearch)
  }, [])

  return (
    <section className="flex h-full w-[318px] shrink-0 flex-col border-r border-line bg-bg">
      <PaneHeader>
        <Search size={14} className="shrink-0 text-text-3" />
        <input
          ref={searchInput}
          value={query}
          onChange={(e) => onQuery(e.target.value)}
          placeholder="Search"
          spellCheck={false}
          aria-label="Search entries"
          className="min-w-0 flex-1 bg-transparent text-base text-text-1 outline-none"
        />
        {query ? (
          <IconButton label="Clear search" onClick={() => onQuery('')}>
            <X size={13} />
          </IconButton>
        ) : (
          <Keys keys={SHORTCUTS.search.keys} />
        )}
        <IconButton label={SHORTCUTS.newEntry.label} keys={SHORTCUTS.newEntry.keys} onClick={onNew}>
          <Plus size={15} />
        </IconButton>
      </PaneHeader>

      {entries.length === 0 ? (
        searching ? (
          <Empty title={`No entry matches "${query}"`} action="Try a shorter search" />
        ) : (
          <Empty
            title="No entries here yet"
            action="Create the first one with"
            keys={SHORTCUTS.newEntry.keys}
          />
        )
      ) : (
        <ul ref={listRef} className="min-h-0 flex-1 overflow-y-auto" role="listbox" aria-label="Entries">
          {entries.map((e) => {
            const active = e.id === selectedId
            return (
              <li key={e.id} className="group relative">
                <button
                  type="button"
                  data-id={e.id}
                  role="option"
                  aria-selected={active}
                  onClick={() => onSelect(e.id)}
                  onContextMenu={(ev) => {
                    ev.preventDefault()
                    onSelect(e.id)
                    onRowMenu(e, anchorFromEvent(ev))
                  }}
                  className={`row-transition relative flex h-[var(--row-h)] w-full items-center gap-2 border-b border-line/60 px-3 text-left ${
                    active ? 'bg-selected' : 'hover:bg-row-hover'
                  }`}
                >
                  {active && (
                    <span className="absolute top-0 bottom-0 left-0 w-[2px] bg-accent" />
                  )}
                  <span className="flex min-w-0 flex-1 flex-col">
                    <span
                      className={`truncate text-base leading-[16px] ${
                        e.expired ? 'text-warn' : 'text-text-1'
                      }`}
                    >
                      {e.title || 'Untitled'}
                    </span>
                    {e.username && (
                      <span className="truncate font-mono text-[11px] leading-[14px] text-text-3">
                        {e.username}
                      </span>
                    )}
                  </span>
                  {e.hasTotp && (
                    <Timer size={12} className="shrink-0 text-text-3" aria-label="has a one-time code" />
                  )}
                  <span className="flex w-[34px] shrink-0 items-center justify-end gap-1 text-[11px] text-text-3 tabular group-hover:invisible">
                    {e.expired ? <Clock size={11} className="text-warn" /> : null}
                    {ago(e.modified)}
                  </span>
                </button>
                {/* Visible on hover and on keyboard focus, so the actions
                    are discoverable without knowing a shortcut. */}
                <span className="absolute top-[3px] right-1 opacity-0 group-hover:opacity-100 focus-within:opacity-100">
                  <IconButton
                    label={`Actions for ${e.title || 'this entry'}`}
                    onClick={(ev) => {
                      onSelect(e.id)
                      onRowMenu(e, anchorFromButton(ev))
                    }}
                  >
                    <MoreHorizontal size={14} />
                  </IconButton>
                </span>
              </li>
            )
          })}
        </ul>
      )}

      <div className="flex h-[26px] shrink-0 items-center border-t border-line px-3 text-[11px] text-text-3 tabular">
        {entries.length} {entries.length === 1 ? 'entry' : 'entries'}
      </div>
    </section>
  )
}
