package domain

import "testing"

func TestGoogleRecoveryRequiresVerificationAndProviderAuthority(t *testing.T) {
	for _, tc := range []struct {
		domain, hosted string
		verified, bind bool
	}{
		{"gmail.com", "", true, true},
		{"googlemail.com", "", true, true},
		{"gmail.com", "", false, false},
		{"example.invalid", "example.invalid", true, true},
		{"example.invalid", "EXAMPLE.INVALID", true, true},
		{"example.invalid", "example.invalid", false, false},
		{"example.invalid", "other.example.invalid", true, false},
		{"example.invalid", "", true, false},
	} {
		// Unicode local part makes this synthetic address an invalid Gmail
		// username. No code or delivery is involved in classifying the domain.
		const local = "àbastevo-test"
		address := local + "@" + tc.domain
		got := GoogleRecoveryAddress(ProviderSubject{Email: address, HostedDomain: tc.hosted, EmailVerified: tc.verified})
		if tc.bind {
			if got.Hash != AddressHash(address) || got.Hint != "à***@"+tc.domain {
				t.Errorf("expected a hashed and masked binding for %v", tc)
			}
		} else if got != (RecoveryAddress{}) {
			t.Errorf("untrusted Google email yielded a recovery binding for %v", tc)
		}
	}
}
