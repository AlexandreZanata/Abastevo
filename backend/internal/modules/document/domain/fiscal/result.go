// Package fiscal checks the structure of NF-e/NFC-e access keys, NFC-e QR
// code payloads and NF-e/NFC-e XML envelopes (EMP-DOC-006). It is pure and
// offline: it never contacts a provider, opens a URL or verifies a signature,
// protocol or QR hash, and a structurally valid input is never a fiscal
// authorization (BR-TRUTH-001/002, EDR-014). A result feeds only the document
// Structure dimension.
package fiscal

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/document/domain"
	exact "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgvalue/domain"
)

// ErrNotComparable is returned when a comparison operand is not a
// STRUCTURE_OK result carrying an access key.
var ErrNotComparable = errors.New("fiscal: not comparable")

type redacted struct{}

func (redacted) String() string                 { return "EnterpriseFiscalValue(redacted)" }
func (r redacted) Format(f fmt.State, _ rune)   { _, _ = io.WriteString(f, r.String()) }
func (r redacted) LogValue() slog.Value         { return slog.StringValue(r.String()) }
func (r redacted) MarshalJSON() ([]byte, error) { return json.Marshal(r.String()) }

// Outcome is the single answer of a check. UNSUPPORTED means the input was not
// identified as a handled layout; it is never reported as invalid.
type Outcome string

const (
	StructureOK      Outcome = "STRUCTURE_OK"
	StructureInvalid Outcome = "STRUCTURE_INVALID"
	Unsupported      Outcome = "UNSUPPORTED"
)

// Kind names what was checked.
type Kind string

const (
	AccessKeyInput Kind = "ACCESS_KEY"
	QRCodeInput    Kind = "QR_CODE"
	XMLInput       Kind = "XML"
	Comparison     Kind = "COMPARISON"
)

// Check is one named structural check, reported in the order it ran.
type Check string

const (
	CheckKeyFormat         Check = "KEY_FORMAT"
	CheckKeyDigit          Check = "KEY_CHECK_DIGIT"
	CheckKeyFields         Check = "KEY_FIELDS"
	CheckQRSize            Check = "QR_SIZE"
	CheckQRURL             Check = "QR_URL"
	CheckQRParameters      Check = "QR_PARAMETERS"
	CheckQRVersion         Check = "QR_VERSION"
	CheckQRFields          Check = "QR_FIELDS"
	CheckQRModel           Check = "QR_MODEL"
	CheckXMLSize           Check = "XML_SIZE"
	CheckXMLParse          Check = "XML_PARSE"
	CheckXMLRoot           Check = "XML_ROOT"
	CheckXMLEnvelope       Check = "XML_ENVELOPE"
	CheckXMLVersion        Check = "XML_VERSION"
	CheckXMLFields         Check = "XML_FIELDS"
	CheckXMLIssuer         Check = "XML_ISSUER"
	CheckXMLKeyConsistency Check = "XML_KEY_CONSISTENCY"
	CheckXMLItems          Check = "XML_ITEMS"
	CheckXMLTotal          Check = "XML_TOTAL"
	CheckKeyMatch          Check = "KEY_MATCH"
)

// Reason is the closed cause of a non-OK outcome; each reason has one outcome.
type Reason string

