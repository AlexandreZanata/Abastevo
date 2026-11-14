//go:build integration

package adapters_test

import (
	"errors"
	"sync"
	"testing"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/document/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/document/domain"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	roles "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
)

func TestDocumentCreateIsAuthorizedIdempotentAndScoped(t *testing.T) {
	f := newDocFixture(t)
	raw := syntheticJPEG(t)
	command := declare(t, raw)
	view, replayed, err := f.store.Create(t.Context(), f.scopeA, f.driver, command)
	if err != nil || replayed {
		t.Fatal("driver could not declare an own document", err)
	}
	want := app.Revisions{Storage: 1, Extraction: 1, Structure: 1, Conference: 1, Fiscal: 1}
	if view.ID != command.DocumentID || view.Kind != app.KindReceipt || view.DataMode != app.DataMode || view.OperationID != command.OperationID ||
		view.CreatedBy != f.driver.MembershipID || view.Original != command.Original || view.Revisions != want || view.FiscalConsultedAt != nil ||
		view.Statuses != (domain.Statuses{Storage: domain.Uploading, Extraction: domain.ExtractionNone, Structure: domain.NotApplicable, Conference: domain.Unconferred, Reconciliation: domain.Unallocated, Fiscal: domain.NotConsulted}) ||
		view.CreatedAt.IsZero() || !view.UpdatedAt.Equal(view.CreatedAt) {
		t.Fatal("declared document does not start at the initial persisted container")
	}
	events := `SELECT count(*) FROM org_audit_events WHERE action='document.upload.created' AND target_id=$1`
	if f.count(t, f.scopeA, events, command.DocumentID) != 1 {
		t.Fatal("declaration not audited exactly once")
	}
	again, replayed, err := f.store.Create(t.Context(), f.scopeA, f.driver, command)
	if err != nil || !replayed || again.ID != view.ID || !again.CreatedAt.Equal(view.CreatedAt) || f.count(t, f.scopeA, `SELECT count(*) FROM org_documents`) != 1 || f.count(t, f.scopeA, events, command.DocumentID) != 1 {
		t.Fatal("identical retry created another record", err)
	}
	changed := command
	changed.Original.Bytes++
	if _, _, err := f.store.Create(t.Context(), f.scopeA, f.driver, changed); !errors.Is(err, app.ErrOperationReused) {
		t.Fatal("changed facts under the same operation accepted", err)
	}
	if _, _, err := f.store.Create(t.Context(), f.scopeA, f.manager, command); !errors.Is(err, app.ErrOperationReused) {
		t.Fatal("another member replayed the operation", err)
	}
	reuse := declare(t, raw)
	reuse.DocumentID = command.DocumentID
	if _, _, err := f.store.Create(t.Context(), f.scopeA, f.driver, reuse); !errors.Is(err, app.ErrConflict) {
		t.Fatal("second operation took an existing document identity", err)
	}
	if f.count(t, f.scopeA, `SELECT count(*) FROM org_audit_events`) != 1 || f.count(t, f.scopeA, `SELECT count(*) FROM org_documents`) != 1 {
		t.Fatal("refused declarations left audit or document rows")
	}
	// The same operation and document UUIDs in organization B are independent.
	if _, replayed, err := f.store.Create(t.Context(), f.scopeB, f.foreign, command); err != nil || replayed {
		t.Fatal("colliding identifiers across organizations", err)
	}
	if _, err := f.store.Pending(t.Context(), f.scopeB, f.driver, command.DocumentID); !errors.Is(err, app.ErrNotFound) && !errors.Is(err, access.ErrNoMembership) {
		t.Fatal("foreign member resolved another organization's document", err)
	}
	// Approvers hold no attach permission; non-members have no membership.
	var denied *roles.PermissionDenial
	if _, _, err := f.store.Create(t.Context(), f.scopeA, f.approver, declare(t, raw)); !errors.As(err, &denied) || denied.Permission() != "doc.document.attach" {
		t.Fatal("role without attach declared a document", err)
	}
	if _, _, err := f.store.Create(t.Context(), f.scopeA, f.foreign, declare(t, raw)); !errors.Is(err, access.ErrNoMembership) {
		t.Fatal("non-member declared a document", err)
	}
	f.authority.set(f.scopeA, f.supervisor, func(s *roleState) { s.suspended = true })
	if _, _, err := f.store.Create(t.Context(), f.scopeA, f.supervisor, declare(t, raw)); !errors.Is(err, access.ErrMembershipSuspended) {
		t.Fatal("suspended member declared a document", err)
	}
	if f.count(t, f.scopeA, `SELECT count(*) FROM org_documents`) != 1 {
		t.Fatal("refused authority wrote a document")
	}
	facts, err := f.store.Facts(t.Context(), f.scopeA, command.DocumentID)
	if err != nil || facts != (app.Facts{TargetID: command.DocumentID, AuthoredBy: f.driver.MembershipID}) {
		t.Fatal("resource facts", err)
	}
	if _, err := f.store.Facts(t.Context(), f.scopeA, newID(t)); !errors.Is(err, app.ErrNotFound) {
		t.Fatal("missing document facts", err)
	}
}

