# Local preview service

The optional service renders documents when a browser requests them. It listens
only on `127.0.0.1`, on an automatically assigned HTTP port. There is no setting
for another listening address. This is a delivery candidate; native service and
browser qualification is recorded in the W006 validation record.

The project is licensed under Apache License 2.0. Font and dependency licences
are distributed with the executable.

## Configure permitted directories

Start with the packaged `config.example.yaml`. Copy it to your configuration
location only if that file does not already exist, then edit its roots:

```yaml
version: 1
serve:
  roots:
    - /absolute/path/to/documentation
    - /another/absolute/documentation/tree
```

An empty roots list serves no files. Configuration accepts at most 32 existing
absolute directory paths. Duplicate canonical paths collapse; the most specific
configured root applies where roots overlap. Files must remain on the root's
filesystem. Symlink escapes, directories and special files cannot be previewed.

The defaults are:

| Platform | Configuration | Private runtime directory |
| --- | --- | --- |
| macOS | `~/Library/Application Support/htmlpreview/config.yaml` | `~/Library/Caches/htmlpreview/runtime` |
| Linux and WSL | `~/.config/htmlpreview/config.yaml` | `~/.cache/htmlpreview/runtime` |

Linux honours `XDG_CONFIG_HOME` and `XDG_CACHE_HOME`. Absolute
`HTMLPREVIEW_CONFIG` and `HTMLPREVIEW_RUNTIME_DIR` values override the respective
locations. The CLI and service must use the same runtime directory. Keep the
configuration user-owned, without group or other write permission. The service
requires a private runtime directory and creates its control socket with mode
0600 inside a mode-0700 directory.

Restart the service after changing its configured roots. The running service's
root snapshot remains authoritative until restart; a later CLI configuration
cannot expand it. `HTMLPREVIEW_ROOT` restricts explicit CLI inputs first and can
only narrow HTTP access. A file excluded by that client restriction is an error.
A permitted CLI input outside the service's roots uses file fallback.

## Start in the foreground

With Pandoc installed, run:

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

The generated template sets the default configuration path for the installing
user. If you choose an override, update the inactive service template's
`HTMLPREVIEW_CONFIG` before installing it into your service manager.

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
`.tmpl` suffix and replace the three named substitutions before manager setup:

| Substitution | LaunchAgent value | systemd value |
| --- | --- | --- |
| `{{.Executable}}` | XML-escaped absolute executable path | Quoted absolute executable path |
| `{{.Config}}` | XML-escaped absolute config path | Quoted `HTMLPREVIEW_CONFIG=/absolute/config/path` assignment |
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
is bound to its parent source revision. The service does not modify documents;
annotations belong to the separate W007 delivery.

## Rollback

Stop or disable the selected user service before restoring a previous package
version or removing your installed unit/plist. Preserve configuration and source
trees. Run the user manager's reload operation after changing unit files. With
no service running, the command retains file-preview fallback.
