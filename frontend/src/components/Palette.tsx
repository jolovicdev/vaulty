import { useEffect, useMemo, useRef, useState } from 'react'
import {
  Check,
  Database,
  ExternalLink,
  FolderOpen,
  FolderPlus,
  Import,
  KeyRound,
  Lock,
  Plus,
  Settings as SettingsIcon,
  Timer,
  Trash2,
  User,
  Wand2,
} from 'lucide-react'
import { api } from '../lib/api'
import { FIELD, type Meta } from '../lib/types'
import { SHORTCUTS } from '../lib/shortcuts'
import { IMPORT_LABEL } from '../lib/actions'
import { Keys } from './primitives'

export type Command =
  | 'newEntry'
  | 'newGroup'
  | 'lock'
  | 'generator'
  | 'settings'
  | 'save'
  | 'emptyRecycleBin'
  | 'openVault'
  | 'createVault'
  | 'importFile'

interface CommandRow {
  id: Command
  label: string
  keys: string
  icon: React.ReactNode
}

const COMMANDS: CommandRow[] = [
  { id: 'newEntry', label: SHORTCUTS.newEntry.label, keys: SHORTCUTS.newEntry.keys, icon: <Plus size={14} /> },
  { id: 'newGroup', label: 'New group', keys: '', icon: <FolderPlus size={14} /> },
  {
    id: 'openVault',
    label: SHORTCUTS.openVault.label,
    keys: SHORTCUTS.openVault.keys,
    icon: <FolderOpen size={14} />,
  },
  { id: 'createVault', label: 'Create a vault', keys: '', icon: <Database size={14} /> },
  { id: 'importFile', label: IMPORT_LABEL, keys: '', icon: <Import size={14} /> },
  { id: 'generator', label: SHORTCUTS.generator.label, keys: SHORTCUTS.generator.keys, icon: <Wand2 size={14} /> },
  { id: 'save', label: SHORTCUTS.save.label, keys: SHORTCUTS.save.keys, icon: <Check size={14} /> },
  { id: 'settings', label: SHORTCUTS.settings.label, keys: SHORTCUTS.settings.keys, icon: <SettingsIcon size={14} /> },
  { id: 'lock', label: SHORTCUTS.lock.label, keys: SHORTCUTS.lock.keys, icon: <Lock size={14} /> },
  { id: 'emptyRecycleBin', label: 'Empty the recycle bin', keys: '', icon: <Trash2 size={14} /> },
]

/** The palette searches entries and runs commands. Enter copies the
 *  password, because that is what the user came for nine times in ten; the
 *  other actions keep the modifier they have everywhere else in the app. */
