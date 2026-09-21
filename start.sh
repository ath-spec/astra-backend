#!/bin/sh

set -e

echo "[$(date)] Starting initialization script..."
echo "[$(date)] Attempting to connect to the database and run migrations..."
echo "[$(date)] WARNING: If this step hangs or fails, your EKS worker nodes are being blocked by the RDS Security Group."

# Use a 15-second timeout so it fails quickly with an error instead of hanging silently forever
if ! timeout 15s migrate -path /app/migrations -database "${DATABASE_URL}" up; then
    echo "[$(date)] FATAL ERROR: Database connection timed out after 15 seconds!"
    echo "[$(date)] ROOT CAUSE: EKS Worker Nodes do not have network access to the RDS instance."
    echo "[$(date)] ACTION REQUIRED: Add an Inbound Rule (Port 5432) to the RDS Security Group allowing traffic from the EKS Node Security Group."
    exit 1
fi

echo "[$(date)] Database migrations ran successfully!"

echo "[$(date)] Running seed script..."
/app/seed_idbi_customers || echo "[$(date)] Seed script failed or already ran"

echo "[$(date)] Starting API server..."
exec /app/main
