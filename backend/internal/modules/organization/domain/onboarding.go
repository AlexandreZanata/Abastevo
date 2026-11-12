package domain

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
)

const SyntheticTermsVersion = "enterprise-synthetic-dpa-v1"
const SyntheticTermsDigest = "590cc4cf31e1909ac9baacb76092897f3dba07b95719953b0f4588b51348f8b3"

var ErrTerms = errors.New("organization: configured terms acceptance required")

type privateSetup struct{}

func (privateSetup) Format(f fmt.State, _ rune)   { redact(f, "OrganizationSetup(redacted)") }
func (privateSetup) MarshalJSON() ([]byte, error) { return json.Marshal("OrganizationSetup(redacted)") }
func (privateSetup) LogValue() slog.Value         { return slog.StringValue("OrganizationSetup(redacted)") }

// TermsAcceptance is a declaration to record, not legal/provider approval.
// Only the explicitly synthetic source fixture is available in this task.
type TermsAcceptance struct {
	privateSetup
	Version, Digest string
	Accepted        bool
}
type CreationInput struct {
	privateSetup
	OperationID string
	Profile     Profile
	UnitName    string
	Terms       TermsAcceptance
}
type Creation struct {
	privateSetup
	value CreationInput
}

func NewCreation(v CreationInput) (Creation, error) {
	op, err := kernel.ParseOrgID(v.OperationID)
	if err != nil || op.String() != v.OperationID {
		return Creation{}, ErrInvalid
	}
	v.Profile.Name = strings.TrimSpace(v.Profile.Name)
	v.UnitName = strings.TrimSpace(v.UnitName)
	profile, err := canonical(v.Profile)
	if err != nil || blankName(profile.Name) || profile.TimeZone == "" || len(profile.Kinds) == 0 {
		return Creation{}, ErrInvalid
	}
	// The first unit name follows the same unique structure-name rule as later units.
	if _, err := structureText(v.UnitName); err != nil {
		return Creation{}, ErrInvalid
	}
	if !v.Terms.Accepted || v.Terms.Version != SyntheticTermsVersion || v.Terms.Digest != SyntheticTermsDigest {
		return Creation{}, ErrTerms
	}
	v.Profile = profile
	return Creation{value: v}, nil
}
func (c Creation) Validate() error        { _, err := NewCreation(c.value); return err }
func (c Creation) OperationID() string    { return c.value.OperationID }
func (c Creation) Profile() Profile       { return clone(c.value.Profile) }
func (c Creation) UnitName() string       { return c.value.UnitName }
func (c Creation) Terms() TermsAcceptance { return c.value.Terms }

// Digest excludes generated identifiers/transport proof and explicitly maps
// descriptions; marshaling a redacted Profile itself would lose intent.
func (c Creation) Digest() string {
	v := c.value
	raw, _ := json.Marshal([]any{1, "org.create", v.OperationID, v.Profile.Name, v.Profile.TimeZone, v.Profile.Kinds, v.UnitName, v.Terms.Version, v.Terms.Digest, v.Terms.Accepted})
	return fmt.Sprintf("%x", sha256.Sum256(raw))
}
