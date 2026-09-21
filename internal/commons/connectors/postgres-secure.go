package connectors

import (
	"context"
	"crypto/tls"
	"github.com/yourusername/astra-backend/internal/commons/logger"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// CreateSecurePostgresSession creates a PostgreSQL connection with TLS enabled
func CreateSecurePostgresSession(dsn string) *pgxpool.Pool {
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		logger.Panic("unable to parse postgres config: %s", err)
	}

	// Configure TLS
	config.ConnConfig.TLSConfig = &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: false, // Always verify certificates in production
	}

	// Connection pool settings
	config.MaxConns = 100
	config.MinConns = 5
	config.MaxConnLifetime = time.Hour
	config.MaxConnIdleTime = time.Minute * 30

	// Connection timeout
	config.ConnConfig.ConnectTimeout = time.Second * 10

	count := 0
	for {
		db, err := pgxpool.NewWithConfig(context.Background(), config)
		if err != nil {
			count++
			if count == 5 {
				logger.Error("unable to connect to postgres: %s", err)
				logger.Info("retrying in 5 seconds...")
				time.Sleep(time.Second * 5)
				count = 0
			}
		} else {
			logger.Info("connected to postgres with TLS!")
			return db
		}
	}
}

// CreatePostgresSessionForEnvironment creates appropriate connection based on environment
func CreatePostgresSessionForEnvironment(dsn string, environment string) *pgxpool.Pool {
	if environment == "production" || environment == "prod" {
		return CreateSecurePostgresSession(dsn)
	}
	// For development/staging, use regular connection
	return CreatePostgresSession(dsn)
}
