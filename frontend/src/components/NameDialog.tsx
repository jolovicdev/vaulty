import { useState } from 'react'
import { Button, Dialog, Field, Input } from './primitives'

/** One text field and one action. Used where the app needs a name and a
 *  whole screen would be too much, in place of the webview's own prompt. */
export function NameDialog({
  title,
  label,
  confirmLabel,
  initial = '',
  onClose,
  onSubmit,
}: {
  title: string
  label: string
  confirmLabel: string
  initial?: string
  onClose: () => void
  onSubmit: (value: string) => void
}) {
  const [value, setValue] = useState(initial)
  const trimmed = value.trim()

  return (
    <Dialog
      title={title}
      onClose={onClose}
      width="w-[380px]"
      footer={
        <div className="flex items-center gap-2">
          <Button variant="primary" disabled={!trimmed} onClick={() => onSubmit(trimmed)}>
            {confirmLabel}
          </Button>
          <Button variant="quiet" onClick={onClose}>
            Cancel
          </Button>
        </div>
      }
    >
      <form
        className="p-4"
        onSubmit={(e) => {
          e.preventDefault()
          if (trimmed) onSubmit(trimmed)
        }}
      >
        <Field label={label}>
          <Input autoFocus value={value} onChange={(e) => setValue(e.target.value)} />
        </Field>
      </form>
    </Dialog>
  )
}
