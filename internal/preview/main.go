// ABOUTME: Coordinates command validation and the foreground preview lifecycle.
// ABOUTME: External operations enter through an internal host boundary.
package preview

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
)

// Main runs one command and returns its public exit status.
func Main(args, env []string, out, diagnostics io.Writer, version, revision string, host Host) int {
	log := &console{out: out, diagnostics: diagnostics}
	files, info, err := arguments(args)
	if err != nil {
		log.warn("%v", err)
		return 2
	}
	if info == "--version" {
		log.print("htmlpreview %s (%s)", version, revision)
		return log.status(0)
	}
	if info != "" {
		log.print("%s", helpText)
		return log.status(0)
	}
	cfg, err := settings(env)
	if err != nil {
		log.warn("%v", err)
		return 2
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return execute(ctx, files, env, cfg, host, log)
}

type console struct {
	out, diagnostics io.Writer
	failed           bool
	warningBytes     int
	warningsOmitted  bool
}

// Source-controlled warnings have a separate allowance so lifecycle failures
// and the retained-session path always remain visible.
func (c *console) notice(format string, args ...any) {
	const allowance = 64 * 1024
	if c.warningsOmitted {
		return
	}
	message := fmt.Sprintf(format, args...)
	if c.warningBytes+len(message)+14 > allowance {
		c.warningsOmitted = true
		c.warn("further source warnings omitted")
		return
	}
	c.warningBytes += len(message) + 14
	c.warn("%s", message)
}

func (c *console) warn(format string, args ...any) {
	if _, err := fmt.Fprintf(c.diagnostics, "htmlpreview: "+format+"\n", args...); err != nil {
		c.failed = true
	}
}
func (c *console) print(format string, args ...any) {
	if _, err := fmt.Fprintf(c.out, format+"\n", args...); err != nil {
		c.failed = true
	}
}
func (c *console) status(code int) int {
	if c.failed {
		return 1
	}
	return code
}

const helpText = `Usage: htmlpreview FILE [FILE ...]
Preview local UTF-8 Markdown (.md, .markdown) and Org (.org) documents.
Use -- before a filename beginning with -. Open the system default browser.

  -h, --help       Show this help without reading documents or settings
  --version        Show build identity without preview side effects

Environment settings (empty values use defaults):
  HTMLPREVIEW_LINKS                   0; 0 or 1 for bounded linked browsing
  HTMLPREVIEW_MODE                    quick; read when LINKS=1
  HTMLPREVIEW_ROOT                    Each entry's canonical parent directory
  HTMLPREVIEW_GRACE                   3s; 100ms to 1h, quick retention
  HTMLPREVIEW_MAX_FILES               50; 1 to 500 contexts including entries
  HTMLPREVIEW_MAX_DEPTH               3; 0 to 10 linked levels
  HTMLPREVIEW_MAX_SOURCE_BYTES        10485760; 1 to 10485760
  HTMLPREVIEW_MAX_TOTAL_SOURCE_BYTES  52428800; 1 to 52428800
  HTMLPREVIEW_MAX_OUTPUT_BYTES        104857600; 1 to 104857600
  HTMLPREVIEW_DEADLINE                60s; 100ms to 10m for conversion

LINKS=1 requires read mode. Read mode retains pages until Ctrl+C or SIGTERM.
Quick mode removes its private temporary directory after the grace period;
the delay does not guarantee browser readiness. SIGKILL can leave that directory.
Source files are never modified. Local assets remain dependent on their originals.

Requires Pandoc 3.9.0.2 or a later 3.9 patch with bundled Lua 5.4.
macOS uses /usr/bin/open; Linux desktops require xdg-open (xdg-utils).
WSL requires wslpath, built-in Windows powershell.exe, enabled interoperation,
and private Linux temporary storage. WSL opens the Windows default browser.
No system font installation is required. Asap and Iosevka Custom are embedded.

Exit status: 0 success, 1 operational failure, 2 invalid invocation.`
