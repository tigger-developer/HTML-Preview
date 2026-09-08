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
They accompany prefix installations, release archives, and embedded HTML.

| Module | Version | Purpose | Licence |
| --- | --- | --- | --- |
| `golang.org/x/net` | 0.58.0 | HTML5 parsing | [BSD 3-Clause](assets/licenses/golang-x-net-LICENSE) |
| `github.com/microcosm-cc/bluemonday` | 1.0.27 | Passive HTML policy engine | [BSD 3-Clause](assets/licenses/bluemonday-LICENSE.md) |
| `github.com/aymerick/douceur` | 0.2.0 | Transitive sanitizer dependency | [MIT](assets/licenses/douceur-LICENSE) |
| `github.com/gorilla/css` | 1.0.1 | Transitive sanitizer dependency | [BSD 3-Clause](assets/licenses/gorilla-css-LICENSE) |

Go 1.26.8 is the reviewed delivery toolchain. Pandoc is separately installed
and is not bundled or relicensed by this project.
