package licensed

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type stub struct {
	scope   string
	now     time.Time
	revoked bool
	seen    map[string]bool
	script  map[string]Response
	last    Request
	calls   int
}

func (s *stub) RoundTrip(ctx context.Context, req Request) (Response, error) {
	if err := ctx.Err(); err != nil {
		return Response{}, err
	}
	s.last = req
	s.calls++
	if s.revoked {
		return Response{ErrorKind: KindRevoked}, nil
	}
	if req.Scope != s.scope {
		return Response{ErrorKind: KindWrongScope}, nil
	}
	if req.Timestamp.Add(5 * time.Minute).Before(s.now) {
		return Response{ErrorKind: KindStaleTimestamp}, nil
	}
	if s.seen == nil {
		s.seen = map[string]bool{}
	}
	if s.seen[req.Nonce] {
		return Response{ErrorKind: KindReplayRefused}, nil
	}
	s.seen[req.Nonce] = true
	if resp, ok := s.script[req.LocationID]; ok {
		return resp, nil
	}
	return Response{Status: StatusAvailable, ObservedAt: s.now}, nil
}

func validConfig() Config {
	return Config{
		ProviderID: "operator-1", Protocol: "OCPI_2_2_1",
		Modules:        []Module{ModuleStatus, ModuleTariffs},
		BaseURL:        "https://operator.example.com/ocpi/2.2.1",
		CredentialEnv:  "ANPFUEL_TEST_OPERATOR_TOKEN",
		Scope:          "granted-scope",
		GrantExpiresAt: time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
	}
}

func testClient(now time.Time, cred string, tr Transport) *Client {
	lookup := func(string) (string, bool) {
		if cred == "" {
			return "", false
		}
		return cred, true
	}
	return &Client{cfg: validConfig(), now: func() time.Time { return now }, lookup: lookup, transport: tr}
}

func TestLicensedConfigValidation(t *testing.T) {
	good := validConfig()
	if err := Validate(good); err != nil {
		t.Fatal(err)
	}
	if _, err := New(good); err != nil {
		t.Fatal(err)
	}
	cases := map[string]func(*Config){
		"provider": func(c *Config) { c.ProviderID = "bad id!" },
		"protocol": func(c *Config) { c.Protocol = "OCPI_3_0" },
		"commands": func(c *Config) { c.Modules = []Module{"COMMANDS"} },
		"dup":      func(c *Config) { c.Modules = []Module{ModuleStatus, ModuleStatus} },
		"empty":    func(c *Config) { c.Modules = nil },
		"http":     func(c *Config) { c.BaseURL = "http://operator.example.com/" },
		"userinfo": func(c *Config) { c.BaseURL = "https://user:pass@operator.example.com/" },
		"fragment": func(c *Config) { c.BaseURL = "https://operator.example.com/#x" },
		"env":      func(c *Config) { c.CredentialEnv = "lowercase-secret" },
		"scope":    func(c *Config) { c.Scope = "has space" },
		"expiry":   func(c *Config) { c.GrantExpiresAt = time.Time{} },
	}
	for name, mutate := range cases {
		c := validConfig()
		mutate(&c)
		if err := Validate(c); err == nil {
			t.Fatalf("%s admitted", name)
		}
		if _, err := New(c); err == nil {
			t.Fatalf("%s constructed", name)
		}
	}
}

func TestLicensedAccessPendingWithoutCredential(t *testing.T) {
	now := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	cl := testClient(now, "", &stub{scope: "granted-scope", now: now})
	if got := cl.Resolve(); got.State != StateAccessPending || got.Reason != ReasonNoCredential {
		t.Fatalf("%+v", got)
	}
	if _, err := cl.Status(context.Background(), "loc-ok"); err == nil {
		t.Fatal("status without credential admitted")
	} else {
		var access *AccessError
		if !errors.As(err, &access) || access.State != StateAccessPending || access.Reason != ReasonNoCredential {
			t.Fatalf("%v", err)
		}
	}
	if _, err := cl.Tariffs(context.Background(), "loc-ok"); err == nil {
		t.Fatal("tariffs without credential admitted")
	}
	// The default constructor has no secret source and no route.
	def, err := New(validConfig())
	if err != nil {
		t.Fatal(err)
	}
	if got := def.Resolve(); got.State != StateAccessPending {
		t.Fatalf("%+v", got)
	}
	if _, err := def.Status(context.Background(), "loc-ok"); err == nil {
		t.Fatal("default client read admitted")
	}
}

func TestLicensedGrantExpiryAndRoute(t *testing.T) {
	now := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	expired := testClient(now, "token", &stub{scope: "granted-scope", now: now})
	expired.cfg.GrantExpiresAt = now.Add(-time.Hour)
	if got := expired.Resolve(); got.State != StateAccessPending || got.Reason != ReasonGrantExpired {
		t.Fatalf("%+v", got)
	}
	if _, err := expired.Status(context.Background(), "loc-ok"); err == nil {
		t.Fatal("expired grant read admitted")
	}
	authorized := testClient(now, "token", nil)
	if got := authorized.Resolve(); got.State != StateAuthorized {
		t.Fatalf("%+v", got)
	}
	if _, err := authorized.Status(context.Background(), "loc-ok"); err == nil {
		t.Fatal("routeless read admitted")
	} else {
		var access *AccessError
		if !errors.As(err, &access) || access.Reason != ReasonRouteUnpinned {
			t.Fatalf("%v", err)
		}
	}
}

