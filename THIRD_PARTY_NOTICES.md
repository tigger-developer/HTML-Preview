---
title: Licensing and third-party notices
version: 1
last-updated: 2026-09-20
---

# Licensing and third-party notices

htmlpreview's project-owned code and documentation are licensed under the
[Apache License, Version 2.0](LICENSE). This includes the project-owned Org
fidelity prototype and any adapted implementation.

## Asap fonts

The regular and italic variable WOFF2 fonts in `assets/fonts/asap/` are licensed
separately under the [SIL Open Font License 1.1](assets/fonts/asap/OFL.txt).
They are not relicensed under Apache 2.0.

> Copyright 2018 The Asap Project Authors (https://github.com/Omnibus-Type/Asap)

The full font licence and copyright notice accompany the files. Their version,
variation axes, import provenance, and checksums are recorded in the
[font asset documentation](assets/fonts/asap/README.md).

Distributions that include or embed these fonts must retain their copyright
notice and licence. The rendering design also carries them in generated HTML
when font data is embedded. Other dependencies added during implementation must
retain their own applicable licences and notices.

## Iosevka Custom fonts

The regular, italic, bold, and bold italic WOFF2 fonts in
`assets/fonts/iosevka-custom/` are licensed separately under the
[SIL Open Font License 1.1](assets/fonts/iosevka-custom/OFL.md).
They remain under that licence and are excluded from the project's Apache 2.0
licence.

> Copyright 2015-2026, Renzhi Li (aka. Belleve Invis, belleve@typeof.net).

The full upstream copyright notice and licence accompany the fonts. Their
version, selected faces, import provenance, and checksums are recorded in the
[Iosevka Custom asset documentation](assets/fonts/iosevka-custom/README.md).
Packages and generated HTML embedding these fonts must retain their copyright
notice and full licence, just as for the separately licensed Asap assets.

## Go dependencies

The reviewed dependency versions are pinned in `go.mod` and `go.sum`.
Their full upstream licence files are retained byte-for-byte under
`assets/licenses`, copied from the checksum-verified Go module downloads.
They accompany prefix installations and release archives. Font licences also accompany embedded font data in generated HTML.

| Module | Version | Purpose | Licence |
| --- | --- | --- | --- |
| `github.com/niklasfasching/go-org` | 1.9.1, local scanner and list corrections | Native Org parsing and HTML writer | [MIT](assets/licenses/go-org-LICENSE) |
| `github.com/alecthomas/chroma/v2` | 2.27.0 | Native Org source highlighting | [MIT](assets/licenses/chroma-COPYING) |
| `github.com/dlclark/regexp2/v2` | 2.2.1 | Chroma lexer expressions | [MIT](assets/licenses/regexp2-LICENSE) |
| `github.com/fsnotify/fsnotify` | 1.10.1 | Native filesystem change notifications | [BSD 3-Clause](assets/licenses/fsnotify-LICENSE) |
| `golang.org/x/sys` | 0.47.0 | Filesystem notification OS bindings | [BSD 3-Clause](assets/licenses/golang-x-sys-LICENSE) |
| `golang.org/x/net` | 0.58.0 | HTML5 parsing | [BSD 3-Clause](assets/licenses/golang-x-net-LICENSE) |
| `github.com/microcosm-cc/bluemonday` | 1.0.27 | Passive HTML policy engine | [BSD 3-Clause](assets/licenses/bluemonday-LICENSE.md) |
| `github.com/aymerick/douceur` | 0.2.0 | Transitive sanitizer dependency | [MIT](assets/licenses/douceur-LICENSE) |
| `github.com/gorilla/css` | 1.0.1 | Transitive sanitizer dependency | [BSD 3-Clause](assets/licenses/gorilla-css-LICENSE) |
| `github.com/tdewolff/parse/v2` | 2.8.16 | Native HTML CSS grammar and tokens | [MIT](assets/licenses/tdewolff-parse-LICENSE.md) |
| `go.yaml.in/yaml/v3` | 3.0.5 | Strict service configuration parsing | [MIT and Apache 2.0](assets/licenses/go-yaml-LICENSE) |

Go 1.26.8 is the reviewed delivery toolchain. Pandoc is separately installed
and is not bundled or relicensed by this project.

The go-org copy and its scanner and list corrections are documented in
[the local module provenance](third_party/go-org/README.md). Chroma's upstream
notice also contains the SIL font licence for its SVG formatter; that formatter
and its font are not linked into htmlpreview.

## Document changes

- Version 1 metadata: document the native Org converter and its dependency provenance where applicable.
