-- name: LockLegalAccount :one
SELECT status,COALESCE(enterprise_managed_org_id::text,'')::text AS managed_org,
 EXISTS(SELECT 1 FROM payment_billing_exits WHERE account_id=accounts.id AND reason='ACCOUNT_DELETION') AS deletion_pending
FROM accounts WHERE id=CAST(@account_id::text AS uuid) FOR UPDATE;

-- name: ShareLegalAccount :one
-- Reads share the account row: concurrent reads proceed, writers still wait.
SELECT status,COALESCE(enterprise_managed_org_id::text,'')::text AS managed_org,
 EXISTS(SELECT 1 FROM payment_billing_exits WHERE account_id=accounts.id AND reason='ACCOUNT_DELETION') AS deletion_pending
FROM accounts WHERE id=CAST(@account_id::text AS uuid) FOR SHARE;

-- name: LockLegalProof :one
SELECT c.id::text AS id FROM company_web_sessions c
JOIN account_session_families f ON (f.account_id,f.id)=(c.account_id,c.family_id)
WHERE c.id=CAST(@session_id::text AS uuid) AND c.account_id=CAST(@account_id::text AS uuid)
 AND c.revoked_at IS NULL AND f.revoked_at IS NULL
 AND c.created_at<=clock_timestamp() AND c.last_seen_at<=clock_timestamp()
 AND c.last_seen_at>clock_timestamp()-interval '30 minutes' AND c.expires_at>clock_timestamp()
 AND (NOT @require_fresh::boolean OR (c.authenticated_at<=clock_timestamp() AND c.authenticated_at>clock_timestamp()-interval '15 minutes'))
 AND f.issued_at<=clock_timestamp() AND f.issued_at>clock_timestamp()-interval '30 days'
FOR SHARE OF c,f;

-- name: LockLegalOrganization :one
SELECT state,has_snapshot,EXISTS(SELECT 1 FROM org_onboarding_commands c WHERE c.org_id=organizations.org_id
 AND c.kind='CREATED' AND c.terms_kind='SYNTHETIC' AND c.terms_version= @terms_version::text AND c.terms_digest= @terms_digest::text) AS synthetic_setup
FROM organizations WHERE org_id=CAST(@org_id::text AS uuid) FOR UPDATE;

-- name: ShareLegalOrganization :one
SELECT state,has_snapshot,EXISTS(SELECT 1 FROM org_onboarding_commands c WHERE c.org_id=organizations.org_id
 AND c.kind='CREATED' AND c.terms_kind='SYNTHETIC' AND c.terms_version= @terms_version::text AND c.terms_digest= @terms_digest::text) AS synthetic_setup
FROM organizations WHERE org_id=CAST(@org_id::text AS uuid) FOR SHARE;

-- name: GetLegalEntity :one
SELECT id::text AS id,kind,legal_name,identifier,purpose,state,revision,verification_history
FROM org_legal_entities WHERE org_id=CAST(@org_id::text AS uuid) AND id=CAST(@id::text AS uuid);

-- name: ListLegalEntities :many
SELECT id::text AS id,kind,legal_name,identifier,purpose,state,revision,verification_history
FROM org_legal_entities WHERE org_id=CAST(@org_id::text AS uuid)
 AND (@state_filter::text='' OR state= @state_filter::text)
 AND (NULLIF(@after_id::text,'') IS NULL OR id>CAST(NULLIF(@after_id::text,'') AS uuid))
ORDER BY id LIMIT @row_limit::int;

-- name: GetEstablishment :one
SELECT id::text AS id,legal_entity_id::text AS legal_entity_id,cnpj,declared_role,trade_name,municipality_code,revision
FROM org_establishments WHERE org_id=CAST(@org_id::text AS uuid) AND id=CAST(@id::text AS uuid);