export function Palette({
  onClose,
  onCommand,
  onSelectEntry,
}: {
  onClose: () => void
  onCommand: (c: Command) => void
  onSelectEntry: (id: string) => void
}) {
  const [query, setQuery] = useState('')
  const [entries, setEntries] = useState<Meta[]>([])
  const [cursor, setCursor] = useState(0)
  const [done, setDone] = useState('')
  const listRef = useRef<HTMLUListElement>(null)

  useEffect(() => {
    let live = true
    void (async () => {
      try {
        const found = await api.search(query, { includeRecycleBin: false })
        if (live) {
          setEntries(found.slice(0, 40))
          setCursor(0)
        }
      } catch {
        if (live) setEntries([])
      }
    })()
    return () => {
      live = false
    }
  }, [query])

  const commands = useMemo(
    () =>
      query.trim() === ''
        ? COMMANDS
        : COMMANDS.filter((c) => c.label.toLowerCase().includes(query.trim().toLowerCase())),
    [query],
  )

  const rows = useMemo(
    () => [
      ...commands.map((c) => ({ kind: 'command' as const, command: c })),
      ...entries.map((e) => ({ kind: 'entry' as const, entry: e })),
    ],
    [commands, entries],
  )

  useEffect(() => {
    listRef.current
      ?.querySelector(`[data-index="${cursor}"]`)
      ?.scrollIntoView({ block: 'nearest' })
  }, [cursor])

  async function act(kind: 'password' | 'username' | 'totp' | 'url' | 'open') {
    const row = rows[cursor]
    if (!row) return
    if (row.kind === 'command') {
      // The palette and the screens a command opens share one overlay slot,
      // so closing here would immediately undo whatever the command opened.
      // runCommand closes the palette itself for the commands that open
      // nothing.
      onCommand(row.command.id)
      return
    }
    const id = row.entry.id
    try {
      switch (kind) {
        case 'password':
          await api.copyField(id, FIELD.password)
          setDone('Password copied')
          break
        case 'username':
          await api.copyField(id, FIELD.username)
          setDone('Username copied')
          break
        case 'totp':
          await api.copyTotp(id)
          setDone('One-time code copied')
          break
        case 'url':
          await api.openUrl(id)
          setDone('Opened in the browser')
          break
        case 'open':
          onSelectEntry(id)
          onClose()
          return
      }
      setTimeout(onClose, 550)
    } catch (e) {
      setDone(e instanceof Error ? e.message : String(e))
    }
  }

  function onKeyDown(e: React.KeyboardEvent) {
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      setCursor((c) => Math.min(c + 1, rows.length - 1))
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      setCursor((c) => Math.max(c - 1, 0))
    } else if (e.key === 'Enter') {
      e.preventDefault()
      void act(e.shiftKey ? 'open' : 'password')
    } else if (SHORTCUTS.copyUsername.match(e.nativeEvent)) {
      e.preventDefault()
      void act('username')
    } else if (SHORTCUTS.copyTotp.match(e.nativeEvent)) {
      e.preventDefault()
      void act('totp')
    } else if (SHORTCUTS.openUrl.match(e.nativeEvent)) {
      e.preventDefault()
      void act('url')
    } else if (e.key === 'Escape') {
      e.preventDefault()
      onClose()
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-start justify-center bg-scrim pt-[13vh]">
      <div className="animate-in flex max-h-[62vh] w-[560px] flex-col overflow-hidden rounded-md border border-line bg-surface-3">
        <div className="flex h-[44px] shrink-0 items-center gap-2 border-b border-line px-3">
          <KeyRound size={15} className="shrink-0 text-text-3" />
          <input
            autoFocus
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            onKeyDown={onKeyDown}
            placeholder="Search entries, or type a command"
            aria-label="Command palette"
            className="min-w-0 flex-1 bg-transparent text-md text-text-1 outline-none"
          />
        </div>

        <ul ref={listRef} className="scroll-fade min-h-0 flex-1 overflow-y-auto py-1" role="listbox">
          {rows.length === 0 && (
            <li className="px-3 py-3 text-base text-text-3">Nothing matches {query}</li>
          )}
          {rows.map((row, i) => {
            const active = i === cursor
            return (
              <li key={row.kind === 'command' ? row.command.id : row.entry.id}>
                <button
                  type="button"
                  data-index={i}
                  role="option"
                  aria-selected={active}
                  onMouseMove={() => setCursor(i)}
                  onClick={() => void act('password')}
                  className={`relative flex h-[var(--row-h)] w-full items-center gap-[10px] px-3 text-left ${
                    active ? 'bg-selected' : ''
                  } ${
                    row.kind === 'entry' && i === commands.length && commands.length > 0
                      ? 'border-t border-line'
                      : ''
                  }`}
                >
                  {active && <span className="absolute top-0 bottom-0 left-0 w-[2px] bg-accent" />}
                  <span className={active ? 'text-accent' : 'text-text-3'}>
                    {row.kind === 'command' ? row.command.icon : <User size={14} />}
                  </span>
                  {row.kind === 'command' ? (
                    <>
                      <span className="min-w-0 flex-1 truncate text-base text-text-1">
                        {row.command.label}
                      </span>
                      {row.command.keys ? <Keys keys={row.command.keys} /> : null}
                    </>
                  ) : (
                    <>
                      <span className="min-w-0 flex-1 truncate text-base text-text-1">
                        {row.entry.title || 'Untitled'}
                      </span>
                      {row.entry.username && (
                        <span className="max-w-[140px] shrink-0 truncate font-mono text-sm text-text-3">
                          {row.entry.username}
                        </span>
                      )}
                      {row.entry.hasTotp && <Timer size={12} className="shrink-0 text-text-3" />}
                      {row.entry.url && <ExternalLink size={12} className="shrink-0 text-text-3" />}
                    </>
                  )}
                </button>
              </li>
            )
          })}
        </ul>

        <div className="flex h-[30px] shrink-0 items-center gap-4 border-t border-line px-3 text-sm text-text-3">
          {done ? (
            <span className="text-ok">{done}</span>
          ) : (
            <>
              <Hint keys="Enter" label="copy password" />
              <Hint keys={SHORTCUTS.copyUsername.keys} label="username" />
              <Hint keys={SHORTCUTS.copyTotp.keys} label="code" />
              <Hint keys={SHORTCUTS.openUrl.keys} label="open URL" />
            </>
          )}
        </div>
      </div>
    </div>
  )
}

function Hint({ keys, label }: { keys: string; label: string }) {
  return (
    <span className="inline-flex items-center gap-[5px]">
      <Keys keys={keys} />
      {label}
    </span>
  )
}
