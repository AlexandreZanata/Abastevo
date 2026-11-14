-- name: EmployeeMergeTeamPlans :many
SELECT l.id::text AS source_link_id,t.id::text AS team_id,t.revision AS team_revision,t.name,
 (t.state='ACTIVE' AND(t.unit_id IS NULL OR EXISTS(SELECT 1 FROM org_units u WHERE u.org_id=t.org_id AND u.id=t.unit_id AND u.state='ACTIVE')))::boolean AS admissible,
 COALESCE((SELECT s.id::text FROM org_team_links s WHERE s.org_id=l.org_id AND s.team_id=l.team_id AND s.employee_id=CAST(@survivor_id::text AS uuid) AND s.ended_at IS NULL),'')::text AS survivor_link_id
FROM org_team_links l JOIN org_teams t ON(t.org_id,t.id)=(l.org_id,l.team_id)
WHERE l.org_id=CAST(@org_id::text AS uuid) AND l.employee_id=CAST(@source_id::text AS uuid) AND l.ended_at IS NULL ORDER BY l.id LIMIT 501;