-- name: ListLegalEstablishments :many
SELECT id::text AS id,legal_entity_id::text AS legal_entity_id,cnpj,declared_role,trade_name,municipality_code,revision
FROM org_establishments WHERE org_id=CAST(@org_id::text AS uuid) AND legal_entity_id=CAST(@legal_entity_id::text AS uuid)
 AND (NULLIF(@after_id::text,'') IS NULL OR id>CAST(NULLIF(@after_id::text,'') AS uuid))
ORDER BY id LIMIT @row_limit::int;

-- name: InsertLegalEntity :exec
INSERT INTO org_legal_entities(org_id,id,kind,legal_name,identifier,purpose,state)
VALUES(CAST(@org_id::text AS uuid),CAST(@id::text AS uuid),@kind::text,@legal_name::text,@identifier::text,@purpose::text,'DECLARED');

-- name: InsertLegalEstablishment :exec
INSERT INTO org_establishments(org_id,id,legal_entity_id,cnpj,declared_role,trade_name,municipality_code)
VALUES(CAST(@org_id::text AS uuid),CAST(@id::text AS uuid),CAST(@legal_entity_id::text AS uuid),@cnpj::text,@declared_role::text,@trade_name::text,@municipality_code::text);

-- name: ChangeLegalEstablishment :execrows
UPDATE org_establishments SET declared_role= @declared_role::text,trade_name= @trade_name::text,municipality_code= @municipality_code::text,revision=revision+1
WHERE org_id=CAST(@org_id::text AS uuid) AND id=CAST(@id::text AS uuid) AND revision= @expected_revision::bigint;

-- name: DeleteLegalDeclaration :execrows
DELETE FROM org_legal_entities WHERE org_id=CAST(@org_id::text AS uuid) AND id=CAST(@id::text AS uuid) AND revision= @expected_revision::bigint;

-- name: RevokeLegalDeclaration :execrows
UPDATE org_legal_entities SET state='REVOKED',revision=revision+1
WHERE org_id=CAST(@org_id::text AS uuid) AND id=CAST(@id::text AS uuid) AND revision= @expected_revision::bigint;

-- name: LegalCommandReceipt :one
SELECT actor_id::text AS actor_id,kind,request_hash,legal_entity_id::text AS legal_entity_id,
 COALESCE(establishment_id::text,'')::text AS establishment_id,result_state,result_revision
FROM org_legal_commands WHERE org_id=CAST(@org_id::text AS uuid) AND operation_id=CAST(@operation_id::text AS uuid);

-- name: InsertLegalCommandReceipt :exec
INSERT INTO org_legal_commands(org_id,operation_id,actor_id,kind,request_hash,legal_entity_id,establishment_id,result_state,result_revision)
VALUES(CAST(@org_id::text AS uuid),CAST(@operation_id::text AS uuid),CAST(@actor_id::text AS uuid),@kind::text,@request_hash::text,
 CAST(@legal_entity_id::text AS uuid),CAST(NULLIF(@establishment_id::text,'') AS uuid),@result_state::text,@result_revision::bigint);

-- name: LegalTargetExists :one
SELECT CASE WHEN @establishment_id::text='' THEN
 EXISTS(SELECT 1 FROM org_legal_entities WHERE org_id=CAST(@org_id::text AS uuid) AND id=CAST(@legal_entity_id::text AS uuid))
 OR EXISTS(SELECT 1 FROM org_legal_commands WHERE org_id=CAST(@org_id::text AS uuid) AND legal_entity_id=CAST(@legal_entity_id::text AS uuid) AND kind='DECLARE')
ELSE
 EXISTS(SELECT 1 FROM org_establishments WHERE org_id=CAST(@org_id::text AS uuid) AND id=CAST(NULLIF(@establishment_id::text,'') AS uuid) AND legal_entity_id=CAST(@legal_entity_id::text AS uuid))
 OR EXISTS(SELECT 1 FROM org_legal_commands WHERE org_id=CAST(@org_id::text AS uuid) AND legal_entity_id=CAST(@legal_entity_id::text AS uuid) AND establishment_id=CAST(NULLIF(@establishment_id::text,'') AS uuid) AND kind IN ('DECLARE','ADD_ESTABLISHMENT'))
END::boolean AS exists;
