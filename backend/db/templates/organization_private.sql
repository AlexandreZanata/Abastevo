-- EMP-PLAT-002 executable migration template, exercised on disposable PostGIS.
-- Copy and rename these example tables in the owning module's new migration.
-- Apply after EMP-ORG-002. Every private owner points to a permanent root.
-- The migrator owns these tables. Runtime API/worker logins receive only
-- schema USAGE and table SELECT/INSERT/UPDATE/DELETE, with no owner membership.
CREATE TABLE org_example_parent (
    org_id UUID NOT NULL CHECK (org_id <> '00000000-0000-0000-0000-000000000000'::uuid),
    id UUID NOT NULL,
    number TEXT NOT NULL,
    PRIMARY KEY (org_id, id),
    FOREIGN KEY (org_id) REFERENCES organizations(org_id),
    UNIQUE (org_id, number)
);

CREATE TABLE org_example_record (
    org_id UUID NOT NULL CHECK (org_id <> '00000000-0000-0000-0000-000000000000'::uuid),
    id UUID NOT NULL,
    parent_id UUID NOT NULL,
    number TEXT NOT NULL,
    revision BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0),
    PRIMARY KEY (org_id, id),
    UNIQUE (org_id, number),
    FOREIGN KEY (org_id, parent_id) REFERENCES org_example_parent (org_id, id)
);

-- Index child references for scoped joins and parent deletion checks.
CREATE INDEX org_example_record_parent_idx ON org_example_record (org_id, parent_id);

ALTER TABLE org_example_parent ENABLE ROW LEVEL SECURITY;
ALTER TABLE org_example_parent FORCE ROW LEVEL SECURITY;
CREATE POLICY org_example_parent_scope ON org_example_parent
    USING (org_id = NULLIF(current_setting('anpfuel.org_id', true), '')::uuid)
    WITH CHECK (org_id = NULLIF(current_setting('anpfuel.org_id', true), '')::uuid);

ALTER TABLE org_example_record ENABLE ROW LEVEL SECURITY;
ALTER TABLE org_example_record FORCE ROW LEVEL SECURITY;
CREATE POLICY org_example_record_scope ON org_example_record
    USING (org_id = NULLIF(current_setting('anpfuel.org_id', true), '')::uuid)
    WITH CHECK (org_id = NULLIF(current_setting('anpfuel.org_id', true), '')::uuid);
