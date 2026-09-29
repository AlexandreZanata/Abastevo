package domain

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/netip"
	"time"
)

// Quota operations. Reads stay outside transactional quotas; edge handles
// coarse read limits additively (see the edge plan in INFRASTRUCTURE_PLAN).
const (
	OperationRegister  = "register"
	OperationWrite     = "write"
	OperationChallenge = "challenge"
)

// OperationQuota is one versioned budget: at most Limit acceptances per
// sliding Window. Policy versions are recorded with the code that enforces
// them; adjustments ship as reviewed changes, never silent edits.
type OperationQuota struct {
	Limit  int32
	Window time.Duration
}

// QuotaPolicy is the audited v1 budget set. Registration stays tight
// against Sybil floods; writes allow pilot throughput; challenges sit
// below writes so nonce farming cannot amplify.
type QuotaPolicy struct {
	Version int
	Ops     map[string]OperationQuota
}

// DefaultQuotaPolicy returns the versioned v1 budgets.
func DefaultQuotaPolicy() QuotaPolicy {
	return QuotaPolicy{
		Version: 1,
		Ops: map[string]OperationQuota{
			OperationRegister:  {Limit: 10, Window: time.Hour},
			OperationWrite:     {Limit: 200, Window: time.Hour},
			OperationChallenge: {Limit: 60, Window: time.Hour},
		},
	}
}

// Lookup returns the budget for an operation.
func (p QuotaPolicy) Lookup(operation string) (OperationQuota, error) {
	q, ok := p.Ops[operation]
	if !ok || q.Limit <= 0 || q.Window <= 0 {
		return OperationQuota{}, fmt.Errorf("identity: unknown quota operation %q", operation)
	}
	return q, nil
}

var (
	ErrQuotaExceeded = errors.New("identity: quota exceeded")
	ErrBadIP         = errors.New("identity: malformed IP")
)

// HashIPSubject digests an IP under a rotating operator key. Only the
// "ip:<keyid>:<hex>" digest ever reaches storage; raw IPs stay out of the
// database entirely, and rotation retires old digests with their windows
// (short retention by construction).
func HashIPSubject(keyID string, key []byte, ip string) (string, error) {
	if keyID == "" || len(key) == 0 {
		return "", ErrBadIP
	}
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return "", ErrBadIP
	}
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(addr.String()))
	return "ip:" + keyID + ":" + hex.EncodeToString(mac.Sum(nil)), nil
}

// ContributorSubject addresses a per-key budget by fingerprint.
func ContributorSubject(fingerprint string) (string, error) {
	if _, err := ParseFingerprint(fingerprint); err != nil {
		return "", err
	}
	return fingerprint, nil
}

// WindowStart truncates now to the current window for an operation budget.
func WindowStart(now time.Time, window time.Duration) time.Time {
	return now.Truncate(window)
}
