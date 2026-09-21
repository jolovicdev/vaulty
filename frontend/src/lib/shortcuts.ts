// One registry for every keyboard action. The handler and the tooltip text
// come from the same row, so what a tooltip promises and what the key does
// cannot drift apart.

export interface Shortcut {
  id: string
  /** Human label for the tooltip and the palette. */
  label: string
  /** Key combination, written the way it is shown to the user. */
  keys: string
  /** Matches a keydown event. */
  match: (e: KeyboardEvent) => boolean
}

const mod = (e: KeyboardEvent) => e.ctrlKey || e.metaKey

function plain(key: string) {
  return (e: KeyboardEvent) =>
    e.key.toLowerCase() === key && !mod(e) && !e.altKey && !e.shiftKey
}

function withMod(key: string) {
  return (e: KeyboardEvent) => e.key.toLowerCase() === key && mod(e) && !e.altKey
}

function withModShift(key: string) {
  return (e: KeyboardEvent) => e.key.toLowerCase() === key && mod(e) && e.shiftKey
}

export const SHORTCUTS: Record<string, Shortcut> = {
  palette: { id: 'palette', label: 'Command palette', keys: 'Ctrl K', match: withMod('k') },
  search: { id: 'search', label: 'Search entries', keys: 'Ctrl F', match: withMod('f') },
  newEntry: { id: 'newEntry', label: 'New entry', keys: 'Ctrl N', match: withMod('n') },
  openVault: { id: 'openVault', label: 'Open a vault', keys: 'Ctrl O', match: withMod('o') },
  save: { id: 'save', label: 'Save vault', keys: 'Ctrl S', match: withMod('s') },
  lock: { id: 'lock', label: 'Lock vault', keys: 'Ctrl L', match: withMod('l') },
  generator: { id: 'generator', label: 'Password generator', keys: 'Ctrl G', match: withMod('g') },
  settings: { id: 'settings', label: 'Settings', keys: 'Ctrl ,', match: withMod(',') },
  copyPassword: {
    id: 'copyPassword',
    label: 'Copy password',
    keys: 'Ctrl C',
    match: withMod('c'),
  },
  copyUsername: {
    id: 'copyUsername',
    label: 'Copy username',
    keys: 'Ctrl B',
    match: withMod('b'),
  },
  copyTotp: { id: 'copyTotp', label: 'Copy one-time code', keys: 'Ctrl T', match: withMod('t') },
  openUrl: { id: 'openUrl', label: 'Open URL', keys: 'Ctrl U', match: withMod('u') },
  edit: { id: 'edit', label: 'Edit entry', keys: 'Ctrl E', match: withMod('e') },
  reveal: { id: 'reveal', label: 'Reveal password', keys: 'Ctrl R', match: withMod('r') },
  del: { id: 'del', label: 'Delete entry', keys: 'Ctrl Del', match: withMod('delete') },
  history: { id: 'history', label: 'Entry history', keys: 'Ctrl H', match: withMod('h') },
  help: { id: 'help', label: 'Keyboard shortcuts', keys: 'Ctrl Shift /', match: withModShift('?') },
  down: { id: 'down', label: 'Next entry', keys: 'Down', match: plain('arrowdown') },
  up: { id: 'up', label: 'Previous entry', keys: 'Up', match: plain('arrowup') },
  escape: { id: 'escape', label: 'Close', keys: 'Esc', match: plain('escape') },
}

/** Splits a shortcut's keys for rendering one Kbd per key. A command
 *  without a shortcut yields nothing rather than one blank cap. */
export function keyParts(keys: string): string[] {
  return keys.split(' ').filter(Boolean)
}

/** Finds the shortcut an event matches, or undefined. */
export function resolve(e: KeyboardEvent, ids: string[]): Shortcut | undefined {
  for (const id of ids) {
    const s = SHORTCUTS[id]
    if (s?.match(e)) return s
  }
  return undefined
}

/** True when the event came from a field where typing must win over a
 *  single-key shortcut. */
export function isTypingTarget(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false
  const tag = target.tagName
  return tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT' || target.isContentEditable
}
