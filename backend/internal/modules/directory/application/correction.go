package application

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// Correction statuses, fields and bounds are frozen by B-BR-COR-01..08.
const (
	CorrectionPending      = "pending"
	CorrectionAutoVerified = "auto_verified"
	CorrectionAccepted     = "accepted"
	CorrectionRejected     = "rejected"
	CorrectionExpired      = "expired"
	CorrectionCancelled    = "cancelled"
	CorrectionSuperseded   = "superseded"
	CorrectionFrozen       = "frozen"

	CorrectionFieldLocation        = "location"
	CorrectionFieldDisplayName     = "display_name"
	CorrectionFieldBrand           = "brand"
	CorrectionFieldAddress         = "address"
	CorrectionFieldOperatingStatus = "operating_status"

	MaxCorrectionsPerDay = 20
	MaxCorrectionNote    = 280
	maxCorrectionText    = 160
	maxCorrectionRef     = 200
)

var (
	ErrCorrectionQuota     = errors.New("directory: daily correction quota exceeded")
	ErrCorrectionConflict  = errors.New("directory: same key with different correction")
	ErrCorrectionNotFound  = errors.New("directory: correction not found")
	ErrCorrectionClosed    = errors.New("directory: correction is no longer pending")
	ErrCorrectionOpen      = errors.New("directory: an open correction already exists for this field")
	ErrCorrectionInvalid   = errors.New("directory: invalid correction")
	ErrCorrectionForbidden = errors.New("directory: account cannot write corrections")
)

var (
	correctionEmail      = regexp.MustCompile(`(?i)[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,}`)
	correctionURL        = regexp.MustCompile(`(?i)https?://|www\.`)
	correctionCPF        = regexp.MustCompile(`\b\d{3}\.\d{3}\.\d{3}-\d{2}\b`)
	correctionLongDigits = regexp.MustCompile(`\d{10,}`)
)

// ProposedCorrection is the validated, canonical payload stored as JSONB.
type ProposedCorrection struct {
	Latitude         float64 `json:"latitude,omitempty"`
	Longitude        float64 `json:"longitude,omitempty"`
	Value            string  `json:"value,omitempty"`
	Street           string  `json:"street,omitempty"`
	Number           string  `json:"number,omitempty"`
	District         string  `json:"district,omitempty"`
	PostalCode       string  `json:"postal_code,omitempty"`
	MunicipalityCode string  `json:"municipality_code,omitempty"`
	State            string  `json:"state,omitempty"`
	Status           string  `json:"status,omitempty"`
}

// CorrectionInput is one owner submission.
type CorrectionInput struct {
	StationID          string
	Field              string
	ProposedValue      json.RawMessage
	Note               string
	EvidenceRef        string
	ProximityBucket    string
	ClientSubmissionID string
}

// Correction is one private correction record.
type Correction struct {
	ID                 string
	StationID          string
	AccountID          string
	Field              string
	ProposedValue      ProposedCorrection
	Note               string
	EvidenceRef        string
	ProximityBucket    string
	ClientSubmissionID string
	Status             string
	ReasonCodes        []string
	CreatedAt          time.Time
	DecidedAt          *time.Time
}

// CorrectionRow is the stored shape used by the adapter.
type CorrectionRow struct {
	ID                 string
	StationID          string
	AccountID          string
	Field              string
	ProposedValue      []byte
	Note               string
	EvidenceRef        string
	ProximityBucket    string
	ClientSubmissionID string
	Status             string
	ReasonCodes        []string
	CreatedAt          time.Time
	DecidedAt          *time.Time
}

// CorrectionStore persists corrections and the audited review trail.
type CorrectionStore interface {
	CreateCorrection(ctx context.Context, id, stationID, accountID, field string, value []byte, note, evidenceRef, proximity, key string) (CorrectionRow, bool, error)
	CorrectionByKey(ctx context.Context, accountID, clientKey string) (CorrectionRow, error)
	GetCorrection(ctx context.Context, id string) (CorrectionRow, error)
	ListOwned(ctx context.Context, accountID string, limit, offset int) ([]CorrectionRow, error)
	CountRecent(ctx context.Context, accountID string) (int64, error)
	CountOpen(ctx context.Context, accountID, stationID, field string) (int64, error)
	CancelCorrection(ctx context.Context, id, accountID string) (int64, error)
	ListPending(ctx context.Context, limit int) ([]CorrectionRow, error)
	Decide(ctx context.Context, id string, status string, reasonCodes []string, decision, reason, deciderKind, deciderID string, evidence []byte) (int64, error)
	ApplyAcceptedLocation(ctx context.Context, correction Correction) error
}

