#!/bin/sh

set -e

echo "Waiting for database to be ready..."
# A simple wait loop could go here, but Docker Compose "depends_on" with "condition: service_healthy" is better.

echo "Running database migrations..."
migrate -path /app/migrations -database "${DATABASE_URL}" up

echo "Running seed script..."
/app/seed_idbi_customers || echo "Seed script failed or already ran"

echo "Starting API server..."
exec /app/main