func TestLicensedFaultTaxonomy(t *testing.T) {
	now := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	tr := &stub{scope: "granted-scope", now: now, script: map[string]Response{
		"loc-unknown":  {Status: StatusUnknown, ObservedAt: now, Reason: "not-reported"},
		"loc-outage":   {ErrorKind: KindProviderOutage},
		"loc-quota":    {ErrorKind: KindQuotaExceeded},
		"loc-replay":   {ErrorKind: KindReplayRefused},
		"loc-bogus":    {ErrorKind: "made-up"},
		"loc-badstate": {Status: "SUPER_AVAILABLE", ObservedAt: now},
	}}
	cl := testClient(now, "token", tr)
	if got, err := cl.Status(context.Background(), "loc-ok"); err != nil || got.State != StatusAvailable || !got.ObservedAt.Equal(now) {
		t.Fatalf("%+v %v", got, err)
	}
	unknown, err := cl.Status(context.Background(), "loc-unknown")
	if err != nil || unknown.State != StatusUnknown || unknown.Reason != "not-reported" {
		t.Fatalf("%+v %v", unknown, err)
	}
	if unknown.State == StatusAvailable {
		t.Fatal("unknown upgraded")
	}
	for loc, want := range map[string]string{
		"loc-outage": KindProviderOutage, "loc-quota": KindQuotaExceeded,
		"loc-replay": KindReplayRefused, "loc-bogus": KindInvalidState,
	} {
		if _, err := cl.Status(context.Background(), loc); err == nil {
			t.Fatalf("%s admitted", loc)
		} else {
			var provider *ProviderError
			if !errors.As(err, &provider) || provider.Kind != want {
				t.Fatalf("%s got %v want %s", loc, err, want)
			}
		}
	}
	if _, err := cl.Status(context.Background(), "loc-badstate"); err == nil {
		t.Fatal("invented state admitted")
	}
	// Replay detection executes on the transport: a reused nonce refuses.
	req := Request{Operation: "STATUS", LocationID: "loc-ok", Scope: "granted-scope", Nonce: "fixed-nonce", Timestamp: now, HasCredential: true}
	if _, err := tr.RoundTrip(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if _, err := tr.RoundTrip(context.Background(), req); err != nil {
		t.Fatal("replay transport contract broken")
	} else if resp, _ := tr.RoundTrip(context.Background(), req); resp.ErrorKind != KindReplayRefused {
		t.Fatalf("reused nonce not refused: %+v", resp)
	}
	// Stale timestamps refuse: provider clock ten minutes ahead.
	stale := &stub{scope: "granted-scope", now: now.Add(10 * time.Minute)}
	staleCl := testClient(now, "token", stale)
	if _, err := staleCl.Status(context.Background(), "loc-ok"); err == nil {
		t.Fatal("stale timestamp admitted")
	} else {
		var provider *ProviderError
		if !errors.As(err, &provider) || provider.Kind != KindStaleTimestamp {
			t.Fatalf("%v", err)
		}
	}
	// Wrong scope refuses before any data.
	scoped := &stub{scope: "other-scope", now: now}
	if _, err := testClient(now, "token", scoped).Status(context.Background(), "loc-ok"); err == nil {
		t.Fatal("wrong scope admitted")
	} else {
		var provider *ProviderError
		if !errors.As(err, &provider) || provider.Kind != KindWrongScope {
			t.Fatalf("%v", err)
		}
	}
	// Revocation mid-grant refuses.
	tr.revoked = true
	if _, err := cl.Status(context.Background(), "loc-ok"); err == nil {
		t.Fatal("revoked read admitted")
	} else {
		var provider *ProviderError
		if !errors.As(err, &provider) || provider.Kind != KindRevoked {
			t.Fatalf("%v", err)
		}
	}
	tr.revoked = false
}

func TestLicensedTariffsShapeAndProvenance(t *testing.T) {
	now := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	valid := TariffResult{ID: "11111111-1111-4111-8111-111111111111", Revision: "r7", ValidFrom: now.Add(-time.Hour)}
	bad := valid
	bad.ID = "not-a-uuid"
	tr := &stub{scope: "granted-scope", now: now, script: map[string]Response{
		"loc-tariffs":     {Tariffs: []TariffResult{valid}},
		"loc-tariffs-bad": {Tariffs: []TariffResult{bad}},
	}}
	cl := testClient(now, "token", tr)
	got, err := cl.Tariffs(context.Background(), "loc-tariffs")
	if err != nil || len(got) != 1 || got[0].Provenance != "LICENSED_OPERATOR" {
		t.Fatalf("%+v %v", got, err)
	}
	if _, err := cl.Tariffs(context.Background(), "loc-tariffs-bad"); err == nil {
		t.Fatal("ill-shaped tariff admitted")
	} else {
		var provider *ProviderError
		if !errors.As(err, &provider) || provider.Kind != KindInvalidState {
			t.Fatalf("%v", err)
		}
	}
}

func TestLicensedCancelAndSecretHygiene(t *testing.T) {
	now := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	tr := &stub{scope: "granted-scope", now: now}
	cl := testClient(now, "s3cr3t-value", tr)
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := cl.Status(canceled, "loc-ok"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel not propagated: %v", err)
	}
	if _, err := cl.Status(context.Background(), "loc-ok"); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{tr.last.Operation, tr.last.LocationID, tr.last.Scope, tr.last.Nonce} {
		if strings.Contains(field, "s3cr3t-value") {
			t.Fatalf("credential leaked into request field %q", field)
		}
	}
	if !tr.last.HasCredential {
		t.Fatal("credential presence not signalled")
	}
	if _, err := cl.Status(context.Background(), "bad location!"); err == nil {
		t.Fatal("malformed location admitted")
	}
}
