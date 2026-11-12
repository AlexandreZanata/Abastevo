package orgapi

import (
	"context"
	"time"
)

// WriteAdmission is supplied by source composition, after current authority.
// Validate refuses a write route without an explicit matching rate policy.
type WriteAdmission interface {
	Validate(Rule) error
	Check(context.Context, Principal, Rule) error
}

type RateLimitError struct{ RetryAfter time.Duration }

func (*RateLimitError) Error() string { return string(RateLimited) }
func (*RateLimitError) Unwrap() error { return RateLimited }
