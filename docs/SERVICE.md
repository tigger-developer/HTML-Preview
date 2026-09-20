---
title: Local preview service
version: 2
last-updated: 2026-09-20
---

# Local preview service

The optional service renders documents when a browser requests them. It listens
only on `127.0.0.1`, on an automatically assigned HTTP port. There is no setting
for another listening address. The [service validation record](../specs/006-local-preview-service/validation.org)
separates automated checks from user tests; Linux/WSL validation remains pending.

The project is licensed under Apache License 2.0. Font and dependency licences
are distributed with the executable.

## Configure permitted directories

For a foreground service, run `htmlpreview --serve` from your documentation
directory. If neither default config exists, that directory and its descendants
are served. No configuration file is required for this workflow.

Configuration selection is first-found, without merging or searching ancestors:

1. An absolute `HTMLPREVIEW_CONFIG` override, if set.
2. `config.yaml` in the current directory.
3. `~/.config/htmlpreview/config.yaml`, on every supported platform.

A missing explicit override, invalid or unreadable file, or dangling config
symlink is an error. Only absence of both default files permits startup-directory
fallback. To select explicit roots, start with the packaged `config.example.yaml`.
Copy it to your chosen location only if that file does not already exist, then
edit its roots:

```yaml
version: 1
serve:
  roots:
    - /absolute/path/to/documentation
    - /another/absolute/documentation/tree
```

An empty roots list serves no files. Configuration accepts at most 32 existing
directory paths, written as absolute paths, with a leading `~/` for your home,
or as `$TMPDIR` / `${TMPDIR}` with an optional child path. TMPDIR is expanded
from the process environment when configuration loads. An unset, empty or
relative TMPDIR is an error; no shell expressions or other variables are expanded.
Duplicate canonical paths collapse; the most specific
configured root applies where roots overlap. Files must remain on the root's
filesystem. Symlink escapes, directories and special files cannot be previewed.

The defaults are:

| Platform | Configuration | Private runtime directory |
| --- | --- | --- |
| macOS | `~/.config/htmlpreview/config.yaml` | `~/Library/Caches/htmlpreview/runtime` |
| Linux and WSL | `~/.config/htmlpreview/config.yaml` | `~/.cache/htmlpreview/runtime` |

Linux honours `XDG_CACHE_HOME` for runtime storage. Configuration discovery does
not use `XDG_CONFIG_HOME`. Absolute `HTMLPREVIEW_CONFIG` and
`HTMLPREVIEW_RUNTIME_DIR` values override the respective locations. The CLI and
service must use the same runtime directory. Keep the
configuration user-owned, without group or other write permission. The service
requires a private runtime directory and creates its control socket with mode
0600 inside a mode-0700 directory.

Restart the service after changing its configured roots. The running service's
root snapshot remains authoritative until restart; a later CLI configuration
cannot expand it. `HTMLPREVIEW_ROOT` restricts explicit CLI inputs first and can
only narrow HTTP access. A file excluded by that client restriction is an error.
A permitted CLI input outside the service's roots uses file fallback.

## Initial folding

The selected configuration may also set initial folding for file and service previews:

```yaml
folding:
  override:
    headers:
      default: open
      todo: open
      done: closed
    drawers: closed
    default: open
```

Each leaf accepts `open` or `closed`. `headers.todo` and `headers.done` use
semantic task categories from the document, including custom Org keywords declared
before and after `|` in `#+TODO`, `#+SEQ_TODO` or `#+TYP_TODO`. They do not match
the literal words TODO or DONE. `headers.default` applies to unclassified headings
and supplies the fallback for an omitted category. If both are omitted, the
heading retains authored or normal initial folding. `drawers` applies to Org
drawers; the outer `default` applies to other foldable content, including
frontmatter. It is not a fallback for headings or drawers.

**Breaking configuration change:** version remains `1`, but the former scalar
`headers: open` or `headers: closed` is no longer accepted. Replace it with
`headers: {default: open}` or `headers: {default: closed}` to preserve its effect,
then add category overrides as needed. Configuration is never rewritten for you.

