import { ChevronDown, ChevronRight, Folder, Hash, Inbox, MoreHorizontal, Trash2 } from 'lucide-react'
import { useState } from 'react'
import type { GroupNode } from '../lib/types'
import { IconButton, PaneHeader } from './primitives'
import { anchorFromButton, anchorFromEvent, type MenuAnchor } from './Menu'

export interface Selection {
  groupId: string
  tag: string
  recycleBin: boolean
}

export const ALL: Selection = { groupId: '', tag: '', recycleBin: false }

function sameSelection(a: Selection, b: Selection) {
  return a.groupId === b.groupId && a.tag === b.tag && a.recycleBin === b.recycleBin
}

export function Sidebar({
  name,
  groups,
  tags,
  total,
  selection,
  onSelect,
  onGroupMenu,
}: {
  name: string
  groups: GroupNode[]
  tags: string[]
  total: number
  selection: Selection
  onSelect: (s: Selection) => void
  onGroupMenu: (group: GroupNode, anchor: MenuAnchor) => void
}) {
  const bin = findRecycleBin(groups)
  return (
    <nav className="flex h-full w-[212px] shrink-0 flex-col border-r border-line bg-surface-1">
      <PaneHeader>
        <span className="truncate text-base font-medium text-text-1">{name || 'Vault'}</span>
      </PaneHeader>

      <div className="min-h-0 flex-1 overflow-y-auto py-2">
        <Row
          icon={<Inbox size={14} />}
          label="All entries"
          count={total}
          depth={0}
          active={sameSelection(selection, ALL)}
          onClick={() => onSelect(ALL)}
        />

        {groups.map((g) => (
          <GroupRow
            key={g.id}
            group={g}
            depth={0}
            selection={selection}
            onSelect={onSelect}
            onGroupMenu={onGroupMenu}
          />
        ))}

        {tags.length > 0 && (
          <>
            <Divider label="Tags" />
            {tags.map((t) => (
              <Row
                key={t}
                icon={<Hash size={14} />}
                label={t}
                depth={0}
                active={selection.tag === t}
                onClick={() => onSelect({ groupId: '', tag: t, recycleBin: false })}
              />
            ))}
          </>
        )}

        {bin && (
          <>
            <Divider />
            <Row
              icon={<Trash2 size={14} />}
              label={bin.name}
              count={bin.count}
              depth={0}
              active={selection.recycleBin}
              onClick={() => onSelect({ groupId: bin.id, tag: '', recycleBin: true })}
              onMenu={bin.count > 0 ? (anchor) => onGroupMenu(bin, anchor) : undefined}
            />
          </>
        )}
      </div>
    </nav>
  )
}

function GroupRow({
  group,
  depth,
  selection,
  onSelect,
  onGroupMenu,
}: {
  group: GroupNode
  depth: number
  selection: Selection
  onSelect: (s: Selection) => void
  onGroupMenu: (group: GroupNode, anchor: MenuAnchor) => void
}) {
  const [open, setOpen] = useState(true)
  if (group.isRecycleBin) return null
  const children = group.children.filter((c) => !c.isRecycleBin)

  return (
    <>
      <Row
        icon={<Folder size={14} />}
        label={group.name}
        count={group.count}
        depth={depth}
        active={selection.groupId === group.id && !selection.recycleBin}
        onClick={() => onSelect({ groupId: group.id, tag: '', recycleBin: false })}
        onMenu={(anchor) => onGroupMenu(group, anchor)}
        twisty={
          children.length > 0 ? (
            <button
              type="button"
              aria-label={open ? `Collapse ${group.name}` : `Expand ${group.name}`}
              onClick={(e) => {
                e.stopPropagation()
                setOpen(!open)
              }}
              className="flex h-[16px] w-[16px] items-center justify-center rounded-[3px] text-text-3 hover:text-text-1"
            >
              {open ? <ChevronDown size={12} /> : <ChevronRight size={12} />}
            </button>
          ) : undefined
        }
      />
      {open &&
        children.map((c) => (
          <GroupRow
            key={c.id}
            group={c}
            depth={depth + 1}
            selection={selection}
            onSelect={onSelect}
            onGroupMenu={onGroupMenu}
          />
        ))}
    </>
  )
}

function Row({
  icon,
  label,
  count,
  depth,
  active,
  onClick,
  onMenu,
  twisty,
}: {
  icon: React.ReactNode
  label: string
  count?: number
  depth: number
  active: boolean
  onClick: () => void
  onMenu?: (anchor: MenuAnchor) => void
  twisty?: React.ReactNode
}) {
  return (
    <span className="group relative flex">
    <button
      type="button"
      onClick={onClick}
      onContextMenu={
        onMenu
          ? (e) => {
              e.preventDefault()
              onClick()
              onMenu(anchorFromEvent(e))
            }
          : undefined
      }
      aria-current={active ? 'true' : undefined}
      className={`row-transition relative flex h-[var(--row-h-sm)] w-full items-center gap-[6px] pr-3 text-left ${
        active ? 'bg-selected text-text-1' : 'text-text-2 hover:bg-row-hover'
      }`}
      style={{ paddingLeft: `${12 + depth * 14}px` }}
    >
      {active && (
        <span className="absolute top-[5px] bottom-[5px] left-0 w-[2px] rounded-full bg-accent" />
      )}
      <span className="flex w-[16px] shrink-0 items-center justify-center">
        {twisty ?? <span className={active ? 'text-accent' : 'text-text-3'}>{icon}</span>}
      </span>
      <span className="min-w-0 flex-1 truncate text-base">{label}</span>
      {count !== undefined && count > 0 && (
        <span className={`shrink-0 text-sm text-text-3 tabular ${onMenu ? 'group-hover:invisible' : ''}`}>
          {count}
        </span>
      )}
    </button>
    {onMenu && (
      <span className="absolute top-0 right-1 opacity-0 group-hover:opacity-100 focus-within:opacity-100">
        <IconButton
          label={`Actions for ${label}`}
          onClick={(e) => {
            onClick()
            onMenu(anchorFromButton(e))
          }}
        >
          <MoreHorizontal size={13} />
        </IconButton>
      </span>
    )}
    </span>
  )
}

function Divider({ label }: { label?: string }) {
  return (
    <div className="mt-3 mb-1 flex items-center gap-2 px-3">
      {label && <span className="text-sm text-text-3">{label}</span>}
      <span className="h-px flex-1 bg-line" />
    </div>
  )
}

function findRecycleBin(groups: GroupNode[]): GroupNode | undefined {
  for (const g of groups) {
    if (g.isRecycleBin) return g
    const found = findRecycleBin(g.children)
    if (found) return found
  }
  return undefined
}
