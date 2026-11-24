package domain

import (
	"context"
	"errors"
)

var ErrMailDelivery = errors.New("account: code delivery unavailable")

// CodePurpose selects only the delivery copy. The verifier is purpose-neutral.
type CodePurpose string

const (
	CodePurposeSignup   CodePurpose = "signup"
	CodePurposeRecovery CodePurpose = "recovery"
	CodePurposeAttach   CodePurpose = "attach"
)

func (p CodePurpose) Valid() bool {
	return p == "" || p == CodePurposeSignup || p == CodePurposeRecovery || p == CodePurposeAttach
}

// LocalizedMailSender can choose purpose- and locale-specific delivery copy.
type LocalizedMailSender interface {
	MailSender
	SendCodeWithPurpose(ctx context.Context, address, code string, purpose CodePurpose, locale string) error
}
