import { useCallback, useEffect, useState } from 'react'
import { Check, Copy, RefreshCw } from 'lucide-react'
import { api } from '../lib/api'
import type { PassphraseOptions, PasswordOptions, StrengthResult } from '../lib/types'
import { Button, Field, Secret } from './primitives'

type Mode = 'password' | 'passphrase'

/** The meter is four segments, coloured by band. Bands, not a gradient, so
 *  the same password always reads the same way. */
export function Meter({ result }: { result: StrengthResult | null }) {
  // A value the user did not choose gets a statement of fact, not a grade.
  // Telling someone their GitHub token is "very strong" is noise: they did
  // not pick it and they cannot change it.
  if (result?.machine) {
    return (
      <p className="flex items-baseline justify-between gap-3 text-sm text-text-3">
        <span className="truncate">{result.issuer || 'Randomly generated'}</span>
        <span className="shrink-0 tabular">{result.entropy.toFixed(0)} bits</span>
      </p>
    )
  }

  const score = result?.score ?? 0
  const tone = score <= 1 ? 'text-err' : score === 2 ? 'text-warn' : 'text-ok'
  const track = score <= 1 ? 'bg-err' : score === 2 ? 'bg-warn' : 'bg-ok'

  // One reading, not three. The word and the number say it; the rule only
  // encodes how far along the scale it sits, which is what stays legible
  // while the length slider is being dragged.
  return (
    <div className="flex flex-col gap-[6px]">
      <span className="flex h-[2px] overflow-hidden rounded-full bg-line">
        <span
          className={`${track} rounded-full`}
          style={{ width: `${((score + 1) / 5) * 100}%`, transition: 'width var(--duration) var(--ease)' }}
        />
      </span>
      {result && (
        <p className="flex items-baseline justify-between gap-3 text-sm">
          <span className={tone}>
            {result.label}
            {result.warning && <span className="text-text-3"> · {result.warning}</span>}
          </span>
          <span className="shrink-0 font-mono text-text-3 tabular">
            {result.entropy.toFixed(0)} bits
          </span>
        </p>
      )}
    </div>
  )
}

