# Use the native platform (AMD64) for the builder to avoid QEMU emulation
FROM --platform=$BUILDPLATFORM golang:alpine AS builder

# Install build dependencies in the native builder
RUN apk add --no-cache curl tar ca-certificates tzdata

WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download all dependencies
RUN go mod download

# Copy the source code
COPY . .

# Build the application and the seed script for ARM64 EKS nodes
RUN CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o main ./cmd/api/main.go
RUN CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o seed_idbi_customers ./scripts/seed_idbi_customers.go

# Download the ARM64 migrate binary inside the AMD64 builder
RUN curl -L https://github.com/golang-migrate/migrate/releases/download/v4.16.2/migrate.linux-arm64.tar.gz | tar xvz && \
    mv migrate /usr/local/bin/migrate && \
    chmod +x /usr/local/bin/migrate

# --- Final Stage ---
# This stage pulls the ARM64 alpine image, but only runs COPY commands
# Since there are no RUN commands, Docker doesn't need QEMU to build this!
FROM alpine:latest  

WORKDIR /app

# Copy certificates and timezone data from builder
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo

# Copy the binaries and scripts
COPY --from=builder /app/main .
COPY --from=builder /app/seed_idbi_customers .
COPY --from=builder /app/start.sh .
COPY --from=builder /app/internal/database/migrations ./migrations
COPY --from=builder /usr/local/bin/migrate /usr/local/bin/migrate
COPY --from=builder ["/app/docs/IDBI APIs - Data.csv", "./docs/IDBI APIs - Data.csv"]

# Copy RDS global bundle for SSL
COPY global-bundle.pem /app/global-bundle.pem

# Expose port 8080
EXPOSE 8080

# Run the script
ENTRYPOINT ["/app/start.sh"]
