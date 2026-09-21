import { useCallback, useEffect, useRef, useState } from 'react'
import {
  Database,
  ChevronDown,
  Lock,
  MoreHorizontal,
  Settings as SettingsIcon,
  Wand2,
} from 'lucide-react'
import { EVENT_CLOSE_REQUESTED, EVENT_CONFLICT, EVENT_LOCKED, api, on } from './lib/api'
import type { Detail, GroupNode, Meta, Settings, Status } from './lib/types'
import { SHORTCUTS, isTypingTarget } from './lib/shortcuts'
import { ALL, Sidebar, type Selection } from './components/Sidebar'
import { EntryList } from './components/EntryList'
import { EntryDetail } from './components/EntryDetail'
import { EntryEditor } from './components/EntryEditor'
import { Palette, type Command } from './components/Palette'
import { History } from './components/History'
import { SettingsPanel, ShortcutsHelp } from './components/Settings'
import { ConflictDialog } from './components/ConflictDialog'
import { Generator } from './components/Generator'
import { Unlock } from './components/Unlock'
import { EmptyVault } from './components/EmptyVault'
import { UnsavedDialog, type UnsavedIntent } from './components/UnsavedDialog'
import { vaultMenu } from './components/VaultMenu'
import { NameDialog } from './components/NameDialog'
import { Dialog, IconButton, Tooltip } from './components/primitives'
import { Menu, anchorFromButton, type MenuAnchor, type MenuEntry } from './components/Menu'
import { MoveDialog } from './components/MoveDialog'
import { appMenu, entryMenu, groupMenu } from './lib/actions'

type Overlay =
  | { kind: 'none' }
  | { kind: 'palette' }
  | { kind: 'editor'; detail: Detail | null }
  | { kind: 'history'; id: string }
  | { kind: 'settings' }
  | { kind: 'generator' }
  | { kind: 'shortcuts' }
  | { kind: 'conflict' }
  | { kind: 'newGroup'; parentId: string }
  | { kind: 'move'; detail: Meta }
  | { kind: 'unsaved'; intent: UnsavedIntent }

/** Applies the theme choice. The dark values are the document default, so
 *  "system" means removing the attribute and letting the media query in
 *  tokens.css decide. */
function applyTheme(theme: Settings['theme']) {
  const root = document.documentElement
  if (theme === 'system') root.removeAttribute('data-theme')
  else root.setAttribute('data-theme', theme)
}

