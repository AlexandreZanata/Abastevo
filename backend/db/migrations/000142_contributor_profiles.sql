-- 000142: contributor profiles, GitHub links, claims and snapshots
-- (ADR-023, B-BR-CTB-01..14). Public reads expose only consented display
-- fields; account erasure deletes or revokes every contributor row.
CREATE TABLE contributor_profiles (
    account_id UUID PRIMARY KEY REFERENCES accounts(id),
    display_name TEXT NOT NULL CHECK (char_length(display_name) BETWEEN 2 AND 40),
    visibility TEXT NOT NULL DEFAULT 'HIDDEN' CHECK (visibility IN ('PUBLIC', 'HIDDEN')),
    public_consent BOOLEAN NOT NULL DEFAULT false,
    show_roles_publicly BOOLEAN NOT NULL DEFAULT false,
    hidden_by_admin BOOLEAN NOT NULL DEFAULT false,
    hidden_reason TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    CONSTRAINT contributor_profiles_hidden CHECK (
        (hidden_by_admin AND hidden_reason IS NOT NULL) OR
        (NOT hidden_by_admin AND hidden_reason IS NULL)
    )
);
CREATE INDEX contributor_profiles_public_idx ON contributor_profiles(visibility, hidden_by_admin)
    WHERE visibility = 'PUBLIC' AND NOT hidden_by_admin;

CREATE TABLE contributor_links (
    account_id UUID NOT NULL REFERENCES accounts(id),
    platform TEXT NOT NULL CHECK (platform IN ('github', 'gitlab', 'linkedin', 'x', 'instagram', 'youtube', 'bluesky')),
    handle TEXT NOT NULL CHECK (octet_length(handle) BETWEEN 1 AND 255),
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (account_id, platform)
);
CREATE FUNCTION contributor_links_max_two() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF (SELECT count(*) FROM contributor_links WHERE account_id = NEW.account_id) >= 2 THEN
        RAISE EXCEPTION 'contributor_links_max_two';
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER contributor_links_max_two BEFORE INSERT ON contributor_links
FOR EACH ROW EXECUTE FUNCTION contributor_links_max_two();

CREATE TABLE contributor_github_links (
    account_id UUID NOT NULL REFERENCES accounts(id),
    github_login TEXT NOT NULL CHECK (octet_length(github_login) BETWEEN 1 AND 39),
    linked_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    revoked_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX contributor_github_links_account_active
    ON contributor_github_links(account_id) WHERE revoked_at IS NULL;
CREATE UNIQUE INDEX contributor_github_links_login_active
    ON contributor_github_links(github_login) WHERE revoked_at IS NULL;

CREATE TABLE contributor_claims (
    id UUID PRIMARY KEY,
    account_id UUID NOT NULL REFERENCES accounts(id),
    operation_id UUID NOT NULL,
    github_login TEXT NOT NULL CHECK (octet_length(github_login) BETWEEN 1 AND 39),
    code TEXT NOT NULL CHECK (octet_length(code) BETWEEN 0 AND 128),
    state TEXT NOT NULL DEFAULT 'PENDING' CHECK (state IN ('PENDING', 'APPROVED', 'REJECTED', 'WITHDRAWN', 'EXPIRED')),
    proof_status TEXT NOT NULL DEFAULT 'UNVERIFIED' CHECK (proof_status IN ('UNVERIFIED', 'VERIFIED')),
    gist_id TEXT,
    override BOOLEAN NOT NULL DEFAULT false,
    decision_reason TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    expires_at TIMESTAMPTZ NOT NULL,
    decided_at TIMESTAMPTZ,
    UNIQUE (account_id, operation_id),
    CONSTRAINT contributor_claims_terminal CHECK (
        (state = 'PENDING' AND decided_at IS NULL AND decision_reason IS NULL) OR
        (state <> 'PENDING' AND decided_at IS NOT NULL)
    )
);
CREATE UNIQUE INDEX contributor_claims_one_open ON contributor_claims(account_id)
    WHERE state = 'PENDING';
CREATE INDEX contributor_claims_window_idx ON contributor_claims(account_id, created_at DESC);
CREATE INDEX contributor_claims_admin_idx ON contributor_claims(state, created_at DESC, id DESC);

CREATE TABLE contributor_profile_edits (
    account_id UUID NOT NULL REFERENCES accounts(id),
    edited_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);
CREATE INDEX contributor_profile_edits_window_idx
    ON contributor_profile_edits(account_id, edited_at DESC);

CREATE TABLE github_contributor_snapshots (
    repo TEXT PRIMARY KEY,
    logins JSONB NOT NULL,
    etag TEXT,
    fetched_at TIMESTAMPTZ NOT NULL
);

CREATE FUNCTION contributor_erase_deleted_owner() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    DELETE FROM contributor_claims WHERE account_id = NEW.id;
    DELETE FROM contributor_profile_edits WHERE account_id = NEW.id;
    DELETE FROM contributor_links WHERE account_id = NEW.id;
    UPDATE contributor_github_links SET revoked_at = clock_timestamp()
        WHERE account_id = NEW.id AND revoked_at IS NULL;
    DELETE FROM contributor_profiles WHERE account_id = NEW.id;
    RETURN NEW;
END $$;
CREATE TRIGGER contributor_owner_erasure AFTER UPDATE OF status ON accounts
FOR EACH ROW WHEN (NEW.status = 'deleted' AND OLD.status IS DISTINCT FROM NEW.status)
EXECUTE FUNCTION contributor_erase_deleted_owner();
