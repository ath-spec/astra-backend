-- name: UpsertSeedCustomer :exec
INSERT INTO idbi_seed_customers (cust_id, pan_number, customer_name, account_type, account_code, updated_at)
VALUES ($1, $2, $3, $4, $5, now())
ON CONFLICT (cust_id) DO UPDATE SET
    pan_number    = EXCLUDED.pan_number,
    customer_name = EXCLUDED.customer_name,
    account_type  = EXCLUDED.account_type,
    account_code  = EXCLUDED.account_code,
    updated_at    = now();

-- name: GetSeedCustomer :one
SELECT cust_id, pan_number, customer_name, account_type, account_code, created_at, updated_at
FROM idbi_seed_customers
WHERE cust_id = $1;

-- name: GetSeedCustomerByPAN :one
SELECT cust_id, pan_number, customer_name, account_type, account_code, created_at, updated_at
FROM idbi_seed_customers
WHERE pan_number = $1
LIMIT 1;

-- name: ListSeedCustomersByAccountType :many
SELECT cust_id, pan_number, customer_name, account_type, account_code, created_at, updated_at
FROM idbi_seed_customers
WHERE account_type = $1
ORDER BY cust_id;

-- name: ListAllSeedCustomers :many
SELECT cust_id, pan_number, customer_name, account_type, account_code, created_at, updated_at
FROM idbi_seed_customers
ORDER BY account_type, cust_id;

-- name: CountSeedCustomers :one
SELECT COUNT(*) FROM idbi_seed_customers;
