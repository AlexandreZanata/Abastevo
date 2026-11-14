package application_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/document/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/document/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	mediaapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgmedia/application"
	media "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgmedia/domain"
)

const (
	orgID     = "7b0e2a54-0000-4000-8000-000000020501"
	accountID = "7b0e2a54-0000-4000-8000-000000020502"
	memberID  = "7b0e2a54-0000-4000-8000-000000020503"
	docID     = "7b0e2a54-0000-4000-8000-000000020504"
	opID      = "7b0e2a54-0000-4000-8000-000000020505"
	storedOp  = "7b0e2a54-0000-4000-8000-000000020506"
	digest    = "5d41402abc4b2a76b9719d911017c592ab5d41402abc4b2a76b9719d911017c5"
)

func scope(t *testing.T) kernel.OrgScope {
	t.Helper()
	id, err := kernel.ParseOrgID(orgID)
	if err != nil {
		t.Fatal(err)
	}
	s, err := kernel.NewOrgScope(id)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func actor() app.Actor {
	return app.Actor{AccountID: accountID, MembershipID: memberID, Channel: "web"}
}

func command() app.Create {
	return app.Create{OperationID: opID, DocumentID: docID, Original: app.Declaration{Kind: app.KindReceipt, SHA256: digest, Bytes: 1024, MediaType: app.MediaTypeJPEG}}
}

// now is the fixed service clock; a fresh declaration is created at it.
var now = time.Unix(1_800_000_000, 0).UTC()

func view(storage domain.State) app.View {
	return app.View{ID: docID, Kind: app.KindReceipt, DataMode: app.DataMode, OperationID: opID, CreatedBy: memberID,
		Original: command().Original, Statuses: domain.Statuses{Storage: storage, Extraction: domain.ExtractionNone, Structure: domain.NotApplicable,
			Conference: domain.Unconferred, Reconciliation: domain.Unallocated, Fiscal: domain.NotConsulted}, Revisions: app.Revisions{Storage: 1, Extraction: 1, Structure: 1, Conference: 1, Fiscal: 1},
		CreatedAt: now, UpdatedAt: now}
}

type fakeRepository struct {
	created, pending  app.View
	createErr         error
	pendingErr        error
	receivedErr       error
	refuseErr         error
	replayed          bool
	creates, received int
	receivedOperation string
	refusals          []app.Reason
	refusedOperation  string
}

func (r *fakeRepository) Create(_ context.Context, _ kernel.OrgScope, _ app.Actor, _ app.Create) (app.View, bool, error) {
	r.creates++
	return r.created, r.replayed, r.createErr
}
func (r *fakeRepository) Pending(context.Context, kernel.OrgScope, app.Actor, string) (app.View, error) {
	return r.pending, r.pendingErr
}
func (r *fakeRepository) MarkReceived(_ context.Context, _ kernel.OrgScope, _ app.Actor, _ string, operation string) (app.View, bool, error) {
	r.received++
	r.receivedOperation = operation
	if r.receivedErr != nil {
		return app.View{}, false, r.receivedErr
	}
	r.pending = view(domain.Received)
	return r.pending, false, nil
}
func (r *fakeRepository) Refuse(_ context.Context, _ kernel.OrgScope, _ app.Actor, _ string, reason app.Reason, operation string) (app.View, error) {
	r.refusals = append(r.refusals, reason)
	r.refusedOperation = operation
	if r.refuseErr != nil {
		return app.View{}, r.refuseErr
	}
	_, to, _ := reason.Transition()
	r.pending = view(to)
	return r.pending, nil
}
func (r *fakeRepository) Facts(context.Context, kernel.OrgScope, string) (app.Facts, error) {
	return app.Facts{TargetID: docID, AuthoredBy: memberID}, nil
}

type fakeMedia struct {
	uploadErr error
	uploads   int
	read      []byte
	actors    []app.Actor
	members   []media.Member
}

func (m *fakeMedia) Upload(ctx context.Context, _ kernel.OrgScope, member media.Member, _ string, body io.Reader) error {
	m.uploads++
	m.capture(ctx, member)
	m.read, _ = io.ReadAll(body)
	return m.uploadErr
}
func (m *fakeMedia) Issue(ctx context.Context, _ kernel.OrgScope, member media.Member, _ string) (mediaapp.Access, error) {
	m.capture(ctx, member)
	return mediaapp.Access{Token: "v1.synthetic-content-capability"}, nil
}
func (m *fakeMedia) Open(ctx context.Context, _ kernel.OrgScope, member media.Member, _ string, _ string) (mediaapp.Content, error) {
	m.capture(ctx, member)
	return mediaapp.Content{Bytes: []byte("synthetic"), MIME: app.MediaTypeJPEG}, nil
}
func (m *fakeMedia) capture(ctx context.Context, member media.Member) {
	bound, _ := app.ActorFrom(ctx)
	m.actors = append(m.actors, bound)
	m.members = append(m.members, member)
}

type fakeTokens struct {
	issued   int
	verified app.Actor
	err      error
}

func (f *fakeTokens) Issue(_ kernel.OrgScope, _ app.Actor, _ string, now time.Time) (app.UploadAccess, error) {
	f.issued++
	return app.UploadAccess{Token: "v1.synthetic-upload-capability", ExpiresAt: now.Add(app.UploadTTL)}, f.err
}
func (f *fakeTokens) Verify(kernel.OrgScope, string, string, time.Time) (app.Actor, error) {
	return f.verified, f.err
}

// fakeSafety stands in for the checker: it settles the repository's pending
// view to its outcome, as the real compare-and-set does.
type fakeSafety struct {
	repository *fakeRepository
	outcome    app.Outcome
	err        error
	calls      int
	finals     []bool
}

func (f *fakeSafety) Converge(_ context.Context, _ kernel.OrgScope, id string, final bool) (app.Outcome, error) {
	f.calls++
	f.finals = append(f.finals, final)
	if id != docID {
		return app.Outcome{}, app.ErrNotFound
	}
	if f.outcome.State != "" {
		f.repository.pending = view(f.outcome.State)
	}
	return f.outcome, f.err
}

func service(repository *fakeRepository, originals *fakeMedia, tokens *fakeTokens) app.Service {
	return app.Service{Enabled: true, Repository: repository, Media: originals, Safety: &fakeSafety{repository: repository, outcome: app.Outcome{State: domain.Stored}},
		Tokens: tokens, Now: func() time.Time { return now }, NewID: func() (string, error) { return storedOp, nil }}
}

func TestCreateValidatesTheC1DeclarationBeforeAnyWrite(t *testing.T) {
	repository := &fakeRepository{created: view(domain.Uploading)}
	tokens := &fakeTokens{}
	s := service(repository, &fakeMedia{}, tokens)
	for name, change := range map[string]func(*app.Create){
		"operation":         func(c *app.Create) { c.OperationID = "not-a-uuid" },
		"document":          func(c *app.Create) { c.DocumentID = strings.ToUpper(docID) },
		"digest-uppercase":  func(c *app.Create) { c.Original.SHA256 = strings.ToUpper(digest) },
		"digest-short":      func(c *app.Create) { c.Original.SHA256 = digest[:63] },
		"kind":              func(c *app.Create) { c.Original.Kind = "INVOICE" },
		"png-budget-only":   func(c *app.Create) { c.Original.MediaType = "image/png" },
		"pdf-budget-only":   func(c *app.Create) { c.Original.MediaType = "application/pdf" },
		"xml-budget-only":   func(c *app.Create) { c.Original.MediaType = "application/xml" },
		"unknown-type":      func(c *app.Create) { c.Original.MediaType = "image/gif" },
		"zero-bytes":        func(c *app.Create) { c.Original.Bytes = 0 },
		"over-jpeg-budget":  func(c *app.Create) { c.Original.Bytes = 5<<20 + 1 },
		"negative-bytes":    func(c *app.Create) { c.Original.Bytes = -1 },
		"padded-media-type": func(c *app.Create) { c.Original.MediaType = " image/jpeg" },
	} {
		t.Run(name, func(t *testing.T) {
			c := command()
			change(&c)
			if _, err := s.Create(t.Context(), scope(t), actor(), c); !errors.Is(err, app.ErrInvalid) {
				t.Fatal("invalid declaration accepted", err)
			}
		})
	}
	if repository.creates != 0 || tokens.issued != 0 {
		t.Fatal("refused declaration reached storage or minted a capability")
	}
	edge := command()
	edge.Original.Bytes = 5 << 20
	if _, err := s.Create(t.Context(), scope(t), actor(), edge); err != nil {
		t.Fatal("exact 5 MiB JPEG budget refused", err)
	}
}

func TestCreateIssuesUploadCapabilityOnlyWhilePending(t *testing.T) {
	repository := &fakeRepository{created: view(domain.Uploading)}
	tokens := &fakeTokens{}
	s := service(repository, &fakeMedia{}, tokens)
	first, err := s.Create(t.Context(), scope(t), actor(), command())
	if err != nil || first.Replayed || first.Upload == nil || first.Upload.Token == "" || first.View.ID != docID {
		t.Fatal("pending declaration without upload capability", err)
	}
	if !first.Upload.ExpiresAt.Equal(now.Add(5 * time.Minute)) {
		t.Fatal("upload capability deadline drift")
	}
	repository.replayed = true
	again, err := s.Create(t.Context(), scope(t), actor(), command())
	if err != nil || !again.Replayed || again.Upload == nil || tokens.issued != 2 {
		t.Fatal("pending replay must return the first record and a fresh capability", err)
	}
	repository.created = view(domain.Stored)
	stored, err := s.Create(t.Context(), scope(t), actor(), command())
	if err != nil || !stored.Replayed || stored.Upload != nil || tokens.issued != 2 {
		t.Fatal("stored replay must not mint an upload capability", err)
	}
	tokens.err = errors.New("synthetic token failure")
	repository.created = view(domain.Uploading)
	if _, err := s.Create(t.Context(), scope(t), actor(), command()); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("token failure not unavailable", err)
	}
	repository.createErr = app.ErrOperationReused
	if _, err := s.Create(t.Context(), scope(t), actor(), command()); !errors.Is(err, app.ErrOperationReused) {
		t.Fatal("repository refusal not passed through", err)
	}
}

