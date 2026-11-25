import { useEffect, useState } from 'react'
import { FolderOpen, KeyRound, X } from 'lucide-react'
import { api } from '../lib/api'
import type { StrengthResult } from '../lib/types'
import { Button, Dialog, Field, IconButton, Input } from './primitives'
import { Meter } from './Generator'

export function CredentialsDialog({
  currentKeyFile,
  showStrength,
  onClose,
  onChanged,
}: {
  currentKeyFile: string
  showStrength: boolean
  onClose: () => void
  onChanged: () => void
}) {
  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [keyFile, setKeyFile] = useState(currentKeyFile)
  const [showKeyFile, setShowKeyFile] = useState(Boolean(currentKeyFile))
  const [strengthState, setStrengthState] = useState<{
    password: string
    result: StrengthResult
  } | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const strength =
    showStrength && password && strengthState?.password === password ? strengthState.result : null

  useEffect(() => {
    if (!password || !showStrength) return
    let live = true
    void (async () => {
      const r = await api.estimateStrength(password)
      if (live) setStrengthState({ password, result: r })
    })()
    return () => {
      live = false
    }
  }, [password, showStrength])

  async function browseKeyFile() {
    const chosen = await api.pickKeyFile()
    if (!chosen) return
    setKeyFile(chosen)
    setError('')
  }

  async function submit() {
    if (busy) return
    if (!password && !keyFile.trim()) {
      setError('Enter a new master password, or choose a key file.')
      return
    }
    if (password !== confirm) {
      setError('The two passwords do not match.')
      return
    }
    setBusy(true)
    setError('')
    try {
      await api.changeCredentials(password, keyFile.trim())
      onChanged()
      onClose()
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
      setBusy(false)
    }
  }

  return (
    <Dialog
      title="Change master password"
      onClose={() => {
        if (!busy) onClose()
      }}
      width="w-[460px]"
      footer={
        <div className="flex items-center gap-2">
          <Button variant="primary" disabled={busy} onClick={() => void submit()}>
            {busy ? 'Saving' : 'Update credentials'}
          </Button>
          <Button variant="quiet" disabled={busy} onClick={onClose}>
            Cancel
          </Button>
        </div>
      }
    >
      <form
        className="flex flex-col gap-3 p-4"
        onSubmit={(e) => {
          e.preventDefault()
          void submit()
        }}
      >
        <p className="measure text-base leading-relaxed text-text-3">
          The vault is re-encrypted with a fresh salt and saved right away. A backup of the previous
          file is kept beside it.
        </p>

        <Field label="New master password">
          <Input
            type="password"
            value={password}
            autoComplete="off"
            autoFocus
            onChange={(e) => setPassword(e.target.value)}
          />
        </Field>

        {showStrength && password && <Meter result={strength} />}

        <Field label="Repeat the new master password">
          <Input
            type="password"
            value={confirm}
            autoComplete="off"
            onChange={(e) => setConfirm(e.target.value)}
          />
        </Field>

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

        {error && (
          <p role="alert" className="text-base text-err">
            {error}
          </p>
        )}
      </form>
    </Dialog>
  )
}
