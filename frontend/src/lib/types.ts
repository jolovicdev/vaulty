// These mirror the structs app.go returns. Nothing here has a field for a
// password, a note or a protected custom value, which is what keeps a secret
// out of the frontend unless it was explicitly revealed.

export interface Meta {
  id: string
  title: string
  username: string
  url: string
  tags: string[]
  groupId: string
  groupPath: string
  modified: string
  created: string
  hasTotp: boolean
  hasNotes: boolean
  expired: boolean
  inRecycleBin: boolean
  score: number
}

export interface CustomField {
  key: string
  protected: boolean
  /** Empty for a protected field; read it with reveal instead. */
  value: string
}

export interface Detail extends Meta {
  custom: CustomField[]
  historyCount: number
  passwordSet: boolean
}

export interface Draft {
  title: string
  username: string
  password: string
  url: string
  notes: string
  tags: string[]
  totpSeed: string
  custom: Record<string, string>
  protectedCustom: string[]
  groupId: string
  /** Takes the one-time code off the entry. An empty totpSeed cannot ask
   *  for that, because it means unchanged. */
  removeTotp: boolean
}

export interface GroupNode {
  id: string
  name: string
  path: string
  count: number
  isRecycleBin: boolean
  children: GroupNode[]
}

export interface HistoryVersion {
  index: number
  title: string
  username: string
  url: string
  modified: string
}

export interface ListOptions {
  groupId?: string
  tag?: string
  includeRecycleBin?: boolean
}

export interface Status {
  unlocked: boolean
  path: string
  name: string
  formatVersion: string
  dirty: boolean
  clipboardWorks: boolean
}

export interface Recent {
  path: string
  keyFile?: string
  lastUsed: string
}

export type Theme = 'system' | 'dark' | 'light'

export interface Settings {
  theme: Theme
  idleLockSeconds: number
  clipboardSeconds: number
  showPasswordStrength: boolean
  recent: Recent[]
}

export interface CopyResult {
  field: string
  expiresAt: number
  seconds: number
}

export interface TotpCode {
  code: string
  digits: number
  period: number
  remaining: number
  issuer: string
  account: string
}

export interface StrengthResult {
  score: number
  entropy: number
  guesses: number
  label: string
  warning: string
  /** True when nothing in the value looks like a human choice. A score is
   *  advice, and there is none to give about a key you were issued. */
  machine: boolean
  /** The service whose key format this matches, when it matches one. */
  issuer: string
}

export interface PasswordOptions {
  length: number
  lower: boolean
  upper: boolean
  digits: boolean
  symbols: boolean
  excludeAmbiguous: boolean
}

export interface PassphraseOptions {
  words: number
  separator: string
  capitalize: boolean
  addNumber: boolean
}

export interface DiskState {
  changed: boolean
  missing: boolean
  modTime: string
}

export interface MergeResult {
  added: number
  updated: number
  skipped: number
  deleted: number
}

export type ConflictChoice = 'reload' | 'copy' | 'merge'

export interface ConflictResult {
  choice: ConflictChoice
  path: string
  merge: MergeResult
}

/** Field names as KeePass stores them, so a reveal names the right key. */
export const FIELD = {
  title: 'Title',
  username: 'UserName',
  password: 'Password',
  url: 'URL',
  notes: 'Notes',
} as const
