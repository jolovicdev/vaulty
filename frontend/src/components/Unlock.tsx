import { useEffect, useRef, useState } from 'react'
import { ChevronRight, FolderOpen, KeyRound, Plus, X } from 'lucide-react'
import { api } from '../lib/api'
import type { Recent, Settings, Status } from '../lib/types'
import { Button, Field, IconButton, Input } from './primitives'

/** Shortens a path for a one-line listing, keeping the file name whole. */
function shortPath(path: string): string {
  const parts = path.split(/[/\\]/)
  const file = parts.pop() ?? path
  const parent = parts.pop()
  return parent ? `${parent}/${file}` : file
}

function fileName(path: string): string {
  const name = path.split(/[/\\]/).pop() ?? path
  return name.replace(/\.kdbx$/i, '')
}

export function Unlock({
  settings,
  initialPath,
  initialCreate,
  onUnlocked,
  onSettingsChanged,
}: {
  settings: Settings
  /** The vault the user asked to switch to, if any. It wins over the most
   *  recent one, which is otherwise what the screen offers. */
  initialPath: string
  /** True when initialPath is where a new vault should be made. The file
   *  does not exist, so the screen has to open ready to create it. */
  initialCreate: boolean
  onUnlocked: (s: Status) => void
  onSettingsChanged: () => void
}) {
  const recents = settings.recent
  const [path, setPath] = useState(initialPath || recents[0]?.path || '')
  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const matching = recents.find((r) => r.path === (initialPath || recents[0]?.path))
  const [keyFile, setKeyFile] = useState(matching?.keyFile ?? '')
  const [showKeyFile, setShowKeyFile] = useState(Boolean(matching?.keyFile))
  const [creating, setCreating] = useState(initialCreate)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const passwordInput = useRef<HTMLInputElement>(null)
  const confirmInput = useRef<HTMLInputElement>(null)

  // Nothing remembered and nothing chosen yet: there is no password to ask
  // for until the user names a file.
  const firstRun = recents.length === 0 && !path && !creating
  // A path the user just picked that is not among the recents has no row in
  // the list to show it, so the file field has to.
  const pickedUnlisted = Boolean(initialPath) && !recents.some((r) => r.path === initialPath)

  useEffect(() => {
    passwordInput.current?.focus()
  }, [])

  function choose(r: Recent) {
    setPath(r.path)
    setKeyFile(r.keyFile ?? '')
    setShowKeyFile(Boolean(r.keyFile))
    setError('')
    passwordInput.current?.focus()
  }

  async function browseVault() {
    const chosen = await api.pickVaultFile()
    if (!chosen) return
    setPath(chosen)
    setCreating(false)
    setError('')
    passwordInput.current?.focus()
  }

  async function browseKeyFile() {
    const chosen = await api.pickKeyFile()
    if (!chosen) return
    setKeyFile(chosen)
    setError('')
  }

  // Creating starts at the save dialog: the user picks the location first,
  // then types the password, which is the order the OS dialog imposes anyway.
  async function startCreating() {
    const chosen = await api.pickNewVaultPath()
    if (!chosen) return
    setCreating(true)
    setPath(chosen)
    setPassword('')
    setConfirm('')
    setError('')
    passwordInput.current?.focus()
  }

  async function submit() {
    if (!path.trim() || busy) return
    if (!password && !keyFile.trim()) {
      setError('Enter the master password, or choose a key file.')
      passwordInput.current?.focus()
      return
    }
    // A new master password is typed blind and nothing can recover it, so a
    // typo here would make a vault nobody can open. Typing it twice is the
    // only check there is.
    if (creating && password !== confirm) {
      setError('The two passwords do not match.')
      setConfirm('')
      confirmInput.current?.focus()
      return
    }
    setBusy(true)
    setError('')
    try {
      const status = creating
        ? await api.createVault(path.trim(), password, keyFile.trim())
        : await api.unlock(path.trim(), password, keyFile.trim())
      setPassword('')
      setConfirm('')
      onUnlocked(status)
    } catch (e) {
      // The Go side never puts a password in an error, so this is safe to
      // show verbatim.
      setError(e instanceof Error ? e.message : String(e))
      setPassword('')
      setConfirm('')
      passwordInput.current?.focus()
    } finally {
      setBusy(false)
    }
  }

  async function forget(r: Recent) {
    await api.forgetRecent(r.path)
    onSettingsChanged()
  }

  return (
    <div className="flex h-full items-center justify-center bg-bg px-6">
      <div className="w-[384px]">
        <div className="mb-7">
          <Wordmark />
          <p className="mt-[10px] text-base leading-relaxed text-text-3">
            One encrypted file on this machine. No account, no server, nothing leaves the
            computer.
          </p>
        </div>

        <div>
          {recents.length > 0 && !creating && (
            <div className="mb-5">
              <p className="mb-[6px] text-sm text-text-3">Recent</p>
              <ul className="-mx-2">
                {recents.map((r) => {
                  const selected = r.path === path
                  return (
                    <li key={r.path} className="group relative">
                      <button
                        type="button"
                        onClick={() => choose(r)}
                        className={`row-transition flex h-[var(--row-h)] w-full items-center gap-2 rounded-sm pr-8 pl-2 text-left ${
                          selected ? 'bg-selected' : 'hover:bg-row-hover'
                        }`}
                      >
                        {selected && (
                          <span className="absolute top-[7px] bottom-[7px] left-0 w-[2px] rounded-full bg-accent" />
                        )}
                        <span className="truncate text-base text-text-1">{fileName(r.path)}</span>
                        <span className="truncate font-mono text-sm text-text-3">
                          {shortPath(r.path)}
                        </span>
                      </button>
                      <span className="absolute top-[3px] right-1 opacity-0 group-hover:opacity-100 focus-within:opacity-100">
                        <IconButton label="Remove from recent" onClick={() => void forget(r)}>
                          <X size={13} />
                        </IconButton>
                      </span>
                    </li>
                  )
                })}
              </ul>
            </div>
          )}

          {firstRun ? (
            <div className="flex flex-col gap-3">
              <p className="text-base leading-relaxed text-text-3">
                Nothing has been opened here yet.
              </p>
              <div className="flex gap-2">
                <Button
                  variant="primary"
                  icon={<Plus size={14} />}
                  onClick={() => void startCreating()}
                >
                  Create a vault
                </Button>
                <Button
                  variant="quiet"
                  icon={<FolderOpen size={14} />}
                  onClick={() => void browseVault()}
                >
                  Open a vault
                </Button>
              </div>
            </div>
          ) : (
          <form
            className="flex flex-col gap-3"
            onSubmit={(e) => {
              e.preventDefault()
              void submit()
            }}
          >
            {(recents.length === 0 || creating || pickedUnlisted) && (
              <Field label={creating ? 'New vault file' : 'Vault file'}>
                <div className="flex gap-1">
                  <Input
                    mono
                    readOnly
                    value={path}
                    spellCheck={false}
                    placeholder={creating ? 'Choose where to save it' : 'Choose a .kdbx file'}
                    onClick={() => void (creating ? startCreating() : browseVault())}
                    className="cursor-pointer"
                  />
                  <Button
                    variant="default"
                    icon={<FolderOpen size={14} />}
                    onClick={() => void (creating ? startCreating() : browseVault())}
                  >
                    Change
                  </Button>
                </div>
              </Field>
            )}

            <Field label={creating ? 'New master password' : 'Master password'}>
              <Input
                ref={passwordInput}
                type="password"
                value={password}
                autoComplete="off"
                onChange={(e) => setPassword(e.target.value)}
              />
            </Field>

            {creating && (
              <Field label="Repeat the master password">
                <Input
                  ref={confirmInput}
                  type="password"
                  value={confirm}
                  autoComplete="off"
                  onChange={(e) => setConfirm(e.target.value)}
                />
              </Field>
            )}

            {showKeyFile ? (
              <Field label="Key file">
                <div className="flex gap-1">
                  <Input
                    mono
                    readOnly
                    value={keyFile}
                    spellCheck={false}
                    placeholder="None chosen"
                    onClick={() => void browseKeyFile()}
                    className={keyFile ? 'cursor-pointer' : 'cursor-pointer font-sans'}
                  />
                  <Button
                    variant="default"
                    icon={<FolderOpen size={14} />}
                    onClick={() => void browseKeyFile()}
                  >
                    Browse
                  </Button>
                  <IconButton
                    label="Remove the key file"
                    onClick={() => {
                      setKeyFile('')
                      setShowKeyFile(false)
                    }}
                  >
                    <X size={13} />
                  </IconButton>
                </div>
              </Field>
            ) : (
              <button
                type="button"
                onClick={() => setShowKeyFile(true)}
                className="self-start text-base text-text-3 hover:text-text-1"
              >
                <span className="inline-flex items-center gap-[5px]">
                  <KeyRound size={13} />
                  Use a key file
                </span>
              </button>
            )}

            {/* Always occupied, so a rejected password does not move the
                field the user is about to type into again. */}
            <p
              role="alert"
              className="min-h-[2.6em] text-base leading-relaxed text-err"
            >
              {error}
            </p>

            <div className="mt-1 flex items-center gap-2">
              <Button type="submit" variant="primary" disabled={busy || !path.trim()}>
                {creating ? 'Create vault' : 'Unlock'}
                <ChevronRight size={14} />
              </Button>
              {!creating && (
                <Button
                  variant="quiet"
                  icon={<FolderOpen size={14} />}
                  onClick={() => void browseVault()}
                >
                  Open a vault
                </Button>
              )}
              <Button
                variant="quiet"
                icon={creating ? undefined : <Plus size={14} />}
                onClick={() => {
                  if (creating) {
                    setCreating(false)
                    setError('')
                    setPath(recents[0]?.path ?? '')
                    return
                  }
                  void startCreating()
                }}
              >
                {creating ? 'Open a vault instead' : 'Create a vault'}
              </Button>
            </div>
          </form>
          )}
        </div>
      </div>
    </div>
  )
}

/** The same open padlock as the app icon, drawn at the proportions in
 *  tools/icon/main.go so the window and the taskbar show one identity. The
 *  shackle's right leg stops short, which is what keeps the shape from
 *  reading as a letter. */
function Wordmark() {
  return (
    <div className="flex items-center gap-[10px]">
      <svg width="26" height="26" viewBox="0 0 100 100" aria-hidden="true" className="shrink-0">
        <rect x="28.5" y="50" width="43" height="30" rx="7.5" fill="var(--accent)" />
        <path
          d="M35.5 52V33a14.5 14.5 0 0 1 29 0v7"
          fill="none"
          stroke="var(--accent)"
          strokeWidth="11.6"
          strokeLinecap="round"
        />
      </svg>
      <span className="text-xl font-semibold tracking-[-0.02em] text-text-1">Vaulty</span>
    </div>
  )
}
