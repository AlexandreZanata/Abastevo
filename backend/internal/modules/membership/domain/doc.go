// Package domain separates private employee identity, membership periods,
// account/credential references and device enrolment. A person is a human,
// not a new platform-wide record. Existing account identities remain owned
// by account; this package neither provisions them nor issues sessions.
//
// This core contains identity/lifecycle associations, not employee descriptive
// fields (IDN-002) or licence contents/eligibility (IDN-014/F2). Conditions are
// facts resolved by trusted commands under current authority, never client
// permission claims. Persistence, concurrent uniqueness, audit, last-owner
// locking and atomic access effects belong to subsequent tasks.
package domain
