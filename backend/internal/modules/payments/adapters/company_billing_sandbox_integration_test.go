//go:build integration && sandbox

package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	billingpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgbilling/adapters"
	billing "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgbilling/domain"
	capability "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/jackc/pgx/v5"
)

const companySandboxCaseFile = "ANPFUEL_SANDBOX_COMPANY_BILLING_CASE"

// The independently reviewed expectations never come from the provider read.
type companySandboxCase struct {
	Channel          domain.Channel `json:"channel"`
	Organization     string         `json:"organization_id"`
	Subscription     string         `json:"subscription_id"`
	Event            string         `json:"event_id"`
	Resource         string         `json:"resource_id"`
	Customer         string         `json:"customer_id"`
	Contract         string         `json:"contract_id"`
	Offer            string         `json:"offer_reference"`
	AsaasReference   string         `json:"asaas_external_reference"`
	ExpectedCentavos int64          `json:"amount_centavos"`
	TermStart        time.Time      `json:"term_start"`
	TermEnd          time.Time      `json:"term_end"`
}

func TestSandboxCompanyBillingReplay(t *testing.T) {
	path := os.Getenv(companySandboxCaseFile)
	if path == "" {
		t.Skip("company sandbox case was not supplied")
	}
	fixture := readCompanySandboxCase(t, path)
	stripe, asaas, err := readSandboxConfig(os.Getenv)
	if err != nil {
		t.Fatal("sandbox credentials refused")
	}
	merchant, secret, host := asaas.merchant, asaas.key, "api-sandbox.asaas.com"
	if fixture.Channel == domain.ChannelCard {
		merchant, secret, host = stripe.merchant, stripe.secret, "api.stripe.com"
		if !stripe.testKey() || !strings.HasPrefix(merchant, "acct_") {
			t.Fatal("Stripe test-mode key and merchant are required")
		}
	} else if !asaas.testKey() || asaas.merchant == "" {
		t.Fatal("Asaas sandbox key and merchant are required")
	}
	location, err := url.Parse(os.Getenv("ANPFUEL_TEST_DATABASE_URL"))
	if err != nil || location.Hostname() != "127.0.0.1" {
		t.Fatal("company sandbox evidence requires disposable loopback PostGIS")
	}
	client := &http.Client{Timeout: 10 * time.Second,
		Transport:     companySandboxReadOnly{host: host, upstream: http.DefaultTransport},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	checkCompanySandboxReplay(t, fixture, merchant, secret, client)
}

func readCompanySandboxCase(t *testing.T, path string) companySandboxCase {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal("could not open the private company sandbox case")
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 || info.Size() > 8_192 {
		t.Fatal("company sandbox case must be a private regular file of at most 8192 bytes")
	}
	d := json.NewDecoder(io.LimitReader(file, 8_193))
	d.DisallowUnknownFields()
	var fixture companySandboxCase
	if err := d.Decode(&fixture); err != nil || d.Decode(new(json.RawMessage)) != io.EOF {
		t.Fatal("invalid company sandbox case")
	}
	return fixture
}

type companySandboxReadOnly struct {
	host     string
	upstream http.RoundTripper
}

func (r companySandboxReadOnly) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Method != http.MethodGet || req.URL.Scheme != "https" || req.URL.Host != r.host || req.URL.User != nil {
		return nil, errors.New("unexpected company sandbox request")
	}
	return r.upstream.RoundTrip(req)
}

type companySandboxProcessor interface {
	Process(context.Context, app.InboxRecord) error
}

type rejectPersonalSandboxSettlement struct{}

func (rejectPersonalSandboxSettlement) Process(context.Context, app.InboxRecord) error {
	return errors.New("company sandbox charge was routed to personal settlement")
}

