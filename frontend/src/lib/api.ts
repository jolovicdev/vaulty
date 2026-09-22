// The only module that touches the Wails bridge. Everything else imports
// from here, so the bridge can be swapped out at this one seam.

import type {
  ConflictChoice,
  ConflictResult,
  CopyResult,
  Detail,
  DiskState,
  Draft,
  GroupNode,
  HistoryVersion,
  ImportPreview,
  ListOptions,
  Meta,
  PassphraseOptions,
  PasswordOptions,
  Settings,
  Status,
  StrengthResult,
  TotpCode,
} from './types'

type Bound = (...args: unknown[]) => Promise<unknown>

interface WailsWindow {
  go?: { main?: { App?: Record<string, Bound> } }
  runtime?: {
    EventsOn: (event: string, handler: (...data: unknown[]) => void) => () => void
    Quit: () => void
    WindowMinimise: () => void
  }
}

function bridge(): Record<string, Bound> {
  const bound = (window as unknown as WailsWindow).go?.main?.App
  if (!bound) {
    throw new Error('the Go bridge is not available')
  }
  return bound
}

async function call<T>(method: string, ...args: unknown[]): Promise<T> {
  const fn = bridge()[method]
  if (typeof fn !== 'function') {
    throw new Error(`binding ${method} is missing`)
  }
  return (await fn(...args)) as T
}

/** Subscribes to a Go-side event and returns the unsubscribe function. */
export function on(event: string, handler: () => void): () => void {
  const rt = (window as unknown as WailsWindow).runtime
  if (!rt) return () => {}
  return rt.EventsOn(event, handler)
}

export const EVENT_LOCKED = 'vault:locked'
export const EVENT_CLOSE_REQUESTED = 'vault:close-requested'
export const EVENT_CONFLICT = 'vault:conflict'

export const api = {
  status: () => call<Status>('Status'),
  unlock: (path: string, password: string, keyFile: string) =>
    call<Status>('Unlock', path, password, keyFile),
  createVault: (path: string, password: string, keyFile: string) =>
    call<Status>('CreateVault', path, password, keyFile),
  lock: () => call<void>('Lock'),
  touch: () => call<void>('Touch'),

  pickVaultFile: () => call<string>('PickVaultFile'),
  pickNewVaultPath: () => call<string>('PickNewVaultPath'),
  pickKeyFile: () => call<string>('PickKeyFile'),
  confirm: (title: string, message: string) => call<boolean>('Confirm', title, message),
  confirmQuit: (save: boolean) => call<void>('ConfirmQuit', save),

  loadSettings: () => call<Settings>('LoadSettings'),
  saveSettings: (s: Settings) => call<void>('SaveSettings', s),
  settingsPath: () => call<string>('SettingsPath'),
  forgetRecent: (path: string) => call<void>('ForgetRecent', path),

  list: (opts: ListOptions) => call<Meta[]>('List', opts),
  search: (query: string, opts: ListOptions) => call<Meta[]>('Search', query, opts),
  groups: () => call<GroupNode[]>('Groups'),
  tags: () => call<string[]>('Tags'),
  detail: (id: string) => call<Detail>('Detail', id),
  reveal: (id: string, field: string) => call<string>('Reveal', id, field),

  history: (id: string) => call<HistoryVersion[]>('History', id),
  revealHistory: (id: string, index: number, field: string) =>
    call<string>('RevealHistory', id, index, field),
  restoreVersion: (id: string, index: number) => call<void>('RestoreVersion', id, index),

  addEntry: (d: Draft) => call<string>('AddEntry', d),
  updateEntry: (id: string, d: Draft) => call<void>('UpdateEntry', id, d),
  deleteEntry: (id: string) => call<void>('DeleteEntry', id),
  restoreEntry: (id: string, groupId: string) => call<void>('RestoreEntry', id, groupId),
  emptyRecycleBin: () => call<void>('EmptyRecycleBin'),
  moveEntry: (id: string, groupId: string) => call<void>('MoveEntry', id, groupId),
  addGroup: (parentId: string, name: string) => call<string>('AddGroup', parentId, name),

  save: () => call<void>('Save'),
  diskState: () => call<DiskState>('DiskState'),
  resolveConflict: (choice: ConflictChoice) => call<ConflictResult>('ResolveConflict', choice),
  backups: () => call<string[]>('Backups'),

  pickImportFile: () => call<string>('PickImportFile'),
  previewImport: (path: string) => call<ImportPreview>('PreviewImport', path),
  importFile: (path: string) => call<void>('ImportFile', path),

  copyField: (id: string, field: string) => call<CopyResult>('CopyField', id, field),
  copyTotp: (id: string) => call<CopyResult>('CopyTOTP', id),
  copyGenerated: (value: string) => call<CopyResult>('CopyGenerated', value),
  clearClipboard: () => call<void>('ClearClipboard'),

  totp: (id: string) => call<TotpCode>('TOTP', id),
  openUrl: (id: string) => call<void>('OpenURL', id),

  generatePassword: (o: PasswordOptions) => call<string>('GeneratePassword', o),
  generatePassphrase: (o: PassphraseOptions) => call<string>('GeneratePassphrase', o),
  defaultGeneratorOptions: () =>
    call<{ password: PasswordOptions; passphrase: PassphraseOptions }>(
      'DefaultGeneratorOptions',
    ),
  estimateStrength: (password: string) => call<StrengthResult>('EstimateStrength', password),
}
