-- EMP-ORG-007 review correction. An invitation that never granted access is
-- not an access reduction: only ACTIVE/SUSPENDED -> SUSPENDED/ENDED closes the
-- employee's current team periods. A withdrawn/expired invitation (or erasure
-- of an invited-only account) keeps them. The function is unchanged.
DROP TRIGGER org_team_consume_membership ON org_memberships;
CREATE TRIGGER org_team_consume_membership AFTER UPDATE OF state ON org_memberships FOR EACH ROW
 WHEN(OLD.state IN('ACTIVE','SUSPENDED') AND NEW.state IN('SUSPENDED','ENDED') AND OLD.state IS DISTINCT FROM NEW.state) EXECUTE FUNCTION org_team_consume_membership();
