import {
  createContext,
  useContext,
  useEffect,
  useId,
  useLayoutEffect,
  useRef,
  useState,
  type ButtonHTMLAttributes,
  type InputHTMLAttributes,
  type Ref,
  type SelectHTMLAttributes,
  type TextareaHTMLAttributes,
  type ReactNode,
} from 'react'
import { createPortal } from 'react-dom'
import { keyParts } from '../lib/shortcuts'

/** Kbd renders one key cap. Machine-ish, so mono, but small. */
export function Kbd({ children }: { children: ReactNode }) {
  return (
    <kbd className="rounded-[4px] border border-line bg-surface-3 px-[5px] py-[1px] font-mono text-[10px] leading-[14px] text-text-3">
      {children}
    </kbd>
  )
}

/** Keys renders a whole shortcut as separate caps. */
export function Keys({ keys }: { keys: string }) {
  return (
    <span className="inline-flex items-center gap-[3px]">
      {keyParts(keys).map((k) => (
        <Kbd key={k}>{k}</Kbd>
      ))}
    </span>
  )
}

/** Tooltip shows a label and, when given, the shortcut that does the same
 *  thing. It opens on hover and on keyboard focus.
 *
 *  It renders into a portal with fixed positioning rather than as an
 *  absolutely positioned child, because an ancestor with its own overflow
 *  clips a positioned descendant and no amount of clamping to the window
 *  can undo that. Portalled, it is bounded only by the window, which is
 *  what the measurement below clamps it to. */
export function Tooltip({
  label,
  keys,
  children,
}: {
  label: string
  keys?: string
  children: ReactNode
}) {
  const [open, setOpen] = useState(false)
  const [pos, setPos] = useState<{ left: number; top: number } | null>(null)
  const trigger = useRef<HTMLSpanElement>(null)
  const bubble = useRef<HTMLSpanElement>(null)
  const id = useId()

  useLayoutEffect(() => {
    if (!open || !trigger.current || !bubble.current) return
    const margin = 6
    const anchor = trigger.current.getBoundingClientRect()
    const box = bubble.current.getBoundingClientRect()

    // Above the trigger by default, below it when there is no room.
    let top = anchor.top - box.height - margin
    if (top < margin) top = anchor.bottom + margin

    let left = anchor.left + anchor.width / 2 - box.width / 2
    left = Math.max(margin, Math.min(left, window.innerWidth - margin - box.width))
    setPos({ left, top })
  }, [open, label, keys])

  const hide = () => {
    setOpen(false)
    setPos(null)
  }

  return (
    <span
      ref={trigger}
      className="relative inline-flex"
      onPointerEnter={() => setOpen(true)}
      onPointerLeave={hide}
      onFocus={() => setOpen(true)}
      onBlur={hide}
    >
      <span aria-describedby={open ? id : undefined}>{children}</span>
      {open &&
        createPortal(
          <span
            ref={bubble}
            id={id}
            role="tooltip"
            style={{
              left: pos?.left ?? 0,
              top: pos?.top ?? 0,
              // Hidden for the frame before the measurement lands, so it
              // never flashes at the wrong place.
              visibility: pos ? 'visible' : 'hidden',
            }}
            className="pointer-events-none fixed z-[70] flex items-center gap-2 whitespace-nowrap rounded-sm border border-line bg-surface-3 px-2 py-1 text-sm text-text-2"
          >
            {label}
            {keys && <Keys keys={keys} />}
          </span>,
          document.body,
        )}
    </span>
  )
}

type ButtonVariant = 'primary' | 'default' | 'quiet' | 'danger'

const BUTTON_VARIANTS: Record<ButtonVariant, string> = {
  primary:
    'bg-accent text-accent-ink hover:brightness-110 border border-transparent disabled:bg-disabled-surface disabled:text-disabled-text disabled:border-line',
  default: 'bg-surface-2 text-text-1 border border-line-strong hover:bg-row-hover',
  quiet: 'bg-transparent text-text-2 border border-transparent hover:bg-row-hover hover:text-text-1',
  danger: 'bg-transparent text-err border border-line-strong hover:bg-surface-2',
}

