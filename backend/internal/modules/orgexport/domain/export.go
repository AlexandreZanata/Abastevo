// Package domain defines organization export jobs (EMP-RPT-001): the closed
// lifecycle, failure codes, ceilings, canonical parameters, the bounded sink
// a format writes through and the private object identity. It performs no I/O.
package domain

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"regexp"
	"slices"
	"time"
	"unicode/utf8"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
)

var (
	ErrInvalid   = errors.New("orgexport: invalid export input")
	ErrTooLarge  = errors.New("orgexport: export ceiling exceeded")
	ErrIntegrity = errors.New("orgexport: stored file does not match")
)

// Infrastructure ceilings and defaults (EDR-156). A format may lower a
// ceiling, never raise it.
const (
	Permission              = "rpt.export.run"
	MaxRows           int64 = 500_000
	MaxBytes          int64 = 16 << 20
	MinDuration             = time.Second
	MaxDuration             = 120 * time.Second
	DefaultRetention        = 7 * 24 * time.Hour
	MinRetention            = 24 * time.Hour
	MaxRetention            = 30 * 24 * time.Hour
	MaxAttempts             = 3
	OpenJobLimit            = 3
	MaxParameterBytes       = 4096
	CapabilityTTL           = 5 * time.Minute
	maxParameterDepth       = 16
)

var (
	kindPattern  = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$`)
	mediaPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9!#$&^_.+-]*/[a-z0-9][a-z0-9!#$&^_.+-]*$`)
	keyPattern   = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	hashPattern  = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// State is the export job lifecycle of 08-state-model.
type State string

const (
	Queued  State = "QUEUED"
	Running State = "RUNNING"
	Ready   State = "READY"
	Expired State = "EXPIRED"
	Failed  State = "FAILED"
)

func (s State) Valid() bool {
	switch s {
	case Queued, Running, Ready, Expired, Failed:
		return true
	}
	return false
}

func (s State) Terminal() bool { return s == Expired || s == Failed }

// Open reports a job that still counts against the requester's open-job limit.
func (s State) Open() bool { return s == Queued || s == Running }

var transitions = map[State][]State{
	Queued:  {Running, Failed},
	Running: {Queued, Ready, Failed},
	Ready:   {Expired},
}

// CanTransition is the closed transition table. RUNNING→QUEUED is only a
// fenced lease release; READY→EXPIRED happens at retention.
func CanTransition(from, to State) bool {
	return to.Valid() && slices.Contains(transitions[from], to)
}

// FailureCode is the closed reason recorded on a FAILED job.
type FailureCode string

const (
	TooLarge          FailureCode = "EXPORT_TOO_LARGE"
	RightsLost        FailureCode = "EXPORT_RIGHTS_LOST"
	FormatUnavailable FailureCode = "EXPORT_FORMAT_UNAVAILABLE"
	AttemptsExhausted FailureCode = "EXPORT_ATTEMPTS_EXHAUSTED"
)

func (c FailureCode) Valid() bool {
	switch c {
	case TooLarge, RightsLost, FormatUnavailable, AttemptsExhausted:
		return true
	}
	return false
}

// ValidID checks canonical non-nil UUID syntax.
func ValidID(raw string) bool {
	id, err := kernel.ParseOrgID(raw)
	return err == nil && id.String() == raw
}

// ValidKind accepts a registered-code shaped kind of at most 64 bytes.
func ValidKind(kind string) bool { return len(kind) <= 64 && kindPattern.MatchString(kind) }

// ValidMediaType accepts a bare lower-case type/subtype without parameters.
func ValidMediaType(media string) bool { return len(media) <= 128 && mediaPattern.MatchString(media) }

// ValidSHA256 accepts a lower-case hexadecimal SHA-256 digest.
func ValidSHA256(digest string) bool { return hashPattern.MatchString(digest) }

// Limits are the per-job ceilings recorded at request.
type Limits struct {
	Rows     int64
	Bytes    int64
	Duration time.Duration
}

func DefaultLimits() Limits { return Limits{Rows: MaxRows, Bytes: MaxBytes, Duration: MaxDuration} }

// Validate requires positive ceilings within the infrastructure ceilings and
// a whole number of milliseconds.
func (l Limits) Validate() error {
	if l.Rows < 1 || l.Rows > MaxRows || l.Bytes < 1 || l.Bytes > MaxBytes ||
		l.Duration < MinDuration || l.Duration > MaxDuration || l.Duration%time.Millisecond != 0 {
		return ErrInvalid
	}
	return nil
}

