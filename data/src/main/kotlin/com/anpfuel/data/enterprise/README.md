# Android enterprise storage and protection

EMP-PLAT-011 supplies immutable creation scope and a separate synthetic Room
foundation. EMP-PRIV-003 now completes signed custody, committed generations
and durable maintenance. Its local/native acceptance passes:
[completion record](../../../../../../../../docs/enterprise/evidence/f2-priv003-protection-completion.md).
The application authority binds the actual signed client; persistence and
protection share the accepted protectedStore singleton. The earlier
[V1 evidence](../../../../../../../../docs/enterprise/evidence/f1-priv003-protected-device-store.md)
is historical; its same-key rollback/lazy-maintenance limitations are the
reason for this continuation.

## Encrypted generations

ProtectedEnterprisePersistence seals records, pending operations, attachments,
caches and lease metadata together with AES-256-GCM. The authenticated frame
binds immutable account/canonical HTTPS origin/organization and generation;
the encrypted body binds exact membership/enrollment/revision and session
family. Provider-generated IVs satisfy Android Keystore's randomized-encryption
policy. Reads never create replacement decryption keys.

Every mutation prepares fresh payload and maintenance keys. Ciphertext data
and the encrypted maintenance descriptor are flushed, atomically replaced and
directory-fsynced before creating the trusted Keystore head marker. Only that
head selects acknowledged work; restorable files cannot lower it or select an
uncommitted generation. Cleanup preserves the current head/pair. Partial
security erasure retains the highest head floor until all decryption material
and files are verified gone and durable denial preserves the lost-work state,
preventing fallback or silent empty reset after independent failures.

Only opaque hashes/generation numbers appear in filenames and aliases.
The dedicated root remains noBackupFilesDir/enterprise-sealed-v1 so legacy
detection and existing cloud/device-transfer exclusions continue to apply.
There is no plaintext Room/WAL/journal/attachment fallback. Personal storage
and pending work are never imported or retagged.

An unbound legacy vault or V2 artifacts without a trusted head require explicit
acknowledged empty recovery with fresh exact proof. The latter conservatively
also covers an interrupted first preparation, which is indistinguishable from
loss of an acknowledged head. A preparation failure with a retained prior head
preserves its acknowledged generation. Malformed committed bytes/key loss are
unrecoverable; ordinary provider I/O outages remain retryable.

Allocation/decoder budgets remain32MiB and10,000 entries per snapshot,
independent of commercial quotas. Capacity failure preserves held work and
evicts nothing. Opaque discovery and preparation recovery also have bounded
allocation budgets; failure reports an outage rather than discarding content.
Owning features must measure capacity/write amplification before activation.
No dependency was added.

## Signed source contact and owned handoff

EnterpriseDeviceContactClient uses the existing identity-v1 key registration,
account binding, nonce and signature transport. It discovers/bootstrap-enrolls
only through the actual source, then calls the separate signed
POST /v1/orgs/{org_id}/devices/contact action for the exact membership/enrollment.
Company cookies are not account/device proof. Requests do not retry mutations,
follow redirects, persist cookies or accept mutable local policy as authority.

Only an exact ACTIVE existing-account response grants the approved30-day
custody lease. It is not business permission: each owning feature still needs
current role/scope/cache authority. Only exact LOST_OR_COMPROMISED maps to ERASE;
ACCESS_ENDED locks/preserves held ciphertext. Generic401/403/404, outages,
foreign identities, malformed wire and late replies never become wipe commands.
Managed server policy remains unavailable until its owning AUTH/GOV source
exists; no managed timeout is guessed.

The native request-start clock anchors validity conservatively through network
and durable I/O. Same-boot elapsed time enforces expiry. Reboot, backward wall
time or decreasing elapsed observations require fresh contact and a durable
non-destructive lock; restoring the clock/restarting the process cannot silently
remove that requirement. Proven privacy expiry discards keys. Ordinary logout
and session expiry retain encrypted pending work.

