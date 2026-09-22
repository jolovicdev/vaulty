import { Keys } from './primitives'
import { SHORTCUTS } from '../lib/shortcuts'

/** Shown instead of the list and detail panes when the vault holds no
 *  entries at all. One empty state, not two, and it explains what to do
 *  rather than reporting that there is nothing. */
export function EmptyVault({ onNew, onImport }: { onNew: () => void; onImport: () => void }) {
  const steps = [
    <>
      Add your first entry with <Keys keys={SHORTCUTS.newEntry.keys} />, or{' '}
      <button
        type="button"
        onClick={onNew}
        className="text-accent underline decoration-transparent underline-offset-2 hover:decoration-inherit"
      >
        create one now
      </button>
      .
    </>,
    <>
      Coming from another password manager?{' '}
      <button
        type="button"
        onClick={onImport}
        className="text-accent underline decoration-transparent underline-offset-2 hover:decoration-inherit"
      >
        Import
      </button>{' '}
      an export from Proton Pass, Bitwarden or 1Password.
    </>,
    <>
      Let the generator pick the password. <Keys keys={SHORTCUTS.generator.keys} /> opens it on its
      own.
    </>,
    <>
      Write the vault to disk with <Keys keys={SHORTCUTS.save.keys} />. A backup from each of the
      last five sessions is kept beside it.
    </>,
    <>
      Everything else is reachable from the command palette, <Keys keys={SHORTCUTS.palette.keys} />.
    </>,
  ]

  return (
    <section className="flex h-full min-w-0 flex-1 items-center justify-center bg-bg px-8">
      <div className="measure">
        <h2 className="text-lg leading-tight font-semibold tracking-[-0.02em] text-text-1">
          This vault is empty
        </h2>
        <ol className="mt-5 flex flex-col gap-[10px]">
          {steps.map((step, i) => (
            <li key={i} className="flex gap-3 text-base leading-relaxed text-text-2">
              <span className="w-[14px] shrink-0 font-mono text-sm text-text-3 tabular">
                {i + 1}
              </span>
              <span>{step}</span>
            </li>
          ))}
        </ol>
      </div>
    </section>
  )
}
