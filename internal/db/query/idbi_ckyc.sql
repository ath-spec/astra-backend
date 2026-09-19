-- name: UpsertCKYC :exec
INSERT INTO idbi_ckyc (
    user_id, pan, ckyc_available, ckyc_id, ckyc_name,
    ckyc_acc_type, ckyc_gen_date, id_types, verified_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, now())
ON CONFLICT (user_id, pan) DO UPDATE SET
    ckyc_available = EXCLUDED.ckyc_available,
    ckyc_id        = EXCLUDED.ckyc_id,
    ckyc_name      = EXCLUDED.ckyc_name,
    ckyc_acc_type  = EXCLUDED.ckyc_acc_type,
    ckyc_gen_date  = EXCLUDED.ckyc_gen_date,
    id_types       = EXCLUDED.id_types,
    verified_at    = now();

-- name: GetCKYC :one
SELECT id, user_id, pan, ckyc_available, ckyc_id, ckyc_name,
       ckyc_acc_type, ckyc_gen_date, id_types, verified_at
FROM idbi_ckyc
WHERE user_id = $1 AND pan = $2;

-- name: ListCKYCByUser :many
SELECT id, user_id, pan, ckyc_available, ckyc_id, ckyc_name,
       ckyc_acc_type, ckyc_gen_date, id_types, verified_at
FROM idbi_ckyc
WHERE user_id = $1
ORDER BY verified_at DESC;
