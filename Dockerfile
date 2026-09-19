FROM golang:alpine AS builder

WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download all dependencies
RUN go mod download

# Copy the source code
COPY . .

# Build the application and the seed script
RUN CGO_ENABLED=0 GOOS=linux go build -o main ./cmd/api/main.go
RUN CGO_ENABLED=0 GOOS=linux go build -o seed_idbi_customers ./scripts/seed_idbi_customers.go

# Start a new stage from scratch
FROM alpine:latest  
RUN apk --no-cache add ca-certificates tzdata curl bash

# Install golang-migrate
RUN curl -L https://github.com/golang-migrate/migrate/releases/download/v4.16.2/migrate.linux-amd64.tar.gz | tar xvz && \
    mv migrate /usr/local/bin/migrate && \
    chmod +x /usr/local/bin/migrate

WORKDIR /app

# Copy the Pre-built binary files from the previous stage
COPY --from=builder /app/main .
COPY --from=builder /app/seed_idbi_customers .
COPY --from=builder /app/start.sh .
COPY --from=builder /app/internal/database/migrations ./migrations
COPY --from=builder ["/app/docs/IDBI APIs - Data.csv", "./docs/IDBI APIs - Data.csv"]

# Expose port 8080 to the outside world
EXPOSE 8080

# Command to run the executable
ENTRYPOINT ["/app/start.sh"]
