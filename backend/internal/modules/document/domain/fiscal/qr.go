package fiscal

import "strings"

// MaxQRBytes is the byte capacity of the largest QR code symbol.
const MaxQRBytes = 2953

// Environment is the QR environment field.
type Environment string

const (
	Production   Environment = "PRODUCTION"
	Homologation Environment = "HOMOLOGATION"
)

// Layout distinguishes the online and contingency (offline) version-2 forms.
type Layout string

const (
	Online  Layout = "ONLINE"
	Offline Layout = "OFFLINE"
)

// QRReference is a structurally checked NFC-e QR code payload. Its hash and
// digest are never verified and its URL is never opened.
type QRReference struct {
	redacted
	key         AccessKey
	version     string
	environment Environment
	layout      Layout
	total       *Amount
}

func (q QRReference) AccessKey() AccessKey     { return q.key }
func (q QRReference) Version() string          { return q.version }
func (q QRReference) Environment() Environment { return q.environment }
func (q QRReference) Layout() Layout           { return q.layout }

// Total is present only for the version-2 offline layout.
func (q QRReference) Total() (Amount, bool) {
	if q.total == nil {
		return Amount{}, false
	}
	return *q.total, true
}

// CheckQRCode checks an NFC-e QR code payload without fetching anything
// (B-BR-EMP-233): size, URL, the single p parameter, version, fields, the
// access key and the NFC-e model, in that order.
func CheckQRCode(payload string) Result {
	c := checker{kind: QRCodeInput}
	c.run(CheckQRSize)
	if len(payload) > MaxQRBytes {
		return c.fail(ReasonQRTooLarge)
	}
	c.run(CheckQRURL)
	rest, ok := cutScheme(payload)
	if !ok {
		return c.fail(ReasonQRNotURL)
	}
	end := strings.IndexAny(rest, "/?")
	if end < 0 {
		end = len(rest)
	}
	if !governmentHost(rest[:end]) {
		return c.fail(ReasonQRHostUnsupported)
	}
	if !visibleURL(payload) {
		return c.fail(ReasonQRMalformed)
	}
	c.run(CheckQRParameters)
	_, query, _ := strings.Cut(rest[end:], "?")
	parts := strings.Split(query, "&")
	value, found := "", 0
	for _, part := range parts {
		if name, v, _ := strings.Cut(part, "="); name == "p" {
			value, found = v, found+1
		}
	}
	if found == 0 {
		return c.fail(ReasonQRFormatUnsupported)
	}
	if len(parts) != 1 {
		return c.fail(ReasonQRUnexpectedParameter)
	}
	c.run(CheckQRVersion)
	f := strings.Split(percentDecode(value), "|")
	if len(f) < 2 || (f[1] != "2" && f[1] != "3") {
		return c.fail(ReasonQRVersionUnsupported)
	}
	if f[1] == "3" && len(f) != 3 {
		return c.fail(ReasonQRVariantUnsupported)
	}
	c.run(CheckQRFields)
	ref, ok := qrFields(f)
	if !ok {
		return c.fail(ReasonQRFields)
	}
	key, reason := c.accessKey(f[0])
	if reason != "" {
		return c.fail(reason)
	}
	c.run(CheckQRModel)
	if key.Model() != "65" {
		return c.fail(ReasonQRModelUnsupported)
	}
	ref.key = key
	out := c.pass()
	out.key, out.qr = &key, &ref
	return out
}

// qrFields checks the pinned field layouts: version 2 online (5 fields) or
// offline (8 fields) and version 3 online (3 fields).
func qrFields(f []string) (QRReference, bool) {
	ref := QRReference{version: f[1], layout: Online}
	if f[1] == "2" && len(f) != 5 && len(f) != 8 {
		return ref, false
	}
	switch f[2] {
	case "1":
		ref.environment = Production
	case "2":
		ref.environment = Homologation
	default:
		return ref, false
	}
	switch len(f) {
	case 5:
		return ref, csc(f[3]) && hexDigits(f[4], 40, 40)
	case 8:
		total, ok := parseAmount(f[4])
		ref.layout, ref.total = Offline, &total
		return ref, ok && len(f[3]) == 2 && f[3] >= "01" && f[3] <= "31" && digits(f[3]) &&
			hexDigits(f[5], 1, 128) && csc(f[6]) && hexDigits(f[7], 40, 40)
	}
	return ref, true
}

func csc(s string) bool { return len(s) <= 6 && digits(s) }

// cutScheme accepts an ASCII case-insensitive http:// or https:// prefix.
func cutScheme(s string) (string, bool) {
	for _, scheme := range []string{"https://", "http://"} {
		if len(s) >= len(scheme) && asciiLower(s[:len(scheme)]) == scheme {
			return s[len(scheme):], true
		}
	}
	return "", false
}

func asciiLower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + ('a' - 'A')
		}
	}
	return string(b)
}

// governmentHost accepts a plain DNS host name under .gov.br with at least
// one label before it (the suffix plus the nonempty-label rule): no user
// info, port, IP literal, empty label or trailing dot.
func governmentHost(raw string) bool {
	host := asciiLower(raw)
	if len(host) > 253 || !strings.HasSuffix(host, ".gov.br") {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for i := 0; i < len(label); i++ {
			if b := label[i]; (b < 'a' || b > 'z') && (b < '0' || b > '9') && b != '-' {
				return false
			}
		}
	}
	return true
}

// visibleURL requires visible ASCII, no fragment and percent escapes of two
// hex digits that decode to visible ASCII.
func visibleURL(s string) bool {
	for i := 0; i < len(s); i++ {
		b := s[i]
		if b < 0x21 || b > 0x7e || b == '#' {
			return false
		}
		if b != '%' {
			continue
		}
		if i+2 >= len(s) {
			return false
		}
		hi, ok1 := hexValue(s[i+1])
		lo, ok2 := hexValue(s[i+2])
		if decoded := hi<<4 | lo; !ok1 || !ok2 || decoded < 0x21 || decoded > 0x7e {
			return false
		}
		i += 2
	}
	return true
}

// percentDecode decodes escapes already proven well formed by visibleURL.
func percentDecode(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '%' {
			b.WriteByte(s[i])
			continue
		}
		hi, _ := hexValue(s[i+1])
		lo, _ := hexValue(s[i+2])
		b.WriteByte(hi<<4 | lo)
		i += 2
	}
	return b.String()
}
