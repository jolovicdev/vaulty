import { useEffect, useState } from 'react'
import { Check, Copy } from 'lucide-react'
import type { CopyResult } from '../lib/types'
import { Tooltip } from './primitives'

/** CopyButton replaces its own label with the countdown to the clipboard
 *  clear. The confirmation lives where the action was, so there is nothing
 *  to dismiss and no toast. */
export function CopyButton({
  label,
  keys,
  onCopy,
}: {
  label: string
  keys?: string
  onCopy: () => Promise<CopyResult>
}) {
  const [result, setResult] = useState<CopyResult | null>(null)
  const [left, setLeft] = useState(0)

  useEffect(() => {
    if (!result) return
    if (!result.expiresAt) {
      const done = setTimeout(() => setResult(null), 1200)
      return () => clearTimeout(done)
    }
    function tick() {
      const secs = Math.ceil((result!.expiresAt - Date.now()) / 1000)
      setLeft(secs)
      if (secs <= 0) setResult(null)
    }
    tick()
    const id = setInterval(tick, 250)
    return () => clearInterval(id)
  }, [result])

  async function run() {
    try {
      setResult(await onCopy())
    } catch {
      setResult(null)
    }
  }

  // The button keeps its place in the row and the confirmation floats over
  // it, right aligned, on one line. Anything that gives the confirmation its
  // own space in the layout either moves the controls beside it or leaves a
  // gap the width of a two digit countdown.
  return (
    <span className="relative inline-flex h-[28px] w-[28px] shrink-0">
      <Tooltip label={label} keys={keys}>
        <button
          type="button"
          aria-label={label}
          onClick={() => void run()}
          className={`row-transition inline-flex h-[28px] w-[28px] shrink-0 items-center justify-center rounded-sm text-text-3 hover:bg-row-hover hover:text-text-1 ${
            result ? 'invisible' : ''
          }`}
        >
          <Copy size={13} />
        </button>
      </Tooltip>
      {result && (
        <span className="pointer-events-none absolute top-0 right-0 flex h-[28px] items-center gap-[5px] rounded-sm bg-surface-2 pr-[7px] pl-2 text-sm whitespace-nowrap text-ok">
          <Check size={13} className="shrink-0" />
          <span className="tabular">{left > 0 ? `Copied · ${left}s` : 'Copied'}</span>
          {result.expiresAt > 0 && (
            <span
              className="animate-drain absolute bottom-0 left-0 h-[1.5px] w-full bg-ok/40"
              style={{ animationDuration: `${result.seconds}s` }}
            />
          )}
        </span>
      )}
    </span>
  )
}
