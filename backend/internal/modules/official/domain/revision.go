package domain

import (
	"errors"
	"time"
)

// Review thresholds, versioned with the gate (ANP_INGESTION stage 6).
// Comparisons are strict: exactly 1% quarantined or exactly 20% fewer rows
// still publishes; anything beyond needs an operator.
const (
	MaxQuarantineShare = 0.01
	MaxRowDropShare    = 0.20
)

// Run and revision lifecycle states.
const (
	RunRunning   = "running"
	RunCompleted = "completed"
	RunFailed    = "failed"

	RevisionStaging     = "staging"
	RevisionPublished   = "published"
	RevisionFailed      = "failed"
	RevisionNeedsReview = "needs-review"
)

var (
	ErrEmptyRevision   = errors.New("official: nothing staged to publish")
	ErrNeedsReview     = errors.New("official: operator review required")
	ErrUnknownRevision = errors.New("official: unknown revision")
	ErrDuplicateImport = errors.New("official: identical bytes already imported")
)

// Verdict is the review-gate outcome for one staged revision.
type Verdict struct {
	Publish bool
	Reason  string
}

// ReviewGate decides whether staged counts may publish. prevTotal is the
// comparable published row count with hasPrev false on first import.
// Quarantine share counts quarantined rows over staged + quarantined rows.
func ReviewGate(staged, quarantined int64, prevTotal int64, hasPrev bool) Verdict {
	total := staged + quarantined
	if total <= 0 {
		return Verdict{Reason: "empty"}
	}
	if float64(quarantined)/float64(total) > MaxQuarantineShare {
		return Verdict{Reason: "quarantine-share"}
	}
	if hasPrev && prevTotal > 0 {
		if float64(prevTotal-staged)/float64(prevTotal) > MaxRowDropShare {
			return Verdict{Reason: "row-drop"}
		}
	}
	return Verdict{Publish: true}
}

// ImportKey is the idempotency identity of one import attempt: same source
// bytes plus same parser version must never open a second revision.
type ImportKey struct {
	SourceURL      string
	SourceChecksum string
	ParserVersion  string
}

// PriceRow is one staged station price. Amounts arrive as validated
// milli-BRL from the kernel; the domain keeps the raw source text beside
// the exact value so evidence never depends on recomputation.
type PriceRow struct {
	StationID   string
	Product     string
	Unit        string
	AmountMilli int64
	RawText     string
	CollectedOn time.Time
	SourceRow   int
}

// QuarantineTally accumulates skip counts by reason code plus a bounded
// sample of offending rows for operator review.
type QuarantineTally struct {
	Counts  map[string]int64
	Samples []string
}

// Add records one skipped row; samples stop growing at 20 entries.
func (q *QuarantineTally) Add(reasonCode, sample string) {
	if q.Counts == nil {
		q.Counts = map[string]int64{}
	}
	q.Counts[reasonCode]++
	if len(q.Samples) < 20 {
		q.Samples = append(q.Samples, reasonCode+":"+sample)
	}
}

// Total returns the quarantined row count.
func (q *QuarantineTally) Total() int64 {
	var n int64
	for _, c := range q.Counts {
		n += c
	}
	return n
}
