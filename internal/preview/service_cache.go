// ABOUTME: Caches bounded immutable render results after source and dependency revalidation.
// ABOUTME: Shares in-flight work while limiting distinct conversions and waiter lifetimes.
package preview

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
)

type assetRevision struct {
	source sourceContext
	sum    [32]byte
}
type httpPage struct {
	data                  []byte
	source                sourceContext
	revision              string
	ids, headings, orgIDs map[string][]string
	dependencies          map[string]assetRevision
	cacheable             bool
	media                 map[string]servedRaster
	assetGrants           map[string]assetGrant
	mediaGrants           map[string]mediaGrant
	cap                   *readCapability
	catalogueSensitive    bool
	used                  uint64
}
type renderWork struct {
	done    chan struct{}
	cancel  context.CancelFunc
	waiters int
	page    *httpPage
	status  int
}

func (s *previewService) currentPage(ctx context.Context, cap *readCapability, src sourceContext) (*httpPage, int) {
	input, err := snapshot(src, min(cap.settings.sourceBytes, cap.settings.totalBytes))
	if err != nil {
		return nil, snapshotStatus(err)
	}
	sum := sha256.Sum256(input)
	revision := hex.EncodeToString(sum[:])
	key := cap.token + "\x00" + src.logical + "\x00" + src.input.key() + "\x00" + revision
	s.mu.Lock()
	cached := s.cache[key]
	s.mu.Unlock()
	if cached != nil && !cached.catalogueSensitive && s.dependenciesCurrent(cached, cap) {
		s.mu.Lock()
		s.clock++
		cached.used = s.clock
		s.mu.Unlock()
		return cached, 200
	}
	s.mu.Lock()
	if work := s.inflight[key]; work != nil {
		work.waiters++
		s.mu.Unlock()
		return s.waitForPage(ctx, work)
	}
	if len(s.inflight) >= 2 || s.ctx.Err() != nil {
		s.mu.Unlock()
		return nil, 503
	}
	workCtx, cancel := context.WithTimeout(s.ctx, cap.settings.deadline)
	work := &renderWork{done: make(chan struct{}), cancel: cancel, waiters: 1}
	s.inflight[key] = work
	s.workers.Add(1)
	s.mu.Unlock()
	go func() {
		defer s.workers.Done()
		page, status := s.buildHTTP(workCtx, cap, src, input)
		cancel()
		s.mu.Lock()
		defer s.mu.Unlock()
		if page != nil && !s.publishGrants(page) {
			page, status = nil, 503
		}
		if page != nil {
			page.revision = revision
			if page.cacheable {
				s.cachePage(key, page)
			}
		}
		work.page, work.status = page, status
		delete(s.inflight, key)
		close(work.done)
	}()
	return s.waitForPage(ctx, work)
}

func (s *previewService) waitForPage(ctx context.Context, work *renderWork) (*httpPage, int) {
	defer func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		work.waiters--
		if work.waiters == 0 {
			work.cancel()
		}
	}()
	select {
	case <-ctx.Done():
		return nil, 504
	case <-work.done:
		return work.page, work.status
	}
}

func (s *previewService) dependenciesCurrent(page *httpPage, cap *readCapability) bool {
	for _, dependency := range page.dependencies {
		src, err := s.localAssetSource(cap, dependency.source.logical)
		if err != nil || src.canonical != dependency.source.canonical || src.device != dependency.source.device {
			return false
		}
		root, err := os.Stat(src.root)
		if err != nil || src.device != deviceOf(root) {
			return false
		}
		data, err := snapshot(src, min(maxAssetBytes, cap.settings.sourceBytes))
		if err != nil || sha256.Sum256(data) != dependency.sum {
			return false
		}
	}
	return true
}

func (s *previewService) cachePage(key string, page *httpPage) {
	if old := s.cache[key]; old != nil {
		s.cacheBytes -= old.byteCost()
	}
	s.clock++
	page.used = s.clock
	s.cache[key] = page
	s.cacheBytes += page.byteCost()
	for len(s.cache) > 50 || s.cacheBytes > 100*1024*1024 {
		oldest := ""
		var stamp uint64
		for candidate, item := range s.cache {
			if oldest == "" || item.used < stamp {
				oldest, stamp = candidate, item.used
			}
		}
		s.cacheBytes -= s.cache[oldest].byteCost()
		delete(s.cache, oldest)
	}
}

func (p *httpPage) byteCost() int64 {
	total := int64(len(p.data) + len(p.source.logical) + len(p.source.canonical) + len(p.revision))
	for _, catalogue := range []map[string][]string{p.ids, p.headings, p.orgIDs} {
		for key, values := range catalogue {
			total += int64(len(key))
			for _, value := range values {
				total += int64(len(value))
			}
		}
	}
	for name, dependency := range p.dependencies {
		total += int64(len(name) + len(dependency.source.logical) + len(dependency.source.canonical) + len(dependency.sum))
	}
	for id, media := range p.media {
		total += int64(len(id) + len(media.data) + len(media.mime))
	}
	return total
}

func snapshotStatus(err error) int {
	var limit *sourceLimitError
	if errors.As(err, &limit) {
		return 413
	}
	return sourceHTTPStatus(err)
}