func checkCompanySandboxReplay(t *testing.T, fixture companySandboxCase, merchant, secret string, client *http.Client) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()
	plan := billing.PlanKey{Product: "fleet", Plan: "sandbox", Revision: 1}
	offer := billing.BillingOffer{Plan: plan, Channel: fixture.Channel, Environment: domain.EnvironmentTest,
		Merchant: merchant, ProviderOfferRef: fixture.Offer, Price: domain.BRL(fixture.ExpectedCentavos),
		ApprovalRef: "synthetic-reviewed-sandbox", Term: domain.TermMonthly}
	event := InboxInput{Provider: fixture.Channel.Provider(), Environment: domain.EnvironmentTest, Merchant: merchant,
		EventID: fixture.Event, PayloadHash: payloadHash("company sandbox notice/" + fixture.Event), ResourceID: fixture.Resource}
	term := billing.Period{Start: fixture.TermStart, End: fixture.TermEnd}
	now := time.Now()
	if offer.Validate() != nil || !validText(fixture.Customer) || !validText(fixture.Contract) || !validText(fixture.Resource) ||
		!validText(fixture.Event) || term.Start.IsZero() || !term.Start.Before(term.End) || now.Before(term.Start) || !now.Before(term.End) {
		t.Fatal("company sandbox case requires an exact reviewed charge for the current term")
	}
	if fixture.Channel == domain.ChannelCard {
		event.EventType, event.ResourceType = "invoice.paid", "invoice"
		if fixture.AsaasReference != "" || !strings.HasPrefix(fixture.Resource, "in_") || !strings.HasPrefix(fixture.Customer, "cus_") ||
			!strings.HasPrefix(fixture.Contract, "sub_") || !strings.HasPrefix(fixture.Offer, "price_") {
			t.Fatal("invalid reviewed Stripe case")
		}
	} else {
		event.EventType, event.ResourceType = "PAYMENT_RECEIVED", "payment"
		scope, err := billing.AsaasReferenceScope(fixture.AsaasReference)
		if err != nil || scope.OrgID().String() != fixture.Organization || !strings.HasPrefix(fixture.Resource, "pay_") ||
			!strings.HasPrefix(fixture.Customer, "cus_") || fixture.Channel == domain.ChannelPixManual && fixture.Contract != fixture.Resource {
			t.Fatal("invalid reviewed Asaas case")
		}
	}
	f := orgtest.New(t)
	org := f.Organizations[0]
	if fixture.Organization != org.ID || fixture.Subscription != org.RecordID {
		t.Fatal("case must use organization A and its synthetic subscription from the shared fixture")
	}
	capabilities, err := capability.NewCatalog([]string{"organizations"})
	if err != nil {
		t.Fatal("could not load fixture capabilities")
	}
	role, password := f.ProvisionScriptRole(t)
	p := f.ScriptRuntimePool(t, role, password, 3)
	ledger := billingpg.SubscriptionLedger{Capabilities: capabilities}
	seedCompanySandboxCase(t, f, fixture, offer, org)
	prior := billing.Period{Start: term.Start.AddDate(0, -1, 0), End: term.Start}
	active := billing.LifecycleEvent{Scope: org.Scope(t), SubscriptionID: org.RecordID, Kind: billing.ConfirmSettlement,
		Source: billing.VerifiedBilling, At: prior.Start, Period: prior, Reference: "synthetic-prior-term"}
	late := billing.LifecycleEvent{Scope: active.Scope, SubscriptionID: org.RecordID, Kind: billing.ReportPastDue,
		Source: billing.VerifiedBilling, ExpectedRevision: 1, At: term.Start, Reference: "synthetic-past-due"}
	if err := database.WithOrgTx(ctx, p, active.Scope, func(tx pgx.Tx) error {
		if _, err := ledger.ApplyTx(ctx, tx, plan, active); err != nil {
			return err
		}
		_, err := ledger.ApplyTx(ctx, tx, plan, late)
		return err
	}); err != nil {
		t.Fatal("could not prepare the past-due fixture")
	}
	inbox := NewStore(p)
	intake := billingpg.BillingIntake{Pool: p, Ledger: ledger, Now: time.Now}
	var processor companySandboxProcessor
	if fixture.Channel == domain.ChannelCard {
		processor = billingpg.StripeSettlement{Resources: NewStripeProvider(secret, event.Environment, client), Record: intake,
			Personal: rejectPersonalSandboxSettlement{}, Inbox: inbox, Merchant: merchant, Environment: event.Environment}
	} else {
		processor = billingpg.AsaasSettlement{Resources: NewAsaasProvider(secret, event.Environment, client), Bindings: intake, Record: intake,
			Personal: rejectPersonalSandboxSettlement{}, Inbox: inbox, Merchant: merchant, Environment: event.Environment,
			AutomaticAllowed: fixture.Channel == domain.ChannelPixAutomatic}
	}
	if result, err := inbox.RecordInbox(ctx, event); err != nil || !result.Created || result.State != "PENDING" {
		t.Fatal("could not admit the controlled sandbox notification")
	}
	stored, err := inbox.Inbox(ctx, event.Provider, event.Environment, event.Merchant, event.EventID)
	if err != nil || processor.Process(ctx, stored) != nil {
		t.Fatal("company sandbox settlement processor failed")
	}
	settled, err := inbox.Inbox(ctx, event.Provider, event.Environment, event.Merchant, event.EventID)
	if err != nil || settled.State != "PROCESSED" {
		t.Fatal("provider evidence did not settle the reviewed company charge")
	}
	results := make(chan bool, 2)
	for range 2 {
		go func() {
			result, err := inbox.RecordInbox(ctx, event)
			results <- err == nil && !result.Created && result.State == "PROCESSED"
		}()
	}
	firstReplay, secondReplay := <-results, <-results
	if !firstReplay || !secondReplay {
		t.Fatal("duplicate sandbox notifications did not return the processed outcome")
	}
	var state string
	var revision, receipts, jobs, foreign, roles int
	var start, end time.Time
	err = f.Admin.QueryRow(ctx, `SELECT state,revision,term_start,term_end,
 (SELECT count(*) FROM org_billing_settlement_receipts WHERE org_id=$1 AND subscription_id=$2 AND event_id=$3),
 (SELECT count(*) FROM job_queue WHERE kind='payment-inbox'),
 (SELECT count(*) FROM org_billing_settlement_receipts WHERE org_id<>$1),
 (SELECT count(*) FROM org_role_assignments)
 FROM org_billing_subscription_events WHERE org_id=$1 AND subscription_id=$2 ORDER BY revision DESC LIMIT 1`,
		org.ID, org.RecordID, event.EventID).Scan(&state, &revision, &start, &end, &receipts, &jobs, &foreign, &roles)
	if err != nil || state != "ACTIVE" || revision != 3 || !start.Equal(term.Start) || !end.Equal(term.End) ||
		receipts != 1 || jobs != 1 || foreign != 0 || roles != 0 {
		t.Fatal("sandbox replay did not preserve one reviewed term, receipt and inbox job")
	}
}