These settings set the initial view; folding bars, explicit buttons, Show all, fragment links
and print remain operational. Show all also opens frontmatter. Annotation
refresh retains heading choices with stable explicit IDs and uniquely identified
details. Ambiguous details use the initial settings again.

A command with folding overrides supplies them to the service for its preview
and linked pages. Otherwise the service's startup folding settings apply. The
CLI reads configuration on each invocation; restart the service to change its
own defaults. Neither case expands the running service's permitted roots.

## Start in the foreground

The current executable requires Pandoc 3.9.0.2 or a later 3.9 patch, with bundled
Lua 5.4, for reader discovery and non-Org documents. Org conversion is native Go.
With the dependency installed, run:

```sh
htmlpreview --serve
```

In another terminal, invoke the command normally:

```sh
htmlpreview /absolute/path/to/documentation/work.org
```

The CLI opens the system default browser and returns after preparing HTTP
entries. Follow relative links to render other supported documents, source code
and plain text. Targets use their own detected format. An individual link may
select a reader with `?htmlpreview-format=markdown%2Bsmart`; encode a literal plus
as `%2B`. `--from` selects the reader for explicit CLI inputs. The service's
Pandoc installation validates HTTP reader choices.

Ctrl+C stops a foreground service. Restarting revokes previously issued browser
URLs; run `htmlpreview` again to obtain a current URL. Cache eviction alone does
not revoke URLs. The browser must retain the complete token-prefixed URL, which
grants reading access within its authorized root context.

## Install service artefacts

`make install` builds the executable and installs a user-local symlink. Its
service examples, guide and expanded native service template are placed under
`~/.local/share/htmlpreview/`. The generated command points to the resolved
checkout executable. Keep that checkout available while using this installation.

An explicit `PREFIX` installs independent executable and shared-data copies:

```sh
make install PREFIX=/absolute/installation/prefix
```

The shared directory is `PREFIX/share/htmlpreview/`; the generated service uses
`PREFIX/bin/htmlpreview`. `DESTDIR` stages destinations only and never enters the
service's executable or configuration arguments. Installation requires Pandoc
to determine a fixed service PATH. Neither installation mode creates or changes
your active configuration, registers a service, or starts it.

The generated template explicitly sets `~/.config/htmlpreview/config.yaml` for
the installing user. Create that configuration before manager activation; this
prevents the manager's incidental working directory from becoming a served root.
If you choose an override, update the inactive service template's
`HTMLPREVIEW_CONFIG` before installing it into your service manager.

The 13 September 2026 configuration amendment supersedes the former macOS
Application Support and Linux XDG configuration defaults. Existing user files
are not moved or rewritten automatically.

## macOS with Homebrew

The generated local Homebrew formula declares the Pandoc dependency and supplies
a service definition. Its archive URLs are local unless release generation was
given a verified publication location. This project does not assume that a tap
has been published. Install the generated formula using the release instructions
before invoking its service commands.

Create your root configuration, then start the installed formula's service:

```sh
brew services start htmlpreview
```

To stop it:

```sh
brew services stop htmlpreview
```

