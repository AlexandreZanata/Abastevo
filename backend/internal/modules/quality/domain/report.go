package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

var (
	ErrReportInvalid      = errors.New("quality: invalid report")
	ErrReportNotFound     = errors.New("quality: report not found")
	ErrReportConflict     = errors.New("quality: report conflict")
	ErrReportUnauthorized = errors.New("quality: authentication required")
	ErrReportUnavailable  = errors.New("quality: report unavailable")
)

// ReportCommand contains only document-free user claims. Ownership is derived
// from current account and signing proof, never supplied in the command.
type ReportCommand struct {
	CommandID        string `json:"command_id"`
	ReportID         string `json:"report_id"`
	Action           string `json:"action"`
	ExpectedRevision int32  `json:"expected_revision"`
	Kind             string `json:"kind,omitempty"`
	StationID        string `json:"station_id,omitempty"`
	Fuel             string `json:"fuel,omitempty"`
	OccurredOn       string `json:"occurred_on,omitempty"`
	Symptom          string `json:"symptom,omitempty"`
	Text             string `json:"text,omitempty"`
	RevisionID       string `json:"official_revision_id,omitempty"`
	SampleRef        string `json:"sample_ref,omitempty"`
}

func reportUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
		} else if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

func (c ReportCommand) Validate(now time.Time) error {
	if !reportUUID(c.CommandID) || !reportUUID(c.ReportID) {
		return ErrReportInvalid
	}
	if c.Action == "WITHDRAW" {
		if c.ExpectedRevision < 1 || c.Kind != "" || c.StationID != "" || c.Fuel != "" || c.OccurredOn != "" || c.Symptom != "" || c.Text != "" || c.RevisionID != "" || c.SampleRef != "" {
			return ErrReportInvalid
		}
		return nil
	}
	if (c.Action != "SUBMIT" && c.Action != "EDIT") || (c.Action == "SUBMIT" && c.ExpectedRevision != 0) || (c.Action == "EDIT" && c.ExpectedRevision < 1) {
		return ErrReportInvalid
	}
	if c.Kind != "EXPERIENCE" && c.Kind != "LINK_REVIEW" || !reportUUID(c.StationID) {
		return ErrReportInvalid
	}
	fuelOK := false
	for _, f := range AllFuels {
		if string(f) == c.Fuel {
			fuelOK = true
		}
	}
	if !fuelOK {
		return ErrReportInvalid
	}
	day, err := time.Parse("2006-01-02", c.OccurredOn)
	if err != nil || day.Format("2006-01-02") > now.UTC().Format("2006-01-02") {
		return ErrReportInvalid
	}
	switch c.Symptom {
	case "STARTING", "PERFORMANCE", "CONSUMPTION", "OTHER":
	default:
		return ErrReportInvalid
	}
	if !utf8.ValidString(c.Text) || strings.TrimSpace(c.Text) == "" || utf8.RuneCountInString(c.Text) > 1000 {
		return ErrReportInvalid
	}
	for _, r := range c.Text {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return ErrReportInvalid
		}
	}
	if (c.RevisionID == "") != (c.SampleRef == "") || c.Kind == "LINK_REVIEW" && c.SampleRef == "" {
		return ErrReportInvalid
	}
	if c.RevisionID != "" && (!reportUUID(c.RevisionID) || !utf8.ValidString(c.SampleRef) || len(c.SampleRef) > 128) {
		return ErrReportInvalid
	}
	return nil
}

// Hash binds canonical decoded fields; credentials never enter durable storage.
func (c ReportCommand) Hash() string {
	b, _ := json.Marshal(c)
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

type Report struct {
	ID                 string `json:"id"`
	Revision           int32  `json:"revision"`
	Kind               string `json:"kind"`
	StationID          string `json:"station_id"`
	Fuel               string `json:"fuel"`
	OccurredOn         string `json:"occurred_on"`
	Symptom            string `json:"symptom"`
	Text               string `json:"text"`
	OfficialRevisionID string `json:"official_revision_id,omitempty"`
	SampleRef          string `json:"sample_ref,omitempty"`
	Status             string `json:"status"`
	EvidenceLevel      string `json:"evidence_level"`
}

type ReportReceipt struct {
	ReportID string `json:"report_id"`
	Revision int32  `json:"revision"`
	Status   string `json:"status"`
}
