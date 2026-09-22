// The item lists every menu is built from. The palette, a row's context
// menu, its hover button and the footer button all read from here, so what
// "Copy password" means cannot drift between them.

import {
  Check,
  Database,
  Copy,
  ExternalLink,
  FolderOpen,
  FolderPlus,
  History as HistoryIcon,
  Import,
  KeyRound,
  Lock,
  Pencil,
  Plus,
  RotateCcw,
  Settings as SettingsIcon,
  Timer,
  Trash2,
  User,
  Wand2,
} from 'lucide-react'
import { createElement } from 'react'
import type { LucideIcon } from 'lucide-react'
import { MENU_SEPARATOR, type MenuEntry } from '../components/Menu'
import { SHORTCUTS } from './shortcuts'
import { FIELD, type Detail, type GroupNode, type Meta } from './types'

const icon = (c: LucideIcon) => createElement(c, { size: 13 })

export const IMPORT_LABEL = 'Import from Proton Pass, Bitwarden or 1Password'

/** Everything that can be done to one entry. */
export interface EntryHandlers {
  copyPassword: (id: string) => void
  copyUsername: (id: string) => void
  copyTotp: (id: string) => void
  openUrl: (id: string) => void
  edit: (id: string) => void
  move: (id: string) => void
  history: (id: string) => void
  del: (id: string) => void
  restore: (id: string) => void
}

/** entryMenu works from a Meta or a Detail, so a list row and the detail
 *  masthead offer the same actions. */
export function entryMenu(entry: Meta | Detail, h: EntryHandlers): MenuEntry[] {
  const historyCount = 'historyCount' in entry ? entry.historyCount : 0
  const out: MenuEntry[] = [
    {
      id: 'copyPassword',
      label: SHORTCUTS.copyPassword.label,
      keys: SHORTCUTS.copyPassword.keys,
      icon: icon(Copy),
      run: () => h.copyPassword(entry.id),
    },
  ]

  if (entry.username) {
    out.push({
      id: 'copyUsername',
      label: SHORTCUTS.copyUsername.label,
      keys: SHORTCUTS.copyUsername.keys,
      icon: icon(User),
      run: () => h.copyUsername(entry.id),
    })
  }
  if (entry.hasTotp) {
    out.push({
      id: 'copyTotp',
      label: SHORTCUTS.copyTotp.label,
      keys: SHORTCUTS.copyTotp.keys,
      icon: icon(Timer),
      run: () => h.copyTotp(entry.id),
    })
  }
  if (entry.url) {
    out.push({
      id: 'openUrl',
      label: SHORTCUTS.openUrl.label,
      keys: SHORTCUTS.openUrl.keys,
      icon: icon(ExternalLink),
      run: () => h.openUrl(entry.id),
    })
  }

  if (entry.inRecycleBin) {
    out.push(MENU_SEPARATOR, {
      id: 'restore',
      label: 'Restore from the recycle bin',
      icon: icon(RotateCcw),
      run: () => h.restore(entry.id),
    })
    out.push({
      id: 'delete',
      label: 'Delete permanently',
      icon: icon(Trash2),
      danger: true,
      run: () => h.del(entry.id),
    })
    return out
  }

  out.push(MENU_SEPARATOR, {
    id: 'edit',
    label: SHORTCUTS.edit.label,
    keys: SHORTCUTS.edit.keys,
    icon: icon(Pencil),
    run: () => h.edit(entry.id),
  })
  out.push({
    id: 'move',
    label: 'Move to a group',
    icon: icon(FolderPlus),
    run: () => h.move(entry.id),
  })
  if (historyCount > 0) {
    out.push({
      id: 'history',
      label: `${SHORTCUTS.history.label} (${historyCount})`,
      keys: SHORTCUTS.history.keys,
      icon: icon(HistoryIcon),
      run: () => h.history(entry.id),
    })
  }
  out.push(MENU_SEPARATOR, {
    id: 'delete',
    label: 'Move to the recycle bin',
    keys: SHORTCUTS.del.keys,
    icon: icon(Trash2),
    danger: true,
    run: () => h.del(entry.id),
  })
  return out
}