export function App() {
  const [status, setStatus] = useState<Status | null>(null)
  const [settings, setSettings] = useState<Settings | null>(null)
  const [groups, setGroups] = useState<GroupNode[]>([])
  const [tags, setTags] = useState<string[]>([])
  const [entries, setEntries] = useState<Meta[]>([])
  const [total, setTotal] = useState(0)
  const [selection, setSelection] = useState<Selection>(ALL)
  const [selectedId, setSelectedId] = useState('')
  const [loadedDetail, setLoadedDetail] = useState<Detail | null>(null)
  const [query, setQuery] = useState('')
  const [overlay, setOverlay] = useState<Overlay>({ kind: 'none' })
  const [menu, setMenu] = useState<{ entries: MenuEntry[]; anchor: MenuAnchor } | null>(null)
  // Set before locking, so the unlock screen opens on the chosen vault, and
  // in create mode when the path came from the save dialog: that file does
  // not exist yet, so it can be created but not unlocked.
  const [pending, setPending] = useState({ path: '', create: false })
  const [error, setError] = useState('')

  const detail = loadedDetail?.id === selectedId ? loadedDetail : null
  const vaultIsEmpty = total === 0

  const listOptions = {
    groupId: selection.recycleBin ? selection.groupId : selection.groupId,
    tag: selection.tag,
    includeRecycleBin: selection.recycleBin,
  }
  const optionsKey = JSON.stringify(listOptions)

  // Closing the window with unsaved work puts the question here rather than
  // in an OS dialog, which on Windows cannot offer three answers.
  useEffect(
    () => on(EVENT_CLOSE_REQUESTED, () => setOverlay({ kind: 'unsaved', intent: { kind: 'quit' } })),
    [],
  )
  useEffect(() => on(EVENT_CONFLICT, () => setOverlay({ kind: 'conflict' })), [])

  // leaveFor locks the current vault and points the unlock screen at
  // another one, so opening a second vault does not need a manual lock.
  const leaveFor = useCallback(async (path: string, create: boolean) => {
    setPending({ path, create })
    await api.lock()
  }, [])

  // switchVault is the guarded version: unsaved work is asked about before
  // the session goes away, never discarded quietly.
  const switchVault = useCallback(
    (path: string, create = false) => {
      setMenu(null)
      if (status?.dirty) {
        setOverlay({ kind: 'unsaved', intent: { kind: 'switch', path, create } })
        return
      }
      void leaveFor(path, create)
    },
    [status?.dirty, leaveFor],
  )

  const browseVault = useCallback(async () => {
    const chosen = await api.pickVaultFile()
    if (chosen) switchVault(chosen)
  }, [switchVault])

  const createVault = useCallback(async () => {
    const chosen = await api.pickNewVaultPath()
    if (chosen) switchVault(chosen, true)
  }, [switchVault])

  const refreshSettings = useCallback(async () => {
    const s = await api.loadSettings()
    setSettings(s)
    applyTheme(s.theme)
  }, [])

  useEffect(() => {
    void (async () => {
      await refreshSettings()
      setStatus(await api.status())
    })()
  }, [refreshSettings])

  // The Go side owns the idle timer; this just resets the frontend when it
  // fires, so no revealed value or cached list survives a lock.
  useEffect(
    () =>
      on(EVENT_LOCKED, () => {
        setStatus((s) => (s ? { ...s, unlocked: false, dirty: false } : s))
        setGroups([])
        setTags([])
        setEntries([])
        setLoadedDetail(null)
        setSelectedId('')
        setQuery('')
        setSelection(ALL)
        setOverlay({ kind: 'none' })
        setMenu(null)
        void refreshSettings()
      }),
    [refreshSettings],
  )

  const refreshTree = useCallback(async () => {
    if (!status?.unlocked) return
    const [g, t, all] = await Promise.all([api.groups(), api.tags(), api.list({})])
    setGroups(g)
    setTags(t)
    setTotal(all.length)
  }, [status?.unlocked])

  const refreshList = useCallback(async () => {
    if (!status?.unlocked) return
    const found = query.trim()
      ? await api.search(query, listOptions)
      : await api.list(listOptions)
    setEntries(found)
    setSelectedId((current) =>
      found.some((e) => e.id === current) ? current : (found[0]?.id ?? ''),
    )
    // listOptions is rebuilt each render; optionsKey is its stable identity.
  }, [status?.unlocked, query, optionsKey]) // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    void refreshTree()
  }, [refreshTree])

  useEffect(() => {
    void refreshList()
  }, [refreshList])

  // The loaded detail carries its own id, so a stale one is never shown for
  // an entry that is no longer selected.
  useEffect(() => {
    if (!selectedId) return
    let live = true
    void (async () => {
      try {
        const d = await api.detail(selectedId)
        if (live) setLoadedDetail(d)
      } catch {
        if (live) setLoadedDetail(null)
      }
    })()
    return () => {
      live = false
    }
  }, [selectedId])

  // Writes save themselves. This remains for Ctrl S, which is what a hand
  // reaches for after resolving a conflict, and for the unsaved dialog.
  const save = useCallback(async () => {
    try {
      await api.save()
      setError('')
      setStatus(await api.status())
    } catch (e) {
      const message = e instanceof Error ? e.message : String(e)
      // The vault package refuses to overwrite a file another writer
      // touched; that is the one error with a real dialog behind it.
      if (message.includes('changed on disk')) setOverlay({ kind: 'conflict' })
      else setError(message)
    }
  }, [])

  const refreshAfterWrite = useCallback(
    async (id?: string) => {
      await Promise.all([refreshTree(), refreshList()])
      if (id) setSelectedId(id)
      setStatus(await api.status())
    },
    [refreshTree, refreshList],
  )

  const createGroup = useCallback(
    async (parentId: string, name: string) => {
      await api.addGroup(parentId, name)
      await refreshAfterWrite()
    },
    [refreshAfterWrite],
  )

  // The two irreversible actions confirm in an OS dialog, not the webview's
  // own confirm box.
  const emptyRecycleBin = useCallback(async () => {
    const ok = await api.confirm(
      'Empty the recycle bin',
      'Everything in the recycle bin will be deleted permanently. This cannot be undone.',
    )
    if (!ok) return
    await api.emptyRecycleBin()
    await refreshAfterWrite()
  }, [refreshAfterWrite])

  // A recycled entry is deleted for good on the second delete, so that one
  // asks first; recycling is reversible and does not.
  const deleteEntry = useCallback(
    // Meta is enough: the id identifies the entry and inRecycleBin decides
    // whether this delete is the reversible one.
    async (target: Meta) => {
      if (target.inRecycleBin) {
        const ok = await api.confirm(
          'Delete permanently',
          `"${target.title || 'Untitled'}" will be deleted permanently. This cannot be undone.`,
        )
        if (!ok) return
      }
      await api.deleteEntry(target.id)
      await refreshAfterWrite()
    },
    [refreshAfterWrite],
  )

  // One keydown listener for the whole app. Anything that needs the vault
  // also pokes the idle timer through the api call it makes.
  const overlayKind = overlay.kind
  const touchTimer = useRef(0)
  useEffect(() => {
    if (!status?.unlocked) return
    function onKey(e: KeyboardEvent) {
      // Throttle the idle-timer reset, so typing does not cost a binding call
      // per keystroke. The interval has to stay well under the shortest idle
      // lock the settings offer, which is a minute: at a minute itself, the
      // lock fires on somebody typing before the next reset is allowed.
      const now = Date.now()
      if (now - touchTimer.current > 10_000) {
        touchTimer.current = now
        void api.touch()
      }

      if (SHORTCUTS.openVault.match(e)) {
        e.preventDefault()
        void browseVault()
        return
      }
      if (SHORTCUTS.palette.match(e)) {
        e.preventDefault()
        setOverlay({ kind: 'palette' })
        return
      }
      if (SHORTCUTS.help.match(e)) {
        e.preventDefault()
        setOverlay({ kind: 'shortcuts' })
        return
      }
      if (SHORTCUTS.settings.match(e)) {
        e.preventDefault()
        setOverlay({ kind: 'settings' })
        return
      }
      if (SHORTCUTS.generator.match(e)) {
        e.preventDefault()
        setOverlay({ kind: 'generator' })
        return
      }
      if (SHORTCUTS.lock.match(e)) {
        e.preventDefault()
        void api.lock()
        return
      }
      if (SHORTCUTS.save.match(e)) {
        e.preventDefault()
        void save()
        return
      }
      if (overlayKind !== 'none') return

      if (SHORTCUTS.newEntry.match(e)) {
        e.preventDefault()
        setOverlay({ kind: 'editor', detail: null })
        return
      }
      if (!detail) return
      if (SHORTCUTS.edit.match(e)) {
        e.preventDefault()
        setOverlay({ kind: 'editor', detail })
      } else if (SHORTCUTS.history.match(e)) {
        e.preventDefault()
        setOverlay({ kind: 'history', id: detail.id })
      } else if (SHORTCUTS.copyPassword.match(e) && !isTypingTarget(e.target)) {
        e.preventDefault()
        void api.copyField(detail.id, 'Password')
      } else if (SHORTCUTS.copyUsername.match(e)) {
        e.preventDefault()
        void api.copyField(detail.id, 'UserName')
      } else if (SHORTCUTS.copyTotp.match(e)) {
        e.preventDefault()
        void api.copyTotp(detail.id)
      } else if (SHORTCUTS.openUrl.match(e)) {
        e.preventDefault()
        void api.openUrl(detail.id)
      } else if (SHORTCUTS.del.match(e)) {
        e.preventDefault()
        void deleteEntry(detail)
      } else if (!isTypingTarget(e.target) && (SHORTCUTS.down.match(e) || SHORTCUTS.up.match(e))) {
        e.preventDefault()
        const i = entries.findIndex((x) => x.id === selectedId)
        const next = SHORTCUTS.down.match(e) ? i + 1 : i - 1
        if (next >= 0 && next < entries.length) setSelectedId(entries[next].id)
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [status?.unlocked, overlayKind, detail, entries, selectedId, save, deleteEntry, browseVault])

  // The handler sets every menu reads from. Defined once so a row menu, the
  // hover button and the palette cannot offer different behaviour for the
  // same label.
  const entryHandlers = {
    copyPassword: (id: string) => void api.copyField(id, 'Password'),
    copyUsername: (id: string) => void api.copyField(id, 'UserName'),
    copyTotp: (id: string) => void api.copyTotp(id),
    openUrl: (id: string) => void api.openUrl(id),
    edit: (id: string) => {
      if (detail?.id === id) setOverlay({ kind: 'editor', detail })
      else void api.detail(id).then((d) => setOverlay({ kind: 'editor', detail: d }))
    },
    move: (id: string) => {
      const target = entries.find((e) => e.id === id)
      if (target) setOverlay({ kind: 'move', detail: target })
    },
    history: (id: string) => setOverlay({ kind: 'history', id }),
    del: (id: string) => {
      const target = entries.find((e) => e.id === id)
      if (target) void deleteEntry(target)
    },
    restore: (id: string) => void api.restoreEntry(id, '').then(() => refreshAfterWrite()),
  }

  const groupHandlers = {
    newEntry: (groupId: string) => {
      setSelection({ groupId, tag: '', recycleBin: false })
      setOverlay({ kind: 'editor', detail: null })
    },
    newGroup: (parentId: string) => setOverlay({ kind: 'newGroup', parentId }),
    emptyRecycleBin: () => void emptyRecycleBin(),
  }

  // Every command replaces the palette: the ones that open a screen set the
  // overlay to it, the ones that act close the overlay instead.
  function runCommand(c: Command) {
    switch (c) {
      case 'newEntry':
        setOverlay({ kind: 'editor', detail: null })
        return
      case 'newGroup':
        setOverlay({ kind: 'newGroup', parentId: selection.groupId || groups[0]?.id || '' })
        return
      case 'generator':
        setOverlay({ kind: 'generator' })
        return
      case 'settings':
        setOverlay({ kind: 'settings' })
        return
      case 'lock':
        setOverlay({ kind: 'none' })
        void api.lock()
        return
      case 'save':
        setOverlay({ kind: 'none' })
        void save()
        return
      case 'emptyRecycleBin':
        setOverlay({ kind: 'none' })
        void emptyRecycleBin()
        return
      case 'openVault':
        setOverlay({ kind: 'none' })
        void browseVault()
        return
      case 'createVault':
        setOverlay({ kind: 'none' })
        void createVault()
        return
    }
  }

  if (!status || !settings) return <div className="h-full bg-bg" />

  if (!status.unlocked) {
    return (
      <Unlock
        settings={settings}
        initialPath={pending.path}
        initialCreate={pending.create}
        onUnlocked={(s) => {
          setPending({ path: '', create: false })
          setStatus(s)
        }}
        onSettingsChanged={() => void refreshSettings()}
      />
    )
  }

  return (
    <div className="flex h-full flex-col bg-bg">
      <div className="flex min-h-0 flex-1">
        <Sidebar
          name={status.name}
          groups={groups}
          tags={tags}
          total={total}
          selection={selection}
          onSelect={(s) => {
            setSelection(s)
            setQuery('')
          }}
          onGroupMenu={(group, anchor) =>
            setMenu({ entries: groupMenu(group, groupHandlers), anchor })
          }
        />
        {vaultIsEmpty ? (
          <EmptyVault onNew={() => setOverlay({ kind: 'editor', detail: null })} />
        ) : (
          <>
            <EntryList
              entries={entries}
          selectedId={selectedId}
          query={query}
          searching={query.trim().length > 0}
          onQuery={setQuery}
          onSelect={setSelectedId}
              onNew={() => setOverlay({ kind: 'editor', detail: null })}
              onRowMenu={(entry, anchor) =>
                setMenu({ entries: entryMenu(entry, entryHandlers), anchor })
              }
            />
            <EntryDetail
              detail={detail}
              onEdit={() => detail && setOverlay({ kind: 'editor', detail })}
              onHistory={() => detail && setOverlay({ kind: 'history', id: detail.id })}
              onDelete={() => detail && void deleteEntry(detail)}
              onRestore={() =>
                detail && void api.restoreEntry(detail.id, '').then(() => refreshAfterWrite())
              }
              onMenu={(d, anchor) => setMenu({ entries: entryMenu(d, entryHandlers), anchor })}
            />
          </>
        )}
      </div>

      <footer className="flex h-[28px] shrink-0 items-center gap-3 border-t border-line bg-surface-1 px-3 text-sm text-text-3">
        <Tooltip label="Switch or create a vault" keys={SHORTCUTS.openVault.keys}>
          <button
            type="button"
            onClick={(e) =>
              setMenu({
                entries: vaultMenu(settings.recent, status.path, {
                  open: switchVault,
                  browse: () => void browseVault(),
                  create: () => void createVault(),
                }),
                anchor: anchorFromButton(e),
              })
            }
            className="row-transition flex min-w-0 items-center gap-[6px] rounded-sm px-1 hover:text-text-1"
          >
            <Database size={12} className="shrink-0" />
            <span className="min-w-0 truncate font-mono">{status.path}</span>
            <ChevronDown size={11} className="shrink-0" />
          </button>
        </Tooltip>
        <span className="text-line-strong">·</span>
        <span className="font-mono">KDBX {status.formatVersion}</span>
        {/* Writes save themselves, so "unsaved changes" only appears when
            an autosave was refused and the conflict is still unresolved. */}
        {status.dirty && (
          <>
            <span className="text-line-strong">·</span>
            <button
              type="button"
              onClick={() => setOverlay({ kind: 'conflict' })}
              className="text-warn hover:underline"
            >
              not saved, resolve the conflict
            </button>
          </>
        )}
        {error && (
          <>
            <span className="text-line-strong">·</span>
            <span className="truncate text-err">{error}</span>
          </>
        )}
        <span className="flex-1" />
        <IconButton
          label={SHORTCUTS.generator.label}
          keys={SHORTCUTS.generator.keys}
          onClick={() => setOverlay({ kind: 'generator' })}
        >
          <Wand2 size={13} />
        </IconButton>
        <IconButton
          label={SHORTCUTS.settings.label}
          keys={SHORTCUTS.settings.keys}
          onClick={() => setOverlay({ kind: 'settings' })}
        >
          <SettingsIcon size={13} />
        </IconButton>
        <IconButton
          label={SHORTCUTS.lock.label}
          keys={SHORTCUTS.lock.keys}
          onClick={() => void api.lock()}
        >
          <Lock size={13} />
        </IconButton>
        {/* The app-level actions, for anyone who has not learnt the keys. */}
        <IconButton
          label="All actions"
          onClick={(e) =>
            setMenu({
              entries: appMenu(
                {
                  newEntry: () => setOverlay({ kind: 'editor', detail: null }),
                  newGroup: () =>
                    setOverlay({ kind: 'newGroup', parentId: groups[0]?.id ?? '' }),
                  openVault: () => void browseVault(),
                  createVault: () => void createVault(),
                  generator: () => setOverlay({ kind: 'generator' }),
                  save: () => void save(),
                  settings: () => setOverlay({ kind: 'settings' }),
                  shortcuts: () => setOverlay({ kind: 'shortcuts' }),
                  lock: () => void api.lock(),
                },
                status.dirty,
              ),
              anchor: anchorFromButton(e),
            })
          }
        >
          <MoreHorizontal size={13} />
        </IconButton>
      </footer>

      {overlay.kind === 'palette' && (
        <Palette
          onClose={() => setOverlay({ kind: 'none' })}
          onCommand={runCommand}
          onSelectEntry={setSelectedId}
        />
      )}
      {overlay.kind === 'editor' && (
        <EntryEditor
          detail={overlay.detail}
          groups={groups}
          defaultGroupId={selection.groupId || groups[0]?.id || ''}
          showStrength={settings.showPasswordStrength}
          onClose={() => setOverlay({ kind: 'none' })}
          onSaved={(id) => {
            setOverlay({ kind: 'none' })
            void refreshAfterWrite(id)
          }}
        />
      )}
      {overlay.kind === 'history' && (
        <History
          entryId={overlay.id}
          onClose={() => setOverlay({ kind: 'none' })}
          onRestored={() => void refreshAfterWrite(overlay.id)}
        />
      )}
      {overlay.kind === 'settings' && (
        <SettingsPanel
          settings={settings}
          status={status}
          onClose={() => setOverlay({ kind: 'none' })}
          onSaved={(s) => {
            setSettings(s)
            applyTheme(s.theme)
          }}
        />
      )}
      {overlay.kind === 'generator' && (
        <Dialog
          title="Password generator"
          onClose={() => setOverlay({ kind: 'none' })}
          width="w-[460px]"
        >
          <Generator showStrength={settings.showPasswordStrength} />
        </Dialog>
      )}
      {overlay.kind === 'shortcuts' && (
        <ShortcutsHelp onClose={() => setOverlay({ kind: 'none' })} />
      )}
      {overlay.kind === 'newGroup' && (
        <NameDialog
          title="New group"
          label="Group name"
          confirmLabel="Create group"
          onClose={() => setOverlay({ kind: 'none' })}
          onSubmit={(name) => {
            const parentId = overlay.parentId
            setOverlay({ kind: 'none' })
            void createGroup(parentId, name)
          }}
        />
      )}
      {overlay.kind === 'move' && (
        <MoveDialog
          groups={groups}
          currentGroupId={overlay.detail.groupId}
          title={overlay.detail.title || 'Untitled'}
          onClose={() => setOverlay({ kind: 'none' })}
          onMove={(groupId) => {
            const id = overlay.detail.id
            setOverlay({ kind: 'none' })
            void api.moveEntry(id, groupId).then(() => refreshAfterWrite(id))
          }}
        />
      )}
      {overlay.kind === 'unsaved' && (
        <UnsavedDialog
          intent={overlay.intent}
          onClose={() => setOverlay({ kind: 'none' })}
          onProceed={(intent) => {
            setOverlay({ kind: 'none' })
            if (intent.kind === 'switch') void leaveFor(intent.path, intent.create)
          }}
        />
      )}
      {menu && (
        <Menu entries={menu.entries} anchor={menu.anchor} onClose={() => setMenu(null)} />
      )}
      {overlay.kind === 'conflict' && (
        <ConflictDialog
          onClose={() => setOverlay({ kind: 'none' })}
          onResolved={() => void refreshAfterWrite()}
        />
      )}
    </div>
  )
}
