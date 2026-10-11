package http

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	application "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/domain"
)

func helpfulHandler() Handler {
	cast := application.CastResult{
		VoteID: "e6c74c23-63db-4c24-a2e5-408cb23bad26",
		Vote: domain.StoredVote{
			ID: "e6c74c23-63db-4c24-a2e5-408cb23bad26", ContributorRef: "c9",
			ObservationID: "d6c74c23-63db-4c24-a2e5-408cb23bad27", DecisionSequence: 2,
			Choice: domain.ChoiceHelpful, CreatedAt: time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC),
		},
		Tally:    domain.Tally{ObservationID: "d6c74c23-63db-4c24-a2e5-408cb23bad27", DecisionSequence: 2, Helpful: 1},
		Share:    domain.Share{Helpful: 1, BasisPoints: 10000, HasVotes: true},
		MyChoice: domain.ChoiceHelpful,
	}
	tally := application.TallyResult{
		Tally: domain.Tally{ObservationID: "d6c74c23-63db-4c24-a2e5-408cb23bad27", DecisionSequence: 2, Helpful: 1},
		Share: domain.Share{Helpful: 1, BasisPoints: 10000, HasVotes: true},
	}
	return Handler{
		Authenticate: func(*http.Request) (application.Caller, error) {
			return application.Caller{ContributorID: "c9", Fingerprint: "fp:y", KeyID: "k2", Token: "tok-c9"}, nil
		},
		CastHelpfulness: func(_ context.Context, _ application.Caller, _ string, _ application.CastDTO) (application.CastResult, error) {
			return cast, nil
		},
		RemoveHelpfulness: func(_ context.Context, _ application.Caller, _ string, _ int64) (application.TallyResult, error) {
			empty := tally
			empty.Tally.Helpful = 0
			empty.Share = domain.Share{}
			empty.MyChoice = nil
			return empty, nil
		},
		HelpfulnessTally: func(_ context.Context, caller application.Caller, _ string, _ int64) (application.TallyResult, error) {
			out := tally
			if caller.ContributorID == "c9" {
				choice := domain.ChoiceHelpful
				out.MyChoice = &choice
			}
			return out, nil
		},
		Secrets: []byte("test-secrets-32-bytes-long-value!"),
	}
}

const helpfulTarget = "/v1/observations/d6c74c23-63db-4c24-a2e5-408cb23bad27/helpfulness"

const helpfulCastBody = `{"choice":"HELPFUL","decision_sequence":2,"client_submission_id":"help-1"}`

func TestCastHelpfulnessHappyPath(t *testing.T) {
	w := serveVotes(helpfulHandler(), http.MethodPost, helpfulTarget, helpfulCastBody)
	if w.Code != http.StatusCreated {
		t.Fatalf("code = %d, body %s", w.Code, w.Body.String())
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Error("cast misses no-store")
	}
	for _, want := range []string{`"vote_id"`, `"observation_id"`, `"decision_sequence"`, `"helpful"`, `"not_helpful"`, `"total"`, `"helpful_basis_points"`, `"my_choice"`} {
		if !strings.Contains(w.Body.String(), want) {
			t.Errorf("body lacks %s: %s", want, w.Body.String())
		}
	}
	for _, leak := range []string{"c9", "fp:y", "k2", "tok-c9", "cursor", "device", "voter"} {
		if strings.Contains(w.Body.String(), leak) {
			t.Errorf("body leaks %q: %s", leak, w.Body.String())
		}
	}
}

func TestCastHelpfulnessRequiresAuth(t *testing.T) {
	h := helpfulHandler()
	h.Authenticate = func(*http.Request) (application.Caller, error) { return application.Caller{}, errors.New("no proof") }
	w := serveVotes(h, http.MethodPost, helpfulTarget, helpfulCastBody)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("code = %d, body %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "community.auth-required") {
		t.Errorf("body lacks auth-required code: %s", w.Body.String())
	}
}

func TestCastHelpfulnessRejectsBadBodies(t *testing.T) {
	big := `{"choice":"HELPFUL","decision_sequence":2,"client_submission_id":"` + strings.Repeat("x", 70<<10) + `"}`
	cases := map[string]string{
		"malformed":      `{"choice":`,
		"unknown field":  `{"choice":"HELPFUL","decision_sequence":2,"client_submission_id":"s","extra":1}`,
		"duplicate":      `{"choice":"HELPFUL","choice":"NOT_HELPFUL","decision_sequence":2,"client_submission_id":"s"}`,
		"trailing data":  helpfulCastBody + " x",
		"missing choice": `{"decision_sequence":2,"client_submission_id":"s"}`,
		"bad choice":     `{"choice":"VALID","decision_sequence":2,"client_submission_id":"s"}`,
		"zero sequence":  `{"choice":"HELPFUL","decision_sequence":0,"client_submission_id":"s"}`,
		"missing sub":    `{"choice":"HELPFUL","decision_sequence":2}`,
		"oversized":      big,
		"empty":          ``,
	}
	for name, body := range cases {
		if w := serveVotes(helpfulHandler(), http.MethodPost, helpfulTarget, body); w.Code != http.StatusBadRequest {
			t.Errorf("%s: code = %d, body %s", name, w.Code, w.Body.String())
		}
	}
	if w := serveVotes(helpfulHandler(), http.MethodPost, helpfulTarget, "{\"choice\":\"HELPFUL\xff\",\"decision_sequence\":2,\"client_submission_id\":\"s\"}"); w.Code != http.StatusBadRequest {
		t.Errorf("invalid utf-8: code = %d", w.Code)
	}
}

