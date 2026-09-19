-- name: UpsertLinkedAccount :exec
INSERT INTO idbi_aa_linked_accounts (
    consent_handle, link_ref_number, fip_id, fip_name,
    account_type, fi_type, masked_account_number, linked_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, now())
ON CONFLICT (consent_handle, link_ref_number) DO UPDATE SET
    fip_id                = EXCLUDED.fip_id,
    fip_name              = EXCLUDED.fip_name,
    account_type          = EXCLUDED.account_type,
    fi_type               = EXCLUDED.fi_type,
    masked_account_number = EXCLUDED.masked_account_number;

-- name: ListLinkedAccounts :many
SELECT consent_handle, link_ref_number, fip_id, fip_name,
       account_type, fi_type, masked_account_number, linked_at
FROM idbi_aa_linked_accounts
WHERE consent_handle = $1
ORDER BY link_ref_number;

-- name: DeleteLinkedAccountsExcept :exec
DELETE FROM idbi_aa_linked_accounts
WHERE consent_handle = $1 AND link_ref_number <> ALL($2::text[]);

-- name: DeleteAllLinkedAccounts :exec
DELETE FROM idbi_aa_linked_accounts WHERE consent_handle = $1;