One exact selection owner may consume one expected E→E+1 handoff after durable
commit. Account/family/origin/background changes, replacement selection and late
responses invalidate that owner. The flow then opens a new company channel and
revalidates metadata; it never adopts an arbitrary current epoch. Same-family
token rotation preserves ownership. Epoch observers must be published outside
local ownership/fence locks, including nested access, to avoid synchronous
collector lock inversion.

The UI's system credential prompt grants no server authority and resumes no old
request; the person starts a fresh selection afterward. Confirmed lost work
requires a separate explicit empty-recovery acknowledgement bound to its
rendered selection/epoch. Managed keys enforce only an explicit supplied
credential/strong-biometric window. Unlock-required, permanent key loss,
ordinary outage and irreversible work loss remain distinct outcomes.

## Durable maintenance

Encrypted descriptors contain the scope/lease needed for expiry, bound to the
same trusted committed head. A maintenance key can operate without the managed
payload unlock. Denial intent has independent opaque filesystem and Keystore
markers; cleanup attempts independent channels and verifies disposal before
success. Neither ordinary logout nor a transient source denial queues erasure.

The existing WorkManager schedules startup and periodic maintenance. Each
worker owns its in-memory sweep handle and consumes cooperative slices of50;
concurrent workers/retries cannot inherit another invocation's cursor.
Discovery includes file, legacy/current key and durable account/partition intent
namespaces. Each item still needs exact trusted descriptor/ownership validation.
Failed cleanup remains retryable; no IDs, credentials or payloads enter worker
input/logs. Read-time expiry remains mandatory when the OS delays work.
No durable-success claim is made if all independent persistent channels fail.
Only a newly created reconfirmation requirement advances the session fence; an
existing one is a quiet wait for every maintenance, discovery and read path. A
bare account lock file is never prior data, and discovery of an account with no
protected footprint creates nothing (EDR-122).

## Validation

EMP-OFFL-003 adds a versioned QUEUE entry in the same sealed generation. Local
save and its owning projection commit together; each final immutable ACK and
its projection commit independently. Legacy opaque OPERATION bytes stay
retained and unsent. Exact account/origin/organization/member/enrollment and
current family/epoch/background leases surround storage, signing and transport.
Network never holds a vault/fence lock. Retry metadata uses structural CAS,
monotonic timing and retained server floors; no send attempt becomes acceptance.
The production producer registry stays empty until real owning integrations.

The separate operation WorkManager uses empty system input and discovers only
the current account/origin through authenticated protected descriptors. The
actual Android provider accepts exact scoped normal/denial/lock prefixes;
broader marker prefixes remain invalid. Native two-account key isolation and
real signed lost-answer/process-death recovery pass against ordinary PostGIS.
The worker idles while signed out, drains at most ten acknowledged batches per
partition per run, schedules a delayed continuation for a known due time and
keeps system backoff for unknown failures; Retry-After is bounded at the 6-hour
queue backoff cap (EDR-122).
[Queue evidence](../../../../../../../../docs/enterprise/evidence/f2-offl003-protected-operation-queue.md)
keeps host, native, normal product composition and integration states distinct.

Host crypto/files/process tests and actual ordinary PostGIS/signed HTTP source
tests are separate from Android Keystore/OS proof. The current completion record
pins actual commands, failure counterexamples, manifests and native acceptance
status. The isolated enterprisevalidation APK exercises real key use, explicit
synthetic managed unlock, process/reboot/clock behavior and framework
backup/restore with a public positive control. The independent data native test
uses actual identity keys and a disposable HTTPS/ordinary-PostGIS source,
without the synthetic Hilt UI graph supplying authority.

No real receipts, production credentials, precise GPS or company pilot data
belong in fixtures or logs. Emulator evidence certifies only its named image
and test boundaries; it is not physical/manufacturer or production acceptance.
