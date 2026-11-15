-- EMP-OFFL-006. Per-membership reference feed log derived on pull (EDR-218):
-- a monotonic head and append-only entries carrying digests, never content.
-- No trigger on fleet tables, no backfill, no runtime grants.
CREATE TABLE org_reference_feed_heads(
 org_id UUID NOT NULL,membership_id UUID NOT NULL,
 epoch UUID NOT NULL CHECK(epoch<>'00000000-0000-0000-0000-000000000000'::uuid),
 revision BIGINT NOT NULL DEFAULT 0 CHECK(revision>=0),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(org_id,membership_id),
 FOREIGN KEY(org_id,membership_id) REFERENCES org_memberships(org_id,id)
);
CREATE TABLE org_reference_feed_entries(
 org_id UUID NOT NULL,membership_id UUID NOT NULL,revision BIGINT NOT NULL CHECK(revision>0),
 entity_type TEXT NOT NULL CHECK(entity_type IN('asset')),
 entity_id UUID NOT NULL CHECK(entity_id<>'00000000-0000-0000-0000-000000000000'::uuid),
 op TEXT NOT NULL CHECK(op IN('UPSERT','DELETE')),
 digest TEXT NOT NULL CHECK((op='UPSERT' AND digest~'^[a-f0-9]{64}$') OR(op='DELETE' AND digest='')),
 recorded_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(org_id,membership_id,revision),
 FOREIGN KEY(org_id,membership_id) REFERENCES org_reference_feed_heads(org_id,membership_id)
);
CREATE INDEX org_reference_feed_entity ON org_reference_feed_entries(org_id,membership_id,entity_type,entity_id,revision);
DO $$DECLARE name TEXT;BEGIN
 FOREACH name IN ARRAY ARRAY['org_reference_feed_heads','org_reference_feed_entries'] LOOP
 EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY',name);EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY',name);
 EXECUTE format('CREATE POLICY %I ON %I USING(org_id=NULLIF(current_setting(''anpfuel.org_id'',true),'''')::uuid) WITH CHECK(org_id=NULLIF(current_setting(''anpfuel.org_id'',true),'''')::uuid)',name||'_scope',name);
 END LOOP;
END $$;
REVOKE ALL ON org_reference_feed_heads,org_reference_feed_entries FROM PUBLIC;
CREATE TRIGGER org_reference_feed_entries_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON org_reference_feed_entries FOR EACH STATEMENT EXECUTE FUNCTION org_sync_keep_history();
CREATE TRIGGER org_reference_feed_heads_kept BEFORE DELETE OR TRUNCATE ON org_reference_feed_heads FOR EACH STATEMENT EXECUTE FUNCTION org_sync_keep_history();

-- A head never moves back and never changes its epoch; it starts at zero.
CREATE FUNCTION org_reference_feed_head_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 IF TG_OP='INSERT' THEN
  IF NEW.revision<>0 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='reference feed head unavailable';END IF;
  NEW.created_at=clock_timestamp();RETURN NEW;
 END IF;
 IF NEW.membership_id<>OLD.membership_id OR NEW.epoch<>OLD.epoch OR NEW.created_at<>OLD.created_at OR NEW.revision<OLD.revision THEN
  RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='reference feed head is monotonic';END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER org_reference_feed_head_guard BEFORE INSERT OR UPDATE ON org_reference_feed_heads FOR EACH ROW EXECUTE FUNCTION org_reference_feed_head_guard();

-- An entry is the next revision of its head (advanced first, in the same
-- transaction), alternates state per entity and is stamped by the server.
CREATE FUNCTION org_reference_feed_entry_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE head BIGINT;last_op TEXT;last_digest TEXT;last_revision BIGINT;
BEGIN
 IF NEW.org_id IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 SELECT revision INTO head FROM public.org_reference_feed_heads WHERE org_id=NEW.org_id AND membership_id=NEW.membership_id;
 SELECT max(revision) INTO last_revision FROM public.org_reference_feed_entries WHERE org_id=NEW.org_id AND membership_id=NEW.membership_id;
 IF head IS NULL OR NEW.revision>head OR NEW.revision<>COALESCE(last_revision,0)+1 THEN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='reference feed revision unavailable';END IF;
 SELECT op,digest INTO last_op,last_digest FROM public.org_reference_feed_entries WHERE org_id=NEW.org_id AND membership_id=NEW.membership_id AND entity_type=NEW.entity_type AND entity_id=NEW.entity_id ORDER BY revision DESC LIMIT 1;
 IF(NEW.op='DELETE' AND last_op IS DISTINCT FROM 'UPSERT') OR(NEW.op='UPSERT' AND last_op='UPSERT' AND last_digest=NEW.digest) THEN
  RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='reference feed change unavailable';END IF;
 NEW.recorded_at=clock_timestamp();RETURN NEW;
END $$;
CREATE TRIGGER org_reference_feed_entry_guard BEFORE INSERT ON org_reference_feed_entries FOR EACH ROW EXECUTE FUNCTION org_reference_feed_entry_guard();
