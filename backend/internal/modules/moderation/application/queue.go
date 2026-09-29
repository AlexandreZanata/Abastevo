package application

import (
	"context"
	"errors"
	"strings"
	"time"

	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/moderation/domain"
)

// CaseStore is the owned persistence port for the queue: idempotent
// opens plus keyset listing. Implementations converge duplicate open
// targets instead of flooding.
type CaseStore interface {
	OpenCase(ctx context.Context, c domain.Case) (id string, replayed bool, err error)
	ListOpen(ctx context.Context, cursorRank int32, cursorAt time.Time, cursorID string, limit int32) ([]domain.Case, error)
}

// Ports wires case opening and queue reads through server clock and IDs.
type Ports struct {
	Clock func() time.Time
	NewID func() (string, error)
	Store CaseStore
}

// OpenDTO carries one case-open intent. Priority may be empty to accept
// the target default (contributor abuse triages P1, facts P2); explicit
// hints still validate through the domain.
type OpenDTO struct {
	TargetType string
	TargetID   string
	Priority   string
	Reason     string
	Detail     string
	EvidenceID string
}

// OpenResult is the safe acknowledgment: the case identity and whether
// the call converged on an already-open case.
type OpenResult struct {
	CaseID   string
	Replayed bool
}

// Open validates and persists one case. It authenticates nobody itself:
// workers open from dispute reports and abuse signals, and operator
// authentication lands with the P07-T02 callers that invoke actions.
func Open(ctx context.Context, p Ports, dto OpenDTO) (OpenResult, error) {
	priority := strings.TrimSpace(dto.Priority)
	if priority == "" {
		priority = domain.DefaultPriority(strings.TrimSpace(dto.TargetType))
	}
	now := time.Now()
	if p.Clock != nil {
		now = p.Clock()
	}
	id, err := p.NewID()
	if err != nil {
		return OpenResult{}, err
	}
	c, _, err := domain.NewCase(domain.CaseParams{
		ID: id, TargetType: strings.TrimSpace(dto.TargetType),
		TargetID: strings.TrimSpace(dto.TargetID), Priority: priority,
		Reason: dto.Reason, Detail: dto.Detail,
		EvidenceID: strings.TrimSpace(dto.EvidenceID),
		OpenedAt:   now,
	})
	if err != nil {
		return OpenResult{}, err
	}
	storedID, replayed, err := p.Store.OpenCase(ctx, c)
	if err != nil {
		return OpenResult{}, err
	}
	return OpenResult{CaseID: storedID, Replayed: replayed}, nil
}

// ListDTO carries one queue page: limit plus the opaque cursor of the
// last row. The first page sets HasCursor=false.
type ListDTO struct {
	Limit     int32
	HasCursor bool
	Rank      int32
	At        time.Time
	ID        string
}

// ListResult is one queue page with its next cursor. Empty NextID means
// the queue drained; callers pass the cursor back verbatim.
type ListResult struct {
	Cases    []domain.Case
	NextRank int32
	NextAt   time.Time
	NextID   string
}

// List serves the operator queue with minimal sensitive fields: case
// identifiers, priority, reason and the evidence reference only. No
// coordinates, media bytes, IPs or URLs ever leave this path (B-BR-011).
func List(ctx context.Context, p Ports, dto ListDTO) (ListResult, error) {
	limit := dto.Limit
	if limit < 1 || limit > 100 {
		if dto.Limit == 0 {
			limit = 20
		} else {
			return ListResult{}, errors.New("moderation: page limit out of range")
		}
	}
	rank := int32(-1)
	var at time.Time
	var id string
	if dto.HasCursor {
		rank = dto.Rank
		at = dto.At
		id = dto.ID
	}
	cases, err := p.Store.ListOpen(ctx, rank, at, id, limit)
	if err != nil {
		return ListResult{}, err
	}
	res := ListResult{Cases: cases}
	if len(cases) > 0 {
		last := cases[len(cases)-1]
		res.NextRank = int32(domain.PriorityRank(last.Priority))
		res.NextAt = last.OpenedAt
		res.NextID = last.ID
	}
	return res, nil
}
