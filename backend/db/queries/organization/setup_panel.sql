-- name: ListCreatedSetupRefs :many
SELECT org_id::text AS org_id
FROM org_onboarding_commands
WHERE kind='CREATED' AND actor_id=CAST(@account_id::text AS uuid)
 AND(NULLIF(@after_org_id::text,'') IS NULL OR org_id>CAST(NULLIF(@after_org_id::text,'') AS uuid))
ORDER BY org_id LIMIT @row_limit::int;

-- name: ListSettingsEligibleMembers :many
SELECT m.id::text AS membership_id,COALESCE(p.display_name,'')::text AS display_name
FROM org_memberships m
LEFT JOIN org_employee_profiles p ON(p.org_id,p.employee_id)=(m.org_id,m.employee_id)
WHERE m.org_id=CAST(@org_id::text AS uuid) AND org_settings_member_available(m.org_id,m.id)
 AND(NULLIF(@after_membership_id::text,'') IS NULL OR m.id>CAST(NULLIF(@after_membership_id::text,'') AS uuid))
ORDER BY m.id LIMIT @row_limit::int;

-- name: SettingsSelectedMemberLabels :many
SELECT m.id::text AS membership_id,COALESCE(p.display_name,'')::text AS display_name,
 org_settings_member_available(m.org_id,m.id)::boolean AS eligible
FROM org_memberships m
LEFT JOIN org_employee_profiles p ON(p.org_id,p.employee_id)=(m.org_id,m.employee_id)
WHERE m.org_id=CAST(@org_id::text AS uuid) AND m.id::text=ANY(@membership_ids::text[])
ORDER BY m.id;
