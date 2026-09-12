// ABOUTME: Admits container-owned and rooted local rasters using bounded signatures.
// ABOUTME: Exports validated bytes as inert data URLs independent of the source location.
package preview

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path"
	"path/filepath"
	"strings"

	"golang.org/x/net/html"
)

func rasterMIME(data []byte) string {
	switch {
	case bytes.HasPrefix(data, []byte{137, 80, 78, 71, 13, 10, 26, 10}):
		return "image/png"
	case bytes.HasPrefix(data, []byte{255, 216, 255}):
		return "image/jpeg"
	case bytes.HasPrefix(data, []byte("GIF87a")) || bytes.HasPrefix(data, []byte("GIF89a")):
		return "image/gif"
	case len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP":
		return "image/webp"
	case len(data) >= 16 && string(data[4:8]) == "ftyp":
		size := int(binary.BigEndian.Uint32(data[:4]))
		if size < 16 || size > 4096 || size > len(data) || size%4 != 0 {
			return ""
		}
		for offset := 8; offset < size; offset += 4 {
			if offset != 12 && (string(data[offset:offset+4]) == "avif" || string(data[offset:offset+4]) == "avis") {
				return "image/avif"
			}
		}
	}
	return ""
}

func rasterURL(data []byte, name, declared string) (string, error) {
	mime := rasterMIME(data)
	if len(data) > int(maxAssetBytes) || mime == "" || (declared != "" && mime != declared) {
		return "", errors.New("image has an unsupported, truncated or conflicting raster header")
	}
	if name != "" {
		if rasterSuffix(name) != mime {
			return "", errors.New("image suffix conflicts with its raster header")
		}
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data), nil
}

func rasterSuffix(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".avif":
		return "image/avif"
	}
	return ""
}

func (s *session) restoreMedia(doc *html.Node, p *page, token string) error {
	var record *html.Node
	for n := range doc.Descendants() {
		if attribute(n, "id") == "htmlpreview-media-"+token {
			if record != nil || n.Data != "pre" {
				return errors.New("invalid sandbox-safe media export record")
			}
			record = n
		}
	}
	if record == nil {
		return errors.New("Pandoc dependency lacks sandbox-safe media export; no preview published")
	}
	var result struct {
		Status string `json:"status"`
		Items  []struct {
			Name string `json:"name"`
			MIME string `json:"mime"`
			Hex  string `json:"hex"`
		} `json:"items"`
	}
	decoder := json.NewDecoder(strings.NewReader(contentText(record)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return fmt.Errorf("invalid sandbox-safe media export: %w", err)
	}
	if result.Status != "ok" || len(result.Items) > maxArchiveEntries {
		return errors.New("Pandoc sandbox-safe media export unavailable or over budget; no preview published")
	}
	p.media = make(map[string]string)
	for _, item := range result.Items {
		name := path.Clean(item.Name)
		if name == "." || name == ".." || strings.HasPrefix(name, "../") || path.IsAbs(name) || strings.ContainsAny(item.Name, "\\:\x00") {
			return errors.New("container media has an unsafe name")
		}
		if _, exists := p.media[name]; exists {
			return errors.New("container media has duplicate normalized names")
		}
		if int64(len(item.Hex)) > 2*min(maxAssetBytes, s.cfg.sourceBytes, s.cfg.outputBytes-s.used) {
			return errors.New("container media exceeds remaining byte budget")
		}
		data, err := hex.DecodeString(item.Hex)
		if err != nil {
			return fmt.Errorf("invalid container media encoding: %w", err)
		}
		s.used += int64(len(data))
		value, err := rasterURL(data, "", item.MIME)
		if err != nil {
			s.log.notice("%q: container image omitted: %v", p.source.logical, err)
		}
		p.media[name] = value
	}
	record.Parent.RemoveChild(record)
	return nil
}

func (s *session) localAsset(p *page, name string) ([]byte, error) {
	src, err := identify(name, p.source.root)
	if err != nil {
		return nil, err
	}
	if src.device != p.source.device {
		return nil, errors.New("asset is on another filesystem")
	}
	limit := min(maxAssetBytes, s.cfg.sourceBytes, s.cfg.totalBytes-s.sourceUsed, s.cfg.outputBytes-s.used)
	data, err := snapshot(src, limit)
	if err != nil {
		return nil, err
	}
	s.sourceUsed += int64(len(data))
	s.used += int64(len(data))
	return data, nil
}

func (s *session) imageURL(p *page, value, parent string) (string, error) {
	if p.images == nil {
		p.images = make(map[string]string)
	}
	key := parent + "\x00" + value
	if cached, ok := p.images[key]; ok {
		return cached, nil
	}
	if media, ok := p.media[path.Clean(value)]; ok {
		if media == "" {
			return "", errors.New("container image omitted by raster policy")
		}
		return media, nil
	}
	var data []byte
	var name, mime string
	if strings.HasPrefix(value, "data:") {
		header, payload, ok := strings.Cut(strings.TrimPrefix(value, "data:"), ",")
		if !ok {
			return "", errors.New("malformed image data URL")
		}
		var encoded bool
		mime, encoded = strings.CutSuffix(header, ";base64")
		if !encoded || int64(len(payload)) > 4*(min(maxAssetBytes, s.cfg.sourceBytes, s.cfg.outputBytes-s.used)+2)/3 {
			return "", errors.New("image data URL exceeds policy or byte budget")
		}
		var err error
		data, err = base64.StdEncoding.DecodeString(payload)
		if err != nil {
			return "", errors.New("malformed image base64")
		}
		s.used += int64(len(data))
	} else {
		r, err := parseReference(value, parent)
		if err != nil || !r.local {
			return "", errors.New("automatic non-local image omitted")
		}
		name = r.path
		data, err = s.localAsset(p, name)
		if err != nil {
			return "", fmt.Errorf("image omitted: %w", err)
		}
	}
	result, err := rasterURL(data, name, mime)
	if err != nil {
		return "", err
	}
	p.images[key] = result
	// Repeated publication passes recognize the already-owned representation.
	p.images[parent+"\x00"+result] = result
	return result, nil
}

func fileReference(name string) string { return (&url.URL{Scheme: "file", Path: name}).String() }
