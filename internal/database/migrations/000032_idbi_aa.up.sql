-- IDBI integration, feature 4: Account Aggregator (AA) consent + linked
-- accounts (590 requestConsent / 591 getConsentList / 592 webRedirection /
-- 497 pushConsentNotification / 498 pushDataNotification / 595 statement).
-- New tables — the existing aa_handler.go stub kept no state at all (it
-- returned a fake "CONSENT-xxxx"). AA statement transactions are written
-- into spend_transactions with source='aa' (000028), so the analytics
-- engine picks them up with no further change. Gated by IDBI_AA_ENABLED.

CREATE TABLE IF NOT EXISTS idbi_aa_consents (
    consent_handle   TEXT PRIMARY KEY,
    user_id          UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    consent_id       TEXT NOT NULL DEFAULT '',
    status           TEXT NOT NULL DEFAULT 'PENDING',
    party_id_type    TEXT NOT NULL DEFAULT 'MOBILE',
    party_id_value   TEXT NOT NULL DEFAULT '',
    vua              TEXT NOT NULL DEFAULT '',
    product_id       TEXT NOT NULL DEFAULT '',
    account_id       TEXT NOT NULL DEFAULT '',
    redirect_url     TEXT NOT NULL DEFAULT '',
    web_redirect_url TEXT NOT NULL DEFAULT '',
    approved_at      TIMESTAMPTZ,
    expires_at       TIMESTAMPTZ,
    last_fetched_at  TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_idbi_aa_consents_user ON idbi_aa_consents (user_id);

CREATE TABLE IF NOT EXISTS idbi_aa_linked_accounts (
    consent_handle        TEXT NOT NULL REFERENCES idbi_aa_consents (consent_handle) ON DELETE CASCADE,
    link_ref_number       TEXT NOT NULL,
    fip_id                TEXT NOT NULL DEFAULT '',
    fip_name              TEXT NOT NULL DEFAULT '',
    account_type          TEXT NOT NULL DEFAULT '',
    fi_type               TEXT NOT NULL DEFAULT '',
    masked_account_number TEXT NOT NULL DEFAULT '',
    linked_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (consent_handle, link_ref_number)
);
