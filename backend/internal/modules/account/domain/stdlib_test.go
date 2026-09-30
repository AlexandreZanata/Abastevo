package domain

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestDomainStdlibOnly(t *testing.T) {
	// TARGET_ARCHITECTURE: account domain depends on the standard library
	// only, mirroring the kernel and identity packages.
	dir, err := filepath.Abs(".")
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range f.Imports {
			path, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(path, ".") {
				t.Errorf("%s imports non-stdlib %q", name, path)
			}
		}
	}
}

func TestVerdictCodesAreStable(t *testing.T) {
	cases := map[error]string{
		ErrCodeExpired:           "code-expired",
		ErrCodeConsumed:          "code-consumed",
		ErrCodeAttemptsExhausted: "code-attempts-exhausted",
		ErrCodeResendCooldown:    "code-resend-cooldown",
		ErrCodeUnknown:           "code-unknown",
		ErrOIDCUnknownIssuer:     "oidc-unknown-issuer",
		ErrOIDCWrongAudience:     "oidc-wrong-audience",
		ErrOIDCExpired:           "oidc-expired",
		ErrOIDCNonceReused:       "oidc-nonce-reused",
		ErrOIDCNonceMismatch:     "oidc-nonce-mismatch",
		ErrOIDCUnavailable:       "oidc-unavailable",
		ErrLinkCrossAccount:      "link-cross-account-refused",
		ErrLinkEmailOnly:         "link-email-only-refused",
		ErrLastLoginMethod:       "link-last-method-refused",
		ErrProviderNotLinked:     "link-provider-not-linked",
		ErrAccountNotFound:       "account-unknown",
		ErrAccountSuspended:      "account-suspended",
		ErrAccountDeleted:        "account-deleted",
		ErrBindingCrossAccount:   "binding-cross-account-refused",
		ErrBindingNotFound:       "binding-not-found",
		ErrBindingInvalid:        "binding-invalid",
		ErrKeyUnavailable:        "key-unavailable",
		ErrKeyProofDenied:        "key-proof-denied",
		ErrAddressLinked:         "address-linked",
		ErrSessionReuse:          "session-reuse-revoked",
		ErrSessionRevoked:        "session-revoked",
		ErrSessionExpired:        "session-expired",
	}
	for err, want := range cases {
		if got := VerdictCode(err); got != want {
			t.Errorf("VerdictCode(%v) = %q, want %q", err, got, want)
		}
	}
	if got := VerdictCode(nil); got != VerdictOK {
		t.Errorf("VerdictCode(nil) = %q, want ok", got)
	}
}
