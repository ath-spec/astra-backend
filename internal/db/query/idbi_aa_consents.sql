-- name: CreateConsent :exec
INSERT INTO idbi_aa_consents (
    consent_handle, user_id, consent_id, status, party_id_type, party_id_value,
    vua, product_id, account_id, redirect_url, web_redirect_url, created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, now(), now())
ON CONFLICT (consent_handle) DO UPDATE SET
    consent_id       = COALESCE(NULLIF(EXCLUDED.consent_id, ''), idbi_aa_consents.consent_id),
    status           = CASE WHEN idbi_aa_consents.status = 'PENDING'
                            THEN EXCLUDED.status ELSE idbi_aa_consents.status END,
    redirect_url     = COALESCE(NULLIF(EXCLUDED.redirect_url, ''), idbi_aa_consents.redirect_url),
    web_redirect_url = COALESCE(NULLIF(EXCLUDED.web_redirect_url, ''), idbi_aa_consents.web_redirect_url),
    updated_at       = now();

-- name: GetConsent :one
SELECT consent_handle, user_id, consent_id, status, party_id_type, party_id_value,
       vua, product_id, account_id, redirect_url, web_redirect_url,
       approved_at, expires_at, last_fetched_at, created_at, updated_at
FROM idbi_aa_consents
WHERE consent_handle = $1;

-- name: ListConsentsByUser :many
SELECT consent_handle, user_id, consent_id, status, party_id_type, party_id_value,
       vua, product_id, account_id, redirect_url, web_redirect_url,
       approved_at, expires_at, last_fetched_at, created_at, updated_at
FROM idbi_aa_consents
WHERE user_id = $1
ORDER BY created_at DESC;

-- name: UpdateConsentStatus :execrows
UPDATE idbi_aa_consents SET
    status      = $2,
    consent_id  = COALESCE(NULLIF($3::text, ''), consent_id),
    approved_at = COALESCE($4, approved_at),
    updated_at  = now()
WHERE consent_handle = $1;

-- name: MarkConsentFetched :exec
UPDATE idbi_aa_consents
SET last_fetched_at = now(), updated_at = now()
WHERE consent_handle = $1;
