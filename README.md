---
title: HTML-Preview
version: 7
last-updated: 2026-10-01
---

# HTML-Preview

## What it is

HTML-Preview opens local documents in the system's default browser with a
readable layout, navigation, and the source document's structure intact. It
handles Org and Markdown directly, along with code, plain text, and passive
HTML. Other document formats can use an optional Pandoc installation.

## Why use it

Long specifications and design documents are easier to assess when their
headings, code, tables, footnotes, and metadata are easy to navigate. In a
service preview, a reviewer can place an annotation beside the relevant
passage; the feedback stays with the document as a native footnote, giving an
author or coding agent the context needed for a precise revision.

HTML-Preview began as a quick Markdown preview script for reviews in the
[Lean SDLC for Coding Agents](https://github.com/tigger-developer/sdlc). Org
support and browser annotations made it useful for more kinds of documents and
beyond that original project.

## Quickstart

If `htmlpreview` is already on `PATH`, open a document:

```sh
htmlpreview README.md
```

To build and install from this checkout, use Go 1.26.8 and run:

```sh
make install
```

```sh
htmlpreview README.md
```

The default installation links `~/.local/bin/htmlpreview` to this checkout's
built command. Keep the checkout in place and put `~/.local/bin` on `PATH`.
Installed binary use does not require Go. The [installation guide](docs/INSTALLATION.md)
covers copied installations, macOS, Linux, WSL, and browser handoff.

## Choose a reading mode

**Quick preview** opens each requested file in a private temporary session.
That session remains for three seconds after browser handoff. For a slow
browser start or a page you intend to reload, keep the session open:

```sh
HTMLPREVIEW_MODE=read htmlpreview README.md
```

The command stays in the foreground until Ctrl+C or SIGTERM, then removes its
own session directory. Neither reading mode changes the source.

**Linked reading and annotation** use the optional local service. From a
documentation directory, start it explicitly:

```sh
htmlpreview --serve
```

From another terminal, open an Org or Markdown file:

```sh
htmlpreview docs/VISION.md
```

The service renders linked documents on request inside its permitted roots.
When no configuration exists, its startup directory is the root. Installation
does not start it. Inputs outside its roots use a single-document file preview.
The [service guide](docs/SERVICE.md) explains configuration and lifetime.

## Reading and reviewing

Org and Markdown previews have a contents outline, folding controls, light
and dark appearance, source-path copying, and code copying. Org task states,
tags, planning, drawers, and frontmatter retain useful visual structure.
Source code uses local highlighting; the reader never executes it. Open the
[Org example](examples/work.org) or [Markdown example](examples/code.md) to
explore the controls. The [format guide](docs/FORMATS.md) explains what each
reader supports and where optional Pandoc applies. The
[reading guide](docs/READING.md) covers controls, preview lifetime, and linked
navigation.

The **Annotations** control appears on service previews of genuine Org and
Markdown sources. Select a supported paragraph, heading, list item, or block
and write feedback where a reviser will find its context. Notes autosave as
native footnotes. An unwritable source uses an adjacent Org sidecar, while
ordinary reading and file previews leave sources untouched. The
[annotation guide](docs/ANNOTATIONS.md) covers attribution, editing, source
changes, and recovery.

Local previews make source scripts and automatic remote resources passive.
The service uses loopback addresses and permitted filesystem roots. These
boundaries make it a local reading tool, not a general web publisher or an
execution sandbox. The [architecture](docs/ARCHITECTURE.md) records the exact
boundaries and design decisions.

## Find the detail

| Need | Read |
|---|---|
| Check formats and optional readers | [Supported formats](docs/FORMATS.md) |
| Use reader controls and linked navigation | [Reading and navigation](docs/READING.md) |
| Install or package a binary | [Installation](docs/INSTALLATION.md) |
| Configure limits and environment settings | [Configuration](docs/CONFIGURATION.md) |
| Set up linked browsing | [Local service](docs/SERVICE.md) |
| Review and revise annotations | [Annotations](docs/ANNOTATIONS.md) |
| Build and verify the project | [Development and packaging](docs/DEVELOPMENT.md) |
| Understand purpose and design | [Vision](docs/VISION.md) and [architecture](docs/ARCHITECTURE.md) |
| Check work and validation | [Work ledger](docs/work.org) and [Org](specs/011-conversion-performance/validation.org) and [Markdown](specs/014-native-markdown/validation.org) validation records |

The [previous detailed README](docs/archive/README-v6.md) is retained as a
historical snapshot. Its installation, configuration, and development
sections now have the dedicated guides above. Historical validation records
distinguish automated checks, browser review, and platform checks; evidence
from one does not establish the others.

## Licensing

Project code and documentation use Apache License 2.0.
Bundled Asap and Iosevka Custom fonts retain their SIL OFL 1.1 licences.
Font and dependency provenance is recorded in `THIRD_PARTY_NOTICES.md`.

## Document changes

- Version 7: Introduce the viewer and its review use before the commands;
  move detailed operational reference sections to dedicated guides and
  preserve the previous README as a historical snapshot.
