import { Database, FolderOpen, Plus } from 'lucide-react'
import { createElement } from 'react'
import { MENU_SEPARATOR, type MenuEntry } from './Menu'
import type { Recent } from '../lib/types'

function shortPath(path: string): string {
  const parts = path.split(/[/\\]/)
  const file = parts.pop() ?? path
  const parent = parts.pop()
  return parent ? `${parent}/${file}` : file
}

function vaultName(path: string): string {
  return (path.split(/[/\\]/).pop() ?? path).replace(/\.kdbx$/i, '')
}

/** The vault menu, hung off the name in the footer. Opening and creating a
 *  vault belong here rather than only on the unlock screen, so reaching them
 *  does not require locking the current vault first. */
export function vaultMenu(
  recents: Recent[],
  currentPath: string,
  h: { open: (path: string) => void; browse: () => void; create: () => void },
): MenuEntry[] {
  const others = recents.filter((r) => r.path !== currentPath).slice(0, 6)

  return [
    ...others.map((r) => ({
      id: r.path,
      label: vaultName(r.path),
      hint: shortPath(r.path),
      icon: createElement(Database, { size: 13 }),
      run: () => h.open(r.path),
    })),
    ...(others.length > 0 ? [MENU_SEPARATOR] : []),
    {
      id: 'browse',
      label: 'Open a vault',
      keys: 'Ctrl O',
      icon: createElement(FolderOpen, { size: 13 }),
      run: h.browse,
    },
    {
      id: 'create',
      label: 'Create a vault',
      icon: createElement(Plus, { size: 13 }),
      run: h.create,
    },
  ]
}
