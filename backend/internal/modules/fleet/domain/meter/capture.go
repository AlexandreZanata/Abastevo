package meter

import "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"

type CaptureKind string

const (
	CaptureEnrolledDevice CaptureKind = "ENROLLED_DEVICE"
	CaptureWebSession     CaptureKind = "WEB_SESSION"
)

// CaptureSource retains a supplied historic locator, never live source proof,
// a token, a signing key, or evidence of repository provenance.
type CaptureSource struct {
	redacted
	kind    CaptureKind
	locator scopedID
}

func NewCaptureSource(scope kernel.OrgScope, kind CaptureKind, id string) (CaptureSource, error) {
	if scope.Validate() != nil {
		return CaptureSource{}, ErrScope
	}
	if kind != CaptureEnrolledDevice && kind != CaptureWebSession {
		return CaptureSource{}, ErrInvalid
	}
	locator, err := scoped(scope, id)
	if err != nil {
		return CaptureSource{}, err
	}
	return CaptureSource{kind: kind, locator: locator}, nil
}
func (s CaptureSource) Kind() CaptureKind            { return s.kind }
func (s CaptureSource) ID() string                   { return s.locator.id }
func (s CaptureSource) OrganizationID() kernel.OrgID { return s.locator.organization }

func (s CaptureSource) validate(scope kernel.OrgScope) error {
	if s.kind != CaptureEnrolledDevice && s.kind != CaptureWebSession {
		return ErrInvalid
	}
	return s.locator.validate(scope)
}
