# Backup operations (P08-T03)

Encrypted off-host backup pipeline: daily logical dumps with roles
and extensions manifest, SHA-256 verification, 7 daily + 4 weekly
copies, and a stale-age alarm. Objectives (INFRASTRUCTURE_PLAN):
RPO ≤24 h, RTO ≤4 h; pilot acceptance required.

## Commands

```sh
# Create (writes outbox/anpfuel-<ts>.dump.enc + .sha256.json, prunes old copies)
ANPFUEL_BACKUP_COMPOSE_FILE=infra/compose.staging.yml \
ANPFUEL_BACKUP_DB_USER=... ANPFUEL_BACKUP_DB_NAME=... ANPFUEL_BACKUP_DB_PASSWORD=... \
ANPFUEL_BACKUP_OUTBOX=/srv/anpfuel/backups \
ANPFUEL_BACKUP_PASSPHRASE="$(cat /srv/anpfuel/backup.passphrase)" \
  bash infra/scripts/backup.sh run

# Verify (decrypt + checksums + archive catalog list)
ANPFUEL_BACKUP_PASSPHRASE="$(cat /srv/anpfuel/backup.passphrase)" \
  bash infra/scripts/backup.sh verify /srv/anpfuel/backups/anpfuel-<ts>.dump.enc

# Stale alarm for monitoring (fails past ANPFUEL_BACKUP_MAX_AGE_HOURS, default 26)
ANPFUEL_BACKUP_OUTBOX=/srv/anpfuel/backups bash infra/scripts/backup.sh check-age

# Test the whole pipeline (disposable dev database, nothing provisioned)
make test-backup
```

## Schedule and transport

- Cron daily on the backup host (example `0 3 * * *` invoking `run`
  with the environment above); the stale alarm wires into the
  operator monitoring from P08-T05 (backup age >26 h).
- The outbox syncs off-host via operator transport (rsync/rclone/S3
  with credentials separate from the database): the pipeline writes
  self-contained artifacts, transport only moves bytes.
- Retention pruning runs inside every `run`: newest 7 daily copies
  plus newest 4 Sunday copies survive; anything older than 35 days
  drops regardless (backup horizon).

## Credential scope

- Database backup credentials read the database only (dump role);
  they never leave the backup host and never enter Git.
- The encryption passphrase lives outside the VPS (recovery needs
  only the passphrase plus any PostgreSQL 18 + PostGIS host with the
  pinned images). Losing the passphrase loses the backups: no
  backdoor, by design.
- The manifest carries names and hashes only (verified secret-free
  by the harness); it may sit next to the artifact.
- Transport credentials are separate from database credentials, so a
  leaked transport key cannot read the database (and the bytes it
  fetches stay encrypted).

## Recovery linkage

- `ANPFUEL_BACKUP_MANIFEST` in `deploy.sh` points at the newest
  verified manifest: no release rolls out without a fresh backup.
- Restore drills rebuild into an isolated environment, verify
  schema/counts/hashes, sample read paths, rebuild projections,
  reconcile jobs and evidence references, replay the deletion ledger
  and apply retention before traffic (see [recovery.md](recovery.md)).
- Transient tables (exact GPS/OCR, challenges, IP windows) are
  excluded from long-lived assurance by retention, not by backup
  filters: restores must reject pending validation whose transient
  evidence is unavailable instead of fabricating it.
