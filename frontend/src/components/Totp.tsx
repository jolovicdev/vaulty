import { useEffect, useState } from 'react'
import { api } from '../lib/api'
import type { TotpCode } from '../lib/types'
import { CopyButton } from './CopyButton'
import { SHORTCUTS } from '../lib/shortcuts'

/** Ring draws the time left in the current step. The stroke is a text tone,
 *  not the accent: a countdown that is always running should not be the
 *  loudest thing on the screen. */
function Ring({ fraction }: { fraction: number }) {
  const r = 6
  const circumference = 2 * Math.PI * r
  return (
    <svg width="16" height="16" viewBox="0 0 16 16" aria-hidden="true" className="shrink-0">
      <circle cx="8" cy="8" r={r} fill="none" stroke="var(--line)" strokeWidth="1.5" />
      <circle
        cx="8"
        cy="8"
        r={r}
        fill="none"
        stroke="var(--text-2)"
        strokeWidth="1.5"
        strokeLinecap="round"
        strokeDasharray={circumference}
        strokeDashoffset={circumference * (1 - fraction)}
        transform="rotate(-90 8 8)"
        style={{ transition: 'stroke-dashoffset var(--duration) linear' }}
      />
    </svg>
  )
}

export function Totp({ entryId }: { entryId: string }) {
  const [code, setCode] = useState<TotpCode | null>(null)
  const [remaining, setRemaining] = useState(0)

  useEffect(() => {
    let live = true
    async function refresh() {
      try {
        const next = await api.totp(entryId)
        if (!live) return
        setCode(next)
        setRemaining(next.remaining)
      } catch {
        if (live) setCode(null)
      }
    }
    void refresh()
    const id = setInterval(() => {
      setRemaining((r) => {
        if (r <= 1) {
          void refresh()
          return 0
        }
        return r - 1
      })
    }, 1000)
    return () => {
      live = false
      clearInterval(id)
    }
  }, [entryId])

  if (!code) return null

  // A code is grouped in threes, which is how every authenticator shows it.
  const half = Math.ceil(code.code.length / 2)
  const grouped = `${code.code.slice(0, half)} ${code.code.slice(half)}`

  return (
    <div className="flex items-center gap-[8px]">
      <Ring fraction={code.period > 0 ? remaining / code.period : 0} />
      <span className="font-mono text-base tracking-[0.08em] text-text-1 tabular">
        {grouped}
      </span>
      <span className="w-[22px] text-sm text-text-3 tabular">{remaining}s</span>
      <CopyButton
        label={SHORTCUTS.copyTotp.label}
        keys={SHORTCUTS.copyTotp.keys}
        onCopy={() => api.copyTotp(entryId)}
      />
    </div>
  )
}
