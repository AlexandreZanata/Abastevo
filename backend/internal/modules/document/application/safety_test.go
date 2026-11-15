package application_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sync"
	"testing"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/document/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/document/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	media "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgmedia/domain"
)

var checkedBytes = []byte("synthetic checked original")

func checkedObject() media.Object {
	sum := sha256.Sum256(checkedBytes)
	return media.Object{OrgID: orgID, DocumentID: docID, SHA256: hex.EncodeToString(sum[:]), MIME: app.MediaTypeJPEG, Bytes: int64(len(checkedBytes)), Revision: 2, Synthetic: true}
}

type settlement struct {
	from      domain.State
	revision  int64
	reason    app.Reason
	operation string
}

// safetyRepository is a compare-and-set row: Settle writes only while the
// row is still at the expected state and revision.
type safetyRepository struct {
	mu         sync.Mutex
	subject    app.Subject
	subjectErr error
	settleErr  error
	settles    []settlement
	// race changes the row between Subject and Settle, as a concurrent
	// checker that committed first would.
	race func(*app.Subject)
}

func (r *safetyRepository) Subject(_ context.Context, _ kernel.OrgScope, id string) (app.Subject, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.subjectErr != nil {
		return app.Subject{}, r.subjectErr
	}
	if id != docID {
		return app.Subject{}, app.ErrNotFound
	}
	return r.subject, nil
}
func (r *safetyRepository) Settle(_ context.Context, _ kernel.OrgScope, _ string, from domain.State, revision int64, reason app.Reason, operation string) (app.Outcome, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.race != nil {
		r.race(&r.subject)
		r.race = nil
	}
	if r.settleErr != nil {
		return app.Outcome{}, r.settleErr
	}
	if r.subject.State != from || r.subject.Revision != revision {
		return app.Outcome{State: r.subject.State, Reason: r.subject.Reason}, nil
	}
	start, to, ok := reason.Transition()
	if !ok || start != from {
		return app.Outcome{}, errors.New("synthetic invalid settlement")
	}
	r.settles = append(r.settles, settlement{from, revision, reason, operation})
	r.subject.State, r.subject.Reason, r.subject.Revision = to, reason, revision+1
	return app.Outcome{State: to, Reason: reason}, nil
}

type safetyObjects struct {
	raw          []byte
	getErr       error
	deleteErr    error
	gets, delete int
	deleted      []media.Object
}

func (o *safetyObjects) Get(_ context.Context, _ kernel.OrgScope, object media.Object) ([]byte, error) {
	o.gets++
	if o.getErr != nil {
		return nil, o.getErr
	}
	return bytes.Clone(o.raw), nil
}
func (o *safetyObjects) Delete(_ context.Context, _ kernel.OrgScope, object media.Object) error {
	o.delete++
	o.deleted = append(o.deleted, object)
	return o.deleteErr
}

type safetyInspector struct {
	mime  string
	err   error
	edit  bool
	calls int
}

func (i *safetyInspector) Inspect(_ context.Context, raw []byte) (string, error) {
	i.calls++
	if i.edit {
		raw[0] ^= 1
	}
	return i.mime, i.err
}

func checker(state domain.State, revision int64) (app.Checker, *safetyRepository, *safetyObjects, *safetyInspector) {
	repository := &safetyRepository{subject: app.Subject{Object: checkedObject(), State: state, Revision: revision}}
	objects := &safetyObjects{raw: bytes.Clone(checkedBytes)}
	inspector := &safetyInspector{mime: app.MediaTypeJPEG}
	return app.Checker{Repository: repository, Objects: objects, Inspector: inspector, NewID: func() (string, error) { return storedOp, nil }}, repository, objects, inspector
}