func seedCompanySandboxCase(t *testing.T, f *orgtest.Fixture, fixture companySandboxCase, offer billing.BillingOffer, org orgtest.Organization) {
	t.Helper()
	ctx := t.Context()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := f.Admin.Exec(ctx, query, args...); err != nil {
			t.Fatal("could not provision the disposable company sandbox fixture")
		}
	}
	exec(`INSERT INTO enterprise_plan_catalog(product_code,plan_code,revision,display_name,capabilities,active_vehicles)
 VALUES('fleet','sandbox',1,'Synthetic sandbox plan',ARRAY['organizations'],3)`)
	exec(`INSERT INTO enterprise_billing_offers
 (id,product_code,plan_code,plan_revision,channel,environment,merchant,provider_offer_ref,amount_centavos,term,approval_ref)
 VALUES($1,'fleet','sandbox',1,$2,'test',$3,$4,$5,'P1M',$6)`, org.ParentID, offer.Channel, offer.Merchant,
		offer.ProviderOfferRef, offer.Price.Centavos, offer.ApprovalRef)
	exec(`INSERT INTO org_billing_charge_bindings
 (org_id,provider,environment,merchant,resource_ref,subscription_id,offer_id,customer_ref,contract_ref,term_start,term_end,asaas_external_reference)
 VALUES($1,$2,'test',$3,$4,$5,$6,$7,$8,$9,$10,$11)`, org.ID, offer.Channel.Provider(), offer.Merchant,
		fixture.Resource, org.RecordID, org.ParentID, fixture.Customer, fixture.Contract, fixture.TermStart, fixture.TermEnd, fixture.AsaasReference)
}

type companySandboxResponses map[string]string

func (r companySandboxResponses) RoundTrip(req *http.Request) (*http.Response, error) {
	body, ok := r[req.URL.Path]
	if !ok {
		return nil, errors.New("unexpected synthetic company provider resource")
	}
	return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Request: req,
		Body: io.NopCloser(strings.NewReader(body))}, nil
}

func TestCompanySandboxTransportAllowsOnlyPinnedRead(t *testing.T) {
	const host = "api-sandbox.asaas.com"
	path := "/v3/payments/pay_synthetic"
	transport := companySandboxReadOnly{host: host, upstream: companySandboxResponses{path: `{}`}}
	for _, endpoint := range []string{"http://" + host, "https://example.invalid", "https://api.asaas.com", "https://synthetic@" + host} {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, endpoint+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := transport.RoundTrip(req); err == nil {
			t.Fatal("unapproved sandbox destination reached transport")
		}
	}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "https://"+host+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := transport.RoundTrip(req); err == nil {
		t.Fatal("sandbox write reached transport")
	}
	req.Method = http.MethodGet
	res, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatal("sandbox read was refused")
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatal("sandbox read did not reach the pinned transport")
	}
}

