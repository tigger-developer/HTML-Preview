// ABOUTME: Defines and validates the versioned annotation event contract.
// ABOUTME: Keeps durable record and selector rules independent of HTTP and browser state.
package annotation

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const MaxStore = 10 * 1024 * 1024
const MaxEvents = 10000
const MaxFrame = 64 * 1024

type Header struct {
	Schema       int    `json:"schema"`
	DocumentID   string `json:"document_id"`
	SourceFormat string `json:"source_format"`
}

type Target struct {
	BlockID      string `json:"block_id,omitempty"`
	AfterBlock   bool   `json:"-"`
	Type         string `json:"type"`
	BodyRevision string `json:"body_revision,omitempty"`
	Exact        string `json:"exact,omitempty"`
	Prefix       string `json:"prefix,omitempty"`
	Suffix       string `json:"suffix,omitempty"`
	Start        int    `json:"start,omitempty"`
	End          int    `json:"end,omitempty"`
	HeadingID    string `json:"heading_id,omitempty"`
	Position     int    `json:"position,omitempty"`
	Run          string `json:"run,omitempty"`
	RunOffset    int    `json:"run_offset,omitempty"`
	AfterLink    bool   `json:"after_link,omitempty"`
}

type Event struct {
	Schema       int    `json:"schema"`
	OperationID  string `json:"operation_id"`
	AnnotationID string `json:"annotation_id"`
	ComposerID   string `json:"composer_id"`
	Sequence     int    `json:"sequence"`
	Kind         string `json:"kind"`
	Author       string `json:"author"`
	CreatedAt    string `json:"created_at"`
	RecordedAt   string `json:"recorded_at"`
	Target       Target `json:"target"`
	Text         string `json:"text"`
	Label        string `json:"label,omitempty"`
}

// Request contains only client-owned event fields; identity and time are server-owned.
type Request struct {
	OperationID    string `json:"operation_id"`
	AnnotationID   string `json:"annotation_id"`
	ComposerID     string `json:"composer_id"`
	Sequence       int    `json:"sequence"`
	Revision       string `json:"revision"`
	SourceRevision string `json:"source_revision"`
	BodyRevision   string `json:"body_revision"`
	Kind           string `json:"kind"`
	Target         Target `json:"target"`
	Text           string `json:"text"`
	Label          string `json:"label,omitempty"`
	Action         string `json:"action,omitempty"`
}

var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func ValidID(value string) bool { return uuidPattern.MatchString(value) }

func NewID() (string, error) {
	var data [16]byte
	if _, err := rand.Read(data[:]); err != nil {
		return "", err
	}
	data[6] = (data[6] & 15) | 64
	data[8] = (data[8] & 63) | 128
	h := hex.EncodeToString(data[:])
	return h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:], nil
}

func Digest(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }

func ValidateName(name string) error {
	if !utf8.ValidString(name) || len(name) > 512 || utf8.RuneCountInString(name) > 128 {
		return errors.New("display name exceeds UTF-8 limits")
	}
	for _, r := range name {
		if unicode.IsControl(r) || r == '\u2028' || r == '\u2029' {
			return errors.New("display name contains controls or line breaks")
		}
	}
	return nil
}

func ValidText(text string) bool {
	return utf8.ValidString(text) && strings.TrimSpace(text) != "" && !strings.ContainsRune(text, 0) && len(text) <= 16384 && utf8.RuneCountInString(text) <= 4000
}

func (t Target) Validate() error {
	if t.Type == "document" {
		if t != (Target{Type: "document"}) {
			return errors.New("document target has text selector fields")
		}
		return nil
	}
	if t.Type != "text" || t.Exact == "" || len(t.Exact) > 8192 || t.Start < 0 || t.End <= t.Start || len(t.BodyRevision) != 64 || len(t.HeadingID) > 4096 {
		return errors.New("invalid text selector")
	}
	for _, value := range []string{t.Exact, t.Prefix, t.Suffix, t.HeadingID} {
		if !utf8.ValidString(value) || strings.ContainsRune(value, 0) {
			return errors.New("invalid selector text")
		}
	}
	if utf8.RuneCountInString(t.Prefix) > 64 || utf8.RuneCountInString(t.Suffix) > 64 {
		return errors.New("selector context exceeds limit")
	}
	return nil
}