// CorrectionService owns submission and owner reads. The HTTP layer
// resolves the live account first; this service never sees tokens.
type CorrectionService struct {
	Store CorrectionStore
	Clock func() time.Time
	NewID func() (string, error)
}

// Submit stores one correction idempotently and enforces the quota and the
// one-open-per-field rule before any write.
func (s CorrectionService) Submit(ctx context.Context, accountID, clientKey string, in CorrectionInput) (Correction, bool, error) {
	if accountID == "" || clientKey == "" || in.StationID == "" {
		return Correction{}, false, ErrCorrectionInvalid
	}
	value, note, err := NormalizeCorrection(in.Field, in.ProposedValue, in.Note)
	if err != nil {
		return Correction{}, false, err
	}
	evidenceRef, err := normalizeEvidenceRef(in.EvidenceRef)
	if err != nil {
		return Correction{}, false, err
	}
	if !validProximityBucket(in.ProximityBucket) {
		return Correction{}, false, ErrCorrectionInvalid
	}
	body, err := json.Marshal(value)
	if err != nil {
		return Correction{}, false, err
	}
	// Idempotent replay wins over quota/open checks: the same key and body
	// must return the existing row even while it is still pending.
	if existing, err := s.Store.CorrectionByKey(ctx, accountID, clientKey); err == nil {
		var stored ProposedCorrection
		if err := json.Unmarshal(existing.ProposedValue, &stored); err != nil || !reflect.DeepEqual(stored, value) ||
			existing.Note != note || existing.EvidenceRef != evidenceRef ||
			existing.ProximityBucket != in.ProximityBucket || existing.StationID != in.StationID || existing.Field != in.Field {
			return Correction{}, false, ErrCorrectionConflict
		}
		return mapCorrection(existing), false, nil
	} else if !errors.Is(err, ErrCorrectionNotFound) {
		return Correction{}, false, err
	}
	recent, err := s.Store.CountRecent(ctx, accountID)
	if err != nil {
		return Correction{}, false, err
	}
	if recent >= MaxCorrectionsPerDay {
		return Correction{}, false, ErrCorrectionQuota
	}
	open, err := s.Store.CountOpen(ctx, accountID, in.StationID, in.Field)
	if err != nil {
		return Correction{}, false, err
	}
	if open >= 1 {
		return Correction{}, false, ErrCorrectionOpen
	}
	id, err := s.newID()
	if err != nil {
		return Correction{}, false, err
	}
	row, created, err := s.Store.CreateCorrection(ctx, id, in.StationID, accountID, in.Field, body, note, evidenceRef, in.ProximityBucket, clientKey)
	if err != nil {
		return Correction{}, false, err
	}
	if !created {
		existing, err := s.Store.CorrectionByKey(ctx, accountID, clientKey)
		if err != nil {
			return Correction{}, false, err
		}
		var stored ProposedCorrection
		if err := json.Unmarshal(existing.ProposedValue, &stored); err != nil || !reflect.DeepEqual(stored, value) ||
			existing.Note != note || existing.EvidenceRef != evidenceRef ||
			existing.ProximityBucket != in.ProximityBucket || existing.StationID != in.StationID || existing.Field != in.Field {
			return Correction{}, false, ErrCorrectionConflict
		}
		return mapCorrection(existing), false, nil
	}
	return mapCorrection(row), true, nil
}

// Cancel closes an owner's pending correction. Foreign owners and non-pending
// rows fail closed without leaking which condition fired.
func (s CorrectionService) Cancel(ctx context.Context, accountID, id string) error {
	if accountID == "" || id == "" {
		return ErrCorrectionNotFound
	}
	row, err := s.Store.GetCorrection(ctx, id)
	if err != nil {
		return ErrCorrectionNotFound
	}
	if row.AccountID != accountID {
		return ErrCorrectionNotFound
	}
	if row.Status != CorrectionPending {
		return ErrCorrectionClosed
	}
	n, err := s.Store.CancelCorrection(ctx, id, accountID)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrCorrectionClosed
	}
	return nil
}

// Owned returns one correction only to its owner.
func (s CorrectionService) Owned(ctx context.Context, accountID, id string) (Correction, error) {
	if accountID == "" || id == "" {
		return Correction{}, ErrCorrectionNotFound
	}
	row, err := s.Store.GetCorrection(ctx, id)
	if err != nil {
		return Correction{}, ErrCorrectionNotFound
	}
	if row.AccountID != accountID {
		return Correction{}, ErrCorrectionNotFound
	}
	return mapCorrection(row), nil
}

