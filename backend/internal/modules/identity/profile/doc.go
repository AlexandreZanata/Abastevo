// Package profile pins the anonymous proof-of-possession signature profile
// (P03-T01): golden vectors in contracts/testdata/identity plus two
// agreeing verifiers. The first follows the profile helpers; the second
// rebuilds the base and the EC point by hand so serialization mismatches
// cannot hide in shared code. Cross-library replay of these vectors is a
// later gate; the profile document is docs/security/identity-profile.md.
package profile
