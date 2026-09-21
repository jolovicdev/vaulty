import { useState } from 'react'
import type { GroupNode } from '../lib/types'
import { Button, Dialog, Field, Select } from './primitives'

/** Flattens the tree into indented options, skipping the recycle bin: an
 *  entry is recycled by deleting it, not by moving it there. */
function options(groups: GroupNode[], depth = 0): { id: string; label: string }[] {
  return groups.flatMap((g) =>
    g.isRecycleBin
      ? []
      : [
          { id: g.id, label: `${'  '.repeat(depth)}${g.name}` },
          ...options(g.children, depth + 1),
        ],
  )
}

export function MoveDialog({
  groups,
  currentGroupId,
  title,
  onClose,
  onMove,
}: {
  groups: GroupNode[]
  currentGroupId: string
  title: string
  onClose: () => void
  onMove: (groupId: string) => void
}) {
  const rows = options(groups)
  const [groupId, setGroupId] = useState(currentGroupId || rows[0]?.id || '')

  return (
    <Dialog
      title="Move to a group"
      description={title}
      onClose={onClose}
      width="w-[380px]"
      footer={
        <div className="flex items-center gap-2">
          <Button
            variant="primary"
            disabled={!groupId || groupId === currentGroupId}
            onClick={() => onMove(groupId)}
          >
            Move
          </Button>
          <Button variant="quiet" onClick={onClose}>
            Cancel
          </Button>
        </div>
      }
    >
      <div className="p-4">
        <Field label="Group">
          <Select autoFocus value={groupId} onChange={(e) => setGroupId(e.target.value)}>
            {rows.map((g) => (
              <option key={g.id} value={g.id}>
                {g.label}
              </option>
            ))}
          </Select>
        </Field>
      </div>
    </Dialog>
  )
}
