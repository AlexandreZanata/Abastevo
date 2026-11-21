-- EMP-SUBS-005 and EMP-FLEET-005: include a retained fueling in the asset's
-- protected usage history. The exact foreign-key guard fails closed for any
-- other new source. Fleet usage counts the receipt without posting a charge.
CREATE INDEX org_held_fuelings_asset_lookup ON org_held_fuelings (org_id,asset_id);

CREATE OR REPLACE FUNCTION org_asset_current_usage(selected_org UUID,selected_asset UUID) RETURNS JSONB
 LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 IF selected_org IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=selected_org AND has_snapshot FOR UPDATE;
 IF NOT FOUND OR NOT EXISTS(SELECT 1 FROM public.org_assets WHERE org_id=selected_org AND id=selected_asset) THEN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='private reference not found';END IF;
 IF EXISTS(SELECT 1 FROM pg_constraint c WHERE c.contype='f' AND c.confrelid='public.org_assets'::regclass
 AND NOT((c.conrelid='public.org_asset_tanks'::regclass AND c.conname='org_asset_tanks_asset_fkey')
 OR(c.conrelid='public.org_asset_expected'::regclass AND c.conname='org_asset_expected_asset_fkey')
 OR(c.conrelid='public.org_asset_unit_allocations'::regclass AND c.conname='org_asset_allocation_asset_fkey')
 OR(c.conrelid='public.org_asset_revisions'::regclass AND c.conname='org_asset_revision_asset_fkey')
 OR(c.conrelid='public.org_meters'::regclass AND c.conname='org_meters_asset_fkey')
 OR(c.conrelid='public.org_meter_held_readings'::regclass AND c.conname='org_meter_held_readings_asset_fkey')
 OR(c.conrelid='public.org_asset_unit_periods'::regclass AND c.conname='org_asset_unit_periods_asset_fkey')
 OR(c.conrelid='public.org_asset_cost_centre_periods'::regclass AND c.conname='org_asset_cost_centre_periods_asset_fkey')
 OR(c.conrelid='public.org_role_assignments'::regclass AND c.conname='org_role_assignments_scope_asset_fkey')
 OR(c.conrelid=to_regclass('public.org_document_links') AND c.conname='org_document_links_asset_fkey')
 OR(c.conrelid='public.org_asset_assignments'::regclass AND c.conname='org_asset_assignments_asset_fkey')
 OR(c.conrelid='public.org_fuelings'::regclass AND c.conname='org_fuelings_asset_fkey')
 OR(c.conrelid='public.org_asset_reservations'::regclass AND c.conname='org_asset_reservations_asset_fkey')
 OR(c.conrelid='public.org_asset_blocks'::regclass AND c.conname='org_asset_blocks_asset_fkey')
 OR(c.conrelid='public.org_trips'::regclass AND c.conname='org_trips_asset_fkey')
 OR(c.conrelid='public.org_trip_held_starts'::regclass AND c.conname='org_trip_held_starts_asset_fkey')
 OR(c.conrelid='public.org_asset_implement_attachments'::regclass AND c.conname='org_asset_implement_attachments_implement_fkey')
 OR(c.conrelid='public.org_asset_implement_attachments'::regclass AND c.conname='org_asset_implement_attachments_powered_fkey')
 OR(c.conrelid='public.org_checklist_template_targets'::regclass AND c.conname='org_checklist_template_targets_asset_fkey')
 OR(c.conrelid='public.org_trip_missions'::regclass AND c.conname='org_trip_missions_asset_fkey')
 OR(c.conrelid='public.org_incident_reports'::regclass AND c.conname='org_incident_reports_asset_fkey')
 OR(c.conrelid='public.org_trip_mission_vehicles'::regclass AND c.conname='org_trip_mission_vehicles_previous_fkey')
 OR(c.conrelid='public.org_trip_mission_vehicles'::regclass AND c.conname='org_trip_mission_vehicles_asset_fkey')
 OR(c.conrelid='public.org_expenses'::regclass AND c.conname='org_expenses_asset_fkey')
 OR(c.conrelid='public.org_checklist_runs'::regclass AND c.conname='org_checklist_runs_asset_fkey')
 OR(c.conrelid='public.org_fuel_authorizations'::regclass AND c.conname='org_fuel_authorizations_asset_fkey')
 OR(c.conrelid='public.org_held_fuelings'::regclass AND c.conname='org_held_fuelings_org_id_asset_id_fkey'))) THEN RAISE EXCEPTION USING ERRCODE='P7904',MESSAGE='asset usage source unavailable';END IF;
 RETURN jsonb_build_object(
   'readings', (SELECT count(*) FROM public.org_meter_readings r
       JOIN public.org_meters m ON(m.org_id,m.id)=(r.org_id,r.meter_id)
       WHERE m.org_id=selected_org AND m.asset_id=selected_asset),
   'operations', (SELECT count(*) FROM public.org_sync_operations o
       JOIN public.org_sync_effects e USING(org_id,operation_id)
       WHERE o.org_id=selected_org AND o.entity_id=selected_asset AND o.entity_type='fleet_asset')
     + (SELECT count(*) FROM public.org_meter_held_readings h WHERE h.org_id=selected_org AND h.asset_id=selected_asset)
     + (SELECT count(*) FROM public.org_fuelings f WHERE f.org_id=selected_org AND f.asset_id=selected_asset)
     + (SELECT count(*) FROM public.org_trips t WHERE t.org_id=selected_org AND t.asset_id=selected_asset)
     + (SELECT count(*) FROM public.org_trip_held_starts h WHERE h.org_id=selected_org AND h.asset_id=selected_asset)
     + (SELECT count(*) FROM public.org_asset_implement_attachments i
         WHERE i.org_id=selected_org AND (i.implement_id=selected_asset OR i.powered_asset_id=selected_asset))
     + (SELECT count(*) FROM public.org_held_fuelings h WHERE h.org_id=selected_org AND h.asset_id=selected_asset)
 );
END $$;
