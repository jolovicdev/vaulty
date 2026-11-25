import { useEffect, useState } from 'react'
import { CheckCircle2, Clock, Copy, ShieldAlert } from 'lucide-react'
import { api } from '../lib/api'
import type { AuditReport, Meta } from '../lib/types'
import { Button, Dialog } from './primitives'

type Tab = 'weak' | 'reused' | 'expiry'

function formatDate(iso?: string): string {
  if (!iso) return ''
  const d = new Date(iso)
  if (!Number.isFinite(d.getTime())) return ''
  return d.toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' })
}

export function AuditDialog({
  onClose,
  onSelectEntry,
}: {
  onClose: () => void
  onSelectEntry: (id: string) => void
}) {
  const [report, setReport] = useState<AuditReport | null>(null)
  const [tab, setTab] = useState<Tab>('weak')
  const [error, setError] = useState('')

  useEffect(() => {
    let live = true
    void (async () => {
      try {
        const rep = await api.audit()
        if (!live) return
        setReport(rep)
        if (rep.weak.length > 0) setTab('weak')
        else if (rep.reused.length > 0) setTab('reused')
        else if (rep.expired.length > 0 || rep.expiringSoon.length > 0) setTab('expiry')
      } catch (e) {
        if (live) setError(e instanceof Error ? e.message : String(e))
      }
    })()
    return () => {
      live = false
    }
  }, [])

  const expiryCount = (report?.expired.length ?? 0) + (report?.expiringSoon.length ?? 0)
  const totalIssues =
    (report?.weak.length ?? 0) + (report?.reused.length ?? 0) + expiryCount

  function open(entry: Meta) {
    onSelectEntry(entry.id)
    onClose()
  }

  return (
    <Dialog
      title="Vault health"
      description={
        report
          ? `${report.totalChecked} ${report.totalChecked === 1 ? 'entry' : 'entries'} checked`
          : undefined
      }
      onClose={onClose}
      width="w-[580px]"
      footer={
        <Button variant="quiet" onClick={onClose}>
          Close
        </Button>
      }
    >
      <div className="flex flex-col gap-4 p-4">
        {error && (
          <p role="alert" className="text-base text-err">
            {error}
          </p>
        )}

        {report && totalIssues === 0 && (
          <div className="flex flex-col items-center gap-2 py-8 text-center">
            <CheckCircle2 size={22} className="text-ok" />
            <p className="text-md text-text-1">Nothing needs attention</p>
            <p className="text-base text-text-3">
              No weak, reused, or expiring passwords across {report.totalChecked}{' '}
              {report.totalChecked === 1 ? 'entry' : 'entries'}.
            </p>
          </div>
        )}

        {report && totalIssues > 0 && (
          <>
            <div className="flex gap-1 border-b border-line pb-3">
              <TabButton
                active={tab === 'weak'}
                icon={<ShieldAlert size={13} />}
                label="Weak"
                count={report.weak.length}
                onClick={() => setTab('weak')}
              />
              <TabButton
                active={tab === 'reused'}
                icon={<Copy size={13} />}
                label="Reused"
                count={report.reused.length}
                onClick={() => setTab('reused')}
              />
              <TabButton
                active={tab === 'expiry'}
                icon={<Clock size={13} />}
                label="Expiring"
                count={expiryCount}
                onClick={() => setTab('expiry')}
              />
            </div>

            {tab === 'weak' && (
              <div className="flex flex-col">
                {report.weak.length === 0 ? (
                  <p className="py-6 text-center text-base text-text-3">No weak passwords found.</p>
                ) : (
                  report.weak.map((item) => (
                    <EntryRow
                      key={item.entry.id}
                      entry={item.entry}
                      right={
                        <span className="shrink-0 text-sm text-err">
                          {item.label}
                          {item.warning ? ` · ${item.warning}` : ''}
                        </span>
                      }
                      onClick={() => open(item.entry)}
                    />
                  ))
                )}
              </div>
            )}

            {tab === 'reused' && (
              <div className="flex flex-col gap-3">
                {report.reused.length === 0 ? (
                  <p className="py-6 text-center text-base text-text-3">
                    No reused passwords found.
                  </p>
                ) : (
                  report.reused.map((grp, idx) => (
                    <div key={idx} className="rounded-sm border border-line bg-surface-2">
                      <div className="border-b border-line px-3 py-[6px] text-sm text-warn">
                        {grp.entries.length} entries share the same password
                      </div>
                      {grp.entries.map((entry) => (
                        <EntryRow key={entry.id} entry={entry} onClick={() => open(entry)} />
                      ))}
                    </div>
                  ))
                )}
              </div>
            )}

            {tab === 'expiry' && (
              <div className="flex flex-col gap-3">
                {expiryCount === 0 ? (
                  <p className="py-6 text-center text-base text-text-3">
                    No expired or soon-expiring entries.
                  </p>
                ) : (
                  <>
                    {report.expired.map((entry) => (
                      <EntryRow
                        key={entry.id}
                        entry={entry}
                        right={
                          <span className="shrink-0 font-mono text-sm text-err tabular">
                            Expired {formatDate(entry.expiryTime)}
                          </span>
                        }
                        onClick={() => open(entry)}
                      />
                    ))}
                    {report.expiringSoon.map((entry) => (
                      <EntryRow
                        key={entry.id}
                        entry={entry}
                        right={
                          <span className="shrink-0 font-mono text-sm text-warn tabular">
                            Expires {formatDate(entry.expiryTime)}
                          </span>
                        }
                        onClick={() => open(entry)}
                      />
                    ))}
                  </>
                )}
              </div>
            )}
          </>
        )}
      </div>
    </Dialog>
  )
}

function TabButton({
  active,
  icon,
  label,
  count,
  onClick,
}: {
  active: boolean
  icon: React.ReactNode
  label: string
  count: number
  onClick: () => void
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={`row-transition inline-flex h-[28px] items-center gap-[6px] rounded-sm px-[10px] text-base ${
        active ? 'bg-selected text-text-1' : 'text-text-3 hover:text-text-1'
      }`}
    >
      {icon}
      <span>{label}</span>
      <span className="font-mono text-sm tabular">{count}</span>
    </button>
  )
}

function EntryRow({
  entry,
  right,
  onClick,
}: {
  entry: Meta
  right?: React.ReactNode
  onClick: () => void
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className="row-transition flex w-full items-center gap-3 border-b border-line/60 px-3 py-[8px] text-left last:border-0 hover:bg-row-hover"
    >
      <span className="flex min-w-0 flex-1 flex-col">
        <span className="truncate text-base text-text-1">{entry.title || 'Untitled'}</span>
        <span className="truncate font-mono text-sm text-text-3">
          {entry.username || entry.groupPath}
        </span>
      </span>
      {right}
    </button>
  )
}
