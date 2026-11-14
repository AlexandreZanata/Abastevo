package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	meter "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain/meter"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/go-chi/chi/v5"
)

type unavailableReviewRepository struct{}

func (*unavailableReviewRepository) ListMeterReviews(context.Context, kernel.OrgScope, app.FleetActor, string, app.MeterReviewFilter) (app.MeterReviewPage, error) {
	return app.MeterReviewPage{}, app.ErrUnavailable
}
func (*unavailableReviewRepository) ReadMeterReview(context.Context, kernel.OrgScope, app.FleetActor, string, string) (app.MeterReviewView, error) {
	return app.MeterReviewView{}, app.ErrUnavailable
}
func (*unavailableReviewRepository) ResolveMeterReview(context.Context, kernel.OrgScope, app.FleetActor, string, app.MeterReviewDecision) (protocol.ReviewResolution, bool, error) {
	return protocol.ReviewResolution{}, false, app.ErrUnavailable
}

type reviewAdmission struct{}

func (reviewAdmission) Validate(r orgapi.Rule) error {
	if r.Permission != "meter.reading.correct" || !r.Write || !r.Fresh || !r.CompanyOnly || !r.RequireRevision {
		return orgapi.Unavailable
	}
	return nil
}
func (reviewAdmission) Check(context.Context, orgapi.Principal, orgapi.Rule) error {
	return orgapi.Unavailable
}

const reviewHTTPID = "7b0e2a54-0000-4000-8000-000000008502"

func TestMeterReviewDTOExactPrivateDecisionAndClosedKeys(t *testing.T) {
	raw := `{"schema_version":1,"operation_id":"` + reviewHTTPID + `","revision":1,"decision":"REFUSE","reason":" Private 🚀 reason "}`
	var input meterReviewResolveRequest
	if e := json.Unmarshal([]byte(raw), &input); e != nil {
		t.Fatal("exact typed review refused", e)
	}
	command, e := input.command(reviewHTTPID)
	if e != nil {
		t.Fatal(e)
	}
	v, e := command.Snapshot()
	if e != nil || v.Reason != " Private 🚀 reason " || v.ReviewItemID != reviewHTTPID {
		t.Fatal("reason or target normalized", e)
	}
	if strings.Contains(fmt.Sprintf("%+v", input), "Private") {
		t.Fatal("review diagnostic exposes reason")
	}
	for _, bad := range []string{`null`, `[]`, `{"account_id":"synthetic"}`, `{"reason":null}`, `{"reason":"\ud800"}`, `{"reason":"a","reason":"b"}`, `{"Reason":"alias"}`, `{"state":"ACCEPTED"}`, `{} {}`} {
		if json.Unmarshal([]byte(bad), &input) == nil {
			t.Fatal("ambiguous review input accepted")
		}
	}
	var list meterReviewListRequest
	if json.Unmarshal([]byte(`{"limit":20,"state":"EXPIRED_UNRESOLVED"}`), &list) != nil || list.State != app.MeterReviewExpired {
		t.Fatal("actual expired review filter unavailable")
	}
}
func TestMeterReviewProjectionKeepsHeldFactSeparateFromResolution(t *testing.T) {
	at := time.Date(2026, 11, 14, 0, 0, 0, 123456789, time.UTC)
	value, err := meter.NewValue(meter.Odometer, 1000000000000, nil)
	if err != nil {
		t.Fatal(err)
	}
	v := app.MeterReviewView{ReviewItemID: reviewHTTPID, OperationID: reviewHTTPID, PayloadDigest: "sha256:" + strings.Repeat("a", 64), Code: "PERMISSION_CHANGED", State: app.MeterReviewOpen, Revision: 1, AuthorAccountID: reviewHTTPID, AuthorMembershipID: reviewHTTPID, AuthorEmployeeID: reviewHTTPID, EnrollmentID: reviewHTTPID, ReceivedAt: at.Add(time.Minute), ExpiresAt: at.Add(60 * 24 * time.Hour), Fact: app.MeterReadingInput{OperationID: reviewHTTPID, AssetID: reviewHTTPID, MeterID: reviewHTTPID, SegmentID: reviewHTTPID, ReadingID: reviewHTTPID, Value: value, Declared: orgtime.Declaration{OccurredAt: at, EntryZone: "UTC"}, Origin: meter.Standalone, Method: meter.Typed}}
	out, e := projectMeterReview(v)
	if e != nil {
		t.Fatal("actual held projection unavailable", e)
	}
	fact := out["fact"].(map[string]any)
	if fact["value"] != "1000000000000" || fact["occurred_at"] != "2026-11-14T00:00:00.123456789Z" || out["resolution"] != nil {
		t.Fatal("held evidence rounded or accepted")
	}
	for _, edit := range []func(*app.MeterReviewView){func(v *app.MeterReviewView) { v.Fact.OperationID = "" }, func(v *app.MeterReviewView) { v.State = app.MeterReviewAccepted; v.Revision = 2 }, func(v *app.MeterReviewView) { v.AuthorMembershipID = "" }} {
		copy := v
		edit(&copy)
		if _, e := projectMeterReview(copy); !errors.Is(e, orgapi.Unavailable) {
			t.Fatal("unverified private review projected", e)
		}
	}
}
func TestMeterReviewRegistrationHasActualCurrentCompanyDescriptors(t *testing.T) {
	_, cookies, gateway, authorization := registrationFixture(t)
	gateway.WriteAdmission = reviewAdmission{}
	service := app.MeterReviewService{Enabled: true, Repository: &unavailableReviewRepository{}}
	router := chi.NewRouter()
	if e := RegisterMeterReviews(router, service, cookies, gateway, authorization); e != nil {
		t.Fatal("current review routes unavailable", e)
	}
	seen := 0
	if e := chi.Walk(router, func(method, path string, h http.Handler, _ ...func(http.Handler) http.Handler) error {
		d, ok := h.(orgapi.Descriptor)
		if !ok || method != "POST" {
			t.Fatal("unguarded review route")
		}
		r := d.OrganizationRule()
		write := path == MeterReviewResolvePath
		if r.Permission != "meter.reading.correct" || r.Write != write || r.Fresh != write || r.RequireRevision != write || !r.CompanyOnly || r.Success != 200 {
			t.Fatal("review source policy drift")
		}
		seen++
		return nil
	}); e != nil || seen != 3 {
		t.Fatal("incomplete review descriptors", e)
	}
	service.Enabled = false
	if e := RegisterMeterReviews(nil, service, cookies, gateway, authorization); e != nil {
		t.Fatal("disabled review registered")
	}
}
