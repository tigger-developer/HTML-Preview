---
title: Configuration reference
version: 1
last-updated: 2026-10-01
---

# Configuration reference

Empty values select defaults. Unknown `HTMLPREVIEW_` settings are errors.
Every supplied setting is validated, including those inactive in the selected
mode. The service uses a strict versioned YAML configuration for permitted roots;
there is no arbitrary Pandoc-argument interface. Use `--` before a filename
beginning with `-`.

An optional `--from FORMAT` or `--from=FORMAT` selects a native or installed
built-in reader for the explicit input batch. It never propagates to linked documents.
`--list-input-formats` lists available readers without opening a browser; without
Pandoc it lists `html`, `markdown` and `org`. Native Markdown supports `+smart`,
`-smart` and `-raw_html`; other Markdown qualifiers are errors. Embedded Markdown
HTML is omitted, with one CLI notice per document and a visible page warning.
Ordinary `.json` defaults to pretty-printed code; `--from=json` selects Pandoc's
JSON document AST. A suffix matching an installed reader also selects that reader;
otherwise an unmapped extension needs explicit selection. Native HTML
uses its separate passive policy and receives no application reading controls.

| Setting | Default | Accepted values |
| --- | --- | --- |
| `HTMLPREVIEW_TOC` | `1` | `0` or `1`; enable navigation for all documents |
| `HTMLPREVIEW_TOC_DEPTH` | `3` | Integer, 1 to 6; maximum source heading level |
| `HTMLPREVIEW_LINKS` | `0` | Deprecated; `0` or `1`, no graph effect |
| `HTMLPREVIEW_MODE` | `quick` | `quick` or `read`; file retention only |
| `HTMLPREVIEW_ROOT` | Each entry's canonical parent | Existing directory containing every explicit canonical source |
| `HTMLPREVIEW_GRACE` | `3s` | Go duration, `100ms` to `1h` |
| `HTMLPREVIEW_MAX_FILES` | `50` | Integer, 1 to 500 source contexts |
| `HTMLPREVIEW_MAX_DEPTH` | `3` | Deprecated; integer, 0 to 10, no graph effect |
| `HTMLPREVIEW_MAX_SOURCE_BYTES` | `10485760` | Integer, 1 to 10485760 |
| `HTMLPREVIEW_MAX_TOTAL_SOURCE_BYTES` | `52428800` | Integer, 1 to 52428800 |
| `HTMLPREVIEW_MAX_OUTPUT_BYTES` | `104857600` | Integer, 1 to 104857600; includes embedded fonts and staging |
| `HTMLPREVIEW_DEADLINE` | `60s` | Go duration, `100ms` to `10m` |
| `HTMLPREVIEW_CONFIG` | First existing `./config.yaml`, then `~/.config/htmlpreview/config.yaml` | Absolute version-1 YAML override; no merging |
| `HTMLPREVIEW_RUNTIME_DIR` | Platform user cache directory + `htmlpreview/runtime` | Absolute user-owned private directory |
| `HTMLPREVIEW_USER_DISPLAY_NAME` | Trimmed `USER` | Annotation label; at most 128 Unicode characters and 512 UTF-8 bytes, without controls |

With neither default config present, explicit `htmlpreview --serve` serves its
startup directory and descendants. An existing config with empty roots grants
nothing; invalid or unreadable configuration is an error. Service roots remain
fixed until restart.

HTTP requests additionally apply the service's 10 MiB source, 50 MiB output
and 60-second conversion ceilings. `HTMLPREVIEW_ROOT` restricts explicit inputs
before transport selection and can only narrow configured service roots.
The [service guide](SERVICE.md) describes exact paths, permissions, limits
and restart behaviour.

Published entry URLs go to stdout; diagnostics go to stderr. Exit status is 0
for success, 1 for an operational failure, and 2 for an invalid invocation.
Successful entries may still open when another input fails. Publication is
transactional: a publication failure opens no entry. Help and version requests
have no preview side effects and bypass environment validation.

Standalone output is always enabled. `HTMLPREVIEW_STANDALONE` is unsupported
and rejected as an unknown setting. Unset/empty TOC enables navigation for both
formats. Depth controls the displayed source levels. Application defaults
and explicit settings override source TOC metadata; depth is validated even
when the TOC is disabled. A document with no eligible
headings has no empty contents navigation.

