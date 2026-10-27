package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"time"
)

var recordUUID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
var sourceDigest = regexp.MustCompile(`^[0-9a-f]{64}$`)

func ValidID(s string) bool {
	return recordUUID.MatchString(s) && s != "00000000-0000-0000-0000-000000000000"
}
func ValidSource(s string) bool { return sourceDigest.MatchString(s) }

type EarningRecord struct {
	ID         string    `json:"id"`
	VehicleID  string    `json:"vehicle_id"`
	Platform   string    `json:"platform"`
	SourceHash string    `json:"source_hash"`
	Coverage   string    `json:"coverage"`
	Currency   string    `json:"currency"`
	From       time.Time `json:"from"`
	To         time.Time `json:"to"`
	Timezone   string    `json:"timezone"`
	Money      Earnings  `json:"money"`
}

func (r EarningRecord) Normalize() (EarningRecord, error) {
	// Capture value semantics before any caller-owned pointer can change during I/O.
	own := func(v *int64) *int64 {
		if v == nil {
			return nil
		}
		n := *v
		return &n
	}
	m := r.Money
	r.Money = Earnings{Basis: m.Basis, Gross: own(m.Gross), Fees: own(m.Fees), Tips: own(m.Tips), Bonus: own(m.Bonus), Refund: own(m.Refund), Net: own(m.Net)}

	if !ValidID(r.ID) || !ValidID(r.VehicleID) || !ValidSource(r.SourceHash) || (r.Platform != "UBER" && r.Platform != "99" && r.Platform != "INDRIVE" && r.Platform != "OTHER") || (r.Coverage != "DETAIL" && r.Coverage != "SUMMARY") || r.Currency != "BRL" || r.Timezone == "" || r.Timezone == "Local" {
		return r, ErrInvalid
	}
	if _, e := time.LoadLocation(r.Timezone); e != nil {
		return r, ErrInvalid
	}
	r.From = r.From.UTC()
	r.To = r.To.UTC()
	if r.From.Year() < 1 || r.From.Year() > 9999 || r.To.Year() < 1 || r.To.Year() > 9999 || !r.To.After(r.From) || r.From.Nanosecond()%1000 != 0 || r.To.Nanosecond()%1000 != 0 {
		return r, ErrInvalid
	}
	if _, e := r.Money.Earned(); e != nil {
		return r, e
	}
	return r, nil
}
func Digest(value any) (string, error) {
	b, e := json.Marshal(value)
	if e != nil {
		return "", ErrInvalid
	}
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:]), nil
}
func (r EarningRecord) BusinessHash() (string, error) {
	r, e := r.Normalize()
	if e != nil {
		return "", e
	}
	r.ID = ""
	return Digest(r)
}
