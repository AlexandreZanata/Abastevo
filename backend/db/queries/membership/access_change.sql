-- name: MembershipAccessAccount :one
SELECT COALESCE(account_id::text,'')::text AS account_id FROM org_memberships WHERE org_id=CAST(@org_id::text AS uuid) AND id=CAST(@id::text AS uuid);
-- name: LockMembershipAccessAccount :exec
SELECT id FROM accounts WHERE id=CAST(@id::text AS uuid) FOR UPDATE;
-- name: ShareMembershipAccessAccount :exec
SELECT id FROM accounts WHERE id=CAST(@id::text AS uuid) FOR SHARE;
-- name: MembershipAccessPolicy :one
SELECT org_membership_access_policy(CAST(@org_id::text AS uuid),CAST(@id::text AS uuid),@action::text)::text AS policy;
-- name: MembershipAccessCommand :one
SELECT actor_account_id::text AS actor_id,kind,request_hash,reason,result,occurred_at FROM org_membership_access_commands WHERE org_id=CAST(@org_id::text AS uuid) AND operation_id=CAST(@operation_id::text AS uuid);
-- name: InsertMembershipAccessCommand :one
INSERT INTO org_membership_access_commands(org_id,operation_id,actor_account_id,membership_id,kind,request_hash,reason,result)
VALUES(CAST(@org_id::text AS uuid),CAST(@operation_id::text AS uuid),CAST(@actor_id::text AS uuid),CAST(@membership_id::text AS uuid),@kind::text,@request_hash::text,@reason::text,@result::jsonb)RETURNING occurred_at;
-- name: InsertMembershipAccessNotice :exec
INSERT INTO org_membership_access_notices(org_id,operation_id,membership_id,revision,kind,occurred_at)
SELECT org_id,operation_id,membership_id,(result->>'revision')::bigint,kind,occurred_at FROM org_membership_access_commands WHERE org_id=CAST(@org_id::text AS uuid) AND operation_id=CAST(@operation_id::text AS uuid);
