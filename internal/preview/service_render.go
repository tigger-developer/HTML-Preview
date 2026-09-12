// ABOUTME: Adapts the owned renderer to one authorized HTTP source snapshot.
// ABOUTME: Rewrites local document links without eagerly converting their targets.
package preview

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/net/html"
)

func (service *previewService) renderHTTP(ctx context.Context, cap *readCapability, src sourceContext, lookup *orgLookup) ([]byte, int) {
	result, status := service.currentPage(ctx, cap, src)
	if status != 200 {
		return nil, status
	}
	if lookup == nil {
		return result.data, 200
	}
	lookup.fragment = catalogueAnchor(result.ids, result.headings, lookup.search)
	if lookup.fragment != "" {
		return result.data, 200
	}
	doc, err := html.Parse(bytes.NewReader(result.data))
	if err != nil {
		return nil, 422
	}
	notice := &html.Node{Type: html.ElementNode, Data: "p"}
	notice.AppendChild(nodeText("Org search not resolved: " + lookup.search))
	body := element(doc, "body")
	body.InsertBefore(notice, body.FirstChild)
	var output bytes.Buffer
	if err = html.Render(&output, doc); err != nil {
		return nil, 422
	}
	if int64(output.Len()) > cap.settings.outputBytes {
		return nil, 413
	}
	return output.Bytes(), 200
}

func (service *previewService) buildHTTP(ctx context.Context, cap *readCapability, src sourceContext, input []byte) (result *httpPage, status int) {
	path, err := service.host.Temp()
	if err != nil {
		return nil, 503
	}
	defer func() {
		if err := service.host.Remove(path); err != nil {
			result = nil
			status = 503
		}
	}()
	cfg := cap.settings
	cfg.httpOrigin = service.origin
	s := &session{path: path, pandoc: service.pandoc, host: service.host, cfg: cfg, log: &console{out: io.Discard, diagnostics: io.Discard}, byKey: make(map[string]*page), sourceUsed: int64(len(input))}
	revision := digestText(input)
	s.imageResolver = func(p *page, name, kind string, data []byte) (string, error) {
		return service.registerImage(cap, p, revision, name, kind, data)
	}
	s.assetSource = func(name string) (sourceContext, error) {
		return service.localAssetSource(cap, name)
	}
	if err = s.extract(); err != nil {
		return nil, 503
	}
	p := s.admit(src)
	p.url = service.documentURL(cap, src.logical, "", "", "")
	if err = s.render(ctx, p, input); err != nil {
		return nil, renderStatus(ctx, err)
	}
	if err = service.resolveHTTP(ctx, s, p, cap); err != nil {
		return nil, renderStatus(ctx, err)
	}
	if err = ctx.Err(); err != nil {
		return nil, 504
	}
	if p.resourceLimit {
		return nil, 413
	}
	data, err := s.document(p)
	if err != nil {
		return nil, 422
	}
	if int64(len(data)) > cfg.outputBytes-s.used {
		return nil, 413
	}
	return &httpPage{data: data, source: src, ids: p.ids, headings: p.headings, orgIDs: p.orgIDs, dependencies: p.dependencies, cacheable: !p.uncacheable, media: p.httpMedia, assetGrants: p.assetGrants, mediaGrants: p.mediaGrants}, 200
}
func renderStatus(ctx context.Context, err error) int {
	if ctx.Err() != nil || errors.Is(err, context.DeadlineExceeded) {
		return 504
	}
	if strings.Contains(err.Error(), "limit") || strings.Contains(err.Error(), "exhausted") {
		return 413
	}
	if strings.Contains(err.Error(), "dependency") {
		return 503
	}
	return 422
}

func (service *previewService) resolveHTTP(ctx context.Context, s *session, p *page, cap *readCapability) error {
	var refs []*html.Node
	for n := range p.dom.Descendants() {
		if n.Type == html.ElementNode && (n.Data == "a" || n.Data == "img") {
			refs = append(refs, n)
		}
	}
	for _, n := range refs {
		if err := ctx.Err(); err != nil {
			return err
		}
		if n.Data == "img" {
			value, err := s.imageURL(p, attribute(n, "src"), filepath.Dir(p.source.logical))
			if err != nil {
				s.inactive(p, n, "src", "Image unavailable within authorized roots")
			} else {
				setAttribute(n, "src", value)
			}
			continue
		}
		value := attribute(n, "href")
		if value == "" {
			continue
		}
		r, err := parseReference(value, filepath.Dir(p.source.logical))
		if err != nil {
			s.inactive(p, n, "href", "Unsupported reference")
			continue
		}
		if !r.local {
			if r.id != "" {
				s.inactive(p, n, "href", "Unknown or ambiguous Org ID")
			}
			if r.url != nil && (r.url.Scheme == "data" || strings.HasPrefix(value, service.origin+"/")) {
				s.inactive(p, n, "href", "Source-authored service or data URL is not authorized")
			}
			continue
		}
		canonical, err := canonicalLocation(r.path)
		if err != nil {
			s.inactive(p, n, "href", "Target unavailable")
			continue
		}
		root := service.effectiveRoot(r.path, canonical, cap.settings.root)
		if root == "" {
			s.inactive(p, n, "href", "Target outside authorized roots")
			continue
		}
		from := ""
		if r.url != nil {
			from = r.url.Query().Get("htmlpreview-format")
		}
		if _, err := service.base.formats.resolve(r.path, from); err != nil {
			kind, download := linkedAssetType(r.path)
			if from == "" && kind != "" {
				value, err := service.registerAsset(cap, p, r.path, kind, download)
				if err == nil {
					setAttribute(n, "href", value)
					continue
				}
			}
			s.inactive(p, n, "href", "Unsupported linked format")
			continue
		}
		target, err := service.capability(root, filepath.Dir(r.path), cap.settings)
		if err != nil {
			s.inactive(p, n, "href", "Preview capacity exceeded")
			continue
		}
		query, fragment := "", ""
		if r.url != nil {
			query = r.url.RawQuery
			fragment = r.url.Fragment
		}
		if r.search != "" {
			rel, err := filepath.Rel(target.logicalRoot, r.path)
			if err != nil {
				return err
			}
			values := url.Values{"path": []string{filepath.ToSlash(rel)}, "search": []string{r.search}}
			if from != "" {
				values.Set("htmlpreview-format", from)
			}
			setAttribute(n, "href", service.origin+"/"+target.token+"/_org?"+values.Encode())
		} else {
			setAttribute(n, "href", service.documentURL(target, r.path, "", query, fragment))
		}
	}
	return nil
}

func canonicalLocation(path string) (string, error) {
	for existing := path; ; existing = filepath.Dir(existing) {
		canonical, err := filepath.EvalSymlinks(existing)
		if err == nil {
			rel, err := filepath.Rel(existing, path)
			if err != nil {
				return "", err
			}
			return filepath.Join(canonical, rel), nil
		}
		if !os.IsNotExist(err) || existing == filepath.Dir(existing) {
			return "", err
		}
	}
}
