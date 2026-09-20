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

The sole parser change is in `org/document.go`: `Configuration.MaxScanTokenSize`
sets the scanner allowance from the already bounded intermediary input length.
A zero value retains upstream's scanner default. No source offsets, source
rewriting or grammar changes are added. HTML compatibility belongs to
`internal/orgconvert`, using the upstream writer extension interface.

The root module pins this version and replaces it with this local module.
`provenance.json` retains the upstream identity. The `advisory` module pins the
unreplaced upstream parser and the application's highlighter and shared
transitive versions. `make vulncheck` scans application code, checks those
versions agree, then copies the advisory manifests to an OS temporary directory
and runs `govulncheck -scan=module` there. It never updates tracked manifests.
The module scan supplements local-code review; it cannot verify the scanner
patch itself. Update provenance, notices and both locked dependency graphs
together when upgrading.

## Document changes

- Version 1 metadata: document the native Org converter and its dependency provenance where applicable.
