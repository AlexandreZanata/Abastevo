-- EMP-IDN-016: account-bound discovery of opaque current membership references.
-- This SELECT-only exception is confined to the unselected bootstrap. Every
-- organization read resets it and installs the actual organization scope.
CREATE POLICY org_memberships_account_context_read ON org_memberships FOR SELECT
 USING(NULLIF(current_setting('anpfuel.org_id',true),'') IS NULL
  AND account_id=NULLIF(current_setting('anpfuel.account_context',true),'')::uuid
  AND state='ACTIVE' AND access_model='EXISTING_ACCOUNT');
