package domain

// Basis is the BR-REV-009 counting basis of a review state.
type Basis string

const (
	BasisCounted  Basis = "COUNTED"
	BasisPending  Basis = "PENDING"
	BasisExcluded Basis = "EXCLUDED"
)

// Basis classifies a valid state for costs and indicators.
func (s State) Basis() (Basis, error) {
	switch s {
	case Approved:
		return BasisCounted, nil
	case Submitted, InReview:
		return BasisPending, nil
	case Draft, Returned, Rejected, Cancelled:
		return BasisExcluded, nil
	}
	return "", ErrInvalid
}

// Presentation is the review dimension shown on a device. ServerConfirmed is
// false when no server case exists; ReadyToSend flags a pending local author
// command that has not been accepted by the server.
type Presentation struct {
	State           State
	ServerConfirmed bool
	ReadyToSend     bool
}

// Present combines the last server-confirmed state (nil when the server has
// no case) with an optional pending local author command. Local commands
// never advance the state; local decisions are not representable.
func Present(server *State, local *Kind) (Presentation, error) {
	if local != nil && local.Role() != RoleAuthor {
		return Presentation{}, ErrInvalid
	}
	if server == nil {
		if local != nil && *local != Submit {
			return Presentation{}, ErrInvalid
		}
		return Presentation{State: Draft, ReadyToSend: local != nil}, nil
	}
	if !server.valid() {
		return Presentation{}, ErrInvalid
	}
	return Presentation{State: *server, ServerConfirmed: true, ReadyToSend: local != nil}, nil
}
