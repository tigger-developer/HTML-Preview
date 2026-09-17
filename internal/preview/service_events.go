// ABOUTME: Pushes document invalidations over capability-authorized server-sent events.
// ABOUTME: Watches containing directories so atomic saves retain notification coverage.
package preview

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
)

func (s *previewService) invalidateAnnotationState(path string) {
	s.annotationPollMu.Lock()
	defer s.annotationPollMu.Unlock()
	// Keys contain the canonical path between NUL delimiters.
	for key := range s.annotationPolls {
		if strings.Contains(key, "\x00"+path+"\x00") {
			delete(s.annotationPolls, key)
		}
	}
}

func (s *previewService) serveAnnotationEvents(w http.ResponseWriter, r *http.Request, src sourceContext) {
	s.mu.Lock()
	if s.eventStreams >= 32 {
		s.mu.Unlock()
		s.annotationError(w, r, 503, "event_capacity")
		return
	}
	s.eventStreams++
	s.mu.Unlock()
	defer func() { s.mu.Lock(); s.eventStreams--; s.mu.Unlock() }()
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		s.annotationError(w, r, 503, "watch_unavailable")
		return
	}
	defer func() {
		if err := watcher.Close(); err != nil && s.diagnostics != nil {
			s.diagnostics.Print("document events: watcher close failed")
		}
	}()
	directories := map[string]bool{filepath.Dir(src.canonical): true, filepath.Dir(src.logical): true}
	for path := range directories {
		if err := watcher.Add(path); err != nil {
			s.annotationError(w, r, 503, "watch_unavailable")
			return
		}
	}
	control := http.NewResponseController(w)
	send := func(message string) bool {
		if err := control.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
			return false
		}
		if _, err := fmt.Fprint(w, message); err != nil {
			return false
		}
		return control.Flush() == nil
	}
	w.Header().Set("Content-Type", "text/event-stream")
	// The first event also reconciles changes made while the stream was disconnected.
	s.invalidateAnnotationState(src.canonical)
	if !send("retry: 1000\nevent: change\ndata: {}\n\n") {
		return
	}
	heartbeat := time.NewTicker(30 * time.Second)
	defer heartbeat.Stop()
	var burst *time.Timer
	var changed <-chan time.Time
	defer func() {
		if burst != nil {
			burst.Stop()
		}
	}()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-s.ctx.Done():
			return
		case <-heartbeat.C:
			if !send(": keepalive\n\n") {
				return
			}
		case <-changed:
			changed = nil
			s.invalidateAnnotationState(src.canonical)
			if !send("event: change\ndata: {}\n\n") {
				return
			}
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			relevant := event.Name == src.canonical || event.Name == src.logical || event.Name == src.canonical+"-annotations.org"
			for directory := range directories {
				if event.Name == directory && event.Has(fsnotify.Remove|fsnotify.Rename) {
					s.invalidateAnnotationState(src.canonical)
					send("event: change\ndata: {}\n\n")
					return
				}
			}
			if relevant && changed == nil {
				if burst == nil {
					burst = time.NewTimer(100 * time.Millisecond)
				} else {
					burst.Reset(100 * time.Millisecond)
				}
				changed = burst.C
			}
		case _, ok := <-watcher.Errors:
			if !ok {
				return
			}
			if s.diagnostics != nil {
				s.diagnostics.Print("document events: watcher failed; reconnect required")
			}
			// Reconciliation after reconnect covers queue overflow as well as missed events.
			return
		}
	}
}