// B-BR-EMP-251: only a verified, permitted stored object becomes STORED.
func TestCheckStoresOnlyAVerifiedPermittedObject(t *testing.T) {
	c, repository, objects, inspector := checker(domain.Received, 2)
	outcome, err := c.Converge(t.Context(), scope(t), docID, false)
	if err != nil || outcome != (app.Outcome{State: domain.Stored}) {
		t.Fatal("verified object not stored", outcome, err)
	}
	if len(repository.settles) != 1 || repository.settles[0] != (settlement{domain.Received, 2, "", storedOp}) || objects.gets != 1 || inspector.calls != 1 || objects.delete != 0 {
		t.Fatal("stored settlement", repository.settles)
	}
	// A converged document is never checked again.
	if again, err := c.Converge(t.Context(), scope(t), docID, true); err != nil || again.State != domain.Stored || objects.gets != 1 || len(repository.settles) != 1 {
		t.Fatal("repeated check did work again", err)
	}
}

// B-BR-EMP-252: every refusal is a terminal outcome with its closed reason;
// a failed object is removed, a quarantined one is kept.
func TestCheckClassifiesRefusedStoredContent(t *testing.T) {
	longer := append(bytes.Clone(checkedBytes), 'x')
	other := bytes.Clone(checkedBytes)
	other[0] ^= 1
	for name, c := range map[string]struct {
		raw     []byte
		getErr  error
		mime    string
		inspect error
		edit    bool
		want    app.Outcome
	}{
		"unsafe":            {inspect: media.ErrUnsafe, want: app.Outcome{State: domain.Quarantined, Reason: app.UnsafeContent}},
		"unsupported":       {inspect: media.ErrUnsupported, want: app.Outcome{State: domain.StorageFailed, Reason: app.UnsupportedContent}},
		"other-type":        {mime: "image/png", want: app.Outcome{State: domain.StorageFailed, Reason: app.UnsupportedContent}},
		"stored-mismatch":   {getErr: media.ErrIntegrity, want: app.Outcome{State: domain.StorageFailed, Reason: app.IntegrityMismatch}},
		"digest-differs":    {raw: other, want: app.Outcome{State: domain.StorageFailed, Reason: app.IntegrityMismatch}},
		"longer":            {raw: longer, want: app.Outcome{State: domain.StorageFailed, Reason: app.IntegrityMismatch}},
		"partial":           {raw: checkedBytes[:4], want: app.Outcome{State: domain.StorageFailed, Reason: app.IntegrityMismatch}},
		"inspector-edits":   {edit: true, want: app.Outcome{State: domain.StorageFailed, Reason: app.IntegrityMismatch}},
		"inspector-size":    {inspect: media.ErrTooLarge, want: app.Outcome{State: domain.StorageFailed, Reason: app.IntegrityMismatch}},
		"inspector-differs": {inspect: media.ErrIntegrity, want: app.Outcome{State: domain.StorageFailed, Reason: app.IntegrityMismatch}},
	} {
		t.Run(name, func(t *testing.T) {
			checker, repository, objects, inspector := checker(domain.Received, 2)
			if c.raw != nil {
				objects.raw = c.raw
			}
			objects.getErr = c.getErr
			inspector.err, inspector.edit = c.inspect, c.edit
			if c.mime != "" {
				inspector.mime = c.mime
			}
			outcome, err := checker.Converge(t.Context(), scope(t), docID, false)
			if err != nil || outcome != c.want || len(repository.settles) != 1 || repository.settles[0].reason != c.want.Reason {
				t.Fatal("refusal outcome", outcome, err)
			}
			removed := c.want.State == domain.StorageFailed
			if (objects.delete == 1) != removed || (removed && objects.deleted[0] != checkedObject()) {
				t.Fatal("object retention of the outcome", objects.delete)
			}
		})
	}
}