func (r Request) Validate() error {
	if len(r.Text) > 16384 || utf8.RuneCountInString(r.Text) > 4000 || len(r.Target.Exact) > 8192 || utf8.RuneCountInString(r.Target.Prefix) > 64 || utf8.RuneCountInString(r.Target.Suffix) > 64 || len(r.Target.HeadingID) > 4096 {
		return fail("body_limit")
	}
	if !ValidID(r.OperationID) || !ValidID(r.AnnotationID) || !ValidID(r.ComposerID) || r.Sequence < 1 || r.Sequence > MaxEvents || (r.Kind != "draft" && r.Kind != "close") || !ValidText(r.Text) {
		return fail("invalid_event")
	}
	for _, revision := range []string{r.Revision, r.SourceRevision, r.BodyRevision} {
		if len(revision) != 64 {
			return fail("invalid_event")
		}
		if _, err := hex.DecodeString(revision); err != nil {
			return fail("invalid_event")
		}
	}
	if r.Target.Validate() != nil {
		return fail("invalid_event")
	}
	return nil
}

func (e Event) Validate() error {
	if e.Schema != 1 || !ValidID(e.OperationID) || !ValidID(e.AnnotationID) || !ValidID(e.ComposerID) || e.Sequence < 1 || e.Sequence > MaxEvents || (e.Kind != "draft" && e.Kind != "close") || !ValidText(e.Text) || strings.TrimSpace(e.Author) == "" {
		return errors.New("invalid stored event")
	}
	if err := ValidateName(e.Author); err != nil {
		return err
	}
	for _, value := range []string{e.CreatedAt, e.RecordedAt} {
		if _, err := time.Parse(time.RFC3339Nano, value); err != nil || !strings.HasSuffix(value, "Z") {
			return errors.New("invalid event timestamp")
		}
	}
	return e.Target.Validate()
}

// Decode rejects duplicate keys, extra fields, invalid UTF-8 and trailing JSON.
func Decode(data []byte, value any) error {
	if !utf8.Valid(data) {
		return errors.New("invalid UTF-8 JSON")
	}
	d := json.NewDecoder(bytes.NewReader(data))
	if err := jsonValue(d, 0); err != nil {
		return err
	}
	if _, err := d.Token(); !errors.Is(err, io.EOF) {
		return errors.New("trailing JSON")
	}
	d = json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	return d.Decode(value)
}

func jsonValue(d *json.Decoder, depth int) error {
	if depth > 8 {
		return errors.New("JSON nesting limit")
	}
	token, err := d.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	seen := make(map[string]bool)
	for d.More() {
		if delim == '{' {
			key, err := d.Token()
			name, ok := key.(string)
			if err != nil || !ok || seen[name] {
				return errors.New("duplicate JSON key")
			}
			seen[name] = true
		}
		if err := jsonValue(d, depth+1); err != nil {
			return err
		}
	}
	_, err = d.Token()
	return err
}

func SameTarget(a, b Target) bool {
	return a.Type == b.Type && a.Exact == b.Exact && a.HeadingID == b.HeadingID
}

func RequestMatches(r Request, e Event) bool {
	return r.OperationID == e.OperationID && r.AnnotationID == e.AnnotationID && r.ComposerID == e.ComposerID && r.Sequence == e.Sequence && r.Kind == e.Kind && r.Target == e.Target && r.Text == e.Text
}

func ValidateNext(previous *Event, next Event) error {
	if err := next.Validate(); err != nil {
		return err
	}
	if previous == nil {
		if next.Sequence != 1 || next.Kind != "draft" {
			return errors.New("initial event must be draft sequence one")
		}
		return nil
	}
	if previous.Kind == "close" || next.Sequence != previous.Sequence+1 || next.Author != previous.Author || next.CreatedAt != previous.CreatedAt || next.ComposerID != previous.ComposerID || !SameTarget(next.Target, previous.Target) {
		return errors.New("event sequence or immutable identity conflict")
	}
	if next.Kind == "close" && (next.Text != previous.Text || next.Target != previous.Target) {
		return errors.New("close must repeat acknowledged draft")
	}
	return nil
}

func HeaderValid(h Header) bool {
	return h.Schema == 1 && ValidID(h.DocumentID) && (h.SourceFormat == "org" || h.SourceFormat == "markdown")
}

func Frame(format, marker string, value any, ending string) ([]byte, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	data = bytes.ReplaceAll(data, []byte("-"), []byte(`\u002d`))
	var frame string
	if format == "org" {
		frame = "#+begin_comment" + ending + marker + ending + string(data) + ending + "#+end_comment" + ending
	} else {
		frame = "<!-- " + marker + ending + string(data) + ending + "-->" + ending
	}
	if len(frame) > MaxFrame {
		return nil, fail("body_limit")
	}
	return []byte(frame), nil
}
