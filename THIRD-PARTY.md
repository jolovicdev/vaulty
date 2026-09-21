# Third-party components

Everything Vaulty bundles or links against, and the terms it comes under.
Nothing here is fetched at runtime; the fonts and the word list are compiled
into the binary.

## Go modules

| Component | Licence | Used for |
|---|---|---|
| [gokeepasslib](https://github.com/tobischo/gokeepasslib) | MIT | Reading and writing the KDBX format |
| [wails](https://github.com/wailsapp/wails) | MIT | The desktop window and the Go to JavaScript bridge |
| [golang.org/x/sys](https://pkg.go.dev/golang.org/x/sys) | BSD-3-Clause | The Win32 clipboard calls |

`go.mod` lists these three as direct requirements. Their own dependencies are
recorded there as indirect.

## Bundled assets

| Component | Licence | Location |
|---|---|---|
| [Inter](https://github.com/rsms/inter) | SIL Open Font License 1.1 | `frontend/public/fonts/Inter-LICENSE.txt` |
| [JetBrains Mono](https://github.com/JetBrains/JetBrainsMono) | SIL Open Font License 1.1 | `frontend/public/fonts/JetBrainsMono-OFL.txt` |
| [EFF long wordlist](https://www.eff.org/dice) | CC BY 3.0 US | `internal/generator/wordlist-LICENSE.txt` |

The word list is the 7776-entry long list published by the Electronic
Frontier Foundation, stored one word per line with the original dice numbers
removed. Its size is what makes each word worth 12.925 bits, so it is
reproduced verbatim apart from that.

## Frontend packages

React, Vite and Tailwind CSS are MIT, lucide-react is ISC and TypeScript is
Apache-2.0. See `frontend/package.json` for versions and
`frontend/package-lock.json` for the resolved tree.
