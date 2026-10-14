package http

import (
	"context"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/application"
	"net/http"
	"testing"
)

func TestPhotoCaptureHTTPFailsClosedBeforeCallingPort(t *testing.T) {
	called := false
	h := voteHandler()
	h.PhotoCapture = func(context.Context, app.Caller, string, app.PhotoCaptureIntent, []byte) (app.PhotoCapture, bool, error) {
		called = true
		return app.PhotoCapture{}, false, app.ErrPhotoCaptureIneligible
	}
	for _, body := range []string{`{}`, `{"client_capture_id":"c","station_id":"s","location":null}`, `{"client_capture_id":"c","station_id":"s","unknown":1}`} {
		if w := serveVotes(h, http.MethodPost, "/v1/photo-captures", body); w.Code != 400 {
			t.Fatalf("malformed status=%d", w.Code)
		}
	}
	if called {
		t.Fatal("malformed body reached port")
	}
	h.Authenticate = func(*http.Request) (app.Caller, error) { return app.Caller{}, app.ErrUnauthorized }
	if w := serveVotes(h, http.MethodPost, "/v1/photo-captures", `{}`); w.Code != 401 {
		t.Fatalf("unauthenticated status=%d", w.Code)
	}
	if called {
		t.Fatal("unauthenticated request reached port")
	}
}

func TestCommunityFirstPhotoCaptureHTTPAcceptsAbsentLocation(t *testing.T) {
	called := false
	h := voteHandler()
	h.PhotoCapture = func(_ context.Context, _ app.Caller, _ string, in app.PhotoCaptureIntent, _ []byte) (app.PhotoCapture, bool, error) {
		called = true
		if in.Location != nil || in.DevelopmentPreview {
			t.Fatal("normal capture invented location or preview")
		}
		return app.PhotoCapture{PolicyVersion: app.CommunityPhotoCapturePolicy}, false, nil
	}
	w := serveVotes(h, http.MethodPost, "/v1/photo-captures", `{"client_capture_id":"c","station_id":"d6c74c23-63db-4c24-a2e5-408cb23bad26"}`)
	if w.Code != 201 || !called {
		t.Fatalf("no-location HTTP %d", w.Code)
	}
}
