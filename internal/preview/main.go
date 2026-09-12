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

	bundle "github.com/tigger-developer/HTML-Preview"
)

// Main runs one command and returns its public exit status.
func Main(args, env []string, out, diagnostics io.Writer, version, revision string, host Host) int {
	log := &console{out: out, diagnostics: diagnostics}
	files, info, from, err := arguments(args)
	if err != nil {
		log.warn("%v", err)
		return 2
	}
	if info == "--version" {
		log.print("htmlpreview %s (%s)", version, revision)
		return log.status(0)
	}
	if info == "--list-input-formats" {
		return listFormats(host, log)
	}
	if info != "" && info != "--serve" {
		log.print("%s", bundle.HelpText)
		return log.status(0)
	}
	cfg, err := settings(env)
	if err != nil {
		log.warn("%v", err)
		return 2
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	if cfg.legacyGraph {
		log.warn("HTMLPREVIEW_LINKS and HTMLPREVIEW_MAX_DEPTH are deprecated; linked previews use the optional local service")
	}
	defer stop()
	cfg.from, cfg.version = from, version
	svc, err := serviceSettings(cfg)
	if err != nil {
		log.warn("service configuration: %v", err)
		return 2
	}
	if info == "--serve" {
		return runService(ctx, cfg, svc, host, log)
	}
	cfg.runtimePath = svc.runtime
	return execute(ctx, files, env, cfg, host, log)
}

func listFormats(host Host, log *console) int {
	ctx := context.Background()
	path, err := host.LookPath("pandoc")
	if err == nil {
		err = checkPandoc(ctx, host, path)
	}
	if err != nil {
		log.warn("Pandoc input-format listing: %v", err)
		return 1
	}
	catalogue, err := discoverFormats(ctx, host, path)
	if err != nil {
		log.warn("%v", err)
		return 1
	}
	log.print("%s", catalogue.listing)
	return log.status(0)
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
