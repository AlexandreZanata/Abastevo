package main

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func tstamps(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}

func TestEvidenceCopyExpiryPolicy(t *testing.T) {
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	received := time.Date(2026, 9, 29, 11, 0, 0, 0, time.UTC)
	if !evidenceCopyExpired(tstamps(received), tstamps(received), now) {
		t.Error("25-hour copy must read expired")
	}
	fresh := time.Date(2026, 9, 29, 13, 0, 0, 0, time.UTC)
	if evidenceCopyExpired(tstamps(fresh), tstamps(fresh), now) {
		t.Error("23-hour copy must still read")
	}
	atDeadline := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	if !evidenceCopyExpired(tstamps(atDeadline), tstamps(atDeadline), now) {
		t.Error("copy exactly at deadline must read expired")
	}
	// Rows predating the migration backfill fall back to creation.
	if !evidenceCopyExpired(pgtype.Timestamptz{}, tstamps(received), now) {
		t.Error("null received_at must fall back to created_at")
	}
	// Zero stamps never refuse: missing history fails open to the
	// deleted-marker check, never to a fabricated deadline.
	if evidenceCopyExpired(pgtype.Timestamptz{}, pgtype.Timestamptz{}, now) {
		t.Error("zero stamps must not refuse")
	}
}
