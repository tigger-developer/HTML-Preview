// ABOUTME: Runs a bounded, same-executable Org converter protocol.
// ABOUTME: Keeps parser failures and memory pressure inside one disposable process.
package orgconvert

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"runtime"
	"runtime/debug"
	"sync/atomic"
	"time"
	"unicode/utf8"
)

const (
	MaxInputBytes  int64 = 100 << 20
	MaxOutputBytes int64 = 50 << 20
	MaxMemoryBytes int64 = 512 << 20
	LimitExit            = 3
)

// Request contains only authorized intermediary bytes and private conversion settings.
type Request struct {
	Version     int      `json:"version"`
	Text        string   `json:"text"`
	Token       string   `json:"token"`
	TOC         bool     `json:"toc"`
	TOCDepth    int      `json:"toc_depth"`
	InputBytes  int64    `json:"input_bytes"`
	OutputBytes int64    `json:"output_bytes"`
	MemoryBytes int64    `json:"memory_bytes"`
	ProbeLabels []string `json:"probe_labels,omitempty"`
}

type conversionResult struct {
	output []byte
	err    error
}

var errLimit = errors.New("Org conversion resource limit exceeded")
var tokenPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)
var labelPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// Worker is a process entry point: the caller must exit with its returned status.
// Returning after a memory abort terminates the in-flight library conversion too.
func Worker(input io.Reader, output, diagnostic io.Writer) int {
	debug.SetMemoryLimit(MaxMemoryBytes)
	var ceiling atomic.Int64
	ceiling.Store(MaxMemoryBytes)
	result := make(chan conversionResult, 1)
	go func() {
		defer func() {
			if recover() != nil {
				result <- conversionResult{err: errors.New("Org conversion failed")}
			}
		}()
		request, err := readRequest(input)
		if err != nil {
			result <- conversionResult{err: err}
			return
		}
		ceiling.Store(request.MemoryBytes)
		debug.SetMemoryLimit(request.MemoryBytes)
		if memoryUsed() > request.MemoryBytes {
			result <- conversionResult{err: fmt.Errorf("memory limit: %w", errLimit)}
			return
		}
		data, err := convert(request)
		if err == nil && int64(len(data)) > request.OutputBytes {
			err = errLimit
		}
		result <- conversionResult{output: data, err: err}
	}()
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-tick.C:
			if memoryUsed() > ceiling.Load() {
				return workerFailure(diagnostic, fmt.Errorf("memory limit: %w", errLimit))
			}
		case r := <-result:
			if memoryUsed() > ceiling.Load() {
				return workerFailure(diagnostic, fmt.Errorf("memory limit: %w", errLimit))
			}
			if r.err != nil {
				return workerFailure(diagnostic, r.err)
			}
			if _, err := output.Write(r.output); err != nil {
				return 1
			}
			return 0
		}
	}
}
func memoryUsed() int64 {
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	return int64(stats.Sys - stats.HeapReleased) // #nosec G115 -- Go runtime accounting is bounded far below int64.
}
func workerFailure(w io.Writer, err error) int {
	message, code := "Org conversion failed", 1
	if errors.Is(err, errLimit) {
		message, code = "Org conversion resource limit exceeded", LimitExit
	}
	if err != nil && bytes.Contains([]byte(err.Error()), []byte("memory limit")) {
		message = "Org conversion memory limit exceeded"
	}
	if _, writeErr := fmt.Fprintln(w, message); writeErr != nil {
		return 1
	}
	return code
}
func readRequest(input io.Reader) (Request, error) {
	var r Request
	data, err := io.ReadAll(io.LimitReader(input, MaxInputBytes+1))
	if err != nil {
		return r, errors.New("request read failed")
	}
	if int64(len(data)) > MaxInputBytes {
		return r, errLimit
	}
	if !utf8.Valid(data) {
		return r, errors.New("invalid encoding")
	}
	if err = uniqueObject(data); err != nil {
		return r, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&r); err != nil {
		return r, errors.New("invalid request")
	}
	if r.Version != 1 || !tokenPattern.MatchString(r.Token) || r.TOCDepth < 1 || r.TOCDepth > 6 {
		return r, errors.New("invalid request settings")
	}
	if r.InputBytes <= 0 || r.InputBytes > MaxInputBytes || r.OutputBytes <= 0 || r.OutputBytes > MaxOutputBytes || r.MemoryBytes <= 0 || r.MemoryBytes > MaxMemoryBytes {
		return r, errors.New("invalid resource allowance")
	}
	if int64(len(data)) > r.InputBytes {
		return r, errLimit
	}
	seen := make(map[string]bool)
	for _, label := range r.ProbeLabels {
		if !labelPattern.MatchString(label) || seen[label] {
			return r, errors.New("invalid probe label")
		}
		seen[label] = true
	}
	return r, nil
}

// uniqueObject rejects duplicate keys and trailing JSON before typed decoding.
func uniqueObject(data []byte) error {
	d := json.NewDecoder(bytes.NewReader(data))
	token, err := d.Token()
	if err != nil || token != json.Delim('{') {
		return errors.New("invalid object")
	}
	seen := make(map[string]bool)
	for d.More() {
		token, err = d.Token()
		key, ok := token.(string)
		if err != nil || !ok || seen[key] {
			return errors.New("duplicate or invalid field")
		}
		seen[key] = true
		var value json.RawMessage
		if err = d.Decode(&value); err != nil {
			return errors.New("invalid value")
		}
	}
	if _, err = d.Token(); err != nil {
		return errors.New("invalid object end")
	}
	if _, err = d.Token(); err != io.EOF {
		return errors.New("trailing data")
	}
	return nil
}
