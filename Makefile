DATABASE_URL ?= "postgres://postgres:password@localhost:5432/astra?sslmode=disable"

.PHONY: run build test clean generate seed migrate-up migrate-down

# Build and run the main API
run:
	go run cmd/api/main.go

# Build the API binary
build:
	mkdir -p bin
	go build -o bin/api cmd/api/main.go

# Run tests
test:
	go test -v ./...

# Clean build artifacts
clean:
	rm -rf bin/
	go clean

# Generate SQLC code
generate:
	sqlc generate

# Run the IDBI seed script
seed:
	go run scripts/seed_idbi_customers.go

# --- Database commands (Docker) ---
# Start the local PostgreSQL database using docker-compose
db-up:
	docker-compose up -d

# Stop the local PostgreSQL database and remove the container
db-down:
	docker-compose down

# Stop the local PostgreSQL database and WIPE all data (removes volume)
db-clean:
	docker-compose down -v

# --- Database commands (Migrations) ---
# Note: Migrations expect the DATABASE_URL environment variable to be set.
# You can define it in your .env file or run like:
# DATABASE_URL="postgres://postgres:password@localhost:5432/astra?sslmode=disable" make migrate-up

migrate-up:
	migrate -path internal/database/migrations -database $(DATABASE_URL) up

migrate-down:
	migrate -path internal/database/migrations -database $(DATABASE_URL) down

# Database management (runs inside the docker container)
# Usage: make createdb DB_NAME=astra
createdb: db-up
	docker-compose exec -T db createdb --username=postgres --owner=postgres $${DB_NAME:-astra}

dropdb: db-up
	docker-compose exec -T db dropdb --username=postgres $${DB_NAME:-astra}