export function Button({
  variant = 'default',
  icon,
  children,
  className = '',
  ...rest
}: ButtonHTMLAttributes<HTMLButtonElement> & { variant?: ButtonVariant; icon?: ReactNode }) {
  return (
    <button
      type="button"
      // A disabled button changes surface rather than fading: a filled
      // accent at reduced opacity still reads as an action, and its label
      // falls below 2:1.
      className={`row-transition inline-flex h-[var(--control-h)] shrink-0 items-center justify-center gap-[6px] rounded-sm px-[10px] text-base font-medium disabled:cursor-not-allowed ${BUTTON_VARIANTS[variant]} ${className}`}
      {...rest}
    >
      {icon}
      {children}
    </button>
  )
}

/** IconButton is a square button for an icon with no label; the tooltip
 *  supplies the name, so it is never unlabelled. */
export function IconButton({
  label,
  keys,
  children,
  ...rest
}: ButtonHTMLAttributes<HTMLButtonElement> & { label: string; keys?: string }) {
  return (
    <Tooltip label={label} keys={keys}>
      <button
        type="button"
        aria-label={label}
        className="row-transition inline-flex h-[28px] w-[28px] items-center justify-center rounded-sm text-text-3 hover:bg-row-hover hover:text-text-1 disabled:cursor-not-allowed disabled:opacity-40"
        {...rest}
      >
        {children}
      </button>
    </Tooltip>
  )
}

export function Input({
  className = '',
  mono = false,
  id,
  ...rest
}: InputHTMLAttributes<HTMLInputElement> & {
  mono?: boolean
  ref?: Ref<HTMLInputElement>
}) {
  const fromField = useContext(fieldID)
  return (
    <input
      id={id ?? fromField}
      className={`h-[var(--control-h)] w-full rounded-sm border border-line-strong bg-surface-1 px-[9px] text-base text-text-1 focus-visible:border-accent ${mono ? 'font-mono' : ''} ${className}`}
      {...rest}
    />
  )
}

/** Textarea and Select take the same id, so a Field labels them too. */
export function Textarea({
  className = '',
  id,
  ...rest
}: TextareaHTMLAttributes<HTMLTextAreaElement>) {
  const fromField = useContext(fieldID)
  return (
    <textarea
      id={id ?? fromField}
      className={`w-full resize-y rounded-sm border border-line-strong bg-surface-1 px-[9px] py-[6px] text-base text-text-1 ${className}`}
      {...rest}
    />
  )
}

export function Select({
  className = '',
  id,
  children,
  ...rest
}: SelectHTMLAttributes<HTMLSelectElement>) {
  const fromField = useContext(fieldID)
  return (
    <select
      id={id ?? fromField}
      className={`control-select h-[var(--control-h)] w-full rounded-sm border border-line-strong bg-surface-1 px-[7px] text-base text-text-1 ${className}`}
      {...rest}
    >
      {children}
    </select>
  )
}

/** fieldID carries the generated id from a Field down to the Input inside
 *  it, so the label points at the control without every caller wiring an id
 *  by hand. */
const fieldID = createContext<string | undefined>(undefined)

/** Field is a label above a control. Labels are sans and dim; the value
 *  carries the weight.
 *
 *  Buttons that act on the control go in actions, not in children. They
 *  must sit outside the label element: a label's accessible name is built
 *  from every descendant's text, so a button inside it becomes part of the
 *  control's announced name. */
export function Field({
  label,
  hint,
  actions,
  children,
}: {
  label: string
  hint?: string
  actions?: ReactNode
  children: ReactNode
}) {
  const id = useId()
  return (
    <div className="flex flex-col gap-[5px]">
      <label className="flex items-baseline justify-between gap-2" htmlFor={id}>
        <span className="text-sm text-text-3">{label}</span>
        {hint && <span className="text-sm text-text-3 tabular">{hint}</span>}
      </label>
      <fieldID.Provider value={id}>
        {actions ? <div className="flex gap-1">{children}{actions}</div> : children}
      </fieldID.Provider>
    </div>
  )
}

/** Secret renders a password so that 0/O and 1/l can be told apart: letters
 *  read as text, digits take the accent hue, symbols sit one tier down. */
export function Secret({ value, className = '' }: { value: string; className?: string }) {
  return (
    <span className={`font-mono tabular ${className}`}>
      {[...value].map((ch, i) => {
        const cls = /[0-9]/.test(ch)
          ? 'text-pw-digit'
          : /[a-z]/i.test(ch)
            ? 'text-pw-letter'
            : 'text-pw-symbol'
        return (
          <span key={i} className={cls}>
            {ch}
          </span>
        )
      })}
    </span>
  )
}

