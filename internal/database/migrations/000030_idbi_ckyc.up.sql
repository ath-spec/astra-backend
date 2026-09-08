-- IDBI integration, feature 6: CKYC verification (415 searchCkycDetails).
-- Fills the existing KYC handler stub (POST /api/v1/kyc/pan/verify). New
-- table rather than the PAN-verification-shaped kyc_verifications (000006),
-- since 415 returns a CKYC record (ckycId, ckycName, id-type list), not a
-- PAN/Aadhaar-seeding result. Gated by IDBI_KYC_ENABLED.

CREATE TABLE IF NOT EXISTS idbi_ckyc (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id        UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    pan            VARCHAR(10) NOT NULL,
    ckyc_available BOOLEAN NOT NULL DEFAULT false,
    ckyc_id        TEXT NOT NULL DEFAULT '',
    ckyc_name      TEXT NOT NULL DEFAULT '',
    ckyc_acc_type  TEXT NOT NULL DEFAULT '',
    ckyc_gen_date  TEXT NOT NULL DEFAULT '',
    id_types       TEXT[] NOT NULL DEFAULT '{}',
    verified_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, pan)
);