// EMP-DOC-014 (B-BR-EMP-251): verified bytes are RECEIVED once and only the
// safety check makes them STORED; integrity refusals leave UPLOADING.
func TestUploadReceivesVerifiedBytesAndStoresOnlyThroughTheCheck(t *testing.T) {
	repository := &fakeRepository{pending: view(domain.Uploading)}
	originals := &fakeMedia{}
	s := service(repository, originals, &fakeTokens{})
	safety := s.Safety.(*fakeSafety)
	stored, err := s.Upload(t.Context(), scope(t), actor(), docID, strings.NewReader("synthetic bytes"))
	if err != nil || stored.Replayed || stored.View.Statuses.Storage != domain.Stored || repository.received != 1 || repository.receivedOperation != storedOp {
		t.Fatal("verified upload did not pass RECEIVED and the check", err)
	}
	if safety.calls != 1 || safety.finals[0] {
		t.Fatal("the inline check must run once and never as a final attempt")
	}
	if originals.uploads != 1 || string(originals.read) != "synthetic bytes" || len(originals.actors) != 1 || originals.actors[0] != actor() || originals.members[0] != actor().Member() {
		t.Fatal("bytes did not pass through orgmedia with the bound actor")
	}
	for _, refusal := range []error{media.ErrIntegrity, media.ErrTooLarge, media.ErrNotFound, media.ErrForbidden, media.ErrUnavailable, media.ErrInvalid} {
		repository.pending = view(domain.Uploading)
		originals.uploadErr = refusal
		if _, err := s.Upload(t.Context(), scope(t), actor(), docID, strings.NewReader("x")); !errors.Is(err, refusal) {
			t.Fatal("orgmedia refusal changed", refusal, err)
		}
	}
	if repository.received != 1 || len(repository.refusals) != 0 || safety.calls != 1 {
		t.Fatal("an unverified transfer was received, refused terminally or checked")
	}
}