// B-BR-EMP-253: an outage leaves RECEIVED with nothing written; the final
// attempt fails as exhausted; a failed removal keeps FAILED and retries.
func TestIncompleteCheckRetriesThenExhausts(t *testing.T) {
	for name, change := range map[string]func(*safetyObjects, *safetyInspector, *app.Checker){
		"storage":   func(o *safetyObjects, _ *safetyInspector, _ *app.Checker) { o.getErr = media.ErrUnavailable },
		"missing":   func(o *safetyObjects, _ *safetyInspector, _ *app.Checker) { o.getErr = media.ErrNotFound },
		"inspector": func(_ *safetyObjects, i *safetyInspector, _ *app.Checker) { i.err = media.ErrUnavailable },
		"context":   func(_ *safetyObjects, i *safetyInspector, _ *app.Checker) { i.err = context.Canceled },
	} {
		t.Run(name, func(t *testing.T) {
			c, repository, objects, inspector := checker(domain.Received, 2)
			change(objects, inspector, &c)
			outcome, err := c.Converge(t.Context(), scope(t), docID, false)
			if !errors.Is(err, app.ErrRetry) || outcome.State != domain.Received || len(repository.settles) != 0 || objects.delete != 0 {
				t.Fatal("incomplete check wrote an outcome", outcome, err)
			}
			outcome, err = c.Converge(t.Context(), scope(t), docID, true)
			if err != nil || outcome != (app.Outcome{State: domain.StorageFailed, Reason: app.CheckExhausted}) || objects.delete != 1 {
				t.Fatal("final attempt did not settle exhausted", outcome, err)
			}
		})
	}
	c, repository, objects, _ := checker(domain.Received, 2)
	objects.getErr, objects.deleteErr = media.ErrUnavailable, media.ErrUnavailable
	outcome, err := c.Converge(t.Context(), scope(t), docID, true)
	if !errors.Is(err, app.ErrRetry) || outcome.State != domain.StorageFailed || len(repository.settles) != 1 {
		t.Fatal("failed removal must keep FAILED and ask for a retry", outcome, err)
	}
	objects.deleteErr = nil
	if outcome, err := c.Converge(t.Context(), scope(t), docID, false); err != nil || outcome.State != domain.StorageFailed || objects.delete != 2 || len(repository.settles) != 1 {
		t.Fatal("retried removal", outcome, err)
	}
	c, repository, _, _ = checker(domain.Received, 2)
	repository.settleErr = errors.New("synthetic database outage")
	if outcome, err := c.Converge(t.Context(), scope(t), docID, false); !errors.Is(err, app.ErrRetry) || outcome.State != domain.Received {
		t.Fatal("settlement outage", outcome, err)
	}
	c, repository, _, _ = checker(domain.Received, 2)
	c.NewID = func() (string, error) { return "not-a-uuid", nil }
	if _, err := c.Converge(t.Context(), scope(t), docID, false); !errors.Is(err, app.ErrRetry) || len(repository.settles) != 0 {
		t.Fatal("settlement without a server operation", err)
	}
}

