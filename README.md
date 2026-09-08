# htmlpreview

Preview local Markdown and Org in the system default browser, using Pandoc,
embedded Asap and Iosevka Custom fonts, and a private temporary directory.

**Status:** Implementation and automated verification are in progress under
[the approved specification](specs/001-local-document-preview/spec.org).
Browser qualification and the Homebrew installation trial remain pending in
[the validation record](specs/001-local-document-preview/validation.org).

```sh
htmlpreview README.md docs/notes.org
```

Each distinct source context receives a separate preview. The header identifies
the original source; activating it copies its full logical path. When browser
clipboard access is unavailable, the header offers manual copying.

## Reading and navigation

Quick previews retain their files for three seconds after the last browser
handoff. This delay does not establish browser readiness. Use a reading session
for slow browser startup, reloading, or linked navigation:

```sh
HTMLPREVIEW_MODE=read htmlpreview README.md
```

The command stays in the foreground until Ctrl+C or SIGTERM. It then removes
only its own session directory. SIGKILL or a system failure can leave that
directory behind; the reported path identifies the exact directory for manual
removal. There is no background service or cross-session scavenger.

Linked browsing is opt-in and implies reading mode:

```sh
HTMLPREVIEW_LINKS=1 htmlpreview docs/VISION.md
```

By default, traversal stays within each entry's physical parent directory and
filesystem. An explicit root permits a wider document neighbourhood:

```sh
HTMLPREVIEW_LINKS=1 HTMLPREVIEW_ROOT="$PWD" htmlpreview docs/VISION.md
```

The graph follows rendered document anchors breadth-first. Cycles reuse the
same source context; symlink aliases with different logical parents retain
separate relative-link contexts. Images and other files remain references to
their original locations. Skipped or failed linked documents retain original
file links with diagnostics. Org ID and heading searches require unique actual
destinations; unresolved searches receive an explanation.

Sources remain unchanged. Source scripts, event handlers, executable embeds,
and automatic remote resources are removed or made passive. Org includes remain
visible without expansion. Literal source/example blocks remain literal.
This is a local preview, not a portable export or a whole-process sandbox.

## Prerequisites and installation

Binary use requires **Pandoc 3.9.0.2 through the 3.9 patch series**, including its
bundled Lua 5.4. Prefix installation does not download Pandoc. The command checks
compatibility before allocating preview output; a distribution package is not
automatically a compatible version.

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
make install PREFIX="$HOME/.local"
```

Put the selected prefix's `bin` directory on PATH. Check command shadowing with
`command -v htmlpreview`, particularly if you already have a personal script.
An executable symlink works without neighbouring assets: fonts, templates,
CSS, browser JavaScript, and Lua are embedded in the binary. Installation copies
only the binary and notices; it does not register system fonts.

The default prefix is `/usr/local`. `DESTDIR` supports staged installation;
the installer never invokes sudo. Repeating installation replaces only the
managed binary and licence files.

## Configuration

Empty values select defaults. Unknown `HTMLPREVIEW_` settings are errors.
Every supplied setting is validated, including those inactive in the selected
mode. There is no personal configuration file or arbitrary Pandoc-argument
interface. Use `--` before a filename beginning with `-`.

| Setting | Default | Accepted values |
| --- | --- | --- |
| `HTMLPREVIEW_LINKS` | `0` | `0` or `1` |
| `HTMLPREVIEW_MODE` | `quick`; `read` with links | `quick` or `read`; links require `read` |
| `HTMLPREVIEW_ROOT` | Each entry's canonical parent | Existing directory containing every explicit canonical source |
| `HTMLPREVIEW_GRACE` | `3s` | Go duration, `100ms` to `1h` |
| `HTMLPREVIEW_MAX_FILES` | `50` | Integer, 1 to 500 source contexts |
| `HTMLPREVIEW_MAX_DEPTH` | `3` | Integer, 0 to 10; entries are depth zero |
| `HTMLPREVIEW_MAX_SOURCE_BYTES` | `10485760` | Integer, 1 to 10485760 |
| `HTMLPREVIEW_MAX_TOTAL_SOURCE_BYTES` | `52428800` | Integer, 1 to 52428800 |
| `HTMLPREVIEW_MAX_OUTPUT_BYTES` | `104857600` | Integer, 1 to 104857600; includes embedded fonts and staging |
| `HTMLPREVIEW_DEADLINE` | `60s` | Go duration, `100ms` to `10m` |

Published entry URLs go to stdout; diagnostics go to stderr. Exit status is 0
for success, 1 for an operational failure, and 2 for an invalid invocation.
Successful entries may still open when another input fails. Publication is
transactional: a publication failure opens no entry. Help and version requests
have no preview side effects and bypass environment validation.

## Development and packaging

```sh
make test
```

```sh
make lint
```

```sh
make vulncheck
```

`make test` uses Go's race detector, real Pandoc conversion, subprocess tests,
and controlled desktop boundaries. It also checks staged installation and
cross-compiled archives. Cross-compilation and doubles do not establish native
execution on another OS or actual browser behaviour.

Provision development tools separately: golangci-lint 1.64.8, StyLua 2.5.2,
and govulncheck 1.7.0 are the inspected tool versions for this delivery.
Lint includes Go formatting, vet, the selected Go linters, StyLua, and a Lua
check in Pandoc's own host. No Node/npm or standalone Lua runtime is used;
CSS and browser JavaScript require source review and the recorded browser tests.
The vulnerability target neither installs tools nor updates dependencies.

```sh
make release VERSION=0.1.0
```

Release generation produces `dist/htmlpreview-VERSION-OS-ARCH.tar.gz` for
darwin/amd64, darwin/arm64, linux/amd64, and linux/arm64, plus `SHA256SUMS`
and `dist/Formula/htmlpreview.rb`. WSL uses a Linux archive. Each archive
contains a CGO-free binary and the application, font, and dependency licences.

The macOS-only Homebrew formula declares Pandoc and selects the corresponding
architecture's archive and checksum. Its default URLs point to the actual local
archives. `RELEASE_BASE_URL` can identify an existing HTTP(S) archive location;
generation verifies those remote bytes before emitting that formula. It does
not publish assets, create a tap, or install a formula automatically.

`make sync` stages the whole working tree, commits when needed, then pulls and
pushes. `COMMIT_MESSAGE` defaults to `chore: sync`. Invoke it only when you intend
to include all current changes.

## Design and licensing

See [VISION.md](docs/VISION.md), [ARCHITECTURE.md](docs/ARCHITECTURE.md), and
[the owned Org prototype assessment](docs/ORG-FIDELITY-REVIEW.md).

Project code and documentation use [Apache 2.0](LICENSE). Asap and Iosevka
Custom retain their SIL OFL 1.1 licences. Dependency licences and font provenance
are listed in [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).
