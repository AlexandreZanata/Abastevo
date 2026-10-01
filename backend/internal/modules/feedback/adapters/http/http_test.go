package http

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/feedback/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/feedback/domain"
)

type testService struct {
	mu       sync.Mutex
	comments map[string]domain.StoredComment
	views    map[string]domain.CommentView
	aliases  map[string]string
	ratings  map[string]domain.StoredRating
	err      error
	nextPage application.CommentPage
}

func newTestService() *testService {
	return &testService{
		comments: map[string]domain.StoredComment{},
		views:    map[string]domain.CommentView{},
		aliases:  map[string]string{"acc-1": "adore-fox-42"},
		ratings:  map[string]domain.StoredRating{},
	}
}

func (s *testService) sessioned(_ context.Context, family, token string) (string, error) {
	if family == "fam-1" && token == "tok-1" {
		return "acc-1", nil
	}
	return "", domain.ErrSessionInvalid
}

func (s *testService) SubmitComment(_ context.Context, accountID, stationID, product, text string) (domain.StoredComment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return domain.StoredComment{}, s.err
	}
	rec := domain.StoredComment{
		ID: "c1", AccountID: accountID, StationID: stationID, Product: product,
		Text: text, Scalars: len(text), Revision: 1, CreatedAt: 1_700_000_000, UpdatedAt: 1_700_000_000,
	}
	s.comments["c1"] = rec
	s.views["c1"] = domain.CommentView{
		ID: "c1", Alias: s.aliases[accountID], StationID: stationID, Product: product,
		Text: text, Revision: 1, CreatedAt: 1_700_000_000, UpdatedAt: 1_700_000_000,
	}
	return rec, nil
}

func (s *testService) Reply(_ context.Context, accountID, stationID, product, parentID, text string) (domain.StoredComment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return domain.StoredComment{}, s.err
	}
	rec := domain.StoredComment{
		ID: "c2", AccountID: accountID, StationID: stationID, Product: product,
		ParentID: parentID, Depth: 1, Text: text, Revision: 1,
	}
	s.comments["c2"] = rec
	s.views["c2"] = domain.CommentView{ID: "c2", Alias: s.aliases[accountID], ParentID: parentID, Depth: 1, Text: text, Revision: 1}
	return rec, nil
}

func (s *testService) EditComment(_ context.Context, accountID, commentID, text string, expectedRevision int) (domain.StoredComment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return domain.StoredComment{}, s.err
	}
	rec := s.comments[commentID]
	rec.Text = text
	rec.Revision = expectedRevision + 1
	s.comments[commentID] = rec
	view := s.views[commentID]
	view.Text = text
	view.Revision = rec.Revision
	s.views[commentID] = view
	return rec, nil
}

func (s *testService) DeleteComment(_ context.Context, accountID, commentID string) error {
	if s.err != nil {
		return s.err
	}
	return nil
}

func (s *testService) ViewComment(_ context.Context, id string) (domain.CommentView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return domain.CommentView{}, s.err
	}
	view, ok := s.views[id]
	if !ok {
		return domain.CommentView{}, domain.ErrCommentNotFound
	}
	return view, nil
}

func (s *testService) ListComments(_ context.Context, stationID, product, cursor string, limit int) (application.CommentPage, error) {
	if s.err != nil {
		return application.CommentPage{}, s.err
	}
	return s.nextPage, nil
}

func (s *testService) ListReplies(_ context.Context, parentID, cursor string, limit int) (application.CommentPage, error) {
	if s.err != nil {
		return application.CommentPage{}, s.err
	}
	return s.nextPage, nil
}

func (s *testService) Vote(_ context.Context, accountID, commentID, choice string) (application.VoteResult, error) {
	if s.err != nil {
		return application.VoteResult{}, s.err
	}
	if choice != domain.VoteValid && choice != domain.VoteInvalid {
		return application.VoteResult{}, domain.ErrVoteChoiceInvalid
	}
	return application.VoteResult{
		Vote:  domain.StoredVote{AccountID: accountID, CommentID: commentID, Choice: choice},
		Tally: domain.VoteTally{CommentID: commentID, Revision: 1, Valid: 1},
	}, nil
}

func (s *testService) RemoveVote(_ context.Context, accountID, commentID string) error {
	return s.err
}

func (s *testService) ReportComment(_ context.Context, accountID, commentID, reason string) error {
	return s.err
}

