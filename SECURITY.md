# Reporting a security problem

Email **dusan.jolovic@proton.me**. Please do not open a public issue for
anything that could put someone's vault at risk.

Include what you need to make the problem reproducible: the platform, the
Vaulty version or commit, and the steps. If a vault file is involved, create
a throwaway one rather than sending your own.

You should get a reply within a week. If a fix is warranted I will credit you
in the release notes unless you prefer otherwise.

## What is in scope

The Security section of the README is the reference for what Vaulty claims to
defend against. A report is in scope if it breaks one of those claims. In
particular:

- A secret reaching the entry list, a log, an error message shown to the
  user, or anywhere else outside an explicit reveal or copy.
- A clipboard copy that is not cleared, or that lands in Windows Clipboard
  History or the cloud clipboard.
- A vault that Vaulty corrupts, truncates, or overwrites after another
  program changed the file.
- A lock that does not drop the database and its key material.
- Any network request. Vaulty makes none; one would be a bug in itself.
- A KDBX file Vaulty writes that KeePassXC cannot open, or the reverse,
  where the difference could cost someone access to their entries.

## What is out of scope

Some things are outside what a desktop password manager can defend against,
and they are not vulnerabilities in this one:

- Malware or any process already running as the user. It can read this
  program's memory and log its keystrokes, and no password manager on a
  general purpose operating system prevents that.
- Recovering secrets from a core dump, a swap file or a memory image. Go is
  garbage collected, so a value cannot be reliably erased after use. Vaulty
  keeps secrets out of long-lived structures and drops the database on lock;
  it does not claim more than that.
- A weak master password. Argon2d makes guessing expensive, not impossible.
- A machine that was already compromised when the master password was typed.
- A modified Vaulty binary. It does not verify itself.

## Cryptography

Vaulty implements none. Key derivation, encryption and authentication come
from the KDBX format via gokeepasslib, and the primitives from the Go
standard library. A flaw in either belongs upstream, though I would still
like to hear about it so this project can pin or patch around it.
