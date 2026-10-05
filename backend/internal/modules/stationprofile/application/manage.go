package application

import (
	"context"
	"errors"
	"strings"

	profiledomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/stationprofile/domain"
)

// ManagePorts isolates scoped business management: live grant and
// operator checks, profile projection writes, feedback reply
// submission and business attribution. Every privilege enforces
// server-side before any app button.
type ManagePorts struct {
	Grants      DecisionStore
	Profiles    ProfileStore
	OperatorOf  func(ctx context.Context, stationID string) (cnpj string, found bool, err error)
	AccountLive func(ctx context.Context, accountID string) (bool, error)
	SubmitReply func(ctx context.Context, accountID, stationID, product, text string) (commentID string, err error)
	Attribute   func(ctx context.Context, commentID, accountID, stationID, grantID string) error
}

var (
	ErrManageGrant   = errors.New("stationprofile: no active grant with this scope")
	ErrManageStale   = errors.New("stationprofile: operator changed since grant")
	ErrManageFields  = errors.New("stationprofile: business fields outside the frozen policy")
	ErrManageVersion = errors.New("stationprofile: profile changed, reload and retry")
)

// allowedBusinessFields mirrors the frozen P30-T01 field policy.
var allowedBusinessFields = map[string]bool{
	"opening_hours": true, "services": true, "phone": true,
	"website": true, "description": true,
}

var allowedServices = map[string]bool{
	"fuel": true, "convenience": true, "carwash": true, "tire-service": true,
	"oil-change": true, "restaurant": true, "atm": true, "restroom": true,
	"wifi": true, "parking": true,
}

// liveGrant loads the caller's active grant for the station and
// verifies scope, account liveness and operator currency in one
// place. Canonical facts (name/address/location/operator) never
// pass through here: they route to Directory review.
func liveGrant(ctx context.Context, ports ManagePorts, accountID, stationID, scope string) (GrantRow, error) {
	if accountID == "" || stationID == "" {
		return GrantRow{}, ErrClaimNotFound
	}
	grant, found, err := ports.Grants.ActiveGrant(ctx, accountID, stationID)
	if err != nil {
		return GrantRow{}, err
	}
	if !found {
		return GrantRow{}, ErrManageGrant
	}
	live, err := ports.AccountLive(ctx, accountID)
	if err != nil {
		return GrantRow{}, err
	}
	if !live {
		return GrantRow{}, ErrAccountGone
	}
	cnpj, found, err := ports.OperatorOf(ctx, stationID)
	if err != nil {
		return GrantRow{}, err
	}
	if !found || cnpj == "" || cnpj != grant.OperatorCNPJ {
		return GrantRow{}, ErrManageStale
	}
	hasScope := false
	for _, granted := range grant.Scopes {
		if granted == scope {
			hasScope = true
		}
	}
	if !hasScope {
		return GrantRow{}, ErrManageGrant
	}
	return grant, nil
}

// EditBusinessFields applies permitted field changes under the
// current approved scope with optimistic revisioning. Only frozen
// policy fields pass; regulatory identity, prices, locations and
// community facts are unreachable here.
func EditBusinessFields(ctx context.Context, ports ManagePorts, accountID, stationID string, expectedRevision int, fields map[string]string) (StoredProfile, error) {
	grant, err := liveGrant(ctx, ports, accountID, stationID, "profile.edit")
	if err != nil {
		return StoredProfile{}, err
	}
	sanitized, err := sanitizeBusinessFields(fields)
	if err != nil {
		return StoredProfile{}, err
	}
	_ = grant
	return ports.Profiles.UpdateProjection(ctx, stationID, expectedRevision, sanitized)
}

func sanitizeBusinessFields(fields map[string]string) (map[string]string, error) {
	out := map[string]string{}
	for key, value := range fields {
		if !allowedBusinessFields[key] {
			return nil, ErrManageFields
		}
		value = strings.TrimSpace(value)
		switch key {
		case "services":
			if value == "" {
				return nil, ErrManageFields
			}
			for _, service := range strings.Split(value, ",") {
				if !allowedServices[strings.TrimSpace(service)] {
					return nil, ErrManageFields
				}
			}
			out[key] = value
		case "description":
			if value == "" || len([]rune(value)) > 280 {
				return nil, ErrManageFields
			}
			out[key] = value
		case "phone", "website", "opening_hours":
			if value == "" || len(value) > 256 {
				return nil, ErrManageFields
			}
			out[key] = value
		}
	}
	if len(out) == 0 {
		return nil, ErrManageFields
	}
	return out, nil
}

// PostOfficialReply publishes a business reply through the existing
// feedback transport with server-verified at-time attribution: grant
// scope `reply.official`, live account/operator rechecked at
// publication, 280-scalar text rules enforced by the transport, and
// the grant id stamped only after the comment authorship check
// passes. No vote weight, ranking or criticism suppression follows.
func PostOfficialReply(ctx context.Context, ports ManagePorts, accountID, stationID, product, text string) (commentID string, err error) {
	grant, err := liveGrant(ctx, ports, accountID, stationID, "reply.official")
	if err != nil {
		return "", err
	}
	commentID, err = ports.SubmitReply(ctx, accountID, stationID, product, text)
	if err != nil {
		return "", err
	}
	if err := ports.Attribute(ctx, commentID, accountID, stationID, grant.ID); err != nil {
		return "", err
	}
	_ = profiledomain.PolicyVersion
	return commentID, nil
}