func (s *testService) Tally(_ context.Context, commentID string) (domain.VoteTally, error) {
	if s.err != nil {
		return domain.VoteTally{}, s.err
	}
	return domain.VoteTally{CommentID: commentID, Revision: 1, Valid: 2, Invalid: 1}, nil
}

func ratingKey(stationID, product, accountID string) string {
	return stationID + "|" + product + "|" + accountID
}

func (s *testService) Rate(_ context.Context, accountID, stationID, product string, stars int) (application.RateResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return application.RateResult{}, s.err
	}
	if strings.TrimSpace(stationID) == "" || strings.TrimSpace(product) == "" {
		return application.RateResult{}, domain.ErrTargetInvalid
	}
	if err := domain.Rating(stars).Validate(); err != nil {
		return application.RateResult{}, err
	}
	key := ratingKey(stationID, product, accountID)
	rec, existed := s.ratings[key]
	if !existed {
		rec = domain.StoredRating{ID: "r1", AccountID: accountID, StationID: stationID, Product: product, Revision: 1}
	}
	rec.Stars = stars
	if existed {
		rec.Revision++
	}
	s.ratings[key] = rec
	stats, _, _ := s.statsLocked(stationID, product)
	return application.RateResult{Rating: rec, Stats: stats, Created: !existed}, nil
}

func (s *testService) DeleteRating(_ context.Context, accountID, stationID, product string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return s.err
	}
	key := ratingKey(stationID, product, accountID)
	if _, ok := s.ratings[key]; !ok {
		return domain.ErrRatingNotFound
	}
	delete(s.ratings, key)
	return nil
}

func (s *testService) RatingStats(_ context.Context, stationID, product string) (domain.RatingStats, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return domain.RatingStats{}, s.err
	}
	if strings.TrimSpace(stationID) == "" || strings.TrimSpace(product) == "" {
		return domain.RatingStats{}, domain.ErrTargetInvalid
	}
	stats, _, _ := s.statsLocked(stationID, product)
	return stats, nil
}

func (s *testService) statsLocked(stationID, product string) (domain.RatingStats, bool, error) {
	var count, sum int64
	for _, rec := range s.ratings {
		if rec.StationID == stationID && rec.Product == product {
			count++
			sum += int64(rec.Stars)
		}
	}
	return domain.RatingStats{StationID: stationID, Product: product, Count: count, Sum: sum}, count > 0, nil
}

func testHandler() (*testService, Handler) {
	svc := newTestService()
	return svc, Handler{Service: svc, Sessions: svc.sessioned}
}

