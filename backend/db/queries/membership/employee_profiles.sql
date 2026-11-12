-- name: GetEmployeeRecord :one
SELECT e.id::text AS id,e.state,e.revision AS identity_revision,COALESCE(e.merged_into::text,'')::text AS merged_into,
 COALESCE(p.revision,0)::bigint AS profile_revision,COALESCE(p.display_name,'')::text AS display_name,
 COALESCE(p.registration_number,'')::text AS registration_number,COALESCE(p.work_email,'')::text AS work_email,
 COALESCE(p.work_phone,'')::text AS work_phone,COALESCE(p.job_title,'')::text AS job_title,COALESCE(p.home_unit_id::text,'')::text AS home_unit_id
FROM org_employee_identities e LEFT JOIN org_employee_profiles p ON (p.org_id,p.employee_id)=(e.org_id,e.id)
WHERE e.org_id=CAST(@org_id::text AS uuid) AND e.id=CAST(@id::text AS uuid);

-- name: InsertEmployeeProfile :exec
INSERT INTO org_employee_profiles(org_id,employee_id,display_name,registration_number,work_email,work_phone,job_title,home_unit_id)
VALUES(CAST(@org_id::text AS uuid),CAST(@employee_id::text AS uuid),@display_name::text,@registration_number::text,@work_email::text,@work_phone::text,@job_title::text,NULLIF(@home_unit_id::text,'')::uuid);

-- name: ChangeEmployeeProfile :execrows
UPDATE org_employee_profiles SET display_name= @display_name::text,registration_number= @registration_number::text,
 work_email= @work_email::text,work_phone= @work_phone::text,job_title= @job_title::text,home_unit_id=NULLIF(@home_unit_id::text,'')::uuid,revision=revision+1
WHERE org_id=CAST(@org_id::text AS uuid) AND employee_id=CAST(@employee_id::text AS uuid) AND revision= @expected_revision::bigint;

-- name: EmployeeRegistrationExists :one
SELECT EXISTS(SELECT 1 FROM org_employee_profiles WHERE org_id=CAST(@org_id::text AS uuid)
 AND employee_id<>CAST(@exclude_id::text AS uuid) AND @registration_number::text<>'' AND lower(registration_number)=lower(@registration_number::text)) AS duplicate;

-- name: ListEmployeeRecords :many
SELECT e.id::text AS id,e.state,e.revision AS identity_revision,COALESCE(e.merged_into::text,'')::text AS merged_into,
 COALESCE(p.revision,0)::bigint AS profile_revision,COALESCE(p.display_name,'')::text AS display_name,
 COALESCE(p.registration_number,'')::text AS registration_number,COALESCE(p.work_email,'')::text AS work_email,
 COALESCE(p.work_phone,'')::text AS work_phone,COALESCE(p.job_title,'')::text AS job_title,COALESCE(p.home_unit_id::text,'')::text AS home_unit_id
FROM org_employee_identities e LEFT JOIN org_employee_profiles p ON (p.org_id,p.employee_id)=(e.org_id,e.id)
WHERE e.org_id=CAST(@org_id::text AS uuid) AND (@state_filter::text='' OR e.state= @state_filter::text)
 AND (@after_id::text='' OR e.id>NULLIF(@after_id::text,'')::uuid)
ORDER BY e.id LIMIT @row_limit::int;

-- name: MatchEmployeeRecords :many
SELECT e.id::text AS id,e.state,e.revision AS identity_revision,COALESCE(e.merged_into::text,'')::text AS merged_into,
 p.revision AS profile_revision,p.display_name,p.registration_number,p.work_email,p.work_phone,p.job_title,COALESCE(p.home_unit_id::text,'')::text AS home_unit_id
FROM org_employee_identities e JOIN org_employee_profiles p ON (p.org_id,p.employee_id)=(e.org_id,e.id)
WHERE e.org_id=CAST(@org_id::text AS uuid) AND e.id<>CAST(@exclude_id::text AS uuid)
 AND lower(regexp_replace(p.display_name,' +',' ','g'))=lower(regexp_replace(@display_name::text,' +',' ','g'))
 AND ((@work_email::text<>'' AND p.work_email= @work_email::text) OR (@work_phone::text<>'' AND p.work_phone= @work_phone::text))
 AND (@after_id::text='' OR e.id>NULLIF(@after_id::text,'')::uuid)
ORDER BY e.id LIMIT @row_limit::int;

-- name: EmployeeCommandReceipt :one
SELECT actor_id::text AS actor_id,employee_id::text AS employee_id,kind,request_hash,
 result_profile_revision,result_identity_revision,result_state,duplicate_reviewed
FROM org_employee_commands WHERE org_id=CAST(@org_id::text AS uuid) AND operation_id=CAST(@operation_id::text AS uuid);

-- name: InsertEmployeeReceipt :exec
INSERT INTO org_employee_commands(org_id,operation_id,actor_id,employee_id,kind,request_hash,result_profile_revision,result_identity_revision,result_state,duplicate_reviewed)
VALUES(CAST(@org_id::text AS uuid),CAST(@operation_id::text AS uuid),CAST(@actor_id::text AS uuid),CAST(@employee_id::text AS uuid),@kind::text,@request_hash::text,@result_profile_revision::bigint,@result_identity_revision::bigint,@result_state::text,@duplicate_reviewed::boolean);