const (
	ReasonKeyLength             Reason = "KEY_LENGTH"
	ReasonKeyCharacter          Reason = "KEY_CHARACTER"
	ReasonKeyCheckDigit         Reason = "KEY_CHECK_DIGIT"
	ReasonKeyState              Reason = "KEY_STATE"
	ReasonKeyMonth              Reason = "KEY_MONTH"
	ReasonKeyModelUnsupported   Reason = "KEY_MODEL_UNSUPPORTED"
	ReasonQRTooLarge            Reason = "QR_TOO_LARGE"
	ReasonQRNotURL              Reason = "QR_NOT_URL"
	ReasonQRHostUnsupported     Reason = "QR_HOST_UNSUPPORTED"
	ReasonQRMalformed           Reason = "QR_MALFORMED"
	ReasonQRFormatUnsupported   Reason = "QR_FORMAT_UNSUPPORTED"
	ReasonQRUnexpectedParameter Reason = "QR_UNEXPECTED_PARAMETER"
	ReasonQRVersionUnsupported  Reason = "QR_VERSION_UNSUPPORTED"
	ReasonQRVariantUnsupported  Reason = "QR_VARIANT_UNSUPPORTED"
	ReasonQRFields              Reason = "QR_FIELDS"
	ReasonQRModelUnsupported    Reason = "QR_MODEL_UNSUPPORTED"
	ReasonXMLTooLarge           Reason = "XML_TOO_LARGE"
	ReasonXMLDoctype            Reason = "XML_DOCTYPE"
	ReasonXMLInstruction        Reason = "XML_INSTRUCTION"
	ReasonXMLDepth              Reason = "XML_DEPTH"
	ReasonXMLElements           Reason = "XML_ELEMENTS"
	ReasonXMLMalformed          Reason = "XML_MALFORMED"
	ReasonXMLRootUnsupported    Reason = "XML_ROOT_UNSUPPORTED"
	ReasonXMLEnvelope           Reason = "XML_ENVELOPE"
	ReasonXMLVersionUnsupported Reason = "XML_VERSION_UNSUPPORTED"
	ReasonXMLFields             Reason = "XML_FIELDS"
	ReasonXMLIssuer             Reason = "XML_ISSUER"
	ReasonXMLIssuerUnsupported  Reason = "XML_ISSUER_UNSUPPORTED"
	ReasonXMLKeyMismatch        Reason = "XML_KEY_MISMATCH"
	ReasonXMLItems              Reason = "XML_ITEMS"
	ReasonXMLTotal              Reason = "XML_TOTAL"
	ReasonKeyMismatch           Reason = "KEY_MISMATCH"
)

var unsupportedReasons = map[Reason]bool{
	ReasonKeyModelUnsupported: true, ReasonQRNotURL: true, ReasonQRHostUnsupported: true,
	ReasonQRFormatUnsupported: true, ReasonQRVersionUnsupported: true, ReasonQRVariantUnsupported: true,
	ReasonQRModelUnsupported: true, ReasonXMLRootUnsupported: true, ReasonXMLVersionUnsupported: true,
	ReasonXMLIssuerUnsupported: true,
}

var invalidReasons = map[Reason]bool{
	ReasonKeyLength: true, ReasonKeyCharacter: true, ReasonKeyCheckDigit: true, ReasonKeyState: true,
	ReasonKeyMonth: true, ReasonQRTooLarge: true, ReasonQRMalformed: true, ReasonQRUnexpectedParameter: true,
	ReasonQRFields: true, ReasonXMLTooLarge: true, ReasonXMLDoctype: true, ReasonXMLInstruction: true,
	ReasonXMLDepth: true, ReasonXMLElements: true, ReasonXMLMalformed: true, ReasonXMLEnvelope: true,
	ReasonXMLFields: true, ReasonXMLIssuer: true, ReasonXMLKeyMismatch: true, ReasonXMLItems: true,
	ReasonXMLTotal: true, ReasonKeyMismatch: true,
}

// Outcome returns the fixed outcome of a reason, or "" for an unknown token.
func (r Reason) Outcome() Outcome {
	switch {
	case unsupportedReasons[r]:
		return Unsupported
	case invalidReasons[r]:
		return StructureInvalid
	}
	return ""
}

// Amount is an exact decimal total as written in the source. Centavos come
// only from the exact parser within the accepted bound; nothing is rounded.
type Amount struct {
	redacted
	text     string
	centavos *int64
}

func (a Amount) Text() string { return a.text }

// Centavos reports the exact integer amount, absent above the accepted bound.
func (a Amount) Centavos() (int64, bool) {
	if a.centavos == nil {
		return 0, false
	}
	return *a.centavos, true
}

// Result is one immutable structure answer. It has no authorization,
// cancellation, consultation or "validated" field.
type Result struct {
	redacted
	kind    Kind
	outcome Outcome
	reason  Reason
	checks  []Check
	key     *AccessKey
	qr      *QRReference
	xml     *XMLSummary
}

