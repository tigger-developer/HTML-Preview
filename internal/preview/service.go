// ABOUTME: Runs the explicitly requested loopback service and its private control plane.
// ABOUTME: Coordinates bounded HTTP lifetimes and shutdown without desktop activation.
package preview

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"golang.org/x/net/netutil"
)

type previewService struct {
	mu                       sync.Mutex
	workers                  sync.WaitGroup
	ctx                      context.Context
	config                   serviceConfig
	base                     config
	host                     Host
	pandoc, origin, instance string
	capabilities             map[string]*readCapability
	contexts                 map[string]*readCapability
	cache                    map[string]*httpPage
	inflight                 map[string]*renderWork
	cacheBytes               int64
	clock                    uint64
	assets                   map[string]assetGrant
	media                    map[string]mediaGrant
}

type readCapability struct {
	token, root, logicalRoot, parent string
	settings                         config
}

func randomCapability() (string, error) {
	var secret [32]byte
	if _, err := rand.Read(secret[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(secret[:]), nil
}

func runService(ctx context.Context, cfg config, svc serviceConfig, host Host, console *console) (code int) {
	ctx, stop := context.WithCancel(ctx)
	defer stop()
	pandoc, err := host.LookPath("pandoc")
	if err == nil {
		err = checkPandoc(ctx, host, pandoc)
	}
	if err != nil {
		console.warn("service dependency: %v", err)
		return 1
	}
	cfg.formats, err = discoverFormats(ctx, host, pandoc)
	if err != nil || len(cfg.formats.readers) > 256 {
		console.warn("service reader catalogue unavailable or oversized")
		return 1
	}
	for reader := range cfg.formats.readers {
		if len(reader) > 128 {
			console.warn("service reader name exceeds limit")
			return 1
		}
	}
	runtime, err := openServiceRuntime(svc.runtime)
	if err != nil {
		console.warn("service runtime: %v", err)
		return 1
	}
	defer func() {
		if err := runtime.close(); err != nil {
			console.warn("service runtime cleanup: %v", err)
			code = 1
		}
	}()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		console.warn("service listener unavailable")
		return 1
	}
	instance, err := randomCapability()
	if err != nil {
		console.warn("service identity unavailable")
		if err := listener.Close(); err != nil {
			console.warn("service listener cleanup failed")
		}
		return 1
	}
	s := &previewService{ctx: ctx, config: svc, base: cfg, host: host, pandoc: pandoc, origin: "http://" + listener.Addr().String(), instance: instance, capabilities: make(map[string]*readCapability), contexts: make(map[string]*readCapability)}
	s.cache = make(map[string]*httpPage)
	s.inflight = make(map[string]*renderWork)
	s.assets = make(map[string]assetGrant)
	s.media = make(map[string]mediaGrant)
	public := serviceHTTPServer(http.HandlerFunc(s.serveHTTP), ctx, 70*time.Second)
	control := serviceHTTPServer(http.HandlerFunc(s.serveControl), ctx, 5*time.Second)
	done := make(chan error, 2)
	go func() { done <- public.Serve(netutil.LimitListener(listener, 128)) }()
	go func() { done <- control.Serve(privateListener{runtime.listener}) }()
	remaining := 2
	select {
	case <-ctx.Done():
	case err = <-done:
		remaining--
		if !errors.Is(err, http.ErrServerClosed) {
			console.warn("service listener stopped unexpectedly")
			code = 1
		}
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	stop()
	for _, server := range []*http.Server{public, control} {
		if err := server.Shutdown(shutdown); err != nil {
			console.warn("service shutdown deadline exceeded")
			code = 1
			if err := server.Close(); err != nil {
				console.warn("service connection cleanup failed")
			}
		}
	}
	for range remaining {
		if err := <-done; !errors.Is(err, http.ErrServerClosed) {
			code = 1
		}
	}
	workersDone := make(chan struct{})
	go func() { s.workers.Wait(); close(workersDone) }()
	select {
	case <-workersDone:
	case <-shutdown.Done():
		console.warn("service conversion cleanup deadline exceeded")
		code = 1
	}
	return code
}

func serviceHTTPServer(handler http.Handler, ctx context.Context, lifetime time.Duration) *http.Server {
	return &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: lifetime, WriteTimeout: lifetime, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 * 1024,
		BaseContext: func(net.Listener) context.Context { return ctx },
		// The standard server's panic log can contain request details. Handlers
		// return bounded errors; source/token-bearing default logs are disabled.
		ErrorLog: log.New(io.Discard, "", 0),
	}
}
