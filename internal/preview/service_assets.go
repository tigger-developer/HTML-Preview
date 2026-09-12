// ABOUTME: Registers local assets and parent-revision-scoped embedded media.
// ABOUTME: Reauthorizes each response and keeps downloads separate from raster presentation.
package preview

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

type servedRaster struct {
	data []byte
	mime string
}
type assetGrant struct {
	path, mime string
	download   bool
}
type mediaGrant struct {
	source       sourceContext
	revision, id string
}

func linkedAssetType(path string) (string, bool) {
	if kind := rasterSuffix(path); kind != "" {
		return kind, false
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".pdf", ".svg":
		return "application/octet-stream", true
	}
	return "", false
}

func digestText(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }

func (s *previewService) registerImage(cap *readCapability, p *page, revision, name, kind string, data []byte) (string, error) {
	if name != "" {
		return s.registerAsset(cap, p, name, kind, false)
	}
	docID := digestText([]byte(p.source.logical + "\x00" + p.source.input.key()))
	assetID := digestText(append([]byte(kind+"\x00"), data...))
	path := "/" + cap.token + "/_media/" + docID + "/" + revision + "/" + assetID
	if _, exists := p.mediaGrants[path]; !exists && len(p.mediaGrants)+len(p.assetGrants) >= 8192 {
		p.resourceLimit = true
		return "", errors.New("registered asset capacity exceeded")
	}
	if p.mediaGrants == nil {
		p.mediaGrants = make(map[string]mediaGrant)
	}
	p.mediaGrants[path] = mediaGrant{source: p.source, revision: revision, id: assetID}
	if p.httpMedia == nil {
		p.httpMedia = make(map[string]servedRaster)
	}
	p.httpMedia[assetID] = servedRaster{data: data, mime: kind}
	return s.origin + path, nil
}

func (s *previewService) registerAsset(cap *readCapability, p *page, path, kind string, download bool) (string, error) {
	canonical, err := canonicalLocation(path)
	if err != nil {
		return "", err
	}
	root := s.effectiveRoot(path, canonical, cap.settings.root)
	if root == "" {
		return "", errors.New("asset outside authorized roots")
	}
	target := cap
	if root != cap.root || !contained(cap.logicalRoot, path) {
		target, err = s.capability(root, filepath.Dir(path), cap.settings)
		if err != nil {
			return "", err
		}
	}
	key := target.token + "\x00" + path
	if _, exists := p.assetGrants[key]; !exists && len(p.assetGrants)+len(p.mediaGrants) >= 8192 {
		p.resourceLimit = true
		return "", errors.New("registered asset capacity exceeded")
	}
	if p.assetGrants == nil {
		p.assetGrants = make(map[string]assetGrant)
	}
	p.assetGrants[key] = assetGrant{path: path, mime: kind, download: download}
	return s.documentURL(target, path, "", "", ""), nil
}

// Called under the service mutex after conversion and scratch cleanup succeed.
// Count first so failure cannot publish a partial set of grants.
func (s *previewService) publishGrants(page *httpPage) bool {
	count := len(s.assets) + len(s.media)
	for key := range page.assetGrants {
		if _, exists := s.assets[key]; !exists {
			count++
		}
	}
	for key := range page.mediaGrants {
		if _, exists := s.media[key]; !exists {
			count++
		}
	}
	if count > 8192 {
		return false
	}
	for key, grant := range page.assetGrants {
		s.assets[key] = grant
	}
	for key, grant := range page.mediaGrants {
		s.media[key] = grant
	}
	return true
}

func (s *previewService) localAssetSource(cap *readCapability, name string) (sourceContext, error) {
	canonical, err := canonicalLocation(name)
	if err != nil {
		return sourceContext{}, err
	}
	root := s.effectiveRoot(name, canonical, cap.settings.root)
	if root == "" {
		return sourceContext{}, errors.New("asset outside authorized roots")
	}
	src, err := identify(name, root)
	if err != nil {
		return src, err
	}
	info, err := os.Stat(root)
	if err != nil {
		return src, err
	}
	if src.device != fileDevice(info) {
		return src, errors.New("asset outside authorized filesystem")
	}
	return src, nil
}