func call(t *testing.T, h Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	r := chi.NewRouter()
	h.RegisterRoutes(r)
	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	// chi URL params need a route context for {id} paths.
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

const authed = `{"family_id":"fam-1","access_token":"tok-1"`

func TestSubmitReplyEditDeleteFlow(t *testing.T) {
	_, h := testHandler()

	rec := call(t, h, "POST", "/v1/feedback/comments",
		authed+`,"station_id":"s1","product":"GASOLINE_REGULAR","text":"Preço bom"}`)
	if rec.Code != 201 {
		t.Fatalf("submit: %d (%s)", rec.Code, rec.Body.String())
	}
	var out struct {
		Comment struct {
			ID    string `json:"id"`
			Alias string `json:"public_alias"`
			Text  string `json:"text"`
		} `json:"comment"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Comment.Alias != "adore-fox-42" || out.Comment.Text != "Preço bom" {
		t.Errorf("response must carry alias and verbatim text: %+v", out.Comment)
	}
	if strings.Contains(rec.Body.String(), "acc-1") {
		t.Error("response must not leak the account id")
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Error("write response must be no-store")
	}

	rec = call(t, h, "POST", "/v1/feedback/comments/c1/replies", authed+`,"text":"Concordo"}`)
	if rec.Code != 201 {
		t.Fatalf("reply: %d (%s)", rec.Code, rec.Body.String())
	}
	rec = call(t, h, "POST", "/v1/feedback/comments/c1/edit",
		authed+`,"text":"Editado","expected_revision":1}`)
	if rec.Code != 200 {
		t.Fatalf("edit: %d (%s)", rec.Code, rec.Body.String())
	}
	rec = call(t, h, "POST", "/v1/feedback/comments/c1/remove", authed+`}`)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "deleted") {
		t.Fatalf("delete: %d (%s)", rec.Code, rec.Body.String())
	}
}

func TestWriteAuthAndMapping(t *testing.T) {
	svc, h := testHandler()

	rec := call(t, h, "POST", "/v1/feedback/comments",
		`{"family_id":"bad","access_token":"bad","station_id":"s1","product":"P","text":"x"}`)
	if rec.Code != 401 || !strings.Contains(rec.Body.String(), "feedback.session-invalid") {
		t.Errorf("forged session must be 401 session-invalid, got %d (%s)", rec.Code, rec.Body.String())
	}
	svc.err = domain.ErrAuthorForbidden
	rec = call(t, h, "POST", "/v1/feedback/comments",
		authed+`,"station_id":"s1","product":"P","text":"x"}`)
	if rec.Code != 403 || !strings.Contains(rec.Body.String(), "feedback.author-forbidden") {
		t.Errorf("forbidden must be 403, got %d (%s)", rec.Code, rec.Body.String())
	}
	svc.err = domain.ErrCommentNotFound
	rec = call(t, h, "POST", "/v1/feedback/comments/c9/edit",
		authed+`,"text":"x","expected_revision":1}`)
	if rec.Code != 404 {
		t.Errorf("missing must be 404, got %d", rec.Code)
	}
	svc.err = domain.ErrNotAuthor
	rec = call(t, h, "POST", "/v1/feedback/comments/c1/edit",
		authed+`,"text":"x","expected_revision":1}`)
	if rec.Code != 404 || !strings.Contains(rec.Body.String(), "feedback.comment-not-found") {
		t.Errorf("foreign must share the 404 shape, got %d (%s)", rec.Code, rec.Body.String())
	}
	svc.err = domain.ErrStaleRevision
	rec = call(t, h, "POST", "/v1/feedback/comments/c1/edit",
		authed+`,"text":"x","expected_revision":1}`)
	if rec.Code != 409 || !strings.Contains(rec.Body.String(), "feedback.stale-revision") {
		t.Errorf("stale must be 409, got %d (%s)", rec.Code, rec.Body.String())
	}
	svc.err = domain.ErrTextTooLong
	rec = call(t, h, "POST", "/v1/feedback/comments",
		authed+`,"station_id":"s1","product":"P","text":"x"}`)
	if rec.Code != 400 {
		t.Errorf("oversize must be 400, got %d", rec.Code)
	}
	svc.err = nil
	for _, body := range []string{`not json`, `{"family_id":"fam-1"}`} {
		if rec := call(t, h, "POST", "/v1/feedback/comments",
			body); rec.Code != 400 {
			t.Errorf("malformed must be 400, got %d for %q", rec.Code, body)
		}
	}
}

func TestPublicReadsAnonymous(t *testing.T) {
	svc, h := testHandler()
	svc.nextPage = application.CommentPage{
		Items: []domain.CommentView{
			{ID: "c1", Alias: "adore-fox-42", StationID: "s1", Product: "P", Text: "hi", Revision: 1},
		},
		NextCursor: "cursor-1",
	}
	rec := call(t, h, "GET", "/v1/feedback/comments?station_id=s1&product=P&limit=20", "")
	if rec.Code != 200 {
		t.Fatalf("public list: %d (%s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "cursor-1") {
		t.Errorf("page must carry cursor: %s", rec.Body.String())
	}
	rec = call(t, h, "GET", "/v1/feedback/comments/c1/replies?limit=200", "")
	if rec.Code != 400 {
		t.Errorf("over-limit page must be 400, got %d", rec.Code)
	}
	rec = call(t, h, "GET", "/v1/feedback/comments?product=P", "")
	if rec.Code != 400 {
		t.Errorf("missing station must be 400, got %d", rec.Code)
	}
}

func TestVoteRemoveTallyFlow(t *testing.T) {
	_, h := testHandler()

	rec := call(t, h, "POST", "/v1/feedback/comments/c1/votes", authed+`,"choice":"VALID"}`)
	if rec.Code != 200 {
		t.Fatalf("vote: %d (%s)", rec.Code, rec.Body.String())
	}
	var out struct {
		Tally struct {
			Valid         int64  `json:"valid"`
			Invalid       int64  `json:"invalid"`
			Total         int64  `json:"total"`
			PercentageBps *int64 `json:"percentage_bps"`
		} `json:"tally"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Tally.Valid != 1 || out.Tally.Total != 1 || out.Tally.PercentageBps == nil || *out.Tally.PercentageBps != 10000 {
		t.Errorf("tally must be exact with floor math: %+v", out.Tally)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Error("vote response must be no-store")
	}

	rec = call(t, h, "POST", "/v1/feedback/comments/c1/votes", authed+`,"choice":"MAYBE"}`)
	if rec.Code != 400 || !strings.Contains(rec.Body.String(), "feedback.vote-choice-invalid") {
		t.Errorf("bad choice must be 400, got %d (%s)", rec.Code, rec.Body.String())
	}
	rec = call(t, h, "POST", "/v1/feedback/comments/c1/votes/remove", authed+`}`)
	if rec.Code != 200 {
		t.Fatalf("remove: %d (%s)", rec.Code, rec.Body.String())
	}
	// The stub tally is 2/1: the shared golden denominator over HTTP.
	rec = call(t, h, "GET", "/v1/feedback/comments/c1/tally", "")
	if rec.Code != 200 {
		t.Fatalf("tally: %d (%s)", rec.Code, rec.Body.String())
	}
	var tout struct {
		Tally struct {
			Valid         int64  `json:"valid"`
			Invalid       int64  `json:"invalid"`
			Total         int64  `json:"total"`
			PercentageBps *int64 `json:"percentage_bps"`
		} `json:"tally"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &tout); err != nil {
		t.Fatal(err)
	}
	if tout.Tally.Valid != 2 || tout.Tally.Invalid != 1 || tout.Tally.Total != 3 ||
		tout.Tally.PercentageBps == nil || *tout.Tally.PercentageBps != 6666 {
		t.Errorf("tally must match the golden 2/1 vector: %+v", tout.Tally)
	}
}

func TestRatingRateRemoveStatsFlow(t *testing.T) {
	_, h := testHandler()

	rec := call(t, h, "POST", "/v1/feedback/ratings",
		authed+`,"station_id":"s1","product":"GASOLINE","stars":5}`)
	if rec.Code != 201 {
		t.Fatalf("rate: %d (%s)", rec.Code, rec.Body.String())
	}
	var out struct {
		Rating struct {
			Stars    int  `json:"stars"`
			Revision int  `json:"revision"`
			Created  bool `json:"created"`
		} `json:"rating"`
		Stats struct {
			Count     int64  `json:"count"`
			Sum       int64  `json:"sum"`
			MeanMilli *int64 `json:"mean_milli"`
		} `json:"stats"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Rating.Stars != 5 || out.Rating.Revision != 1 || !out.Rating.Created {
		t.Errorf("first rating must create revision 1: %+v", out.Rating)
	}
	if out.Stats.Count != 1 || out.Stats.Sum != 5 || out.Stats.MeanMilli == nil || *out.Stats.MeanMilli != 5000 {
		t.Errorf("stats must be 1/5 with milli mean 5000: %+v", out.Stats)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Error("rating response must be no-store")
	}
	if strings.Contains(rec.Body.String(), "acc-1") {
		t.Error("rating response must not leak the account id")
	}

	rec = call(t, h, "POST", "/v1/feedback/ratings",
		authed+`,"station_id":"s1","product":"GASOLINE","stars":6}`)
	if rec.Code != 400 || !strings.Contains(rec.Body.String(), "feedback.rating-out-of-range") {
		t.Errorf("six stars must be 400 rating-out-of-range, got %d (%s)", rec.Code, rec.Body.String())
	}

	rec = call(t, h, "GET", "/v1/feedback/ratings/stats?station_id=s1&product=GASOLINE", "")
	if rec.Code != 200 {
		t.Fatalf("stats: %d (%s)", rec.Code, rec.Body.String())
	}
	rec = call(t, h, "GET", "/v1/feedback/ratings/stats?station_id=unknown&product=GASOLINE", "")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"count":0`) {
		t.Errorf("missing key must be honest empty, got %d (%s)", rec.Code, rec.Body.String())
	}
	rec = call(t, h, "GET", "/v1/feedback/ratings/stats?product=GASOLINE", "")
	if rec.Code != 400 {
		t.Errorf("missing station must be 400, got %d", rec.Code)
	}

	rec = call(t, h, "POST", "/v1/feedback/ratings/remove",
		authed+`,"station_id":"s1","product":"GASOLINE"}`)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "deleted") {
		t.Fatalf("delete: %d (%s)", rec.Code, rec.Body.String())
	}
	rec = call(t, h, "POST", "/v1/feedback/ratings/remove",
		authed+`,"station_id":"s1","product":"GASOLINE"}`)
	if rec.Code != 404 || !strings.Contains(rec.Body.String(), "feedback.rating-not-found") {
		t.Errorf("second delete must be 404 rating-not-found, got %d (%s)", rec.Code, rec.Body.String())
	}

	rec = call(t, h, "POST", "/v1/feedback/ratings",
		`{"family_id":"bad","access_token":"bad","station_id":"s1","product":"GASOLINE","stars":4}`)
	if rec.Code != 401 || !strings.Contains(rec.Body.String(), "feedback.session-invalid") {
		t.Errorf("forged session must be 401 session-invalid, got %d (%s)", rec.Code, rec.Body.String())
	}
}

func TestVoteMapping(t *testing.T) {
	svc, h := testHandler()

	svc.err = domain.ErrSelfVote
	rec := call(t, h, "POST", "/v1/feedback/comments/c1/votes", authed+`,"choice":"VALID"}`)
	if rec.Code != 403 || !strings.Contains(rec.Body.String(), "feedback.self-vote") {
		t.Errorf("self-vote must be 403, got %d (%s)", rec.Code, rec.Body.String())
	}
	svc.err = domain.ErrAuthorForbidden
	rec = call(t, h, "POST", "/v1/feedback/comments/c1/votes", authed+`,"choice":"VALID"}`)
	if rec.Code != 403 {
		t.Errorf("forbidden must be 403, got %d", rec.Code)
	}
	svc.err = domain.ErrCommentNotFound
	rec = call(t, h, "GET", "/v1/feedback/comments/c9/tally", "")
	if rec.Code != 404 {
		t.Errorf("missing tally must be 404, got %d", rec.Code)
	}
	svc.err = nil
	if rec := call(t, h, "POST", "/v1/feedback/comments/c1/votes", authed+`} inadequate`); rec.Code != 400 {
		t.Errorf("malformed must be 400, got %d", rec.Code)
	}
}

func TestReportFlow(t *testing.T) {
	svc, h := testHandler()

	rec := call(t, h, "POST", "/v1/feedback/comments/c1/report", authed+`,"reason":"spam"}`)
	if rec.Code != 202 || !strings.Contains(rec.Body.String(), "reported") {
		t.Fatalf("report: %d (%s)", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Error("report response must be no-store")
	}
	svc.err = domain.ErrReportInvalid
	rec = call(t, h, "POST", "/v1/feedback/comments/c1/report", authed+`,"reason":""}`)
	if rec.Code != 400 || !strings.Contains(rec.Body.String(), "feedback.report-invalid") {
		t.Errorf("bad reason must be 400, got %d (%s)", rec.Code, rec.Body.String())
	}
	svc.err = &domain.QuotaDeniedError{RetryAfterSeconds: 30}
	rec = call(t, h, "POST", "/v1/feedback/comments/c1/report", authed+`,"reason":"spam"}`)
	if rec.Code != 429 || !strings.Contains(rec.Body.String(), "feedback.quota-exceeded") {
		t.Errorf("quota must be 429, got %d (%s)", rec.Code, rec.Body.String())
	}
	svc.err = domain.ErrCommentNotFound
	rec = call(t, h, "POST", "/v1/feedback/comments/c9/report", authed+`,"reason":"spam"}`)
	if rec.Code != 404 {
		t.Errorf("missing target must be 404, got %d", rec.Code)
	}
	svc.err = nil
	if rec := call(t, h, "POST", "/v1/feedback/comments/c1/report", authed+`} inadequate`); rec.Code != 400 {
		t.Errorf("malformed must be 400, got %d", rec.Code)
	}
}

func TestBodiesNeverLeakSecrets(t *testing.T) {
	_, h := testHandler()
	// Comment text round-trips verbatim by design (no rendering
	// server-side); session proof, however, never echoes, even in
	// refusals for forged sessions.
	rec := call(t, h, "POST", "/v1/feedback/comments",
		`{"family_id":"forged-fam","access_token":"forged-tok","station_id":"s1","product":"P","text":"hi"}`)
	body := rec.Body.String()
	if rec.Code != 401 {
		t.Fatalf("forged session must be 401, got %d", rec.Code)
	}
	if strings.Contains(body, "forged-fam") || strings.Contains(body, "forged-tok") {
		t.Errorf("refusal must not echo session proof: %s", body)
	}
}