// B-BR-EMP-252: a content verdict on a verified snapshot is terminal and
// mapped to its closed reason; nothing is received or checked.
func TestRefusedVerifiedContentClosesTheDocumentWithItsReason(t *testing.T) {
	for verdict, reason := range map[error]app.Reason{media.ErrUnsafe: app.UnsafeContent, media.ErrUnsupported: app.UnsupportedContent} {
		repository := &fakeRepository{pending: view(domain.Uploading)}
		s := service(repository, &fakeMedia{uploadErr: verdict}, &fakeTokens{})
		if _, err := s.Upload(t.Context(), scope(t), actor(), docID, strings.NewReader("x")); !errors.Is(err, verdict) {
			t.Fatal("content verdict changed", verdict, err)
		}
		if len(repository.refusals) != 1 || repository.refusals[0] != reason || repository.refusedOperation != storedOp || repository.received != 0 || s.Safety.(*fakeSafety).calls != 0 {
			t.Fatal("refused content not closed once with its reason", verdict, repository.refusals)
		}
		repository.pending = view(domain.Uploading)
		repository.refuseErr = app.ErrUnavailable
		if _, err := s.Upload(t.Context(), scope(t), actor(), docID, strings.NewReader("x")); !errors.Is(err, app.ErrUnavailable) {
			t.Fatal("a failed refusal write must not answer the verdict", err)
		}
	}
	repository := &fakeRepository{pending: view(domain.Uploading)}
	s := service(repository, &fakeMedia{uploadErr: media.ErrUnsafe}, &fakeTokens{})
	s.NewID = func() (string, error) { return "", errors.New("synthetic id failure") }
	if _, err := s.Upload(t.Context(), scope(t), actor(), docID, strings.NewReader("x")); !errors.Is(err, app.ErrUnavailable) || len(repository.refusals) != 0 {
		t.Fatal("terminal refusal without a server operation", err)
	}
}