func (s *previewService) serveAsset(w http.ResponseWriter, r *http.Request, cap *readCapability, path string) {
	s.mu.Lock()
	grant, exists := s.assets[cap.token+"\x00"+path]
	s.mu.Unlock()
	if !exists {
		publicError(w, r, 404, "Not found")
		return
	}
	src, err := identify(path, cap.root)
	if err != nil {
		publicError(w, r, sourceHTTPStatus(err), "Asset unavailable")
		return
	}
	root, err := os.Stat(cap.root)
	if err != nil || !contained(cap.logicalRoot, path) || src.device != fileDevice(root) {
		publicError(w, r, 403, "Asset outside authorized filesystem")
		return
	}
	data, err := snapshot(src, min(maxAssetBytes, cap.settings.sourceBytes, cap.settings.totalBytes, cap.settings.outputBytes))
	if err != nil {
		publicError(w, r, snapshotStatus(err), "Asset unavailable")
		return
	}
	if grant.download {
		disposition, err := downloadDisposition(filepath.Base(path))
		if err != nil {
			publicError(w, r, 415, "Invalid download filename")
			return
		}
		w.Header().Set("Content-Disposition", disposition)
		publicResponse(w, r, 200, "application/octet-stream", data)
		return
	}
	if rasterSuffix(path) != grant.mime || rasterMIME(data) != grant.mime {
		publicError(w, r, 415, "Raster signature refused")
		return
	}
	publicResponse(w, r, 200, grant.mime, data)
}

func downloadDisposition(name string) (string, error) {
	if strings.ContainsAny(name, "\r\n\x00") {
		return "", errors.New("unsafe download filename")
	}
	ascii := strings.Map(func(r rune) rune {
		if r < 32 || r > 126 {
			return '_'
		}
		return r
	}, name)
	value := mime.FormatMediaType("attachment", map[string]string{"filename": ascii})
	encoded := strings.ReplaceAll(url.QueryEscape(name), "+", "%20")
	return value + "; filename*=UTF-8''" + encoded, nil
}

func (s *previewService) serveMedia(w http.ResponseWriter, r *http.Request, cap *readCapability) {
	s.mu.Lock()
	grant, exists := s.media[r.URL.Path]
	s.mu.Unlock()
	if !exists {
		publicError(w, r, 404, "Not found")
		return
	}
	src, err := identifyFormat(grant.source.logical, cap.root, grant.source.input.reader, s.base.formats)
	if err != nil {
		publicError(w, r, 404, "Media parent unavailable")
		return
	}
	// Preserve code/native classification independently of the original reader field.
	src.input, src.selected = grant.source.input, grant.source.selected
	root, err := os.Stat(cap.root)
	if err != nil || src.canonical != grant.source.canonical || !contained(cap.logicalRoot, src.logical) || src.device != fileDevice(root) {
		publicError(w, r, 404, "Media parent unavailable")
		return
	}
	input, err := snapshot(src, min(cap.settings.sourceBytes, cap.settings.totalBytes))
	if err != nil || digestText(input) != grant.revision {
		publicError(w, r, 404, "Media parent revision changed")
		return
	}
	page, status := s.currentPage(r.Context(), cap, src)
	if status != 200 {
		publicError(w, r, status, "Media conversion unavailable")
		return
	}
	asset, exists := page.media[grant.id]
	if !exists || page.revision != grant.revision {
		publicError(w, r, 404, "Media unavailable")
		return
	}
	if rasterMIME(asset.data) != asset.mime {
		publicError(w, r, 415, "Raster signature refused")
		return
	}
	publicResponse(w, r, 200, asset.mime, asset.data)
}
