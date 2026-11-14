-- name: InstallInvitationLocator :exec
SELECT set_config('anpfuel.invitation_locator',@locator::text,true);
-- name: ResolveInvitationLocator :one
SELECT org_id::text AS org_id FROM org_invitation_locators WHERE invitation_id=CAST(@locator::text AS uuid);
-- name: RedemptionVerifier :one
SELECT verifier,failed_attempts FROM org_invitations WHERE org_id=CAST(@org_id::text AS uuid) AND id=CAST(@id::text AS uuid);
-- name: FailedInvitationSecret :exec
UPDATE org_invitations SET failed_attempts=failed_attempts+1,state=CASE WHEN failed_attempts=4 THEN 'REVOKED' ELSE state END,revision=CASE WHEN failed_attempts=4 THEN revision+1 ELSE revision END,cause=CASE WHEN failed_attempts=4 THEN 'TOO_MANY_ATTEMPTS' ELSE cause END
WHERE org_id=CAST(@org_id::text AS uuid) AND id=CAST(@id::text AS uuid) AND state='PENDING' AND expires_at>clock_timestamp() AND failed_attempts<5;
-- name: ConsumeInvitation :execrows
UPDATE org_invitations SET state= @state::text,revision=revision+1,cause= @cause::text WHERE org_id=CAST(@org_id::text AS uuid) AND id=CAST(@id::text AS uuid) AND state='PENDING' AND expires_at>clock_timestamp();
-- name: ExistingInvitationAccountMember :one
SELECT id::text AS id FROM org_memberships WHERE org_id=CAST(@org_id::text AS uuid) AND account_id=CAST(@account_id::text AS uuid) AND state<>'ENDED';
-- name: PersonalInvitationReceipt :one
SELECT actor_account_id::text AS actor_id,kind,request_hash,result FROM org_invitation_person_commands WHERE org_id=CAST(@org_id::text AS uuid) AND operation_id=CAST(@operation_id::text AS uuid);
-- name: InsertPersonalInvitationReceipt :exec
INSERT INTO org_invitation_person_commands(org_id,operation_id,actor_account_id,invitation_id,kind,request_hash,result)VALUES(CAST(@org_id::text AS uuid),CAST(@operation_id::text AS uuid),CAST(@actor_id::text AS uuid),CAST(@invitation_id::text AS uuid),@kind::text,@request_hash::text,@result::jsonb);
-- name: BeginInvitationAcceptance :exec
INSERT INTO org_invitation_acceptances(org_id,invitation_id,membership_id,account_id,account_alias,state,deadline,confirmation_required,settings_revision)VALUES(CAST(@org_id::text AS uuid),CAST(@invitation_id::text AS uuid),CAST(@membership_id::text AS uuid),CAST(@account_id::text AS uuid),@alias::text,'WAITING',clock_timestamp()+interval '168 hours',@confirmation_required::boolean,@settings_revision::bigint);
-- name: InvitationAcceptance :one
SELECT invitation_id::text AS invitation_id,membership_id::text AS membership_id,COALESCE(account_id::text,'')::text AS account_id,account_alias,state,revision,accepted_at,deadline,confirmation_required,settings_revision FROM org_invitation_acceptances WHERE org_id=CAST(@org_id::text AS uuid) AND invitation_id=CAST(@id::text AS uuid);
-- name: TransitionInvitationAcceptance :execrows
UPDATE org_invitation_acceptances SET state= @state::text,revision=revision+1,cause= @cause::text,confirmed_by=NULLIF(@confirmed_by::text,'')::uuid WHERE org_id=CAST(@org_id::text AS uuid) AND invitation_id=CAST(@id::text AS uuid) AND state='WAITING' AND revision= @revision::bigint AND deadline>clock_timestamp();
-- name: ExpireInvitationAcceptances :exec
UPDATE org_invitation_acceptances SET state='EXPIRED',revision=revision+1,cause='CONFIRMATION_EXPIRED' WHERE org_id=CAST(@org_id::text AS uuid) AND state='WAITING' AND deadline<=clock_timestamp();
