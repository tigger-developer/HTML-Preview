---
title: Installation and prerequisites
version: 1
last-updated: 2026-10-01
---

# Installation and prerequisites

Org, Markdown, code/plaintext and passive HTML need no external converter.
Other readers require optional **Pandoc 3.9.0.2 through the 3.9 patch series**,
including its bundled Lua 5.4. Compatibility and reader discovery run only when
an optional reader or the full installed-reader listing is requested. Installation
and native service startup work without Pandoc; installation never downloads it.

| Platform | Default-browser handoff |
| --- | --- |
| macOS | `/usr/bin/open` and the default local HTML association |
| Linux desktop | `xdg-open` from xdg-utils and an active desktop session |
| WSL1 or WSL2 | Existing `wslpath`, built-in Windows `powershell.exe`, enabled interoperation, and Windows access to the distribution |

WSL opens the Windows default browser, including when WSLg is present. It needs
private Linux temporary storage; Windows-mounted temporary output is rejected.
Unrepresentable original paths become inactive references. No browser
association, font cache, file permission, shell profile, or WSL setting is
changed. Native Windows executables are outside the supported build targets.

Building requires Go **1.26.8**. Normal binary use needs neither Go nor a
standalone Lua or JavaScript runtime.

```sh
make build
```

```sh
make install
```

The default installation creates `~/.local/bin/htmlpreview` as an absolute
symlink to this checkout's `bin/htmlpreview`. Keep the checkout at a stable
location; rebuilding the binary updates the linked command. Reinstalling retains
a matching link and atomically replaces a different or dangling symlink, so
installation can switch between checkouts. The previous link target is untouched.
A regular file or directory at the destination is preserved and reported; move
that conflicting entry aside before retrying.

Put `~/.local/bin` on PATH. Check command shadowing with
`command -v htmlpreview`, particularly if you already have a personal script.
An executable symlink works without neighbouring assets: fonts, templates,
CSS, browser JavaScript, and Lua are embedded in the binary. The default link
uses the checkout's licence notices and does not register system fonts.

For a copied installation independent of the checkout, supply a non-empty,
absolute prefix explicitly:

```sh
make install PREFIX="$HOME/.local"
```

This copies the binary and notices into the prefix; repeated prefix installation
replaces its managed binary and licence files. Put that prefix's `bin` on PATH.
When switching from a linked installation, move the existing link aside first;
copy installation refuses symlink destinations.
`DESTDIR` stages either mode beneath an absolute temporary root. A staged
default symlink still points to the checkout; use explicit prefix installation
for packaging. Neither mode invokes sudo or downloads Pandoc. The former
`/usr/local` default is superseded by the user-local symlink; an earlier
installation there is not removed automatically.
