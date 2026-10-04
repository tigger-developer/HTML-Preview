// ABOUTME: Converts bounded local SVG image references into verified PNG bytes.
// ABOUTME: Uses stdin without a base URL so source SVG cannot load local or remote files.
package preview

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
)

const (
	maxSVGDimension = 4096
	maxSVGArea      = 4096 * 4096
	minRsvgMajor    = 2
	minRsvgMinor    = 57
)

func svgLength(value string) (float64, error) {
	value = strings.TrimSpace(value)
	value = strings.TrimSuffix(value, "px")
	result, err := strconv.ParseFloat(value, 64)
	if err != nil || math.IsNaN(result) || math.IsInf(result, 0) || result <= 0 {
		return 0, errors.New("SVG dimensions must be positive numbers or px lengths")
	}
	return result, nil
}

func svgViewBox(value string) (float64, float64, error) {
	fields := strings.Fields(strings.ReplaceAll(value, ",", " "))
	if len(fields) != 4 {
		return 0, 0, errors.New("SVG viewBox must contain four numbers")
	}
	width, err := svgLength(fields[2])
	if err != nil {
		return 0, 0, err
	}
	height, err := svgLength(fields[3])
	return width, height, err
}

func svgRasterDimensions(data []byte) (int, int, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			return 0, 0, errors.New("SVG root element is missing")
		}
		if err != nil {
			return 0, 0, fmt.Errorf("parse SVG dimensions: %w", err)
		}
		switch value := token.(type) {
		case xml.Directive:
			return 0, 0, errors.New("SVG document type declarations are unsupported")
		case xml.StartElement:
			if value.Name.Local != "svg" || value.Name.Space != "http://www.w3.org/2000/svg" {
				return 0, 0, errors.New("image is not an SVG document")
			}
			var width, height, viewWidth, viewHeight float64
			for _, attribute := range value.Attr {
				switch attribute.Name.Local {
				case "width":
					width, err = svgLength(attribute.Value)
				case "height":
					height, err = svgLength(attribute.Value)
				case "viewBox":
					viewWidth, viewHeight, err = svgViewBox(attribute.Value)
				}
				if err != nil {
					return 0, 0, err
				}
			}
			if width == 0 && height == 0 {
				width, height = viewWidth, viewHeight
			} else if width == 0 && viewWidth > 0 && viewHeight > 0 {
				width = height * viewWidth / viewHeight
			} else if height == 0 && viewWidth > 0 && viewHeight > 0 {
				height = width * viewHeight / viewWidth
			}
			w, h := int(math.Ceil(width)), int(math.Ceil(height))
			if w <= 0 || h <= 0 || w > maxSVGDimension || h > maxSVGDimension || w*h > maxSVGArea {
				return 0, 0, fmt.Errorf("SVG dimensions exceed %dx%d raster limit", maxSVGDimension, maxSVGDimension)
			}
			return w, h, nil
		}
	}
}

func checkSVGConverter(ctx context.Context, host Host, path string) error {
	data, err := helper(ctx, host, path, []string{"--version"}, nil)
	if err != nil {
		return err
	}
	var major, minor, patch int
	if _, err = fmt.Sscanf(string(data), "rsvg-convert version %d.%d.%d", &major, &minor, &patch); err != nil || major < minRsvgMajor || (major == minRsvgMajor && minor < minRsvgMinor) {
		return fmt.Errorf("requires rsvg-convert %d.%d.0 or later", minRsvgMajor, minRsvgMinor)
	}
	return nil
}

func (s *session) svgConverter(ctx context.Context) (string, error) {
	if s.svgChecked {
		if s.svg == "" {
			return "", errors.New("SVG images require rsvg-convert 2.57.0 or later")
		}
		return s.svg, nil
	}
	s.svgChecked = true
	path, err := s.host.LookPath("rsvg-convert")
	if err != nil {
		return "", fmt.Errorf("SVG images require rsvg-convert 2.57.0 or later: %w", err)
	}
	if err = checkSVGConverter(ctx, s.host, path); err != nil {
		return "", fmt.Errorf("SVG converter compatibility: %w", err)
	}
	s.svg = path
	return path, nil
}

func (s *session) rasterizeSVG(ctx context.Context, data []byte) ([]byte, error) {
	width, height, err := svgRasterDimensions(data)
	if err != nil {
		return nil, err
	}
	converter, err := s.svgConverter(ctx)
	if err != nil {
		return nil, err
	}
	limit := min(maxAssetBytes, s.cfg.outputBytes-s.used)
	if limit <= 0 {
		return nil, errors.New("SVG raster output budget exhausted")
	}
	output, err := s.host.Execute(ctx, Command{
		Path:  converter,
		Args:  []string{"--format=png", fmt.Sprintf("--width=%d", width), fmt.Sprintf("--height=%d", height), "-"},
		Input: data,
		Dir:   s.path,
		Limit: limit,
	})
	if err != nil {
		return nil, fmt.Errorf("rasterize SVG: %w", err)
	}
	if rasterMIME(output) != "image/png" {
		return nil, errors.New("SVG converter returned invalid PNG data")
	}
	s.used += int64(len(output))
	return output, nil
}
