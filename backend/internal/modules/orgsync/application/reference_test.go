package application_test

import (
	"errors"
	"strings"
	"testing"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
)

func referenceAsset(id, name string) app.ReferenceAsset {
	return app.ReferenceAsset{ID: id, Revision: 1, Kind: "VEHICLE", UsageBasis: "DISTANCE", State: "ACTIVE", Name: name, Catalog: app.ReferenceCatalog, Products: []string{"DIESEL_S10"}, Meters: []app.ReferenceMeter{}, Assignment: app.ReferenceAssignment{Kind: "PRIMARY"}}
}

func TestReferenceDigestIsDeterministicAndContentBound(t *testing.T) {
	a := referenceAsset("7b0e2a54-0000-4000-8000-000000000001", "Synthetic truck")
	first, err := a.Digest()
	again, err2 := a.Digest()
	if err != nil || err2 != nil || first != again || len(first) != 64 || strings.ToLower(first) != first {
		t.Fatal("reference digest is not a stable lowercase SHA-256", err, err2)
	}
	b := a
	b.Name = "Synthetic truck 2"
	if changed, err := b.Digest(); err != nil || changed == first {
		t.Fatal("reference digest ignored a content change")
	}
	b = a
	b.Products = []string{"DIESEL_S10", "GASOLINE_C"}
	if changed, err := b.Digest(); err != nil || changed == first {
		t.Fatal("reference digest ignored a product change")
	}
}

func TestReferenceDiffLogsUpsertsAndTombstonesInEntityOrder(t *testing.T) {
	last := map[string]app.ReferenceEntry{
		"b": {Revision: 1, EntityType: app.ReferenceEntityAsset, EntityID: "b", Op: app.ReferenceUpsert, Digest: "d1"},
		"c": {Revision: 2, EntityType: app.ReferenceEntityAsset, EntityID: "c", Op: app.ReferenceUpsert, Digest: "d2"},
		"d": {Revision: 3, EntityType: app.ReferenceEntityAsset, EntityID: "d", Op: app.ReferenceDelete},
		"e": {Revision: 4, EntityType: app.ReferenceEntityAsset, EntityID: "e", Op: app.ReferenceDelete},
	}
	current := map[string]string{"a": "n1", "b": "d1", "c": "changed", "e": "back"}
	got := app.DiffReference(last, current)
	want := []app.ReferenceEntry{
		{EntityType: app.ReferenceEntityAsset, EntityID: "a", Op: app.ReferenceUpsert, Digest: "n1"},
		{EntityType: app.ReferenceEntityAsset, EntityID: "c", Op: app.ReferenceUpsert, Digest: "changed"},
		{EntityType: app.ReferenceEntityAsset, EntityID: "e", Op: app.ReferenceUpsert, Digest: "back"},
	}
	if len(got) != len(want) {
		t.Fatalf("diff %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("diff[%d] %+v want %+v", i, got[i], want[i])
		}
	}
	gone := app.DiffReference(last, map[string]string{})
	if len(gone) != 2 || gone[0].EntityID != "b" || gone[1].EntityID != "c" || gone[0].Op != app.ReferenceDelete || gone[0].Digest != "" {
		t.Fatalf("missing entities were not tombstoned once %+v", gone)
	}
	if len(app.DiffReference(nil, nil)) != 0 {
		t.Fatal("empty diff invented changes")
	}
}

func TestReferenceCursorIsSignedAndBound(t *testing.T) {
	secret := []byte(strings.Repeat("k", 32))
	cursor := app.ReferenceCursor{OrgID: "7b0e2a54-0000-4000-8000-000000000001", MembershipID: "7b0e2a54-0000-4000-8000-000000000002", EnrollmentID: "7b0e2a54-0000-4000-8000-000000000003", Epoch: "7b0e2a54-0000-4000-8000-000000000004", Revision: 7}
	raw, err := app.SignReferenceCursor(secret, cursor)
	if err != nil || raw == "" || len(raw) > 1024 {
		t.Fatal("cursor not signed", err)
	}
	opened, err := app.OpenReferenceCursor(secret, raw)
	if err != nil || opened != cursor {
		t.Fatal("cursor did not round-trip", err)
	}
	for _, bad := range []string{"", "x", raw + "a", "a" + raw, strings.Replace(raw, ".", "", 1), raw[:len(raw)-2]} {
		if _, err := app.OpenReferenceCursor(secret, bad); !errors.Is(err, app.ErrReferenceCursor) {
			t.Fatal("tampered cursor opened", bad)
		}
	}
	if _, err := app.OpenReferenceCursor([]byte(strings.Repeat("o", 32)), raw); !errors.Is(err, app.ErrReferenceCursor) {
		t.Fatal("cursor opened with another key")
	}
	for _, invalid := range []app.ReferenceCursor{{}, {OrgID: cursor.OrgID, MembershipID: cursor.MembershipID, EnrollmentID: cursor.EnrollmentID, Epoch: cursor.Epoch, Revision: -1}, {OrgID: "x", MembershipID: cursor.MembershipID, EnrollmentID: cursor.EnrollmentID, Epoch: cursor.Epoch}} {
		if _, err := app.SignReferenceCursor(secret, invalid); err == nil {
			t.Fatal("invalid cursor signed", invalid)
		}
	}
	if _, err := app.SignReferenceCursor(secret[:8], cursor); err == nil {
		t.Fatal("cursor signed with a short key")
	}
}

// A cursor has exactly one accepted spelling: an unpadded base64url segment
// whose final character carries unused bits must have them zero, so a
// tampered final character never opens the cursor it came from (the signed
// route's foreign-cursor test flipped such a character and passed or failed by
// chance).
func TestReferenceCursorAcceptsOnlyTheCanonicalSpelling(t *testing.T) {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"
	secret := []byte(strings.Repeat("k", 32))
	cursor := app.ReferenceCursor{OrgID: "7b0e2a54-0000-4000-8000-000000000001", MembershipID: "7b0e2a54-0000-4000-8000-000000000002", EnrollmentID: "7b0e2a54-0000-4000-8000-000000000003", Epoch: "7b0e2a54-0000-4000-8000-000000000004", Revision: 7}
	raw, err := app.SignReferenceCursor(secret, cursor)
	if err != nil {
		t.Fatal(err)
	}
	payload, signature, _ := strings.Cut(raw, ".")
	variant := func(segment string) (string, bool) {
		unused := map[int]int{2: 4, 3: 2}[len(segment)%4] // bits left unused by the final character
		if unused == 0 {
			return "", false
		}
		last := strings.IndexByte(alphabet, segment[len(segment)-1])
		return segment[:len(segment)-1] + string(alphabet[last^1]), true
	}
	tried := 0
	for _, alt := range []func() (string, bool){
		func() (string, bool) { s, ok := variant(signature); return payload + "." + s, ok },
		func() (string, bool) { s, ok := variant(payload); return s + "." + signature, ok },
	} {
		bad, ok := alt()
		if !ok {
			continue
		}
		tried++
		if _, err := app.OpenReferenceCursor(secret, bad); !errors.Is(err, app.ErrReferenceCursor) {
			t.Fatal("non-canonical cursor spelling opened", bad)
		}
	}
	if tried == 0 {
		t.Fatal("no segment carries unused bits; the 32-byte MAC always does")
	}
}