The service runs as your user. It uses Homebrew's standard service PATH and
restarts after unsuccessful exits. The generated definition follows
[Homebrew's service contract](https://docs.brew.sh/Formula-Cookbook#service-files).

## macOS with a source or prefix installation

From this checkout, start the user LaunchAgent with:

```sh
make serve
```

`make service` is the equivalent target. It builds the executable, writes
`~/Library/LaunchAgents/org.htmlpreview.agent.plist` with absolute executable,
configuration and Pandoc PATH values. It replaces the existing
`gui/<uid>/org.htmlpreview.agent` registration using `launchctl bootout`,
`enable` and `bootstrap`. Run it directly after switching checkouts: no manual
unload or plist deletion is needed. An absent job is normal; other manager errors
are reported. Existing regular or symlinked plists are replaced without writing
through a symlink to another checkout.
It uses the configuration search order above, but requires an existing file;
it never uses launchd's incidental working directory as a fallback root.
The generated plist uses the checkout binary, so retain the checkout.

To stop it:

```sh
make service-stop
```

This invokes `launchctl bootout` by service label and retains the plist. Repeating
it when the job is absent succeeds. `make service` also handles a running job,
so a separate stop is optional. If replacement activation fails, the installer
restores the previous plist and attempts to restart a previously registered job;
it reports both activation and recovery errors. If recovery cannot restore the
plist, its diagnostic identifies retained recovery files. Existing configuration
and service logs are preserved. These convenience targets are macOS-only; Linux and WSL
use the foreground command or the user service instructions below. They do not
open a browser. The following manual procedure remains an alternative for
prefix installations or inspecting a plist before activation.

Check for an existing `org.htmlpreview.agent` job before copying a plist:

```sh
launchctl print "gui/$(id -u)/org.htmlpreview.agent"
```

Preserve an existing conflicting job or plist. After resolving any conflict,
create `~/Library/LaunchAgents/` if needed and copy the expanded
`service/org.htmlpreview.agent.plist` from the installed shared directory to
`~/Library/LaunchAgents/org.htmlpreview.agent.plist`. Inspect its executable,
configuration and PATH values before activation.

```sh
plutil -lint "$HOME/Library/LaunchAgents/org.htmlpreview.agent.plist"
```

```sh
launchctl bootstrap "gui/$(id -u)" "$HOME/Library/LaunchAgents/org.htmlpreview.agent.plist"
```

To stop and unregister it:

```sh
launchctl bootout "gui/$(id -u)" "$HOME/Library/LaunchAgents/org.htmlpreview.agent.plist"
```

The LaunchAgent starts on load, restarts after unsuccessful exits, and throttles
restarts by five seconds. It is a user LaunchAgent, with no root LaunchDaemon.

## Linux and WSL with a user service manager

Preserve an existing conflicting `htmlpreview.service`. Copy the expanded
`service/htmlpreview.service` to `~/.config/systemd/user/htmlpreview.service`
after inspecting its executable, configuration and PATH. Then run:

```sh
systemctl --user daemon-reload
```

```sh
systemctl --user enable --now htmlpreview.service
```

Inspect its status and user journal:

```sh
systemctl --user status htmlpreview.service
```

```sh
journalctl --user -u htmlpreview.service
```

To stop and disable it:

```sh
systemctl --user disable --now htmlpreview.service
```

The unit uses `Type=simple`, `Restart=on-failure` and `RestartSec=5`. Installation
does not enable a system unit, lingering, or systemd within WSL. Use foreground
`htmlpreview --serve` where a user service manager is unavailable.

WSL uses the Windows default browser. Before passing it an HTTP preview, the CLI
performs a five-second Windows-side health probe with proxies and redirects
disabled. The response must identify the same service instance as private
discovery. Failure selects a file preview without changing WSL networking.

## Archive templates

Every archive contains the empty-root example and this guide under
`share/htmlpreview/`. Darwin archives include
`service/org.htmlpreview.agent.plist.tmpl`; Linux archives include
`service/htmlpreview.service.tmpl`.

Archives cannot know your installation paths. Make an inactive copy without the
`.tmpl` suffix and replace the named substitutions before manager setup:

| Substitution | LaunchAgent value | systemd value |
| --- | --- | --- |
| `{{.Executable}}` | XML-escaped absolute executable path | Quoted absolute executable path |
| `{{.Config}}` | XML-escaped absolute config path | Quoted `HTMLPREVIEW_CONFIG=/absolute/config/path` assignment |
| `{{.Log}}` | XML-escaped absolute service log path | Not used; systemd captures stderr |
| `{{.Path}}` | XML-escaped fixed PATH | Quoted `PATH=/pandoc/directory:/usr/bin:/bin` assignment |

Use your installed Pandoc directory and platform system directories in PATH.
Keep `--serve` as its separate argument. In systemd values, escape backslashes
and double quotes, and double literal percent signs. In the executable argument,
also double literal dollar signs. Do not introduce shell commands, profile
loading or runtime command substitution. Validate the resulting plist or unit
before activation. Source/prefix installation performs these substitutions,
following the upstream [systemd quoting rules](https://www.freedesktop.org/software/systemd/man/latest/systemd.syntax.html)
and [command expansion rules](https://www.freedesktop.org/software/systemd/man/latest/systemd.service.html).

## Fallback, limits and troubleshooting

Absent, refused or timed-out private discovery selects a temporary file preview.
Service root exclusion does the same. Invalid configuration, unsafe runtime
permissions, incompatible protocols and conversion failures remain errors.
Read the diagnostic and correct its cause; restarting an incompatible service
requires the operator's explicit action.

If the public endpoint disappears during entry preparation, the CLI makes one
token-free health request with a five-second deadline. Confirmed unavailability
selects file fallback before browser handoff. A reachable endpoint or a failed
document conversion remains an error. Failed temporary-directory cleanup reports
the exact remaining owned path and refuses publication.

File fallback converts explicitly requested inputs only. `HTMLPREVIEW_LINKS`
and `HTMLPREVIEW_MAX_DEPTH` retain their old validation ranges but are deprecated
and have no graph effect. Non-empty values emit one migration notice per command.
`HTMLPREVIEW_MODE=quick` or `read` still controls file-preview retention. It never
controls service lifetime. Mixed batches prepare valid entries before browser
handoff and preserve their input order.

The service bounds conversion workers, source/output sizes, cached pages and
registered assets. Capacity returns HTTP 503 with a one-second retry hint;
oversized documents return 413, invalid conversions 422 and conversion timeouts
504. A service restart resets capability capacity. No directory listing or
automatic filesystem index is available.

Local raster images require an authorized document reference and a matching
allowlisted signature. Registered PDF and SVG anchors download unless an
explicit installed reader was selected. Native HTML retains passive styling;
its scripts, forms and automatic remote resources are disabled. Container media
is bound to its parent source revision. Reading does not modify documents.
[Annotations](ANNOTATIONS.md) add a separate, constrained write route for genuine
Org and Markdown sources. The CLI captures `HTMLPREVIEW_USER_DISPLAY_NAME`, with
`USER` as fallback. Older services receive the original read-only registration
request after one annotation-unavailable notice; other protocol errors remain
errors. The daemon never supplies its own display name.

## Rollback

Stop or disable the selected user service before restoring a previous package
version or removing your installed unit/plist. Preserve configuration and source
trees. Run the user manager's reload operation after changing unit files. With
no service running, the command retains file-preview fallback.

## Service logs

`make serve` creates a mode-0600 log file at
`~/Library/Logs/htmlpreview/service.log` and directs the LaunchAgent's stdout
and stderr there. The directory is created with mode 0700. Existing log content
is retained. The service records startup/shutdown and annotation failures with a
timestamp, HTTP method, status, error code and opaque document identifier.
Annotation diagnostics omit document text, comment text, author names and
capability URLs. Successful refresh requests are not access-logged.

After updating an older installation, run `make serve` to regenerate
and reload its plist. Reopen previews to load the current browser code. Inspect
recent diagnostics without opening a browser:

```sh
tail -n 100 "$HOME/Library/Logs/htmlpreview/service.log"
```

The application does not rotate the log. Stop the service before archiving or
truncating it. For manual/prefix LaunchAgent installation, create the private log
directory/file before loading the generated plist. Linux user services use their
existing journal; foreground mode writes diagnostics to stderr.

## Document changes

- Version 2: clarify converter prerequisites and current service status; repair
  the TMPDIR configuration paragraph.
- 20 September 2026: service activation replaces an existing checkout registration;
  stopping an absent job succeeds, and failed activation attempts prior-state recovery.