// The check's outcome decides the answer: STORED, RECEIVED while it is
// incomplete, or the refusal its reason names.
func TestUploadAnswersTheCheckOutcome(t *testing.T) {
	for _, c := range []struct {
		outcome app.Outcome
		err     error
		storage domain.State
		want    error
	}{
		{app.Outcome{State: domain.Stored}, nil, domain.Stored, nil},
		{app.Outcome{State: domain.Received}, app.ErrRetry, domain.Received, nil},
		{app.Outcome{State: domain.Quarantined, Reason: app.UnsafeContent}, nil, "", media.ErrUnsafe},
		{app.Outcome{State: domain.StorageFailed, Reason: app.UnsupportedContent}, nil, "", media.ErrUnsupported},
		{app.Outcome{State: domain.StorageFailed, Reason: app.IntegrityMismatch}, app.ErrRetry, "", media.ErrIntegrity},
		{app.Outcome{State: domain.StorageFailed, Reason: app.CheckExhausted}, nil, "", app.ErrConflict},
	} {
		repository := &fakeRepository{pending: view(domain.Uploading)}
		s := service(repository, &fakeMedia{}, &fakeTokens{})
		s.Safety.(*fakeSafety).outcome, s.Safety.(*fakeSafety).err = c.outcome, c.err
		stored, err := s.Upload(t.Context(), scope(t), actor(), docID, strings.NewReader("x"))
		if !errors.Is(err, c.want) || (c.want == nil && stored.View.Statuses.Storage != c.storage) || (c.want != nil && stored.View.ID != "") {
			t.Fatal("check outcome answered wrongly", c.outcome, err)
		}
	}
	repository := &fakeRepository{pending: view(domain.Uploading)}
	s := service(repository, &fakeMedia{}, &fakeTokens{})
	s.Safety.(*fakeSafety).err = app.ErrUnavailable
	s.Safety.(*fakeSafety).outcome = app.Outcome{}
	if stored, err := s.Upload(t.Context(), scope(t), actor(), docID, strings.NewReader("x")); err != nil || stored.View.Statuses.Storage != domain.Received {
		t.Fatal("a check outage must leave the received bytes for the job", err)
	}
	repository = &fakeRepository{pending: view(domain.Uploading), receivedErr: app.ErrConflict}
	s = service(repository, &fakeMedia{}, &fakeTokens{})
	if _, err := s.Upload(t.Context(), scope(t), actor(), docID, strings.NewReader("x")); !errors.Is(err, app.ErrConflict) || s.Safety.(*fakeSafety).calls != 0 {
		t.Fatal("check ran without a received record", err)
	}
}

