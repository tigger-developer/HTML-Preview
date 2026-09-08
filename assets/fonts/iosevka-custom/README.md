# Iosevka Custom fonts

Iosevka Custom is the selected default for code blocks, inline code,
preformatted text, and other fixed-width presentation in htmlpreview.
The four standard-width WOFF2 faces were copied byte-for-byte from the
operator's font archive on 8 September 2026. No font was rebuilt, converted,
subsetted, renamed, or otherwise modified for this import.

## Faces and integrity

All four fonts identify family **Iosevka Custom**, version **34.1.0**. They are
static faces: their OpenType tables contain no variation axes. The OS/2 width
class is 5 (normal), and the `post` table marks each as fixed pitch.

| File | CSS style | CSS weight | Bytes | SHA-256 |
| --- | --- | --- | --- | --- |
| `IosevkaCustom-Regular.woff2` | normal | 400 | 336416 | `974f9d6cf94f8c8c55a279ffd7cbe5aab00c9df63bdeffe7c2450b8820cf756e` |
| `IosevkaCustom-Italic.woff2` | italic | 400 | 384720 | `ef85723a68f371f853604a8580de306dffc6c6c8d46822a2e1d8f9e474ed7b40` |
| `IosevkaCustom-Bold.woff2` | normal | 700 | 336244 | `9ab02901db66ac603ca9f08cc99a33199a632f176cdfb76d0cdc8bbb4f58d87b` |
| `IosevkaCustom-BoldItalic.woff2` | italic | 700 | 385208 | `226bd10650c64a3cd576d358391a4eb6522948a96b169b89b1deb6353c838134` |

These four faces cover ordinary and emphasized fixed-width text. Condensed,
extended, extra-light, and heavy variants are not part of this selected set.
The four files total 1,442,588 bytes; their base64 payloads total 1,923,456
bytes per generated page, before CSS and notices. Existing session-output
limits still apply and can limit the number of generated pages.

## Licence and provenance

Each font's embedded metadata identifies SIL Open Font License 1.1 and carries:

> Copyright 2015-2026, Renzhi Li (aka. Belleve Invis, belleve@typeof.net).

[OFL.md](OFL.md) retains the full licence and upstream copyright notice from
the [Iosevka licence](https://github.com/be5invis/Iosevka/blob/main/LICENSE.md),
retrieved on 8 September 2026. Only trailing whitespace was normalized.
The holder and years match all four imported fonts. The retained licence
specifies no Reserved Font Name after its copyright statement.

The fonts remain under OFL 1.1, separately from the project's Apache 2.0
licence. Bundling and embedding must retain the copyright notice and full OFL
text, including in readable generated HTML source. See the retained licence
for all conditions.

## Rendering contract and inspection

The renderer embeds these exact WOFF2 bytes in the executable and as data
URLs in generated CSS. Use four `@font-face` declarations with the style and
weight above, normal stretch, and an `Iosevka Custom` family followed by a
generic `monospace` fallback. Use packaged data rather than a `local()` source
or network font download. Asap remains the proportional default.

A bounded inspection decompressed temporary copies for OpenType metadata and
HarfBuzz shaping. All faces shaped `Taḋg ÁÉÍÓÚ áéíóú 0O1Il WMi -> != ===`
without missing glyphs; sampled letters, digits, and spaces had equal advances.
The imported WOFF2 files remain unchanged. This is asset evidence, not browser
or application validation. Renderer integration now has regression checks for
payload hashes and declarations; actual browser face selection remains pending.
