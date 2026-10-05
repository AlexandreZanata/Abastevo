-- Owned by stationprofile (review and grants, P31-T03). Audited
-- decisions plus narrowly scoped capability. Concurrent approvals
-- converge on the active-grant guard; reviewer/self, stale-operator
-- and dead-account cases fail before any write.

-- name: CreateClaimDecision :one
INSERT INTO claim_decisions (id, claim_id, reviewer, decision, reason, policy_version, proof_version, operator_cnpj, scopes)
VALUES (@id, @claim_id, @reviewer, @decision, @reason, @policy_version, @proof_version, @operator_cnpj, @scopes)
RETURNING id, claim_id, reviewer, decision, reason, policy_version, proof_version, operator_cnpj, scopes, decided_at;

-- name: CreateGrant :one
INSERT INTO representation_grants (id, account_id, station_id, operator_cnpj, role, scopes, version, claim_id, decision_id)
VALUES (@id, @account_id, @station_id, @operator_cnpj, @role, @scopes, @version, @claim_id, @decision_id)
ON CONFLICT (account_id, station_id) WHERE status = 'active' DO NOTHING
RETURNING id, account_id, station_id, operator_cnpj, role, scopes, version, status, claim_id, decision_id, valid_from, valid_to;

-- name: ActiveGrant :one
SELECT id, account_id, station_id, operator_cnpj, role, scopes, version, status, claim_id, decision_id, valid_from, valid_to
FROM representation_grants
WHERE account_id = @account_id AND station_id = @station_id AND status = 'active';
