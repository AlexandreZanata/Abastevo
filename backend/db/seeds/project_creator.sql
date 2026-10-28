-- Dev/staging seed: the project creator as the first public contributor.
--
-- Idempotent; safe to re-run. Never part of the migration chain (seeds are
-- not embedded), so production only gets this through an explicit operator
-- run. Creates a dedicated account (no login credentials) whose contributor
-- profile is public, carries an active CONTRIBUTOR grant from the first commit
-- of the project (2026-09-28) so it sorts first, and the public
-- "Criador do projeto" badge (staff-roles-v2 project_creator, no permission).
-- Owner-approved public data only: display name and GitHub handle.
--
-- Run:  psql "$ANPFUEL_DATABASE_URL" -v ON_ERROR_STOP=1 -f backend/db/seeds/project_creator.sql
BEGIN;

INSERT INTO accounts (id, alias, status, created_at)
VALUES ('c4ea7000-0000-4000-8000-000000000001', 'alexandre-zanata', 'active', '2026-09-28T16:50:30Z')
ON CONFLICT (id) DO UPDATE SET status = 'active';

INSERT INTO contributor_profiles (account_id, display_name, visibility, public_consent, show_roles_publicly)
VALUES ('c4ea7000-0000-4000-8000-000000000001', 'Alexandre Zanata', 'PUBLIC', true, true)
ON CONFLICT (account_id) DO UPDATE SET
    display_name = EXCLUDED.display_name, visibility = 'PUBLIC', public_consent = true,
    show_roles_publicly = true, updated_at = clock_timestamp();

INSERT INTO contributor_links (account_id, platform, handle)
VALUES ('c4ea7000-0000-4000-8000-000000000001', 'github', 'AlexandreZanata')
ON CONFLICT (account_id, platform) DO NOTHING;

INSERT INTO contributor_github_links (account_id, github_login)
SELECT 'c4ea7000-0000-4000-8000-000000000001', 'AlexandreZanata'
WHERE NOT EXISTS (
    SELECT 1 FROM contributor_github_links
    WHERE github_login = 'AlexandreZanata' AND revoked_at IS NULL
);

INSERT INTO entitlement_grants (id, account_id, tier, policy_version, state, start_at, reason)
SELECT 'c4ea7000-0000-4000-8000-000000000002', 'c4ea7000-0000-4000-8000-000000000001',
       'CONTRIBUTOR', 1, 'ACTIVE', '2026-09-28T16:50:30Z', 'Seed: project creator contributor grant'
WHERE NOT EXISTS (
    SELECT 1 FROM entitlement_grants
    WHERE account_id = 'c4ea7000-0000-4000-8000-000000000001' AND tier = 'CONTRIBUTOR' AND state = 'ACTIVE'
);

INSERT INTO staff_assignments (id, account_id, role, state, start_at, reason)
SELECT 'c4ea7000-0000-4000-8000-000000000003', 'c4ea7000-0000-4000-8000-000000000001',
       'project_creator', 'ACTIVE', '2026-09-28T16:50:30Z', 'Seed: project creator public badge'
WHERE NOT EXISTS (
    SELECT 1 FROM staff_assignments
    WHERE account_id = 'c4ea7000-0000-4000-8000-000000000001' AND role = 'project_creator' AND state = 'ACTIVE'
);

COMMIT;
