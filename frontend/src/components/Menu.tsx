import { useEffect, useLayoutEffect, useRef, useState, type ReactNode } from 'react'
import { Keys } from './primitives'

export interface MenuItem {
  id: string
  label: string
  /** The shortcut that does the same thing, shown dim beside the label. It
   *  is how people learn the keys: use the menu three times and stop. */
  keys?: string
  icon?: ReactNode
  /** A dim machine value beside the label, such as a file path. */
  hint?: string
  danger?: boolean
  run: () => void
}

/** A separator between groups of items. */
export const MENU_SEPARATOR = { id: '-', separator: true } as const

export type MenuEntry = MenuItem | typeof MENU_SEPARATOR

function isItem(e: MenuEntry): e is MenuItem {
  return !('separator' in e)
}

export interface MenuAnchor {
  x: number
  y: number
  /** Opens to the left when the anchor is a button rather than a pointer, so
   *  the menu does not run off the edge it was opened from. */
  fromRight?: boolean
  /** The trigger's own height, so a flipped menu clears the button instead
   *  of covering it. */
  height?: number
}

/** One menu, three triggers: right-click on a row, the hover button at the
 *  row's edge, and the footer's app-level button. Because the webview's own
 *  context menu is switched off, this is the only menu that appears. */
export function Menu({
  entries,
  anchor,
  onClose,
}: {
  entries: MenuEntry[]
  anchor: MenuAnchor
  onClose: () => void
}) {
  const items = entries.filter(isItem)
  const [cursor, setCursor] = useState(0)
  const [pos, setPos] = useState({ left: anchor.x, top: anchor.y })
  const panel = useRef<HTMLDivElement>(null)

  // Measure, then clamp to the window, so a menu opened near the bottom
  // right corner is not cut off.
  useLayoutEffect(() => {
    const el = panel.current
    if (!el) return
    const margin = 6
    const box = el.getBoundingClientRect()

    // Preferred placement: below and to the right of the anchor, or above
    // and to the left when it would not fit.
    let left = anchor.fromRight ? anchor.x - box.width : anchor.x
    let top = anchor.y
    if (top + box.height > window.innerHeight - margin) {
      top = anchor.y - box.height - (anchor.fromRight ? anchor.height ?? 0 : 0)
    }

    // Then clamp to the window regardless. The flip alone is not enough:
    // a footer button's anchor is itself within a few pixels of the bottom
    // edge, so placing the menu's bottom there still overflows.
    const maxLeft = window.innerWidth - margin - box.width
    const maxTop = window.innerHeight - margin - box.height
    left = Math.max(margin, Math.min(left, maxLeft))
    top = Math.max(margin, Math.min(top, maxTop))
    setPos({ left, top })
  }, [anchor.x, anchor.y, anchor.fromRight, anchor.height, entries.length])

  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.key === 'Escape') {
        e.preventDefault()
        e.stopPropagation()
        onClose()
      } else if (e.key === 'ArrowDown') {
        e.preventDefault()
        setCursor((c) => (c + 1) % items.length)
      } else if (e.key === 'ArrowUp') {
        e.preventDefault()
        setCursor((c) => (c - 1 + items.length) % items.length)
      } else if (e.key === 'Home') {
        e.preventDefault()
        setCursor(0)
      } else if (e.key === 'End') {
        e.preventDefault()
        setCursor(items.length - 1)
      } else if (e.key === 'Enter' || e.key === ' ') {
        e.preventDefault()
        const item = items[cursor]
        if (item) {
          onClose()
          item.run()
        }
      }
    }
    document.addEventListener('keydown', onKey, true)
    return () => document.removeEventListener('keydown', onKey, true)
  }, [items, cursor, onClose])

  return (
    <>
      {/* A click anywhere else closes the menu, including a right-click that
          would otherwise open a second one. */}
      <div
        className="fixed inset-0 z-[60]"
        onMouseDown={onClose}
        onContextMenu={(e) => {
          e.preventDefault()
          onClose()
        }}
      />
      <div
        ref={panel}
        role="menu"
        aria-orientation="vertical"
        style={{ left: pos.left, top: pos.top }}
        className="animate-in fixed z-[61] max-w-[380px] min-w-[204px] overflow-hidden rounded-md border border-line bg-surface-3 py-1"
      >
        {entries.map((entry, i) =>
          isItem(entry) ? (
            <button
              key={entry.id}
              type="button"
              role="menuitem"
              autoFocus={items.indexOf(entry) === 0}
              onMouseEnter={() => setCursor(items.indexOf(entry))}
              onClick={() => {
                onClose()
                entry.run()
              }}
              className={`flex h-[var(--row-h-sm)] w-full items-center gap-[10px] px-3 text-left text-base outline-none ${
                items.indexOf(entry) === cursor ? 'bg-row-hover' : ''
              } ${entry.danger ? 'text-err' : 'text-text-1'}`}
            >
              <span
                className={`flex w-[14px] shrink-0 items-center justify-center ${
                  entry.danger ? 'text-err' : 'text-text-3'
                }`}
              >
                {entry.icon}
              </span>
              <span className="min-w-0 truncate">{entry.label}</span>
              {entry.hint && (
                <span className="min-w-0 flex-1 truncate font-mono text-sm text-text-3">
                  {entry.hint}
                </span>
              )}
              {!entry.hint && <span className="flex-1" />}
              {entry.keys && <Keys keys={entry.keys} />}
            </button>
          ) : (
            <span key={`sep-${i}`} className="my-1 block h-px bg-line" />
          ),
        )}
      </div>
    </>
  )
}

/** Opens a menu at the pointer. Used from onContextMenu handlers. */
export function anchorFromEvent(e: React.MouseEvent): MenuAnchor {
  return { x: e.clientX, y: e.clientY }
}

/** Opens a menu under a button, right-aligned to it. */
export function anchorFromButton(e: React.MouseEvent): MenuAnchor {
  const box = (e.currentTarget as HTMLElement).getBoundingClientRect()
  return { x: box.right, y: box.bottom + 4, fromRight: true, height: box.height + 8 }
}
