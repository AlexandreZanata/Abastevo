// Package domain separates private employee identity, membership periods,
// account/credential references and device enrolment. A person is a human,
// not a new platform-wide record. Existing account identities remain owned
// by account; this package neither provisions them nor issues sessions.
//
// Descriptive employee profiles have their own revisions (IDN-002), separate
// from identity lifecycle and membership periods. Licence contents/eligibility
// remain IDN-014/F2. Conditions are facts resolved by trusted commands under
// current authority, never client permission claims. Persistence composes RLS,
// uniqueness, audit and existing last-owner/access lifecycle guards explicitly;
// editing a profile never provisions or transitions access.
package domain