/** Dots stands in for a hidden password. Fixed width so revealing does not
 *  jump the layout around. */
export function Dots() {
  return <span className="font-mono text-text-3">••••••••••••</span>
}

interface DialogContext {
  close: () => void
}

const dialogCtx = createContext<DialogContext | null>(null)

export function useDialog() {
  const ctx = useContext(dialogCtx)
  if (!ctx) throw new Error('useDialog outside a Dialog')
  return ctx
}

/** Dialog traps focus and closes on Escape. It is the only element in the
 *  app that floats above everything else. */
export function Dialog({
  title,
  description,
  onClose,
  children,
  footer,
  width = 'w-[520px]',
}: {
  title: string
  description?: string
  onClose: () => void
  children: ReactNode
  /** Pinned below the scroll area, so a long form's primary action stays
   *  reachable. */
  footer?: ReactNode
  width?: string
}) {
  const panel = useRef<HTMLDivElement>(null)

  useEffect(() => {
    // Only look inside the scrolling body. Focusing a control in the pinned
    // footer scrolls the body to its end, so the dialog would open part way
    // down its own content.
    const body = panel.current?.querySelector<HTMLElement>('[data-dialog-body]')
    const first = body?.querySelector<HTMLElement>(
      'input, button, select, textarea, [tabindex]:not([tabindex="-1"])',
    )
    if (first) {
      first.focus()
      return
    }
    // Nothing focusable in the body: focus the panel itself so Escape and
    // the arrow keys still reach it, without scrolling anywhere.
    panel.current?.focus()
  }, [])

  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.key === 'Escape') {
        e.stopPropagation()
        onClose()
        return
      }
      if (e.key !== 'Tab' || !panel.current) return
      const focusable = [
        ...panel.current.querySelectorAll<HTMLElement>(
          'input:not([disabled]), button:not([disabled]), select, textarea, [tabindex]:not([tabindex="-1"])',
        ),
      ]
      if (focusable.length === 0) return
      const first = focusable[0]
      const last = focusable[focusable.length - 1]
      if (!e.shiftKey && document.activeElement === last) {
        e.preventDefault()
        first.focus()
      } else if (e.shiftKey && document.activeElement === first) {
        e.preventDefault()
        last.focus()
      }
    }
    document.addEventListener('keydown', onKey, true)
    return () => document.removeEventListener('keydown', onKey, true)
  }, [onClose])

  return (
    <div className="fixed inset-0 z-40 flex items-start justify-center bg-scrim pt-[12vh]">
      <div
        ref={panel}
        role="dialog"
        aria-modal="true"
        aria-label={title}
        tabIndex={-1}
        className={`animate-in flex max-h-[76vh] flex-col overflow-hidden rounded-md border border-line bg-surface-3 outline-none ${width}`}
      >
        <div className="flex items-baseline justify-between gap-4 border-b border-line px-4 py-[10px]">
          <h2 className="text-md font-medium text-text-1">{title}</h2>
          {description && <p className="text-sm text-text-3">{description}</p>}
        </div>
        <dialogCtx.Provider value={{ close: onClose }}>
          <div data-dialog-body className="scroll-fade min-h-0 flex-1 overflow-y-auto">
            {children}
          </div>
          {footer && (
            <div className="shrink-0 border-t border-line bg-surface-3 px-4 py-3">{footer}</div>
          )}
        </dialogCtx.Provider>
      </div>
    </div>
  )
}

/** Empty is the shape every empty state takes: one line that says what is
 *  missing, one that says how to fix it. No icon, no card. */
export function Empty({
  title,
  action,
  keys,
}: {
  title: string
  action?: string
  keys?: string
}) {
  return (
    <div className="flex h-full flex-col items-center justify-center gap-2 px-6 text-center">
      <p className="text-md text-text-2">{title}</p>
      {action && (
        <p className="flex items-center gap-[6px] text-base text-text-3">
          {action}
          {keys && <Keys keys={keys} />}
        </p>
      )}
    </div>
  )
}

/** PaneHeader is the 40px strip at the top of each of the three panes. All
 *  three share the height so their bottom borders form one line. */
export function PaneHeader({ children }: { children: ReactNode }) {
  return (
    <div className="flex h-[40px] shrink-0 items-center gap-2 border-b border-line px-3">
      {children}
    </div>
  )
}
