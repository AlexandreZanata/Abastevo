package read

import (
	"context"
	"errors"
	quality "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/quality"
	"time"
)

// AtRevision returns an immutable request-local reader. Shared Reader instances
// remain safe under concurrent requests selecting different revisions.
func (r *Reader) AtRevision(id string) *Reader { return &Reader{pool: r.pool, revisionID: id} }
func (r *Reader) RevisionLease(ctx context.Context, now time.Time, source, stream, id string) (Lease, error) {
	if id == "" {
		return Lease{}, ErrNotFound
	}
	l, _, e := r.AtRevision(id).lease(ctx, source, stream, now)
	if errors.Is(e, ErrNoLease) {
		return l, nil
	}
	return l, e
}

type RevisionHeader struct {
	SupersedesRevisionID *string `json:"supersedes_revision_id"`
	ID                   string  `json:"revision_id"`
	Edition              string  `json:"edition"`
	Status               string  `json:"revision_status"`
	ProvenanceState      string  `json:"provenance_state"`
	IsCurrentStream      bool    `json:"is_current_stream"`
	PublishedAt          *string `json:"published_at"`
}

func (r *Reader) Revisions(ctx context.Context, source, stream, after string, limit int) ([]RevisionHeader, string, error) {
	if limit < 1 || limit > 100 {
		return nil, "", errors.New("read: page limit out of range")
	}
	if after == "" {
		after = "00000000-0000-0000-0000-000000000000"
	}
	id, e := mustUUID(after)
	if e != nil {
		return nil, "", e
	}
	rows, e := quality.New(r.pool).RevisionCatalog(ctx, quality.RevisionCatalogParams{Source: source, Stream: stream, AfterID: id, LimitPlusOne: int32(limit + 1)})
	if e != nil {
		return nil, "", e
	}
	next := ""
	if len(rows) > limit {
		rows = rows[:limit]
		next = uuidString(rows[limit-1].ID)
	}
	out := make([]RevisionHeader, 0, len(rows))
	for _, row := range rows {
		var at *string
		if row.PublishedAt.Valid {
			value := row.PublishedAt.Time.UTC().Format(time.RFC3339)
			at = &value
		}
		var supersedes *string
		if row.SupersedesRevisionID.Valid {
			value := uuidString(row.SupersedesRevisionID)
			supersedes = &value
		}
		out = append(out, RevisionHeader{SupersedesRevisionID: supersedes, ID: uuidString(row.ID), Edition: row.Edition, Status: row.Status, ProvenanceState: row.ProvenanceState, IsCurrentStream: row.IsCurrentStream, PublishedAt: at})
	}
	return out, next, nil
}

type RevisionResource struct {
	Member       string `json:"member_name,omitempty"`
	MemberSHA256 string `json:"member_sha256,omitempty"`
	MemberBytes  int64  `json:"member_bytes,omitempty"`
	URL          string `json:"url"`
	SHA256       string `json:"sha256"`
	Bytes        int64  `json:"bytes"`
}

func (r *Reader) RevisionResources(ctx context.Context, lease Lease) ([]RevisionResource, error) {
	out := make([]RevisionResource, 0)
	if !lease.HasPublished {
		return out, nil
	}
	id, e := mustUUID(lease.RevisionID)
	if e != nil {
		return nil, e
	}
	rows, e := quality.New(r.pool).ReadRevisionResources(ctx, id)
	if e != nil {
		return nil, e
	}
	for _, row := range rows {
		out = append(out, RevisionResource{URL: row.Url, SHA256: row.Sha256, Bytes: row.Bytes, Member: row.MemberName, MemberSHA256: row.MemberSha256.String, MemberBytes: row.MemberBytes.Int64})
	}
	return out, nil
}
