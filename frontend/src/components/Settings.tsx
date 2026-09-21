import { useEffect, useState } from 'react'
import { api } from '../lib/api'
import type { Settings as SettingsData, Status, Theme } from '../lib/types'
import { SHORTCUTS, keyParts } from '../lib/shortcuts'
import { Button, Dialog, Field, Keys } from './primitives'
import { Toggle } from './Generator'

const IDLE_CHOICES = [60, 120, 300, 600, 1800, 3600]
const CLIPBOARD_CHOICES = [5, 10, 15, 30, 60]
const THEMES: { id: Theme; label: string }[] = [
  { id: 'system', label: 'Follow the system' },
  { id: 'dark', label: 'Dark' },
  { id: 'light', label: 'Light' },
]

export function SettingsPanel({
  settings,
  status,
  onClose,
  onSaved,
}: {
  settings: SettingsData
  status: Status
  onClose: () => void
  onSaved: (s: SettingsData) => void
}) {
  const [draft, setDraft] = useState(settings)
  const [path, setPath] = useState('')
  const [backups, setBackups] = useState<string[]>([])

  useEffect(() => {
    void (async () => {
      setPath(await api.settingsPath())
      if (status.unlocked) {
        try {
          setBackups(await api.backups())
        } catch {
          setBackups([])
        }
      }
    })()
  }, [status.unlocked])

  async function apply(next: SettingsData) {
    setDraft(next)
    await api.saveSettings(next)
    onSaved(next)
  }

  return (
    <Dialog title="Settings" onClose={onClose} width="w-[560px]">
      <div className="flex flex-col gap-5 p-4">
        <Field label="Theme">
          <div className="flex gap-1">
            {THEMES.map((t) => (
              <button
                key={t.id}
                type="button"
                onClick={() => void apply({ ...draft, theme: t.id })}
                className={`row-transition h-[28px] rounded-sm px-[10px] text-base ${
                  draft.theme === t.id
                    ? 'bg-selected text-text-1'
                    : 'text-text-3 hover:text-text-1'
                }`}
              >
                {t.label}
              </button>
            ))}
          </div>
        </Field>

        <Field label="Lock after idle">
          <div className="flex gap-1">
            {IDLE_CHOICES.map((s) => (
              <button
                key={s}
                type="button"
                onClick={() => void apply({ ...draft, idleLockSeconds: s })}
                className={`row-transition h-[28px] rounded-sm px-[9px] text-base tabular ${
                  draft.idleLockSeconds === s
                    ? 'bg-selected text-text-1'
                    : 'text-text-3 hover:text-text-1'
                }`}
              >
                {s < 60 ? `${s}s` : `${s / 60}m`}
              </button>
            ))}
          </div>
        </Field>

        <Field label="Clear the clipboard after">
          <div className="flex gap-1">
            {CLIPBOARD_CHOICES.map((s) => (
              <button
                key={s}
                type="button"
                onClick={() => void apply({ ...draft, clipboardSeconds: s })}
                className={`row-transition h-[28px] rounded-sm px-[9px] text-base tabular ${
                  draft.clipboardSeconds === s
                    ? 'bg-selected text-text-1'
                    : 'text-text-3 hover:text-text-1'
                }`}
              >
                {s}s
              </button>
            ))}
          </div>
        </Field>

        <Toggle
          label="Show a strength estimate while typing a password"
          checked={draft.showPasswordStrength}
          onChange={(checked) => void apply({ ...draft, showPasswordStrength: checked })}
        />

        {!status.clipboardWorks && (
          <p className="text-base text-warn">
            No clipboard helper was found. Install wl-clipboard on Wayland, or xclip on X11, to
            enable copying.
          </p>
        )}

        <div className="flex flex-col gap-1 border-t border-line pt-4 text-sm text-text-3">
          <span className="flex gap-2">
            <span className="w-[92px] shrink-0">Settings file</span>
            <span className="min-w-0 truncate font-mono">{path || 'unavailable'}</span>
          </span>
          {/* The vault's path and format are in the status bar at the same
              time, so only what is not visible there is repeated here. */}
          {status.unlocked && (
            <span className="flex gap-2">
              <span className="w-[92px] shrink-0">Backups</span>
              <span className="font-mono tabular">{backups.length}</span>
              <span>kept beside the vault</span>
            </span>
          )}
        </div>
      </div>
    </Dialog>
  )
}

/** ShortcutsHelp lists every binding from the one registry that also drives
 *  the handlers, so this list cannot go stale. */
export function ShortcutsHelp({ onClose }: { onClose: () => void }) {
  const rows = Object.values(SHORTCUTS).filter((s) => keyParts(s.keys).length > 0)
  return (
    <Dialog title="Keyboard shortcuts" onClose={onClose} width="w-[420px]">
      <ul className="p-2">
        {rows.map((s) => (
          <li
            key={s.id}
            className="flex h-[var(--row-h-sm)] items-center justify-between gap-4 px-2"
          >
            <span className="truncate text-base text-text-2">{s.label}</span>
            <Keys keys={s.keys} />
          </li>
        ))}
      </ul>
      <div className="border-t border-line p-3">
        <Button variant="quiet" onClick={onClose}>
          Close
        </Button>
      </div>
    </Dialog>
  )
}