export interface GroupHandlers {
  newEntry: (groupId: string) => void
  newGroup: (parentId: string) => void
  emptyRecycleBin: () => void
}

export function groupMenu(group: GroupNode, h: GroupHandlers): MenuEntry[] {
  if (group.isRecycleBin) {
    return [
      {
        id: 'emptyRecycleBin',
        label: 'Empty the recycle bin',
        icon: icon(Trash2),
        danger: true,
        run: h.emptyRecycleBin,
      },
    ]
  }
  return [
    {
      id: 'newEntry',
      label: 'New entry here',
      keys: SHORTCUTS.newEntry.keys,
      icon: icon(Plus),
      run: () => h.newEntry(group.id),
    },
    {
      id: 'newGroup',
      label: 'New group inside',
      icon: icon(FolderPlus),
      run: () => h.newGroup(group.id),
    },
  ]
}

export interface AppHandlers {
  newEntry: () => void
  newGroup: () => void
  openVault: () => void
  createVault: () => void
  importFile: () => void
  generator: () => void
  save: () => void
  settings: () => void
  shortcuts: () => void
  lock: () => void
}

/** The app-level actions, for the footer button and the palette. Every one
 *  of these has no row to hang off. */
export function appMenu(h: AppHandlers, dirty: boolean): MenuEntry[] {
  return [
    {
      id: 'newEntry',
      label: SHORTCUTS.newEntry.label,
      keys: SHORTCUTS.newEntry.keys,
      icon: icon(Plus),
      run: h.newEntry,
    },
    { id: 'newGroup', label: 'New group', icon: icon(FolderPlus), run: h.newGroup },
    MENU_SEPARATOR,
    {
      id: 'openVault',
      label: SHORTCUTS.openVault.label,
      keys: SHORTCUTS.openVault.keys,
      icon: icon(FolderOpen),
      run: h.openVault,
    },
    { id: 'createVault', label: 'Create a vault', icon: icon(Database), run: h.createVault },
    { id: 'importFile', label: IMPORT_LABEL, icon: icon(Import), run: h.importFile },
    {
      id: 'generator',
      label: SHORTCUTS.generator.label,
      keys: SHORTCUTS.generator.keys,
      icon: icon(Wand2),
      run: h.generator,
    },
    MENU_SEPARATOR,
    ...(dirty
      ? [
          {
            id: 'save',
            label: SHORTCUTS.save.label,
            keys: SHORTCUTS.save.keys,
            icon: icon(Check),
            run: h.save,
          },
        ]
      : []),
    {
      id: 'settings',
      label: SHORTCUTS.settings.label,
      keys: SHORTCUTS.settings.keys,
      icon: icon(SettingsIcon),
      run: h.settings,
    },
    {
      id: 'shortcuts',
      label: SHORTCUTS.help.label,
      keys: SHORTCUTS.help.keys,
      icon: icon(KeyRound),
      run: h.shortcuts,
    },
    MENU_SEPARATOR,
    {
      id: 'lock',
      label: SHORTCUTS.lock.label,
      keys: SHORTCUTS.lock.keys,
      icon: icon(Lock),
      run: h.lock,
    },
  ]
}

/** The two actions a detail field row offers. */
export function fieldMenu(
  field: string,
  opts: { revealed: boolean; onCopy: () => void; onToggle: () => void },
): MenuEntry[] {
  return [
    { id: 'copy', label: `Copy ${field}`, icon: icon(Copy), run: opts.onCopy },
    {
      id: 'reveal',
      label: opts.revealed ? `Hide ${field}` : `Reveal ${field}`,
      keys: field === FIELD.password ? SHORTCUTS.reveal.keys : undefined,
      icon: icon(KeyRound),
      run: opts.onToggle,
    },
  ]
}
