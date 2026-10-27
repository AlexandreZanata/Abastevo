package http

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/driver/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/driver/domain"
	"github.com/go-chi/chi/v5"
)

type fixtureCommands struct {
	receipt app.Receipt
	err     error
}

func (f fixtureCommands) Save(context.Context, string, app.SaveEarning) (app.Receipt, error) {
	return f.receipt, f.err
}
func (f fixtureCommands) ApplyShift(context.Context, string, app.ApplyShift) (domain.ShiftTransition, error) {
	return domain.ShiftTransition{}, f.err
}
func (f fixtureCommands) SaveCostProfile(context.Context, string, app.SaveCostProfile) (app.CostProfileReceipt, error) {
	return app.CostProfileReceipt{}, f.err
}
func (f fixtureCommands) EraseRecord(context.Context, string, string, int64, int64) error {
	return f.err
}
func (f fixtureCommands) EraseCostProfile(context.Context, string, string, int64, int64) error {
	return f.err
}

func fixturePath(name string) string {
	dir, _ := os.Getwd()
	for {
		path := filepath.Join(dir, "contracts", "testdata", "driver-suite", "http", name)
		if _, err := os.Stat(path); err == nil {
			return path
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return filepath.Join("contracts", "testdata", "driver-suite", "http", name)
}

func TestDriverHTTPWireFixtures(t *testing.T) {
	cases := []struct {
		name    string
		store   fixtureCommands
		signed  bool
		trace   string
		fixture string
	}{
		{"success", fixtureCommands{receipt: app.Receipt{ID: "11111111-1111-4111-8111-111111111111", Revision: 1}}, true, "", "receipt-200.json"},
		{"conflict", fixtureCommands{err: domain.ErrConflict}, true, "trace-409", "conflict-409.json"},
		{"auth", fixtureCommands{}, false, "trace-401", "auth-401.json"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := Handler{
				Commands: tc.store,
				Sessions: func(context.Context, string, string) (string, error) {
					return "88888888-8888-4888-8888-888888888888", nil
				},
				SignedOwner: func(r *http.Request, _ string) (string, string, error) {
					if !tc.signed {
						return "", "", errors.New("unsigned")
					}
					return "66666666-6666-4666-8666-666666666666", "77777777-7777-4777-8777-777777777777", nil
				},
				CursorSecret: []byte(strings.Repeat("s", 32)), Origin: "one.example.test",
			}
			body := `{"family_id":"44444444-4444-4444-8444-444444444444","access_token":"x","kind":"EARNING_SAVE","payload":{}}`
			r := httptest.NewRequest(http.MethodPost, "/v1/driver/commands", strings.NewReader(body))
			r.Header.Set("Content-Type", "application/json")
			if tc.trace != "" {
				r.Header.Set("X-Request-ID", tc.trace)
			}
			w := httptest.NewRecorder()
			router := chi.NewRouter()
			h.RegisterRoutes(router)
			router.ServeHTTP(w, r)
			raw, err := os.ReadFile(fixturePath(tc.fixture))
			if err != nil {
				t.Fatal(err)
			}
			if got, want := strings.TrimSpace(w.Body.String()), strings.TrimSpace(string(raw)); got != want {
				t.Fatalf("wire mismatch\n got: %s\nwant: %s", got, want)
			}
		})
	}
}
