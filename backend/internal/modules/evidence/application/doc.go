// Package application owns the evidence reservation use-case (P05-T01):
// quota-checked creation of owned upload sessions with safe retry
// convergence, before any object-store work exists. Reserve composes the
// quota port, the natural-key store port and the domain through explicitly
// declared ports; later tasks add presigned issuance, verification and
// binding without changing this reservation contract. Depends on its
// domain and ports only — never on adapters, SQL or other modules.
package application
