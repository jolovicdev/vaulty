import { useEffect, useState } from 'react'
import { Eye, EyeOff, Plus, Wand2, X } from 'lucide-react'
import { api } from '../lib/api'
import { FIELD, type Detail, type Draft, type GroupNode, type StrengthResult } from '../lib/types'
import { Button, Dialog, Field, IconButton, Input, Secret, Select, Textarea } from './primitives'
import { Generator, Toggle } from './Generator'

interface CustomRow {
  key: string
  value: string
  protected: boolean
}

function flatten(groups: GroupNode[], depth = 0): { id: string; label: string }[] {
  return groups.flatMap((g) =>
    g.isRecycleBin
      ? []
      : [
          { id: g.id, label: `${'  '.repeat(depth)}${g.name}` },
          ...flatten(g.children, depth + 1),
        ],
  )
}

export function EntryEditor({
  detail,
  groups,
  defaultGroupId,
  showStrength,
  onClose,
  onSaved,
}: {
  detail: Detail | null
  groups: GroupNode[]
  defaultGroupId: string
  showStrength: boolean
  onClose: () => void
  onSaved: (id: string) => void
}) {
  const editing = detail !== null
  const [title, setTitle] = useState(detail?.title ?? '')
  const [username, setUsername] = useState(detail?.username ?? '')
  const [password, setPassword] = useState('')
  const [url, setUrl] = useState(detail?.url ?? '')
  const [notes, setNotes] = useState('')
  const [tags, setTags] = useState((detail?.tags ?? []).join(', '))
  const [totpSeed, setTotpSeed] = useState('')
  const [removeTotp, setRemoveTotp] = useState(false)
  const [groupId, setGroupId] = useState(detail?.groupId ?? defaultGroupId)
  const [custom, setCustom] = useState<CustomRow[]>([])
  const [strength, setStrength] = useState<StrengthResult | null>(null)
  const [showGenerator, setShowGenerator] = useState(false)
  const [revealPassword, setRevealPassword] = useState(!editing)
  const [error, setError] = useState('')
  const [loaded, setLoaded] = useState(!editing)

  // Editing pulls the existing secrets in, because an edit that silently
  // blanked the password would be worse than asking for it again.
  useEffect(() => {
    if (!detail) return
    void (async () => {
      const rows: CustomRow[] = []
      for (const f of detail.custom) {
        rows.push({
          key: f.key,
          value: f.protected ? await api.reveal(detail.id, f.key) : f.value,
          protected: f.protected,
        })
      }
      setCustom(rows)
      if (detail.passwordSet) setPassword(await api.reveal(detail.id, FIELD.password))
      if (detail.hasNotes) setNotes(await api.reveal(detail.id, FIELD.notes))
      setLoaded(true)
    })()
  }, [detail])

  useEffect(() => {
    if (!password || !showStrength) {
      setStrength(null)
      return
    }
    let live = true
    void (async () => {
      const r = await api.estimateStrength(password)
      if (live) setStrength(r)
    })()
    return () => {
      live = false
    }
  }, [password, showStrength])

  async function submit() {
    const draft: Draft = {
      title: title.trim(),
      username: username.trim(),
      password,
      url: url.trim(),
      notes,
      tags: tags
        .split(',')
        .map((t) => t.trim())
        .filter(Boolean),
      totpSeed: removeTotp ? '' : totpSeed.trim(),
      custom: Object.fromEntries(custom.filter((c) => c.key.trim()).map((c) => [c.key.trim(), c.value])),
      protectedCustom: custom.filter((c) => c.protected && c.key.trim()).map((c) => c.key.trim()),
      groupId,
      removeTotp,
    }
    try {
      if (detail) {
        await api.updateEntry(detail.id, draft)
        onSaved(detail.id)
      } else {
        onSaved(await api.addEntry(draft))
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    }
  }

  const groupOptions = flatten(groups)

  return (
    <Dialog
      title={editing ? 'Edit entry' : 'New entry'}
      onClose={onClose}
      width="w-[620px]"
      footer={
        showGenerator ? undefined : (
          <div className="flex items-center gap-2">
            <Button variant="primary" disabled={!loaded} onClick={() => void submit()}>
              {editing ? 'Save changes' : 'Create entry'}
            </Button>
            <Button variant="quiet" onClick={onClose}>
              Cancel
            </Button>
          </div>
        )
      }
    >
      {showGenerator ? (
        <Generator
          showStrength={showStrength}
          onUse={(value) => {
            setPassword(value)
            setRevealPassword(true)
            setShowGenerator(false)
          }}
        />
      ) : (
        <form
          className="flex flex-col gap-3 p-4"
          onSubmit={(e) => {
            e.preventDefault()
            void submit()
          }}
        >
          <Field label="Title">
            <Input value={title} onChange={(e) => setTitle(e.target.value)} autoFocus />
          </Field>

          <div className="grid grid-cols-2 gap-3">
            <Field label="Username">
              <Input mono value={username} onChange={(e) => setUsername(e.target.value)} />
            </Field>
            <Field label="Group">
              <Select value={groupId} onChange={(e) => setGroupId(e.target.value)}>
                {groupOptions.map((g) => (
                  <option key={g.id} value={g.id}>
                    {g.label}
                  </option>
                ))}
              </Select>
            </Field>
          </div>

          <Field
            label="Password"
            hint={
              strength
                ? strength.machine
                  ? `${strength.issuer || 'random'} · ${strength.entropy.toFixed(0)} bits`
                  : `${strength.label} · ${strength.entropy.toFixed(0)} bits`
                : undefined
            }
            actions={
              <>
                <IconButton
                  label={revealPassword ? 'Hide password' : 'Reveal password'}
                  onClick={() => setRevealPassword(!revealPassword)}
                >
                  {revealPassword ? <EyeOff size={14} /> : <Eye size={14} />}
                </IconButton>
                <IconButton label="Open the generator" onClick={() => setShowGenerator(true)}>
                  <Wand2 size={14} />
                </IconButton>
              </>
            }
          >
            <div className="relative min-w-0 flex-1">
              <Input
                mono
                type={revealPassword ? 'text' : 'password'}
                value={password}
                autoComplete="off"
                onChange={(e) => setPassword(e.target.value)}
                className={revealPassword ? 'text-transparent caret-[var(--text-1)]' : ''}
              />
              {revealPassword && (
                <span className="pointer-events-none absolute inset-0 flex items-center overflow-hidden px-[9px]">
                  <Secret value={password} />
                </span>
              )}
            </div>
          </Field>

          {/* No rule for a value the user was issued rather than chose; the
              hint above already states what it is and how big it is. */}
          {strength && !strength.machine && (
            <span className="flex h-[2px] overflow-hidden rounded-full bg-line">
              <span
                className={`rounded-full ${
                  strength.score <= 1 ? 'bg-err' : strength.score === 2 ? 'bg-warn' : 'bg-ok'
                }`}
                style={{
                  width: `${((strength.score + 1) / 5) * 100}%`,
                  transition: 'width var(--duration) var(--ease)',
                }}
              />
            </span>
          )}

          <Field label="URL">
            <Input value={url} onChange={(e) => setUrl(e.target.value)} spellCheck={false} />
          </Field>

          <Field label="Tags" hint="comma separated">
            <Input value={tags} onChange={(e) => setTags(e.target.value)} />
          </Field>

          <Field label="Notes">
            <Textarea value={notes} onChange={(e) => setNotes(e.target.value)} rows={3} />
          </Field>

          {/* The stored seed is never loaded, so an empty box means unchanged
              and taking the code off needs its own control. It only shows
              for an entry that has a code to take off. */}
          <Field
            label="One-time code secret"
            hint="otpauth:// URI or base32 seed"
            actions={
              detail?.hasTotp ? (
                // The action row's gap is sized for icon buttons, which carry
                // their own padding. A bare checkbox needs the rest to sit as
                // far from its input as Protect does in the rows below.
                <span className="flex items-center pl-1">
                  <Toggle label="Remove" checked={removeTotp} onChange={setRemoveTotp} />
                </span>
              ) : undefined
            }
          >
            <Input
              mono
              value={removeTotp ? '' : totpSeed}
              onChange={(e) => setTotpSeed(e.target.value)}
              disabled={removeTotp}
              placeholder={
                removeTotp
                  ? 'Removed when you save.'
                  : detail?.hasTotp
                    ? 'Unchanged. Type to replace.'
                    : ''
              }
              spellCheck={false}
              className="disabled:cursor-not-allowed disabled:bg-disabled-surface"
            />
          </Field>

          <div className="flex flex-col gap-2 border-t border-line pt-3">
            <div className="flex items-center justify-between">
              <span className="text-sm text-text-3">Custom fields</span>
              <Button
                variant="quiet"
                icon={<Plus size={13} />}
                onClick={() => setCustom([...custom, { key: '', value: '', protected: false }])}
              >
                Add
              </Button>
            </div>
            {custom.map((row, i) => (
              <div key={i} className="flex items-center gap-2">
                <Input
                  aria-label={`Custom field ${i + 1} name`}
                  value={row.key}
                  placeholder="Name"
                  onChange={(e) =>
                    setCustom(custom.map((c, j) => (i === j ? { ...c, key: e.target.value } : c)))
                  }
                  className="w-[150px]"
                />
                <Input
                  mono
                  aria-label={row.key ? `${row.key} value` : `Custom field ${i + 1} value`}
                  value={row.value}
                  placeholder="Value"
                  type={row.protected ? 'password' : 'text'}
                  onChange={(e) =>
                    setCustom(custom.map((c, j) => (i === j ? { ...c, value: e.target.value } : c)))
                  }
                />
                <Toggle
                  label="Protect"
                  checked={row.protected}
                  onChange={(checked) =>
                    setCustom(custom.map((c, j) => (i === j ? { ...c, protected: checked } : c)))
                  }
                />
                <IconButton
                  label={`Remove ${row.key || 'field'}`}
                  onClick={() => setCustom(custom.filter((_, j) => j !== i))}
                >
                  <X size={13} />
                </IconButton>
              </div>
            ))}
          </div>

          {error && (
            <p role="alert" className="text-base text-err">
              {error}
            </p>
          )}
        </form>
      )}
    </Dialog>
  )
}
