package application

import "errors"

var (
	// ErrUnauthorized marks a missing owner identity. Callers map it
	// onto 401 without distinguishing anything else.
	ErrUnauthorized = errors.New("privacy: owner proof required")
	// ErrConflict marks a divergent-payload retry on a converged
	// operation identity. Callers map it onto 409.
	ErrConflict = errors.New("privacy: conflicting request")
)