func (r Result) Kind() Kind       { return r.kind }
func (r Result) Outcome() Outcome { return r.outcome }

// Reason is empty for STRUCTURE_OK.
func (r Result) Reason() Reason { return r.reason }

// Checks lists the checks that ran, in order; the last decides a non-OK outcome.
func (r Result) Checks() []Check { return append([]Check(nil), r.checks...) }

// AccessKey is present only on a STRUCTURE_OK result.
func (r Result) AccessKey() (AccessKey, bool) {
	if r.key == nil {
		return AccessKey{}, false
	}
	return *r.key, true
}

// QRCode is present only on a STRUCTURE_OK QR code result.
func (r Result) QRCode() (QRReference, bool) {
	if r.qr == nil {
		return QRReference{}, false
	}
	return *r.qr, true
}

// XML is present only on a STRUCTURE_OK XML result.
func (r Result) XML() (XMLSummary, bool) {
	if r.xml == nil {
		return XMLSummary{}, false
	}
	return *r.xml, true
}

// StructureTransition is the only document command a result can feed: the
// Structure dimension from UNCHECKED. UNSUPPORTED (or an empty result)
// commands nothing, and no result ever carries a consultation instant.
func (r Result) StructureTransition() (domain.Transition, bool) {
	command := domain.Transition{Dimension: domain.Structure, From: domain.Unchecked}
	switch r.outcome {
	case StructureOK:
		command.To = domain.StructureOK
	case StructureInvalid:
		command.To = domain.StructureInvalid
	default:
		return domain.Transition{}, false
	}
	return command, true
}

// Compare checks that two STRUCTURE_OK results carry the same access key. It
// says nothing about either document's fiscal status.
func Compare(a, b Result) (Result, error) {
	left, okLeft := a.AccessKey()
	right, okRight := b.AccessKey()
	if a.outcome != StructureOK || b.outcome != StructureOK || !okLeft || !okRight {
		return Result{}, ErrNotComparable
	}
	c := checker{kind: Comparison}
	c.run(CheckKeyMatch)
	if left.text != right.text {
		return c.fail(ReasonKeyMismatch), nil
	}
	out := c.pass()
	out.key = &left
	return out, nil
}

// checker accumulates the checks that ran for one result.
type checker struct {
	kind   Kind
	checks []Check
}

func (c *checker) run(check Check) { c.checks = append(c.checks, check) }

func (c *checker) fail(reason Reason) Result {
	return Result{kind: c.kind, outcome: reason.Outcome(), reason: reason, checks: c.checks}
}

func (c *checker) pass() Result { return Result{kind: c.kind, outcome: StructureOK, checks: c.checks} }

func digits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return s != ""
}

func hexDigits(s string, minimum, maximum int) bool {
	if len(s) < minimum || len(s) > maximum {
		return false
	}
	for i := 0; i < len(s); i++ {
		if _, ok := hexValue(s[i]); !ok {
			return false
		}
	}
	return true
}

func hexValue(b byte) (byte, bool) {
	switch {
	case b >= '0' && b <= '9':
		return b - '0', true
	case b >= 'a' && b <= 'f':
		return b - 'a' + 10, true
	case b >= 'A' && b <= 'F':
		return b - 'A' + 10, true
	}
	return 0, false
}

// parseAmount accepts exactly the NF-e TDec_1302 text
// 0|0\.[0-9]{2}|[1-9][0-9]{0,12}(\.[0-9]{2})? and converts it to centavos
// with the existing exact parser; above its bound the centavos stay absent.
func parseAmount(text string) (Amount, bool) {
	whole, fraction, decimal := strings.Cut(text, ".")
	if decimal && (len(fraction) != 2 || !digits(fraction)) {
		return Amount{}, false
	}
	if !digits(whole) || len(whole) > 13 || (whole[0] == '0' && len(whole) > 1) {
		return Amount{}, false
	}
	if !decimal {
		fraction = "00"
	}
	cents := strings.TrimLeft(whole+fraction, "0")
	if cents == "" {
		cents = "0"
	}
	out := Amount{text: text}
	if n, err := exact.ParseCentavos(cents); err == nil {
		out.centavos = &n
	}
	return out, true
}
