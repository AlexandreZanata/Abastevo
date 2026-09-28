# Data model (logical design)

Status: proposed, no production migrations in this delivery. One PostgreSQL/PostGIS database; table names below are explicit ownership prefixes, not separate services/databases. UTC `timestamptz`; UUID PKs; money `bigint` milli-BRL; exact source statistical decimals use `numeric`. Foreign keys RESTRICT by default; no broad CASCADE that erases history.

## Directory ownership

- `directory_stations(id,display_name,address,municipality_code,state,status,created_at)`; canonical ID independent of source identity.
- `directory_identifiers(id,station_id,kind,normalized_value,valid_from,valid_to,source_revision_id)`; unique active `(kind,normalized_value)`; ASCII normalized CNPJ with format/checksum validated by domain. Historical identifier changes remain recorded.
- `directory_location_revisions(id,station_id,point geography(Point,4326),quality,provider,source_reference,obtained_at,supersedes_id)`; append-only; current station location projection stores one reviewed point/quality and revision reference. GiST on current point, B-tree municipality/state; nullable/unverified location must remain explicit.
- `directory_condition_qualifiers(id,kind,normalized_name,status)` for canonical APP/LOYALTY/OTHER restrictions. Unresolved client text is not an automatically trusted catalog entry.

Geo nearby query uses `ST_DWithin` on geography (metres), indexed bounding prefilter then distance ordering and stable UUID tie-break. No computed full-table distance sort without indexed radius restriction. Verify plans with realistic density and EXPLAIN ANALYZE. [PostGIS documentation](https://postgis.net/docs/ST_DWithin.html) defines units and index behavior.

## Official ownership

- `official_import_runs(id,source_url,source_checksum,parser_version,discovered_at,status,started_at,finished_at,row_counts,error_summary)`; unique dataset identity+checksum+parser version; same bytes with changed parser produce a traceable reprocessing revision.
- `official_revisions(id,import_run_id,survey_start,survey_end,published_at,supersedes_revision_id,status)`; unique successful publication per revision ID; an active pointer selects current revision transactionally after complete validation.
- `official_station_prices(id,revision_id,station_id,fuel_product,unit,amount_milli_brl,raw_price_text,collected_on,source_row)`; unique `(revision_id,source_row)` and domain natural key including collection date/product/unit; duplicates with conflicting source values quarantined, not last-row-wins.
- `official_summary_prices(id,revision_id,scope,location_code,fuel_product,unit,mean,min,max,std_dev,station_count,raw_fields)`; explicit decimal columns, approved source fields only.
- `official_rejected_rows(id,import_run_id,row_number,reason_code,redacted_source)`; scoped retention. Raw source file checksum is immutable; raw public files archived separately if redistribution terms permit.

Indexes: station+fuel+collected_on DESC+revision, revision+station, summary scope/location/fuel/week. Approved read adapters can join directory station IDs, but only Official writes its tables. Partial import never changes the published revision pointer.

## Identity ownership

- `identity_contributors(id,status,created_at,deleted_at)`; no email/phone columns in MVP.
- `identity_keys(id,contributor_id,algorithm,public_jwk,fingerprint,created_at,revoked_at)`; unique fingerprint; no private key.
- `identity_challenges(id,nonce_hash,key_id_or_fingerprint,purpose,expires_at,consumed_at)`; atomic consume with condition `consumed_at IS NULL AND expires_at>server_now`; expiry index.
- `identity_idempotency(contributor_id,method,route,key,request_hash,response_code,response_json,created_at,expires_at)`; unique scope; no raw sensitive request body.
- `identity_rate_windows(subject_digest,operation,window_start,count,expires_at)`; unique tuple and atomic UPSERT under transaction. IP digests use rotating keyed hashing and short retention; public key identity is still personal/pseudonymous data.

## Community ownership

- `community_observations(id,contributor_ref,client_submission_id,station_id,fuel_product,unit,amount_milli_brl,condition_kind,qualifier_id,evidence_id,received_at,claimed_captured_at,supersedes_id,policy_version)`; immutable core. Unique contributor_ref+client_submission_id; indexes price key+received_at and contributor_ref+received_at. Check constraints mirror structural price/unit/condition rules.
- `community_private_observation_data(observation_id,encrypted_location,accuracy_m,fix_age_ms,ocr_claims,expires_at)`; separate short-lived personal data; restricted role. Delete after derivation within 24 h, never copy into events.
- `community_observation_decisions(id,observation_id,sequence,to_state,reason_codes,policy_version,occurred_at,actor_ref)`; unique observation+sequence. Latest state/signal projection is mutable and rebuildable.
- `community_confirmations(id,observation_id,contributor_ref,client_submission_id,received_at)`; unique observation+contributor and contributor+client submission. Eligibility joins use documented read port; attribution separate from publicly visible content.
- `community_disputes(id,target_observation_id,contributor_ref,client_submission_id,reason,replacement_id,received_at)` and append-only dispute decision history; partial unique open-report projection per reporter/target/reason.
- `community_current_prices(station_id,fuel_product,unit,condition_kind,qualifier_key,amount_milli_brl,availability,confidence,representative_observation_id,independent_supporters,confirmation_count,anchor_received_at,expires_at,next_recompute_at,computed_at,projection_version,algorithm_version,policy_config_version)`; composite PK uses nonnullable qualifier_key (`STANDARD` uses a documented sentinel) so NULL uniqueness cannot create duplicate rows. Index expires_at and station+fuel. Representative observation matches displayed amount/condition and is revalidated on confirmation. No private coordinates or contributor IDs.
- `community_projection_inputs(projection_key,version,input_cutoff,supporting_event_ids,reason_codes)`; restricted explanation snapshot with bounded retention; enough to reproduce while permitted source signals exist. After privacy erasure, record that full replay is no longer possible rather than keeping personal data indefinitely.

Queries need a narrow current-state/trust/evidence snapshot through declared ports/read adapters. No handler loads millions of facts per GET. `contributor_ref` is a random attribution token, not the public contributor UUID; a restricted identity-owned mapping resolves it for ownership/trust and deletion. Removing that mapping is not automatically anonymization: if retained content remains identifying/linkable, delete/anonymize it too under the reviewed policy. Normal business writes cannot change attribution; the narrow privacy workflow can remove it. Append-only protects business corrections, not an exemption from data rights.

## Evidence, trust, moderation, platform

- Evidence: `evidence_uploads(id,owner_ref,client_submission_id,quarantine_key,declared_size,declared_sha256,state,expires_at)`; `evidence_objects(id,upload_id,owner_ref,final_key,verified_sha256,perceptual_hash,mime,width,height,size_bytes,ready_at,delete_after,deleted_at)`. Unique upload/client command; one active binding per observation/evidence. Object key is server-generated and private. Verified hash index for duplicate signals.
- Trust: `trust_decisions(id,contributor_ref,tier,reason,case_refs,policy_version,occurred_at)`, `trust_current(contributor_ref,tier,version,updated_at)`; immutable decisions, rebuildable current view.
- Moderation: `moderation_cases(id,target_type,target_id,status,priority,opened_at)`, `moderation_actions(id,case_id,actor_id,action,reason,occurred_at,request_id)`; index open priority/date; no arbitrary payload dump or unbounded GPS copy.
- Platform: `jobs(id,type,payload_version,payload,status,dedupe_key,attempts,scheduled_at,lease_until,lease_token,started_at,finished_at,last_error_code)`; unique dedupe key; partial ready index `(scheduled_at,id)` where queued, lease index for expired running jobs. Payload includes IDs, no sensitive media/location.
- Privacy: `privacy_requests(id,contributor_ref,type,status,created_at,completed_at)`, minimal `privacy_deletion_ledger` needed to reapply removals after restore; restricted access. Ledger itself has a documented retention bounded by backup horizon and legal need.
- Migrations: ordered version/checksum ledger; dedicated migration role, advisory lock and checksum mismatch failure.

## Transaction boundaries and constraints

Observation insert + initial event + idempotency result + validation job is one transaction. Confirmation unique insert + idempotency + consensus job is one transaction. Moderation action + domain decision + recomputation jobs is one transaction. Cross-module application orchestration can share a transaction through an explicitly owned unit of work; no nested autonomous commits.

FK relationships enforce existence without handing ownership to callers. Optimistic version checks protect aggregate decisions; per-price-key locks serialize projection publication. Unique constraints arbitrate concurrent first creation/confirmations; don't rely on check-then-insert application code. API/worker use separate least-privilege roles; raw observation price updates are unavailable through normal repositories/roles. Personal-data cleanup has explicit narrow privileges.

## Retention, volume and evolution

See SECURITY_PRIVACY for the authoritative proposed retention periods. Initial sizing assumptions: 20k station-price rows/week means ~1.04M/year before corrections; 1,000 observations/day means 365k/year plus votes/events; 10,000/day means 3.65M/year. These are workload scenarios, not actual current usage. Measure relation/index size and WAL amplification from seeded data.

Do not partition v1. Reassess time partitions when retention deletion/vacuum, index size or p95 queries miss budgets after indexing/tuning. A future partition design must preserve natural uniqueness, FKs and idempotency across partitions; document it in an ADR before migration. Object storage growth depends on photo rate×average bytes×retention, not MAU alone.

Migrations are SQL and append-only after application. Every migration includes forward change, lock estimate, compatibility with old application version, recovery/rollback plan and tests from empty plus previous supported schema. Irreversible data transformations require backup and explicit operator review. Expand→deploy→backfill→validate→contract; rolling back a binary must not require deleting user data.