type countingReader struct{ reads int }

func (r *countingReader) Read([]byte) (int, error) { r.reads++; return 0, io.EOF }

func TestUploadReplayAndRefusalsReadNoBytes(t *testing.T) {
	repository := &fakeRepository{pending: view(domain.Stored)}
	originals := &fakeMedia{}
	s := service(repository, originals, &fakeTokens{})
	safety := s.Safety.(*fakeSafety)
	body := &countingReader{}
	stored, err := s.Upload(t.Context(), scope(t), actor(), docID, body)
	if err != nil || !stored.Replayed || originals.uploads != 0 || repository.received != 0 || body.reads != 0 || safety.calls != 0 {
		t.Fatal("upload after STORED must return the stored record and write nothing", err)
	}
	// A retry while the check is pending runs the check, never the transfer.
	repository.pending = view(domain.Received)
	stored, err = s.Upload(t.Context(), scope(t), actor(), docID, body)
	if err != nil || !stored.Replayed || stored.View.Statuses.Storage != domain.Stored || safety.calls != 1 || safety.finals[0] || originals.uploads != 0 || body.reads != 0 {
		t.Fatal("retry of a received upload did not converge it", err)
	}
	for _, state := range []domain.State{domain.Quarantined, domain.StorageFailed, domain.Deleted, domain.Replaced} {
		repository.pending = view(state)
		if _, err := s.Upload(t.Context(), scope(t), actor(), docID, body); !errors.Is(err, app.ErrConflict) {
			t.Fatal("terminal storage accepted bytes", state, err)
		}
	}
	repository.pending, repository.pendingErr = app.View{}, app.ErrNotFound
	if _, err := s.Upload(t.Context(), scope(t), actor(), docID, body); !errors.Is(err, app.ErrNotFound) {
		t.Fatal("another member's or foreign document accepted bytes", err)
	}
	repository.pending, repository.pendingErr = view(domain.Uploading), nil
	for name, change := range map[string]func(*app.Service){
		"media":  func(s *app.Service) { s.Media = nil },
		"safety": func(s *app.Service) { s.Safety = nil },
	} {
		broken := s
		change(&broken)
		if _, err := broken.Upload(t.Context(), scope(t), actor(), docID, body); !errors.Is(err, app.ErrUnavailable) {
			t.Fatal("unconfigured upload accepted bytes", name, err)
		}
	}
	repository.pending = view(domain.Received)
	noSafety := s
	noSafety.Safety = nil
	if _, err := noSafety.Upload(t.Context(), scope(t), actor(), docID, body); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("received retry without the check", err)
	}
	repository.pending = view(domain.Uploading)
	s.NewID = func() (string, error) { return "", errors.New("synthetic id failure") }
	if _, err := s.Upload(t.Context(), scope(t), actor(), docID, strings.NewReader("x")); !errors.Is(err, app.ErrUnavailable) || repository.received != 0 {
		t.Fatal("received transition without server operation", err)
	}
	if body.reads != 0 || originals.uploads != 1 {
		t.Fatal("refused upload read bytes")
	}
	if _, err := s.Upload(t.Context(), scope(t), actor(), "not-a-uuid", body); !errors.Is(err, app.ErrNotFound) {
		t.Fatal("invalid document selector", err)
	}
}