// ListOwned returns the owner's private corrections, newest first.
func (s CorrectionService) ListOwned(ctx context.Context, accountID string, limit, offset int) ([]Correction, error) {
	if accountID == "" {
		return nil, ErrCorrectionNotFound
	}
	if limit <= 0 || limit > 20 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := s.Store.ListOwned(ctx, accountID, limit, offset)
	if err != nil {
		return nil, err
	}
	out := make([]Correction, 0, len(rows))
	for _, row := range rows {
		out = append(out, mapCorrection(row))
	}
	return out, nil
}

// ListPending is the operator review port.
func (s CorrectionService) ListPending(ctx context.Context, limit int) ([]Correction, error) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	rows, err := s.Store.ListPending(ctx, limit)
	if err != nil {
		return nil, err
	}
	out := make([]Correction, 0, len(rows))
	for _, row := range rows {
		out = append(out, mapCorrection(row))
	}
	return out, nil
}

// Decide applies one audited review decision. Only pending rows transition.
// Accepted locations create a reviewed revision through the existing ladder
// after the decision; rejected/frozen/superseded rows never project a point.
func (s CorrectionService) Decide(ctx context.Context, id, decision, reason, deciderKind, deciderID string, evidence []byte) error {
	if id == "" || deciderKind == "" || deciderID == "" || strings.TrimSpace(reason) == "" {
		return ErrCorrectionInvalid
	}
	switch decision {
	case CorrectionAccepted, CorrectionRejected, CorrectionExpired, CorrectionSuperseded, CorrectionFrozen:
	default:
		return ErrCorrectionInvalid
	}
	if len(reason) > 1000 {
		return ErrCorrectionInvalid
	}
	row, err := s.Store.GetCorrection(ctx, id)
	if err != nil {
		return ErrCorrectionNotFound
	}
	if row.Status != CorrectionPending {
		return ErrCorrectionClosed
	}
	n, err := s.Store.Decide(ctx, id, decision, reasonCodesFor(decision), decision, reason, deciderKind, deciderID, evidence)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrCorrectionClosed
	}
	if decision == CorrectionAccepted && row.Field == CorrectionFieldLocation {
		correction := mapCorrection(row)
		correction.Status = decision
		if err := s.Store.ApplyAcceptedLocation(ctx, correction); err != nil {
			return err
		}
	}
	return nil
}

func reasonCodesFor(decision string) []string {
	switch decision {
	case CorrectionRejected:
		return []string{"review_rejected"}
	case CorrectionFrozen:
		return []string{"review_frozen"}
	case CorrectionExpired:
		return []string{"expired"}
	case CorrectionSuperseded:
		return []string{"superseded"}
	default:
		return []string{"review_accepted"}
	}
}

