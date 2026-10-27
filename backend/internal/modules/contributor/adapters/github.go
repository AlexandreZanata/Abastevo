// Package adapters owns the contributor GitHub client and PostgreSQL store.
package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/contributor/domain"
)

var (
	ErrGitHubUnavailable = errors.New("contributor: github unavailable")
	ErrGitHubNotFound    = errors.New("contributor: github not found")
)

const (
	githubTimeout     = 5 * time.Second
	githubMaxResponse = 1 << 20
	githubMaxPages    = 20
)

type GitHubClient struct {
	BaseURL string
	Repo    string
	Token   string
	Client  *http.Client
}

func NewGitHubClient(repo, token string) *GitHubClient {
	return &GitHubClient{
		BaseURL: "https://api.github.com",
		Repo:    repo,
		Token:   token,
		Client:  &http.Client{Timeout: githubTimeout},
	}
}

func (c *GitHubClient) get(ctx context.Context, path string) ([]byte, int, error) {
	if c == nil || c.Client == nil || c.Repo == "" {
		return nil, 0, ErrGitHubUnavailable
	}
	base := c.BaseURL
	if base == "" {
		base = "https://api.github.com"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(base, "/")+path, nil)
	if err != nil {
		return nil, 0, ErrGitHubUnavailable
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "abastevo-contributor")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, 0, ErrGitHubUnavailable
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, githubMaxResponse+1))
	if err != nil {
		return nil, resp.StatusCode, ErrGitHubUnavailable
	}
	if len(raw) > githubMaxResponse {
		return nil, resp.StatusCode, ErrGitHubUnavailable
	}
	return raw, resp.StatusCode, nil
}

// Contributors returns all logins in repository order plus the response ETag
// of the first page. Pagination stops after a short page or the page cap.
func (c *GitHubClient) Contributors(ctx context.Context) ([]string, string, error) {
	if c == nil || c.Repo == "" {
		return nil, "", ErrGitHubUnavailable
	}
	var logins []string
	etag := ""
	for page := 1; page <= githubMaxPages; page++ {
		path := fmt.Sprintf("/repos/%s/contributors?per_page=100&page=%d", c.Repo, page)
		raw, status, err := c.get(ctx, path)
		if err != nil {
			return nil, "", err
		}
		if status == http.StatusNotFound {
			return nil, "", ErrGitHubNotFound
		}
		if status != http.StatusOK {
			return nil, "", ErrGitHubUnavailable
		}
		var rows []struct {
			Login string `json:"login"`
		}
		if err := json.Unmarshal(raw, &rows); err != nil {
			return nil, "", ErrGitHubUnavailable
		}
		for _, row := range rows {
			if row.Login != "" {
				logins = append(logins, domain.NormalizeGitHubLogin(row.Login))
			}
		}
		if len(rows) < 100 {
			break
		}
	}
	return logins, etag, nil
}

// Gist returns the owner login and every file content for one public gist.
func (c *GitHubClient) Gist(ctx context.Context, id string) (domain.Gist, error) {
	if strings.TrimSpace(id) == "" || strings.ContainsAny(id, "/?#") || len(id) > 128 {
		return domain.Gist{}, ErrGitHubNotFound
	}
	raw, status, err := c.get(ctx, "/gists/"+url.PathEscape(id))
	if err != nil {
		return domain.Gist{}, err
	}
	if status == http.StatusNotFound {
		return domain.Gist{}, ErrGitHubNotFound
	}
	if status != http.StatusOK {
		return domain.Gist{}, ErrGitHubUnavailable
	}
	var payload struct {
		Owner struct {
			Login string `json:"login"`
		} `json:"owner"`
		Files map[string]struct {
			Content string `json:"content"`
		} `json:"files"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return domain.Gist{}, ErrGitHubUnavailable
	}
	gist := domain.Gist{Owner: domain.NormalizeGitHubLogin(payload.Owner.Login)}
	for _, file := range payload.Files {
		gist.Files = append(gist.Files, file.Content)
	}
	return gist, nil
}
