---
title: go-org local module
version: 1
last-updated: 2026-09-20
---

# go-org local module

The Org package is copied from `github.com/niklasfasching/go-org` v1.9.1,
revision `2f088a12697ba4dad46c2c2084db3ae1707830fb`. The upstream MIT licence
is retained in `LICENSE` and in the distribution notices. The upstream CLI,
website, fixtures and test dependencies are not copied into this module.

The scanner change is in `org/document.go`: `Configuration.MaxScanTokenSize`
sets the scanner allowance from the already bounded intermediary input length.
A zero value retains upstream's scanner default. Two corrections in `org/list.go`
preserve the application's existing Org reading semantics:

- Ordered-list markers use digits. Upstream also accepts letters unconditionally,
  consuming wrapped prose such as `p. 32` as a nested list and hiding `p.`.
  [Org's default grammar](https://orgmode.org/manual/Plain-Lists.html) uses numeric
  markers; alphabetical lists require an editor setting. The preview does not
  import that editor setting.
- A change between descriptive and ordinary items ends the current list.
  Upstream otherwise converts ordinary items after a definition into empty-term
  definitions and renders an invented `?` label.

`TestRT011_2_CorpusListSemantics` supplies synthetic regressions for both cases,
including numeric lists. The alphabetic-marker correction also replaces the
earlier writer workaround for a definition whose complete description was `I.`.
No source offsets or source rewriting are added. HTML compatibility belongs to
`internal/orgconvert`, using the upstream writer extension interface.

The root module pins this version and replaces it with this local module.
`provenance.json` retains the upstream identity. The `advisory` module pins the
unreplaced upstream parser and the application's highlighter and shared
transitive versions. `make vulncheck` scans application code, checks those
versions agree, then copies the advisory manifests to an OS temporary directory
and runs `govulncheck -scan=module` there. It never updates tracked manifests.
The module scan supplements local-code review; it cannot verify the local
patches themselves. Update provenance, notices and both locked dependency graphs
together when upgrading.

## Document changes

- Version 1 metadata: document the native Org converter and its dependency provenance where applicable.
