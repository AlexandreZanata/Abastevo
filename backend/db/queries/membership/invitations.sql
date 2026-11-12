-- name: ListInvitations :many
SELECT COALESCE((to_jsonb(org_invitations)->>'failed_attempts')::integer,0)::integer AS failed_attempts,id::text AS id,employee_id::text AS employee_id,membership_id::text AS membership_id,issuer_membership_id::text AS issuer_membership_id,operation_id::text AS operation_id,
 state,revision,settings_revision,validity_days,created_at,expires_at,changed_at,grants,cause,source_type,COALESCE(source_id::text,'')::text AS source_id,COALESCE(source_revision,0)::bigint AS source_revision
FROM org_invitations WHERE org_id=CAST(@org_id::text AS uuid)
 AND(@id_filter::text='' OR id=NULLIF(@id_filter::text,'')::uuid)
 AND(@employee_filter::text='' OR employee_id=NULLIF(@employee_filter::text,'')::uuid)
 AND(@state_filter::text='' OR state= @state_filter::text)
 AND(@after_id::text='' OR id>NULLIF(@after_id::text,'')::uuid)
ORDER BY id LIMIT @row_limit::int;

-- name: InsertInvitation :exec
INSERT INTO org_invitations(org_id,id,employee_id,membership_id,issuer_membership_id,operation_id,verifier,state,grants,validity_days,settings_revision,expires_at)
VALUES(CAST(@org_id::text AS uuid),CAST(@id::text AS uuid),CAST(@employee_id::text AS uuid),CAST(@membership_id::text AS uuid),CAST(@issuer_id::text AS uuid),CAST(@operation_id::text AS uuid),@verifier::text,'PENDING',@grants::jsonb,@validity_days::int,@settings_revision::bigint,clock_timestamp());

-- name: TransitionInvitation :execrows
UPDATE org_invitations SET state= @state::text,revision=revision+1,cause= @cause::text
WHERE org_id=CAST(@org_id::text AS uuid) AND id=CAST(@id::text AS uuid) AND revision= @expected_revision::bigint AND state='PENDING' AND expires_at>clock_timestamp();

-- name: ExpireInvitations :exec
UPDATE org_invitations SET state='EXPIRED',revision=revision+1,cause='EXPIRED'
WHERE org_id=CAST(@org_id::text AS uuid) AND state='PENDING' AND expires_at<=clock_timestamp();

-- name: InvitationLiveMembership :one
SELECT id::text AS id,state,access_model,COALESCE(account_id::text,'')::text AS account_id FROM org_memberships WHERE org_id=CAST(@org_id::text AS uuid) AND employee_id=CAST(@employee_id::text AS uuid) AND state<>'ENDED';

-- name: InvitationCommand :one
SELECT actor_account_id::text AS actor_id,kind,request_hash,result FROM org_invitation_commands WHERE org_id=CAST(@org_id::text AS uuid) AND operation_id=CAST(@operation_id::text AS uuid);

-- name: InsertInvitationCommand :exec
INSERT INTO org_invitation_commands(org_id,operation_id,actor_account_id,invitation_id,kind,request_hash,result)
VALUES(CAST(@org_id::text AS uuid),CAST(@operation_id::text AS uuid),CAST(@actor_id::text AS uuid),CAST(@invitation_id::text AS uuid),@kind::text,@request_hash::text,@result::jsonb);

-- name: EndUnusedInvitationReservations :exec
UPDATE org_memberships m SET state='ENDED',revision=revision+1,end_reason='ADMISSION_ENDED'
WHERE m.org_id=CAST(@org_id::text AS uuid) AND m.state='INVITED' AND m.account_id IS NULL AND m.access_model='EXISTING_ACCOUNT'
 AND EXISTS(SELECT 1 FROM org_invitations i WHERE i.org_id=m.org_id AND i.membership_id=m.id)
 AND NOT EXISTS(SELECT 1 FROM org_invitations i WHERE i.org_id=m.org_id AND i.membership_id=m.id AND i.state='PENDING');