func TestConcurrentDuplicateDeclarationsWriteOneRowAndOneEvent(t *testing.T) {
	f := newDocFixture(t)
	pool := f.db.RuntimePool(t, 12)
	f.store.Pool = pool
	command := declare(t, syntheticJPEG(t))
	var wg sync.WaitGroup
	results := make([]error, 12)
	replays := make([]bool, 12)
	start := make(chan struct{})
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, replays[i], results[i] = f.store.Create(t.Context(), f.scopeA, f.driver, command)
		}(i)
	}
	close(start)
	wg.Wait()
	first := 0
	for i, err := range results {
		if err != nil {
			t.Fatal("concurrent identical declaration refused", err)
		}
		if !replays[i] {
			first++
		}
	}
	if first != 1 || f.count(t, f.scopeA, `SELECT count(*) FROM org_documents`) != 1 || f.count(t, f.scopeA, `SELECT count(*) FROM org_audit_events`) != 1 {
		t.Fatal("concurrent duplicate declarations wrote more than one record", first)
	}
	// Concurrent different operations naming one document: exactly one wins.
	contested := declare(t, syntheticJPEG(t))
	wins, conflicts := 0, 0
	var mu sync.Mutex
	start = make(chan struct{})
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c := contested
			c.OperationID = newID(t)
			<-start
			_, _, err := f.store.Create(t.Context(), f.scopeA, f.driver, c)
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				wins++
			case errors.Is(err, app.ErrConflict):
				conflicts++
			default:
				t.Error("unexpected contested outcome", err)
			}
		}()
	}
	close(start)
	wg.Wait()
	if wins != 1 || conflicts != 7 || f.count(t, f.scopeA, `SELECT count(*) FROM org_documents`) != 2 || f.count(t, f.scopeA, `SELECT count(*) FROM org_audit_events`) != 2 {
		t.Fatal("contested document identity", wins, conflicts)
	}
}

func TestStoredTransitionIsCreatorBoundAuditedOnceAndReplaySafe(t *testing.T) {
	f := newDocFixture(t)
	command := declare(t, syntheticJPEG(t))
	if _, _, err := f.store.Create(t.Context(), f.scopeA, f.driver, command); err != nil {
		t.Fatal(err)
	}
	pending, err := f.store.Pending(t.Context(), f.scopeA, f.driver, command.DocumentID)
	if err != nil || pending.Statuses.Storage != domain.Uploading {
		t.Fatal("creator pending view", err)
	}
	for name, other := range map[string]app.Actor{"manager": f.manager, "approver": f.approver} {
		if _, err := f.store.Pending(t.Context(), f.scopeA, other, command.DocumentID); !errors.Is(err, app.ErrNotFound) {
			t.Fatal("another member opened the upload session", name, err)
		}
		if _, _, err := f.store.MarkStored(t.Context(), f.scopeA, other, command.DocumentID, newID(t)); !errors.Is(err, app.ErrNotFound) {
			t.Fatal("another member completed the upload", name, err)
		}
	}
	if _, _, err := f.store.MarkStored(t.Context(), f.scopeB, f.foreign, command.DocumentID, newID(t)); err == nil {
		t.Fatal("foreign organization completed the upload")
	}
	stored, replayed, err := f.store.MarkStored(t.Context(), f.scopeA, f.driver, command.DocumentID, newID(t))
	if err != nil || replayed || stored.Statuses.Storage != domain.Stored || stored.Revisions.Storage != 3 || stored.Revisions.Conference != 1 || !stored.UpdatedAt.After(stored.CreatedAt) {
		t.Fatal("verified upload did not move UPLOADING -> RECEIVED -> STORED", err, stored.Revisions)
	}
	storedEvents := `SELECT count(*) FROM org_audit_events WHERE action='document.upload.stored' AND target_id=$1 AND actor_id=$2`
	if f.count(t, f.scopeA, storedEvents, command.DocumentID, f.driver.AccountID) != 1 {
		t.Fatal("stored transition not audited once")
	}
	again, replayed, err := f.store.MarkStored(t.Context(), f.scopeA, f.driver, command.DocumentID, newID(t))
	if err != nil || !replayed || again.Revisions.Storage != 3 || f.count(t, f.scopeA, storedEvents, command.DocumentID, f.driver.AccountID) != 1 {
		t.Fatal("repeated completion wrote another record", err)
	}
	// Losing the attach permission closes the session even for the creator.
	f.authority.set(f.scopeA, f.driver, func(s *roleState) { s.role = roles.Auditor })
	var denied *roles.PermissionDenial
	if _, err := f.store.Pending(t.Context(), f.scopeA, f.driver, command.DocumentID); !errors.As(err, &denied) {
		t.Fatal("creator without attach permission reopened the session", err)
	}
}
