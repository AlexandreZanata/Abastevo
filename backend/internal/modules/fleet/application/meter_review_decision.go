package application

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"unicode"
	"unicode/utf8"

	meter "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain/meter"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
)

type MeterReviewDecisionKind string

const (
	MeterReviewAccept MeterReviewDecisionKind = "ACCEPT"
	MeterReviewRefuse MeterReviewDecisionKind = "REFUSE"
)

type MeterReviewDecisionInput struct {
	private
	OperationID, ReviewItemID string
	ExpectedRevision          int64
	Decision                  MeterReviewDecisionKind
	Reason                    string
}
type MeterReviewDecision struct {
	private
	value MeterReviewDecisionInput
}

func NewMeterReviewDecision(in MeterReviewDecisionInput) (MeterReviewDecision, error) {
	if !canonicalID(in.OperationID) || !canonicalID(in.ReviewItemID) || in.ExpectedRevision < 1 || in.ExpectedRevision == math.MaxInt64 || in.Decision != MeterReviewAccept && in.Decision != MeterReviewRefuse {
		return MeterReviewDecision{}, meter.ErrInvalid
	}
	if in.Decision == MeterReviewRefuse && strings.TrimSpace(in.Reason) == "" || !utf8.ValidString(in.Reason) || utf8.RuneCountInString(in.Reason) > 280 {
		return MeterReviewDecision{}, meter.ErrInvalid
	}
	if in.Reason != "" && strings.TrimSpace(in.Reason) == "" {
		return MeterReviewDecision{}, meter.ErrInvalid
	}
	for _, r := range in.Reason {
		if unicode.IsControl(r) {
			return MeterReviewDecision{}, meter.ErrInvalid
		}
	}
	return MeterReviewDecision{value: in}, nil
}
func (v MeterReviewDecision) Snapshot() (MeterReviewDecisionInput, error) {
	copy, e := NewMeterReviewDecision(v.value)
	return copy.value, e
}
func (v MeterReviewDecision) Digest(scope kernel.OrgScope) string {
	in, e := v.Snapshot()
	if e != nil || scope.Validate() != nil {
		return ""
	}
	raw, e := json.Marshal([]any{1, "meter.review.resolve", scope.OrgID().String(), in.OperationID, in.ReviewItemID, in.ExpectedRevision, in.Decision, in.Reason})
	if e != nil {
		return ""
	}
	return fmt.Sprintf("%x", sha256.Sum256(raw))
}
