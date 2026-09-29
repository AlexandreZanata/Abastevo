-- 000006: operation quotas (P03-T05). Append-only.
-- One counter per subject, operation and window start. Subjects are either
-- contributor fingerprints or rotating keyed IP digests ("ip:<keyid>:<hex>"),
-- never raw IPs or personal identifiers. Consumption is a single atomic
-- upsert capped at the configured limit, so no successful operation can
-- exceed quota under concurrency. Expired windows behave as absent and are
-- swept by CleanupExpiredWindows; retention of abuse evidence follows the
-- documented inventory, not this table.
CREATE TABLE identity_rate_windows (
    subject_digest TEXT NOT NULL,
    operation TEXT NOT NULL,
    window_start TIMESTAMPTZ NOT NULL,
    count INTEGER NOT NULL DEFAULT 1,
    expires_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (subject_digest, operation, window_start),
    CONSTRAINT identity_rate_windows_operation_check CHECK (operation IN
        ('register', 'write', 'challenge'))
);
CREATE INDEX identity_rate_windows_expiry_idx ON identity_rate_windows (expires_at);
