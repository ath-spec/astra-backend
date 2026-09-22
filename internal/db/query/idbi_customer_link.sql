-- name: GetCustomerLink :one
SELECT user_id, cif_id, cust_id, source, linked_at, updated_at
FROM idbi_customer_link
WHERE user_id = $1;

-- name: GetCustomerLinkByCifID :one
SELECT user_id, cif_id, cust_id, source, linked_at, updated_at
FROM idbi_customer_link
WHERE cif_id = $1
LIMIT 1;

-- name: UpsertCustomerLink :exec
INSERT INTO idbi_customer_link (user_id, cif_id, cust_id, source, linked_at, updated_at)
VALUES ($1, $2, $3, $4, now(), now())
ON CONFLICT (user_id) DO UPDATE SET
    cif_id     = EXCLUDED.cif_id,
    cust_id    = CASE WHEN EXCLUDED.cust_id <> '' THEN EXCLUDED.cust_id ELSE idbi_customer_link.cust_id END,
    source     = EXCLUDED.source,
    updated_at = now();

-- name: DeleteCustomerLink :exec
DELETE FROM idbi_customer_link WHERE user_id = $1;