// NormalizeCorrection validates one field payload and returns the canonical
// value plus the normalized optional note. Unknown JSON members fail closed.
func NormalizeCorrection(field string, raw json.RawMessage, note string) (ProposedCorrection, string, error) {
	note, err := normalizeNote(note)
	if err != nil {
		return ProposedCorrection{}, "", err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var value ProposedCorrection
	switch field {
	case CorrectionFieldLocation:
		if err := dec.Decode(&value); err != nil {
			return ProposedCorrection{}, "", ErrCorrectionInvalid
		}
		if !validBrazilPoint(value.Latitude, value.Longitude) {
			return ProposedCorrection{}, "", ErrCorrectionInvalid
		}
		return ProposedCorrection{Latitude: value.Latitude, Longitude: value.Longitude}, note, nil
	case CorrectionFieldDisplayName, CorrectionFieldBrand:
		if err := dec.Decode(&value); err != nil {
			return ProposedCorrection{}, "", ErrCorrectionInvalid
		}
		text, err := normalizeText(value.Value, 1, maxCorrectionText)
		if err != nil {
			return ProposedCorrection{}, "", err
		}
		return ProposedCorrection{Value: text}, note, nil
	case CorrectionFieldAddress:
		if err := dec.Decode(&value); err != nil {
			return ProposedCorrection{}, "", ErrCorrectionInvalid
		}
		street, err := normalizeText(value.Street, 1, maxCorrectionText)
		if err != nil {
			return ProposedCorrection{}, "", err
		}
		number, err := normalizeText(value.Number, 0, 20)
		if err != nil {
			return ProposedCorrection{}, "", err
		}
		district, err := normalizeText(value.District, 0, 80)
		if err != nil {
			return ProposedCorrection{}, "", err
		}
		postal, err := normalizeDigits(value.PostalCode, 8)
		if err != nil {
			return ProposedCorrection{}, "", err
		}
		municipality, err := normalizeDigits(value.MunicipalityCode, 7)
		if err != nil {
			return ProposedCorrection{}, "", err
		}
		state, err := normalizeState(value.State)
		if err != nil {
			return ProposedCorrection{}, "", err
		}
		return ProposedCorrection{
			Street: street, Number: number, District: district,
			PostalCode: postal, MunicipalityCode: municipality, State: state,
		}, note, nil
	case CorrectionFieldOperatingStatus:
		if err := dec.Decode(&value); err != nil {
			return ProposedCorrection{}, "", ErrCorrectionInvalid
		}
		switch value.Status {
		case "active", "temporarily_closed", "permanently_closed":
			return ProposedCorrection{Status: value.Status}, note, nil
		default:
			return ProposedCorrection{}, "", ErrCorrectionInvalid
		}
	default:
		return ProposedCorrection{}, "", ErrCorrectionInvalid
	}
}

func normalizeText(value string, min, max int) (string, error) {
	value = strings.TrimSpace(value)
	if utf8.RuneCountInString(value) < min || utf8.RuneCountInString(value) > max {
		return "", ErrCorrectionInvalid
	}
	for _, r := range value {
		if r == unicode.ReplacementChar || r == '\x00' || (unicode.IsControl(r) && r != '\n' && r != '\t') || r == '<' || r == '>' {
			return "", ErrCorrectionInvalid
		}
	}
	return value, nil
}

func normalizeNote(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if utf8.RuneCountInString(value) > MaxCorrectionNote {
		return "", ErrCorrectionInvalid
	}
	if correctionEmail.MatchString(value) || correctionURL.MatchString(value) ||
		correctionCPF.MatchString(value) || correctionLongDigits.MatchString(value) {
		return "", ErrCorrectionInvalid
	}
	if _, err := normalizeText(value, 1, MaxCorrectionNote); err != nil {
		return "", err
	}
	return value, nil
}

func normalizeDigits(value string, length int) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if len(value) != length {
		return "", ErrCorrectionInvalid
	}
	if _, err := strconv.Atoi(value); err != nil {
		return "", ErrCorrectionInvalid
	}
	return value, nil
}

func normalizeState(value string) (string, error) {
	value = strings.ToUpper(strings.TrimSpace(value))
	if value == "" {
		return "", nil
	}
	if len(value) != 2 || value[0] < 'A' || value[0] > 'Z' || value[1] < 'A' || value[1] > 'Z' {
		return "", ErrCorrectionInvalid
	}
	return value, nil
}

func normalizeEvidenceRef(value string) (string, error) {
	value = strings.TrimSpace(value)
	if len(value) > maxCorrectionRef {
		return "", ErrCorrectionInvalid
	}
	for _, r := range value {
		if unicode.IsControl(r) || r == '<' || r == '>' {
			return "", ErrCorrectionInvalid
		}
	}
	return value, nil
}

func validProximityBucket(value string) bool {
	switch value {
	case "", "same_municipality", "within_1km", "within_250m", "within_50m":
		return true
	default:
		return false
	}
}

func validBrazilPoint(lat, lon float64) bool {
	if math.IsNaN(lat) || math.IsNaN(lon) || math.IsInf(lat, 0) || math.IsInf(lon, 0) {
		return false
	}
	if lat == 0 && lon == 0 {
		return false
	}
	return lat >= -33.75 && lat <= 5.27 && lon >= -73.99 && lon <= -34.79
}

func mapCorrection(row CorrectionRow) Correction {
	var value ProposedCorrection
	_ = json.Unmarshal(row.ProposedValue, &value)
	return Correction{
		ID: row.ID, StationID: row.StationID, AccountID: row.AccountID,
		Field: row.Field, ProposedValue: value, Note: row.Note,
		EvidenceRef: row.EvidenceRef, ProximityBucket: row.ProximityBucket,
		ClientSubmissionID: row.ClientSubmissionID, Status: row.Status,
		ReasonCodes: row.ReasonCodes, CreatedAt: row.CreatedAt, DecidedAt: row.DecidedAt,
	}
}

func (s CorrectionService) newID() (string, error) {
	if s.NewID != nil {
		return s.NewID()
	}
	return "", fmt.Errorf("directory: correction id generator is not configured")
}