func TestCastHelpfulnessMapsDomainErrors(t *testing.T) {
	cases := []struct {
		name string
		err  error
		code int
		want string
	}{
		{"self vote", domain.ErrSelfVote, http.StatusForbidden, "community.self-vote"},
		{"stale", application.ErrStaleSequence, http.StatusConflict, "community.stale-revision"},
		{"blocked", application.ErrAccountBlocked, http.StatusForbidden, "community.account-blocked"},
		{"not found", application.ErrTargetNotFound, http.StatusNotFound, "community.not-found"},
		{"ineligible", application.ErrIneligibleTarget, http.StatusConflict, "community.ineligible-target"},
	}
	for _, tc := range cases {
		h := helpfulHandler()
		h.CastHelpfulness = func(_ context.Context, _ application.Caller, _ string, _ application.CastDTO) (application.CastResult, error) {
			return application.CastResult{}, tc.err
		}
		w := serveVotes(h, http.MethodPost, helpfulTarget, helpfulCastBody)
		if w.Code != tc.code || !strings.Contains(w.Body.String(), tc.want) {
			t.Errorf("%s: code = %d, body %s, want %d %s", tc.name, w.Code, w.Body.String(), tc.code, tc.want)
		}
	}
}

func TestRemoveHelpfulnessHappyPath(t *testing.T) {
	w := serveVotes(helpfulHandler(), http.MethodPost, helpfulTarget+"/remove", `{"decision_sequence":2}`)
	if w.Code != http.StatusOK {
		t.Fatalf("code = %d, body %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"helpful":0`) {
		t.Errorf("body lacks zeroed tally: %s", w.Body.String())
	}
	if strings.Contains(w.Body.String(), "my_choice") {
		t.Errorf("removed vote must clear the overlay: %s", w.Body.String())
	}
}

func TestRemoveHelpfulnessRequiresAuth(t *testing.T) {
	h := helpfulHandler()
	h.Authenticate = func(*http.Request) (application.Caller, error) { return application.Caller{}, errors.New("no proof") }
	w := serveVotes(h, http.MethodPost, helpfulTarget+"/remove", `{"decision_sequence":2}`)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("code = %d", w.Code)
	}
}

func TestTallyHelpfulnessPublicWithoutOverlay(t *testing.T) {
	h := helpfulHandler()
	h.Authenticate = func(*http.Request) (application.Caller, error) { return application.Caller{}, errors.New("anon") }
	w := serveVotes(h, http.MethodGet, helpfulTarget+"?decision_sequence=2", "")
	if w.Code != http.StatusOK {
		t.Fatalf("code = %d, body %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "my_choice") {
		t.Errorf("anonymous tally must carry no owner overlay: %s", w.Body.String())
	}
	for _, leak := range []string{"c9", "fp:y", "k2", "tok-c9", "cursor", "device", "voter"} {
		if strings.Contains(w.Body.String(), leak) {
			t.Errorf("tally leaks %q: %s", leak, w.Body.String())
		}
	}
}

func TestTallyHelpfulnessWithAuthOverlaysChoice(t *testing.T) {
	w := serveVotes(helpfulHandler(), http.MethodGet, helpfulTarget+"?decision_sequence=2", "")
	if w.Code != http.StatusOK {
		t.Fatalf("code = %d, body %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"my_choice":"HELPFUL"`) {
		t.Errorf("authenticated tally must overlay the owner choice: %s", w.Body.String())
	}
}

func TestTallyHelpfulnessRejectsBadSequence(t *testing.T) {
	for _, target := range []string{helpfulTarget, helpfulTarget + "?decision_sequence=0", helpfulTarget + "?decision_sequence=nope"} {
		if w := serveVotes(helpfulHandler(), http.MethodGet, target, ""); w.Code != http.StatusBadRequest {
			t.Errorf("%s: code = %d", target, w.Code)
		}
	}
}

func TestTallyHelpfulnessMapsNotFound(t *testing.T) {
	h := helpfulHandler()
	h.HelpfulnessTally = func(_ context.Context, _ application.Caller, _ string, _ int64) (application.TallyResult, error) {
		return application.TallyResult{}, application.ErrTargetNotFound
	}
	w := serveVotes(h, http.MethodGet, helpfulTarget+"?decision_sequence=2", "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("code = %d, body %s", w.Code, w.Body.String())
	}
}

func TestCastHelpfulnessQuotaDenialMaps429(t *testing.T) {
	h := helpfulHandler()
	h.CastHelpfulness = func(_ context.Context, _ application.Caller, _ string, _ application.CastDTO) (application.CastResult, error) {
		return application.CastResult{}, &application.QuotaDeniedError{RetryAfter: 90 * time.Second}
	}
	w := serveVotes(h, http.MethodPost, helpfulTarget, helpfulCastBody)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("code = %d, body %s", w.Code, w.Body.String())
	}
	if w.Header().Get("Retry-After") != "90" {
		t.Errorf("Retry-After = %q, want 90", w.Header().Get("Retry-After"))
	}
	if !strings.Contains(w.Body.String(), "community.quota-exceeded") {
		t.Errorf("body lacks quota-exceeded code: %s", w.Body.String())
	}
}
