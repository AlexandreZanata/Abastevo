// Package domain defines the closed, reference-only enterprise notice catalog.
// It performs no I/O; persistence, dispatch and routes live in adapters.
package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
)

const (
	// ConsumerName is the orgevent consumer a producer lists for its notices.
	ConsumerName        = "notif.dispatch"
	MaxRecipients       = 50 // orgevent's bounded fan-out precedent.
	MaxRequestsPerEvent = 10
	MaxParams           = 8
	MaxCodes            = 32
	MaxTemplates        = 8
	MaxCount            = 1_000_000_000
	// HourlyLimit bounds IN_APP notices per recipient and kind per UTC hour.
	HourlyLimit   = 20
	MaxKeyLength  = 128
	MaxLinkLength = 256
)

var (
	ErrInvalid     = errors.New("orgnotify: invalid notice")
	ErrValidation  = errors.New("orgnotify: validation failed")
	ErrNotFound    = errors.New("orgnotify: not found")
	ErrSuspended   = errors.New("orgnotify: membership suspended")
	ErrConflict    = errors.New("orgnotify: revision conflict")
	ErrReused      = errors.New("orgnotify: operation reused")
	ErrDenied      = errors.New("orgnotify: denied")
	ErrUnavailable = errors.New("orgnotify: unavailable")

	codeName  = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$`)
	paramName = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)
	codeValue = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,63}$`)
	literal   = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)
	keyValue  = regexp.MustCompile(`^[a-z0-9][a-z0-9._:-]*$`)
	count     = regexp.MustCompile(`^(0|[1-9][0-9]{0,9})$`)
)

// Channel is a delivery channel. Only IN_APP is active (EDR-032); the
// external channels stay disabled until EMP-NOTIF-002 and its decision.
type Channel string

const (
	ChannelInApp        Channel = "IN_APP"
	ChannelAndroidLocal Channel = "ANDROID_LOCAL"
	ChannelPush         Channel = "PUSH"
	ChannelEmail        Channel = "EMAIL"
)

func (c Channel) Enabled() bool { return c == ChannelInApp }
func Channels() []Channel {
	return []Channel{ChannelInApp, ChannelAndroidLocal, ChannelPush, ChannelEmail}
}

type Outcome string

const (
	Delivered  Outcome = "DELIVERED"
	Suppressed Outcome = "SUPPRESSED"
)

type Reason string

const (
	Ineligible Reason = "INELIGIBLE"
	Muted      Reason = "MUTED"
	Limited    Reason = "LIMITED"
)

// ParamType is the closed set of reference-only parameter values.
type ParamType uint8

const (
	ParamID ParamType = iota + 1
	ParamCode
	ParamCount
	ParamDate
)

type Param struct {
	Type  ParamType
	Codes []string
}

// Kind is source-owned vocabulary registered by a producer at composition.
// Permission, when set, must currently be held by each recipient at dispatch.
type Kind struct {
	Name       string
	Permission string
	Mandatory  bool
	Templates  []string
	Params     map[string]Param
	Link       string
}

func (k Kind) clone() Kind {
	k.Templates = slices.Clone(k.Templates)
	params := make(map[string]Param, len(k.Params))
	for name, p := range k.Params {
		p.Codes = slices.Clone(p.Codes)
		params[name] = p
	}
	k.Params = params
	return k
}

func validName(s string) bool { return len(s) <= 64 && codeName.MatchString(s) }

func validKind(k Kind) bool {
	if !validName(k.Name) || len(k.Templates) == 0 || len(k.Templates) > MaxTemplates || len(k.Params) > MaxParams {
		return false
	}
	if k.Permission != "" {
		if _, err := access.SystemCatalog().Lookup(access.Owner, access.Permission(k.Permission)); err != nil {
			return false
		}
	}
	seen := map[string]bool{}
	for _, t := range k.Templates {
		if !validName(t) || seen[t] {
			return false
		}
		seen[t] = true
	}
	for name, p := range k.Params {
		if !paramName.MatchString(name) {
			return false
		}
		switch p.Type {
		case ParamCode:
			if len(p.Codes) == 0 || len(p.Codes) > MaxCodes {
				return false
			}
			values := map[string]bool{}
			for _, c := range p.Codes {
				if !codeValue.MatchString(c) || values[c] {
					return false
				}
				values[c] = true
			}
		case ParamID, ParamCount, ParamDate:
			if len(p.Codes) != 0 {
				return false
			}
		default:
			return false
		}
	}
	if k.Link == "" {
		return true
	}
	if len(k.Link) > 200 || !strings.HasPrefix(k.Link, "/") {
		return false
	}
	for _, segment := range strings.Split(k.Link[1:], "/") {
		if strings.HasPrefix(segment, "{") && strings.HasSuffix(segment, "}") {
			if p, ok := k.Params[segment[1:len(segment)-1]]; !ok || p.Type != ParamID {
				return false
			}
		} else if !literal.MatchString(segment) {
			return false
		}
	}
	return true
}

// Catalog is the closed, immutable composition-registered kind vocabulary.
type Catalog struct{ kinds map[string]Kind }

// NewCatalog accepts an empty catalog: routes may exist before any producer.
func NewCatalog(kinds []Kind) (Catalog, error) {
	c := Catalog{kinds: make(map[string]Kind, len(kinds))}
	for _, k := range kinds {
		if !validKind(k) {
			return Catalog{}, ErrInvalid
		}
		if _, exists := c.kinds[k.Name]; exists {
			return Catalog{}, ErrInvalid
		}
		c.kinds[k.Name] = k.clone()
	}
	return c, nil
}

