package domain

import (
	"errors"
	"strings"
	"time"
)

// Policy version frozen with this aggregate shape. Changes require fixtures
// and a new version, never silent reinterpretation.
const PolicyV1 = "community-pricing-v1"

// StandardQualifier is the stored sentinel for STANDARD conditions, which
// carry no qualifier on the wire. The composite key stays non-nullable.
const StandardQualifier = "STANDARD"

// Capture skew rules (COMMUNITY_PRICING_SPEC): a capture claimed more than
// a day old or over five minutes in the future is historical-only.
const (
	MaxCaptureAge    = 24 * time.Hour
	FutureSkewMargin = 5 * time.Minute
)

// Exact milli-BRL bounds mirror kernel/OpenAPI Money (B-BR-002). The submit
// path parses through the kernel first; this range keeps stored facts
// honest even if a caller bypasses it.
const (
	MinMilliBRL = 1
	MaxMilliBRL = 1000000
)

// Wire vocabularies mirror kernel constants and the OpenAPI enums. A
// cross-check test pins them together; edit both sides or neither.
var wireProducts = map[string]string{
	"ETHANOL": "L", "GASOLINE_REGULAR": "L", "GASOLINE_ADDITIVED": "L",
	"DIESEL_S500": "L", "DIESEL_S10": "L", "CNG": "M3", "LPG_P13": "KG_13",
}

var wireConditions = map[string]bool{
	"STANDARD": true, "CASH": true, "DEBIT": true, "CREDIT": true,
	"APP": true, "LOYALTY": true, "OTHER": true,
}

var (
	ErrInvalidObservation = errors.New("community: invalid observation")
	ErrUnknownProduct     = errors.New("community: unknown fuel product")
	ErrUnitMismatch       = errors.New("community: unit does not match product")
	ErrInvalidAmount      = errors.New("community: amount outside 1..1000000 milli-BRL")
	ErrUnknownCondition   = errors.New("community: unknown condition")
	ErrFutureCapture      = errors.New("community: capture too far in the future")
)

// Freshness classifies the claimed capture time for downstream validation.
type Freshness int

const (
	Fresh Freshness = iota
	Historical
)

// CaptureFreshness labels old or future-skewed captures historical-only.
// Missing capture time (zero) labels as received time per spec.
func CaptureFreshness(claimed, received time.Time) Freshness {
	if claimed.IsZero() {
		return Fresh
	}
	if claimed.After(received.Add(FutureSkewMargin)) {
		return Historical
	}
	if received.Sub(claimed) > MaxCaptureAge {
		return Historical
	}
	return Fresh
}

// Params carries submission content plus server-resolved attribution. Every
// server-controlled field (IDs, times, contributor, policy) arrives from
// the caller, never from the client body.
type Params struct {
	ID                 string
	ContributorRef     string
	ClientSubmissionID string
	StationID          string
	Product            string
	Unit               string
	AmountMilli        int64
	RawText            string
	ConditionKind      string
	Qualifier          string
	EvidenceID         string
	ClaimedCapturedAt  time.Time
	SupersedesID       string
	ReceivedAt         time.Time
	PolicyVersion      string
}

// Observation is one immutable price fact.
type Observation struct {
	ID                 string
	ContributorRef     string
	ClientSubmissionID string
	StationID          string
	Product            string
	Unit               string
	AmountMilli        int64
	RawText            string
	ConditionKind      string
	QualifierKey       string
	EvidenceID         string
	ClaimedCapturedAt  time.Time
	SupersedesID       string
	ReceivedAt         time.Time
	PolicyVersion      string
	Freshness          Freshness
}

// PriceObserved is the domain event recorded at construction.
type PriceObserved struct {
	ObservationID string
	OccurredAt    time.Time
}

// NewObservation validates and freezes one fact. RawText preserves the
// source decimal beside the exact amount; Qualifier normalizes a missing
// STANDARD qualifier to the storage sentinel.
func NewObservation(p Params) (Observation, PriceObserved, error) {
	if strings.TrimSpace(p.ID) == "" || strings.TrimSpace(p.ContributorRef) == "" ||
		strings.TrimSpace(p.ClientSubmissionID) == "" || strings.TrimSpace(p.StationID) == "" {
		return Observation{}, PriceObserved{}, ErrInvalidObservation
	}
	wantUnit, ok := wireProducts[p.Product]
	if !ok {
		return Observation{}, PriceObserved{}, ErrUnknownProduct
	}
	if p.Unit != wantUnit {
		return Observation{}, PriceObserved{}, ErrUnitMismatch
	}
	if p.AmountMilli < MinMilliBRL || p.AmountMilli > MaxMilliBRL {
		return Observation{}, PriceObserved{}, ErrInvalidAmount
	}
	if !wireConditions[p.ConditionKind] {
		return Observation{}, PriceObserved{}, ErrUnknownCondition
	}
	qualifier := p.Qualifier
	if p.ConditionKind == "STANDARD" {
		// The wire null becomes the storage sentinel; other kinds keep
		// whatever arrived (empty means pending classification downstream,
		// never silently STANDARD).
		qualifier = StandardQualifier
	}
	if p.ReceivedAt.IsZero() {
		return Observation{}, PriceObserved{}, ErrInvalidObservation
	}
	if !p.ClaimedCapturedAt.IsZero() && p.ClaimedCapturedAt.After(p.ReceivedAt.Add(FutureSkewMargin)) {
		return Observation{}, PriceObserved{}, ErrFutureCapture
	}
	if p.PolicyVersion == "" {
		p.PolicyVersion = PolicyV1
	}
	freshness := CaptureFreshness(p.ClaimedCapturedAt, p.ReceivedAt)
	obs := Observation{
		ID: p.ID, ContributorRef: p.ContributorRef,
		ClientSubmissionID: p.ClientSubmissionID, StationID: p.StationID,
		Product: p.Product, Unit: p.Unit, AmountMilli: p.AmountMilli,
		RawText: p.RawText, ConditionKind: p.ConditionKind,
		QualifierKey: qualifier, EvidenceID: p.EvidenceID,
		ClaimedCapturedAt: p.ClaimedCapturedAt, SupersedesID: p.SupersedesID,
		ReceivedAt: p.ReceivedAt, PolicyVersion: p.PolicyVersion,
		Freshness: freshness,
	}
	return obs, PriceObserved{ObservationID: obs.ID, OccurredAt: obs.ReceivedAt}, nil
}
