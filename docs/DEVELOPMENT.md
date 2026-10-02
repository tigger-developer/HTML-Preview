---
title: Development and packaging
version: 1
last-updated: 2026-10-01
---

# Development and packaging

```sh
make test
```

```sh
make lint
```

```sh
make vulncheck
```

`make test` uses Go's race detector, real native/Pandoc conversion, subprocess tests,
and controlled desktop boundaries. It also checks staged installation and
cross-compiled archives. Its per-package timeout is twenty minutes for the full
reader and filename-mapping corpus; application deadlines remain separate.
Cross-compilation and doubles do not establish native
execution on another OS or actual browser behaviour.

Provision development tools separately: golangci-lint 1.64.8, StyLua 2.5.2,
and govulncheck 1.7.0 are the inspected tool versions for this delivery.
Lint includes Go formatting, vet, the selected Go linters, StyLua, and a Lua
check in Pandoc's own host. No Node/npm or standalone Lua runtime is used;
native oxlint and biome check browser JavaScript and CSS; paired human checks
cover actual browser interaction and appearance. The historical browser fixture
is retired from the current workflow and is not execution evidence.
The vulnerability target scans linked code and the pinned upstream Org parser
identity, including its highlighter dependencies. It rejects stale advisory
manifests, and neither installs tools nor updates dependencies.

```sh
make release VERSION=0.1.0
```

Release generation produces `dist/htmlpreview-VERSION-OS-ARCH.tar.gz` for
darwin/amd64, darwin/arm64, linux/amd64, and linux/arm64, plus `SHA256SUMS`
and `dist/Formula/htmlpreview.rb`. WSL uses a Linux archive. Each archive
contains a CGO-free binary and the application, font, and dependency licences.

The macOS-only Homebrew formula leaves Pandoc optional and selects the corresponding
architecture's archive and checksum. Its default URLs point to the actual local
archives. `RELEASE_BASE_URL` can identify an existing HTTP(S) archive location;
generation verifies those remote bytes before emitting that formula. It does
not publish assets, create a tap, or install a formula automatically.

`make sync` stages the whole working tree, commits when needed, then pulls and
pushes. `COMMIT_MESSAGE` defaults to `chore: sync`. Invoke it only when you intend
to include all current changes.
