# Asap variable fonts

The regular and italic WOFF2 files in this directory are the selected default
fonts for htmlpreview. They were copied byte-for-byte from the operator's font
archive on 7 September 2026. This import performed no conversion, subsetting,
or modification of the font files.

## Verified metadata

Both files identify Asap version 3.002 and carry this copyright notice:

> Copyright 2018 The Asap Project Authors (https://github.com/Omnibus-Type/Asap)

Their embedded metadata identifies SIL Open Font License 1.1. Inspection of
their OpenType variation tables established these ranges for both faces:

| Axis | Minimum | Default | Maximum |
| --- | --- | --- | --- |
| Weight (`wght`) | 100 | 400 | 900 |
| Width (`wdth`) | 75 | 100 | 125 |

The width values correspond to percentage stretch values. Both faces should be
declared with their full weight and stretch ranges; use the italic file for
italic text. Code and preformatted content use the separately bundled
[Iosevka Custom faces](../iosevka-custom/README.md).

A one-off HarfBuzz shaping check of temporary decompressed copies produced no
missing glyphs for `Taḋg ÁÉÍÓÚ áéíóú` in either face. A direct character-map check
alone was insufficient: the precomposed dotted d has no entry, while the base
letter and combining dot are available for shaping. This does not replace a
browser typography check.

## Files and integrity

| File | Size in bytes | SHA-256 |
| --- | --- | --- |
| `Asap-VariableFont_wdth,wght.woff2` | 141080 | `2a03eab9fff645ef7ffcb6f9c81065a7cbd2d76d87873a8cc5bf4f23d97fdf7a` |
| `Asap-Italic-VariableFont_wdth,wght.woff2` | 166692 | `346d8f2e0c37b3cb777dbe9b759a5b042b1b459db28c0f285760678241ef3f17` |

## Licence and distribution

[OFL.txt](OFL.txt) contains the Asap copyright notice and full SIL Open Font
License 1.1. It was retrieved from the
[Google Fonts Asap distribution](https://github.com/google/fonts/blob/main/ofl/asap/OFL.txt)
on 7 September 2026; only trailing whitespace was normalized. The copyright
matches the embedded notices in both imported fonts.

The licence permits bundling and embedding with software, with the copyright
notice and licence retained. The fonts remain under OFL 1.1 and are excluded
from the project's Apache 2.0 licence. The licence does not impose OFL on the
documents displayed with these fonts. Consult the retained licence for its
full terms, including modification and standalone-sale conditions.

The intended build embeds both files and their notice in the executable. The
renderer should include the font bytes as WOFF2 data URLs in its CSS and the
full copyright/licence text in readable generated HTML source. Release packages
must also carry the notice and licence. No runtime font download is required
by this design; renderer integration remains to be implemented.
