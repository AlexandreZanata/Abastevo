package domain

import (
	"errors"
	"strings"
)

var ErrProviderEmailInUse = errors.New("account: provider recovery email already in use")

// GoogleRecoveryAddress accepts verified email for Gmail and for Workspace
// tokens whose hosted domain matches the address. Other Google identities
// can still sign in by subject. This proof never selects an existing user.
func GoogleRecoveryAddress(subject ProviderSubject) RecoveryAddress {
	if !subject.EmailVerified || !ValidEmail(subject.Email) {
		return RecoveryAddress{}
	}
	_, emailDomain, _ := strings.Cut(NormalizeAddress(subject.Email), "@")
	if emailDomain != "gmail.com" && emailDomain != "googlemail.com" && !strings.EqualFold(subject.HostedDomain, emailDomain) {
		return RecoveryAddress{}
	}
	return RecoveryAddress{Hash: AddressHash(subject.Email), Hint: EmailHint(subject.Email)}
}