// ValidRetention accepts whole seconds from one to thirty days.
func ValidRetention(d time.Duration) bool {
	return d >= MinRetention && d <= MaxRetention && d%time.Second == 0
}

// CanonicalParameters re-encodes a JSON object with sorted lower-snake keys,
// exact number text and no null, duplicate key, lossy string or trailing
// input. The canonical form is at most MaxParameterBytes.
func CanonicalParameters(raw []byte) ([]byte, error) {
	if len(raw) > 4*MaxParameterBytes || !lossless(raw) {
		return nil, ErrInvalid
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	value, err := strictValue(decoder, 0)
	if err != nil {
		return nil, ErrInvalid
	}
	if _, isObject := value.(map[string]any); !isObject {
		return nil, ErrInvalid
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, ErrInvalid
	}
	canonical, err := json.Marshal(value)
	if err != nil || len(canonical) > MaxParameterBytes {
		return nil, ErrInvalid
	}
	return canonical, nil
}

func strictValue(decoder *json.Decoder, depth int) (any, error) {
	if depth > maxParameterDepth {
		return nil, ErrInvalid
	}
	token, err := decoder.Token()
	if err != nil {
		return nil, ErrInvalid
	}
	switch v := token.(type) {
	case json.Delim:
		switch v {
		case '{':
			object := map[string]any{}
			for decoder.More() {
				keyToken, err := decoder.Token()
				key, isString := keyToken.(string)
				if err != nil || !isString || !keyPattern.MatchString(key) || len(key) > 64 {
					return nil, ErrInvalid
				}
				if _, duplicate := object[key]; duplicate {
					return nil, ErrInvalid
				}
				if object[key], err = strictValue(decoder, depth+1); err != nil {
					return nil, err
				}
			}
			if end, err := decoder.Token(); err != nil || end != json.Delim('}') {
				return nil, ErrInvalid
			}
			return object, nil
		case '[':
			list := []any{}
			for decoder.More() {
				item, err := strictValue(decoder, depth+1)
				if err != nil {
					return nil, err
				}
				list = append(list, item)
			}
			if end, err := decoder.Token(); err != nil || end != json.Delim(']') {
				return nil, ErrInvalid
			}
			return list, nil
		}
		return nil, ErrInvalid
	case nil:
		return nil, ErrInvalid
	default:
		return v, nil
	}
}

// lossless refuses invalid UTF-8 and lone escaped surrogates, which the JSON
// decoder would otherwise replace before canonicalization.
func lossless(raw []byte) bool {
	if !utf8.Valid(raw) {
		return false
	}
	quoted := false
	for i := 0; i < len(raw); i++ {
		switch raw[i] {
		case '"':
			quoted = !quoted
		case '\\':
			if !quoted || i+1 >= len(raw) {
				return false
			}
			i++
			if raw[i] != 'u' {
				continue
			}
			high, ok := hexUnit(raw, i+1)
			if !ok {
				return false
			}
			i += 4
			switch {
			case high >= 0xd800 && high <= 0xdbff:
				if i+6 >= len(raw) || raw[i+1] != '\\' || raw[i+2] != 'u' {
					return false
				}
				low, ok := hexUnit(raw, i+3)
				if !ok || low < 0xdc00 || low > 0xdfff {
					return false
				}
				i += 6
			case high >= 0xdc00 && high <= 0xdfff:
				return false
			}
		}
	}
	return !quoted
}

func hexUnit(raw []byte, at int) (int, bool) {
	if at+4 > len(raw) {
		return 0, false
	}
	value := 0
	for _, b := range raw[at : at+4] {
		value <<= 4
		switch {
		case b >= '0' && b <= '9':
			value += int(b - '0')
		case b >= 'a' && b <= 'f':
			value += int(b-'a') + 10
		case b >= 'A' && b <= 'F':
			value += int(b-'A') + 10
		default:
			return 0, false
		}
	}
	return value, true
}

// ParametersDigest binds canonical parameters to the organization and kind.
func ParametersDigest(scope kernel.OrgScope, kind string, canonical []byte) (string, error) {
	if scope.Validate() != nil || !ValidKind(kind) {
		return "", ErrInvalid
	}
	again, err := CanonicalParameters(canonical)
	if err != nil || !bytes.Equal(again, canonical) {
		return "", ErrInvalid
	}
	raw, _ := json.Marshal([]string{"orgexport.parameters.v1", scope.OrgID().String(), kind, string(canonical)})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

// File is a finished, bounded export produced through a Sink.
type File struct {
	Bytes  []byte
	Rows   int64
	SHA256 string
}

func (File) String() string       { return "orgexport: file redacted" }
func (File) GoString() string     { return "orgexport: file redacted" }
func (File) LogValue() slog.Value { return slog.StringValue("orgexport: file redacted") }

// Sink is the only writer a format receives. Once a ceiling is exceeded it is
// poisoned: every later write and Result fail, so no truncated file exists.
type Sink struct {
	limits Limits
	rows   int64
	buf    []byte
	failed bool
}

func NewSink(limits Limits) (*Sink, error) {
	if err := limits.Validate(); err != nil {
		return nil, err
	}
	return &Sink{limits: limits}, nil
}

func (s *Sink) append(p []byte, row bool) error {
	if s.failed {
		return ErrTooLarge
	}
	if int64(len(p)) > s.limits.Bytes-int64(len(s.buf)) || row && s.rows >= s.limits.Rows {
		s.failed = true
		return ErrTooLarge
	}
	s.buf = append(s.buf, p...)
	if row {
		s.rows++
	}
	return nil
}

// Write appends bytes that are not a data row (header, manifest).
func (s *Sink) Write(p []byte) (int, error) {
	if err := s.append(p, false); err != nil {
		return 0, err
	}
	return len(p), nil
}

// Row appends one data row and counts it.
func (s *Sink) Row(p []byte) error { return s.append(p, true) }

// CountRow counts one data row whose bytes reach the sink through Write, as
// in a packaged file (EMP-RPT-002); the row ceiling applies as for Row.
func (s *Sink) CountRow() error { return s.append(nil, true) }

func (s *Sink) Rows() int64  { return s.rows }
func (s *Sink) Bytes() int64 { return int64(len(s.buf)) }

// Result returns the complete file, or ErrTooLarge after any refusal and
// ErrInvalid for an empty file.
func (s *Sink) Result() (File, error) {
	if s.failed {
		return File{}, ErrTooLarge
	}
	if len(s.buf) == 0 {
		return File{}, ErrInvalid
	}
	sum := sha256.Sum256(s.buf)
	return File{Bytes: bytes.Clone(s.buf), Rows: s.rows, SHA256: hex.EncodeToString(sum[:])}, nil
}

// Object is the identity of one private export file.
type Object struct {
	OrgID, ExportID, SHA256, MediaType string
	Bytes, Rows                        int64
}

func (o Object) Validate(scope kernel.OrgScope) error {
	if scope.Validate() != nil || o.OrgID != scope.OrgID().String() || !ValidID(o.ExportID) || !ValidSHA256(o.SHA256) ||
		!ValidMediaType(o.MediaType) || o.Bytes < 1 || o.Bytes > MaxBytes || o.Rows < 0 || o.Rows > MaxRows {
		return ErrInvalid
	}
	return nil
}

// Key is org/<org_id>/export/<export_id>/<sha256>.
func (o Object) Key(scope kernel.OrgScope) (string, error) {
	prefix, err := o.Prefix(scope)
	if err != nil {
		return "", err
	}
	return prefix + o.SHA256, nil
}

// Prefix is the namespace of one job's objects.
func (o Object) Prefix(scope kernel.OrgScope) (string, error) {
	if err := o.Validate(scope); err != nil {
		return "", err
	}
	return "org/" + o.OrgID + "/export/" + o.ExportID + "/", nil
}

// Check verifies exact size and digest of retrieved bytes.
func (o Object) Check(raw []byte) error {
	if int64(len(raw)) != o.Bytes {
		return ErrIntegrity
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != o.SHA256 {
		return ErrIntegrity
	}
	return nil
}

// ObjectFor derives the identity of a finished file.
func ObjectFor(scope kernel.OrgScope, exportID, mediaType string, file File) (Object, error) {
	if scope.Validate() != nil {
		return Object{}, ErrInvalid
	}
	object := Object{OrgID: scope.OrgID().String(), ExportID: exportID, SHA256: file.SHA256, MediaType: mediaType, Bytes: int64(len(file.Bytes)), Rows: file.Rows}
	if err := object.Validate(scope); err != nil {
		return Object{}, err
	}
	if object.Check(file.Bytes) != nil {
		return Object{}, ErrInvalid
	}
	return object, nil
}