// B-BR-EMP-254: a declaration replay mints a capability only if it expires
// by the session close, 24 hours after the declaration.
func TestUploadCapabilityNeverOutlivesTheUploadSession(t *testing.T) {
	if app.UploadWindow != 24*time.Hour || app.UploadGrace != 5*time.Minute || app.CheckDelay != time.Minute || app.UploadTTL != 5*time.Minute {
		t.Fatal("upload session constants drifted from B-BR-EMP-254")
	}
	repository := &fakeRepository{created: view(domain.Uploading)}
	tokens := &fakeTokens{}
	s := service(repository, &fakeMedia{}, tokens)
	repository.replayed = true
	repository.created.CreatedAt = now.Add(-(app.UploadWindow - app.UploadTTL))
	edge, err := s.Create(t.Context(), scope(t), actor(), command())
	if err != nil || edge.Upload == nil || edge.Upload.ExpiresAt.After(repository.created.CreatedAt.Add(app.UploadWindow)) {
		t.Fatal("capability at the session edge", err)
	}
	repository.created.CreatedAt = repository.created.CreatedAt.Add(-time.Second)
	closed, err := s.Create(t.Context(), scope(t), actor(), command())
	if err != nil || !closed.Replayed || closed.Upload != nil || tokens.issued != 1 {
		t.Fatal("capability outlives the upload session", err)
	}
}

func TestVerifyUploadAndContentBindTheVerifiedActor(t *testing.T) {
	tokens := &fakeTokens{verified: actor()}
	originals := &fakeMedia{}
	s := service(&fakeRepository{}, originals, tokens)
	got, err := s.VerifyUpload(scope(t), docID, "v1.synthetic-upload-capability")
	if err != nil || got != actor() {
		t.Fatal("verified capability actor", err)
	}
	tokens.err = errors.New("synthetic expired capability")
	if _, err := s.VerifyUpload(scope(t), docID, "v1.synthetic-upload-capability"); !errors.Is(err, app.ErrNotFound) {
		t.Fatal("refused capability must look not found", err)
	}
	tokens.err, tokens.verified = nil, app.Actor{AccountID: accountID}
	if _, err := s.VerifyUpload(scope(t), docID, "v1.synthetic-upload-capability"); !errors.Is(err, app.ErrNotFound) {
		t.Fatal("incomplete capability actor accepted", err)
	}
	if _, err := s.VerifyUpload(scope(t), "not-a-uuid", "token"); !errors.Is(err, app.ErrNotFound) {
		t.Fatal("invalid selector", err)
	}
	if access, err := s.IssueContent(t.Context(), scope(t), actor(), docID); err != nil || access.Token == "" {
		t.Fatal("content issue", err)
	}
	if content, err := s.ReadContent(t.Context(), scope(t), actor(), docID, "v1.synthetic-content-capability"); err != nil || string(content.Bytes) != "synthetic" {
		t.Fatal("content read", err)
	}
	if len(originals.actors) != 2 || originals.actors[0] != actor() || originals.actors[1] != actor() {
		t.Fatal("content calls did not bind the verified actor")
	}
	s.Media = nil
	if _, err := s.IssueContent(t.Context(), scope(t), actor(), docID); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("unconfigured content issue", err)
	}
	if _, err := s.ReadContent(t.Context(), scope(t), actor(), docID, "token"); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("unconfigured content read", err)
	}
}

