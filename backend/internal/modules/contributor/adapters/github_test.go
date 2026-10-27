package adapters

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func testClient(server *httptest.Server) *GitHubClient {
	c := NewGitHubClient("owner/repo", "synthetic-token")
	c.BaseURL = server.URL
	c.Client = server.Client()
	c.Client.Timeout = time.Second
	return c
}

// TestGitHubContributors_CTB01 covers pagination, sorted logins and token use.
func TestGitHubContributors_CTB01(t *testing.T) {
	var pages int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer synthetic-token" {
			t.Error("missing read-only token")
		}
		pages++
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("page") == "1" {
			rows := make([]string, 100)
			for i := range rows {
				rows[i] = fmt.Sprintf(`{"login":"user%d"}`, i)
			}
			fmt.Fprintf(w, "[%s]", strings.Join(rows, ","))
			return
		}
		fmt.Fprint(w, `[{"login":"LAST"}]`)
	}))
	defer server.Close()
	logins, _, err := testClient(server).Contributors(context.Background())
	if err != nil || len(logins) != 101 || pages != 2 {
		t.Fatalf("logins=%d pages=%d err=%v", len(logins), pages, err)
	}
	if logins[0] != "user0" || logins[100] != "last" {
		t.Fatalf("order/normalization: %q %q", logins[0], logins[100])
	}
}

// TestGitHubFailures_CTB01 covers 404, 500, timeout and oversized responses.
func TestGitHubFailures_CTB01(t *testing.T) {
	for _, tc := range []struct {
		status int
		want   error
	}{
		{http.StatusNotFound, ErrGitHubNotFound},
		{http.StatusInternalServerError, ErrGitHubUnavailable},
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(tc.status) }))
		_, _, err := testClient(server).Contributors(context.Background())
		server.Close()
		if err != tc.want {
			t.Fatalf("status=%d err=%v", tc.status, err)
		}
	}
	oversize := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(make([]byte, githubMaxResponse+1))
	}))
	_, _, err := testClient(oversize).Contributors(context.Background())
	oversize.Close()
	if err != ErrGitHubUnavailable {
		t.Fatalf("oversize err=%v", err)
	}
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(100 * time.Millisecond)
		_, _ = w.Write([]byte(`[]`))
	}))
	client := testClient(slow)
	client.Client.Timeout = 20 * time.Millisecond
	_, _, err = client.Contributors(context.Background())
	slow.Close()
	if err != ErrGitHubUnavailable {
		t.Fatalf("timeout err=%v", err)
	}
}

// TestGitHubGist_CTB04 covers owner/code verification input and failures.
func TestGitHubGist_CTB04(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/gists/good":
			fmt.Fprint(w, `{"owner":{"login":"OctoCat"},"files":{"a":{"content":"prefix abastevo-contributor-ABCDEFGHIJKLMNOP suffix"}}}`)
		case "/gists/notfound":
			w.WriteHeader(http.StatusNotFound)
		default:
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer server.Close()
	client := testClient(server)
	gist, err := client.Gist(context.Background(), "good")
	if err != nil || gist.Owner != "octocat" || len(gist.Files) != 1 || !strings.Contains(gist.Files[0], "abastevo-contributor-") {
		t.Fatalf("gist=%+v err=%v", gist, err)
	}
	if _, err := client.Gist(context.Background(), "notfound"); err != ErrGitHubNotFound {
		t.Fatalf("notfound err=%v", err)
	}
	if _, err := client.Gist(context.Background(), "boom"); err != ErrGitHubUnavailable {
		t.Fatalf("500 err=%v", err)
	}
}
