-- name: RedemptionIssuerAccount :one
SELECT COALESCE(m.account_id::text,'')::text AS account_id FROM org_invitations i JOIN org_memberships m ON(m.org_id,m.id)=(i.org_id,i.issuer_membership_id) WHERE i.org_id=CAST(@org_id::text AS uuid) AND i.id=CAST(@id::text AS uuid);
-- name: RedemptionOrganizationFacts :one
SELECT display_name,revision,state,has_snapshot,EXISTS(SELECT 1 FROM org_legal_entities l WHERE l.org_id=organizations.org_id AND l.state='VERIFIED') AS verified FROM organizations WHERE org_id=CAST(@org_id::text AS uuid);
-- name: RedemptionSourceFacts :one
SELECT m.id::text AS member_id,m.employee_id::text AS issuer_employee_id,m.state AS issuer_state,e.state AS issuer_employee_state,t.state AS employee_state,m.account_id::text AS issuer_account_id,a.status AS issuer_account_state,EXISTS(SELECT 1 FROM payment_billing_exits b WHERE b.account_id=m.account_id AND b.reason='ACCOUNT_DELETION') AS issuer_deletion
FROM org_invitations i JOIN org_memberships m ON(m.org_id,m.id)=(i.org_id,i.issuer_membership_id) JOIN org_employee_identities e ON(e.org_id,e.id)=(m.org_id,m.employee_id) JOIN org_employee_identities t ON(t.org_id,t.id)=(i.org_id,i.employee_id) JOIN accounts a ON a.id=m.account_id WHERE i.org_id=CAST(@org_id::text AS uuid) AND i.id=CAST(@id::text AS uuid);
-- name: RedemptionAccountAlias :one
SELECT alias FROM accounts WHERE id=CAST(@id::text AS uuid);

-- name: PersonalRedemptionProof :one
SELECT a.status,(a.enterprise_managed_org_id IS NOT NULL)::boolean AS managed,
 EXISTS(SELECT 1 FROM payment_billing_exits b WHERE b.account_id=a.id AND b.reason='ACCOUNT_DELETION') AS deletion_pending,
 EXISTS(SELECT 1 FROM company_web_sessions c JOIN account_session_families f ON(f.account_id,f.id)=(c.account_id,c.family_id)
 WHERE c.account_id=a.id AND c.id=CAST(@session_id::text AS uuid) AND c.revoked_at IS NULL AND f.revoked_at IS NULL
 AND c.created_at<=clock_timestamp() AND c.last_seen_at<=clock_timestamp() AND c.last_seen_at>clock_timestamp()-interval '30 minutes'
 AND c.expires_at>clock_timestamp() AND c.authenticated_at<=clock_timestamp() AND c.authenticated_at>clock_timestamp()-interval '15 minutes'
 AND f.issued_at<=clock_timestamp() AND f.issued_at>clock_timestamp()-interval '30 days') AS proof
FROM accounts a WHERE a.id=CAST(@account_id::text AS uuid);