export function Generator({
  onUse,
  showStrength = true,
  useLabel = 'Use this password',
}: {
  onUse?: (value: string) => void
  showStrength?: boolean
  useLabel?: string
}) {
  const [mode, setMode] = useState<Mode>('password')
  const [pwOpts, setPwOpts] = useState<PasswordOptions | null>(null)
  const [ppOpts, setPpOpts] = useState<PassphraseOptions | null>(null)
  const [value, setValue] = useState('')
  const [result, setResult] = useState<StrengthResult | null>(null)
  const [error, setError] = useState('')
  const [copied, setCopied] = useState(false)

  useEffect(() => {
    void (async () => {
      const defaults = await api.defaultGeneratorOptions()
      setPwOpts(defaults.password)
      setPpOpts(defaults.passphrase)
    })()
  }, [])

  const regenerate = useCallback(async () => {
    if (!pwOpts || !ppOpts) return
    try {
      const next =
        mode === 'password'
          ? await api.generatePassword(pwOpts)
          : await api.generatePassphrase(ppOpts)
      setValue(next)
      setError('')
      setResult(await api.estimateStrength(next))
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
      setValue('')
      setResult(null)
    }
  }, [mode, pwOpts, ppOpts])

  useEffect(() => {
    void regenerate()
  }, [regenerate])

  // The copy goes back through Go even though the value is already here:
  // that is what applies the timed clear and, on Windows, the formats that
  // keep it out of Clipboard History. navigator.clipboard would skip both.
  async function copy() {
    if (!value) return
    try {
      await api.copyGenerated(value)
      setCopied(true)
      setTimeout(() => setCopied(false), 1600)
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    }
  }

  if (!pwOpts || !ppOpts) return null

  return (
    <div className="flex flex-col gap-4 p-4">
      <div className="flex items-center gap-2 rounded-sm border border-line bg-surface-2 p-3">
        <span className="min-w-0 flex-1 break-all">
          {value ? <Secret value={value} className="text-md" /> : <span className="text-err text-base">{error}</span>}
        </span>
        <Button
          variant="quiet"
          aria-label="Generate another"
          onClick={() => void regenerate()}
          icon={<RefreshCw size={14} />}
        />
      </div>

      {showStrength && <Meter result={result} />}

      <div className="flex gap-1">
        {(['password', 'passphrase'] as const).map((m) => (
          <button
            key={m}
            type="button"
            onClick={() => setMode(m)}
            className={`row-transition h-[26px] rounded-sm px-[10px] text-base ${
              mode === m ? 'bg-selected text-text-1' : 'text-text-3 hover:text-text-1'
            }`}
          >
            {m === 'password' ? 'Password' : 'Passphrase'}
          </button>
        ))}
      </div>

      {mode === 'password' ? (
        <div className="flex flex-col gap-3">
          <Field label="Length" hint={String(pwOpts.length)}>
            <input
              type="range"
              min={8}
              max={64}
              value={pwOpts.length}
              onChange={(e) => setPwOpts({ ...pwOpts, length: Number(e.target.value) })}
              className="control-range my-[8px]"
            />
          </Field>
          <div className="flex flex-col gap-[6px]">
            {(
              [
                ['lower', 'Lower case'],
                ['upper', 'Upper case'],
                ['digits', 'Digits'],
                ['symbols', 'Symbols'],
                ['excludeAmbiguous', 'Exclude look-alike characters'],
              ] as const
            ).map(([key, label]) => (
              <Toggle
                key={key}
                label={label}
                checked={pwOpts[key]}
                onChange={(checked) => setPwOpts({ ...pwOpts, [key]: checked })}
              />
            ))}
          </div>
        </div>
      ) : (
        <div className="flex flex-col gap-3">
          <Field label="Words" hint={String(ppOpts.words)}>
            <input
              type="range"
              min={3}
              max={12}
              value={ppOpts.words}
              onChange={(e) => setPpOpts({ ...ppOpts, words: Number(e.target.value) })}
              className="control-range my-[8px]"
            />
          </Field>
          <Field label="Separator">
            <div className="flex gap-1">
              {['-', '.', '_', ' '].map((sep) => (
                <button
                  key={sep}
                  type="button"
                  onClick={() => setPpOpts({ ...ppOpts, separator: sep })}
                  className={`h-[26px] w-[30px] rounded-sm font-mono text-base ${
                    ppOpts.separator === sep
                      ? 'bg-selected text-text-1'
                      : 'text-text-3 hover:text-text-1'
                  }`}
                >
                  {sep === ' ' ? '␣' : sep}
                </button>
              ))}
            </div>
          </Field>
          <div className="flex flex-col gap-[6px]">
            <Toggle
              label="Capitalise each word"
              checked={ppOpts.capitalize}
              onChange={(checked) => setPpOpts({ ...ppOpts, capitalize: checked })}
            />
            <Toggle
              label="Add a number"
              checked={ppOpts.addNumber}
              onChange={(checked) => setPpOpts({ ...ppOpts, addNumber: checked })}
            />
          </div>
          <p className="text-sm text-text-3">
            Words come from the EFF long list, 7776 entries, bundled with the app.
          </p>
        </div>
      )}

      {onUse ? (
        <Button variant="primary" disabled={!value} onClick={() => onUse(value)}>
          {useLabel}
        </Button>
      ) : (
        // Opened on its own rather than from the editor. Without this the
        // dialog produced a password and offered no way to take it.
        <Button
          variant="primary"
          disabled={!value}
          icon={copied ? <Check size={14} /> : <Copy size={14} />}
          onClick={() => void copy()}
        >
          {copied ? 'Copied to the clipboard' : 'Copy to the clipboard'}
        </Button>
      )}
    </div>
  )
}

export function Toggle({
  label,
  checked,
  onChange,
}: {
  label: string
  checked: boolean
  onChange: (checked: boolean) => void
}) {
  return (
    <label className="flex cursor-pointer items-center gap-[8px] text-base text-text-2 select-none">
      <input
        type="checkbox"
        checked={checked}
        onChange={(e) => onChange(e.target.checked)}
        className="control-checkbox"
      />
      {label}
    </label>
  )
}