func TestCompanyBillingReplayWithSyntheticSandboxResponse(t *testing.T) {
	const org, subscription, checkout = "7b0e2a54-0000-4000-8000-000000000001", "7b0e2a54-0000-4000-8200-000000000001", "7b0e2a54-0000-4000-8000-000000000089"
	location, err := time.LoadLocation(domain.BillingTimezone)
	if err != nil {
		t.Fatal("could not load the billing time zone")
	}
	now := time.Now().UTC().Truncate(time.Second)
	fixture := companySandboxCase{Channel: domain.ChannelPixManual, Organization: org, Subscription: subscription,
		Event: "evt_synthetic", Resource: "pay_synthetic", Customer: "cus_synthetic", Contract: "pay_synthetic", Offer: "synthetic-offer",
		AsaasReference: "abv_org:" + org + ":" + checkout, ExpectedCentavos: 1723, TermStart: now.Add(-time.Hour), TermEnd: now.AddDate(0, 1, 0)}
	for _, channel := range []domain.Channel{domain.ChannelPixManual, domain.ChannelPixAutomatic} {
		t.Run(string(channel), func(t *testing.T) {
			fixture := fixture
			fixture.Channel = channel
			authorization := ""
			if channel == domain.ChannelPixAutomatic {
				fixture.Contract = checkout
				authorization = fixture.Contract
			}
			body := fmt.Sprintf(`{"id":%q,"customer":%q,"externalReference":%q,"billingType":"PIX","value":17.23,
 "status":"RECEIVED","paymentDate":%q,"pixAutomaticAuthorizationId":%q}`,
				fixture.Resource, fixture.Customer, fixture.AsaasReference, now.In(location).Format(time.DateOnly), authorization)
			client := &http.Client{Transport: companySandboxReadOnly{host: "api-sandbox.asaas.com",
				upstream: companySandboxResponses{"/v3/payments/" + fixture.Resource: body}}}
			checkCompanySandboxReplay(t, fixture, "synthetic-merchant", "synthetic-test-only", client)
		})
	}
	t.Run(string(domain.ChannelCard), func(t *testing.T) {
		fixture.Channel = domain.ChannelCard
		fixture.AsaasReference = ""
		fixture.Resource, fixture.Contract, fixture.Offer = "in_synthetic", "sub_synthetic", "price_synthetic"
		invoice := fmt.Sprintf(`{
 "id":"in_synthetic","livemode":false,"status":"paid","currency":"brl","customer":"cus_synthetic",
 "amount_paid":1723,"amount_due":1723,"collection_method":"charge_automatically","billing_reason":"subscription_cycle",
 "status_transitions":{"paid_at":%d},"parent":{"type":"subscription_details","subscription_details":{
 "subscription":"sub_synthetic","metadata":{"abastevo_billing_scope":"organization",
 "abastevo_organization_id":%q,"abastevo_organization_subscription_id":%q}}},
 "lines":{"data":[{"currency":"brl","amount":1723,"quantity":1,"period":{"start":%d,"end":%d},
 "parent":{"type":"subscription_item_details","subscription_item_details":{"subscription":"sub_synthetic"}},
 "pricing":{"type":"price_details","price_details":{"price":"price_synthetic"}}}]},
 "payments":{"data":[{"status":"paid","is_default":true,"currency":"brl","amount_paid":1723,
 "payment":{"type":"payment_intent","payment_intent":"pi_synthetic"}}]}}`,
			now.Unix(), fixture.Organization, fixture.Subscription, fixture.TermStart.Unix(), fixture.TermEnd.Unix())
		const intent = `{"id":"pi_synthetic","livemode":false,"status":"succeeded","amount_received":1723,"currency":"brl",
 "customer":"cus_synthetic","latest_charge":{"id":"ch_synthetic","paid":true,"captured":true,
 "amount_captured":1723,"currency":"brl","payment_method_details":{"type":"card"}}}`
		client := &http.Client{Transport: companySandboxReadOnly{host: "api.stripe.com", upstream: companySandboxResponses{
			"/v1/invoices/in_synthetic": invoice, "/v1/payment_intents/pi_synthetic": intent,
		}}}
		checkCompanySandboxReplay(t, fixture, "acct_synthetic", "synthetic-test-only", client)
	})
}
