# RVA09 — bounded per-issuer JWKS recovery

Status: LOCAL_DONE / INTEGRATION_PENDING. B-BR-A03: valid issuer/audience/signature/nonce required.
Unknown key recovery may bypass TTL once per issuer per 30 seconds, including
failed requests. Requests inside the window reuse the recovered set/refusal;
they never authorize an unknown key or create per-kid unbounded state. Initial
issuer loading plus first forced recovery costs at most two fetches; subsequent
forced fetches at most one per 30s. Provider failures on ordinary loading also
have a 30s retry gate, never stale-key authorization. Legitimate unexpected key
rotation may require retry after that bounded window. No valid token is weakened.

Issuer single-flight gates are independent, context-cancellable and capped at
16 configured issuer entries. A network fetch never holds the global map mutex.
RED/GREEN: 20 invalid tokens cost at most two fetches, unique-kid burst/races stay
bounded, legitimate rotation resumes after cooldown, provider outage fails
closed and a blocked provider cannot hold another issuer's verification.

## Validation

Tested tree: `950e736` plus this task. RED: 20 invalid tokens caused 21 source
fetches. GREEN: same-key concurrent and distinct-key bursts cause at most two;
one provider's blocked fetch does not block another; waiting requests honor
cancellation; repeated provider outage attempts cost one fetch per cooldown.
Existing per-issuer TTL, valid rotation/removal, outage, signature/nonce and
redirect-refusal tests pass under race. The rotation test now advances the
injected clock through the documented cooldown before testing recovery; it
still verifies the real rotated key. Affected vet/gofmt/diff/secret checks passed.
No external provider was contacted. Global write/link-route admission remains
part of RVA12 rather than being inferred from cache limits.
