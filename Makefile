DATABASE_URL ?= "postgres://postgres:password@localhost:5432/astra?sslmode=disable"

.PHONY: run run-idbi build test clean generate seed migrate-up migrate-down

# Core environment (no IDBI features, uses mock providers)
ENV_BASE := DATABASE_URL=$(DATABASE_URL) \
	JWT_SECRET=local-dev-jwt-secret \
	RM_JWT_SECRET=local-dev-rm-jwt-secret \
	PORT=8080 \
	LLM_PROVIDER=groq \
	RM_OTP_DEV_CODE=123456

# Build and run the main API (mock providers, no IDBI features)
run:
	$(ENV_BASE) go run cmd/api/main.go

# Run with ALL IDBI features enabled (sandbox mode).
# Uses the public IDBI sandbox + sandbox CKYC credentials from the testdata.
run-idbi:
	$(ENV_BASE) \
	IDBI_ACCOUNTS_ENABLED=true \
	IDBI_SPEND_ENABLED=true \
	IDBI_LOANS_ENABLED=true \
	IDBI_KYC_ENABLED=true \
	IDBI_CREDIT_SCORE_ENABLED=true \
	IDBI_LEADS_ENABLED=true \
	IDBI_AA_ENABLED=true \
	IDBI_RM_RISK_ENABLED=true \
	IDBI_BASE_URL=mock://sandbox \
	IDBI_CKYC_API_TOKEN=3420c172-fba9-44ac-ba95-2a9bb66f788f \
	IDBI_CKYC_PARENT_COMPANY=AAAAA8597P \
	IDBI_CKYC_BRANCH_CODE=HOBRANCH \
	IDBI_CKYC_SOURCE_SYSTEM=Finacle \
	IDBI_CKYC_APP_FORM_NO=FF01 \
	IDBI_LEAD_CHANNEL=Online \
	IDBI_LEAD_SOURCE=Website \
	IDBI_AA_REDIRECT_MODE=stub \
	IDBI_AA_PRODUCT_ID=TEST \
	IDBI_AA_VUA_SUFFIX=@onemoney \
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

# --- Deployment Commands ---
# EC2 SSH Configuration
BASTION_IP ?= 43.205.52.148
BASTION_USER ?= ec2-user
PRIVATE_EC2_IP ?= 10.0.x.x # REPLACE THIS WITH THE PRIVATE IP OF THE TARGET EC2
EC2_USER ?= ec2-user
PEM_FILE ?= ./zeyro-idbi.pem
DEPLOY_DIR ?= /home/$(EC2_USER)/astra-backend

deploy:
	@echo "1. Building Linux binaries locally..."
	@mkdir -p bin-prod
	@CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin-prod/main ./cmd/api/main.go
	@CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin-prod/seed_idbi_customers ./scripts/seed_idbi_customers.go
	@echo "2. Deploying to Private IP $(PRIVATE_EC2_IP) via Bastion $(BASTION_IP)..."
	@echo "3. Syncing files via rsync through bastion proxy..."
	@rsync -avz --delete \
		--exclude '.git' \
		--exclude 'bin' \
		--exclude '.env' \
		--exclude '.DS_Store' \
		-e "ssh -i $(PEM_FILE) -o StrictHostKeyChecking=no -J $(BASTION_USER)@$(BASTION_IP)" \
		. $(EC2_USER)@$(PRIVATE_EC2_IP):$(DEPLOY_DIR)
	@echo "4. Rebuilding and starting Docker containers on Private EC2..."
	@ssh -i $(PEM_FILE) -o StrictHostKeyChecking=no -J $(BASTION_USER)@$(BASTION_IP) $(EC2_USER)@$(PRIVATE_EC2_IP) \
		"cd $(DEPLOY_DIR) && docker compose -f docker-compose.prod.yml up -d --build"
	@echo "Deployment successful! API is now running on internal IP http://$(PRIVATE_EC2_IP)"

# --- SSM Deployment Commands ---
# EC2 SSM Configuration
EC2_INSTANCE_ID ?= i-0123456789abcdef0

deploy-ssm:
	@echo "1. Building Linux binaries locally..."
	@mkdir -p bin-prod
	@CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin-prod/main ./cmd/api/main.go
	@CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin-prod/seed_idbi_customers ./scripts/seed_idbi_customers.go
	@echo "2. Deploying to $(EC2_INSTANCE_ID) via AWS SSM..."
	@echo "3. Syncing files via rsync over SSM tunnel..."
	@rsync -avz --delete \
		--exclude '.git' \
		--exclude 'bin' \
		--exclude '.env' \
		--exclude '.DS_Store' \
		-e "ssh -i $(PEM_FILE) -o StrictHostKeyChecking=no -o ProxyCommand='aws ssm start-session --target %h --document-name AWS-StartSSHSession --parameters portNumber=%p'" \
		. $(EC2_USER)@$(EC2_INSTANCE_ID):$(DEPLOY_DIR)
	@echo "4. Rebuilding and starting Docker containers on EC2..."
	@ssh -i $(PEM_FILE) -o StrictHostKeyChecking=no -o ProxyCommand="aws ssm start-session --target %h --document-name AWS-StartSSHSession --parameters portNumber=%p" $(EC2_USER)@$(EC2_INSTANCE_ID) \
		"cd $(DEPLOY_DIR) && docker compose -f docker-compose.prod.yml up -d --build"
	@echo "SSM Deployment successful!"

# --- EKS Deployment Commands ---
# Usage: make deploy-eks DOCKER_USERNAME=yourdockerhubusername
DOCKER_USERNAME ?= fieryice24
IMAGE_TAG ?= $(DOCKER_USERNAME)/astra-backend-api:latest

deploy-eks:
	@echo "1. Building Docker image for EKS..."
	docker build -t $(IMAGE_TAG) .
	@echo "2. Pushing to Docker Hub..."
	docker push $(IMAGE_TAG)
	@echo "3. Applying Kubernetes manifests..."
	kubectl apply -f k8s/
	@echo "4. Restarting deployment to pull new image..."
	kubectl rollout restart deployment/astra-api
	@echo "EKS Deployment pipeline complete! Check pod status with 'kubectl get pods'."
