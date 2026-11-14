package http

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"time"

	account "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/adapters/auth"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	member "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	guarded "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/adapters/http"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/go-chi/chi/v5"
)

const ReviewResultsPath = "/v1/orgs/{org_id}/sync/review-results"

type OwnReviewRepository interface {
	OwnMeterReviewResults(context.Context, app.Source, []protocol.ReviewSelector) ([]protocol.ReviewResult, error)
}
type ReviewResultsOptions struct {
	Enabled    bool
	Origin     string
	Accounts   *account.Service
	Verifier   auth.Verifier
	Devices    DeviceSource
	Repository OwnReviewRepository
}
type reviewResultsEndpoint struct{ ReviewResultsOptions }

func (reviewResultsEndpoint) SignedReviewResultsRule() orgapi.SignedReviewResultsRule {
	return orgapi.SignedReviewResultsRule{AccountProof: true, Signed: true, ExactEnrollment: true, OriginalAuthor: true, WholeBodyPreflight: true, Success: 200}
}
func reviewMissing(value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface, reflect.Func, reflect.Map, reflect.Chan, reflect.Slice:
		return v.IsNil()
	}
	return false
}
func RegisterReviewResults(router chi.Router, options ReviewResultsOptions) error {
	if !options.Enabled {
		return nil
	}
	origin, err := url.Parse(options.Origin)
	if router == nil || err != nil || protocol.ValidateOrigin(options.Origin) != nil || origin.Host != options.Verifier.Authority || reviewMissing(options.Devices) || reviewMissing(options.Repository) {
		return app.ErrUnavailable
	}
	router.Method(http.MethodPost, ReviewResultsPath, reviewResultsEndpoint{options})
	return nil
}
func (e reviewResultsEndpoint) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Del("ETag")
	fail := func(err error) { orgapi.WriteError(w, r, mapped(err)) }
	if !e.Enabled {
		orgapi.WriteError(w, r, orgapi.NotFound)
		return
	}
	origin, err := url.Parse(e.Origin)
	if err != nil || r.Host != origin.Host || len(r.Header.Values("Origin")) > 1 || len(r.Header.Values("Origin")) == 1 && r.Header.Get("Origin") != e.Origin {
		orgapi.WriteError(w, r, orgapi.Unauthenticated)
		return
	}
	for _, name := range []string{"Cookie", "Authorization", "X-CSRF", "Signature-Input"} {
		if len(r.Header.Values(name)) > 0 {
			orgapi.WriteError(w, r, orgapi.Unauthenticated)
			return
		}
	}
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || len(r.Header.Values("Content-Type")) != 1 || media != "application/json" || r.URL.RawQuery != "" || r.URL.ForceQuery {
		orgapi.WriteError(w, r, orgapi.Malformed)
		return
	}
	raw, err := httpapi.ReadBody(r, protocol.MaxReviewResultsRequestBytes)
	if err != nil {
		orgapi.WriteError(w, r, orgapi.Malformed)
		return
	}
	request, err := protocol.ParseReviewResultsRequest(raw)
	if err != nil {
		fail(err)
		return
	}
	id, err := kernel.ParseOrgID(chi.URLParam(r, "org_id"))
	if err != nil || id.String() != chi.URLParam(r, "org_id") {
		orgapi.WriteError(w, r, orgapi.NotFound)
		return
	}
	scope, err := kernel.NewOrgScope(id)
	if err != nil {
		orgapi.WriteError(w, r, orgapi.NotFound)
		return
	}
	if e.Accounts == nil || reviewMissing(e.Accounts.Store) || reviewMissing(e.Accounts.Clock) || e.Verifier.Pool == nil || reviewMissing(e.Devices) || reviewMissing(e.Repository) {
		orgapi.WriteError(w, r, orgapi.Unavailable)
		return
	}
	in := request.Snapshot()
	accountID, err := (guarded.AccountSessions{Service: e.Accounts}).Confirm(r.Context(), in.Proof.FamilyID, in.Proof.AccessToken, false)
	if err != nil {
		orgapi.WriteError(w, r, err)
		return
	}
	r.Body, r.ContentLength = io.NopCloser(bytes.NewReader(raw)), int64(len(raw))
	proof, err := e.Verifier.Verify(r.Context(), r)
	if err != nil {
		if errors.Is(err, auth.ErrAuthMissing) || errors.Is(err, auth.ErrAuthMalformed) || errors.Is(err, auth.ErrAuthDenied) || errors.Is(err, auth.ErrAuthReplayed) || errors.Is(err, auth.ErrAuthExpired) {
			orgapi.WriteError(w, r, orgapi.Unauthenticated)
		} else {
			orgapi.WriteError(w, r, orgapi.Unavailable)
		}
		return
	}
	owner, found, err := e.Accounts.Store.FindBindingOwner(r.Context(), proof.ContributorID)
	if err != nil {
		orgapi.WriteError(w, r, orgapi.Unavailable)
		return
	}
	if !found || owner != accountID {
		orgapi.WriteError(w, r, orgapi.Unauthenticated)
		return
	}
	expiry, err := strconv.ParseInt(r.Header.Get(auth.HeaderExpires), 10, 64)
	if err != nil {
		orgapi.WriteError(w, r, orgapi.Unauthenticated)
		return
	}
	actor := member.DeviceActor{AccountID: accountID, FamilyID: in.Proof.FamilyID, AccessToken: in.Proof.AccessToken, ContributorID: proof.ContributorID, KeyID: proof.KeyID, Fingerprint: proof.Fingerprint, ProofExpires: time.Unix(expiry, 0).UTC()}
	if actor.Validate() != nil {
		orgapi.WriteError(w, r, orgapi.Unauthenticated)
		return
	}
	device, err := e.Devices.CheckDevice(r.Context(), scope, actor)
	if err != nil {
		fail(err)
		return
	}
	state, err := device.Enrollment.Snapshot(scope)
	if err != nil || device.Validate(scope) != nil || state.State != d.DeviceEnrolled || state.DeviceID != actor.KeyID || state.Fingerprint != actor.Fingerprint {
		orgapi.WriteError(w, r, orgapi.Unavailable)
		return
	}
	source := app.Source{Scope: scope, Origin: e.Origin, Actor: actor, MembershipID: state.MembershipID, EnrollmentID: state.ID}
	bound, err := protocol.NewSourceBinding(accountID, e.Origin, scope.OrgID().String(), source.MembershipID, source.EnrollmentID)
	if err != nil {
		orgapi.WriteError(w, r, orgapi.Unavailable)
		return
	}
	if request.ValidateSource(bound) != nil {
		orgapi.WriteError(w, r, orgapi.NotFound)
		return
	}
	results, repoErr := e.Repository.OwnMeterReviewResults(r.Context(), source, in.Selectors)
	// Even an error after a completed prefix cannot disclose private results from
	// an ended/replaced period. The real repository also rechecks source in its TX.
	if err := confirmCurrentDevice(r.Context(), e.Devices, source); err != nil {
		fail(err)
		return
	}
	if repoErr != nil {
		fail(repoErr)
		return
	}
	response, err := protocol.NewReviewResultsResponse(request, bound, results)
	if err != nil {
		orgapi.WriteError(w, r, orgapi.Unavailable)
		return
	}
	if err := confirmCurrentDevice(r.Context(), e.Devices, source); err != nil {
		fail(err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(response.CanonicalBytes())
}
