package config

import (
	"fmt"
	"strings"
	"testing"
)

// TestContributorConfig_CTB01_CTB11 covers repo, token and CORS validation.
func TestContributorConfig_CTB01_CTB11(t *testing.T) {
	cfg, err := load(envFunc(nil))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.GitHubRepo != "" || cfg.GitHubToken != "" || len(cfg.PublicWebOrigins) != 0 {
		t.Fatal("contributor surface unexpectedly configured")
	}
	cfg, err = load(envFunc(map[string]string{
		keyGitHubRepo:       "owner/repo",
		keyGitHubToken:      "synthetic-token",
		keyPublicWebOrigins: "https://landing.example, https://landing.example/",
	}))
	if err != nil || cfg.GitHubRepo != "owner/repo" || cfg.GitHubToken != "synthetic-token" || len(cfg.PublicWebOrigins) != 1 {
		t.Fatalf("cfg=%+v err=%v", cfg, err)
	}
	for _, repo := range []string{"owner", "owner/repo/extra", "owner/re po", "/repo"} {
		if _, err := load(envFunc(map[string]string{keyGitHubRepo: repo})); err == nil {
			t.Fatalf("repo %q accepted", repo)
		}
	}
	for _, origin := range []string{"landing.example", "https://landing.example/path", "https://landing.example?x=1"} {
		if _, err := load(envFunc(map[string]string{keyPublicWebOrigins: origin})); err == nil {
			t.Fatalf("origin %q accepted", origin)
		}
	}
	rendered := fmt.Sprintf("%v", cfg.LogValue())
	if strings.Contains(rendered, "synthetic-token") {
		t.Fatal("log value leaked the GitHub token")
	}
}
