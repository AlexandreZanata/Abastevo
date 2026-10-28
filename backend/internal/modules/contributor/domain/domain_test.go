package domain

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

// TestGitHubLogin_CTB03 covers GitHub's login rules.
func TestGitHubLogin_CTB03(t *testing.T) {
	for _, valid := range []string{"a", "octocat", "user-name", "A1-b2"} {
		if !ValidGitHubLogin(valid) {
			t.Fatalf("valid login refused: %q", valid)
		}
	}
	for _, invalid := range []string{"", strings.Repeat("a", 40), "-abc", "abc-", "a--b", "a_b", "a.b", "a/b", "a b"} {
		if ValidGitHubLogin(invalid) {
			t.Fatalf("invalid login accepted: %q", invalid)
		}
	}
	if NormalizeGitHubLogin(" OctoCat ") != "octocat" {
		t.Fatal("normalization")
	}
}

// TestDisplayName_CTB08 covers NFC, rune bounds and the allowed set.
func TestDisplayName_CTB08(t *testing.T) {
	for _, valid := range []string{"Ana", "José", "Ana-Maria", "Ana Maria", "O'Neil", "João 2", "A\u0301na"} {
		if name, ok := NormalizeDisplayName(valid); !ok || name == "" {
			t.Fatalf("valid name refused: %q", valid)
		}
	}
	for _, invalid := range []string{"<script>", "\u202eabc", "a\u200db", strings.Repeat("a", 41), "😀", "a", "@user", "a/b", "a:b", "\x00abc"} {
		if _, ok := NormalizeDisplayName(invalid); ok {
			t.Fatalf("invalid name accepted: %q", invalid)
		}
	}
}

// TestLinks_CTB08 covers every platform, URL builder and hostile handles.
func TestLinks_CTB08(t *testing.T) {
	cases := []struct{ platform, handle, url string }{
		{"github", "octocat", "https://github.com/octocat"},
		{"gitlab", "user.name", "https://gitlab.com/user.name"},
		{"linkedin", "user-name", "https://www.linkedin.com/in/user-name"},
		{"x", "user_1", "https://x.com/user_1"},
		{"instagram", "user.name", "https://www.instagram.com/user.name"},
		{"youtube", "user.name-1", "https://www.youtube.com/@user.name-1"},
		{"bluesky", "user.bsky.social", "https://bsky.app/profile/user.bsky.social"},
	}
	for _, tc := range cases {
		links, ok := ValidateLinks([]Link{{Platform: tc.platform, Handle: tc.handle}})
		if !ok || len(links) != 1 {
			t.Fatalf("valid link refused: %+v", tc)
		}
		public := PublicLinks(links)
		if len(public) != 1 || public[0].URL != tc.url {
			t.Fatalf("url %q want %q", public[0].URL, tc.url)
		}
	}
	invalid := [][]Link{
		{{Platform: "unknown", Handle: "x"}},
		{{Platform: "x", Handle: "javascript:alert(1)"}},
		{{Platform: "x", Handle: "https://evil.com"}},
		{{Platform: "x", Handle: "user/../x"}},
		{{Platform: "x", Handle: "аbc"}},
		{{Platform: "x", Handle: strings.Repeat("a", 16)}},
		{{Platform: "github", Handle: ""}},
		{{Platform: "bluesky", Handle: "nodot"}},
		{{Platform: "github", Handle: "a"}, {Platform: "x", Handle: "b"}, {Platform: "gitlab", Handle: "cc"}},
		{{Platform: "x", Handle: "a"}, {Platform: "x", Handle: "b"}},
	}
	for _, links := range invalid {
		if _, ok := ValidateLinks(links); ok {
			t.Fatalf("invalid links accepted: %+v", links)
		}
	}
}

// TestClaimCode_CTB03 covers the one-time code shape.
func TestClaimCode_CTB03(t *testing.T) {
	code, err := GenerateClaimCode(bytes.NewReader(make([]byte, ClaimCodeBytes)))
	if err != nil || !ValidClaimCode(code) {
		t.Fatalf("code=%q err=%v", code, err)
	}
	if len(code) != len(ClaimCodePrefix)+16 || !strings.HasPrefix(code, ClaimCodePrefix) {
		t.Fatalf("code shape %q", code)
	}
	for _, invalid := range []string{"", ClaimCodePrefix, ClaimCodePrefix + strings.Repeat("!", 16), "abv_admin_x"} {
		if ValidClaimCode(invalid) {
			t.Fatalf("invalid code accepted: %q", invalid)
		}
	}
}

// TestClaimState_CTB07 covers the terminal state machine.
func TestClaimState_CTB07(t *testing.T) {
	for _, to := range []ClaimState{ClaimApproved, ClaimRejected, ClaimWithdrawn, ClaimExpired} {
		if !CanTransition(ClaimPending, to) {
			t.Fatalf("pending -> %s refused", to)
		}
	}
	for _, from := range []ClaimState{ClaimApproved, ClaimRejected, ClaimWithdrawn, ClaimExpired} {
		for _, to := range []ClaimState{ClaimPending, ClaimApproved, ClaimRejected, ClaimWithdrawn, ClaimExpired} {
			if CanTransition(from, to) {
				t.Fatalf("%s -> %s accepted", from, to)
			}
		}
	}
	now := time.Now().UTC()
	claim := Claim{State: ClaimPending, ExpiresAt: now.Add(time.Minute)}
	if !ClaimOpen(claim, now) || ClaimOpen(claim, now.Add(2*time.Minute)) {
		t.Fatal("open window")
	}
}
