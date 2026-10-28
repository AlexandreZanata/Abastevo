-- 000145: staff-roles-v2 adds the public "project_creator" badge (Criador do
-- projeto). Roles still live only in code and grant no permission (STF-01,
-- STF-03); the table CHECK mirrors the code catalog.
ALTER TABLE staff_assignments DROP CONSTRAINT staff_assignments_role_check;
ALTER TABLE staff_assignments ADD CONSTRAINT staff_assignments_role_check
    CHECK (role IN ('project_creator', 'community_moderator', 'station_reviewer',
                    'contributor_curator', 'support_agent'));