func TestServiceIsDefaultOffAndRefusesInvalidActors(t *testing.T) {
	var off app.Service
	if _, err := off.Create(t.Context(), scope(t), actor(), command()); !errors.Is(err, app.ErrDisabled) {
		t.Fatal("default service enabled", err)
	}
	if _, err := off.VerifyUpload(scope(t), docID, "token"); !errors.Is(err, app.ErrDisabled) {
		t.Fatal("default ingress enabled", err)
	}
	if _, err := off.Upload(t.Context(), scope(t), actor(), docID, nil); !errors.Is(err, app.ErrDisabled) {
		t.Fatal("default upload enabled", err)
	}
	if _, err := off.IssueContent(t.Context(), scope(t), actor(), docID); !errors.Is(err, app.ErrDisabled) {
		t.Fatal("default content enabled", err)
	}
	if _, err := off.ReadContent(t.Context(), scope(t), actor(), docID, "token"); !errors.Is(err, app.ErrDisabled) {
		t.Fatal("default content enabled", err)
	}
	if _, err := (app.Service{Enabled: true}).Create(t.Context(), scope(t), actor(), command()); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("missing repository", err)
	}
	if _, err := (app.Service{Enabled: true, Repository: &fakeRepository{}}).Create(t.Context(), scope(t), actor(), command()); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("missing upload tokens", err)
	}
	s := service(&fakeRepository{created: view(domain.Uploading)}, &fakeMedia{}, &fakeTokens{})
	for name, bad := range map[string]app.Actor{
		"account":           {MembershipID: memberID, Channel: "web"},
		"membership":        {AccountID: accountID, Channel: "web"},
		"channel":           {AccountID: accountID, MembershipID: memberID, Channel: "api"},
		"web-with-device":   {AccountID: accountID, MembershipID: memberID, Channel: "web", DeviceKeyID: docID},
		"android-unsigned":  {AccountID: accountID, MembershipID: memberID, Channel: "android"},
		"uppercase-account": {AccountID: strings.ToUpper(accountID), MembershipID: memberID, Channel: "web"},
	} {
		if _, err := s.Create(t.Context(), scope(t), bad, command()); !errors.Is(err, app.ErrNotFound) {
			t.Fatal("invalid actor accepted", name, err)
		}
	}
	if (app.Actor{AccountID: accountID, MembershipID: memberID, Channel: "android", ContributorID: opID, DeviceKeyID: docID}).Validate() != nil {
		t.Fatal("signed device actor refused")
	}
	if _, ok := app.ActorFrom(t.Context()); ok {
		t.Fatal("unbound context supplied an actor")
	}
	if _, ok := app.ActorFrom(app.WithActor(t.Context(), app.Actor{AccountID: accountID})); ok {
		t.Fatal("invalid bound actor accepted")
	}
}

func TestPrivateValuesNeverFormatIdentifiersDigestsOrCapabilities(t *testing.T) {
	upload := app.UploadAccess{Token: "v1.synthetic-upload-capability", ExpiresAt: time.Unix(1, 0)}
	values := []any{actor(), command(), command().Original, view(domain.Stored), upload, app.Created{View: view(domain.Uploading), Upload: &upload}, app.Stored{View: view(domain.Stored)},
		app.Subject{Object: media.Object{OrgID: orgID, DocumentID: docID, SHA256: digest, MIME: app.MediaTypeJPEG, Bytes: 1024, Revision: 2, Synthetic: true}, State: domain.Received, Revision: 2}}
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	for _, value := range values {
		raw, _ := json.Marshal(value)
		logger.Info("synthetic", "value", value)
		for _, out := range []string{fmt.Sprint(value), fmt.Sprintf("%+v", value), fmt.Sprintf("%#v", value), string(raw), logs.String()} {
			for _, secret := range []string{accountID, memberID, docID, opID, digest, "synthetic-upload-capability", "image/jpeg"} {
				if strings.Contains(out, secret) {
					t.Fatalf("private document value leaked %q", secret)
				}
			}
		}
	}
	if got := command().Digest(scope(t), actor()); len(got) != 64 || strings.Contains(got, digest) {
		t.Fatal("request digest malformed")
	}
	changed := command()
	changed.Original.Bytes++
	other := actor()
	other.MembershipID = opID
	if command().Digest(scope(t), actor()) == changed.Digest(scope(t), actor()) || command().Digest(scope(t), actor()) == command().Digest(scope(t), other) {
		t.Fatal("request digest ignores command facts or actor")
	}
	if app.AttachPolicy().Validate() != nil || app.ContentPolicy().Validate() != nil || app.AttachPolicy().Effect.Write() == false || app.ContentPolicy().Effect.Write() {
		t.Fatal("source-owned document policies invalid")
	}
}
