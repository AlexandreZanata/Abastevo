package domain

import "time"

// App Store subscription status values from the App Store Server API
// "Get All Subscription Statuses" response.
const (
	AppStoreStatusActive       = 1
	AppStoreStatusExpired      = 2
	AppStoreStatusBillingRetry = 3
	AppStoreStatusGrace        = 4
	AppStoreStatusRevoked      = 5
)

// AppStoreState maps one App Store subscription status to the frozen Premium
// lifecycle (B-BR-STORE-06). A revocation date always wins. An active
// subscription whose renewal is turned off keeps access until it ends, as a
// cancelled Play subscription does. Unknown values grant nothing.
func AppStoreState(status int, autoRenewOn, revoked bool) State {
	if revoked {
		return Revoked
	}
	switch status {
	case AppStoreStatusActive:
		if autoRenewOn {
			return Active
		}
		return Canceled
	case AppStoreStatusExpired:
		return Expired
	case AppStoreStatusBillingRetry:
		return Hold
	case AppStoreStatusGrace:
		return Grace
	case AppStoreStatusRevoked:
		return Revoked
	}
	return Unknown
}

// AppStoreValidUntil is the end of paid access: the transaction expiry, or
// the billing grace period end while the subscription is in grace.
func AppStoreValidUntil(state State, expires, graceEnds time.Time) time.Time {
	if state == Grace && graceEnds.After(expires) {
		return graceEnds
	}
	return expires
}