func (c Catalog) Lookup(name string) (Kind, bool) {
	k, ok := c.kinds[name]
	if !ok {
		return Kind{}, false
	}
	return k.clone(), true
}

// Kinds returns copies sorted by name.
func (c Catalog) Kinds() []Kind {
	names := slices.Sorted(maps.Keys(c.kinds))
	out := make([]Kind, 0, len(names))
	for _, name := range names {
		out = append(out, c.kinds[name].clone())
	}
	return out
}

// Mutable allows a member preference only for a registered optional kind.
func (c Catalog) Mutable(kind string) error {
	k, ok := c.kinds[kind]
	if !ok || k.Mandatory {
		return ErrValidation
	}
	return nil
}

// Screen applies eligibility before preference; "" proceeds to the bound.
func Screen(k Kind, eligible, muted bool) Reason {
	switch {
	case !eligible:
		return Ineligible
	case muted && !k.Mandatory:
		return Muted
	default:
		return ""
	}
}

// CommandDigest frames each part so different splits never collide.
func CommandDigest(action string, parts ...string) string {
	raw, _ := json.Marshal(append([]string{"org-notify.command.v1", action}, parts...))
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func canonicalID(s string) bool {
	id, err := kernel.ParseOrgID(s)
	return err == nil && id.String() == s
}

func validValue(p Param, v string) bool {
	switch p.Type {
	case ParamID:
		return canonicalID(v)
	case ParamCode:
		return slices.Contains(p.Codes, v)
	case ParamCount:
		if !count.MatchString(v) {
			return false
		}
		n, err := strconv.ParseInt(v, 10, 64)
		return err == nil && n <= MaxCount
	case ParamDate:
		d, err := time.Parse(time.DateOnly, v)
		return err == nil && d.Format(time.DateOnly) == v
	default:
		return false
	}
}

type privateNotice struct{}

const redacted = "orgnotify: notice references redacted"

func (privateNotice) String() string               { return redacted }
func (privateNotice) GoString() string             { return redacted }
func (privateNotice) LogValue() slog.Value         { return slog.StringValue(redacted) }
func (privateNotice) MarshalJSON() ([]byte, error) { return json.Marshal(redacted) }

// Request is what a producer asks for. Parameters are typed references only.
type Request struct {
	privateNotice
	Key, Kind, Template string
	Params              map[string]string
	Recipients          []string
}

// Prepared is a validated request with its rendered link and content digest.
type Prepared struct {
	privateNotice
	Key, Kind, Template string
	Params              map[string]string
	Link                string
	Recipients          []string
	Hash                string
}

// ParamsJSON is the canonical (sorted-key) JSON object of the parameters.
func (p Prepared) ParamsJSON() []byte {
	params := p.Params
	if params == nil {
		params = map[string]string{}
	}
	raw, _ := json.Marshal(params)
	return raw
}

// Prepare refuses anything outside the kind's declared, typed vocabulary.
func (c Catalog) Prepare(r Request) (Prepared, error) {
	k, ok := c.kinds[r.Kind]
	if !ok || len(r.Key) > MaxKeyLength || !keyValue.MatchString(r.Key) || !slices.Contains(k.Templates, r.Template) ||
		len(r.Params) != len(k.Params) || len(r.Recipients) == 0 || len(r.Recipients) > MaxRecipients {
		return Prepared{}, ErrInvalid
	}
	params := make(map[string]string, len(r.Params))
	for name, spec := range k.Params {
		v, present := r.Params[name]
		if !present || !validValue(spec, v) {
			return Prepared{}, ErrInvalid
		}
		params[name] = v
	}
	recipients := slices.Clone(r.Recipients)
	slices.Sort(recipients)
	for i, m := range recipients {
		if !canonicalID(m) || (i > 0 && m == recipients[i-1]) {
			return Prepared{}, ErrInvalid
		}
	}
	link := ""
	if k.Link != "" {
		segments := strings.Split(k.Link[1:], "/")
		for i, segment := range segments {
			if strings.HasPrefix(segment, "{") {
				segments[i] = params[segment[1:len(segment)-1]]
			}
		}
		link = "/" + strings.Join(segments, "/")
		if len(link) > MaxLinkLength {
			return Prepared{}, ErrInvalid
		}
	}
	p := Prepared{Key: r.Key, Kind: r.Kind, Template: r.Template, Params: params, Link: link, Recipients: recipients}
	raw, _ := json.Marshal(struct {
		Key, Kind, Template string
		Params              map[string]string
		Link                string
		Recipients          []string
	}{p.Key, p.Kind, p.Template, p.Params, p.Link, p.Recipients})
	sum := sha256.Sum256(append([]byte("org-notify.request.v1\x00"), raw...))
	p.Hash = hex.EncodeToString(sum[:])
	return p, nil
}

// PrepareAll validates one producer event's bounded, key-distinct requests.
func (c Catalog) PrepareAll(requests []Request) ([]Prepared, error) {
	if len(requests) == 0 || len(requests) > MaxRequestsPerEvent {
		return nil, ErrInvalid
	}
	out := make([]Prepared, 0, len(requests))
	keys := map[string]bool{}
	for _, r := range requests {
		p, err := c.Prepare(r)
		if err != nil || keys[p.Key] {
			return nil, ErrInvalid
		}
		keys[p.Key] = true
		out = append(out, p)
	}
	return out, nil
}

// Registered is the producer-port outcome for one request.
type Registered struct {
	RequestID, Key string
	Deduplicated   bool
}