// B-BR-EMP-253/254: the job acts on the current state only, converges with
// another settlement and closes only an expired session.
func TestConvergeActsOnTheCurrentStateOnly(t *testing.T) {
	for _, state := range []domain.State{domain.Stored, domain.Quarantined, domain.Deleted, domain.Replaced} {
		c, repository, objects, inspector := checker(state, 3)
		outcome, err := c.Converge(t.Context(), scope(t), docID, true)
		if err != nil || outcome.State != state || len(repository.settles) != 0 || objects.gets+objects.delete+inspector.calls != 0 {
			t.Fatal("settled document touched again", state, err)
		}
	}
	c, repository, objects, _ := checker(domain.StorageFailed, 3)
	repository.subject.Reason = app.UnsupportedContent
	if outcome, err := c.Converge(t.Context(), scope(t), docID, false); err != nil || outcome != (app.Outcome{State: domain.StorageFailed, Reason: app.UnsupportedContent}) || objects.delete != 1 || objects.gets != 0 {
		t.Fatal("failed document object not removed", err)
	}
	objects.deleteErr = media.ErrUnavailable
	if _, err := c.Converge(t.Context(), scope(t), docID, false); !errors.Is(err, app.ErrRetry) {
		t.Fatal("failed removal not retried", err)
	}
	c, repository, objects, _ = checker(domain.Uploading, 1)
	if outcome, err := c.Converge(t.Context(), scope(t), docID, true); !errors.Is(err, app.ErrNotDue) || outcome.State != domain.Uploading || len(repository.settles) != 0 || objects.delete != 0 {
		t.Fatal("open session closed", err)
	}
	repository.subject.Abandoned = true
	outcome, err := c.Converge(t.Context(), scope(t), docID, false)
	if err != nil || outcome != (app.Outcome{State: domain.StorageFailed, Reason: app.UploadAbandoned}) || len(repository.settles) != 1 ||
		repository.settles[0] != (settlement{domain.Uploading, 1, app.UploadAbandoned, storedOp}) || objects.delete != 1 || objects.gets != 0 {
		t.Fatal("abandoned session not closed with its object removed", outcome, err)
	}
	// Another checker committed first: the compare-and-set returns its
	// outcome, and nothing else is written or removed.
	c, repository, objects, _ = checker(domain.Received, 2)
	objects.getErr = media.ErrIntegrity
	repository.race = func(s *app.Subject) { s.State, s.Revision = domain.Stored, 3 }
	if outcome, err := c.Converge(t.Context(), scope(t), docID, false); err != nil || outcome.State != domain.Stored || len(repository.settles) != 0 || objects.delete != 0 {
		t.Fatal("lost compare-and-set did not converge", outcome, err)
	}
	c, repository, _, _ = checker(domain.Received, 2)
	repository.subjectErr = app.ErrNotFound
	if _, err := c.Converge(t.Context(), scope(t), docID, false); !errors.Is(err, app.ErrNotFound) {
		t.Fatal("missing document", err)
	}
	repository.subjectErr = errors.New("synthetic database outage")
	if _, err := c.Converge(t.Context(), scope(t), docID, false); !errors.Is(err, app.ErrRetry) {
		t.Fatal("subject outage", err)
	}
	if _, err := (app.Checker{}).Converge(t.Context(), scope(t), docID, false); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("unconfigured checker", err)
	}
	c, _, _, _ = checker(domain.Received, 2)
	for _, id := range []string{"not-a-uuid", ""} {
		if _, err := c.Converge(t.Context(), scope(t), id, false); !errors.Is(err, app.ErrNotFound) {
			t.Fatal("invalid selector", err)
		}
	}
	if _, err := c.Converge(t.Context(), kernel.OrgScope{}, docID, false); !errors.Is(err, app.ErrNotFound) {
		t.Fatal("invalid scope", err)
	}
}

func TestReasonsAreClosedAndFixedByTheirTransition(t *testing.T) {
	want := map[app.Reason][3]string{
		app.UnsafeContent:      {"RECEIVED", "QUARANTINED", "content.unsafe"},
		app.UnsupportedContent: {"RECEIVED", "FAILED", "content.unsupported"},
		app.IntegrityMismatch:  {"RECEIVED", "FAILED", "integrity.mismatch"},
		app.CheckExhausted:     {"RECEIVED", "FAILED", "check.exhausted"},
		app.UploadAbandoned:    {"UPLOADING", "FAILED", "upload.abandoned"},
		"":                     {"RECEIVED", "STORED", ""},
	}
	if len(app.Reasons()) != 5 {
		t.Fatal("closed reason set changed")
	}
	for _, reason := range append(app.Reasons(), "") {
		from, to, ok := reason.Transition()
		if !ok || [3]string{string(from), string(to), reason.Code()} != want[reason] {
			t.Fatal("reason transition", reason, from, to, reason.Code())
		}
		if reason != "" {
			if allowed, _ := domain.Allowed(domain.Storage, from); !containsState(allowed, to) {
				t.Fatal("reason outside the EMP-DOC-001 storage graph", reason)
			}
		}
	}
	for _, unknown := range []app.Reason{"unsafe_content", "VALIDATED", " UNSAFE_CONTENT"} {
		if _, _, ok := unknown.Transition(); ok || unknown.Code() != "" {
			t.Fatal("unknown reason accepted", unknown)
		}
	}
}

func containsState(states []domain.State, want domain.State) bool {
	for _, state := range states {
		if state == want {
			return true
		}
	}
	return false
}
